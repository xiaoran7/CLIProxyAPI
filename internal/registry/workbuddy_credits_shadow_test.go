package registry

import "testing"

// A model ID can be served by several providers at once, and the shared
// registration's Info is replaced on every registration. Credits belong to the
// provider that publishes them, so they must survive regardless of the order in
// which providers register. These tests follow the production ordering observed
// with a WorkBuddy account and an openai-compatibility upstream that both expose
// the same model IDs.
const shadowRate = 0.06

func ratedWorkBuddyModel(id string) *ModelInfo {
	return &ModelInfo{
		ID:      id,
		OwnedBy: "zhipu",
		Type:    "workbuddy",
		Credits: &ModelCredits{Multiplier: shadowRate, Source: "workbuddy"},
	}
}

func unratedOpenCodeModel(id string) *ModelInfo {
	return &ModelInfo{
		ID:            id,
		OwnedBy:       "zhipu",
		Type:          "opencode",
		ContextLength: 1048576,
	}
}

func requireCredits(t *testing.T, r *ModelRegistry, id string) *ModelCredits {
	t.Helper()
	if got := r.GetModelInfo(id, ""); got != nil {
		if got.Credits != nil {
			return got.Credits
		}
	}
	t.Fatalf("%s lost its credits", id)
	return nil
}

// An unrated provider registering after the rated one must not erase the rate.
func TestCreditsSurviveLaterUnratedProvider(t *testing.T) {
	r := newTestModelRegistry()
	r.RegisterClient("workbuddy-account", "workbuddy", []*ModelInfo{ratedWorkBuddyModel("glm-5.3-flash")})
	r.RegisterClient("opencode-account", "openai-compatible-opencode", []*ModelInfo{unratedOpenCodeModel("glm-5.3-flash")})

	credits := requireCredits(t, r, "glm-5.3-flash")
	if credits.Multiplier != shadowRate {
		t.Fatalf("multiplier = %v, want %v", credits.Multiplier, shadowRate)
	}
	if credits.Source != "workbuddy" {
		t.Fatalf("source = %q, want workbuddy", credits.Source)
	}
	// The later provider still owns the non-billing fields.
	if got := r.GetModelInfo("glm-5.3-flash", ""); got.ContextLength != 1048576 {
		t.Fatalf("context length = %d, want 1048576", got.ContextLength)
	}
}

// The production ordering: the unrated upstream creates the ID first, the rated
// provider then increments it, and later upstream clients keep re-registering.
func TestCreditsSurviveRatedProviderRegisteredAfterUnrated(t *testing.T) {
	r := newTestModelRegistry()
	r.RegisterClient("opencode-1", "openai-compatible-opencode", []*ModelInfo{unratedOpenCodeModel("glm-5.3-flash")})

	// The shared entry exists but knows no rate yet.
	if got := r.GetModelInfo("glm-5.3-flash", ""); got.Credits != nil {
		t.Fatalf("unexpected credits before the rated provider registered: %v", got.Credits)
	}

	r.RegisterClient("workbuddy-account", "workbuddy", []*ModelInfo{ratedWorkBuddyModel("glm-5.3-flash")})
	requireCredits(t, r, "glm-5.3-flash")

	// Repeated registrations from the unrated upstream must not clear the rate.
	for i, clientID := range []string{"opencode-2", "opencode-3", "opencode-4"} {
		r.RegisterClient(clientID, "openai-compatible-opencode", []*ModelInfo{unratedOpenCodeModel("glm-5.3-flash")})
		if credits := requireCredits(t, r, "glm-5.3-flash"); credits.Multiplier != shadowRate {
			t.Fatalf("after registration %d: multiplier = %v, want %v", i, credits.Multiplier, shadowRate)
		}
	}
}

// A client re-registering the same models takes a separate refresh path in
// registerClientLocked; it must preserve credits too.
func TestCreditsSurviveClientReRegistration(t *testing.T) {
	r := newTestModelRegistry()
	r.RegisterClient("opencode-1", "openai-compatible-opencode", []*ModelInfo{unratedOpenCodeModel("glm-5.3-flash")})
	r.RegisterClient("workbuddy-account", "workbuddy", []*ModelInfo{ratedWorkBuddyModel("glm-5.3-flash")})
	requireCredits(t, r, "glm-5.3-flash")

	// Same client ID, same provider, re-announcing its models.
	r.RegisterClient("workbuddy-account", "workbuddy", []*ModelInfo{ratedWorkBuddyModel("glm-5.3-flash")})
	requireCredits(t, r, "glm-5.3-flash")

	// And the unrated client re-announcing its own models.
	r.RegisterClient("opencode-1", "openai-compatible-opencode", []*ModelInfo{unratedOpenCodeModel("glm-5.3-flash")})
	credits := requireCredits(t, r, "glm-5.3-flash")
	if credits.Multiplier != shadowRate {
		t.Fatalf("multiplier = %v, want %v", credits.Multiplier, shadowRate)
	}
}

// A provider that does publish credits keeps ownership of them.
func TestCreditsFromLaterRatedProviderWin(t *testing.T) {
	r := newTestModelRegistry()
	r.RegisterClient("a", "workbuddy", []*ModelInfo{
		{ID: "m", Credits: &ModelCredits{Multiplier: 1.0, Source: "workbuddy"}},
	})
	r.RegisterClient("b", "workbuddy-cn", []*ModelInfo{
		{ID: "m", Credits: &ModelCredits{Multiplier: 2.5, Source: "workbuddy"}},
	})

	credits := requireCredits(t, r, "m")
	if credits.Multiplier != 2.5 {
		t.Fatalf("multiplier = %v, want 2.5", credits.Multiplier)
	}
}
