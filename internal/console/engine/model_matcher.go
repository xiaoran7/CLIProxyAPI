package engine

import (
	"encoding/json"
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
		if strings.Contains(pattern, "*") {
			escaped := regexp.QuoteMeta(pattern)
			regexStr := "^" + strings.ReplaceAll(escaped, "\\*", ".*") + "$"
			if re, err := regexp.Compile(regexStr); err == nil {
				for _, t := range targets {
					if re.MatchString(t) {
						return true
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
