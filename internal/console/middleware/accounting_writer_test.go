// 记账中间件测试（流式 SSE 记账与 Outbox 队列、反重力安全校验拦截、配额持久化连续性、非推理探测与扫描器清洗、本地错误分类） —— 拆自 middleware_test.go

package middleware

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

func TestStreamingAccountingAndOutbox(t *testing.T) {
	database, eng, keyID, rawKey := setupTestEnvironment(t)
	defer func() { _ = database.Close() }()

	r := gin.New()
	for _, mw := range Gateway(eng) {
		r.Use(mw)
	}
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("X-Cpa-Trace-Id", "cpa-0123456789abcdef-trace")
		c.Writer.WriteHeader(http.StatusOK)

		chunks := []string{
			"data: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{\"content\":\"world!\"}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{}}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":6,\"total_tokens\":18}}\n\n",
			"data: [DONE]\n\n",
		}
		for _, ch := range chunks {
			_, _ = c.Writer.Write([]byte(ch))
			c.Writer.Flush()
		}
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o","stream":true}`))
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for streaming, got %d", w.Code)
	}

	// Verify outbox was populated synchronously
	ctx := context.Background()
	cnt, err := database.CountOutbox(ctx)
	if err != nil || cnt != 1 {
		t.Fatalf("expected 1 outbox record, got %d (err: %v)", cnt, err)
	}

	outboxList, err := database.SelectAllOutbox(ctx)
	if err != nil || len(outboxList) == 0 {
		t.Fatalf("failed to query outbox: %v", err)
	}
	item := outboxList[0]
	if item.TotalTokens != 18 || item.PromptTokens != 12 || item.CompletionTokens != 6 {
		t.Fatalf("unexpected token accounting: prompt=%d, comp=%d, total=%d", item.PromptTokens, item.CompletionTokens, item.TotalTokens)
	}
	if item.AuthIndex != "0123456789abcdef" {
		t.Fatalf("expected auth_index 0123456789abcdef from X-Cpa-Trace-Id, got %s", item.AuthIndex)
	}
	if item.Stream != 1 {
		t.Fatalf("expected stream=1, got %d", item.Stream)
	}

	// Verify live in-memory accumulation
	liveTokens := eng.Memory().GetTodayUsageForKey(keyID)
	if liveTokens != 18 {
		t.Fatalf("expected live today usage to be 18, got %d", liveTokens)
	}
}

func TestAntigravityValidationInterception(t *testing.T) {
	database, eng, _, rawKey := setupTestEnvironment(t)
	defer func() { _ = database.Close() }()

	r := gin.New()
	for _, mw := range Gateway(eng) {
		r.Use(mw)
	}
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Header("X-Cpa-Trace-Id", "cpa-fedcba9876543210-trace")
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"code":    403,
				"message": "VALIDATION_REQUIRED: https://accounts.google.com/signin/continue?sarp=1&continue=https%3A%2F%2Fdevelopers.google.com",
				"metadata": gin.H{
					"validation_error_message": "Action required: complete security check.",
				},
			},
		})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o"}`))
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}

	// Verify validation row inserted in database
	ctx := context.Background()
	validations, err := database.SelectValidations(ctx, "pending")
	if err != nil || len(validations) == 0 {
		t.Fatalf("expected validation row to be recorded, got %d (err: %v)", len(validations), err)
	}
	v := validations[0]
	if v.AuthIndex != "fedcba9876543210" {
		t.Fatalf("expected auth_index fedcba9876543210, got %s", v.AuthIndex)
	}
	if v.Provider != "antigravity" {
		t.Fatalf("expected provider antigravity, got %s", v.Provider)
	}
	if v.ErrorMessage == nil || *v.ErrorMessage != "Action required: complete security check." {
		t.Fatalf("unexpected validation message: %v", v.ErrorMessage)
	}
}

