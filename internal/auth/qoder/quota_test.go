package qoder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestQueryQuota_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != QuotaUsagePath {
			http.NotFound(w, r)
			return
		}
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-qoder-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 401, "msg": "invalid token"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"total":     1000,
				"used":      250,
				"remaining": 750,
				"tier":      "Pro",
				"expire_at": 1780000000,
			},
		})
	}))
	defer srv.Close()

	t.Setenv("QODER_INTL_OPENAPI", srv.URL)

	auth := &cliproxyauth.Auth{
		ID:       "test-qoder",
		Provider: "qoder",
		Attributes: map[string]string{
			"access_token": "Bearer test-qoder-token",
		},
	}

	resp, err := QueryQuota(context.Background(), srv.Client(), auth)
	if err != nil {
		t.Fatalf("QueryQuota failed: %v", err)
	}

	if resp.Subscription == nil || resp.Subscription.Plan != "Qoder Global (en) · Pro" {
		t.Errorf("unexpected plan: %#v", resp.Subscription)
	}

	if len(resp.Groups) != 1 || len(resp.Groups[0].Buckets) != 1 {
		t.Fatalf("unexpected quota groups: %#v", resp.Groups)
	}
	bucket := resp.Groups[0].Buckets[0]
	if bucket.RemainingFraction != 0.75 {
		t.Errorf("RemainingFraction = %v, want 0.75", bucket.RemainingFraction)
	}
	if bucket.ResetTime == "" {
		t.Errorf("ResetTime is empty")
	}

	metricMap := make(map[string]float64)
	for _, m := range resp.Summary {
		metricMap[m.Key] = m.Value
	}
	if metricMap["points_remain"] != 750 {
		t.Errorf("points_remain = %v, want 750", metricMap["points_remain"])
	}
	if metricMap["points_total"] != 1000 {
		t.Errorf("points_total = %v, want 1000", metricMap["points_total"])
	}
	if metricMap["points_used"] != 250 {
		t.Errorf("points_used = %v, want 250", metricMap["points_used"])
	}
}

func TestQueryQuota_AlternativeFieldsAndFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 200,
			"data": map[string]any{
				"total_points": 500,
				"used_points":  100,
				"level":        "Enterprise",
				"expires_at":   int64(1780000000000), // millisecond timestamp
			},
		})
	}))
	defer srv.Close()

	t.Setenv("QODER_CN_OPENAPI", srv.URL)

	auth := &cliproxyauth.Auth{
		ID:       "test-qoder-cn",
		Provider: "qoder-cn",
		Metadata: map[string]any{
			"device_token": "cn-device-token",
		},
	}

	resp, err := QueryQuota(context.Background(), srv.Client(), auth)
	if err != nil {
		t.Fatalf("QueryQuota failed: %v", err)
	}

	if resp.Subscription == nil || resp.Subscription.Plan != "Qoder Mainland (cn) · Enterprise" {
		t.Errorf("unexpected plan: %#v", resp.Subscription)
	}

	metricMap := make(map[string]float64)
	for _, m := range resp.Summary {
		metricMap[m.Key] = m.Value
	}
	// Fallback calculation: remaining = 500 - 100 = 400
	if metricMap["points_remain"] != 400 {
		t.Errorf("points_remain = %v, want 400", metricMap["points_remain"])
	}
}

func TestQueryQuota_PointsBalanceFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"balance": 88.5,
			},
		})
	}))
	defer srv.Close()

	t.Setenv("QODER_INTL_OPENAPI", srv.URL)

	auth := &cliproxyauth.Auth{
		ID:       "test-qoder-pat",
		Provider: "qoder",
		Attributes: map[string]string{
			"api_key": "my-pat-key",
		},
	}

	resp, err := QueryQuota(context.Background(), srv.Client(), auth)
	if err != nil {
		t.Fatalf("QueryQuota failed: %v", err)
	}

	if resp.Subscription == nil || !strings.Contains(resp.Subscription.Plan, "Free") {
		t.Errorf("expected Free plan, got %#v", resp.Subscription)
	}

	var foundRemain bool
	for _, m := range resp.Summary {
		if m.Key == "points_remain" {
			foundRemain = true
			if m.Value != 88.5 {
				t.Errorf("points_remain = %v, want 88.5", m.Value)
			}
		}
	}
	if !foundRemain {
		t.Errorf("points_remain metric not found")
	}
}

