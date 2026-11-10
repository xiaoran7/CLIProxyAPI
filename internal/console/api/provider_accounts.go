// 提供商注册中心账号与渠道映射管理 —— 拆自 provider_registry.go
package api

import (
	"sort"
	"strings"
)

// RegisterAuthAccount registers or updates an authoritative upstream account mapping.
func (pe *ProviderRegistryEngine) RegisterAuthAccount(authIndex, providerKey, name string) {
	idx := strings.ToLower(strings.TrimSpace(authIndex))
	if idx == "" {
		return
	}
	prov := strings.ToLower(strings.TrimSpace(providerKey))
	nm := strings.TrimSpace(name)

	pe.mu.Lock()
	defer pe.mu.Unlock()

	existing, exists := pe.authAccounts[idx]
	if exists {
		if prov == "" {
			prov = existing.ProviderKey
		}
		if nm == "" {
			nm = existing.Name
		}
	}
	pe.authAccounts[idx] = AuthAccountInfo{
		AuthIndex:   idx,
		ProviderKey: prov,
		Name:        nm,
	}
}

// GetAuthAccount retrieves account metadata for a specific auth_index.
func (pe *ProviderRegistryEngine) GetAuthAccount(authIndex string) (AuthAccountInfo, bool) {
	idx := strings.ToLower(strings.TrimSpace(authIndex))
	if idx == "" {
		return AuthAccountInfo{}, false
	}
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	info, ok := pe.authAccounts[idx]
	return info, ok
}

// GetProviderAuthIndexes returns all registered auth_index hashes mapped to a provider.
func (pe *ProviderRegistryEngine) GetProviderAuthIndexes(providerKey string) []string {
	targetProv := strings.ToLower(strings.TrimSpace(providerKey))
	if targetProv == "" || targetProv == "all" {
		return nil
	}
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	var res []string
	for idx, acc := range pe.authAccounts {
		if strings.EqualFold(acc.ProviderKey, targetProv) {
			res = append(res, idx)
		}
	}
	sort.Strings(res)
	return res
}

// GetExcludeAuthIndexes returns all registered auth_index hashes belonging to other known providers.
func (pe *ProviderRegistryEngine) GetExcludeAuthIndexes(providerKey string) []string {
	targetProv := strings.ToLower(strings.TrimSpace(providerKey))
	if targetProv == "" || targetProv == "all" {
		return nil
	}
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	var res []string
	for idx, acc := range pe.authAccounts {
		if acc.ProviderKey != "" && !strings.EqualFold(acc.ProviderKey, targetProv) {
			res = append(res, idx)
		}
	}
	sort.Strings(res)
	return res
}

// RegisterChannel registers or updates a custom channel display name mapping.
func (pe *ProviderRegistryEngine) RegisterChannel(channelKey, label string) {
	k := strings.ToLower(strings.TrimSpace(channelKey))
	lbl := strings.TrimSpace(label)
	if k == "" || lbl == "" {
		return
	}
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.channelNames[k] = lbl
}

// GetChannelLabel returns the registered channel display label or empty string.
func (pe *ProviderRegistryEngine) GetChannelLabel(channelKey string) string {
	k := strings.ToLower(strings.TrimSpace(channelKey))
	if k == "" {
		return ""
	}
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return pe.channelNames[k]
}

func (pe *ProviderRegistryEngine) getProviderLabelLocked(key string) string {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return ""
	}
	for _, cp := range pe.providers {
		if cp.ProviderKey == k {
			return cp.Label
		}
	}
	return ""
}
