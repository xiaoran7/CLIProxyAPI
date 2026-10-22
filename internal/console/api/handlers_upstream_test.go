package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

func setupUpstreamMockServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		cleanPath := strings.TrimPrefix(r.URL.Path, "/v0/management")
		switch cleanPath {
		case "/config":
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"debug":                    false,
					"logging-to-file":          true,
					"request-log":              true,
					"ws-auth":                  false,
					"force-model-prefix":       false,
					"usage-statistics-enabled": true,
					"request-retry":            3,
					"routing": map[string]interface{}{
						"strategy":             "smart",
						"session-affinity":     true,
						"session-affinity-ttl": "15m",
					},
				})
			} else if r.Method == http.MethodPatch || r.Method == http.MethodPut {
				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				_ = json.NewEncoder(w).Encode(body)
			}
		case "/latest-version":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"latest-version": "v8.12.0",
			})
		case "/usage-statistics-enabled":
			_ = json.NewEncoder(w).Encode(true)
		case "/auth-files":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"observed_at": time.Now().Format(time.RFC3339),
				"files": []map[string]interface{}{
					{
						"name":           "antigravity-1.json",
						"auth_index":     "ag-1",
						"provider":       "antigravity",
						"disabled":       false,
						"unavailable":    false,
						"status":         "active",
						"status_message": "",
						"email":          "user1@gmail.com",
						"priority":       100,
						"success":        45,
						"failed":         2,
						"cooldowns": []map[string]interface{}{
							{
								"scope":             "model",
								"model_key":         "claude-3-5-sonnet",
								"reason":            "quota",
								"remaining_seconds": 3600,
								"retry_at":          time.Now().Add(1 * time.Hour).Format(time.RFC3339),
							},
						},
						"recent_requests": []map[string]interface{}{
							{"time": "12:00-12:10", "success": 10, "failed": 0},
							{"time": "12:10-12:20", "success": 20, "failed": 1},
							{"time": "12:20-12:30", "success": 15, "failed": 1},
						},
					},
					{
						"name":           "codex-disabled.json",
						"auth_index":     "cx-2",
						"provider":       "codex",
						"disabled":       true,
						"unavailable":    false,
						"status":         "disabled",
						"status_message": "",
						"email":          "user2@openai.com",
						"priority":       50,
						"success":        10,
						"failed":         0,
					},
				},
			})
		case "/api-key-usage":
			_ = json.NewEncoder(w).Encode(map[string]map[string]interface{}{
				"claude": {
					"key-1": map[string]interface{}{
						"success": 30,
						"failed":  0,
						"recent_requests": []map[string]interface{}{
							{"time": "12:00-12:10", "success": 10, "failed": 0},
							{"time": "12:10-12:20", "success": 20, "failed": 0},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func parseHostPort(serverURL string) (string, int) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "127.0.0.1", 80
	}
	host := u.Hostname()
	portStr := u.Port()
	port, _ := strconv.Atoi(portStr)
	return host, port
}

func setupMockRouter(t *testing.T, cpaServerURL string) (*gin.Engine, *engine.Engine, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir, err := os.MkdirTemp("", "console_upstream_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test_upstream.db")
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
		t.Fatalf("failed to init pricing: %v", err)
	}

	providers, err := NewProviderRegistryEngine(database)
	if err != nil {
		_ = database.Close()
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to init providers: %v", err)
	}

	r := gin.New()
	host, port := parseHostPort(cpaServerURL)
	cfg := RouterConfig{
		AdminSecret: "test-admin-secret",
		CpaHost:     host,
		CpaPort:     port,
		CpaSecret:   "test-cpa-secret",
	}

	RegisterRoutes(r, eng, cfg, pricing, providers)

	cleanup := func() {
		eng.Stop()
		_ = database.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return r, eng, cleanup
}

func TestHandleGetRuntimeConfig(t *testing.T) {
	t.Parallel()

	srv := setupUpstreamMockServer()
	defer srv.Close()

	r, _, cleanup := setupMockRouter(t, srv.URL)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/console/api/runtime-config", nil)
	req.Header.Set("Authorization", "Bearer test-admin-secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Config struct {
			Ok    bool                   `json:"ok"`
			Value map[string]interface{} `json:"value"`
			Error string                 `json:"error"`
		} `json:"config"`
		Routing struct {
			Ok    bool                   `json:"ok"`
			Value map[string]interface{} `json:"value"`
			Error string                 `json:"error"`
		} `json:"routing"`
		Version struct {
			Ok    bool                   `json:"ok"`
			Value map[string]interface{} `json:"value"`
			Error string                 `json:"error"`
		} `json:"version"`
		UsageStats struct {
			Ok    bool        `json:"ok"`
			Value interface{} `json:"value"`
			Error string      `json:"error"`
		} `json:"usage_statistics"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	if !resp.Config.Ok || resp.Config.Value == nil {
		t.Errorf("expected config.ok = true, got %+v", resp.Config)
	}
	if !resp.Routing.Ok || resp.Routing.Value == nil {
		t.Errorf("expected routing.ok = true, got %+v", resp.Routing)
	} else {
		if resp.Routing.Value["strategy"] != "smart" {
			t.Errorf("expected routing strategy 'smart', got %v", resp.Routing.Value["strategy"])
		}
	}
	if !resp.Version.Ok || resp.Version.Value == nil {
		t.Errorf("expected version.ok = true, got %+v", resp.Version)
	}
	if !resp.UsageStats.Ok {
		t.Errorf("expected usage_statistics.ok = true, got %+v", resp.UsageStats)
	}
}

func TestHandleGetRuntimeFacts_Normal(t *testing.T) {
	t.Parallel()

	srv := setupUpstreamMockServer()
	defer srv.Close()

	r, _, cleanup := setupMockRouter(t, srv.URL)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/console/api/runtime-facts?window=3h20m", nil)
	req.Header.Set("Authorization", "Bearer test-admin-secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Source   string `json:"source"`
		Degraded bool   `json:"degraded"`
		Window   string `json:"window"`
		Totals   struct {
			CredentialsTotal    int            `json:"credentials_total"`
			CredentialsActive   int            `json:"credentials_active"`
			CredentialsDisabled int            `json:"credentials_disabled"`
			CredentialsCooldown int            `json:"credentials_cooldown"`
			CredentialsBlocked  int            `json:"credentials_blocked"`
			Buckets             []RecentBucket `json:"buckets"`
			TotalRequests       int64          `json:"total_requests"`
			Scope               string         `json:"scope"`
		} `json:"totals"`
		Providers []struct {
			Provider string         `json:"provider"`
			KeyCount int            `json:"key_count"`
			Buckets  []RecentBucket `json:"buckets"`
		} `json:"providers"`
		Credentials []struct {
			AuthIndex string         `json:"auth_index"`
			Provider  string         `json:"provider"`
			Buckets   []RecentBucket `json:"buckets"`
			Health    AuthHealthInfo `json:"health"`
		} `json:"credentials"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	if resp.Degraded {
		t.Errorf("expected degraded = false, got true")
	}
	if resp.Totals.CredentialsTotal != 2 {
		t.Errorf("expected 2 credentials, got %d", resp.Totals.CredentialsTotal)
	}
	if resp.Totals.CredentialsActive != 1 {
		t.Errorf("expected 1 active credential, got %d", resp.Totals.CredentialsActive)
	}
	if resp.Totals.CredentialsDisabled != 1 {
		t.Errorf("expected 1 disabled credential, got %d", resp.Totals.CredentialsDisabled)
	}
	if resp.Totals.CredentialsCooldown != 1 {
		t.Errorf("expected 1 cooldown credential, got %d", resp.Totals.CredentialsCooldown)
	}
	if resp.Totals.CredentialsBlocked != 2 {
		t.Errorf("expected 2 blocked credentials (1 cooldown + 1 disabled), got %d", resp.Totals.CredentialsBlocked)
	}
	if len(resp.Totals.Buckets) == 0 {
		t.Errorf("expected non-empty totals.buckets")
	}
	if len(resp.Providers) == 0 {
		t.Errorf("expected non-empty providers")
	}
}

func TestHandleGetRuntimeFacts_DegradedMode(t *testing.T) {
	t.Parallel()

	// Points to an unreachable host:port to verify graceful degradation
	r, _, cleanup := setupMockRouter(t, "http://127.0.0.1:54321")
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/console/api/runtime-facts?window=5h", nil)
	req.Header.Set("Authorization", "Bearer test-admin-secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (graceful degraded mode), got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Degraded bool   `json:"degraded"`
		Error    string `json:"error"`
		Window   string `json:"window"`
		Totals   struct {
			CredentialsTotal int `json:"credentials_total"`
		} `json:"totals"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	if !resp.Degraded {
		t.Errorf("expected degraded = true")
	}
	if resp.Error == "" {
		t.Errorf("expected non-empty error message in degraded mode")
	}
	if resp.Window != "5h" {
		t.Errorf("expected window '5h', got %s", resp.Window)
	}
}

func TestHandleUpstreamRouting(t *testing.T) {
	t.Parallel()

	srv := setupUpstreamMockServer()
	defer srv.Close()

	r, _, cleanup := setupMockRouter(t, srv.URL)
	defer cleanup()

	// 1. GET routing
	req := httptest.NewRequest(http.MethodGet, "/console/api/upstream-routing", nil)
	req.Header.Set("Authorization", "Bearer test-admin-secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET routing, got %d: %s", w.Code, w.Body.String())
	}

	var getResp struct {
		Routing struct {
			Strategy        string `json:"strategy"`
			SessionAffinity bool   `json:"session_affinity"`
		} `json:"routing"`
		Strategy        string `json:"strategy"`
		SessionAffinity bool   `json:"session_affinity"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &getResp)
	strategy := getResp.Strategy
	if strategy == "" {
		strategy = getResp.Routing.Strategy
	}
	affinity := getResp.SessionAffinity || getResp.Routing.SessionAffinity
	if strategy != "smart" || !affinity {
		t.Errorf("unexpected routing: %+v", getResp)
	}

	// 2. PUT routing
	putBody := `{"strategy":"round-robin","session_affinity":false}`
	putReq := httptest.NewRequest(http.MethodPut, "/console/api/upstream-routing", strings.NewReader(putBody))
	putReq.Header.Set("Authorization", "Bearer test-admin-secret")
	putReq.Header.Set("Content-Type", "application/json")
	putW := httptest.NewRecorder()
	r.ServeHTTP(putW, putReq)

	if putW.Code != http.StatusOK {
		t.Fatalf("expected 200 for PUT routing, got %d: %s", putW.Code, putW.Body.String())
	}
}

func TestResolveWindowPreset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		param          string
		expectedID     string
		expectedCount  int
		expectedBucket int
	}{
		{"1h", "1h", 12, 5},
		{"6h", "6h", 24, 15},
		{"1d", "1d", 24, 60},
		{"12h", "1d", 24, 60}, // alias
		{"24h", "1d", 24, 60}, // alias
		{"3d", "3d", 24, 180},
		{"7d", "7d", 28, 360},
		{"30d", "30d", 30, 1440},
		{"3h20m", "3h20m", 20, 10}, // legacy preset
		{"5h", "5h", 30, 10},       // legacy preset
		{"unknown", "1h", 12, 5},   // fallback
		{"", "1h", 12, 5},          // default
	}

	for _, tt := range tests {
		preset := resolveWindowPreset(tt.param)
		if preset.ID != tt.expectedID {
			t.Errorf("resolveWindowPreset(%q).ID = %s, want %s", tt.param, preset.ID, tt.expectedID)
		}
		if preset.BucketCount != tt.expectedCount {
			t.Errorf("resolveWindowPreset(%q).BucketCount = %d, want %d", tt.param, preset.BucketCount, tt.expectedCount)
		}
		if preset.BucketMinutes != tt.expectedBucket {
			t.Errorf("resolveWindowPreset(%q).BucketMinutes = %d, want %d", tt.param, preset.BucketMinutes, tt.expectedBucket)
		}
	}
}

func TestHandleGetUpstreamAuthStats(t *testing.T) {
	t.Parallel()

	srv := setupUpstreamMockServer()
	defer srv.Close()

	r, _, cleanup := setupMockRouter(t, srv.URL)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/console/api/upstream-auth-stats?range=all", nil)
	req.Header.Set("Authorization", "Bearer test-admin-secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		TimeRange string                 `json:"time_range"`
		Items     []AuthStatResponseItem `json:"items"`
		AuthStats []AuthStatResponseItem `json:"auth_stats"`
		Total     int                    `json:"total"`
		Summary   map[string]interface{} `json:"summary"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if resp.Total == 0 || len(resp.Items) == 0 {
		t.Fatalf("expected non-empty items/auth_stats, got %d", resp.Total)
	}

	if len(resp.Items) != len(resp.AuthStats) {
		t.Errorf("expected items and auth_stats to have same length, got %d vs %d", len(resp.Items), len(resp.AuthStats))
	}

	if resp.Summary == nil {
		t.Errorf("expected summary to be non-nil")
	}
}
