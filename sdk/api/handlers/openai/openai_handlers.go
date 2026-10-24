// Package openai provides HTTP handlers for OpenAI API endpoints.
// This package implements the OpenAI-compatible API interface, including model listing
// and chat completion functionality. It supports both streaming and non-streaming responses,
// and manages a pool of clients to interact with backend services.
// The handlers translate OpenAI API requests to the appropriate backend format and
// convert responses back to OpenAI-compatible format.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	codexmodels "github.com/router-for-me/CLIProxyAPI/v8/internal/client/codex/models"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/clienterror"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	. "github.com/router-for-me/CLIProxyAPI/v8/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	responsesconverter "github.com/router-for-me/CLIProxyAPI/v8/internal/translator/openai/openai/responses"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/api/handlers"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// OpenAIAPIHandler contains the handlers for OpenAI API endpoints.
// It holds a pool of clients to interact with the backend service.
type OpenAIAPIHandler struct {
	*handlers.BaseAPIHandler
}

// NewOpenAIAPIHandler creates a new OpenAI API handlers instance.
// It takes an BaseAPIHandler instance as input and returns an OpenAIAPIHandler.
//
// Parameters:
//   - apiHandlers: The base API handlers instance
//
// Returns:
//   - *OpenAIAPIHandler: A new OpenAI API handlers instance
func NewOpenAIAPIHandler(apiHandlers *handlers.BaseAPIHandler) *OpenAIAPIHandler {
	return &OpenAIAPIHandler{
		BaseAPIHandler: apiHandlers,
	}
}

// HandlerType returns the identifier for this handler implementation.
func (h *OpenAIAPIHandler) HandlerType() string {
	return OpenAI
}

// Models returns the OpenAI-compatible model metadata supported by this handler.
func (h *OpenAIAPIHandler) Models() []map[string]any {
	// Get dynamic models from the global registry
	modelRegistry := registry.GetGlobalRegistry()
	return modelRegistry.GetAvailableModels("openai")
}

