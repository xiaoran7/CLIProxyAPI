package console

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/api"
	serverconfig "github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestConsoleConfig(t *testing.T) {
	os.Setenv("CONSOLE_ENABLED", "true")
	os.Setenv("CONSOLE_ADMIN_SECRET", "test-secret")
	os.Setenv("CONSOLE_FLUSH_INTERVAL", "250ms")
	defer func() {
		os.Unsetenv("CONSOLE_ENABLED")
		os.Unsetenv("CONSOLE_ADMIN_SECRET")
		os.Unsetenv("CONSOLE_FLUSH_INTERVAL")
	}()

	cfg := LoadConfig()
	if !cfg.Enabled {
		t.Fatal("expected console to be enabled")
	}
	if cfg.AdminSecret != "test-secret" {
		t.Fatalf("expected admin secret 'test-secret', got '%s'", cfg.AdminSecret)
	}
	if cfg.FlushInterval != 250*time.Millisecond {
		t.Fatalf("expected flush interval 250ms, got %v", cfg.FlushInterval)
	}
}

func TestConsoleSubsystemLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmpDir, err := os.MkdirTemp("", "console_subsystem_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "lifecycle.db")
	cfg := Config{
		Enabled:       true,
		DBPath:        dbPath,
		AdminSecret:   "adm-sec-999",
		CpaHost:       "127.0.0.1",
		CpaPort:       8000,
		CpaSecret:     "cpa-sec-888",
		FlushInterval: 50 * time.Millisecond,
	}

	sub, err := InitSubsystem(cfg)
	if err != nil {
		t.Fatalf("InitSubsystem failed: %v", err)
	}
	if sub == nil {
		t.Fatal("subsystem should not be nil")
	}
	if GetSubsystem() != sub {
		t.Fatal("GetSubsystem should match initialized instance")
	}
	if sub.DB() == nil || sub.Engine() == nil || sub.Pricing() == nil || sub.Providers() == nil {
		t.Fatal("subsystem components should not be nil")
	}

	// Test ServerOptions production
	opts := ServerOptions(cfg)
	if len(opts) == 0 {
		t.Fatal("ServerOptions should return configured options")
	}

	// Verify that registering options and constructing a server applies them
	serverCfg := &serverconfig.Config{
		Host: "127.0.0.1",
		Port: 8000,
		RemoteManagement: serverconfig.RemoteManagement{
			SecretKey: "cpa-sec-888",
		},
	}
	authMgr := auth.NewManager(nil, nil, nil)

	// Register global option
	api.ClearGlobalServerOptions()
	api.RegisterGlobalServerOption(opts...)
	defer api.ClearGlobalServerOptions()

	srv := api.NewServer(serverCfg, authMgr, nil, "")
	if srv == nil {
		t.Fatal("NewServer failed to create server")
	}

	// Test reaching /console and /console/api/keys
	req := httptest.NewRequest(http.MethodGet, "/console", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /console, got %d", w.Code)
	}

	reqAuth := httptest.NewRequest(http.MethodGet, "/console/api/keys", nil)
	reqAuth.Header.Set("Authorization", "Bearer adm-sec-999")
	wAuth := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wAuth, reqAuth)
	if wAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 for /console/api/keys, got %d", wAuth.Code)
	}

	// Shutdown
	sub.Stop()
	if GetSubsystem() != nil {
		t.Fatal("GetSubsystem should be nil after Stop")
	}
}

func TestBootstrap(t *testing.T) {
	if sub := GetSubsystem(); sub != nil {
		sub.Stop()
	}
	defer func() {
		if sub := GetSubsystem(); sub != nil {
			sub.Stop()
		}
	}()
	// Call Bootstrap to ensure it executes without panics or errors
	Bootstrap()
	// Second call to test sync.Once idempotency
	Bootstrap()
}

