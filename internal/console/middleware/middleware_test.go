package middleware

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
	"github.com/tidwall/gjson"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestEnvironment(t *testing.T) (*db.DB, *engine.Engine, int64, string) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_mw.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rawKey := "sk-cpa-mw-test-token-abcdef123456"
	keyID, err := database.InsertKey(ctx, rawKey, "Middleware Test Key")
	if err != nil {
		t.Fatalf("insert key failed: %v", err)
	}

	dailyLimit := int64(500)
	rule := &db.QuotaRuleRow{
		KeyID:          keyID,
		Enabled:        1,
		DailyLimit:     &dailyLimit,
		HardRatio:      1.2, // Hard limit: 600
		RPMLimit:       2,   // Max 2 req/min
		ModelWhitelist: `["gpt-4o","claude-3-7-sonnet"]`,
	}
	if err := database.UpsertQuotaRule(ctx, rule); err != nil {
		t.Fatalf("upsert rule failed: %v", err)
	}

	eng, err := engine.NewEngine(database, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("new engine failed: %v", err)
	}

	return database, eng, keyID, rawKey
}

func TestKeyExtractionAndAuth(t *testing.T) {
	database, eng, _, rawKey := setupTestEnvironment(t)
	defer func() { _ = database.Close() }()

	r := gin.New()
	for _, mw := range Gateway(eng) {
		r.Use(mw)
	}
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 1. Missing key -> 401 Unauthorized
	reqNoKey := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o"}`))
	reqNoKey.Header.Set("Content-Type", "application/json")
	wNoKey := httptest.NewRecorder()
	r.ServeHTTP(wNoKey, reqNoKey)
	if wNoKey.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing key, got %d", wNoKey.Code)
	}

	// 2. Invalid key -> 401 Unauthorized
	reqBadKey := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o"}`))
	reqBadKey.Header.Set("Authorization", "Bearer sk-invalid-key")
	wBadKey := httptest.NewRecorder()
	r.ServeHTTP(wBadKey, reqBadKey)
	if wBadKey.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid key, got %d", wBadKey.Code)
	}

	// 3. Valid key across all 5 penetration headers
	testHeaders := []struct {
		header string
		val    string
		url    string
	}{
		{"Authorization", "Bearer " + rawKey, "/v1/chat/completions"},
		{"x-api-key", rawKey, "/v1/chat/completions"},
		{"api-key", rawKey, "/v1/chat/completions"},
		{"x-goog-api-key", rawKey, "/v1/chat/completions"},
		{"", "", "/v1/chat/completions?key=" + rawKey},
	}

	// We reset RPM limit for each iteration
	for i, th := range testHeaders {
		eng.Memory().ResetLiveCounters()
		req := httptest.NewRequest(http.MethodPost, th.url, bytes.NewBufferString(`{"model":"gpt-4o"}`))
		req.Header.Set("Content-Type", "application/json")
		if th.header != "" {
			req.Header.Set(th.header, th.val)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("penetration header test %d failed: expected 200, got %d (body: %s)", i+1, w.Code, w.Body.String())
		}
	}
}

