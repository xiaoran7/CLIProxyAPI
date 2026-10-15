package workbuddy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestDeriveID(t *testing.T) {
	// DeriveID("machine", "uid123") should be deterministic MD5 hex
	id1 := DeriveID("machine", "uid123")
	id2 := DeriveID("machine", "uid123")
	if id1 != id2 {
		t.Fatalf("DeriveID is not deterministic: %q != %q", id1, id2)
	}
	if len(id1) != 32 {
		t.Fatalf("DeriveID length = %d, want 32", len(id1))
	}

	// Different salt should yield different ID
	idSession := DeriveID("session", "uid123")
	if id1 == idSession {
		t.Fatalf("DeriveID with different salt yielded same ID: %q", id1)
	}

	// Different UID should yield different ID
	idOther := DeriveID("machine", "uid456")
	if id1 == idOther {
		t.Fatalf("DeriveID with different uid yielded same ID: %q", id1)
	}
}

func TestResolveRealm(t *testing.T) {
	tests := []struct {
		name string
		auth *cliproxyauth.Auth
		want string
	}{
		{
			name: "nil auth defaults to intl",
			auth: nil,
			want: RealmIntl,
		},
		{
			name: "attributes realm cn",
			auth: &cliproxyauth.Auth{
				Attributes: map[string]string{"realm": "cn"},
			},
			want: RealmCN,
		},
		{
			name: "metadata realm cn",
			auth: &cliproxyauth.Auth{
				Metadata: map[string]any{"realm": "cn"},
			},
			want: RealmCN,
		},
		{
			name: "provider workbuddy-cn",
			auth: &cliproxyauth.Auth{
				Provider: "workbuddy-cn",
			},
			want: RealmCN,
		},
		{
			name: "base_url contains tencent.com",
			auth: &cliproxyauth.Auth{
				Attributes: map[string]string{"base_url": "https://copilot.tencent.com"},
			},
			want: RealmCN,
		},
		{
			name: "default intl",
			auth: &cliproxyauth.Auth{
				Provider: "workbuddy",
			},
			want: RealmIntl,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveRealm(tt.auth)
			if got != tt.want {
				t.Errorf("ResolveRealm() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://www.workbuddy.ai/v2/chat/completions", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	auth := &cliproxyauth.Auth{
		ID:       "test-user-42",
		Provider: "workbuddy",
		Metadata: map[string]any{
			"user_id": "u-42",
			"realm":   "intl",
		},
	}

	ApplyHeaders(req, auth, "test-access-token")

	if got := req.Header.Get("Authorization"); got != "Bearer test-access-token" {
		t.Errorf("Authorization = %q, want Bearer test-access-token", got)
	}
	if got := req.Header.Get("X-IDE-Name"); got != DefaultIDEName {
		t.Errorf("X-IDE-Name = %q, want %q", got, DefaultIDEName)
	}
	if got := req.Header.Get("X-Domain"); got != DomainIntl {
		t.Errorf("X-Domain = %q, want %q", got, DomainIntl)
	}
	if got := req.Header.Get("X-Machine-ID"); got != DeriveID("machine", "u-42") {
		t.Errorf("X-Machine-ID = %q, want %q", got, DeriveID("machine", "u-42"))
	}
	if got := req.Header.Get("X-Session-ID"); got != DeriveID("session", "u-42") {
		t.Errorf("X-Session-ID = %q, want %q", got, DeriveID("session", "u-42"))
	}
	if req.Header.Get("X-Request-ID") == "" {
		t.Error("X-Request-ID is empty")
	}
}

func TestRefreshToken(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		if r.Header.Get("X-Refresh-Token") != "valid-refresh" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code": 401, "msg": "invalid token"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Return Tencent standard envelope
		_, _ = w.Write([]byte(`{
			"code": 0,
			"msg": "success",
			"data": {
				"accessToken": "new-access-123",
				"refreshToken": "new-refresh-456",
				"expires_in": 3600
			}
		}`))
	}))
	defer server.Close()

	// Intercept BaseURLIntl by mocking client roundtripper or hitting server directly
	customClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = server.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	data, err := RefreshToken(context.Background(), customClient, RealmIntl, "valid-refresh", "u-1")
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}

	if data.AccessToken != "new-access-123" {
		t.Errorf("AccessToken = %q, want new-access-123", data.AccessToken)
	}
	if data.RefreshToken != "new-refresh-456" {
		t.Errorf("RefreshToken = %q, want new-refresh-456", data.RefreshToken)
	}
	if data.ExpiresIn != 3600 {
		t.Errorf("ExpiresIn = %d, want 3600", data.ExpiresIn)
	}
}

