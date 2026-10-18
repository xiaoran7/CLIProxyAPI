package cliproxy

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

const workbuddyCreditsTestCatalog = `{"data":{"models":[
	{"id":"deepseek-v4.1-flash","credits":"x0.00"},
	{"id":"glm-5.3","credits":"x0.79"},
	{"id":"gpt-5.5","credits":"x3.31"}
],"modelPromotions":[
	{"id":"hy3-free","enabled":true,"modelIds":["hy3"],"discount":{"discountedCredits":"0x","factor":0}}
]}}`

func workbuddyCreditsTestAuth(id, realm string) *coreauth.Auth {
	provider := "workbuddy"
	if realm == "cn" {
		provider = "workbuddy-cn"
	}
	return &coreauth.Auth{
		ID:       id,
		Provider: provider,
		Metadata: map[string]any{"access_token": "token-" + id, "user_id": "uid-" + id},
	}
}

// TestWorkBuddyCreditMultipliersReachRegisteredModels is the end-to-end guard:
// a registered WorkBuddy account must publish credit multipliers on the models
// the registry exposes.
func TestWorkBuddyCreditMultipliersReachRegisteredModels(t *testing.T) {
	var requestedURL string
	restore := registry.SetWorkBuddyCreditsFetcherForTest(func(url string) ([]byte, bool) {
		requestedURL = url
		return []byte(workbuddyCreditsTestCatalog), true
	}, nil)
	defer restore()
	registry.InvalidateWorkBuddyCredits("")

	manager := coreauth.NewManager(nil, nil, nil)
	svc := &Service{coreManager: manager}
	auth := workbuddyCreditsTestAuth("wb-credits-intl", "intl")
	registered, err := manager.Register(t.Context(), auth)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { GlobalModelRegistry().UnregisterClient(registered.ID) })

	svc.registerModelsForAuth(t.Context(), registered)

	if requestedURL == "" {
		t.Fatal("registration did not fetch the credit catalogue")
	}
	if want := "https://www.workbuddy.ai/v3/config"; requestedURL != want {
		t.Errorf("catalogue URL = %q, want %q", requestedURL, want)
	}

	credits := make(map[string]float64)
	for _, m := range GlobalModelRegistry().GetModelsForClient(registered.ID) {
		if m.Credits != nil {
			credits[m.ID] = m.Credits.Multiplier
		}
	}

	if got, ok := credits["glm-5.3"]; !ok || got != 0.79 {
		t.Errorf("glm-5.3 credits = %v (present=%v), want 0.79", got, ok)
	}
	if got, ok := credits["deepseek-v4.1-flash"]; !ok || got != 0 {
		t.Errorf("deepseek-v4.1-flash credits = %v (present=%v), want 0", got, ok)
	}
	// A "-free" alias inherits the base model's rate.
	if got, ok := credits["deepseek-v4.1-flash-free"]; !ok || got != 0 {
		t.Errorf("deepseek-v4.1-flash-free credits = %v (present=%v), want 0", got, ok)
	}
	// The overlay annotates the static list; it must not invent models that the
	// registry does not publish, even when the catalogue knows their rate.
	if _, ok := credits["hy3"]; ok {
		t.Error("overlay added hy3, which is not in the static intl registry")
	}
}

