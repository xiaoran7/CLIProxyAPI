// 模型守卫中间件测试（模型白名单校验、Content-Length 十进制计算、WorkBuddy 命名空间重写与模型列表过滤） —— 拆自 middleware_test.go

package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

func TestModelGuard_WorkBuddyNamespaceRewriteAndListing(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_wb_rewrite.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rawKey := "sk-cpa-workbuddy-rewrite-test"
	keyID, err := database.InsertKey(ctx, rawKey, "WorkBuddy Whitelist Key")
	if err != nil {
		t.Fatalf("insert key failed: %v", err)
	}

	dailyLimit := int64(1000)
	rule := &db.QuotaRuleRow{
		KeyID:          keyID,
		Enabled:        1,
		DailyLimit:     &dailyLimit,
		HardRatio:      1.5,
		RPMLimit:       100,
		ModelWhitelist: `["workbuddy/claude-opus-5-5", "workbuddy-cn/deepseek-v4.1-flash"]`,
	}
	if err := database.UpsertQuotaRule(ctx, rule); err != nil {
		t.Fatalf("insert quota rule failed: %v", err)
	}

	eng, err := engine.NewEngine(database, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("new engine failed: %v", err)
	}
	defer eng.Stop()

	r := gin.New()
	for _, mw := range Gateway(eng) {
		r.Use(mw)
	}

	var receivedBodyModel string
	var receivedCtxModel string
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		receivedBodyModel = gjson.GetBytes(body, "model").String()
		receivedCtxModel = c.GetString(ContextModel)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/v1/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"object": "list",
			"data": []gin.H{
				{"id": "claude-opus-5-5", "object": "model", "owned_by": "antigravity"},
				{"id": "workbuddy/claude-opus-5-5", "object": "model", "owned_by": "workbuddy"},
				{"id": "deepseek-v4.1-flash", "object": "model", "owned_by": "deepseek"},
				{"id": "workbuddy-cn/deepseek-v4.1-flash", "object": "model", "owned_by": "workbuddy-cn"},
				{"id": "gpt-4o", "object": "model", "owned_by": "openai"},
			},
		})
	})

	// 1. Send bare claude-opus-5-5 -> must be rewritten to workbuddy/claude-opus-5-5
	req1 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"claude-opus-5-5"}`))
	req1.Header.Set("Authorization", "Bearer "+rawKey)
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 for bare claude-opus-5-5, got %d", w1.Code)
	}
	if receivedBodyModel != "workbuddy/claude-opus-5-5" {
		t.Fatalf("body model = %q, want rewritten 'workbuddy/claude-opus-5-5'", receivedBodyModel)
	}
	if receivedCtxModel != "workbuddy/claude-opus-5-5" {
		t.Fatalf("context model = %q, want 'workbuddy/claude-opus-5-5'", receivedCtxModel)
	}

	// 2. Send bare deepseek-v4.1-flash -> must be rewritten to workbuddy-cn/deepseek-v4.1-flash
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"deepseek-v4.1-flash"}`))
	req2.Header.Set("Authorization", "Bearer "+rawKey)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 for bare deepseek-v4.1-flash, got %d", w2.Code)
	}
	if receivedBodyModel != "workbuddy-cn/deepseek-v4.1-flash" {
		t.Fatalf("body model = %q, want rewritten 'workbuddy-cn/deepseek-v4.1-flash'", receivedBodyModel)
	}

	// 3. GET /v1/models -> must only list the two namespaced models, suppressing bare unrouted models
	reqModels := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqModels.Header.Set("Authorization", "Bearer "+rawKey)
	wModels := httptest.NewRecorder()
	r.ServeHTTP(wModels, reqModels)
	if wModels.Code != http.StatusOK {
		t.Fatalf("expected 200 for /v1/models, got %d", wModels.Code)
	}
	dataArr := gjson.GetBytes(wModels.Body.Bytes(), "data").Array()
	if len(dataArr) != 2 {
		t.Fatalf("expected exactly 2 models in /v1/models, got %d: %s", len(dataArr), wModels.Body.String())
	}
	m1 := dataArr[0].Get("id").String()
	m2 := dataArr[1].Get("id").String()
	if m1 != "workbuddy/claude-opus-5-5" || m2 != "workbuddy-cn/deepseek-v4.1-flash" {
		t.Fatalf("unexpected filtered models: %s, %s", m1, m2)
	}
}
