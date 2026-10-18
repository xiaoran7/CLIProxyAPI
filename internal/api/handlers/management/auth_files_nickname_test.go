package management

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestListAuthFiles_IncludesNicknameAndUserIDFromManager(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")

	authDir := t.TempDir()
	fileName := "workbuddy-cn-1791808639195.json"
	filePath := filepath.Join(authDir, fileName)
	if errWrite := os.WriteFile(filePath, []byte(`{"type":"workbuddy-cn","nickname":"tangwg","user_id":"u-123456","realm":"cn"}`), 0o600); errWrite != nil {
		t.Fatalf("failed to write auth file: %v", errWrite)
	}

	manager := coreauth.NewManager(nil, nil, nil)
	record := &coreauth.Auth{
		ID:       fileName,
		FileName: fileName,
		Provider: "workbuddy-cn",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"path": filePath,
		},
		Metadata: map[string]any{
			"type":     "workbuddy-cn",
			"nickname": "tangwg",
			"user_id":  "u-123456",
			"realm":    "cn",
		},
	}
	if _, errRegister := manager.Register(context.Background(), record); errRegister != nil {
		t.Fatalf("failed to register auth record: %v", errRegister)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	h.tokenStore = &memoryAuthStore{}

	entry := firstAuthFileEntry(t, h)
	if got := entry["nickname"]; got != "tangwg" {
		t.Fatalf("expected nickname %q, got %#v", "tangwg", got)
	}
	if got := entry["user_id"]; got != "u-123456" {
		t.Fatalf("expected user_id %q, got %#v", "u-123456", got)
	}
	if got := entry["realm"]; got != "cn" {
		t.Fatalf("expected realm %q, got %#v", "cn", got)
	}
}

func TestListAuthFiles_NicknameFallbackFromLabel(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")

	authDir := t.TempDir()
	fileName := "workbuddy-cn-1791808639195.json"
	filePath := filepath.Join(authDir, fileName)
	if errWrite := os.WriteFile(filePath, []byte(`{"type":"workbuddy-cn"}`), 0o600); errWrite != nil {
		t.Fatalf("failed to write auth file: %v", errWrite)
	}

	manager := coreauth.NewManager(nil, nil, nil)
	record := &coreauth.Auth{
		ID:       fileName,
		FileName: fileName,
		Provider: "workbuddy-cn",
		Label:    "WorkBuddy (cn) (tangwg)",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"path": filePath,
		},
		Metadata: map[string]any{
			"type": "workbuddy-cn",
		},
	}
	if _, errRegister := manager.Register(context.Background(), record); errRegister != nil {
		t.Fatalf("failed to register auth record: %v", errRegister)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	h.tokenStore = &memoryAuthStore{}

	entry := firstAuthFileEntry(t, h)
	if got := entry["nickname"]; got != "tangwg" {
		t.Fatalf("expected nickname fallback %q, got %#v", "tangwg", got)
	}
}
