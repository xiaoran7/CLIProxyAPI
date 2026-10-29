package engine

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	multiplierRegex = regexp.MustCompile(`:\d+(?:\.\d+)?x$`)
)

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

	// 1. Check exact match or universal wildcard
	for _, pattern := range allowedModels {
		if pattern == "*" || pattern == reqLower {
			return true
		}
	}

	// 2. Strip namespace / provider prefix
	slashIdx := strings.LastIndex(reqLower, "/")
	reqStripped := reqLower
	if slashIdx != -1 {
		reqStripped = reqLower[slashIdx+1:]
	}

	// 3. Strip dynamic multiplier suffix
	reqUnmultiplied := multiplierRegex.ReplaceAllString(reqLower, "")
	reqStrippedUnmultiplied := multiplierRegex.ReplaceAllString(reqStripped, "")

	for _, pattern := range allowedModels {
		if pattern == reqStripped || pattern == reqUnmultiplied || pattern == reqStrippedUnmultiplied {
			return true
		}

		// Normalize pattern as well
		patSlashIdx := strings.LastIndex(pattern, "/")
		patStripped := pattern
		if patSlashIdx != -1 {
			patStripped = pattern[patSlashIdx+1:]
		}
		patUnmultiplied := multiplierRegex.ReplaceAllString(pattern, "")
		patStrippedUnmultiplied := multiplierRegex.ReplaceAllString(patStripped, "")

		candidates := []string{patStripped, patUnmultiplied, patStrippedUnmultiplied}
		targets := []string{reqLower, reqStripped, reqUnmultiplied, reqStrippedUnmultiplied}

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

		// 4. Wildcard matching (glob to regex)
		patWildcards := []string{pattern, patStripped, patUnmultiplied, patStrippedUnmultiplied}
		for _, pw := range patWildcards {
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

		// 5. Date / version suffix matching (- or @)
		prefixes := []string{
			pattern + "-", pattern + "@",
			patStripped + "-", patStripped + "@",
			patUnmultiplied + "-", patUnmultiplied + "@",
			patStrippedUnmultiplied + "-", patStrippedUnmultiplied + "@",
		}
		for _, prefix := range prefixes {
			for _, t := range targets {
				if strings.HasPrefix(t, prefix) {
					return true
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
