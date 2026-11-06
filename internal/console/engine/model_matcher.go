package engine

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	multiplierRegex          = regexp.MustCompile(`:\d+(?:\.\d+)?x$`)
	dateOrVersionSuffixRegex = regexp.MustCompile(`^-(20\d{6}|20\d{2}-\d{2}-\d{2}|latest)$|^@`)
)

// isManufacturerPrefix identifies public vendor model namespace aliases (e.g. anthropic/claude-*, openai/gpt-*, google/gemini-*)
// commonly emitted by client SDKs (LiteLLM, LangChain) that do not represent isolated gateway routing channels.
func isManufacturerPrefix(prefix string) bool {
	switch strings.ToLower(strings.TrimSpace(prefix)) {
	case "anthropic", "openai", "google", "meta", "deepseek", "mistral", "models":
		return true
	default:
		return false
	}
}

// isChannelNamespace reports whether prefix represents an upstream proxy routing channel/realm
// (e.g. workbuddy-cn, workbuddy, qoder, zcode, minimax, stepfun, etc.) rather than a generic vendor alias.
func isChannelNamespace(prefix string) bool {
	p := strings.ToLower(strings.TrimSpace(prefix))
	return p != "" && !isManufacturerPrefix(p)
}

// NormalizeModelWhitelist parses and formats raw whitelist input into a canonical JSON array string.
func NormalizeModelWhitelist(raw string) string {
	parsed := ParseModelWhitelist(raw)
	if len(parsed) == 0 {
		return ""
	}
	bytes, err := json.Marshal(parsed)
	if err != nil {
		return ""
	}
	return string(bytes)
}

// ParseModelWhitelist converts a raw whitelist (JSON array, comma-separated, or whitespace-separated)
// into a slice of lowercase model names. Returns nil if wildcard '*' is present (meaning all models allowed).
func ParseModelWhitelist(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "*" || trimmed == `["*"]` {
		return nil
	}

	var items []string
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		var arr []string
		if err := json.Unmarshal([]byte(trimmed), &arr); err == nil {
			items = arr
		} else {
			inner := strings.Trim(trimmed, "[]")
			items = strings.Split(inner, ",")
		}
	} else {
		items = strings.Split(trimmed, ",")
	}

	var result []string
	seen := make(map[string]struct{})

	for _, item := range items {
		parts := strings.FieldsFunc(item, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
		})
		for _, p := range parts {
			m := strings.ToLower(strings.TrimSpace(p))
			if m == "*" {
				return nil // Wildcard allows all
			}
			if m != "" {
				if _, ok := seen[m]; !ok {
					seen[m] = struct{}{}
					result = append(result, m)
				}
			}
		}
	}

	return result
}