func TestConsole_ConfigReloadKeySynchronization(t *testing.T) {
	if existing := GetSubsystem(); existing != nil {
		existing.Stop()
	}
	gin.SetMode(gin.TestMode)
	tmpDir, err := os.MkdirTemp("", "console_reload_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "reload.db")
	cfg := Config{
		Enabled:       true,
		DBPath:        dbPath,
		AdminSecret:   "adm-sec-reload",
		CpaHost:       "127.0.0.1",
		CpaPort:       8000,
		CpaSecret:     "cpa-sec-reload",
		FlushInterval: 50 * time.Millisecond,
	}

	sub, err := InitSubsystem(cfg)
	if err != nil {
		t.Fatalf("InitSubsystem failed: %v", err)
	}
	defer sub.Stop()

	opts := ServerOptions(cfg)
	serverCfg := &serverconfig.Config{
		Host: "127.0.0.1",
		Port: 8000,
		SDKConfig: serverconfig.SDKConfig{
			APIKeys: []string{"sk-cpa-initial-key-1"},
		},
		RemoteManagement: serverconfig.RemoteManagement{
			SecretKey: "cpa-sec-reload",
		},
	}
	authMgr := auth.NewManager(nil, nil, nil)

	api.ClearGlobalServerOptions()
	api.RegisterGlobalServerOption(opts...)
	defer api.ClearGlobalServerOptions()

	srv := api.NewServer(serverCfg, authMgr, nil, "")
	if srv == nil {
		t.Fatal("NewServer failed to create server")
	}

	// 1. Initial key is reconciled
	if _, ok := sub.Engine().ResolveKeyID("sk-cpa-initial-key-1"); !ok {
		t.Fatal("expected initial key to be reconciled in console engine")
	}

	// 2. Simulate new key added to config via Management API
	newKey := "sk-cpa-mgmt-created-key-2"
	if _, ok := sub.Engine().ResolveKeyID(newKey); ok {
		t.Fatal("expected newKey not in engine yet")
	}

	nextCfg := &serverconfig.Config{
		Host: "127.0.0.1",
		Port: 8000,
		SDKConfig: serverconfig.SDKConfig{
			APIKeys: []string{"sk-cpa-initial-key-1", newKey},
		},
		RemoteManagement: serverconfig.RemoteManagement{
			SecretKey: "cpa-sec-reload",
		},
	}

	// 3. Trigger server client/config update (which invokes configUpdateHook)
	srv.UpdateClientsContext(context.Background(), nextCfg)

	// 4. Verify new key is immediately reconciled in Console engine and DB
	keyID, ok := sub.Engine().ResolveKeyID(newKey)
	if !ok || keyID <= 0 {
		t.Fatalf("expected newKey to be reconciled after UpdateClientsContext, got ok=%v, id=%d", ok, keyID)
	}

	// 5. Verify /console/api/keys lists the newly reconciled key
	reqAuth := httptest.NewRequest(http.MethodGet, "/console/api/keys", nil)
	reqAuth.Header.Set("Authorization", "Bearer adm-sec-reload")
	wAuth := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wAuth, reqAuth)
	if wAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 for /console/api/keys, got %d", wAuth.Code)
	}

	bodyStr := wAuth.Body.String()
	if !strings.Contains(bodyStr, "sk-cpa...ey-2") || !strings.Contains(bodyStr, `"total":2`) {
		t.Fatalf("expected /console/api/keys response to contain masked key, got: %s", bodyStr)
	}

	// 6. Test JIT gateway request with a 3rd key present in config but before reload
	thirdKey := "sk-cpa-jit-direct-key-3"
	nextCfg.APIKeys = append(nextCfg.APIKeys, thirdKey)
	sub.cpaConfig.Store(nextCfg)

	reqProxy := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o"}`))
	reqProxy.Header.Set("Authorization", "Bearer "+thirdKey)
	reqProxy.Header.Set("Content-Type", "application/json")
	wProxy := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wProxy, reqProxy)
	// It should NOT be 401 Unauthorized
	if wProxy.Code == http.StatusUnauthorized {
		t.Fatalf("gateway intercepted newly created key with 401: %s", wProxy.Body.String())
	}
	if _, ok := sub.Engine().ResolveKeyID(thirdKey); !ok {
		t.Fatal("expected thirdKey to be resolved in engine after gateway request")
	}
}
