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

// ParsedModelMultiplier holds the result of extracting a provider prefix and multiplier suffix from a model name.
type ParsedModelMultiplier struct {
	CleanModel     string  // Clean model name (with thinking suffix if present, but stripped of provider prefix and multiplier suffix)
	BaseModel      string  // Base model name (without provider prefix, multiplier suffix, and thinking suffix)
	ProviderPrefix string  // Provider prefix as requested (e.g. "workbuddy", "workbuddy-cn", "workbuddy-intl")
	TargetProvider string  // Mapped standard provider identifier (e.g. "workbuddy")
	TargetRealm    string  // Mapped realm ("intl" or "cn") if specified by prefix
	Multiplier     float64 // Numeric multiplier value parsed from the request
	RawMultiplier  string  // Raw multiplier string as passed by the client (e.g. "0.06x", "0.06", "0x")
	HasMultiplier  bool    // True if a multiplier suffix was provided
}

func splitProviderPrefix(model string) (prefix, targetProvider, targetRealm, remainder string) {
	trimmed := strings.TrimSpace(model)
	slashIdx := strings.Index(trimmed, "/")
	if slashIdx == -1 {
		return "", "", "", trimmed
	}
	cand := strings.ToLower(strings.TrimSpace(trimmed[:slashIdx]))
	rem := strings.TrimSpace(trimmed[slashIdx+1:])
	switch cand {
	case "workbuddy", "workbuddy-intl", "wb":
		return cand, "workbuddy", "intl", rem
	case "workbuddy-cn", "codebuddy", "wb-cn":
		return cand, "workbuddy", "cn", rem
	case "opencode", "opencode-zen", "zen":
		return cand, "opencode", "", rem
	case "antigravity", "ag":
		return cand, "antigravity", "", rem
	default:
		return "", "", "", trimmed
	}
}

// ParseModelMultiplier extracts any provider prefix and model multiplier suffix from a model name.
// Supported formats:
//   - "workbuddy/claude-opus-5-5:3.33x"        -> CleanModel: "claude-opus-5-5", TargetProvider: "workbuddy", Multiplier: 3.33
//   - "workbuddy-cn/hy3:0x"                    -> CleanModel: "hy3", TargetProvider: "workbuddy", TargetRealm: "cn", Multiplier: 0.0
//   - "workbuddy/claude-opus-5-5"              -> CleanModel: "claude-opus-5-5", TargetProvider: "workbuddy"
//   - "glm-5.3-flash:0.06x"                    -> CleanModel: "glm-5.3-flash", Multiplier: 0.06, HasMultiplier: true
//   - "glm-5.3-flash(16384):0.06x"             -> CleanModel: "glm-5.3-flash(16384)", Multiplier: 0.06, HasMultiplier: true
//   - "workbuddy/claude-opus-5-5:3.33x(16384)" -> CleanModel: "claude-opus-5-5(16384)", Multiplier: 3.33, HasMultiplier: true
func ParseModelMultiplier(model string) ParsedModelMultiplier {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return ParsedModelMultiplier{CleanModel: model}
	}

	prefix, targetProvider, targetRealm, remainder := splitProviderPrefix(trimmed)

	// Case 1: Outer thinking suffix, e.g. "model:0.06x(16384)"
	ts := thinking.ParseSuffix(remainder)
	if ts.HasSuffix {
		lastColon := strings.LastIndex(ts.ModelName, ":")
		if lastColon != -1 {
			cand := strings.TrimSpace(ts.ModelName[lastColon+1:])
			if mult, ok := registry.ParseWorkBuddyCreditsRaw(cand); ok {
				base := strings.TrimSpace(ts.ModelName[:lastColon])
				clean := fmt.Sprintf("%s(%s)", base, ts.RawSuffix)
				return ParsedModelMultiplier{
					CleanModel:     clean,
					BaseModel:      base,
					ProviderPrefix: prefix,
					TargetProvider: targetProvider,
					TargetRealm:    targetRealm,
					Multiplier:     mult,
					RawMultiplier:  cand,
					HasMultiplier:  true,
				}
			}
		}
	}

	// Case 2: Multiplier at the end, e.g. "model:0.06x" or "model(16384):0.06x"
	lastColon := strings.LastIndex(remainder, ":")
	if lastColon != -1 {
		cand := strings.TrimSpace(remainder[lastColon+1:])
		if mult, ok := registry.ParseWorkBuddyCreditsRaw(cand); ok {
			baseWithThinking := strings.TrimSpace(remainder[:lastColon])
			baseOnly := strings.TrimSpace(thinking.ParseSuffix(baseWithThinking).ModelName)
			return ParsedModelMultiplier{
				CleanModel:     baseWithThinking,
				BaseModel:      baseOnly,
				ProviderPrefix: prefix,
				TargetProvider: targetProvider,
				TargetRealm:    targetRealm,
				Multiplier:     mult,
				RawMultiplier:  cand,
				HasMultiplier:  true,
			}
		}
	}

	baseOnly := strings.TrimSpace(thinking.ParseSuffix(remainder).ModelName)
	return ParsedModelMultiplier{
		CleanModel:     remainder,
		BaseModel:      baseOnly,
		ProviderPrefix: prefix,
		TargetProvider: targetProvider,
		TargetRealm:    targetRealm,
	}
}

// ValidateModelMultiplier validates that the requested multiplier matches the catalog rate.
// If the multiplier does not match the catalog rate, it returns a 400 Bad Request error.
func ValidateModelMultiplier(parsed ParsedModelMultiplier) *interfaces.ErrorMessage {
	if !parsed.HasMultiplier {
		return nil
	}

	cleanBase := parsed.BaseModel
	var expectedRate *registry.WorkBuddyCreditsRate
	if parsed.TargetRealm != "" {
		expectedRate = registry.LookupWorkBuddyModelCredits(cleanBase, parsed.TargetRealm)
	}
	if expectedRate == nil {
		expectedRate = registry.ResolveWorkBuddyModelCreditRate(cleanBase)
	}
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
		errMsg := fmt.Sprintf("model multiplier mismatch: requested %s, expected %s for model %s",
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

// ExtractAndValidateModelMultiplierDetails parses and validates any prefix and multiplier in modelName,
// returning the parsed details or an error if the multiplier is mismatched.
func ExtractAndValidateModelMultiplierDetails(modelName string) (ParsedModelMultiplier, *interfaces.ErrorMessage) {
	parsed := ParseModelMultiplier(modelName)
	if err := ValidateModelMultiplier(parsed); err != nil {
		return parsed, err
	}
	return parsed, nil
}

// ExtractAndValidateModelMultiplier parses and validates any multiplier in modelName,
// returning the clean model name to route, or an error if the multiplier is wrong.
func ExtractAndValidateModelMultiplier(modelName string) (string, *interfaces.ErrorMessage) {
	parsed, err := ExtractAndValidateModelMultiplierDetails(modelName)
	if err != nil {
		return "", err
	}
	return parsed.CleanModel, nil
}
