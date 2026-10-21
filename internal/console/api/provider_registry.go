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

const (
	UnknownProviderKey        = "other"
	UnknownProviderLabel      = "其他上游"
	UnregisteredProviderLabel = "未登记提供商"
	OtherModelLabel           = "其他模型"
)

// PatternItem represents a matching rule with an anchor ('^' for prefix, ” for substring).
type PatternItem struct {
	P      string `json:"p"`
	Anchor string `json:"anchor"`
}

// SeedProvider defines a default provider registry entry.
type SeedProvider struct {
	ProviderKey   string
	Label         string
	ModelPatterns []PatternItem
	AuthPatterns  []PatternItem
	SortOrder     int
}

// InitialProviderRegistry holds the canonical builtin providers and their patterns.
var InitialProviderRegistry = []SeedProvider{
	{
		ProviderKey: "antigravity",
		Label:       "反重力 Antigravity",
		ModelPatterns: []PatternItem{
			{P: "antigravity", Anchor: "^"},
		},
		AuthPatterns: []PatternItem{
			{P: "antigravity", Anchor: ""},
		},
		SortOrder: 10,
	},
	{
		ProviderKey: "codex",
		Label:       "OpenAI Codex",
		ModelPatterns: []PatternItem{
			{P: "gpt-", Anchor: "^"},
			{P: "chatgpt", Anchor: ""},
			{P: "o1", Anchor: "^"},
			{P: "o3", Anchor: "^"},
			{P: "o4", Anchor: "^"},
			{P: "text-embedding", Anchor: "^"},
		},
		AuthPatterns: []PatternItem{
			{P: "codex", Anchor: ""},
			{P: "openai", Anchor: ""},
		},
		SortOrder: 20,
	},
	{
		ProviderKey: "gemini",
		Label:       "Google Gemini",
		ModelPatterns: []PatternItem{
			{P: "gemini", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "gemini", Anchor: ""},
		},
		SortOrder: 30,
	},
	{
		ProviderKey: "claude",
		Label:       "Anthropic Claude",
		ModelPatterns: []PatternItem{
			{P: "claude", Anchor: ""},
			{P: "anthropic", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "claude", Anchor: ""},
			{P: "anthropic", Anchor: ""},
		},
		SortOrder: 40,
	},
	{
		ProviderKey: "opencode",
		Label:       "OpenCode Zen",
		ModelPatterns: []PatternItem{
			{P: "opencode", Anchor: ""},
			{P: "-free", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "opencode", Anchor: ""},
		},
		SortOrder: 45,
	},
	{
		ProviderKey: "stepfun",
		Label:       "StepFun 阶跃星辰",
		ModelPatterns: []PatternItem{
			{P: "step", Anchor: ""},
			{P: "stepfun", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "step", Anchor: ""},
			{P: "stepfun", Anchor: ""},
		},
		SortOrder: 50,
	},
	{
		ProviderKey: "stealth",
		Label:       "stealth",
		ModelPatterns: []PatternItem{
			{P: "stealth/", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "stealth", Anchor: ""},
		},
		SortOrder: 65,
	},
	{
		ProviderKey: "workbuddy-cn",
		Label:       "WorkBuddy (CodeBuddy)",
		ModelPatterns: []PatternItem{
			{P: "workbuddy-cn", Anchor: ""},
			{P: "codebuddy", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "workbuddy-cn", Anchor: ""},
			{P: "codebuddy", Anchor: ""},
		},
		SortOrder: 68,
	},
	{
		ProviderKey: "workbuddy",
		Label:       "腾讯 WorkBuddy",
		ModelPatterns: []PatternItem{
			{P: "workbuddy", Anchor: ""},
			{P: "copilot.tencent", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "workbuddy", Anchor: ""},
		},
		SortOrder: 69,
	},
	{
		ProviderKey: "deepseek",
		Label:       "DeepSeek",
		ModelPatterns: []PatternItem{
			{P: "deepseek", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "deepseek", Anchor: ""},
		},
		SortOrder: 70,
	},
	{
		ProviderKey: "qoder-cn",
		Label:       "Qoder (通义灵码)",
		ModelPatterns: []PatternItem{
			{P: "qoder-cn", Anchor: ""},
			{P: "lingma-cn", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "qoder-cn", Anchor: ""},
			{P: "lingma-cn", Anchor: ""},
		},
		SortOrder: 74,
	},
	{
		ProviderKey: "qoder",
		Label:       "Qoder 阿里通义灵码",
		ModelPatterns: []PatternItem{
			{P: "qoder", Anchor: ""},
			{P: "lingma", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "qoder", Anchor: ""},
			{P: "lingma", Anchor: ""},
		},
		SortOrder: 75,
	},
	{
		ProviderKey: "openrouter",
		Label:       "OpenRouter",
		ModelPatterns: []PatternItem{
			{P: "openrouter", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "openrouter", Anchor: ""},
		},
		SortOrder: 80,
	},
	{
		ProviderKey: "qwen",
		Label:       "Qwen 通义千问",
		ModelPatterns: []PatternItem{
			{P: "qwen", Anchor: ""},
			{P: "qianwen", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "qwen", Anchor: ""},
		},
		SortOrder: 90,
	},
	{
		ProviderKey: "moonshot",
		Label:       "Moonshot Kimi",
		ModelPatterns: []PatternItem{
			{P: "kimi", Anchor: ""},
			{P: "moonshot", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "kimi", Anchor: ""},
			{P: "moonshot", Anchor: ""},
		},
		SortOrder: 100,
	},
	{
		ProviderKey: "zhipu",
		Label:       "Zhipu GLM",
		ModelPatterns: []PatternItem{
			{P: "glm", Anchor: ""},
			{P: "zhipu", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "zhipu", Anchor: ""},
		},
		SortOrder: 110,
	},
}

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

// ProviderRegistryEngine coordinates in-memory provider matching.
type ProviderRegistryEngine struct {
	mu        sync.RWMutex
	providers []*CompiledProvider
	db        *db.DB
}

// NewProviderRegistryEngine initializes and populates the registry engine.
func NewProviderRegistryEngine(database *db.DB) (*ProviderRegistryEngine, error) {
	pe := &ProviderRegistryEngine{
		db: database,
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

	pe.mu.Lock()
	pe.providers = compiled
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

// InferProvider identifies the AI provider from model name and/or auth_index.
func (pe *ProviderRegistryEngine) InferProvider(model, authIndex string) ProviderInferenceResult {
	modelStr := strings.ToLower(strings.TrimSpace(model))
	authStr := strings.ToLower(strings.TrimSpace(authIndex))

	pe.mu.RLock()
	defer pe.mu.RUnlock()

	// 1. Evaluate auth_patterns first, ordered by sort_order
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

	return ProviderInferenceResult{
		Key:       "",
		Label:     "",
		MatchedBy: "none",
		Source:    "",
	}
}

// InferModelProviderLabel returns display label inferred purely from model name.
func (pe *ProviderRegistryEngine) InferModelProviderLabel(model string) string {
	res := pe.InferProvider(model, "")
	if res.Label != "" {
		return res.Label
	}
	return OtherModelLabel
}

// GetProviderLabel returns provider label by provider key.
func (pe *ProviderRegistryEngine) GetProviderLabel(key string) string {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return ""
	}

	pe.mu.RLock()
	defer pe.mu.RUnlock()

	for _, cp := range pe.providers {
		if cp.ProviderKey == k {
			return cp.Label
		}
	}
	return ""
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
	fb := strings.TrimSpace(fallbackName)
	if fb != "" {
		return fb
	}
	return UnknownProviderLabel
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
