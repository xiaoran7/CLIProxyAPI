package auth

import (
	"context"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
)

func TestReconcileRegistryModelStates_PreservesDisabledStatus(t *testing.T) {
	ctx := context.Background()
	m := NewManager(nil, nil, nil)
	r := registry.GetGlobalRegistry()

	authID := "test-disabled-antigravity"
	models := []*registry.ModelInfo{{ID: "gemini-2.5-pro"}}
	r.RegisterClient(authID, "antigravity", models)
	t.Cleanup(func() { r.UnregisterClient(authID) })

	initialAuth := &Auth{
		ID:       authID,
		Provider: "antigravity",
		Status:   StatusDisabled,
		Disabled: true,
		ModelStates: map[string]*ModelState{
			"gemini-2.5-pro": {
				Status: StatusActive,
				Quota: QuotaState{
					Exceeded: false,
				},
			},
			"disappeared-model": {
				Status: StatusActive,
			},
		},
	}

	if _, err := m.Register(ctx, initialAuth); err != nil {
		t.Fatalf("failed to register auth: %v", err)
	}

	// Trigger reconciliation where candidateChanged is true due to model cleanup
	m.ReconcileRegistryModelStates(ctx, authID)

	got, ok := m.GetByID(authID)
	if !ok {
		t.Fatalf("auth not found after reconciliation")
	}

	if got.Status != StatusDisabled {
		t.Fatalf("expected auth status %s, got %s", StatusDisabled, got.Status)
	}
	if !got.Disabled {
		t.Fatalf("expected auth Disabled = true, got false")
	}
}

func TestClearAuthStateOnSuccess_PreservesDisabledStatus(t *testing.T) {
	now := time.Now()
	auth := &Auth{
		ID:          "test-disabled-auth",
		Status:      StatusDisabled,
		Disabled:    true,
		Unavailable: false,
	}

	clearAuthStateOnSuccess(auth, now)

	if auth.Status != StatusDisabled {
		t.Fatalf("expected status %s, got %s", StatusDisabled, auth.Status)
	}
	if !auth.Disabled {
		t.Fatalf("expected Disabled = true, got false")
	}
}
