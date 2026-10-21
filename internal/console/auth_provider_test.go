package console

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v8/sdk/access"
)

func TestConsoleAccessProvider_Authenticate(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_provider.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rawKey := "sk-cpa-access-test-token-0011223344"
	keyID, err := database.InsertKey(ctx, rawKey, "Access Provider Test Key")
	if err != nil {
		t.Fatalf("insert key failed: %v", err)
	}
	if keyID <= 0 {
		t.Fatalf("invalid key id: %d", keyID)
	}

	eng, err := engine.NewEngine(database, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("new engine failed: %v", err)
	}
	defer eng.Stop()

	provider := NewConsoleAccessProvider(eng)
	if provider.Identifier() != AccessProviderTypeConsole {
		t.Fatalf("expected provider identifier %q, got %q", AccessProviderTypeConsole, provider.Identifier())
	}

	// 1. Missing credentials -> NoCredentials error
	reqEmpty := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	res, authErr := provider.Authenticate(ctx, reqEmpty)
	if res != nil || authErr == nil {
		t.Fatalf("expected error for empty credentials, got res: %v, err: %v", res, authErr)
	}
	if !sdkaccess.IsAuthErrorCode(authErr, sdkaccess.AuthErrorCodeNoCredentials) {
		t.Fatalf("expected AuthErrorCodeNoCredentials, got %v", authErr.Code)
	}

	// 2. Invalid credentials -> InvalidCredential error
	reqInvalid := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	reqInvalid.Header.Set("Authorization", "Bearer sk-invalid-key-999")
	res, authErr = provider.Authenticate(ctx, reqInvalid)
	if res != nil || authErr == nil {
		t.Fatalf("expected error for invalid key, got res: %v, err: %v", res, authErr)
	}
	if !sdkaccess.IsAuthErrorCode(authErr, sdkaccess.AuthErrorCodeInvalidCredential) {
		t.Fatalf("expected AuthErrorCodeInvalidCredential, got %v", authErr.Code)
	}

	// 3. Test all 5 penetration credentials
	testCases := []struct {
		name   string
		header string
		val    string
		url    string
	}{
		{"Bearer", "Authorization", "Bearer " + rawKey, "/v1/chat/completions"},
		{"RawAuth", "Authorization", rawKey, "/v1/chat/completions"},
		{"Anthropic", "x-api-key", rawKey, "/v1/chat/completions"},
		{"Azure", "api-key", rawKey, "/v1/chat/completions"},
		{"Google", "x-goog-api-key", rawKey, "/v1/chat/completions"},
		{"QueryKey", "", "", "/v1/chat/completions?key=" + rawKey},
		{"QueryAuthToken", "", "", "/v1/chat/completions?auth_token=" + rawKey},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.url, nil)
			if tc.header != "" {
				req.Header.Set(tc.header, tc.val)
			}
			result, errAuth := provider.Authenticate(ctx, req)
			if errAuth != nil {
				t.Fatalf("unexpected error for %s: %v", tc.name, errAuth)
			}
			if result == nil {
				t.Fatalf("expected non-nil result for %s", tc.name)
			}
			if result.Principal != rawKey {
				t.Fatalf("expected principal %s, got %s", rawKey, result.Principal)
			}
			if result.Provider != AccessProviderTypeConsole {
				t.Fatalf("expected provider %s, got %s", AccessProviderTypeConsole, result.Provider)
			}
		})
	}

	// 4. Test integration with sdkaccess.Manager
	mgr := sdkaccess.NewManager()
	mgr.SetProviders([]sdkaccess.Provider{provider})

	reqMgr := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	reqMgr.Header.Set("Authorization", "Bearer "+rawKey)
	resMgr, errMgr := mgr.Authenticate(ctx, reqMgr)
	if errMgr != nil || resMgr == nil {
		t.Fatalf("manager authentication failed: %v", errMgr)
	}
	if resMgr.Principal != rawKey {
		t.Fatalf("manager returned wrong principal: %s", resMgr.Principal)
	}
}