// TestWorkBuddyCreditsUseRealmSpecificCatalog ensures a CN account reads the CN
// catalogue and does not adopt the international rates.
func TestWorkBuddyCreditsUseRealmSpecificCatalog(t *testing.T) {
	const cnCatalog = `{"data":{"models":[{"id":"deepseek-v4.1-flash","credits":"x0.11"}]}}`
	restore := registry.SetWorkBuddyCreditsFetcherForTest(func(url string) ([]byte, bool) {
		if url == "https://copilot.tencent.com/v3/config" {
			return []byte(cnCatalog), true
		}
		return []byte(workbuddyCreditsTestCatalog), true
	}, nil)
	defer restore()
	registry.InvalidateWorkBuddyCredits("")

	manager := coreauth.NewManager(nil, nil, nil)
	svc := &Service{coreManager: manager}
	auth := workbuddyCreditsTestAuth("wb-credits-cn", "cn")
	registered, err := manager.Register(t.Context(), auth)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { GlobalModelRegistry().UnregisterClient(registered.ID) })

	svc.registerModelsForAuth(t.Context(), registered)

	var found bool
	for _, m := range GlobalModelRegistry().GetModelsForClient(registered.ID) {
		if m.ID != "deepseek-v4.1-flash" {
			continue
		}
		found = true
		if m.Credits == nil {
			t.Fatal("cn deepseek-v4.1-flash has no credits")
		}
		if m.Credits.Multiplier != 0.11 {
			t.Errorf("cn multiplier = %v, want 0.11 (intl rate leaked?)", m.Credits.Multiplier)
		}
	}
	if !found {
		t.Fatal("cn deepseek-v4.1-flash not registered")
	}
}

// TestWorkBuddyRegistrationSurvivesCatalogFailure ensures a broken catalogue
// never removes models from the API surface.
func TestWorkBuddyRegistrationSurvivesCatalogFailure(t *testing.T) {
	restore := registry.SetWorkBuddyCreditsFetcherForTest(func(string) ([]byte, bool) {
		return nil, false
	}, nil)
	defer restore()
	registry.InvalidateWorkBuddyCredits("")

	manager := coreauth.NewManager(nil, nil, nil)
	svc := &Service{coreManager: manager}
	auth := workbuddyCreditsTestAuth("wb-credits-broken", "intl")
	registered, err := manager.Register(t.Context(), auth)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { GlobalModelRegistry().UnregisterClient(registered.ID) })

	svc.registerModelsForAuth(t.Context(), registered)

	models := GlobalModelRegistry().GetModelsForClient(registered.ID)
	if len(models) == 0 {
		t.Fatal("catalogue failure removed every model")
	}
	for _, m := range models {
		if m.Credits != nil && m.Credits.Source == registry.WorkBuddyCreditsSource {
			t.Errorf("%s got live credits from a failed fetch: %+v", m.ID, m.Credits)
		}
	}
}

// TestWorkBuddyCreditsEmittedInOpenAIModelList guards the client-visible shape.
func TestWorkBuddyCreditsEmittedInOpenAIModelList(t *testing.T) {
	restore := registry.SetWorkBuddyCreditsFetcherForTest(func(string) ([]byte, bool) {
		return []byte(workbuddyCreditsTestCatalog), true
	}, nil)
	defer restore()
	registry.InvalidateWorkBuddyCredits("")

	models := registry.GetWorkBuddyModels("intl")
	registry.ApplyWorkBuddyCredits(models, "intl")

	reg := registry.GetGlobalRegistry()
	reg.RegisterClient("wb-list-client", "workbuddy", models)

	payload := reg.GetAvailableModels("openai")
	if len(payload) == 0 {
		t.Fatal("no models returned")
	}

	var checked bool
	for _, entry := range payload {
		if entry["id"] != "glm-5.3" {
			continue
		}
		checked = true
		raw, ok := entry["credits"]
		if !ok {
			t.Fatalf("glm-5.3 entry has no credits field: %v", entry)
		}
		credits, okCast := raw.(map[string]any)
		if !okCast {
			t.Fatalf("credits field is %T, want map", raw)
		}
		if got, okMult := credits["multiplier"].(float64); !okMult || got != 0.79 {
			t.Errorf("credits.multiplier = %v, want 0.79", credits["multiplier"])
		}
		if free, okFree := credits["free"].(bool); !okFree || free {
			t.Errorf("credits.free = %v, want false", credits["free"])
		}
		if credits["source"] != "workbuddy" {
			t.Errorf("credits.source = %v, want workbuddy", credits["source"])
		}
	}
	if !checked {
		t.Fatal("glm-5.3 not found in the published model list")
	}
}
