package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

func setupTestRouter(t *testing.T) (*gin.Engine, *engine.Engine, *PricingEngine, *ProviderRegistryEngine, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir, err := os.MkdirTemp("", "console_api_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test_api.db")
	database, err := db.Open(dbPath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to open test db: %v", err)
	}

	eng, err := engine.NewEngine(database, 50*time.Millisecond)
	if err != nil {
		_ = database.Close()
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to init engine: %v", err)
	}

	pricing, err := NewPricingEngine(database)
	if err != nil {
		_ = database.Close()
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to init pricing engine: %v", err)
	}

	providers, err := NewProviderRegistryEngine(database)
	if err != nil {
		_ = database.Close()
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to init provider registry engine: %v", err)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	cfg := RouterConfig{
		AdminSecret: "test-secret-12345",
		CpaHost:     "127.0.0.1",
		CpaPort:     8000,
		CpaSecret:   "cpa-secret-67890",
	}

	RegisterRoutes(r, eng, cfg, pricing, providers)

	cleanup := func() {
		eng.Stop()
		_ = database.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return r, eng, pricing, providers, cleanup
}

func TestAdminAuth(t *testing.T) {
	r, _, _, _, cleanup := setupTestRouter(t)
	defer cleanup()

	// 1. Missing secret
	req := httptest.NewRequest(http.MethodGet, "/console/api/keys", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing auth, got %d", w.Code)
	}

	// 2. Wrong secret
	req = httptest.NewRequest(http.MethodGet, "/console/api/keys", nil)
	req.Header.Set("Authorization", "Bearer wrong-secret")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid auth, got %d", w.Code)
	}

	// 3. Valid Bearer
	req = httptest.NewRequest(http.MethodGet, "/console/api/keys", nil)
	req.Header.Set("Authorization", "Bearer test-secret-12345")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid Bearer, got %d", w.Code)
	}

	// 4. Valid x-admin-key
	req = httptest.NewRequest(http.MethodGet, "/console/api/keys", nil)
	req.Header.Set("x-admin-key", "test-secret-12345")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid x-admin-key, got %d", w.Code)
	}

	// 5. Compatibility route /admin/api/keys
	req = httptest.NewRequest(http.MethodGet, "/admin/api/keys", nil)
	req.Header.Set("Authorization", "Bearer test-secret-12345")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /admin/api/keys, got %d", w.Code)
	}
}

