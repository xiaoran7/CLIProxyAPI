package registry

import "testing"

// A shared model ID may be registered by several providers. Billing annotations
// are provider-specific, so a later provider without credits must not erase the
// rate an earlier one published.
func TestAddModelRegistrationPreservesCreditsAcrossProviders(t *testing.T) {
	r := newTestModelRegistry()

	rated := &ModelInfo{
		ID:      "glm-5.3-flash",
		OwnedBy: "workbuddy",
		Type:    "workbuddy",
		Credits: &ModelCredits{Multiplier: 0.06, Source: "workbuddy"},
	}
	r.RegisterClient("workbuddy-account", "workbuddy", []*ModelInfo{rated})

	// A second provider exposes the same ID without any billing information.
	unrated := &ModelInfo{ID: "glm-5.3-flash", OwnedBy: "opencode", Type: "opencode", ContextLength: 1048576}
	r.RegisterClient("opencode-account", "openai-compatible-opencode", []*ModelInfo{unrated})

	got := r.GetModelInfo("glm-5.3-flash", "")
	if got == nil {
		t.Fatal("model disappeared after second registration")
	}
	if got.Credits == nil {
		t.Fatal("credits were erased by a provider that has none")
	}
	if got.Credits.Multiplier != 0.06 {
		t.Fatalf("multiplier = %v, want 0.06", got.Credits.Multiplier)
	}
	if got.Credits.Source != "workbuddy" {
		t.Fatalf("source = %q, want workbuddy", got.Credits.Source)
	}
	// The later provider still owns the non-billing fields.
	if got.ContextLength != 1048576 {
		t.Fatalf("context length = %d, want 1048576", got.ContextLength)
	}
}

// A provider that does publish credits keeps ownership of them.
func TestAddModelRegistrationLaterCreditsWin(t *testing.T) {
	r := newTestModelRegistry()

	first := &ModelInfo{ID: "m", Credits: &ModelCredits{Multiplier: 1.0, Source: "workbuddy"}}
	r.RegisterClient("a", "workbuddy", []*ModelInfo{first})

	second := &ModelInfo{ID: "m", Credits: &ModelCredits{Multiplier: 2.5, Source: "workbuddy"}}
	r.RegisterClient("b", "workbuddy-cn", []*ModelInfo{second})

	got := r.GetModelInfo("m", "")
	if got == nil || got.Credits == nil {
		t.Fatal("credits missing")
	}
	if got.Credits.Multiplier != 2.5 {
		t.Fatalf("multiplier = %v, want 2.5", got.Credits.Multiplier)
	}
}
