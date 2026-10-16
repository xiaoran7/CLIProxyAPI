package handlers

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/thinking"
	"github.com/tidwall/sjson"
)

// ParsedModelMultiplier holds the result of extracting a multiplier suffix from a model name.
type ParsedModelMultiplier struct {
	CleanModel    string  // Clean model name (with thinking suffix if present, but without multiplier suffix)
	BaseModel     string  // Base model name (without multiplier suffix and without thinking suffix)
	Multiplier    float64 // Numeric multiplier value parsed from the request
	RawMultiplier string  // Raw multiplier string as passed by the client (e.g. "0.06x", "0.06", "0x")
	HasMultiplier bool    // True if a multiplier suffix was provided
}

// ParseModelMultiplier extracts any model multiplier suffix from a model name.
// Multiplier formats:
//   - "glm-5.3-flash:0.06x"        -> CleanModel: "glm-5.3-flash", Multiplier: 0.06, HasMultiplier: true
//   - "glm-5.3-flash:0.06"         -> CleanModel: "glm-5.3-flash", Multiplier: 0.06, HasMultiplier: true
//   - "hy3:0x"                     -> CleanModel: "hy3", Multiplier: 0.0, HasMultiplier: true
//   - "glm-5.3-flash(16384):0.06x" -> CleanModel: "glm-5.3-flash(16384)", Multiplier: 0.06, HasMultiplier: true
//   - "glm-5.3-flash:0.06x(16384)" -> CleanModel: "glm-5.3-flash(16384)", Multiplier: 0.06, HasMultiplier: true
func ParseModelMultiplier(model string) ParsedModelMultiplier {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return ParsedModelMultiplier{CleanModel: model}
	}

	// Case 1: Outer thinking suffix, e.g. "model:0.06x(16384)"
	ts := thinking.ParseSuffix(trimmed)
	if ts.HasSuffix {
		lastColon := strings.LastIndex(ts.ModelName, ":")
		if lastColon != -1 {
			cand := strings.TrimSpace(ts.ModelName[lastColon+1:])
			if mult, ok := registry.ParseWorkBuddyCreditsRaw(cand); ok {
				base := strings.TrimSpace(ts.ModelName[:lastColon])
				clean := fmt.Sprintf("%s(%s)", base, ts.RawSuffix)
				return ParsedModelMultiplier{
					CleanModel:    clean,
					BaseModel:     base,
					Multiplier:    mult,
					RawMultiplier: cand,
					HasMultiplier: true,
				}
			}
		}
	}

	// Case 2: Multiplier at the end, e.g. "model:0.06x" or "model(16384):0.06x"
	lastColon := strings.LastIndex(trimmed, ":")
	if lastColon != -1 {
		cand := strings.TrimSpace(trimmed[lastColon+1:])
		if mult, ok := registry.ParseWorkBuddyCreditsRaw(cand); ok {
			baseWithThinking := strings.TrimSpace(trimmed[:lastColon])
			baseOnly := strings.TrimSpace(thinking.ParseSuffix(baseWithThinking).ModelName)
			return ParsedModelMultiplier{
				CleanModel:    baseWithThinking,
				BaseModel:     baseOnly,
				Multiplier:    mult,
				RawMultiplier: cand,
				HasMultiplier: true,
			}
		}
	}

	baseOnly := strings.TrimSpace(thinking.ParseSuffix(trimmed).ModelName)
	return ParsedModelMultiplier{
		CleanModel: trimmed,
		BaseModel:  baseOnly,
	}
}

// ValidateModelMultiplier validates that the requested multiplier matches the catalog rate.
// If the multiplier does not match ("倍率不对"), it returns a 400 Bad Request error.
func ValidateModelMultiplier(parsed ParsedModelMultiplier) *interfaces.ErrorMessage {
	if !parsed.HasMultiplier {
		return nil
	}

	cleanBase := strings.TrimPrefix(parsed.BaseModel, "workbuddy/")
	expectedRate := registry.ResolveWorkBuddyModelCreditRate(cleanBase)
	if expectedRate == nil {
		body := `{"error":{"message":"","type":"invalid_request_error","code":"model_multiplier_mismatch","param":"model"}}`
		errMsg := fmt.Sprintf("model multiplier mismatch: model %s does not publish a credit multiplier", parsed.BaseModel)
		body, _ = sjson.Set(body, "error.message", errMsg)
		return &interfaces.ErrorMessage{
			StatusCode: http.StatusBadRequest,
			Error:      errors.New(body),
		}
	}

	if math.Abs(parsed.Multiplier-expectedRate.Multiplier) > 1e-4 {
		expectedDisplay := expectedRate.Effective
		if expectedDisplay == "" {
			if expectedRate.Multiplier < 1e-9 {
				expectedDisplay = "0x"
			} else {
				expectedDisplay = fmt.Sprintf("%.2fx", expectedRate.Multiplier)
			}
		}
		requestedDisplay := parsed.RawMultiplier
		if !strings.HasSuffix(strings.ToLower(requestedDisplay), "x") {
			requestedDisplay += "x"
		}
		errMsg := fmt.Sprintf("model multiplier mismatch: requested %s, expected %s for model %s (模型倍率不匹配)",
			requestedDisplay, expectedDisplay, parsed.BaseModel)
		body := `{"error":{"message":"","type":"invalid_request_error","code":"model_multiplier_mismatch","param":"model"}}`
		body, _ = sjson.Set(body, "error.message", errMsg)
		return &interfaces.ErrorMessage{
			StatusCode: http.StatusBadRequest,
			Error:      errors.New(body),
		}
	}

	return nil
}

// ExtractAndValidateModelMultiplier parses and validates any multiplier in modelName,
// returning the clean model name to route, or an error if the multiplier is wrong.
func ExtractAndValidateModelMultiplier(modelName string) (string, *interfaces.ErrorMessage) {
	parsed := ParseModelMultiplier(modelName)
	if err := ValidateModelMultiplier(parsed); err != nil {
		return "", err
	}
	return parsed.CleanModel, nil
}
