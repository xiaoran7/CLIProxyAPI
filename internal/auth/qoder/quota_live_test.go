package qoder

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestLiveQoderUpstream_IfCredentialPresent(t *testing.T) {
	raw, err := os.ReadFile("/tmp/qoder_test_auth.json")
	if err != nil {
		t.Skip("skipping live test: /tmp/qoder_test_auth.json not found")
	}

	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Skip("skipping live test: invalid json in /tmp/qoder_test_auth.json")
	}

	auth := &cliproxyauth.Auth{
		ID:       "test-qoder-cn",
		Provider: "qoder-cn",
		Metadata: meta,
		Attributes: map[string]string{
			"user_id":       getString(meta, "user_id"),
			"access_token":  getString(meta, "access_token"),
			"device_token":  getString(meta, "device_token"),
			"refresh_token": getString(meta, "refresh_token"),
			"realm":         "cn",
			"base_url":      getString(meta, "base_url"),
			"openapi_base":  getString(meta, "openapi_base"),
			"machine_id":    getString(meta, "machine_id"),
			"nickname":      getString(meta, "nickname"),
		},
	}

	client := &http.Client{Timeout: 15 * time.Second}
	ctx := context.Background()

	// 1. Live QueryQuota
	quotaResp, errQuota := QueryQuota(ctx, client, auth)
	if errQuota != nil {
		t.Fatalf("Live QueryQuota failed: %v", errQuota)
	}
	t.Logf("Live QueryQuota SUCCESS: Plan = %s", quotaResp.Subscription.Plan)
	for _, m := range quotaResp.Summary {
		t.Logf("  Metric: %s = %v %s", m.Key, m.Value, m.Unit)
	}
	if quotaResp.Subscription.Plan != "Qoder Mainland (cn) · Pro Trial" {
		t.Errorf("expected plan 'Qoder Mainland (cn) · Pro Trial', got %s", quotaResp.Subscription.Plan)
	}

	// 2. Live FetchUpstreamModels
	modelsBody, errModels := FetchUpstreamModels(ctx, client, auth)
	if errModels != nil {
		t.Fatalf("Live FetchUpstreamModels failed: %v", errModels)
	}
	t.Logf("Live FetchUpstreamModels SUCCESS! Received %d bytes", len(modelsBody))
}

func getString(m map[string]any, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}