func TestRefreshTokenSingleflight(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		time.Sleep(30 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken": "tok-concurrent", "refreshToken": "ref-concurrent"}`))
	}))
	defer server.Close()

	customClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = server.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = RefreshToken(context.Background(), customClient, "singleflight-realm", "same-refresh-token", "u-1")
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&callCount); got != 1 {
		t.Fatalf("Upstream called %d times; singleflight should have deduplicated to 1", got)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCheckinEngine_EnabledToggle(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewCheckinEngine(tmpDir, nil)
	if !engine.IsEnabled() {
		t.Fatalf("new engine should be enabled by default")
	}

	engine.SetEnabled(false)
	if engine.IsEnabled() {
		t.Fatalf("engine should be disabled after SetEnabled(false)")
	}

	// Reload from disk to verify persistence
	engineReloaded := NewCheckinEngine(tmpDir, nil)
	if engineReloaded.IsEnabled() {
		t.Fatalf("reloaded engine should retain disabled state from disk")
	}

	engineReloaded.SetEnabled(true)
	if !engineReloaded.IsEnabled() {
		t.Fatalf("engine should be enabled after SetEnabled(true)")
	}

	engineReloaded2 := NewCheckinEngine(tmpDir, nil)
	if !engineReloaded2.IsEnabled() {
		t.Fatalf("reloaded engine should retain enabled state from disk")
	}
}

func TestQueryQuota_ActivityStatusFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case UserPointsPath:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code": 404, "msg": "route not found"}`))
		case "/v2/billing/meter/checkin-activity-status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"code": 0,
				"msg": "ok",
				"data": {
					"active": true,
					"total_credits": 200,
					"daily_credit": 100,
					"theme_name": "Buddy加油站",
					"season": 10
				}
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	customClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = server.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	auth := &cliproxyauth.Auth{
		ID:       "wb-test-1",
		Provider: "workbuddy-cn",
		Attributes: map[string]string{
			"access_token": "mock-token",
			"base_url":     server.URL,
		},
	}

	res, err := QueryQuota(context.Background(), customClient, auth)
	if err != nil {
		t.Fatalf("QueryQuota returned unexpected error: %v", err)
	}

	if res.Subscription == nil || res.Subscription.Plan != "WorkBuddy (cn) · Buddy加油站 (第10季)" {
		t.Errorf("Subscription.Plan = %v, want WorkBuddy (cn) · Buddy加油站 (第10季)", res.Subscription)
	}

	var foundPoints bool
	for _, m := range res.Summary {
		if m.Key == "points_remain" && m.Value == 200 {
			foundPoints = true
			break
		}
	}
	if !foundPoints {
		t.Errorf("Summary points_remain 200 not found in %v", res.Summary)
	}
}

