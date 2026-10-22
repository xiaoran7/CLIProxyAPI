package api

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
)

func TestProviderRegistryEngine_AccountHexHashPriority(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_provider_reg.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer func() { _ = database.Close() }()

	pe, err := NewProviderRegistryEngine(database)
	if err != nil {
		t.Fatalf("failed to create provider registry engine: %v", err)
	}

	antigravityHash := "4a5f6e8d12345678"
	accountName := "AntiGravity-Pro-Account"
	pe.RegisterAuthAccount(antigravityHash, "antigravity", accountName)

	// Verify account lookup
	acc, found := pe.GetAuthAccount(antigravityHash)
	if !found {
		t.Fatalf("expected account %s to be found", antigravityHash)
	}
	if acc.ProviderKey != "antigravity" {
		t.Errorf("expected provider 'antigravity', got %q", acc.ProviderKey)
	}
	if acc.Name != accountName {
		t.Errorf("expected name %q, got %q", accountName, acc.Name)
	}

	// 1. Calling Gemini model with Antigravity auth_index must infer Antigravity, NOT Gemini
	resGemini := pe.InferProvider("gemini-2.5-flash", antigravityHash)
	if resGemini.Key != "antigravity" {
		t.Errorf("expected key 'antigravity', got %q", resGemini.Key)
	}
	if resGemini.MatchedBy != "auth" {
		t.Errorf("expected matched_by 'auth', got %q", resGemini.MatchedBy)
	}
	if resGemini.Source != "account" {
		t.Errorf("expected source 'account', got %q", resGemini.Source)
	}

	// 2. Calling Claude model with Antigravity auth_index must infer Antigravity, NOT Claude
	resClaude := pe.InferProvider("claude-3-7-sonnet", antigravityHash)
	if resClaude.Key != "antigravity" {
		t.Errorf("expected key 'antigravity', got %q", resClaude.Key)
	}
	if resClaude.MatchedBy != "auth" {
		t.Errorf("expected matched_by 'auth', got %q", resClaude.MatchedBy)
	}

	// 3. Calling Gemini model with empty auth_index falls back to Gemini via model pattern
	resFallback := pe.InferProvider("gemini-2.5-flash", "")
	if resFallback.Key != "gemini" {
		t.Errorf("expected key 'gemini', got %q", resFallback.Key)
	}
	if resFallback.MatchedBy != "model" {
		t.Errorf("expected matched_by 'model', got %q", resFallback.MatchedBy)
	}

	// 4. Verify GetProviderAuthIndexes and GetExcludeAuthIndexes
	agAuths := pe.GetProviderAuthIndexes("antigravity")
	if len(agAuths) != 1 || agAuths[0] != antigravityHash {
		t.Errorf("expected antigravity auths [%s], got %v", antigravityHash, agAuths)
	}

	geminiExcludes := pe.GetExcludeAuthIndexes("gemini")
	foundExclude := false
	for _, ea := range geminiExcludes {
		if ea == antigravityHash {
			foundExclude = true
			break
		}
	}
	if !foundExclude {
		t.Errorf("expected %s in gemini ExcludeAuths, got %v", antigravityHash, geminiExcludes)
	}

	agExcludes := pe.GetExcludeAuthIndexes("antigravity")
	for _, ea := range agExcludes {
		if ea == antigravityHash {
			t.Errorf("antigravity auths should not exclude its own index %s", antigravityHash)
		}
	}
}

func TestSyncAuthFilesToBaselines(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_sync_baselines.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer func() { _ = database.Close() }()

	pe, err := NewProviderRegistryEngine(database)
	if err != nil {
		t.Fatalf("failed to create provider registry engine: %v", err)
	}

	authIndex := "9f8e7d6c5b4a3210"
	authFiles := []CpaAuthFile{
		{
			AuthIndex: authIndex,
			Name:      "antigravity_work.json",
			Provider:  "antigravity",
			Nickname:  "Production Antigravity",
		},
	}

	compat := []CpaOpenAICompatProvider{
		{
			Name: "OpenRouter",
			APIKeyEntries: []CpaOpenAICompatKeyEntry{
				{
					AuthIndex: "openrouter_01",
					Note:      "Primary OpenRouter",
				},
			},
		},
	}

	ctx := context.Background()
	SyncAuthFilesToBaselines(ctx, nil, pe, authFiles, compat)

	// Verify in-memory account registration
	acc, found := pe.GetAuthAccount(authIndex)
	if !found {
		t.Fatalf("expected auth %s to be registered in pe", authIndex)
	}
	if acc.ProviderKey != "antigravity" {
		t.Errorf("expected provider 'antigravity', got %q", acc.ProviderKey)
	}
	if acc.Name != "Production Antigravity" {
		t.Errorf("expected name 'Production Antigravity', got %q", acc.Name)
	}

	// Verify OpenRouter account registration
	orAcc, orFound := pe.GetAuthAccount("openrouter_01")
	if !orFound {
		t.Fatalf("expected auth 'openrouter_01' to be registered in pe")
	}
	if orAcc.ProviderKey != "openrouter" {
		t.Errorf("expected provider 'openrouter', got %q", orAcc.ProviderKey)
	}
	if orAcc.Name != "Primary OpenRouter" {
		t.Errorf("expected name 'Primary OpenRouter', got %q", orAcc.Name)
	}
}

func TestInitialProviderRegistry_ExpandedSeeds(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_expanded_seeds.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer func() { _ = database.Close() }()

	pe, err := NewProviderRegistryEngine(database)
	if err != nil {
		t.Fatalf("failed to create provider registry engine: %v", err)
	}

	testCases := []struct {
		model       string
		expectedKey string
	}{
		{"llama-3.3-70b-instruct", "meta"},
		{"meta/llama-3.1-405b", "meta"},
		{"grok-2-vision-1212", "xai"},
		{"grok-beta", "xai"},
		{"mistral-large-2411", "mistral"},
		{"codestral-2501", "mistral"},
		{"pixtral-large-2411", "mistral"},
		{"minimax-text-01", "minimax"},
		{"abab6.5s-chat", "minimax"},
	}

	for _, tc := range testCases {
		res := pe.InferProvider(tc.model, "")
		if res.Key != tc.expectedKey {
			t.Errorf("model %q: expected provider %q, got %q", tc.model, tc.expectedKey, res.Key)
		}
		if res.MatchedBy != "model" {
			t.Errorf("model %q: expected matched_by 'model', got %q", tc.model, res.MatchedBy)
		}
	}
}
