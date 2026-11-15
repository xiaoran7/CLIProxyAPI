package auth_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/watcher/synthesizer"
	sdkauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/auth"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

const tosAuthID = "tos-test.json"

// TestMarkResultTOSViolationDeactivatesCredential drives the whole conductor
// path: a TOS_VIOLATION result error must deactivate the credential, keep the
// appeal metadata for the operator, and persist the disabled flag so the
// suspended account stays out of the rotation across restarts.
func TestMarkResultTOSViolationDeactivatesCredential(t *testing.T) {
	dir := t.TempDir()
	store := sdkauth.NewFileTokenStore()
	store.SetBaseDir(dir)
	manager := coreauth.NewManager(store, nil, nil)

	path := filepath.Join(dir, tosAuthID)
	if err := os.WriteFile(path, []byte(`{"type":"claude","email":"tos-test@example.com"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot := synthesizeTOSAuth(t, path)
	if snapshot.ID != tosAuthID {
		t.Fatalf("synthesized auth ID = %q, want %q", snapshot.ID, tosAuthID)
	}
	if _, err := manager.Register(context.Background(), snapshot); err != nil {
		t.Fatalf("register: %v", err)
	}

	manager.MarkResult(context.Background(), coreauth.Result{
		AuthID:   tosAuthID,
		Provider: "antigravity",
		Model:    "gemini-3.8-flash-high",
		Success:  false,
		Error: &coreauth.Error{
			Code:       "tos_violation",
			Message:    "This service has been disabled in this account for violation of Terms of Service.",
			HTTPStatus: 403,
			Metadata: map[string]string{
				"tos_appeal_url":      "https://forms.gle/hGzM9MEUv2azZsrb9",
				"tos_appeal_url_text": "Submit Appeal",
			},
		},
	})

	got, ok := manager.GetByID(tosAuthID)
	if !ok || got == nil {
		t.Fatal("the credential disappeared from the manager")
	}
	if !got.Disabled {
		t.Fatal("the credential is not disabled, want a permanent deactivation")
	}
	if got.Status != coreauth.StatusDisabled {
		t.Fatalf("status = %q, want %q", got.Status, coreauth.StatusDisabled)
	}
	if got.StatusMessage != "tos_violation" {
		t.Fatalf("status message = %q, want %q", got.StatusMessage, "tos_violation")
	}
	if got.Metadata["tos_appeal_url"] != "https://forms.gle/hGzM9MEUv2azZsrb9" {
		t.Fatalf("appeal metadata = %v, want the appeal url", got.Metadata["tos_appeal_url"])
	}

	// The disabled flag has to reach the auth file, otherwise the suspended
	// account comes back after a restart.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk map[string]any
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	if disabled, _ := disk["disabled"].(bool); !disabled {
		t.Fatalf("the auth file does not carry disabled=true: %s", raw)
	}
}

// TestMarkResultOrdinary403StaysCoolable guards the other half of the
// contract: a plain 403 without the TOS signature must keep the old
// behaviour and only cool the credential down.
func TestMarkResultOrdinary403StaysCoolable(t *testing.T) {
	dir := t.TempDir()
	store := sdkauth.NewFileTokenStore()
	store.SetBaseDir(dir)
	manager := coreauth.NewManager(store, nil, nil)

	path := filepath.Join(dir, tosAuthID)
	if err := os.WriteFile(path, []byte(`{"type":"claude","email":"tos-test@example.com"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Register(context.Background(), synthesizeTOSAuth(t, path)); err != nil {
		t.Fatalf("register: %v", err)
	}

	manager.MarkResult(context.Background(), coreauth.Result{
		AuthID:   tosAuthID,
		Provider: "antigravity",
		Model:    "gemini-3.8-flash-high",
		Success:  false,
		Error: &coreauth.Error{
			Message:    "permission denied",
			HTTPStatus: 403,
		},
	})

	got, ok := manager.GetByID(tosAuthID)
	if !ok || got == nil {
		t.Fatal("the credential disappeared from the manager")
	}
	if got.Disabled || got.Status == coreauth.StatusDisabled {
		t.Fatalf("a plain 403 disabled the credential: disabled=%v status=%q", got.Disabled, got.Status)
	}
}

func synthesizeTOSAuth(t *testing.T, path string) *coreauth.Auth {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	auths, err := synthesizer.SynthesizeAuthFile(&synthesizer.SynthesisContext{
		Config:      &config.Config{},
		AuthDir:     filepath.Dir(path),
		Now:         time.Now(),
		IDGenerator: synthesizer.NewStableIDGenerator(),
	}, path, data)
	if err != nil || len(auths) != 1 {
		t.Fatalf("synthesize %s: err=%v auths=%v", path, err, auths)
	}
	return auths[0]
}
