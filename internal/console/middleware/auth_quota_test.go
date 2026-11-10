// 鉴权与配额中间件测试（密钥提取、白名单路径豁免、JIT 补救对齐、RPM 与每日配额拦截） —— 拆自 middleware_test.go

package middleware

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
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

func TestKeyAuthMiddleware_JITFallbackReconciliation(t *testing.T) {
	database, eng, _, _ := setupTestEnvironment(t)
	defer func() { _ = database.Close() }()

	ctx := context.Background()
	newCpaKey := "sk-cpa-jit-mgmt-new-token-123456"

	// Configure fallback resolver that checks simulated CPA config keys
	eng.SetFallbackKeyResolver(func(ctx context.Context, rawKey string) bool {
		if rawKey != newCpaKey {
			return false
		}
		if isDel, _ := database.IsKeyDeleted(ctx, rawKey); isDel {
			return false
		}
		keyID, err := database.InsertKey(ctx, rawKey, "CPA Config Key")
		if err != nil || keyID <= 0 {
			return false
		}
		rule := &db.QuotaRuleRow{
			KeyID:          keyID,
			Enabled:        0,
			HardRatio:      1.2,
			RPMLimit:       0,
			ModelWhitelist: "[]",
		}
		_ = database.UpsertQuotaRule(ctx, rule)
		_ = eng.ReloadState(ctx)
		return true
	})

	r := gin.New()
	for _, mw := range Gateway(eng) {
		r.Use(mw)
	}
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 1. Initial state: newCpaKey is NOT in memory
	if _, ok := eng.ResolveKeyID(newCpaKey); ok {
		t.Fatal("expected newCpaKey not in engine before request")
	}

	// 2. Request with newly provisioned CPA key should trigger JIT reconciliation and succeed (200)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o"}`))
	req.Header.Set("Authorization", "Bearer "+newCpaKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after JIT fallback reconciliation, got %d: %s", w.Code, w.Body.String())
	}

	// 3. Verify key is now recognized in memory
	if _, ok := eng.ResolveKeyID(newCpaKey); !ok {
		t.Fatal("expected newCpaKey to be cached in engine memory after JIT reconciliation")
	}

	// 4. Request with completely unknown key still returns 401
	badReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o"}`))
	badReq.Header.Set("Authorization", "Bearer sk-random-bad-key")
	badReq.Header.Set("Content-Type", "application/json")
	badW := httptest.NewRecorder()
	r.ServeHTTP(badW, badReq)
	if badW.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unknown key, got %d", badW.Code)
	}

	// 5. Tombstoned key should NOT be lazily reconciled
	tombstoneKey := "sk-cpa-tombstoned-key"
	_ = database.MarkKeyDeleted(ctx, tombstoneKey)
	tombReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o"}`))
	tombReq.Header.Set("Authorization", "Bearer "+tombstoneKey)
	tombReq.Header.Set("Content-Type", "application/json")
	tombW := httptest.NewRecorder()
	r.ServeHTTP(tombW, tombReq)
	if tombW.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for tombstoned key, got %d", tombW.Code)
	}
}
