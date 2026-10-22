package workbuddy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

const creditsFixture = `{"data":{"models":[
	{"id":"deepseek-v4.1-flash","credits":"x0.00"},
	{"id":"glm-5.3","credits":"x0.79"},
	{"id":"gpt-5.5","credits":"x3.31"}
]}}`

// newQuotaTestAuth builds an auth pointed at a stub server.
func newQuotaTestAuth(server *httptest.Server, provider string) *cliproxyauth.Auth {
	return &cliproxyauth.Auth{
		ID:       "wb-quota-test",
		Provider: provider,
		Attributes: map[string]string{
			"access_token": "mock-token",
			"base_url":     server.URL,
		},
	}
}

// TestQueryQuotaReportsCreditMultipliers verifies the quota response carries the
// real per-realm multipliers rather than only a free-tier flag.
func TestQueryQuotaReportsCreditMultipliers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case UserPointsPath:
			_, _ = w.Write([]byte(`{"code":0,"data":{"vip_type":"VIP1","total_points":100,"remaining_points":80}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	restore := registry.SetWorkBuddyCreditsFetcherForTest(
		func(string) ([]byte, bool) { return []byte(creditsFixture), true },
		nil,
	)
	defer restore()
	registry.InvalidateWorkBuddyCredits("")

	auth := newQuotaTestAuth(server, "workbuddy")
	res, err := QueryQuota(context.Background(), server.Client(), auth)
	if err != nil {
		t.Fatalf("QueryQuota: %v", err)
	}

	metrics := make(map[string]float64, len(res.Summary))
	for _, m := range res.Summary {
		metrics[m.Key] = m.Value
	}

	if got, ok := metrics["credits_free_count"]; !ok || got != 1 {
		t.Errorf("credits_free_count = %v (present=%v), want 1", got, ok)
	}
	if got, ok := metrics["credits_paid_count"]; !ok || got != 2 {
		t.Errorf("credits_paid_count = %v (present=%v), want 2", got, ok)
	}
	if got, ok := metrics["credits_rate_min"]; !ok || got != 0.79 {
		t.Errorf("credits_rate_min = %v (present=%v), want 0.79", got, ok)
	}
	if got, ok := metrics["credits_rate_max"]; !ok || got != 3.31 {
		t.Errorf("credits_rate_max = %v (present=%v), want 3.31", got, ok)
	}
}

// TestQueryQuotaOmitsCreditMetricsWhenCatalogUnknown ensures an unavailable
// catalogue reports nothing rather than implying everything is free.
func TestQueryQuotaOmitsCreditMetricsWhenCatalogUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case UserPointsPath:
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_points":100,"remaining_points":80}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	restore := registry.SetWorkBuddyCreditsFetcherForTest(
		func(string) ([]byte, bool) { return nil, false },
		nil,
	)
	defer restore()
	registry.InvalidateWorkBuddyCredits("")

	auth := newQuotaTestAuth(server, "workbuddy")
	res, err := QueryQuota(context.Background(), server.Client(), auth)
	if err != nil {
		t.Fatalf("QueryQuota: %v", err)
	}

	for _, m := range res.Summary {
		switch m.Key {
		case "credits_free_count", "credits_paid_count", "credits_rate_min", "credits_rate_max":
			t.Errorf("metric %q present without a catalogue", m.Key)
		}
	}
}

func TestAccessTokenAndResolveUserID(t *testing.T) {
	auth := &cliproxyauth.Auth{
		ID:       "fallback-id",
		Metadata: map[string]any{"access_token": " tok ", "user_id": "u-42"},
	}
	if got := AccessToken(auth); got != "tok" {
		t.Errorf("AccessToken = %q, want tok", got)
	}
	if got := ResolveUserID(auth); got != "u-42" {
		t.Errorf("ResolveUserID = %q, want u-42", got)
	}

	// Falls back to the auth ID when no metadata uid exists.
	if got := ResolveUserID(&cliproxyauth.Auth{ID: "only-id"}); got != "only-id" {
		t.Errorf("ResolveUserID fallback = %q, want only-id", got)
	}
	if got := ResolveUserID(nil); got != "anonymous" {
		t.Errorf("ResolveUserID(nil) = %q, want anonymous", got)
	}
}

// TestApplyCatalogHeadersAddsUserID covers the extra header the desktop client
// sends when reading its catalogue.
func TestApplyCatalogHeadersAddsUserID(t *testing.T) {
	auth := &cliproxyauth.Auth{
		ID:       "auth-1",
		Provider: "workbuddy",
		Metadata: map[string]any{"user_id": "u-42"},
	}
	req, err := http.NewRequest(http.MethodGet, "https://www.workbuddy.ai/v3/config", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	ApplyCatalogHeaders(req, auth, "tok")

	if got := req.Header.Get("X-User-Id"); got != "u-42" {
		t.Errorf("X-User-Id = %q, want u-42", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("Authorization = %q, want Bearer tok", got)
	}
	// Ordinary API headers must still be present.
	if got := req.Header.Get("X-Product"); got == "" {
		t.Error("X-Product missing")
	}

	// With no resolvable user ID the header is omitted rather than sent as
	// "anonymous".
	anon := &cliproxyauth.Auth{ID: ""}
	reqAnon, errAnon := http.NewRequest(http.MethodGet, "https://www.workbuddy.ai/v3/config", nil)
	if errAnon != nil {
		t.Fatalf("build request: %v", errAnon)
	}
	ApplyCatalogHeaders(reqAnon, anon, "tok")
	if got := reqAnon.Header.Get("X-User-Id"); got != "" {
		t.Errorf("X-User-Id for anonymous = %q, want empty", got)
	}
}

func TestQueryQuotaDynamicPackageDiscovery(t *testing.T) {
	customCode := "TCACA_code_999_DynamicDiscovered"
	var receivedDynamicCodeInPaidQuery bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case MeterUserResourceSummaryPath:
			_, _ = w.Write([]byte(`{"code":0,"data":{"IsPaidUser":true,"Packages":[{"PackageCode":"` + customCode + `","CycleTotalCapacity":"200.00"}]}}`))
		case MeterPaidPackagesPath:
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), customCode) {
				receivedDynamicCodeInPaidQuery = true
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"TotalCount":1,"Accounts":[{"PackageCode":"` + customCode + `","CapacityType":"1","CycleCapacityRemain":"150.00","DeductionEndTime":1900000000000}]}}`))
		case MeterFreePackagesPath:
			_, _ = w.Write([]byte(`{"code":0,"data":{"TotalCount":0,"Accounts":[]}}`))
		case UserPointsPath:
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_points":200,"remaining_points":150}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	auth := newQuotaTestAuth(server, "workbuddy")
	res, err := QueryQuota(context.Background(), server.Client(), auth)
	if err != nil {
		t.Fatalf("QueryQuota error: %v", err)
	}

	if !receivedDynamicCodeInPaidQuery {
		t.Errorf("expected dynamic package code %q to be included in MeterPaidPackages request", customCode)
	}

	var hasExpiresAt bool
	for _, m := range res.Summary {
		if m.Key == "expires_at" || m.Key == "runs_out_at" {
			hasExpiresAt = true
			break
		}
	}
	if !hasExpiresAt {
		t.Errorf("expected expires_at or runs_out_at in quota summary from dynamic package query")
	}
}
