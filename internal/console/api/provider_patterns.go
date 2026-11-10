// 提供商规则校验与模式查询 —— 拆自 provider_registry.go
package api

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidateProviderKey checks if a provider key format is valid.
func ValidateProviderKey(key string) error {
	trimmed := strings.ToLower(strings.TrimSpace(key))
	if trimmed == "" {
		return fmt.Errorf("provider_key cannot be empty")
	}
	matched, _ := regexp.MatchString(`^[a-z0-9][a-z0-9_-]*$`, trimmed)
	if !matched {
		return fmt.Errorf("provider_key must consist of lowercase letters, digits, underscores, or hyphens")
	}
	return nil
}

// GetProviderPatterns returns raw model and auth string patterns for a given provider key.
func (pe *ProviderRegistryEngine) GetProviderPatterns(key string) (modelPatterns, authPatterns []string) {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return nil, nil
	}

	pe.mu.RLock()
	defer pe.mu.RUnlock()

	for _, cp := range pe.providers {
		if cp.ProviderKey == k {
			for _, p := range cp.ModelPatterns {
				if p.P != "" {
					modelPatterns = append(modelPatterns, p.P)
				}
			}
			for _, p := range cp.AuthPatterns {
				if p.P != "" {
					authPatterns = append(authPatterns, p.P)
				}
			}
			return modelPatterns, authPatterns
		}
	}
	return nil, nil
}