func TestModelGuardAndFiltering(t *testing.T) {
	database, eng, _, rawKey := setupTestEnvironment(t)
	defer func() { _ = database.Close() }()

	r := gin.New()
	for _, mw := range Gateway(eng) {
		r.Use(mw)
	}
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/v1/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"object": "list",
			"data": []gin.H{
				{"id": "gpt-4o", "object": "model"},
				{"id": "claude-3-7-sonnet", "object": "model"},
				{"id": "deepseek-r1", "object": "model"},
				{"id": "gemini-2.5-flash", "object": "model"},
			},
		})
	})

	// 1. Allowed model -> 200 OK
	reqAllowed := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o"}`))
	reqAllowed.Header.Set("Authorization", "Bearer "+rawKey)
	reqAllowed.Header.Set("Content-Type", "application/json")
	wAllowed := httptest.NewRecorder()
	r.ServeHTTP(wAllowed, reqAllowed)
	if wAllowed.Code != http.StatusOK {
		t.Fatalf("expected 200 for allowed model, got %d", wAllowed.Code)
	}

	// 2. Disallowed model -> 403 Forbidden
	reqDisallowed := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"deepseek-r1"}`))
	reqDisallowed.Header.Set("Authorization", "Bearer "+rawKey)
	reqDisallowed.Header.Set("Content-Type", "application/json")
	wDisallowed := httptest.NewRecorder()
	r.ServeHTTP(wDisallowed, reqDisallowed)
	if wDisallowed.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disallowed model, got %d", wDisallowed.Code)
	}

	// 3. /v1/models response filtered down to whitelisted models only
	reqModels := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqModels.Header.Set("Authorization", "Bearer "+rawKey)
	wModels := httptest.NewRecorder()
	r.ServeHTTP(wModels, reqModels)
	if wModels.Code != http.StatusOK {
		t.Fatalf("expected 200 for /v1/models, got %d", wModels.Code)
	}

	dataArr := gjson.GetBytes(wModels.Body.Bytes(), "data").Array()
	if len(dataArr) != 2 {
		t.Fatalf("expected filtered /v1/models to have 2 items, got %d (body: %s)", len(dataArr), wModels.Body.String())
	}
	model1 := dataArr[0].Get("id").String()
	model2 := dataArr[1].Get("id").String()
	if model1 != "gpt-4o" || model2 != "claude-3-7-sonnet" {
		t.Fatalf("unexpected filtered models: %s, %s", model1, model2)
	}
}

func TestRPMAndQuotaEnforcement(t *testing.T) {
	database, eng, keyID, rawKey := setupTestEnvironment(t)
	defer func() { _ = database.Close() }()

	r := gin.New()
	for _, mw := range Gateway(eng) {
		r.Use(mw)
	}
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"id": "chatcmpl-test",
			"usage": gin.H{
				"prompt_tokens":     100,
				"completion_tokens": 100,
				"total_tokens":      200,
			},
		})
	})

	// Request 1: Allowed (RPM 1/2)
	req1 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o"}`))
	req1.Header.Set("Authorization", "Bearer "+rawKey)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("request 1 failed with %d", w1.Code)
	}

	// Request 2: Allowed (RPM 2/2)
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o"}`))
	req2.Header.Set("Authorization", "Bearer "+rawKey)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("request 2 failed with %d", w2.Code)
	}

	// Request 3: Blocked by RPM limit (Limit: 2) -> 429
	req3 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o"}`))
	req3.Header.Set("Authorization", "Bearer "+rawKey)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for RPM overflow, got %d", w3.Code)
	}
	if !bytes.Contains(w3.Body.Bytes(), []byte("rate_limit_exceeded")) {
		t.Fatalf("expected rate_limit_exceeded code, got %s", w3.Body.String())
	}

	// Advance time for RPM window or reset RPM window to test Quota
	eng.Memory().ResetLiveCounters()

	// Fill tokens up to hard limit (Limit: 500, Hard: 600)
	// Directly simulate existing usage
	ttft := int64(100)
	_ = eng.RecordRequest(context.Background(), &engine.RequestInfo{
		KeyID:       &keyID,
		Model:       "gpt-4o",
		Status:      200,
		TotalTokens: 650, // exceeds hard limit 600
		LatencyMs:   500,
		TtftMs:      &ttft,
	})

	// Next request should be blocked by Quota -> 429 insufficient_quota
	reqQuota := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o"}`))
	reqQuota.Header.Set("Authorization", "Bearer "+rawKey)
	wQuota := httptest.NewRecorder()
	r.ServeHTTP(wQuota, reqQuota)
	if wQuota.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for quota overflow, got %d", wQuota.Code)
	}
	if !bytes.Contains(wQuota.Body.Bytes(), []byte("quota_exceeded")) {
		t.Fatalf("expected quota_exceeded code, got %s", wQuota.Body.String())
	}
}

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

func TestModelGuard_ContentLengthHeaderDecimal(t *testing.T) {
	database, eng, _, rawKey := setupTestEnvironment(t)
	defer func() { _ = database.Close() }()

	r := gin.New()
	for _, mw := range Gateway(eng) {
		r.Use(mw)
	}
	r.GET("/v1/models", func(c *gin.Context) {
		// Return a response longer than 128 bytes to test multi-byte rune vs decimal string
		models := make([]gin.H, 20)
		for i := 0; i < 20; i++ {
			models[i] = gin.H{"id": fmt.Sprintf("model-test-%d", i), "object": "model"}
		}
		c.JSON(http.StatusOK, gin.H{
			"object": "list",
			"data":   models,
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /v1/models, got %d", w.Code)
	}

	clStr := w.Header().Get("Content-Length")
	if clStr == "" {
		t.Fatal("expected Content-Length header to be set")
	}
	cl, err := strconv.Atoi(clStr)
	if err != nil {
		t.Fatalf("Content-Length header is not a valid integer: %q (err: %v)", clStr, err)
	}
	if cl != w.Body.Len() {
		t.Fatalf("Content-Length mismatch: header=%d, bodyLen=%d", cl, w.Body.Len())
	}
}

func TestKeyAuth_BypassManagementAndOAuth(t *testing.T) {
	database, eng, _, _ := setupTestEnvironment(t)
	defer func() { _ = database.Close() }()

	r := gin.New()
	for _, mw := range Gateway(eng) {
		r.Use(mw)
	}

	bypassPaths := []string{
		"/management.html",
		"/v0/management/config",
		"/v8/management/config",
		"/anthropic/callback",
		"/codex/callback",
		"/antigravity/callback",
		"/oauth/token",
		"/healthz",
		"/readyz",
		"/metrics",
		"/console",
		"/admin",
		"/",
	}

	for _, p := range bypassPaths {
		r.GET(p, func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"path": c.Request.URL.Path})
		})
	}

	for _, p := range bypassPaths {
		// Request without any API key header or parameter
		req := httptest.NewRequest(http.MethodGet, p, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for bypass path %s without key, got %d (body: %s)", p, w.Code, w.Body.String())
		}
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
