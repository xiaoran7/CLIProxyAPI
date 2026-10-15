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
	entry, ok := findModel(t, got, modelID)
	if !ok {
		t.Fatalf("model %q missing from /v1/models", modelID)
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

func getModelsResponse(t *testing.T) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)

	base := handlers.NewBaseAPIHandlers(&config.SDKConfig{}, nil)
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