func TestKeysCRUDLifecycle(t *testing.T) {
	r, _, _, _, cleanup := setupTestRouter(t)
	defer cleanup()

	authHdr := "Bearer test-secret-12345"

	// 1. Create Key
	createPayload := map[string]interface{}{
		"alias":           "Developer Alpha",
		"enabled":         true,
		"daily_limit":     500000,
		"hard_ratio":      1.5,
		"rpm_limit":       60,
		"model_whitelist": []string{"claude-3-5-sonnet", "gemini-1.5-pro"},
	}
	body, _ := json.Marshal(createPayload)
	req := httptest.NewRequest(http.MethodPost, "/console/api/keys", bytes.NewReader(body))
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create key failed with %d: %s", w.Code, w.Body.String())
	}

	var createResp struct {
		Success bool   `json:"success"`
		KeyID   int64  `json:"key_id"`
		APIKey  string `json:"api_key"`
		Alias   string `json:"alias"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &createResp)
	if !createResp.Success || createResp.KeyID <= 0 || createResp.APIKey == "" {
		t.Fatalf("invalid create response: %+v", createResp)
	}
	keyID := createResp.KeyID
	rawKey := createResp.APIKey

	// 2. Reveal Key
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/console/api/keys/%d/reveal", keyID), nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reveal key failed with %d", w.Code)
	}
	var revealResp struct {
		KeyID  int64  `json:"key_id"`
		APIKey string `json:"api_key"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &revealResp)
	if revealResp.APIKey != rawKey {
		t.Fatalf("expected revealed key %s, got %s", rawKey, revealResp.APIKey)
	}

	// 3. Update Alias
	updateAliasBody := []byte(`{"alias":"Developer Beta"}`)
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/console/api/keys/%d/alias", keyID), bytes.NewReader(updateAliasBody))
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update alias failed with %d", w.Code)
	}

	// 4. Update Quota Rule
	updateQuotaBody := []byte(`{"enabled":true,"daily_limit":1000000,"hard_ratio":1.2,"rpm_limit":120,"model_whitelist":["*"]}`)
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/console/api/keys/%d/quota", keyID), bytes.NewReader(updateQuotaBody))
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update quota failed with %d", w.Code)
	}

	// 5. Rotate Key
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/console/api/keys/%d/rotate", keyID), nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("rotate key failed with %d", w.Code)
	}
	var rotateResp struct {
		Success   bool   `json:"success"`
		NewAPIKey string `json:"new_api_key"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &rotateResp)
	if !rotateResp.Success || rotateResp.NewAPIKey == "" || rotateResp.NewAPIKey == rawKey {
		t.Fatalf("invalid rotate response: %+v", rotateResp)
	}

	// 6. Delete Quota Rule
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/console/api/keys/%d/quota", keyID), nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete quota failed with %d", w.Code)
	}

	// 7. Delete Key
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/console/api/keys/%d", keyID), nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete key failed with %d", w.Code)
	}
}

func TestModelPricesAndProviderRegistry(t *testing.T) {
	r, _, _, _, cleanup := setupTestRouter(t)
	defer cleanup()

	authHdr := "Bearer test-secret-12345"

	// 1. Get initial model prices
	req := httptest.NewRequest(http.MethodGet, "/console/api/model-prices", nil)
	req.Header.Set("Authorization", authHdr)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get model prices failed: %d", w.Code)
	}

	// 2. Upsert model price
	priceBody := []byte(`{"pattern":"custom-model-v1","prompt_price":1.20,"completion_price":3.50,"cache_read_price":0.30,"multiplier":1.0}`)
	req = httptest.NewRequest(http.MethodPost, "/console/api/model-prices", bytes.NewReader(priceBody))
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("upsert model price failed: %d", w.Code)
	}

	// 3. Provider registry list
	req = httptest.NewRequest(http.MethodGet, "/console/api/provider-registry", nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get provider registry failed: %d", w.Code)
	}

	// 4. Upsert provider registry
	provBody := []byte(`{
		"provider_key": "stepfun",
		"label": "StepFun AI",
		"model_patterns": [{"pattern": "^step-", "is_regex": true}],
		"auth_patterns": [{"pattern": "^stepfun-", "is_regex": true}],
		"enabled": true,
		"sort_order": 50
	}`)
	req = httptest.NewRequest(http.MethodPost, "/console/api/provider-registry", bytes.NewReader(provBody))
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("upsert provider registry failed with %d: %s", w.Code, w.Body.String())
	}

	// 5. Delete provider registry
	req = httptest.NewRequest(http.MethodDelete, "/console/api/provider-registry/stepfun", nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete provider registry failed with %d", w.Code)
	}
}

func TestValidationsLifecycle(t *testing.T) {
	r, _, _, _, cleanup := setupTestRouter(t)
	defer cleanup()

	authHdr := "Bearer test-secret-12345"

	// 1. Manual validation creation
	manualBody := []byte(`{
		"auth_index": "test-auth-1234",
		"provider": "antigravity",
		"account_name": "user@gmail.com",
		"validation_url": "https://accounts.google.com/signin/continue?sarp=1&continue=https://aistudio.google.com",
		"error_message": "Action required to verify Google account"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/console/api/upstream-validations/manual", bytes.NewReader(manualBody))
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("manual validation creation failed with %d: %s", w.Code, w.Body.String())
	}

	var recResp struct {
		Success bool `json:"success"`
		Record  struct {
			ID int64 `json:"id"`
		} `json:"record"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &recResp)
	if !recResp.Success || recResp.Record.ID <= 0 {
		t.Fatalf("invalid record response: %+v", recResp)
	}
	valID := recResp.Record.ID

	// 2. Query validations
	req = httptest.NewRequest(http.MethodGet, "/console/api/upstream-validations", nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get validations failed with %d", w.Code)
	}

	// 3. Resolve validation
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/console/api/upstream-validations/%d/resolve", valID), nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("resolve validation failed with %d", w.Code)
	}

	// 4. Dismiss validation
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/console/api/upstream-validations/%d/dismiss", valID), nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("dismiss validation failed with %d", w.Code)
	}

	// 5. Delete validation
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/console/api/upstream-validations/%d", valID), nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete validation failed with %d", w.Code)
	}
}

func TestMemberPortalEndpoints(t *testing.T) {
	r, eng, _, _, cleanup := setupTestRouter(t)
	defer cleanup()

	// 1. Create a key in engine
	ctx := context.Background()
	rawKey := "sk-cpa-member-portal-test-key-12345678"
	keyID, err := eng.DB().InsertKey(ctx, rawKey, "Member Key")
	if err != nil {
		t.Fatalf("insert member key failed: %v", err)
	}
	rule := &db.QuotaRuleRow{
		KeyID:          keyID,
		Enabled:        1,
		DailyLimit:     int64Ptr(100000),
		HardRatio:      1.2,
		RPMLimit:       30,
		ModelWhitelist: `["claude-3-5-sonnet"]`,
	}
	_ = eng.DB().UpsertQuotaRule(ctx, rule)
	_ = eng.ReloadState(ctx)

	// 2. Unauthorized request to member info
	req := httptest.NewRequest(http.MethodGet, "/console/api/member/info", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for member without key, got %d", w.Code)
	}

	// 3. Authorized request to /console/api/member/info
	req = httptest.NewRequest(http.MethodGet, "/console/api/member/info", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("member info failed with %d: %s", w.Code, w.Body.String())
	}

	var memberResp struct {
		ID         int64  `json:"id"`
		KeyAlias   string `json:"key_alias"`
		DailyLimit int64  `json:"daily_limit"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &memberResp)
	if memberResp.ID != keyID || memberResp.DailyLimit != 100000 {
		t.Fatalf("unexpected member info payload: %+v", memberResp)
	}

	// 4. Authorized request to /api/member/info (compatibility alias)
	req = httptest.NewRequest(http.MethodGet, "/api/member/info", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("compat member info failed with %d", w.Code)
	}

	// 5. Member status summary
	req = httptest.NewRequest(http.MethodGet, "/console/api/member/status-summary", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("member status summary failed with %d", w.Code)
	}

	// 6. Member requests
	req = httptest.NewRequest(http.MethodGet, "/console/api/member/requests", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("member requests query failed with %d", w.Code)
	}
}