// IsModelPermitted determines if requestedModel satisfies the allowedModels whitelist.
// Features supported:
// 1. Exact case-insensitive match
// 2. Namespace / provider prefix stripping (e.g., anthropic/claude-3-7-sonnet -> claude-3-7-sonnet)
// 3. Dynamic multiplier suffix stripping (e.g., :3.33x, :1x)
// 4. Wildcard matching (e.g., gpt-4o*, claude-3*, *)
// 5. Version and date suffix matching (e.g., claude-3-7-sonnet matches claude-3-7-sonnet-20250219, claude-3-7-sonnet@20250219)
func IsModelPermitted(requestedModel string, allowedModels []string) bool {
	if len(allowedModels) == 0 {
		return true // Empty whitelist permits all
	}
	reqLower := strings.ToLower(strings.TrimSpace(requestedModel))
	if reqLower == "" {
		return false
	}

	reqSlashIdx := strings.Index(reqLower, "/")
	var reqPrefix, reqModel string
	if reqSlashIdx != -1 {
		reqPrefix = reqLower[:reqSlashIdx]
		reqModel = reqLower[reqSlashIdx+1:]
	} else {
		reqModel = reqLower
	}
	reqModelUnmultiplied := multiplierRegex.ReplaceAllString(reqModel, "")
	reqUnmultiplied := multiplierRegex.ReplaceAllString(reqLower, "")

	for _, pattern := range allowedModels {
		patLower := strings.ToLower(strings.TrimSpace(pattern))
		if patLower == "*" || patLower == reqLower || patLower == reqUnmultiplied {
			return true
		}

		patSlashIdx := strings.Index(patLower, "/")
		var patPrefix, patModel string
		if patSlashIdx != -1 {
			patPrefix = patLower[:patSlashIdx]
			patModel = patLower[patSlashIdx+1:]
		} else {
			patModel = patLower
		}
		patModelUnmultiplied := multiplierRegex.ReplaceAllString(patModel, "")
		patUnmultiplied := multiplierRegex.ReplaceAllString(patLower, "")

		// Provider namespace isolation:
		// 1. If both pattern and request specify provider prefixes, they must match.
		if patPrefix != "" && reqPrefix != "" {
			if patPrefix != reqPrefix {
				if isChannelNamespace(patPrefix) || isChannelNamespace(reqPrefix) {
					continue
				}
			}
		}

		// 2. If request explicitly targets a channel namespace (e.g. workbuddy-cn/...),
		// a bare pattern without that provider prefix CANNOT match it (unless pattern was '*').
		if isChannelNamespace(reqPrefix) && patPrefix != reqPrefix {
			continue
		}

		// 3. If pattern explicitly targets a channel namespace (e.g. workbuddy-cn/...),
		// a request specifying a different provider prefix CANNOT match it.
		if isChannelNamespace(patPrefix) && reqPrefix != "" && reqPrefix != patPrefix {
			continue
		}

		// Model base candidates and targets
		candidates := []string{patModel, patModelUnmultiplied}
		targets := []string{reqModel, reqModelUnmultiplied}

		// Also permit full string matching if neither side is an isolated channel
		if !isChannelNamespace(patPrefix) && !isChannelNamespace(reqPrefix) {
			if patLower != patModel {
				candidates = append(candidates, patLower, patUnmultiplied)
			}
			if reqLower != reqModel {
				targets = append(targets, reqLower, reqUnmultiplied)
			}
		}

		matchFound := false
		for _, c := range candidates {
			for _, t := range targets {
				if c == t {
					matchFound = true
					break
				}
			}
			if matchFound {
				break
			}
		}
		if matchFound {
			return true
		}

		// Wildcard matching (e.g. gpt-4o*, workbuddy-cn/*)
		for _, pw := range candidates {
			if strings.Contains(pw, "*") {
				escaped := regexp.QuoteMeta(pw)
				regexStr := "^" + strings.ReplaceAll(escaped, "\\*", ".*") + "$"
				if re, err := regexp.Compile(regexStr); err == nil {
					for _, t := range targets {
						if re.MatchString(t) {
							return true
						}
					}
				}
			}
		}

		// Authentic date or version suffix matching (e.g. claude-3-7-sonnet matches claude-3-7-sonnet-20250219 or @version)
		for _, base := range []string{patModel, patModelUnmultiplied} {
			if base == "" {
				continue
			}
			for _, t := range targets {
				if strings.HasPrefix(t, base) {
					suffix := t[len(base):]
					if dateOrVersionSuffixRegex.MatchString(suffix) {
						return true
					}
				}
			}
		}
	}

	return false
}

// ResolveModelTarget checks if requestedModel satisfies the whitelist, and if an unprefixed
// model was requested against a whitelist with a uniquely matching provider prefix (e.g. "workbuddy/claude-opus-5-5"
// or "workbuddy/*"), resolves targetModel to the namespaced model so requests route strictly to the intended provider.
func ResolveModelTarget(requestedModel string, allowedModels []string) (allowed bool, targetModel string, reason string) {
	if len(allowedModels) == 0 {
		return true, requestedModel, ""
	}
	reqTrimmed := strings.TrimSpace(requestedModel)
	if reqTrimmed == "" {
		return true, "", ""
	}

	if !IsModelPermitted(reqTrimmed, allowedModels) {
		return false, reqTrimmed, fmt.Sprintf("Model '%s' is not permitted for this API key. Permitted models: %s", reqTrimmed, strings.Join(allowedModels, ", "))
	}

	reqLower := strings.ToLower(reqTrimmed)

	// If the request already has a provider prefix, or universal wildcard '*' allows all models,
	// no rewriting is needed.
	if strings.Contains(reqLower, "/") {
		return true, reqTrimmed, ""
	}
	for _, p := range allowedModels {
		if p == "*" {
			return true, reqTrimmed, ""
		}
	}

	// Check if the exact bare model is explicitly permitted without any provider prefix
	for _, p := range allowedModels {
		pLower := strings.ToLower(strings.TrimSpace(p))
		if !strings.Contains(pLower, "/") {
			// Bare pattern like "claude-opus-5-5", "claude*", etc.
			if IsModelPermitted(reqTrimmed, []string{p}) {
				return true, reqTrimmed, ""
			}
		}
	}

	// The bare model was permitted only via prefixed pattern(s).
	// Find all matching provider prefixes from the whitelist.
	matchedPrefixes := make(map[string]string)
	for _, p := range allowedModels {
		pLower := strings.ToLower(strings.TrimSpace(p))
		slashIdx := strings.Index(pLower, "/")
		if slashIdx == -1 {
			continue
		}
		prefix := pLower[:slashIdx]
		if IsModelPermitted(reqTrimmed, []string{p}) {
			matchedPrefixes[prefix] = prefix + "/" + reqTrimmed
		}
	}

	// If exactly one unique provider prefix matched, rewrite to that namespace
	if len(matchedPrefixes) == 1 {
		for _, rewritten := range matchedPrefixes {
			return true, rewritten, ""
		}
	}

	return true, reqTrimmed, ""
}