func TestQueryQuota_Errors(t *testing.T) {
	// 1. Nil auth
	if _, err := QueryQuota(context.Background(), nil, nil); err == nil {
		t.Errorf("expected error for nil auth")
	}

	// 2. Auth without access token
	emptyAuth := &cliproxyauth.Auth{ID: "empty", Provider: "qoder"}
	if _, err := QueryQuota(context.Background(), nil, emptyAuth); err == nil {
		t.Errorf("expected error for auth with no tokens")
	}

	// 3. API error code
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 5001,
			"msg":  "quota service unavailable",
		})
	}))
	defer srv.Close()

	t.Setenv("QODER_INTL_OPENAPI", srv.URL)

	auth := &cliproxyauth.Auth{
		ID:       "test-qoder-err",
		Provider: "qoder",
		Attributes: map[string]string{
			"token": "valid-token",
		},
	}
	if _, err := QueryQuota(context.Background(), srv.Client(), auth); err == nil {
		t.Errorf("expected error for API error code")
	}
}

func TestQueryQuota_RealUpstreamUserQuotaFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"userId":               "01a15e15-7421-7847-99f8-eaf5298e1804",
			"userType":             "personal_professional_trial",
			"usageType":            "credits",
			"totalUsagePercentage": 0.02,
			"isQuotaExceeded":      false,
			"expiresAt":            1793789426886,
			"userQuota": map[string]any{
				"total":      300.0,
				"used":       13.0,
				"remaining":  287.0,
				"percentage": 0.05,
				"unit":       "credits",
			},
			"addOnQuota": map[string]any{
				"total":      300.0,
				"used":       0.0,
				"remaining":  300.0,
				"percentage": 0.0,
				"unit":       "credits",
			},
		})
	}))
	defer srv.Close()

	t.Setenv("QODER_CN_OPENAPI", srv.URL)

	auth := &cliproxyauth.Auth{
		ID:       "test-qoder-cn",
		Provider: "qoder-cn",
		Attributes: map[string]string{
			"access_token": "Bearer dt-test-token",
			"realm":        "cn",
		},
	}

	resp, err := QueryQuota(context.Background(), srv.Client(), auth)
	if err != nil {
		t.Fatalf("QueryQuota failed on real upstream format: %v", err)
	}

	if resp.Subscription == nil || resp.Subscription.Plan != "Qoder Mainland (cn) · Pro Trial" {
		t.Errorf("unexpected plan: %#v", resp.Subscription)
	}

	metricMap := make(map[string]float64)
	for _, m := range resp.Summary {
		metricMap[m.Key] = m.Value
	}

	// 287 (userQuota) + 300 (addOnQuota) = 587
	if metricMap["points_remain"] != 587 {
		t.Errorf("points_remain = %v, want 587", metricMap["points_remain"])
	}
	// 300 + 300 = 600
	if metricMap["points_total"] != 600 {
		t.Errorf("points_total = %v, want 600", metricMap["points_total"])
	}
	// 13 + 0 = 13
	if metricMap["points_used"] != 13 {
		t.Errorf("points_used = %v, want 13", metricMap["points_used"])
	}
	if metricMap["user_quota_remain"] != 287 {
		t.Errorf("user_quota_remain = %v, want 287", metricMap["user_quota_remain"])
	}
	if metricMap["addon_quota_remain"] != 300 {
		t.Errorf("addon_quota_remain = %v, want 300", metricMap["addon_quota_remain"])
	}

	if len(resp.Groups) == 0 || len(resp.Groups[0].Buckets) == 0 {
		t.Fatalf("expected non-empty quota buckets")
	}
	b := resp.Groups[0].Buckets[0]
	// 587 / 600 approx 0.9783
	if b.RemainingFraction < 0.97 || b.RemainingFraction > 0.98 {
		t.Errorf("expected RemainingFraction ~0.978, got %v", b.RemainingFraction)
	}
	if b.ResetTime == "" {
		t.Errorf("expected non-empty ResetTime from expiresAt ms")
	}
}