func TestSystemTelemetryEndpoints(t *testing.T) {
	r, _, _, _, cleanup := setupTestRouter(t)
	defer cleanup()

	paths := []string{
		"/healthz",
		"/readyz",
		"/metrics",
		"/api/status/quota",
		"/console/healthz",
		"/console/readyz",
		"/console/metrics",
	}

	for _, p := range paths {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for %s, got %d", p, w.Code)
		}
	}
}

func TestNewConsoleEndpoints(t *testing.T) {
	r, _, _, _, cleanup := setupTestRouter(t)
	defer cleanup()

	authHdr := "Bearer test-secret-12345"

	// 1. GET /console/api/upstream-quotas and /admin/api/upstream-quotas (fallback mode when CPA offline)
	for _, path := range []string{"/console/api/upstream-quotas", "/admin/api/upstream-quotas"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", authHdr)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d: %s", path, w.Code, w.Body.String())
		}
		var qResp struct {
			Accounts []interface{} `json:"accounts"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &qResp); err != nil {
			t.Fatalf("failed to parse upstream-quotas JSON from %s: %v", path, err)
		}
	}

	// 2. GET /console/api/runtime-facts (degraded mode when CPA offline)
	for _, window := range []string{"1h", "3h20m", "6h", "12h", "24h"} {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/console/api/runtime-facts?window=%s", window), nil)
		req.Header.Set("Authorization", authHdr)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for runtime-facts?window=%s, got %d: %s", window, w.Code, w.Body.String())
		}
		var fResp struct {
			Source   string                 `json:"source"`
			Degraded bool                   `json:"degraded"`
			Totals   map[string]interface{} `json:"totals"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &fResp); err != nil {
			t.Fatalf("failed to parse runtime-facts JSON: %v", err)
		}
		if fResp.Totals == nil {
			t.Fatalf("expected totals in runtime-facts response")
		}
	}

	// 3. GET /console/api/models and /console/api/models/catalog
	for _, path := range []string{"/console/api/models", "/console/api/models/catalog"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", authHdr)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d: %s", path, w.Code, w.Body.String())
		}
		var mResp struct {
			Models []interface{} `json:"models"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &mResp); err != nil {
			t.Fatalf("failed to parse models JSON from %s: %v", path, err)
		}
	}

	// 4. GET /console/api/upstream-auth-stats with range filters
	for _, rng := range []string{"", "1d", "today", "7d", "30d", "all"} {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/console/api/upstream-auth-stats?range=%s", rng), nil)
		req.Header.Set("Authorization", authHdr)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for upstream-auth-stats?range=%s, got %d: %s", rng, w.Code, w.Body.String())
		}
		var sResp struct {
			Totals map[string]interface{} `json:"totals"`
			Groups []interface{}          `json:"groups"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &sResp); err != nil {
			t.Fatalf("failed to parse upstream-auth-stats JSON: %v", err)
		}
	}

	// 5. OpenRouter Accounts CRUD
	// 5.1 GET list
	req := httptest.NewRequest(http.MethodGet, "/console/api/openrouter/accounts", nil)
	req.Header.Set("Authorization", authHdr)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET openrouter/accounts, got %d: %s", w.Code, w.Body.String())
	}

	// 5.2 POST create
	createAccPayload := map[string]interface{}{
		"name":     "Test OpenRouter Account",
		"note":     "Automated Unit Test",
		"apiKey":   "sk-or-v1-abcdef0123456789abcdef0123456789",
		"mgmtKey":  "sk-or-mgmt-9876543210",
		"priority": 1,
		"weight":   100,
		"enabled":  true,
	}
	body, _ := json.Marshal(createAccPayload)
	req = httptest.NewRequest(http.MethodPost, "/console/api/openrouter/accounts", bytes.NewReader(body))
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for POST openrouter/accounts, got %d: %s", w.Code, w.Body.String())
	}
	var accCreated struct {
		Success bool `json:"success"`
		Account struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"account"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &accCreated)
	if !accCreated.Success || accCreated.Account.ID == "" {
		t.Fatalf("invalid create account response: %s", w.Body.String())
	}
	accID := accCreated.Account.ID

	// 5.3 PATCH update
	updatedName := "Test OpenRouter Account Updated"
	updatePayload := map[string]interface{}{
		"name": &updatedName,
		"note": "Updated note",
	}
	body, _ = json.Marshal(updatePayload)
	req = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/console/api/openrouter/accounts/%s", accID), bytes.NewReader(body))
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for PATCH openrouter/accounts/%s, got %d: %s", accID, w.Code, w.Body.String())
	}

	// 5.4 DELETE account
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/console/api/openrouter/accounts/%s", accID), nil)
	req.Header.Set("Authorization", authHdr)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for DELETE openrouter/accounts/%s, got %d: %s", accID, w.Code, w.Body.String())
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}

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

