package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/api/handlers"
)

// OpenAIModels rebuilds every entry into a fixed OpenAI shape, so any field it
// does not copy is invisible to clients no matter what the registry holds. The
// credits annotation is how a client tells a free model from a paid one, so it
// has to survive that rebuild.
func TestOpenAIModelsExposesCreditsAnnotation(t *testing.T) {
	const modelID = "openai-models-credits-test"
	clientID := "openai-models-credits-test-client"

	modelRegistry := registry.GetGlobalRegistry()
	modelRegistry.RegisterClient(clientID, "workbuddy", []*registry.ModelInfo{
		{
			ID:      modelID,
			OwnedBy: "zhipu",
			Credits: &registry.ModelCredits{
				Multiplier:    0.06,
				Source:        "workbuddy",
				PromotionID:   "promo-1",
				PromotionNote: "off-peak",
			},
		},
	})
	t.Cleanup(func() {
		modelRegistry.UnregisterClient(clientID)
	})

	got := getModelsResponse(t)
	targetID := "workbuddy/" + modelID + ":0.06x"
	entry, ok := findModel(t, got, targetID)
	if !ok {
		t.Fatalf("model %q missing from /v1/models", targetID)
	}

	credits, ok := entry["credits"].(map[string]any)
	if !ok {
		t.Fatalf("credits = %#v, want a map; the handler dropped the annotation", entry["credits"])
	}
	if multiplier, _ := credits["multiplier"].(float64); multiplier != 0.06 {
		t.Fatalf("multiplier = %#v, want 0.06", credits["multiplier"])
	}
	if source, _ := credits["source"].(string); source != "workbuddy" {
		t.Fatalf("source = %#v, want workbuddy", credits["source"])
	}
	if promotionID, _ := credits["promotion_id"].(string); promotionID != "promo-1" {
		t.Fatalf("promotion_id = %#v, want promo-1", credits["promotion_id"])
	}
	if promotionNote, _ := credits["promotion_note"].(string); promotionNote != "off-peak" {
		t.Fatalf("promotion_note = %#v, want off-peak", credits["promotion_note"])
	}

	// The OpenAI shape itself must stay intact.
	for _, key := range []string{"id", "object"} {
		if _, exists := entry[key]; !exists {
			t.Fatalf("required field %q missing: %#v", key, entry)
		}
	}
}

// A model without a published rate must not grow a credits key.
func TestOpenAIModelsOmitsCreditsWhenUnrated(t *testing.T) {
	const modelID = "openai-models-unrated-test"
	clientID := "openai-models-unrated-test-client"

	modelRegistry := registry.GetGlobalRegistry()
	modelRegistry.RegisterClient(clientID, "openai-compatibility", []*registry.ModelInfo{{ID: modelID}})
	t.Cleanup(func() {
		modelRegistry.UnregisterClient(clientID)
	})

	entry, ok := findModel(t, getModelsResponse(t), modelID)
	if !ok {
		t.Fatalf("model %q missing from /v1/models", modelID)
	}
	if _, exists := entry["credits"]; exists {
		t.Fatalf("unrated model advertised credits: %#v", entry["credits"])
	}
}

// Models with known multipliers must also emit suffixed variants for client display.
func TestOpenAIModelsEmitsMultiplierSuffixedModels(t *testing.T) {
	const modelID = "openai-models-mult-test"
	clientID := "openai-models-mult-test-client"

	modelRegistry := registry.GetGlobalRegistry()
	modelRegistry.RegisterClient(clientID, "workbuddy", []*registry.ModelInfo{
		{
			ID:      modelID,
			OwnedBy: "zhipu",
			Credits: &registry.ModelCredits{
				Multiplier: 0.06,
				Source:     "workbuddy",
			},
		},
	})
	t.Cleanup(func() {
		modelRegistry.UnregisterClient(clientID)
	})

	got := getModelsResponseWithConfig(t, &config.SDKConfig{ModelVariantMode: config.ModelVariantModeAll})
	// Original model must exist
	if _, ok := findModel(t, got, modelID); !ok {
		t.Fatalf("original model %q missing from /v1/models", modelID)
	}

	// Multiplier suffixed model must also exist for clients like ccswitch
	suffixedID := modelID + ":0.06x"
	entry, ok := findModel(t, got, suffixedID)
	if !ok {
		t.Fatalf("suffixed model %q missing from /v1/models", suffixedID)
	}
	if name, _ := entry["name"].(string); name == "" {
		t.Fatalf("suffixed model %q missing name attribute", suffixedID)
	}

	// Provider namespaced suffixed model must also exist to prevent ccswitch from classifying as Anthropic
	prefixedSuffixedID := "workbuddy/" + modelID + ":0.06x"
	prefixedEntry, ok := findModel(t, got, prefixedSuffixedID)
	if !ok {
		t.Fatalf("prefixed suffixed model %q missing from /v1/models", prefixedSuffixedID)
	}
	if ownedBy, _ := prefixedEntry["owned_by"].(string); ownedBy != "workbuddy" {
		t.Fatalf("owned_by = %q, want workbuddy", ownedBy)
	}

	// Plain prefixed model must also exist
	prefixedBaseID := "workbuddy/" + modelID
	if _, ok := findModel(t, got, prefixedBaseID); !ok {
		t.Fatalf("prefixed base model %q missing from /v1/models", prefixedBaseID)
	}
}

