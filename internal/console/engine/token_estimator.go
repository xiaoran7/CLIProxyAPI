package engine

import (
	"math"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

var (
	effortModelRegex = regexp.MustCompile(`[-_:](high|medium|low|max)$`)
)

// ExtractUsageFromJSON parses prompt, completion, cached, and total tokens from standard response JSON.
// Supports OpenAI, Anthropic, DeepSeek, and Google Gemini formats.
func ExtractUsageFromJSON(body []byte) (prompt, completion, cached, total int64, ok bool) {
	if len(body) == 0 {
		return 0, 0, 0, 0, false
	}

	res := gjson.ParseBytes(body)

	// 1. Standard "usage" object (OpenAI, Anthropic, DeepSeek)
	if u := res.Get("usage"); u.Exists() && u.IsObject() {
		hasPrompt := false
		if val := u.Get("prompt_tokens"); val.Exists() {
			prompt = val.Int()
			hasPrompt = true
		} else if val := u.Get("input_tokens"); val.Exists() {
			prompt = val.Int()
			hasPrompt = true
		} else if val := u.Get("promptTokenCount"); val.Exists() {
			prompt = val.Int()
			hasPrompt = true
		}

		if val := u.Get("completion_tokens"); val.Exists() {
			completion = val.Int()
		} else if val := u.Get("output_tokens"); val.Exists() {
			completion = val.Int()
		} else if val := u.Get("candidatesTokenCount"); val.Exists() {
			completion = val.Int()
		}

		if val := u.Get("total_tokens"); val.Exists() {
			total = val.Int()
		} else if val := u.Get("totalTokenCount"); val.Exists() {
			total = val.Int()
		}

		// Extract cached tokens across provider variations
		if val := u.Get("prompt_tokens_details.cached_tokens"); val.Exists() {
			cached = val.Int()
		} else if val := u.Get("prompt_tokens_details.cache_read_tokens"); val.Exists() {
			cached = val.Int()
		} else if val := u.Get("prompt_cache_hit_tokens"); val.Exists() {
			cached = val.Int()
		} else if val := u.Get("cache_read_input_tokens"); val.Exists() {
			cached = val.Int()
		} else if val := u.Get("cacheReadInputTokens"); val.Exists() {
			cached = val.Int()
		} else if val := u.Get("cache_read_tokens"); val.Exists() {
			cached = val.Int()
		} else if val := u.Get("cached_tokens"); val.Exists() {
			cached = val.Int()
		} else if val := u.Get("cachedTokens"); val.Exists() {
			cached = val.Int()
		}

		// Anthropic cache normalization: if input_tokens only reflects incremental uncached tokens,
		// add cached tokens to total prompt tokens.
		if u.Get("input_tokens").Exists() && !u.Get("prompt_tokens").Exists() && cached > 0 && prompt < cached {
			prompt += cached
		}

		if cached > 0 && prompt < cached {
			prompt = cached
		}
		if total <= 0 {
			total = prompt + completion
		}

		if hasPrompt || completion > 0 || cached > 0 {
			return prompt, completion, cached, total, true
		}
	}

	// 2. Google Gemini "usageMetadata" object
	if gm := res.Get("usageMetadata"); gm.Exists() && gm.IsObject() {
		prompt = gm.Get("promptTokenCount").Int()
		completion = gm.Get("candidatesTokenCount").Int()
		cached = gm.Get("cachedContentTokenCount").Int()
		total = gm.Get("totalTokenCount").Int()

		if cached > 0 && prompt < cached {
			prompt = cached
		}
		if total <= 0 {
			total = prompt + completion
		}
		if prompt > 0 || completion > 0 || cached > 0 {
			return prompt, completion, cached, total, true
		}
	}

	// 3. Flat / Root-level token fields (e.g. Anthropic count_tokens {"input_tokens": 1234})
	if res.Get("input_tokens").Exists() || res.Get("prompt_tokens").Exists() || res.Get("total_tokens").Exists() {
		hasPrompt := false
		if val := res.Get("prompt_tokens"); val.Exists() {
			prompt = val.Int()
			hasPrompt = true
		} else if val := res.Get("input_tokens"); val.Exists() {
			prompt = val.Int()
			hasPrompt = true
		}
		if val := res.Get("output_tokens"); val.Exists() {
			completion = val.Int()
		} else if val := res.Get("completion_tokens"); val.Exists() {
			completion = val.Int()
		}
		if val := res.Get("total_tokens"); val.Exists() {
			total = val.Int()
		} else {
			total = prompt + completion
		}
		if hasPrompt || completion > 0 || total > 0 {
			return prompt, completion, 0, total, true
		}
	}

	return 0, 0, 0, 0, false
}

// EstimateTokensFromChars computes an approximate token count assuming ~3 characters per token for CJK/multilingual.
func EstimateTokensFromChars(charCount int) int64 {
	if charCount <= 0 {
		return 0
	}
	return int64(math.Ceil(float64(charCount) / 3.0))
}

// ExtractReasoningEffort extracts thinking effort parameter across four protocol specifications:
// 1. OpenAI o-series: reasoning_effort, reasoning.effort, reasoningEffort
// 2. Anthropic: output_config.effort, effort, thinking.budget_tokens
// 3. Google Gemini: thinking_config.thinking_budget
// 4. Model name heuristic suffix: -high, -medium, -low, -max
func ExtractReasoningEffort(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	res := gjson.ParseBytes(body)

	// 1. OpenAI / Codex specification
	if val := res.Get("reasoning_effort"); val.Exists() && val.String() != "" {
		return strings.ToLower(strings.TrimSpace(val.String()))
	}
	if val := res.Get("reasoning.effort"); val.Exists() && val.String() != "" {
		return strings.ToLower(strings.TrimSpace(val.String()))
	}
	if val := res.Get("reasoningEffort"); val.Exists() && val.String() != "" {
		return strings.ToLower(strings.TrimSpace(val.String()))
	}

	// 2. Anthropic effort
	if val := res.Get("output_config.effort"); val.Exists() && val.String() != "" {
		return strings.ToLower(strings.TrimSpace(val.String()))
	}
	if val := res.Get("outputConfig.effort"); val.Exists() && val.String() != "" {
		return strings.ToLower(strings.TrimSpace(val.String()))
	}
	if val := res.Get("effort"); val.Exists() && val.String() != "" {
		return strings.ToLower(strings.TrimSpace(val.String()))
	}

	// 3. Anthropic thinking budget or type
	if val := res.Get("thinking.budget_tokens"); val.Exists() && val.Int() > 0 {
		return val.String()
	}
	if val := res.Get("thinking.type"); val.Exists() && val.String() != "" {
		return strings.ToLower(strings.TrimSpace(val.String()))
	}

	// 4. Gemini thinking budget
	if val := res.Get("thinking_config.thinking_budget"); val.Exists() && val.Int() > 0 {
		return val.String()
	}

	// 5. Model name heuristic suffix
	if modelVal := res.Get("model"); modelVal.Exists() && modelVal.String() != "" {
		m := strings.ToLower(strings.TrimSpace(modelVal.String()))
		match := effortModelRegex.FindStringSubmatch(m)
		if len(match) > 1 {
			return match[1]
		}
	}

	return ""
}

// CalculateTokensPerSec calculates generation speed in tokens per second.
// For streaming responses with valid TTFT, TTFT is subtracted from duration to isolate generation phase.
func CalculateTokensPerSec(completionTokens, latencyMs, ttftMs int64, isStream bool) float64 {
	if completionTokens <= 0 || latencyMs <= 0 {
		return 0
	}

	durationMs := latencyMs
	if isStream && ttftMs > 0 && latencyMs > ttftMs && (latencyMs-ttftMs) >= 100 {
		durationMs = latencyMs - ttftMs
	}

	sec := float64(durationMs) / 1000.0
	if sec <= 0 {
		return 0
	}

	tps := float64(completionTokens) / sec
	return math.Round(tps*10.0) / 10.0
}