// OpenAIModels handles the /v1/models endpoint.
// It returns a list of available AI models with their capabilities
// and specifications in OpenAI-compatible format.
func (h *OpenAIAPIHandler) OpenAIModels(c *gin.Context) {
	if _, ok := c.Request.URL.Query()["client_version"]; ok {
		clientVersion := c.Query("client_version")
		body, errMarshal := codexmodels.MarshalCompact(h.codexClientModelsResponse(clientVersion))
		if errMarshal != nil {
			c.JSON(http.StatusInternalServerError, handlers.ErrorResponse{
				Error: handlers.ErrorDetail{
					Message: fmt.Sprintf("Failed to encode model list: %v", errMarshal),
					Type:    "server_error",
				},
			})
			return
		}
		h.WriteModelListResponse(c, h.HandlerType(), body)
		return
	}

	// Get all available models
	allModels := h.Models()
	modelRegistry := registry.GetGlobalRegistry()

	variantMode := config.ModelVariantModeCompact
	if cfg := h.CurrentConfig(); cfg != nil && cfg.ModelVariantMode != "" {
		variantMode = config.NormalizeModelVariantMode(string(cfg.ModelVariantMode))
	}

	// Filter to include the required OpenAI fields plus the provider billing annotation.
	// Mode determines how variants are emitted:
	//   - "compact" (default): filters redundant multiplier and bare variants, keeping only clean
	//     namespaced IDs (e.g. "workbuddy-cn/hy4-preview-f") for models with a provider prefix,
	//     and bare IDs for other models.
	//   - "raw": emits only the bare unprefixed model ID without multiplier or provider namespace.
	//   - "all": emits all variants (bare, with multiplier, namespaced, namespaced with multiplier).
	filteredModels := make([]map[string]any, 0, len(allModels)*2)
	seenIDs := make(map[string]bool)

	for _, model := range allModels {
		modelID, _ := model["id"].(string)
		if modelID == "" {
			continue
		}

		var creditsMap map[string]any
		if credits, exists := model["credits"]; exists && credits != nil {
			if cm, ok := credits.(map[string]any); ok {
				creditsMap = cm
			}
		}

		// Fallback: If model doesn't have credits, resolve from WorkBuddy credit catalog only for WorkBuddy models
		if creditsMap == nil {
			ownedBy, _ := model["owned_by"].(string)
			modelType, _ := model["type"].(string)
			lowerID := strings.ToLower(modelID)
			if strings.HasPrefix(lowerID, "workbuddy") || strings.HasPrefix(lowerID, "codebuddy") || strings.HasPrefix(lowerID, "wb-") || strings.HasPrefix(lowerID, "hy") || strings.EqualFold(ownedBy, "workbuddy") || strings.EqualFold(ownedBy, "tencent") || strings.EqualFold(modelType, "workbuddy") {
				if rate := registry.ResolveWorkBuddyModelCreditRate(modelID); rate != nil {
					creditsMap = map[string]any{
						"multiplier": rate.Multiplier,
						"free":       rate.Free,
						"source":     rate.Source,
					}
					if rate.PromotionID != "" {
						creditsMap["promotion_id"] = rate.PromotionID
					}
					if rate.PromotionNote != "" {
						creditsMap["promotion_note"] = rate.PromotionNote
					}
				}
			}
		}

		// Ensure -free aliases are always marked with 0 multiplier
		if strings.HasSuffix(strings.ToLower(modelID), "-free") && creditsMap != nil {
			creditsMap["multiplier"] = 0.0
			creditsMap["free"] = true
		}

		cleanBaseModel := registry.TrimWorkBuddyPrefixes(modelID)

		// Determine registered providers for this model to decouple multi-realm WorkBuddy models
		providers := modelRegistry.GetModelProviders(cleanBaseModel)
		if len(providers) == 0 {
			providers = modelRegistry.GetModelProviders(modelID)
		}

		hasWBIntl := false
		hasWBCN := false
		hasNonWorkBuddy := false
		var nonWBOwnedBy string

		for _, p := range providers {
			pLower := strings.ToLower(strings.TrimSpace(p))
			switch pLower {
			case "workbuddy-intl":
				hasWBIntl = true
			case "workbuddy-cn", "codebuddy", "copilot-tencent":
				hasWBCN = true
			case "workbuddy":
				if strings.HasPrefix(strings.ToLower(cleanBaseModel), "hy") {
					hasWBCN = true
				} else {
					hasWBIntl = true
				}
			default:
				hasNonWorkBuddy = true
				if nonWBOwnedBy == "" {
					nonWBOwnedBy = p
				}
			}
		}

		// If modelID has explicit realm prefix, restrict to that realm
		lowerID := strings.ToLower(modelID)
		if strings.HasPrefix(lowerID, "workbuddy-cn/") || strings.HasPrefix(lowerID, "codebuddy/") || strings.HasPrefix(lowerID, "wb-cn/") {
			hasWBIntl = false
			hasWBCN = true
			hasNonWorkBuddy = false
		} else if strings.HasPrefix(lowerID, "workbuddy/") || strings.HasPrefix(lowerID, "workbuddy-intl/") || strings.HasPrefix(lowerID, "wb/") {
			hasWBIntl = true
			hasWBCN = false
			hasNonWorkBuddy = false
		}

		// Fallback when registry does not have active provider entries (e.g. offline/mock tests)
		if !hasWBIntl && !hasWBCN && !hasNonWorkBuddy {
			ownedBy, _ := model["owned_by"].(string)
			modelType, _ := model["type"].(string)
			if strings.HasPrefix(strings.ToLower(cleanBaseModel), "hy") {
				hasWBCN = true
			} else if strings.EqualFold(ownedBy, "workbuddy") || strings.EqualFold(modelType, "workbuddy") || strings.EqualFold(ownedBy, "tencent") {
				_, prov := registry.DetectWorkBuddyModelRealm(modelID)
				if prov == "workbuddy-cn" {
					hasWBCN = true
				} else {
					hasWBIntl = true
				}
			}
		}

		// CN realm has no -free tier/models; prevent emitting invalid CN -free entries
		if strings.HasSuffix(strings.ToLower(cleanBaseModel), "-free") {
			hasWBCN = false
		}

		resolveWBRealmRate := func(realm string) (string, map[string]any) {
			rate := registry.LookupWorkBuddyModelCredits(cleanBaseModel, realm)
			if rate == nil {
				rate = registry.GetWorkBuddyModelCredits(cleanBaseModel, realm)
			}
			if rate != nil {
				effStr := rate.Effective
				if effStr == "" {
					if rate.Multiplier < 1e-9 {
						effStr = "0x"
					} else {
						effStr = fmt.Sprintf("%.2fx", rate.Multiplier)
					}
				}
				cMap := map[string]any{
					"multiplier": rate.Multiplier,
					"free":       rate.Free,
					"source":     rate.Source,
				}
				if rate.PromotionID != "" {
					cMap["promotion_id"] = rate.PromotionID
				}
				if rate.PromotionNote != "" {
					cMap["promotion_note"] = rate.PromotionNote
				}
				return effStr, cMap
			}
			if cm, exists := model["credits"]; exists && cm != nil {
				if m, ok := cm.(map[string]any); ok {
					effStr := ""
					if mult, okMult := m["multiplier"].(float64); okMult {
						if mult < 1e-9 {
							effStr = "0x"
						} else {
							effStr = fmt.Sprintf("%.2fx", mult)
						}
					}
					return effStr, m
				}
			}
			return "", nil
		}

		switch variantMode {
		case config.ModelVariantModeRaw:
			if seenIDs[cleanBaseModel] {
				continue
			}
			seenIDs[cleanBaseModel] = true

			filteredModel := map[string]any{
				"id":     cleanBaseModel,
				"object": model["object"],
			}
			if created, exists := model["created"]; exists {
				filteredModel["created"] = created
			}
			if ownedBy, exists := model["owned_by"]; exists {
				filteredModel["owned_by"] = ownedBy
			}
			if hasWBCN && !hasWBIntl {
				filteredModel["owned_by"] = "workbuddy-cn"
			} else if hasWBIntl && !hasWBCN {
				filteredModel["owned_by"] = "workbuddy"
			}
			if dn, exists := model["display_name"]; exists && dn != nil {
				filteredModel["display_name"] = dn
				filteredModel["name"] = dn
			}
			filteredModels = append(filteredModels, filteredModel)

		case config.ModelVariantModeCompact:
			// "compact" (default): Emit exactly one clean entry per model provider.
			// For multi-provider models (e.g. WorkBuddy Intl and WorkBuddy CN, or WorkBuddy and non-WorkBuddy),
			// emit distinct namespaced entries so routes are never masked.
			if hasNonWorkBuddy {
				if !seenIDs[cleanBaseModel] {
					seenIDs[cleanBaseModel] = true
					bareOwnedBy := nonWBOwnedBy
					if bareOwnedBy == "" {
						if origOwnedBy, ok := model["owned_by"].(string); ok && origOwnedBy != "" && !strings.HasPrefix(strings.ToLower(origOwnedBy), "workbuddy") && !strings.HasPrefix(strings.ToLower(origOwnedBy), "codebuddy") {
							bareOwnedBy = origOwnedBy
						}
					}
					bareModel := map[string]any{
						"id":           cleanBaseModel,
						"object":       model["object"],
						"display_name": cleanBaseModel,
						"name":         cleanBaseModel,
					}
					if created, exists := model["created"]; exists {
						bareModel["created"] = created
					}
					if bareOwnedBy != "" {
						bareModel["owned_by"] = bareOwnedBy
					}
					filteredModels = append(filteredModels, bareModel)
				}
			}

			// Emit WorkBuddy International variant if provided
			if hasWBIntl {
				effStr, cMap := resolveWBRealmRate("intl")
				targetID := "workbuddy/" + cleanBaseModel
				displayName := "workbuddy/" + cleanBaseModel
				if effStr != "" {
					targetID = fmt.Sprintf("workbuddy/%s:%s", cleanBaseModel, effStr)
					displayName = fmt.Sprintf("workbuddy/%s (%s)", cleanBaseModel, effStr)
				}
				if !seenIDs[targetID] {
					seenIDs[targetID] = true
					compactModel := map[string]any{
						"id":           targetID,
						"object":       model["object"],
						"display_name": displayName,
						"name":         displayName,
						"owned_by":     "workbuddy",
					}
					if created, exists := model["created"]; exists {
						compactModel["created"] = created
					}
					if cMap != nil {
						compactModel["credits"] = cMap
					}
					filteredModels = append(filteredModels, compactModel)
				}
			}

			// Emit WorkBuddy Domestic (CN) variant if provided
			if hasWBCN {
				effStr, cMap := resolveWBRealmRate("cn")
				targetID := "workbuddy-cn/" + cleanBaseModel
				displayName := "workbuddy-cn/" + cleanBaseModel
				if effStr != "" {
					targetID = fmt.Sprintf("workbuddy-cn/%s:%s", cleanBaseModel, effStr)
					displayName = fmt.Sprintf("workbuddy-cn/%s (%s)", cleanBaseModel, effStr)
				}
				if !seenIDs[targetID] {
					seenIDs[targetID] = true
					compactModel := map[string]any{
						"id":           targetID,
						"object":       model["object"],
						"display_name": displayName,
						"name":         displayName,
						"owned_by":     "workbuddy-cn",
					}
					if created, exists := model["created"]; exists {
						compactModel["created"] = created
					}
					if cMap != nil {
						compactModel["credits"] = cMap
					}
					filteredModels = append(filteredModels, compactModel)
				}
			}

			// If not provided by WorkBuddy at all and not yet emitted as bare non-WorkBuddy model
			if !hasWBIntl && !hasWBCN && !hasNonWorkBuddy {
				targetID := cleanBaseModel
				displayName := cleanBaseModel
				targetOwnedBy, _ := model["owned_by"].(string)
				var cMap map[string]any
				if credits, exists := model["credits"]; exists && credits != nil {
					if cm, ok := credits.(map[string]any); ok {
						cMap = cm
					}
				}
				if cMap != nil {
					if mult, ok := cMap["multiplier"].(float64); ok {
						effStr := fmt.Sprintf("%.2fx", mult)
						if mult < 1e-9 {
							effStr = "0x"
						}
						targetID = fmt.Sprintf("%s:%s", cleanBaseModel, effStr)
						displayName = fmt.Sprintf("%s (%s)", cleanBaseModel, effStr)
					}
				}
				if !seenIDs[targetID] {
					seenIDs[targetID] = true
					compactModel := map[string]any{
						"id":           targetID,
						"object":       model["object"],
						"display_name": displayName,
						"name":         displayName,
					}
					if created, exists := model["created"]; exists {
						compactModel["created"] = created
					}
					if targetOwnedBy != "" {
						compactModel["owned_by"] = targetOwnedBy
					}
					if cMap != nil {
						compactModel["credits"] = cMap
					}
					filteredModels = append(filteredModels, compactModel)
				}
			}

		case config.ModelVariantModeAll:
			// "all": Emit all variants (bare, with multiplier, namespaced, namespaced with multiplier)
			if !seenIDs[cleanBaseModel] {
				seenIDs[cleanBaseModel] = true
				filteredModel := map[string]any{
					"id":     cleanBaseModel,
					"object": model["object"],
				}
				if created, exists := model["created"]; exists {
					filteredModel["created"] = created
				}
				if ownedBy, exists := model["owned_by"]; exists {
					filteredModel["owned_by"] = ownedBy
				}
				if dn, exists := model["display_name"]; exists && dn != nil {
					filteredModel["display_name"] = dn
					filteredModel["name"] = dn
				}
				filteredModels = append(filteredModels, filteredModel)
			}

			if hasWBIntl {
				effStr, cMap := resolveWBRealmRate("intl")
				if effStr != "" {
					prefixedSuffixedID := fmt.Sprintf("workbuddy/%s:%s", cleanBaseModel, effStr)
					if !seenIDs[prefixedSuffixedID] {
						seenIDs[prefixedSuffixedID] = true
						suffixedModel := map[string]any{
							"id":           prefixedSuffixedID,
							"object":       model["object"],
							"owned_by":     "workbuddy",
							"credits":      cMap,
							"display_name": fmt.Sprintf("workbuddy/%s (%s)", cleanBaseModel, effStr),
							"name":         fmt.Sprintf("workbuddy/%s (%s)", cleanBaseModel, effStr),
						}
						if created, exists := model["created"]; exists {
							suffixedModel["created"] = created
						}
						filteredModels = append(filteredModels, suffixedModel)
					}

					unprefixedSuffixedID := fmt.Sprintf("%s:%s", cleanBaseModel, effStr)
					if !seenIDs[unprefixedSuffixedID] {
						seenIDs[unprefixedSuffixedID] = true
						suffixedModel := map[string]any{
							"id":           unprefixedSuffixedID,
							"object":       model["object"],
							"owned_by":     "workbuddy",
							"credits":      cMap,
							"display_name": fmt.Sprintf("%s (%s)", cleanBaseModel, effStr),
							"name":         fmt.Sprintf("%s (%s)", cleanBaseModel, effStr),
						}
						if created, exists := model["created"]; exists {
							suffixedModel["created"] = created
						}
						filteredModels = append(filteredModels, suffixedModel)
					}
				}

				prefixedBaseID := fmt.Sprintf("workbuddy/%s", cleanBaseModel)
				if !seenIDs[prefixedBaseID] {
					seenIDs[prefixedBaseID] = true
					prefixedModel := map[string]any{
						"id":           prefixedBaseID,
						"object":       model["object"],
						"owned_by":     "workbuddy",
						"display_name": fmt.Sprintf("workbuddy/%s", cleanBaseModel),
						"name":         fmt.Sprintf("workbuddy/%s", cleanBaseModel),
					}
					if created, exists := model["created"]; exists {
						prefixedModel["created"] = created
					}
					if cMap != nil {
						prefixedModel["credits"] = cMap
					}
					filteredModels = append(filteredModels, prefixedModel)
				}
			}

			if hasWBCN {
				effStr, cMap := resolveWBRealmRate("cn")
				if effStr != "" {
					prefixedSuffixedID := fmt.Sprintf("workbuddy-cn/%s:%s", cleanBaseModel, effStr)
					if !seenIDs[prefixedSuffixedID] {
						seenIDs[prefixedSuffixedID] = true
						suffixedModel := map[string]any{
							"id":           prefixedSuffixedID,
							"object":       model["object"],
							"owned_by":     "workbuddy-cn",
							"credits":      cMap,
							"display_name": fmt.Sprintf("workbuddy-cn/%s (%s)", cleanBaseModel, effStr),
							"name":         fmt.Sprintf("workbuddy-cn/%s (%s)", cleanBaseModel, effStr),
						}
						if created, exists := model["created"]; exists {
							suffixedModel["created"] = created
						}
						filteredModels = append(filteredModels, suffixedModel)
					}

					unprefixedSuffixedID := fmt.Sprintf("%s:%s", cleanBaseModel, effStr)
					if !seenIDs[unprefixedSuffixedID] {
						seenIDs[unprefixedSuffixedID] = true
						suffixedModel := map[string]any{
							"id":           unprefixedSuffixedID,
							"object":       model["object"],
							"owned_by":     "workbuddy-cn",
							"credits":      cMap,
							"display_name": fmt.Sprintf("%s (%s)", cleanBaseModel, effStr),
							"name":         fmt.Sprintf("%s (%s)", cleanBaseModel, effStr),
						}
						if created, exists := model["created"]; exists {
							suffixedModel["created"] = created
						}
						filteredModels = append(filteredModels, suffixedModel)
					}
				}

				prefixedBaseID := fmt.Sprintf("workbuddy-cn/%s", cleanBaseModel)
				if !seenIDs[prefixedBaseID] {
					seenIDs[prefixedBaseID] = true
					prefixedModel := map[string]any{
						"id":           prefixedBaseID,
						"object":       model["object"],
						"owned_by":     "workbuddy-cn",
						"display_name": fmt.Sprintf("workbuddy-cn/%s", cleanBaseModel),
						"name":         fmt.Sprintf("workbuddy-cn/%s", cleanBaseModel),
					}
					if created, exists := model["created"]; exists {
						prefixedModel["created"] = created
					}
					if cMap != nil {
						prefixedModel["credits"] = cMap
					}
					filteredModels = append(filteredModels, prefixedModel)
				}
			}
		}
	}

	h.WriteModelListResponse(c, h.HandlerType(), gin.H{
		"object": "list",
		"data":   filteredModels,
	})
}