func TestOutboxFlusher_ContinuousQuotaPreservation(t *testing.T) {
	database, eng, keyID, _ := setupTestEnvironment(t)
	defer func() { _ = database.Close() }()

	ctx := context.Background()

	// 1. Record 2 requests through outbox flusher
	err := eng.Flusher().RecordRequest(ctx, &engine.RequestInfo{
		KeyID:        &keyID,
		Model:        "gpt-4o",
		Status:       200,
		TotalTokens:  1500,
		PromptTokens: 1000,
	})
	if err != nil {
		t.Fatalf("record request 1 failed: %v", err)
	}

	err = eng.Flusher().RecordRequest(ctx, &engine.RequestInfo{
		KeyID:        &keyID,
		Model:        "gpt-4o",
		Status:       200,
		TotalTokens:  2500,
		PromptTokens: 2000,
	})
	if err != nil {
		t.Fatalf("record request 2 failed: %v", err)
	}

	// Verify before flush: total usage is 4000
	usageBefore := eng.Memory().GetTodayUsageForKey(keyID)
	if usageBefore != 4000 {
		t.Fatalf("expected 4000 tokens before flush, got %d", usageBefore)
	}

	// 2. Perform flush
	if err := eng.Flusher().FlushUsage(ctx); err != nil {
		t.Fatalf("flush usage failed: %v", err)
	}

	// Verify after flush: total usage must REMAIN 4000 (preloaded=4000, live=0)
	usageAfter := eng.Memory().GetTodayUsageForKey(keyID)
	if usageAfter != 4000 {
		t.Fatalf("expected 4000 tokens preserved after flush, got %d", usageAfter)
	}

	// 3. Add concurrent request
	err = eng.Flusher().RecordRequest(ctx, &engine.RequestInfo{
		KeyID:        &keyID,
		Model:        "gpt-4o",
		Status:       200,
		TotalTokens:  800,
		PromptTokens: 500,
	})
	if err != nil {
		t.Fatalf("record concurrent request failed: %v", err)
	}

	usageConcurrent := eng.Memory().GetTodayUsageForKey(keyID)
	if usageConcurrent != 4800 {
		t.Fatalf("expected 4800 tokens after concurrent request, got %d", usageConcurrent)
	}

	// 4. Second flush
	if err := eng.Flusher().FlushUsage(ctx); err != nil {
		t.Fatalf("second flush failed: %v", err)
	}

	usageAfterSecond := eng.Memory().GetTodayUsageForKey(keyID)
	if usageAfterSecond != 4800 {
		t.Fatalf("expected 4800 tokens preserved after second flush, got %d", usageAfterSecond)
	}
}

func TestNonInferenceDiscoveryAndUtilityNotRecorded(t *testing.T) {
	database, eng, _, rawKey := setupTestEnvironment(t)
	defer func() { _ = database.Close() }()

	r := gin.New()
	for _, mw := range Gateway(eng) {
		r.Use(mw)
	}
	r.GET("/v1/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"object": "list",
			"data": []gin.H{
				{"id": "gpt-4o", "object": "model"},
			},
		})
	})
	r.OPTIONS("/v1/chat/completions", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	r.POST("/v1/messages/count_tokens", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"input_tokens": 128,
		})
	})
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"model":   "gpt-4o",
			"choices": []gin.H{{"message": gin.H{"role": "assistant", "content": "hello"}}},
			"usage":   gin.H{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	})

	ctx := context.Background()

	// 1. GET /v1/models with valid API Key
	reqModels := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqModels.Header.Set("Authorization", "Bearer "+rawKey)
	wModels := httptest.NewRecorder()
	r.ServeHTTP(wModels, reqModels)
	if wModels.Code != http.StatusOK {
		t.Fatalf("expected 200 for /v1/models, got %d", wModels.Code)
	}

	// 2. OPTIONS /v1/chat/completions (CORS Preflight)
	reqOptions := httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)
	wOptions := httptest.NewRecorder()
	r.ServeHTTP(wOptions, reqOptions)
	if wOptions.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS, got %d", wOptions.Code)
	}

	// 2b. POST /v1/messages/count_tokens (Token counting utility)
	reqCount := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewBufferString(`{"model":"gpt-4o","messages":[]}`))
	reqCount.Header.Set("Authorization", "Bearer "+rawKey)
	reqCount.Header.Set("Content-Type", "application/json")
	wCount := httptest.NewRecorder()
	r.ServeHTTP(wCount, reqCount)
	if wCount.Code != http.StatusOK {
		t.Fatalf("expected 200 for count_tokens, got %d", wCount.Code)
	}

	// Flush and verify neither GET /v1/models nor OPTIONS nor count_tokens was recorded in outbox or events
	if err := eng.Flusher().FlushUsage(ctx); err != nil {
		t.Fatalf("flush usage failed: %v", err)
	}

	events, err := database.SelectRecentEvents(ctx, 100)
	if err != nil {
		t.Fatalf("select recent events failed: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events recorded for discovery/options/count_tokens, got %d (first model: %s)", len(events), events[0].Model)
	}

	// 3. Real inference request POST /v1/chat/completions
	reqInference := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o"}`))
	reqInference.Header.Set("Authorization", "Bearer "+rawKey)
	reqInference.Header.Set("Content-Type", "application/json")
	wInference := httptest.NewRecorder()
	r.ServeHTTP(wInference, reqInference)
	if wInference.Code != http.StatusOK {
		t.Fatalf("expected 200 for inference, got %d", wInference.Code)
	}

	if err := eng.Flusher().FlushUsage(ctx); err != nil {
		t.Fatalf("flush usage failed: %v", err)
	}

	eventsAfter, err := database.SelectRecentEvents(ctx, 100)
	if err != nil {
		t.Fatalf("select recent events failed: %v", err)
	}
	if len(eventsAfter) != 1 {
		t.Fatalf("expected exactly 1 event recorded for real inference, got %d", len(eventsAfter))
	}
	if eventsAfter[0].Model != "gpt-4o" || eventsAfter[0].TotalTokens != 15 {
		t.Fatalf("unexpected recorded event: model=%s, tokens=%d", eventsAfter[0].Model, eventsAfter[0].TotalTokens)
	}

	// 4. Test CleanPhantomEvents on legacy dirty records
	_, _ = database.InsertRequestEvent(ctx, &db.RequestEventRow{
		TS:          time.Now().Unix(),
		Model:       "unknown",
		Status:      200,
		TotalTokens: 0,
	})
	cleaned, err := database.CleanPhantomEvents(ctx)
	if err != nil {
		t.Fatalf("clean phantom events failed: %v", err)
	}
	if cleaned != 1 {
		t.Fatalf("expected 1 cleaned phantom event, got %d", cleaned)
	}
}

