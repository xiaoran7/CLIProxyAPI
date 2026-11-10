// 控制台请求日志与统计聚合管道集成测试 —— 拆自 api_test.go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

func TestConsoleRequestLogsAndStatsPipeline(t *testing.T) {
	r, eng, _, _, cleanup := setupTestRouter(t)
	defer cleanup()

	ctx := context.Background()
	authHdr := "Bearer test-secret-12345"

	// 1. Create Key and Baseline
	rawKey := "sk-cpa-test-pipeline-key-8888"
	keyID, err := eng.DB().InsertKey(ctx, rawKey, "Pipeline Key")
	if err != nil {
		t.Fatalf("insert key failed: %v", err)
	}
	_ = eng.ReloadState(ctx)
	_, err = eng.DB().ExecContext(ctx, "INSERT INTO upstream_auth_baselines (auth_index, name, provider) VALUES (?, ?, ?)", "auth-antigravity-1234", "gemini-pro.json", "antigravity")
	if err != nil {
		t.Fatalf("insert baseline failed: %v", err)
	}

	now := time.Now().Unix()
	nowMs := time.Now().UnixMilli()

	// 2. Record simulated requests
	ttft := int64(150)
	err = eng.RecordRequest(ctx, &engine.RequestInfo{
		KeyID:            &keyID,
		Model:            "gemini-2.5-flash",
		Status:           200,
		PromptTokens:     1000,
		CompletionTokens: 500,
		CachedTokens:     200,
		TotalTokens:      1500,
		LatencyMs:        400,
		TtftMs:           &ttft,
		Stream:           true,
		ReasoningEffort:  "high",
		AuthIndex:        "auth-antigravity-1234",
	})
	if err != nil {
		t.Fatalf("record request 1 failed: %v", err)
	}

	err = eng.RecordRequest(ctx, &engine.RequestInfo{
		KeyID:            &keyID,
		Model:            "claude-3-7-sonnet",
		Status:           429,
		PromptTokens:     100,
		CompletionTokens: 0,
		CachedTokens:     0,
		TotalTokens:      100,
		LatencyMs:        120,
		Stream:           false,
		AuthIndex:        "auth-claude-5678",
		ErrorType:        "rate_limited",
	})
	if err != nil {
		t.Fatalf("record request 2 failed: %v", err)
	}

	err = eng.RecordRequest(ctx, &engine.RequestInfo{
		KeyID:            &keyID,
		Model:            "gpt-4o",
		Status:           500,
		PromptTokens:     50,
		CompletionTokens: 0,
		CachedTokens:     0,
		TotalTokens:      50,
		LatencyMs:        200,
		Stream:           false,
		AuthIndex:        "auth-codex-9999",
		ErrorType:        "upstream_error",
	})
	if err != nil {
		t.Fatalf("record request 3 failed: %v", err)
	}

	// Flush usage to SQLite
	_ = eng.Flusher().FlushUsage(ctx)

	// Insert an error event
	recoverAt := nowMs + 60000
	modelGpt := "gpt-4o"
	authGpt := "auth-codex-9999"
	errTypeUpstream := "upstream_error"
	errBody := "Simulated internal server error"
	_, _ = eng.DB().InsertErrorEvent(ctx, &db.ErrorEventRow{
		TS:           now,
		KeyID:        &keyID,
		AuthIndex:    &authGpt,
		Model:        &modelGpt,
		Status:       500,
		ErrorType:    &errTypeUpstream,
		ErrorBody:    &errBody,
		RecoverAt:    recoverAt,
		RetryAfterMs: 60000,
	})

	// 3. Test GET /console/api/requests
	req := httptest.NewRequest(http.MethodGet, "/console/api/requests", nil)
	req.Header.Set("Authorization", authHdr)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /console/api/requests, got %d: %s", w.Code, w.Body.String())
	}

	var reqsResp struct {
		Events     []map[string]interface{} `json:"events"`
		Requests   []map[string]interface{} `json:"requests"`
		Total      int64                    `json:"total"`
		TodayTotal int64                    `json:"today_total"`
		Page       int                      `json:"page"`
		PageSize   int                      `json:"page_size"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reqsResp); err != nil {
		t.Fatalf("failed to parse requests JSON: %v", err)
	}

	if len(reqsResp.Events) != 3 || len(reqsResp.Requests) != 3 {
		t.Fatalf("expected 3 events and requests, got %d events and %d requests", len(reqsResp.Events), len(reqsResp.Requests))
	}
	if reqsResp.Total != 3 || reqsResp.TodayTotal != 3 {
		t.Fatalf("expected total 3 and today_total 3, got total %d, today_total %d", reqsResp.Total, reqsResp.TodayTotal)
	}

	// Verify enriched item metadata
	foundGemini := false
	for _, ev := range reqsResp.Events {
		if ev["model"] == "gemini-2.5-flash" {
			foundGemini = true
			if ev["reasoning_effort"] != "high" {
				t.Errorf("expected reasoning_effort 'high', got %v", ev["reasoning_effort"])
			}
			if ev["auth_name"] != "gemini-pro" {
				t.Errorf("expected auth_name 'gemini-pro', got %v", ev["auth_name"])
			}
			if ev["key_alias"] != "Pipeline Key" {
				t.Errorf("expected key_alias 'Pipeline Key', got %v", ev["key_alias"])
			}
			tsVal, _ := ev["ts"].(float64)
			if tsVal < 1e11 {
				t.Errorf("expected timestamp in milliseconds, got %v", tsVal)
			}
			hitRate, _ := ev["cache_hit_rate"].(float64)
			if hitRate <= 0 {
				t.Errorf("expected positive cache hit rate, got %v", hitRate)
			}
		}
	}
	if !foundGemini {
		t.Fatalf("gemini request not found in events")
	}

	// Test filters: filter=cached
	req = httptest.NewRequest(http.MethodGet, "/console/api/requests?filter=cached", nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &reqsResp)
	if len(reqsResp.Events) != 1 || reqsResp.Events[0]["model"] != "gemini-2.5-flash" {
		t.Fatalf("expected 1 cached event, got %d", len(reqsResp.Events))
	}

	// Test filters: filter=errors
	req = httptest.NewRequest(http.MethodGet, "/console/api/requests?filter=errors", nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &reqsResp)
	if len(reqsResp.Events) != 2 {
		t.Fatalf("expected 2 error events, got %d", len(reqsResp.Events))
	}

	// 4. Test GET /console/api/errors
	req = httptest.NewRequest(http.MethodGet, "/console/api/errors", nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /console/api/errors, got %d: %s", w.Code, w.Body.String())
	}
	var errResp struct {
		Errors     []map[string]interface{} `json:"errors"`
		Total      int64                    `json:"total"`
		TodayTotal int64                    `json:"today_total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse errors JSON: %v", err)
	}
	if len(errResp.Errors) != 1 {
		t.Fatalf("expected 1 error event, got %d", len(errResp.Errors))
	}
	if errResp.Total != 1 || errResp.TodayTotal != 1 {
		t.Fatalf("expected total 1 and today_total 1, got %d, %d", errResp.Total, errResp.TodayTotal)
	}
	errItem := errResp.Errors[0]
	if isActive, _ := errItem["is_active"].(bool); !isActive {
		t.Fatalf("expected is_active true for circuit breaker error")
	}
	if remSec, _ := errItem["remaining_recover_sec"].(float64); remSec <= 0 {
		t.Fatalf("expected positive remaining_recover_sec, got %v", remSec)
	}

	// 5. Test GET /console/api/stats (days=1 and days=14)
	req = httptest.NewRequest(http.MethodGet, "/console/api/stats?days=1", nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /console/api/stats?days=1, got %d: %s", w.Code, w.Body.String())
	}
	var statsResp struct {
		Hourly    []map[string]interface{} `json:"hourly"`
		Daily     []map[string]interface{} `json:"daily"`
		Models    []map[string]interface{} `json:"models"`
		Providers []map[string]interface{} `json:"providers"`
		Summary   map[string]interface{}   `json:"summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &statsResp); err != nil {
		t.Fatalf("failed to parse stats JSON: %v", err)
	}
	if len(statsResp.Hourly) != 24 {
		t.Fatalf("expected 24 hourly buckets for days=1, got %d", len(statsResp.Hourly))
	}
	if len(statsResp.Providers) == 0 {
		t.Fatalf("expected non-empty providers aggregation")
	}
	if statsResp.Summary == nil || statsResp.Summary["period_tokens"] == nil {
		t.Fatalf("expected period_tokens in summary")
	}

	// 6. Test GET /console/api/status-summary
	req = httptest.NewRequest(http.MethodGet, "/console/api/status-summary", nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /console/api/status-summary, got %d: %s", w.Code, w.Body.String())
	}
	var statusResp struct {
		Overall        map[string]interface{}   `json:"overall"`
		ProviderGroups []map[string]interface{} `json:"provider_groups"`
		TodayRequests  int64                    `json:"today_requests"`
		TodayErrors    int64                    `json:"today_errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("failed to parse status summary JSON: %v", err)
	}
	bars, ok := statusResp.Overall["bars"].([]interface{})
	if !ok || len(bars) != 60 {
		t.Fatalf("expected 60 heartbeat bars in overall scope, got %d", len(bars))
	}
	if statusResp.TodayRequests != 3 {
		t.Fatalf("expected today_requests 3, got %d", statusResp.TodayRequests)
	}
	if len(statusResp.ProviderGroups) == 0 {
		t.Fatalf("expected non-empty provider_groups")
	}

	// 7. Test Member Endpoints with key
	memberAuth := "Bearer " + rawKey
	req = httptest.NewRequest(http.MethodGet, "/api/member/requests", nil)
	req.Header.Set("Authorization", memberAuth)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/member/requests, got %d: %s", w.Code, w.Body.String())
	}
	var memberReqsResp struct {
		Events     []map[string]interface{} `json:"events"`
		Requests   []map[string]interface{} `json:"requests"`
		Total      int64                    `json:"total"`
		TodayTotal int64                    `json:"today_total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &memberReqsResp); err != nil {
		t.Fatalf("failed to parse member requests JSON: %v", err)
	}
	if len(memberReqsResp.Events) != 3 || len(memberReqsResp.Requests) != 3 {
		t.Fatalf("expected 3 member events, got %d", len(memberReqsResp.Events))
	}
	if memberReqsResp.TodayTotal != 3 {
		t.Fatalf("expected member today_total 3, got %d", memberReqsResp.TodayTotal)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/member/status-summary", nil)
	req.Header.Set("Authorization", memberAuth)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/member/status-summary, got %d: %s", w.Code, w.Body.String())
	}
	var memberStatusResp struct {
		Overall       map[string]interface{} `json:"overall"`
		TodayRequests int64                  `json:"today_requests"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &memberStatusResp); err != nil {
		t.Fatalf("failed to parse member status summary: %v", err)
	}
	if memberStatusResp.Overall == nil || memberStatusResp.TodayRequests != 3 {
		t.Fatalf("invalid member status summary response: %+v", memberStatusResp)
	}
}