func TestQueryQuota_UserResourceSummaryAndPackages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case MeterUserResourceSummaryPath:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"code": 0,
				"msg": "ok",
				"data": {
					"IsPaidUser": true,
					"Packages": [
						{
							"PackageCode": "TCACA_code_002_AkiJS3ZHF5",
							"CycleTotalCapacity": "500.00000000",
							"CycleUsedCapacity": "61.12000000",
							"CycleRemainCapacity": "438.88000000"
						}
					]
				}
			}`))
		case MeterPaidPackagesPath:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"code": 0,
				"msg": "ok",
				"data": {
					"TotalCount": 1,
					"Accounts": [
						{
							"PackageCode": "TCACA_code_002_AkiJS3ZHF5",
							"CycleCapacityRemainPrecise": "438.88000000",
							"ExpiredTime": "2026-11-20 18:00:00",
							"CapacityType": "1"
						}
					]
				}
			}`))
		case MeterFreePackagesPath:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"code": 0,
				"msg": "ok",
				"data": {
					"TotalCount": 0,
					"Accounts": []
				}
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	customClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = server.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	auth := &cliproxyauth.Auth{
		ID:       "wb-summary-test",
		Provider: "workbuddy",
		Attributes: map[string]string{
			"access_token": "mock-token",
			"base_url":     server.URL,
		},
	}

	res, err := QueryQuota(context.Background(), customClient, auth)
	if err != nil {
		t.Fatalf("QueryQuota returned error: %v", err)
	}

	if res.Subscription == nil || res.Subscription.Plan != "WorkBuddy (en) · Pro" {
		t.Errorf("Subscription.Plan = %v, want WorkBuddy (en) · Pro", res.Subscription)
	}

	var remain, total, used, runsOut float64
	var expiresAt string
	for _, m := range res.Summary {
		switch m.Key {
		case "points_remain":
			remain = m.Value
		case "points_total":
			total = m.Value
		case "points_used":
			used = m.Value
		case "runs_out_points":
			runsOut = m.Value
		case "expires_at":
			expiresAt = m.Unit
		}
	}

	if remain != 438.88 {
		t.Errorf("remain = %v, want 438.88", remain)
	}
	if total != 500 {
		t.Errorf("total = %v, want 500", total)
	}
	if used != 61.12 {
		t.Errorf("used = %v, want 61.12", used)
	}
	if runsOut != 438.88 {
		t.Errorf("runsOut = %v, want 438.88", runsOut)
	}
	if expiresAt != "2026-11-20 18:00:00" {
		t.Errorf("expiresAt = %q, want 2026-11-20 18:00:00", expiresAt)
	}
}

func TestQueryQuota_EnterpriseUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case MeterUserResourceSummaryPath:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"code": 0,
				"msg": "ok",
				"data": {
					"IsPaidUser": false,
					"Packages": []
				}
			}`))
		case MeterEnterpriseUsagePath:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"code": 0,
				"msg": "ok",
				"data": {
					"limitNum": 1000,
					"credit": 250,
					"cycleResetTime": "2026-12-01 00:00:00"
				}
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	customClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = server.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	auth := &cliproxyauth.Auth{
		ID:       "wb-ent-test",
		Provider: "workbuddy-cn",
		Attributes: map[string]string{
			"access_token": "mock-token",
			"base_url":     server.URL,
		},
	}

	res, err := QueryQuota(context.Background(), customClient, auth)
	if err != nil {
		t.Fatalf("QueryQuota returned error: %v", err)
	}

	if res.Subscription == nil || res.Subscription.Plan != "WorkBuddy (cn) · Enterprise" {
		t.Errorf("Subscription.Plan = %v, want WorkBuddy (cn) · Enterprise", res.Subscription)
	}

	var remain, total, used float64
	var expiresAt string
	for _, m := range res.Summary {
		switch m.Key {
		case "points_remain":
			remain = m.Value
		case "points_total":
			total = m.Value
		case "points_used":
			used = m.Value
		case "expires_at":
			expiresAt = m.Unit
		}
	}

	if remain != 750 {
		t.Errorf("remain = %v, want 750", remain)
	}
	if total != 1000 {
		t.Errorf("total = %v, want 1000", total)
	}
	if used != 250 {
		t.Errorf("used = %v, want 250", used)
	}
	if expiresAt != "2026-12-01 00:00:00" {
		t.Errorf("expiresAt = %q, want 2026-12-01 00:00:00", expiresAt)
	}
}
