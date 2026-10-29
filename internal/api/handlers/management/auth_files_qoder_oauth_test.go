package management

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestFindExistingQoderCredential_MatchInMemory(t *testing.T) {
	existingFile := "qoder-cn-custom-name.json"
	targetUID := "01a15e15-7421-7847-99f8-eaf5298e1804"

	manager := coreauth.NewManager(nil, nil, nil)
	_, errReg := manager.Register(context.Background(), &coreauth.Auth{
		ID:       existingFile,
		Provider: "qoder-cn",
		FileName: existingFile,
		Attributes: map[string]string{
			"user_id": targetUID,
			"email":   "user@example.com",
		},
	})
	if errReg != nil {
		t.Fatalf("manager.Register failed: %v", errReg)
	}

	h := &Handler{
		authManager: manager,
	}

	found := h.findExistingQoderCredential("qoder-cn", targetUID, "", "")
	if found == nil {
		t.Fatalf("expected to find existing credential by user_id")
	}
	if found.FileName != existingFile {
		t.Fatalf("expected FileName = %s, got %s", existingFile, found.FileName)
	}

	foundByEmail := h.findExistingQoderCredential("qoder-cn", "", "user@example.com", "")
	if foundByEmail == nil {
		t.Fatalf("expected to find existing credential by email")
	}
	if foundByEmail.FileName != existingFile {
		t.Fatalf("expected FileName = %s, got %s", existingFile, foundByEmail.FileName)
	}
}

func TestFindExistingQoderCredential_MatchOnDisk(t *testing.T) {
	authDir := t.TempDir()
	existingFile := "qoder-cn-1792579829631.json"
	targetUID := "01a15e15-7421-7847-99f8-eaf5298e1804"

	fileContent := map[string]any{
		"type":         "qoder-cn",
		"user_id":      targetUID,
		"email":        "disk-user@example.com",
		"access_token": "dt-sample-token",
	}
	raw, err := json.Marshal(fileContent)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(authDir, existingFile), raw, 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	h := &Handler{
		cfg: &config.Config{AuthDir: authDir},
	}

	found := h.findExistingQoderCredential("qoder-cn", targetUID, "", "")
	if found == nil {
		t.Fatalf("expected to find existing credential on disk by user_id")
	}
	if found.FileName != existingFile {
		t.Fatalf("expected FileName = %s, got %s", existingFile, found.FileName)
	}
}

func TestSanitizeCredentialSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"01a15e15-7421-7847-99f8-eaf5298e1804", "01a15e15-7421-7847-99f8-eaf5298e1804"},
		{"user@example.com", "user@example.com"},
		{"user/with\\unsafe*chars!", "user_with_unsafe_chars"},
		{"", "account"},
		{"   ", "account"},
	}

	for _, tt := range tests {
		got := sanitizeCredentialSlug(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeCredentialSlug(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