// TestOpenAIModelsVariantModes verifies that model variant modes correctly filter or emit variants:
//   - "compact" (default): emits exactly one entry per model with provider namespace and multiplier
//     suffix where applicable (e.g. "workbuddy-cn/hy4-preview-f:1.62x"). Redundant bare IDs and un-suffixed
//     variants are suppressed.
//   - "raw": emits only bare unprefixed model IDs without provider namespaces or multipliers.
//   - "all": emits all variants (bare, multiplier-annotated, and namespaced).
func TestOpenAIModelsVariantModes(t *testing.T) {
	const modelID = "hy4-preview-f"
	clientID := "openai-models-variant-modes-client"

	modelRegistry := registry.GetGlobalRegistry()
	modelRegistry.RegisterClient(clientID, "workbuddy", []*registry.ModelInfo{
		{
			ID:      modelID,
			OwnedBy: "tencent",
			Credits: &registry.ModelCredits{
				Multiplier: 0.0,
				Source:     "alias",
			},
		},
	})
	t.Cleanup(func() {
		modelRegistry.UnregisterClient(clientID)
	})

	t.Run("compact_default", func(t *testing.T) {
		got := getModelsResponseWithConfig(t, &config.SDKConfig{
			ModelVariantMode: config.ModelVariantModeCompact,
		})

		// Compact mode under Scheme A emits exactly one entry: namespaced with multiplier suffix
		const expectedSuffixedID = "workbuddy-cn/hy4-preview-f:1.62x"
		if _, ok := findModel(t, got, expectedSuffixedID); !ok {
			t.Fatalf("compact mode: expected suffixed ID %q in /v1/models, not found", expectedSuffixedID)
		}

		// Redundant bare ID must NOT exist
		if _, ok := findModel(t, got, modelID); ok {
			t.Fatalf("compact mode: redundant bare ID %q should be filtered out", modelID)
		}

		// Un-suffixed namespaced duplicate must NOT exist
		const unSuffixedID = "workbuddy-cn/hy4-preview-f"
		if _, ok := findModel(t, got, unSuffixedID); ok {
			t.Fatalf("compact mode: un-suffixed ID %q should be replaced by %q", unSuffixedID, expectedSuffixedID)
		}
		if _, ok := findModel(t, got, "hy4-preview-f:1.62x"); ok {
			t.Fatalf("compact mode: unprefixed suffixed variant hy4-preview-f:1.62x should be filtered out")
		}
	})

	t.Run("raw", func(t *testing.T) {
		got := getModelsResponseWithConfig(t, &config.SDKConfig{
			ModelVariantMode: config.ModelVariantModeRaw,
		})

		// Bare ID must exist
		if _, ok := findModel(t, got, modelID); !ok {
			t.Fatalf("raw mode: expected bare ID %q in /v1/models, not found", modelID)
		}

		// Namespaced ID must NOT exist
		if _, ok := findModel(t, got, "workbuddy-cn/hy4-preview-f"); ok {
			t.Fatalf("raw mode: namespaced ID workbuddy-cn/hy4-preview-f should not exist")
		}

		// Suffixed variants must NOT exist
		if _, ok := findModel(t, got, "hy4-preview-f:1.62x"); ok {
			t.Fatalf("raw mode: suffixed variant hy4-preview-f:1.62x should not exist")
		}
	})

	t.Run("all", func(t *testing.T) {
		got := getModelsResponseWithConfig(t, &config.SDKConfig{
			ModelVariantMode: config.ModelVariantModeAll,
		})

		// In "all" mode, bare ID, prefixed base, and suffixed variants must exist
		for _, expectedID := range []string{
			"hy4-preview-f",
			"workbuddy-cn/hy4-preview-f",
			"workbuddy-cn/hy4-preview-f:1.62x",
			"hy4-preview-f:1.62x",
		} {
			if _, ok := findModel(t, got, expectedID); !ok {
				t.Errorf("all mode: expected variant %q missing from /v1/models", expectedID)
			}
		}
	})
}