func TestAccounting_DiscardUnauthenticatedProbeAndScanner(t *testing.T) {
	database, eng, _, _ := setupTestEnvironment(t)
	defer func() { _ = database.Close() }()

	ctx := context.Background()

	r := gin.New()
	for _, mw := range Gateway(eng) {
		r.Use(mw)
	}
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": "ok"})
	})

	// 1. Send simulated scanner requests without any API Key to non-inference endpoints
	scannerPaths := []string{
		"/.git/config",
		"/wp-json/gravitysmtp/v1/tests/mock-data",
		"/cdn-cgi/rum",
		"/mcp",
		"/theme/default/assets/umi.js",
	}

	for _, p := range scannerPaths {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for scanner path %s, got %d", p, w.Code)
		}
	}

	// Flush usage to ensure any outbox items are processed
	if err := eng.Flusher().FlushUsage(ctx); err != nil {
		t.Fatalf("flush usage failed: %v", err)
	}

	// Verify error_events has ZERO records inserted
	errorEvents, err := database.SelectRecentErrorEvents(ctx, 100)
	if err != nil {
		t.Fatalf("select recent error events failed: %v", err)
	}
	if len(errorEvents) != 0 {
		t.Fatalf("expected 0 error events from unauthenticated scanner probes, got %d (first: %+v)", len(errorEvents), errorEvents[0])
	}

	// Verify request_events has ZERO records inserted
	reqEvents, err := database.SelectRecentEvents(ctx, 100)
	if err != nil {
		t.Fatalf("select recent request events failed: %v", err)
	}
	if len(reqEvents) != 0 {
		t.Fatalf("expected 0 request events from unauthenticated scanner probes, got %d", len(reqEvents))
	}

	// 2. Test CleanPhantomEvents cleans legacy unauthenticated error_events
	errType := "upstream_status_401"
	errBody := `{"error":{"code":"invalid_api_key"}}`
	_, _ = database.InsertErrorEvent(ctx, &db.ErrorEventRow{
		TS:        time.Now().Unix(),
		Status:    http.StatusUnauthorized,
		ErrorType: &errType,
		ErrorBody: &errBody,
	})
	cleaned, err := database.CleanPhantomEvents(ctx)
	if err != nil {
		t.Fatalf("clean phantom events failed: %v", err)
	}
	_ = cleaned

	afterCleanErrors, err := database.SelectRecentErrorEvents(ctx, 100)
	if err != nil {
		t.Fatalf("select error events after clean failed: %v", err)
	}
	if len(afterCleanErrors) != 0 {
		t.Fatalf("expected 0 error events after CleanPhantomEvents, got %d", len(afterCleanErrors))
	}
}

func TestAccounting_GatewayLocalErrorTypeClassification(t *testing.T) {
	database, eng, keyID, rawKey := setupTestEnvironment(t)
	defer func() { _ = database.Close() }()

	ctx := context.Background()

	// Set model whitelist allowing only gpt-4o
	rule := &db.QuotaRuleRow{
		KeyID:          keyID,
		Enabled:        1,
		HardRatio:      1.0,
		RPMLimit:       100,
		ModelWhitelist: `["gpt-4o"]`,
	}
	if err := database.UpsertQuotaRule(ctx, rule); err != nil {
		t.Fatalf("upsert quota rule failed: %v", err)
	}
	if err := eng.ReloadQuotaRules(ctx); err != nil {
		t.Fatalf("reload quota rules failed: %v", err)
	}

	r := gin.New()
	for _, mw := range Gateway(eng) {
		r.Use(mw)
	}
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": "ok"})
	})

	// 1. Request forbidden model claude-3-opus -> must trigger 403 model_not_allowed
	reqForbidden := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"claude-3-opus"}`))
	reqForbidden.Header.Set("Authorization", "Bearer "+rawKey)
	reqForbidden.Header.Set("Content-Type", "application/json")
	wForbidden := httptest.NewRecorder()
	r.ServeHTTP(wForbidden, reqForbidden)

	if wForbidden.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for forbidden model, got %d: %s", wForbidden.Code, wForbidden.Body.String())
	}

	// Verify error event classified as model_forbidden (not upstream_status_403)
	errors, err := database.SelectRecentErrorEvents(ctx, 10)
	if err != nil {
		t.Fatalf("select error events failed: %v", err)
	}
	if len(errors) != 1 {
		t.Fatalf("expected 1 error event for model_not_allowed, got %d", len(errors))
	}
	if errors[0].ErrorType == nil || *errors[0].ErrorType != "model_forbidden" {
		t.Fatalf("expected error_type 'model_forbidden', got %v", errors[0].ErrorType)
	}
}
