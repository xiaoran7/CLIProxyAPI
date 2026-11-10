// 模型价格、提供商、规则校验与系统端点测试 —— 拆自 api_test.go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
		if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Fatalf("expected Cache-Control to contain no-store for %s, got %q", path, cc)
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