// TestOpenAIModelsMultiProviderNonWorkBuddyNotMasked verifies that when a model
// (such as Stepfun's "step-5-preview") is registered by both an OpenAI compatibility
// provider and WorkBuddy, compact mode does NOT mask the bare model ID under WorkBuddy's
// multiplier format ("workbuddy/step-5-preview:0x"). Both the clean bare model ID
// and the WorkBuddy variant must be present in /v1/models.
func TestOpenAIModelsMultiProviderNonWorkBuddyNotMasked(t *testing.T) {
	const modelID = "step-5-preview"
	compatClientID := "stepfun-compat-client"
	wbClientID := "workbuddy-client"

	modelRegistry := registry.GetGlobalRegistry()

	// 1. Register model from Stepfun (openai-compatibility)
	modelRegistry.RegisterClient(compatClientID, "stepfun", []*registry.ModelInfo{
		{
			ID:          modelID,
			OwnedBy:     "stepfun",
			DisplayName: "Step-5 Preview",
		},
	})

	// 2. Register same model from WorkBuddy (with credits / multiplier)
	modelRegistry.RegisterClient(wbClientID, "workbuddy", []*registry.ModelInfo{
		{
			ID:      modelID,
			OwnedBy: "workbuddy",
			Credits: &registry.ModelCredits{
				Multiplier: 0.0,
				Source:     "workbuddy",
			},
		},
	})

	t.Cleanup(func() {
		modelRegistry.UnregisterClient(compatClientID)
		modelRegistry.UnregisterClient(wbClientID)
	})

	got := getModelsResponseWithConfig(t, &config.SDKConfig{
		ModelVariantMode: config.ModelVariantModeCompact,
	})

	// Bare Stepfun model MUST be exposed in compact mode
	bareEntry, ok := findModel(t, got, modelID)
	if !ok {
		t.Fatalf("compact mode: bare model %q was masked and missing from /v1/models", modelID)
	}
	if ownedBy, _ := bareEntry["owned_by"].(string); ownedBy != "stepfun" {
		t.Errorf("bare model owned_by = %q, want stepfun", ownedBy)
	}

	// WorkBuddy namespaced multiplier entry MUST also be available
	const wbVariant = "workbuddy/step-5-preview:0x"
	wbEntry, ok := findModel(t, got, wbVariant)
	if !ok {
		t.Fatalf("compact mode: expected WorkBuddy variant %q in /v1/models", wbVariant)
	}
	if ownedBy, _ := wbEntry["owned_by"].(string); ownedBy != "workbuddy" {
		t.Errorf("workbuddy variant owned_by = %q, want workbuddy", ownedBy)
	}
}

// TestOpenAIModelsDualRealmWorkBuddyDeepSeek verifies that when a model like
// "deepseek-v4.1-flash" is registered by both WorkBuddy International and WorkBuddy CN,
// compact mode emits both "workbuddy/deepseek-v4.1-flash:0x" and "workbuddy-cn/deepseek-v4.1-flash:0.11x"
// without collision or masking.
func TestOpenAIModelsDualRealmWorkBuddyDeepSeek(t *testing.T) {
	const modelID = "deepseek-v4.1-flash"
	intlClientID := "workbuddy-intl-client"
	cnClientID := "workbuddy-cn-client"

	modelRegistry := registry.GetGlobalRegistry()

	// 1. Register model from International WorkBuddy
	modelRegistry.RegisterClient(intlClientID, "workbuddy", []*registry.ModelInfo{
		{
			ID:      modelID,
			OwnedBy: "deepseek",
		},
	})

	// 2. Register same model from Domestic WorkBuddy CN
	modelRegistry.RegisterClient(cnClientID, "workbuddy-cn", []*registry.ModelInfo{
		{
			ID:      modelID,
			OwnedBy: "deepseek",
		},
	})

	t.Cleanup(func() {
		modelRegistry.UnregisterClient(intlClientID)
		modelRegistry.UnregisterClient(cnClientID)
	})

	got := getModelsResponseWithConfig(t, &config.SDKConfig{
		ModelVariantMode: config.ModelVariantModeCompact,
	})

	// 1. Check International variant with promotional 0x rate
	const intlVariant = "workbuddy/deepseek-v4.1-flash:0x"
	intlEntry, ok := findModel(t, got, intlVariant)
	if !ok {
		t.Fatalf("compact mode: expected international variant %q in /v1/models", intlVariant)
	}
	if ownedBy, _ := intlEntry["owned_by"].(string); ownedBy != "workbuddy" {
		t.Errorf("international variant owned_by = %q, want workbuddy", ownedBy)
	}

	// 2. Check Domestic CN variant with authoritative 0.11x rate
	const cnVariant = "workbuddy-cn/deepseek-v4.1-flash:0.11x"
	cnEntry, ok := findModel(t, got, cnVariant)
	if !ok {
		t.Fatalf("compact mode: expected domestic CN variant %q in /v1/models", cnVariant)
	}
	if ownedBy, _ := cnEntry["owned_by"].(string); ownedBy != "workbuddy-cn" {
		t.Errorf("domestic CN variant owned_by = %q, want workbuddy-cn", ownedBy)
	}
}

func getModelsResponse(t *testing.T) map[string]any {
	return getModelsResponseWithConfig(t, &config.SDKConfig{})
}

func getModelsResponseWithConfig(t *testing.T, cfg *config.SDKConfig) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)

	base := handlers.NewBaseAPIHandlers(cfg, nil)
	handler := NewOpenAIAPIHandler(base)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	handler.OpenAIModels(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return decoded
}

func findModel(t *testing.T, body map[string]any, modelID string) (map[string]any, bool) {
	t.Helper()
	data, _ := body["data"].([]any)
	for _, raw := range data {
		entry, _ := raw.(map[string]any)
		if id, _ := entry["id"].(string); id == modelID {
			return entry, true
		}
	}
	return nil, false
}
