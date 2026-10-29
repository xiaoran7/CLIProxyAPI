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

// An international-realm login must never adopt a mainland-realm credential file
// belonging to the same account, and vice versa.
func TestFindExistingQoderCredential_RealmIsolationOnDisk(t *testing.T) {
	const uid = "01a15e15-7421-7847-99f8-eaf5298e1804"
	const email = "shared@example.com"

	tests := []struct {
		name       string
		targetProv string
		fileName   string
		fileType   string
		fileRealm  string
		wantFile   string
	}{
		{
			name:       "intl login ignores cn credential",
			targetProv: "qoder",
			fileName:   "qoder-cn-" + uid + ".json",
			fileType:   "qoder-cn",
			fileRealm:  "cn",
			wantFile:   "",
		},
		{
			name:       "cn login ignores intl credential",
			targetProv: "qoder-cn",
			fileName:   "qoder-" + uid + ".json",
			fileType:   "qoder",
			fileRealm:  "intl",
			wantFile:   "",
		},
		{
			name:       "intl login adopts intl credential",
			targetProv: "qoder",
			fileName:   "qoder-legacy-name.json",
			fileType:   "qoder",
			fileRealm:  "intl",
			wantFile:   "qoder-legacy-name.json",
		},
		{
			name:       "cn login adopts cn credential",
			targetProv: "qoder-cn",
			fileName:   "qoder-cn-legacy-name.json",
			fileType:   "qoder-cn",
			fileRealm:  "cn",
			wantFile:   "qoder-cn-legacy-name.json",
		},
		{
			name:       "realm falls back to filename prefix when body omits it",
			targetProv: "qoder",
			fileName:   "qoder-norealm.json",
			fileType:   "",
			fileRealm:  "",
			wantFile:   "qoder-norealm.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authDir := t.TempDir()
			content := map[string]any{
				"user_id":      uid,
				"email":        email,
				"access_token": "dt-sample-token",
			}
			if tt.fileType != "" {
				content["type"] = tt.fileType
			}
			if tt.fileRealm != "" {
				content["realm"] = tt.fileRealm
			}
			raw, errMarshal := json.Marshal(content)
			if errMarshal != nil {
				t.Fatalf("json.Marshal failed: %v", errMarshal)
			}
			if errWrite := os.WriteFile(filepath.Join(authDir, tt.fileName), raw, 0o600); errWrite != nil {
				t.Fatalf("WriteFile failed: %v", errWrite)
			}

			h := &Handler{cfg: &config.Config{AuthDir: authDir}}
			found := h.findExistingQoderCredential(tt.targetProv, uid, email, "")
			switch {
			case tt.wantFile == "" && found != nil:
				t.Fatalf("expected no match across realms, got %q", found.FileName)
			case tt.wantFile != "" && found == nil:
				t.Fatalf("expected match %q, got none", tt.wantFile)
			case tt.wantFile != "" && found.FileName != tt.wantFile:
				t.Fatalf("expected %q, got %q", tt.wantFile, found.FileName)
			}
		})
	}
}

// The in-memory branch must apply the same realm scoping as the on-disk branch.
func TestFindExistingQoderCredential_RealmIsolationInMemory(t *testing.T) {
	const uid = "01a15e15-7421-7847-99f8-eaf5298e1804"
	cnFile := "qoder-cn-" + uid + ".json"

	manager := coreauth.NewManager(nil, nil, nil)
	if _, err := manager.Register(t.Context(), &coreauth.Auth{
		ID:       cnFile,
		Provider: "qoder-cn",
		FileName: cnFile,
		Attributes: map[string]string{
			"user_id": uid,
			"realm":   "cn",
		},
	}); err != nil {
		t.Fatalf("manager.Register failed: %v", err)
	}

	h := &Handler{authManager: manager}
	if got := h.findExistingQoderCredential("qoder", uid, "", ""); got != nil {
		t.Fatalf("intl login must not adopt CN credential %q", got.FileName)
	}
	if got := h.findExistingQoderCredential("qoder-cn", uid, "", ""); got == nil || got.FileName != cnFile {
		t.Fatalf("cn login should adopt %q, got %v", cnFile, got)
	}
}