func TestKeyTombstoneAndResurrectionPrevention(t *testing.T) {
	r, eng, _, _, cleanup := setupTestRouter(t)
	defer cleanup()

	authHdr := "Bearer test-secret-12345"
	ctx := context.Background()

	// 1. Create a key
	createPayload := map[string]interface{}{
		"alias": "Ephemeral Key",
	}
	body, _ := json.Marshal(createPayload)
	req := httptest.NewRequest(http.MethodPost, "/console/api/keys", bytes.NewReader(body))
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create key failed: %d: %s", w.Code, w.Body.String())
	}

	var createResp struct {
		Success bool   `json:"success"`
		KeyID   int64  `json:"key_id"`
		APIKey  string `json:"api_key"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &createResp)
	rawKey := createResp.APIKey
	keyID := createResp.KeyID

	// Verify not marked deleted initially
	isDel, err := eng.DB().IsKeyDeleted(ctx, rawKey)
	if err != nil || isDel {
		t.Fatalf("expected key not deleted initially, got isDel=%v, err=%v", isDel, err)
	}

	// 2. Delete the key via API
	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/console/api/keys/%d", keyID), nil)
	delReq.Header.Set("Authorization", authHdr)
	delW := httptest.NewRecorder()
	r.ServeHTTP(delW, delReq)
	if delW.Code != http.StatusOK {
		t.Fatalf("delete key failed: %d: %s", delW.Code, delW.Body.String())
	}

	// 3. Verify key is tombstoned
	isDel, err = eng.DB().IsKeyDeleted(ctx, rawKey)
	if err != nil || !isDel {
		t.Fatalf("expected key tombstoned after deletion, got isDel=%v, err=%v", isDel, err)
	}

	// 4. Simulate reconciliation: check that tombstone prevents re-insert
	if isDeleted, errDel := eng.DB().IsKeyDeleted(ctx, rawKey); errDel == nil && isDeleted {
		// As expected, reconciliation skips it
	} else {
		t.Fatalf("expected reconciliation logic to skip tombstoned key")
	}

	// 5. Explicitly re-create key with custom raw key or unmark
	err = eng.DB().UnmarkKeyDeleted(ctx, rawKey)
	if err != nil {
		t.Fatalf("unmark key deleted failed: %v", err)
	}
	isDel, err = eng.DB().IsKeyDeleted(ctx, rawKey)
	if err != nil || isDel {
		t.Fatalf("expected key no longer tombstoned after unmark, got %v", isDel)
	}
}

func TestHandleUpdateQuotaRule_PreservesExistingFields(t *testing.T) {
	r, eng, _, _, cleanup := setupTestRouter(t)
	defer cleanup()

	authHdr := "Bearer test-secret-12345"
	ctx := context.Background()

	// 1. Create Key with restricted model whitelist and specific quota settings
	createPayload := map[string]interface{}{
		"alias":           "Restricted Dev",
		"enabled":         true,
		"daily_limit":     750000,
		"hard_ratio":      1.8,
		"rpm_limit":       45,
		"model_whitelist": []string{"claude-3-5-sonnet", "gpt-4o"},
	}
	body, _ := json.Marshal(createPayload)
	req := httptest.NewRequest(http.MethodPost, "/console/api/keys", bytes.NewReader(body))
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create key failed: %d: %s", w.Code, w.Body.String())
	}

	var createResp struct {
		KeyID int64 `json:"key_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &createResp)
	keyID := createResp.KeyID

	// Verify initial quota rule in DB
	initialRule, err := eng.DB().SelectQuotaRuleByKeyID(ctx, keyID)
	if err != nil || initialRule == nil {
		t.Fatalf("failed to query initial quota rule: %v", err)
	}
	if initialRule.ModelWhitelist != `["claude-3-5-sonnet","gpt-4o"]` {
		t.Fatalf("expected initial whitelist ['claude-3-5-sonnet','gpt-4o'], got %s", initialRule.ModelWhitelist)
	}
	if initialRule.DailyLimit == nil || *initialRule.DailyLimit != 750000 {
		t.Fatalf("expected daily_limit 750000, got %v", initialRule.DailyLimit)
	}

	// 2. Partial update: only update rpm_limit, omitting model_whitelist and daily_limit
	partialBody := []byte(`{"rpm_limit":90}`)
	updateReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/console/api/keys/%d/quota", keyID), bytes.NewReader(partialBody))
	updateReq.Header.Set("Authorization", authHdr)
	updateReq.Header.Set("Content-Type", "application/json")
	updateW := httptest.NewRecorder()
	r.ServeHTTP(updateW, updateReq)
	if updateW.Code != http.StatusOK {
		t.Fatalf("partial update quota failed: %d: %s", updateW.Code, updateW.Body.String())
	}

	// 3. Verify that model_whitelist and daily_limit are preserved and NOT reset to "[]" or nil
	updatedRule, err := eng.DB().SelectQuotaRuleByKeyID(ctx, keyID)
	if err != nil || updatedRule == nil {
		t.Fatalf("failed to query updated quota rule: %v", err)
	}
	if updatedRule.RPMLimit != 90 {
		t.Fatalf("expected updated rpm_limit 90, got %d", updatedRule.RPMLimit)
	}
	if updatedRule.ModelWhitelist != `["claude-3-5-sonnet","gpt-4o"]` {
		t.Fatalf("regression: model_whitelist was wiped out! expected ['claude-3-5-sonnet','gpt-4o'], got %s", updatedRule.ModelWhitelist)
	}
	if updatedRule.DailyLimit == nil || *updatedRule.DailyLimit != 750000 {
		t.Fatalf("expected daily_limit 750000 to be preserved, got %v", updatedRule.DailyLimit)
	}
	if updatedRule.Enabled != 1 {
		t.Fatalf("expected enabled 1 to be preserved, got %d", updatedRule.Enabled)
	}
	if updatedRule.HardRatio != 1.8 {
		t.Fatalf("expected hard_ratio 1.8 to be preserved, got %f", updatedRule.HardRatio)
	}

	// 4. Explicit update: explicitly update model_whitelist to ["*"]
	explicitBody := []byte(`{"model_whitelist":["*"]}`)
	explicitReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/console/api/keys/%d/quota", keyID), bytes.NewReader(explicitBody))
	explicitReq.Header.Set("Authorization", authHdr)
	explicitReq.Header.Set("Content-Type", "application/json")
	explicitW := httptest.NewRecorder()
	r.ServeHTTP(explicitW, explicitReq)
	if explicitW.Code != http.StatusOK {
		t.Fatalf("explicit update quota failed: %d: %s", explicitW.Code, explicitW.Body.String())
	}

	finalRule, err := eng.DB().SelectQuotaRuleByKeyID(ctx, keyID)
	if err != nil || finalRule == nil {
		t.Fatalf("failed to query final quota rule: %v", err)
	}
	if finalRule.ModelWhitelist != `["*"]` {
		t.Fatalf("expected model_whitelist ['*'], got %s", finalRule.ModelWhitelist)
	}
	if finalRule.RPMLimit != 90 {
		t.Fatalf("expected rpm_limit 90 to be preserved when updating whitelist, got %d", finalRule.RPMLimit)
	}
}
