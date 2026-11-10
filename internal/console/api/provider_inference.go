// 提供商注册中心上游推导与标签解析 —— 拆自 provider_registry.go
package api

import (
	"fmt"
	"strings"
)

// InferProvider identifies the AI provider from model name and/or auth_index.
func (pe *ProviderRegistryEngine) InferProvider(model, authIndex string) ProviderInferenceResult {
	modelStr := strings.ToLower(strings.TrimSpace(model))
	authStr := strings.ToLower(strings.TrimSpace(authIndex))

	pe.mu.RLock()
	defer pe.mu.RUnlock()

	// 0. Check authoritative registered upstream accounts first (highest precedence)
	if authStr != "" {
		if acc, found := pe.authAccounts[authStr]; found && (acc.ProviderKey != "" || acc.Name != "") {
			key := acc.ProviderKey
			if key == "" || key == "openai-compatibility" {
				if acc.Name != "" {
					key = fmt.Sprintf("compat:%s", strings.ToLower(strings.ReplaceAll(acc.Name, " ", "_")))
				} else {
					key = UnknownProviderKey
				}
			}
			label := pe.getProviderLabelLocked(key)
			if label == "" {
				if chLabel, chOk := pe.channelNames[key]; chOk && chLabel != "" {
					label = chLabel
				} else if acc.Name != "" {
					label = acc.Name
				} else {
					label = key
				}
			}
			return ProviderInferenceResult{
				Key:       key,
				Label:     label,
				MatchedBy: "auth",
				Source:    "account",
			}
		}
	}

	// 1. Evaluate auth_patterns next, ordered by sort_order
	if authStr != "" {
		for _, cp := range pe.providers {
			for _, pat := range cp.AuthPatterns {
				if pat.Regex.MatchString(authStr) {
					return ProviderInferenceResult{
						Key:       cp.ProviderKey,
						Label:     cp.Label,
						MatchedBy: "auth",
						Source:    cp.Source,
					}
				}
			}
		}
	}

	// 2. Evaluate model_patterns, ordered by sort_order
	if modelStr != "" {
		// 2a. Check explicit namespace prefix first (e.g. "workbuddy/claude-opus-5-5" -> "workbuddy")
		if slashIdx := strings.Index(modelStr, "/"); slashIdx > 0 {
			candPrefix := modelStr[:slashIdx]
			for _, cp := range pe.providers {
				if strings.EqualFold(cp.ProviderKey, candPrefix) {
					return ProviderInferenceResult{
						Key:       cp.ProviderKey,
						Label:     cp.Label,
						MatchedBy: "model",
						Source:    cp.Source,
					}
				}
				for _, pat := range cp.ModelPatterns {
					if strings.EqualFold(pat.P, candPrefix) || strings.EqualFold(pat.P, candPrefix+"/") || (pat.Anchor == "^" && pat.Regex.MatchString(candPrefix+"/")) {
						return ProviderInferenceResult{
							Key:       cp.ProviderKey,
							Label:     cp.Label,
							MatchedBy: "model",
							Source:    cp.Source,
						}
					}
				}
			}
		}

		for _, cp := range pe.providers {
			for _, pat := range cp.ModelPatterns {
				if pat.Regex.MatchString(modelStr) {
					return ProviderInferenceResult{
						Key:       cp.ProviderKey,
						Label:     cp.Label,
						MatchedBy: "model",
						Source:    cp.Source,
					}
				}
			}
		}
	}

	// 3. Fallback to channel prefix if authIndex is present but unmapped
	if authStr != "" {
		prefix := authStr
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
		channelKey := fmt.Sprintf("upstream:%s", prefix)
		label := pe.channelNames[channelKey]
		if label == "" {
			label = fmt.Sprintf("Upstream %s", prefix)
		}
		return ProviderInferenceResult{
			Key:       channelKey,
			Label:     label,
			MatchedBy: "auth",
			Source:    "channel",
		}
	}

	return ProviderInferenceResult{
		Key:       UnknownProviderKey,
		Label:     UnknownProviderLabel,
		MatchedBy: "none",
		Source:    "",
	}
}

// InferModelProviderLabel returns display label inferred purely from model name.
func (pe *ProviderRegistryEngine) InferModelProviderLabel(model string) string {
	res := pe.InferProvider(model, "")
	if res.Label != "" && res.Key != UnknownProviderKey {
		return res.Label
	}
	return OtherModelLabel
}

// GetProviderLabel returns provider label by provider key.
func (pe *ProviderRegistryEngine) GetProviderLabel(key string) string {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return pe.getProviderLabelLocked(key)
}

// GetProviderGroupLabel returns display name for dashboard grouping.
func (pe *ProviderRegistryEngine) GetProviderGroupLabel(key, fallbackName string) string {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" || k == "unknown" {
		return "未知上游"
	}
	if k == "other" {
		return "其他上游"
	}
	label := pe.GetProviderLabel(k)
	if label != "" {
		return label
	}
	if chLabel := pe.GetChannelLabel(k); chLabel != "" {
		return chLabel
	}
	fb := strings.TrimSpace(fallbackName)
	if fb != "" && fb != UnknownProviderLabel && fb != "未知上游" {
		return fb
	}
	if strings.HasPrefix(k, "compat:") {
		name := strings.TrimPrefix(k, "compat:")
		if len(name) > 0 {
			return strings.ToUpper(name[:1]) + name[1:]
		}
	}
	if strings.HasPrefix(k, "upstream:") {
		prefix := strings.TrimPrefix(k, "upstream:")
		return fmt.Sprintf("上游渠道 %s", prefix)
	}
	return UnknownProviderLabel
}
