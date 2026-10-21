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

func int64Ptr(v int64) *int64 {
	return &v
}