// ChatCompletions handles the /v1/chat/completions endpoint.
// It determines whether the request is for a streaming or non-streaming response
// and calls the appropriate handler based on the model provider.
//
// Parameters:
//   - c: The Gin context containing the HTTP request and response
func (h *OpenAIAPIHandler) ChatCompletions(c *gin.Context) {
	rawJSON, err := handlers.ReadRequestBody(c)
	// If data retrieval fails, return a 400 Bad Request error.
	if err != nil {
		c.JSON(clienterror.HTTPStatusFromErrorOr(err, http.StatusBadRequest), handlers.ErrorResponse{
			Error: handlers.ErrorDetail{
				Message: fmt.Sprintf("Invalid request: %v", err),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Check if the client requested a streaming response.
	streamResult := gjson.GetBytes(rawJSON, "stream")
	stream := streamResult.Type == gjson.True

	// Some clients send OpenAI Responses-format payloads to /v1/chat/completions.
	// Convert them to Chat Completions so downstream translators preserve tool metadata.
	if shouldTreatAsResponsesFormat(rawJSON) {
		modelName := gjson.GetBytes(rawJSON, "model").String()
		rawJSON, _ = responsesconverter.ConvertOpenAIResponsesRequestToOpenAIChatCompletions(modelName, rawJSON, stream)
		stream = gjson.GetBytes(rawJSON, "stream").Bool()
	}

	if stream {
		h.handleStreamingResponse(c, rawJSON)
	} else {
		h.handleNonStreamingResponse(c, rawJSON)
	}

}

// shouldTreatAsResponsesFormat detects OpenAI Responses-style payloads that are
// accidentally sent to the Chat Completions endpoint.
func shouldTreatAsResponsesFormat(rawJSON []byte) bool {
	if gjson.GetBytes(rawJSON, "messages").Exists() {
		return false
	}
	if gjson.GetBytes(rawJSON, "input").Exists() {
		return true
	}
	if gjson.GetBytes(rawJSON, "instructions").Exists() {
		return true
	}
	return false
}

// Completions handles the /v1/completions endpoint.
// It determines whether the request is for a streaming or non-streaming response
// and calls the appropriate handler based on the model provider.
// This endpoint follows the OpenAI completions API specification.
//
// Parameters:
//   - c: The Gin context containing the HTTP request and response
func (h *OpenAIAPIHandler) Completions(c *gin.Context) {
	rawJSON, err := handlers.ReadRequestBody(c)
	// If data retrieval fails, return a 400 Bad Request error.
	if err != nil {
		c.JSON(clienterror.HTTPStatusFromErrorOr(err, http.StatusBadRequest), handlers.ErrorResponse{
			Error: handlers.ErrorDetail{
				Message: fmt.Sprintf("Invalid request: %v", err),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Check if the client requested a streaming response.
	streamResult := gjson.GetBytes(rawJSON, "stream")
	if streamResult.Type == gjson.True {
		h.handleCompletionsStreamingResponse(c, rawJSON)
	} else {
		h.handleCompletionsNonStreamingResponse(c, rawJSON)
	}

}

// convertCompletionsRequestToChatCompletions converts OpenAI completions API request to chat completions format.
// This allows the completions endpoint to use the existing chat completions infrastructure.
//
// Parameters:
//   - rawJSON: The raw JSON bytes of the completions request
//
// Returns:
//   - []byte: The converted chat completions request
func convertCompletionsRequestToChatCompletions(rawJSON []byte) []byte {
	root := gjson.ParseBytes(rawJSON)

	// Extract prompt from completions request
	prompt := root.Get("prompt").String()
	if prompt == "" {
		prompt = "Complete this:"
	}

	// Create chat completions structure
	out := []byte(`{"model":"","messages":[{"role":"user","content":""}]}`)

	// Set model
	if model := root.Get("model"); model.Exists() {
		out, _ = sjson.SetBytes(out, "model", model.String())
	}

	// Set the prompt as user message content
	out, _ = sjson.SetBytes(out, "messages.0.content", prompt)

	// Copy other parameters from completions to chat completions
	if maxTokens := root.Get("max_tokens"); maxTokens.Exists() {
		out, _ = sjson.SetBytes(out, "max_tokens", maxTokens.Int())
	}

	if temperature := root.Get("temperature"); temperature.Exists() {
		out, _ = sjson.SetBytes(out, "temperature", temperature.Float())
	}

	if topP := root.Get("top_p"); topP.Exists() {
		out, _ = sjson.SetBytes(out, "top_p", topP.Float())
	}

	if frequencyPenalty := root.Get("frequency_penalty"); frequencyPenalty.Exists() {
		out, _ = sjson.SetBytes(out, "frequency_penalty", frequencyPenalty.Float())
	}

	if presencePenalty := root.Get("presence_penalty"); presencePenalty.Exists() {
		out, _ = sjson.SetBytes(out, "presence_penalty", presencePenalty.Float())
	}

	if stop := root.Get("stop"); stop.Exists() {
		out, _ = sjson.SetRawBytes(out, "stop", []byte(stop.Raw))
	}

	if stream := root.Get("stream"); stream.Exists() {
		out, _ = sjson.SetBytes(out, "stream", stream.Bool())
	}

	if logprobs := root.Get("logprobs"); logprobs.Exists() {
		out, _ = sjson.SetBytes(out, "logprobs", logprobs.Bool())
	}

	if topLogprobs := root.Get("top_logprobs"); topLogprobs.Exists() {
		out, _ = sjson.SetBytes(out, "top_logprobs", topLogprobs.Int())
	}

	if echo := root.Get("echo"); echo.Exists() {
		out, _ = sjson.SetBytes(out, "echo", echo.Bool())
	}

	return out
}

// convertChatCompletionsResponseToCompletions converts chat completions API response back to completions format.
// This ensures the completions endpoint returns data in the expected format.
//
// Parameters:
//   - rawJSON: The raw JSON bytes of the chat completions response
//
// Returns:
//   - []byte: The converted completions response
func convertChatCompletionsResponseToCompletions(rawJSON []byte) []byte {
	root := gjson.ParseBytes(rawJSON)

	// Base completions response structure
	out := []byte(`{"id":"","object":"text_completion","created":0,"model":"","choices":[]}`)

	// Copy basic fields
	if id := root.Get("id"); id.Exists() {
		out, _ = sjson.SetBytes(out, "id", id.String())
	}

	if created := root.Get("created"); created.Exists() {
		out, _ = sjson.SetBytes(out, "created", created.Int())
	}

	if model := root.Get("model"); model.Exists() {
		out, _ = sjson.SetBytes(out, "model", model.String())
	}

	if usage := root.Get("usage"); usage.Exists() {
		out, _ = sjson.SetRawBytes(out, "usage", []byte(usage.Raw))
	}

	// Convert choices from chat completions to completions format
	var choices []interface{}
	if chatChoices := root.Get("choices"); chatChoices.Exists() && chatChoices.IsArray() {
		chatChoices.ForEach(func(_, choice gjson.Result) bool {
			completionsChoice := map[string]interface{}{
				"index": choice.Get("index").Int(),
			}

			// Extract text content from message.content
			if message := choice.Get("message"); message.Exists() {
				if content := message.Get("content"); content.Exists() {
					completionsChoice["text"] = content.String()
				}
			} else if delta := choice.Get("delta"); delta.Exists() {
				// For streaming responses, use delta.content
				if content := delta.Get("content"); content.Exists() {
					completionsChoice["text"] = content.String()
				}
			}

			// Copy finish_reason
			if finishReason := choice.Get("finish_reason"); finishReason.Exists() {
				completionsChoice["finish_reason"] = finishReason.String()
			}

			// Copy logprobs if present
			if logprobs := choice.Get("logprobs"); logprobs.Exists() {
				completionsChoice["logprobs"] = logprobs.Value()
			}

			choices = append(choices, completionsChoice)
			return true
		})
	}

	if len(choices) > 0 {
		choicesJSON, _ := json.Marshal(choices)
		out, _ = sjson.SetRawBytes(out, "choices", choicesJSON)
	}

	return out
}

// convertChatCompletionsStreamChunkToCompletions converts a streaming chat completions chunk to completions format.
// This handles the real-time conversion of streaming response chunks and filters out empty text responses.
//
// Parameters:
//   - chunkData: The raw JSON bytes of a single chat completions stream chunk
//
// Returns:
//   - []byte: The converted completions stream chunk, or nil if should be filtered out
func convertChatCompletionsStreamChunkToCompletions(chunkData []byte) []byte {
	root := gjson.ParseBytes(chunkData)

	// Check if this chunk has any meaningful content
	hasContent := false
	hasUsage := root.Get("usage").Exists()
	if chatChoices := root.Get("choices"); chatChoices.Exists() && chatChoices.IsArray() {
		chatChoices.ForEach(func(_, choice gjson.Result) bool {
			// Check if delta has content or finish_reason
			if delta := choice.Get("delta"); delta.Exists() {
				if content := delta.Get("content"); content.Exists() && content.String() != "" {
					hasContent = true
					return false // Break out of forEach
				}
			}
			// Also check for finish_reason to ensure we don't skip final chunks
			if finishReason := choice.Get("finish_reason"); finishReason.Exists() && finishReason.String() != "" && finishReason.String() != "null" {
				hasContent = true
				return false // Break out of forEach
			}
			return true
		})
	}

	// If no meaningful content and no usage, return nil to indicate this chunk should be skipped
	if !hasContent && !hasUsage {
		return nil
	}

	// Base completions stream response structure
	out := []byte(`{"id":"","object":"text_completion","created":0,"model":"","choices":[]}`)

	// Copy basic fields
	if id := root.Get("id"); id.Exists() {
		out, _ = sjson.SetBytes(out, "id", id.String())
	}

	if created := root.Get("created"); created.Exists() {
		out, _ = sjson.SetBytes(out, "created", created.Int())
	}

	if model := root.Get("model"); model.Exists() {
		out, _ = sjson.SetBytes(out, "model", model.String())
	}

	// Convert choices from chat completions delta to completions format
	var choices []interface{}
	if chatChoices := root.Get("choices"); chatChoices.Exists() && chatChoices.IsArray() {
		chatChoices.ForEach(func(_, choice gjson.Result) bool {
			completionsChoice := map[string]interface{}{
				"index": choice.Get("index").Int(),
			}

			// Extract text content from delta.content
			if delta := choice.Get("delta"); delta.Exists() {
				if content := delta.Get("content"); content.Exists() && content.String() != "" {
					completionsChoice["text"] = content.String()
				} else {
					completionsChoice["text"] = ""
				}
			} else {
				completionsChoice["text"] = ""
			}

			// Copy finish_reason
			if finishReason := choice.Get("finish_reason"); finishReason.Exists() && finishReason.String() != "null" {
				completionsChoice["finish_reason"] = finishReason.String()
			}

			// Copy logprobs if present
			if logprobs := choice.Get("logprobs"); logprobs.Exists() {
				completionsChoice["logprobs"] = logprobs.Value()
			}

			choices = append(choices, completionsChoice)
			return true
		})
	}

	if len(choices) > 0 {
		choicesJSON, _ := json.Marshal(choices)
		out, _ = sjson.SetRawBytes(out, "choices", choicesJSON)
	}

	// Copy usage if present
	if usage := root.Get("usage"); usage.Exists() {
		out, _ = sjson.SetRawBytes(out, "usage", []byte(usage.Raw))
	}

	return out
}

// handleNonStreamingResponse handles non-streaming chat completion responses
// for Gemini models. It selects a client from the pool, sends the request, and
// aggregates the response before sending it back to the client in OpenAI format.
//
// Parameters:
//   - c: The Gin context containing the HTTP request and response
//   - rawJSON: The raw JSON bytes of the OpenAI-compatible request
func (h *OpenAIAPIHandler) handleNonStreamingResponse(c *gin.Context, rawJSON []byte) {
	c.Header("Content-Type", "application/json")

	modelName := gjson.GetBytes(rawJSON, "model").String()
	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	stopKeepAlive := h.StartNonStreamingKeepAlive(c, cliCtx)
	resp, upstreamHeaders, errMsg := h.ExecuteWithAuthManager(cliCtx, h.HandlerType(), modelName, rawJSON, h.GetAlt(c))
	stopKeepAlive()
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		cliCancel(errMsg.Error)
		return
	}
	handlers.WriteUpstreamHeaders(c.Writer.Header(), upstreamHeaders)
	_, _ = c.Writer.Write(resp)
	cliCancel()
}

// handleStreamingResponse handles streaming responses for Gemini models.
// It establishes a streaming connection with the backend service and forwards
// the response chunks to the client in real-time using Server-Sent Events.
//
// Parameters:
//   - c: The Gin context containing the HTTP request and response
//   - rawJSON: The raw JSON bytes of the OpenAI-compatible request
func (h *OpenAIAPIHandler) handleStreamingResponse(c *gin.Context, rawJSON []byte) {
	// Get the http.Flusher interface to manually flush the response.
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, handlers.ErrorResponse{
			Error: handlers.ErrorDetail{
				Message: "Streaming not supported",
				Type:    "server_error",
			},
		})
		return
	}

	modelName := gjson.GetBytes(rawJSON, "model").String()
	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	setSSEHeaders := func() {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("Access-Control-Allow-Origin", "*")
	}
	var upstreamHeaders http.Header
	committed := false
	commitSSE := func() {
		if committed {
			return
		}
		setSSEHeaders()
		handlers.WriteUpstreamHeaders(c.Writer.Header(), upstreamHeaders)
		committed = true
	}
	writeKeepAlive := func() bool {
		commitSSE()
		_, _ = c.Writer.Write([]byte(": keep-alive\n\n"))
		flusher.Flush()
		return true
	}
	keepAliveC, stopKeepAlive := handlers.NewPreFirstTokenKeepAliveTicker(h.CurrentConfig())
	defer stopKeepAlive()

	dataChan, resolvedHeaders, errChan, okBootstrap := h.ExecuteStreamWithPreFTKeepAlive(
		c, cliCtx, func(err error) { cliCancel(err) }, h.HandlerType(), modelName, rawJSON, h.GetAlt(c), keepAliveC, writeKeepAlive,
	)
	if !okBootstrap {
		return
	}
	upstreamHeaders = resolvedHeaders

	// Peek at the first chunk to determine success or failure before setting headers
	for {
		select {
		case <-c.Request.Context().Done():
			cliCancel(c.Request.Context().Err())
			return
		case errMsg, ok := <-errChan:
			if !ok {
				// Err channel closed cleanly; wait for data channel.
				errChan = nil
				continue
			}
			if committed {
				stopKeepAlive()
				pendingErrors := make(chan *interfaces.ErrorMessage, 1)
				pendingErrors <- errMsg
				close(pendingErrors)
				h.handleStreamResult(c, flusher, func(err error) { cliCancel(err) }, make(chan []byte), pendingErrors, false)
				return
			}
			// Upstream failed immediately. Return proper error status and JSON.
			h.WriteErrorResponse(c, errMsg)
			if errMsg != nil {
				cliCancel(errMsg.Error)
			} else {
				cliCancel(nil)
			}
			return
		case chunk, ok := <-dataChan:
			if !ok {
				stopKeepAlive()
				if errMsg, hasPendingError := handlers.PendingStreamError(errChan); hasPendingError {
					if committed {
						pendingErrors := make(chan *interfaces.ErrorMessage, 1)
						pendingErrors <- errMsg
						close(pendingErrors)
						h.handleStreamResult(c, flusher, func(err error) { cliCancel(err) }, make(chan []byte), pendingErrors, false)
						return
					}
					h.WriteErrorResponse(c, errMsg)
					if errMsg != nil {
						cliCancel(errMsg.Error)
					} else {
						cliCancel(nil)
					}
					return
				}
				// Stream closed without data? Send DONE or just headers.
				commitSSE()
				_, _ = fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
				flusher.Flush()
				cliCancel(nil)
				return
			}

			// Success! Commit to streaming headers.
			stopKeepAlive()
			commitSSE()

			initialHasFinishReason := chunkHasFinishReason(chunk)
			_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", string(chunk))
			flusher.Flush()
			if termErr := chunkTerminalError(chunk); termErr != nil {
				cliCancel(termErr.Error)
				return
			}

			// Continue streaming the rest
			h.handleStreamResult(c, flusher, func(err error) { cliCancel(err) }, dataChan, errChan, initialHasFinishReason)
			return
		case <-keepAliveC:
			writeKeepAlive()
		}
	}
}

// handleCompletionsNonStreamingResponse handles non-streaming completions responses.
// It converts completions request to chat completions format, sends to backend,
// then converts the response back to completions format before sending to client.
//
// Parameters:
//   - c: The Gin context containing the HTTP request and response
//   - rawJSON: The raw JSON bytes of the OpenAI-compatible completions request
func (h *OpenAIAPIHandler) handleCompletionsNonStreamingResponse(c *gin.Context, rawJSON []byte) {
	c.Header("Content-Type", "application/json")

	// Convert completions request to chat completions format
	chatCompletionsJSON := convertCompletionsRequestToChatCompletions(rawJSON)

	modelName := gjson.GetBytes(chatCompletionsJSON, "model").String()
	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	stopKeepAlive := h.StartNonStreamingKeepAlive(c, cliCtx)
	resp, upstreamHeaders, errMsg := h.ExecuteWithAuthManager(cliCtx, h.HandlerType(), modelName, chatCompletionsJSON, "")
	stopKeepAlive()
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		cliCancel(errMsg.Error)
		return
	}
	handlers.WriteUpstreamHeaders(c.Writer.Header(), upstreamHeaders)
	completionsResp := convertChatCompletionsResponseToCompletions(resp)
	_, _ = c.Writer.Write(completionsResp)
	cliCancel()
}

// handleCompletionsStreamingResponse handles streaming completions responses.
// It converts completions request to chat completions format, streams from backend,
// then converts each response chunk back to completions format before sending to client.
//
// Parameters:
//   - c: The Gin context containing the HTTP request and response
//   - rawJSON: The raw JSON bytes of the OpenAI-compatible completions request
func (h *OpenAIAPIHandler) handleCompletionsStreamingResponse(c *gin.Context, rawJSON []byte) {
	// Get the http.Flusher interface to manually flush the response.
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, handlers.ErrorResponse{
			Error: handlers.ErrorDetail{
				Message: "Streaming not supported",
				Type:    "server_error",
			},
		})
		return
	}

	// Convert completions request to chat completions format
	chatCompletionsJSON := convertCompletionsRequestToChatCompletions(rawJSON)

	modelName := gjson.GetBytes(chatCompletionsJSON, "model").String()
	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	setSSEHeaders := func() {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("Access-Control-Allow-Origin", "*")
	}
	var upstreamHeaders http.Header
	committed := false
	commitSSE := func() {
		if committed {
			return
		}
		setSSEHeaders()
		handlers.WriteUpstreamHeaders(c.Writer.Header(), upstreamHeaders)
		committed = true
	}
	writeKeepAlive := func() bool {
		commitSSE()
		_, _ = c.Writer.Write([]byte(": keep-alive\n\n"))
		flusher.Flush()
		return true
	}
	keepAliveC, stopKeepAlive := handlers.NewPreFirstTokenKeepAliveTicker(h.CurrentConfig())
	defer stopKeepAlive()

	dataChan, resolvedHeaders, errChan, okBootstrap := h.ExecuteStreamWithPreFTKeepAlive(
		c, cliCtx, func(err error) { cliCancel(err) }, h.HandlerType(), modelName, chatCompletionsJSON, "", keepAliveC, writeKeepAlive,
	)
	if !okBootstrap {
		return
	}
	upstreamHeaders = resolvedHeaders

	// Peek at the first chunk
	for {
		select {
		case <-c.Request.Context().Done():
			cliCancel(c.Request.Context().Err())
			return
		case errMsg, ok := <-errChan:
			if !ok {
				// Err channel closed cleanly; wait for data channel.
				errChan = nil
				continue
			}
			if committed {
				stopKeepAlive()
				pendingErrors := make(chan *interfaces.ErrorMessage, 1)
				pendingErrors <- errMsg
				close(pendingErrors)
				h.handleStreamResult(c, flusher, func(err error) { cliCancel(err) }, make(chan []byte), pendingErrors, false)
				return
			}
			h.WriteErrorResponse(c, errMsg)
			if errMsg != nil {
				cliCancel(errMsg.Error)
			} else {
				cliCancel(nil)
			}
			return
		case chunk, ok := <-dataChan:
			if !ok {
				stopKeepAlive()
				if errMsg, hasPendingError := handlers.PendingStreamError(errChan); hasPendingError {
					if committed {
						pendingErrors := make(chan *interfaces.ErrorMessage, 1)
						pendingErrors <- errMsg
						close(pendingErrors)
						h.handleStreamResult(c, flusher, func(err error) { cliCancel(err) }, make(chan []byte), pendingErrors, false)
						return
					}
					h.WriteErrorResponse(c, errMsg)
					if errMsg != nil {
						cliCancel(errMsg.Error)
					} else {
						cliCancel(nil)
					}
					return
				}
				commitSSE()
				_, _ = fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
				flusher.Flush()
				cliCancel(nil)
				return
			}

			// Success! Set headers.
			stopKeepAlive()
			commitSSE()

			// Write the first chunk
			converted := convertChatCompletionsStreamChunkToCompletions(chunk)
			var initialHasFinishReason bool
			if converted != nil {
				initialHasFinishReason = chunkHasFinishReason(converted)
				_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", string(converted))
				flusher.Flush()
			}

			done := make(chan struct{})
			var doneOnce sync.Once
			stop := func() { doneOnce.Do(func() { close(done) }) }

			convertedChan := make(chan []byte)
			go func() {
				defer close(convertedChan)
				for {
					select {
					case <-done:
						return
					case chunk, ok := <-dataChan:
						if !ok {
							return
						}
						converted := convertChatCompletionsStreamChunkToCompletions(chunk)
						if converted == nil {
							continue
						}
						select {
						case <-done:
							return
						case convertedChan <- converted:
						}
					}
				}
			}()

			h.handleStreamResult(c, flusher, func(err error) {
				stop()
				cliCancel(err)
			}, convertedChan, errChan, initialHasFinishReason)
			return
		case <-keepAliveC:
			writeKeepAlive()
		}
	}
}
func chunkTerminalError(chunk []byte) *interfaces.ErrorMessage {
	chunk = bytes.TrimSpace(chunk)
	if bytes.HasPrefix(chunk, []byte("data:")) {
		chunk = bytes.TrimSpace(chunk[5:])
	}
	if gjson.GetBytes(chunk, "choices").Exists() || !gjson.GetBytes(chunk, "error").IsObject() {
		return nil
	}
	msg := gjson.GetBytes(chunk, "error.message").String()
	if msg == "" {
		msg = string(chunk)
	}
	return &interfaces.ErrorMessage{
		StatusCode: http.StatusBadRequest,
		Error:      fmt.Errorf("%s", msg),
	}
}

func chunkHasFinishReason(chunk []byte) bool {
	chunk = bytes.TrimSpace(chunk)
	if bytes.HasPrefix(chunk, []byte("data:")) {
		chunk = bytes.TrimSpace(chunk[5:])
	}
	if chunkTerminalError(chunk) != nil {
		return true
	}
	for _, choice := range gjson.GetBytes(chunk, "choices").Array() {
		if reason := choice.Get("finish_reason"); reason.Exists() && reason.Type != gjson.Null && reason.String() != "" {
			return true
		}
	}
	return false
}

func (h *OpenAIAPIHandler) handleStreamResult(c *gin.Context, flusher http.Flusher, cancel func(error), data <-chan []byte, errs <-chan *interfaces.ErrorMessage, initialHasFinishReason bool) {
	sawFinishReason := initialHasFinishReason
	var terminalChunkErr *interfaces.ErrorMessage
	h.ForwardStream(c, flusher, cancel, data, errs, handlers.StreamForwardOptions{
		WriteChunk: func(chunk []byte) {
			if termErr := chunkTerminalError(chunk); termErr != nil {
				terminalChunkErr = termErr
				sawFinishReason = true
			} else if !sawFinishReason && chunkHasFinishReason(chunk) {
				sawFinishReason = true
			}
			_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", string(chunk))
		},
		ChunkError: func() *interfaces.ErrorMessage {
			return terminalChunkErr
		},
		WriteTerminalError: func(errMsg *interfaces.ErrorMessage) {
			if errMsg == nil || terminalChunkErr != nil {
				return
			}
			status := http.StatusInternalServerError
			if errMsg.StatusCode > 0 {
				status = errMsg.StatusCode
			}
			errText := http.StatusText(status)
			if errMsg.Error != nil && errMsg.Error.Error() != "" {
				errText = errMsg.Error.Error()
			}
			body := handlers.BuildErrorResponseBody(status, errText)
			_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", string(body))
		},
		CloseError: func() *interfaces.ErrorMessage {
			if sawFinishReason {
				return nil
			}
			return &interfaces.ErrorMessage{
				StatusCode: http.StatusBadGateway,
				Error:      fmt.Errorf("upstream stream closed before any chunk carried finish_reason"),
			}
		},
		WriteDone: func() {
			_, _ = fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		},
	})
}
