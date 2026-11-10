// 提供商注册中心引擎核心结构、生命周期与模式查询 —— 拆自 provider_registry.go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
)

// CompiledPattern holds a compiled regex for pattern evaluation.
type CompiledPattern struct {
	P      string
	Anchor string
	Regex  *regexp.Regexp
}

// CompiledProvider holds a compiled provider entry.
type CompiledProvider struct {
	ProviderKey   string
	Label         string
	Source        string
	SortOrder     int
	ModelPatterns []CompiledPattern
	AuthPatterns  []CompiledPattern
}

// ProviderInferenceResult represents the outcome of inferring a provider.
type ProviderInferenceResult struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	MatchedBy string `json:"matched_by"` // "auth", "model", or "none"
	Source    string `json:"source"`
}

// AuthAccountInfo stores resolved upstream credential metadata.
type AuthAccountInfo struct {
	AuthIndex   string `json:"auth_index"`
	ProviderKey string `json:"provider_key"`
	Name        string `json:"name"`
}

// ProviderRegistryEngine coordinates in-memory provider matching.
type ProviderRegistryEngine struct {
	mu           sync.RWMutex
	providers    []*CompiledProvider
	db           *db.DB
	authAccounts map[string]AuthAccountInfo // key is lowercase authIndex
	channelNames map[string]string          // key is lowercase channelKey or provKey -> display label
}

// NewProviderRegistryEngine initializes and populates the registry engine.
func NewProviderRegistryEngine(database *db.DB) (*ProviderRegistryEngine, error) {
	pe := &ProviderRegistryEngine{
		db:           database,
		authAccounts: make(map[string]AuthAccountInfo),
		channelNames: make(map[string]string),
	}
	if err := pe.InitAndReload(context.Background()); err != nil {
		return nil, err
	}
	return pe, nil
}

// InitAndReload seeds builtin providers if missing and compiles patterns in memory.
func (pe *ProviderRegistryEngine) InitAndReload(ctx context.Context) error {
	existing, err := pe.db.SelectProviderRegistry(ctx)
	if err != nil {
		return err
	}

	existingMap := make(map[string]*db.ProviderRegistryRow)
	for i := range existing {
		existingMap[existing[i].ProviderKey] = &existing[i]
	}

	// Seed builtin providers if missing
	for _, seed := range InitialProviderRegistry {
		cur, exists := existingMap[seed.ProviderKey]
		needsSeed := !exists
		if exists && cur.Source == "cpa" && (cur.ModelPatterns == "[]" || cur.ModelPatterns == "") {
			needsSeed = true
		}
		if needsSeed {
			mpBytes, _ := json.Marshal(seed.ModelPatterns)
			apBytes, _ := json.Marshal(seed.AuthPatterns)
			_ = pe.db.UpsertProviderRegistry(ctx, &db.ProviderRegistryRow{
				ProviderKey:   seed.ProviderKey,
				Label:         seed.Label,
				ModelPatterns: string(mpBytes),
				AuthPatterns:  string(apBytes),
				Source:        "builtin",
				Enabled:       1,
				SortOrder:     seed.SortOrder,
			})
		}
	}

	rows, err := pe.db.SelectProviderRegistry(ctx)
	if err != nil {
		return err
	}

	var compiled []*CompiledProvider
	for _, r := range rows {
		if r.Enabled == 0 {
			continue
		}

		cp := &CompiledProvider{
			ProviderKey: r.ProviderKey,
			Label:       r.Label,
			Source:      r.Source,
			SortOrder:   r.SortOrder,
		}

		cp.ModelPatterns = compilePatternList(r.ModelPatterns)
		cp.AuthPatterns = compilePatternList(r.AuthPatterns)
		compiled = append(compiled, cp)
	}

	// Load persisted account baselines from database into memory
	loadedAccounts := make(map[string]AuthAccountInfo)
	if pe.db != nil {
		baselines, err := pe.db.SelectBaselinesAll(ctx)
		if err == nil {
			for _, b := range baselines {
				idx := strings.ToLower(strings.TrimSpace(b.AuthIndex))
				if idx == "" {
					continue
				}
				prov := ""
				if b.Provider != nil {
					prov = strings.ToLower(strings.TrimSpace(*b.Provider))
				}
				name := ""
				if b.Name != nil {
					name = strings.TrimSpace(*b.Name)
				}
				if prov != "" || name != "" {
					loadedAccounts[idx] = AuthAccountInfo{
						AuthIndex:   idx,
						ProviderKey: prov,
						Name:        name,
					}
				}
			}
		}
	}

	pe.mu.Lock()
	pe.providers = compiled
	if len(loadedAccounts) > 0 {
		for k, v := range loadedAccounts {
			pe.authAccounts[k] = v
		}
	}
	pe.mu.Unlock()

	return nil
}

func compilePatternList(raw string) []CompiledPattern {
	var list []PatternItem
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &list)
	}

	var compiled []CompiledPattern
	for _, item := range list {
		p := strings.TrimSpace(item.P)
		if p == "" {
			continue
		}
		anchor := item.Anchor
		if anchor != "^" {
			anchor = ""
		}

		patternExpr := "(?i)"
		if anchor == "^" {
			patternExpr += "^"
		}
		patternExpr += regexp.QuoteMeta(p)

		re, err := regexp.Compile(patternExpr)
		if err != nil {
			continue
		}

		compiled = append(compiled, CompiledPattern{
			P:      p,
			Anchor: anchor,
			Regex:  re,
		})
	}
	return compiled
}

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
