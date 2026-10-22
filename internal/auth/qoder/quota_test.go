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
