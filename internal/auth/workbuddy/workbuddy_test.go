package workbuddy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

	// DeriveUUID should be a valid UUID shape with hyphens
	uuidVal := DeriveUUID("machine", "uid123")
	if len(uuidVal) != 36 || strings.Count(uuidVal, "-") != 4 {
		t.Fatalf("DeriveUUID shape invalid: %q", uuidVal)
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
			name: "base_url contains workbuddy.cn",
			auth: &cliproxyauth.Auth{
				Attributes: map[string]string{"base_url": "https://www.workbuddy.cn"},
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

func TestResolveBaseURL_CnUpstreamOverride(t *testing.T) {
	// 1. Unset returns default
	t.Setenv("WB_CN_CHAT_UPSTREAM", "")
	if got := ResolveBaseURL(RealmCN); got != BaseURLCN {
		t.Errorf("ResolveBaseURL(RealmCN) with unset env = %q, want %q", got, BaseURLCN)
	}
	if got := ResolveDomain(RealmCN); got != DomainCN {
		t.Errorf("ResolveDomain(RealmCN) = %q, want %q", got, DomainCN)
	}

	// 2. Valid override overrides BaseURLCN while X-Domain stays DomainCN
	t.Setenv("WB_CN_CHAT_UPSTREAM", "https://www.workbuddy.cn/")
	if got := ResolveBaseURL(RealmCN); got != "https://www.workbuddy.cn" {
		t.Errorf("ResolveBaseURL(RealmCN) with override = %q, want https://www.workbuddy.cn", got)
	}
	if got := ResolveDomain(RealmCN); got != DomainCN {
		t.Errorf("ResolveDomain(RealmCN) should not change, got %q, want %q", got, DomainCN)
	}

	// 3. Non-http URL is ignored and falls back to default
	t.Setenv("WB_CN_CHAT_UPSTREAM", "ftp://www.workbuddy.cn")
	if got := ResolveBaseURL(RealmCN); got != BaseURLCN {
		t.Errorf("ResolveBaseURL(RealmCN) with invalid ftp env = %q, want %q", got, BaseURLCN)
	}

	// 4. International realm remains untouched
	if got := ResolveBaseURL(RealmIntl); got != BaseURLIntl {
		t.Errorf("ResolveBaseURL(RealmIntl) = %q, want %q", got, BaseURLIntl)
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
	if got := req.Header.Get("X-Product"); got != "SaaS" {
		t.Errorf("X-Product = %q, want SaaS", got)
	}
	if got := req.Header.Get("X-IDE-Version"); got != "5.8.4" {
		t.Errorf("X-IDE-Version = %q, want 5.8.4", got)
	}
	if got := req.Header.Get("User-Agent"); got != DefaultUserAgentIntl {
		t.Errorf("User-Agent = %q, want %q", got, DefaultUserAgentIntl)
	}
	if got := req.Header.Get("X-Agent-Intent"); got != "craft" {
		t.Errorf("X-Agent-Intent = %q, want craft", got)
	}
	if got := req.Header.Get("X-Private-Data"); got != "true" {
		t.Errorf("X-Private-Data = %q, want true", got)
	}
	// Verify conversation header family (critical for upstream channel validation)
	convID := req.Header.Get("X-Conversation-ID")
	if len(convID) != 36 || strings.Count(convID, "-") != 4 {
		t.Errorf("X-Conversation-ID shape invalid: %q", convID)
	}
	convReqID := req.Header.Get("X-Conversation-Request-ID")
	rootReqID := req.Header.Get("X-Root-Request-ID")
	if len(convReqID) != 32 || convReqID != rootReqID {
		t.Errorf("X-Conversation-Request-ID (%q) != X-Root-Request-ID (%q)", convReqID, rootReqID)
	}
	msgID := req.Header.Get("X-Conversation-Message-ID")
	reqID := req.Header.Get("X-Request-ID")
	if len(msgID) != 32 || msgID != reqID {
		t.Errorf("X-Conversation-Message-ID (%q) != X-Request-ID (%q)", msgID, reqID)
	}
	if req.Header.Get("X-Trace-ID") == "" || req.Header.Get("traceparent") == "" {
		t.Error("Distributed tracing headers missing")
	}
	// Machine-ID and Session-ID must not be sent on chat completions (triggers upstream 11128)
	if got := req.Header.Get("X-Machine-ID"); got != "" {
		t.Errorf("X-Machine-ID must not be sent on chat completions, got %q", got)
	}
	if got := req.Header.Get("X-Session-ID"); got != "" {
		t.Errorf("X-Session-ID must not be sent on chat completions, got %q", got)
	}
}

func TestApplyHeaders_CN(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://copilot.tencent.com/v2/chat/completions", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	auth := &cliproxyauth.Auth{
		ID:       "test-cn-user",
		Provider: "workbuddy-cn",
		Metadata: map[string]any{
			"user_id": "u-cn-42",
			"realm":   "cn",
		},
	}

	ApplyHeaders(req, auth, "cn-access-token")

	if got := req.Header.Get("X-Domain"); got != DomainCN {
		t.Errorf("X-Domain = %q, want %q", got, DomainCN)
	}
	if got := req.Header.Get("X-IDE-Version"); got != "5.8.3" {
		t.Errorf("X-IDE-Version = %q, want 5.8.3", got)
	}
	if got := req.Header.Get("User-Agent"); got != DefaultUserAgentCN {
		t.Errorf("User-Agent = %q, want %q", got, DefaultUserAgentCN)
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

	if res.Subscription == nil || res.Subscription.Plan != "WorkBuddy (cn) · Buddy加油站 (Season 10)" {
		t.Errorf("Subscription.Plan = %v, want WorkBuddy (cn) · Buddy加油站 (Season 10)", res.Subscription)
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
