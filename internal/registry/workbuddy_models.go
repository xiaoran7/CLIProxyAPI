package registry

import (
	"strings"
	"sync"
)

type workbuddyModelsStore struct {
	mu     sync.RWMutex
	models []*ModelInfo
}

var workbuddyCatalogStore = &workbuddyModelsStore{}

var staticWorkBuddyIntlModels = []*ModelInfo{
	{
		ID:                  "deepseek-v4.1-flash",
		Object:              "model",
		OwnedBy:             "deepseek",
		Type:                "workbuddy",
		DisplayName:         "DeepSeek V4.1 Flash (WorkBuddy)",
		ContextLength:       1048576,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    64000,
		Thinking: &ThinkingSupport{
			Levels: []string{"high", "max"},
		},
	},
	{
		ID:                  "deepseek-v4.1-flash-free",
		Object:              "model",
		OwnedBy:             "deepseek",
		Type:                "workbuddy",
		DisplayName:         "DeepSeek V4.1 Flash (Free / 1M)",
		ContextLength:       1048576,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    64000,
		Thinking: &ThinkingSupport{
			Levels: []string{"high", "max"},
		},
	},
	{
		ID:                  "deepseek-v4-pro",
		Object:              "model",
		OwnedBy:             "deepseek",
		Type:                "workbuddy",
		DisplayName:         "DeepSeek V4 Pro (WorkBuddy)",
		ContextLength:       1048576,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    64000,
		Thinking: &ThinkingSupport{
			Levels: []string{"high", "max"},
		},
	},
	{
		ID:                  "gpt-6-astra",
		Object:              "model",
		OwnedBy:             "openai",
		Type:                "workbuddy",
		DisplayName:         "GPT-6 Astra (WorkBuddy)",
		ContextLength:       1000000,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1000000,
		OutputTokenLimit:    64000,
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "medium", "high", "max"},
		},
	},
	{
		ID:                  "gpt-5.6-sol",
		Object:              "model",
		OwnedBy:             "openai",
		Type:                "workbuddy",
		DisplayName:         "GPT-5.6 Sol (WorkBuddy)",
		ContextLength:       1000000,
		MaxCompletionTokens: 128000,
		InputTokenLimit:     1000000,
		OutputTokenLimit:    128000,
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "medium", "high", "max"},
		},
	},
	{
		ID:                  "claude-fable-5-1",
		Object:              "model",
		OwnedBy:             "anthropic",
		Type:                "workbuddy",
		DisplayName:         "Claude Fable 5.1 (WorkBuddy)",
		ContextLength:       1000000,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1000000,
		OutputTokenLimit:    64000,
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "medium", "high", "xhigh", "max"},
		},
	},
	{
		ID:                  "claude-opus-5-5",
		Object:              "model",
		OwnedBy:             "anthropic",
		Type:                "workbuddy",
		DisplayName:         "Claude Opus 5.5 (WorkBuddy)",
		ContextLength:       1000000,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1000000,
		OutputTokenLimit:    64000,
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "medium", "high", "xhigh", "max"},
		},
	},
	{
		ID:                  "claude-sonnet-5-5",
		Object:              "model",
		OwnedBy:             "anthropic",
		Type:                "workbuddy",
		DisplayName:         "Claude Sonnet 5.5 (WorkBuddy)",
		ContextLength:       1000000,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1000000,
		OutputTokenLimit:    64000,
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "medium", "high", "xhigh", "max"},
		},
	},
	{
		ID:                  "glm-5.3",
		Object:              "model",
		OwnedBy:             "zhipu",
		Type:                "workbuddy",
		DisplayName:         "GLM-5.3 (WorkBuddy)",
		ContextLength:       1048576,
		MaxCompletionTokens: 128000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    128000,
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "high", "max"},
		},
	},
	{
		ID:                  "glm-5.3-flash",
		Object:              "model",
		OwnedBy:             "zhipu",
		Type:                "workbuddy",
		DisplayName:         "GLM-5.3 Flash (WorkBuddy)",
		ContextLength:       1048576,
		MaxCompletionTokens: 128000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    128000,
	},
	{
		ID:                  "glm-5.3-flash-free",
		Object:              "model",
		OwnedBy:             "zhipu",
		Type:                "workbuddy",
		DisplayName:         "GLM-5.3 Flash (Free / 1M)",
		ContextLength:       1048576,
		MaxCompletionTokens: 128000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    128000,
	},
	{
		ID:                  "kimi-k3",
		Object:              "model",
		OwnedBy:             "moonshotai",
		Type:                "workbuddy",
		DisplayName:         "Kimi K3 (WorkBuddy)",
		ContextLength:       1048576,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    64000,
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "medium", "high"},
		},
	},
	{
		ID:                  "minimax-m3",
		Object:              "model",
		OwnedBy:             "minimax",
		Type:                "workbuddy",
		DisplayName:         "MiniMax M3 (WorkBuddy)",
		ContextLength:       512000,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     512000,
		OutputTokenLimit:    64000,
	},
}

var staticWorkBuddyCNModels = []*ModelInfo{
	{
		ID:                  "hy4-preview-f",
		Object:              "model",
		OwnedBy:             "tencent",
		Type:                "workbuddy",
		DisplayName:         "Hunyuan 4 Preview F (CodeBuddy)",
		ContextLength:       1048576,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    64000,
	},
	{
		ID:                  "hy3",
		Object:              "model",
		OwnedBy:             "tencent",
		Type:                "workbuddy",
		DisplayName:         "Hunyuan 3 (CodeBuddy)",
		ContextLength:       192000,
		MaxCompletionTokens: 32000,
		InputTokenLimit:     192000,
		OutputTokenLimit:    32000,
	},
	{
		ID:                  "deepseek-v4.1-flash",
		Object:              "model",
		OwnedBy:             "deepseek",
		Type:                "workbuddy",
		DisplayName:         "DeepSeek V4.1 Flash (CodeBuddy)",
		ContextLength:       1048576,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    64000,
		Thinking: &ThinkingSupport{
			Levels: []string{"high", "max"},
		},
	},
	{
		ID:                  "deepseek-v4-pro",
		Object:              "model",
		OwnedBy:             "deepseek",
		Type:                "workbuddy",
		DisplayName:         "DeepSeek V4 Pro (CodeBuddy)",
		ContextLength:       1048576,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    64000,
		Thinking: &ThinkingSupport{
			Levels: []string{"high", "max"},
		},
	},
	{
		ID:                  "glm-5.3",
		Object:              "model",
		OwnedBy:             "zhipu",
		Type:                "workbuddy",
		DisplayName:         "GLM-5.3 (CodeBuddy)",
		ContextLength:       1048576,
		MaxCompletionTokens: 128000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    128000,
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "high", "max"},
		},
	},
	{
		ID:                  "kimi-k3",
		Object:              "model",
		OwnedBy:             "moonshotai",
		Type:                "workbuddy",
		DisplayName:         "Kimi K3 (CodeBuddy)",
		ContextLength:       1048576,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    64000,
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "medium", "high"},
		},
	},
	{
		ID:                  "minimax-m3",
		Object:              "model",
		OwnedBy:             "minimax",
		Type:                "workbuddy",
		DisplayName:         "MiniMax M3 (CodeBuddy)",
		ContextLength:       512000,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     512000,
		OutputTokenLimit:    64000,
	},
}

// GetWorkBuddyModels returns models filtered by realm ("intl" or "cn").
func GetWorkBuddyModels(realm string) []*ModelInfo {
	if strings.EqualFold(strings.TrimSpace(realm), "cn") {
		return cloneModelInfos(staticWorkBuddyCNModels)
	}
	return cloneModelInfos(staticWorkBuddyIntlModels)
}

// GetAllWorkBuddyModels returns all unique WorkBuddy models across both realms.
func GetAllWorkBuddyModels() []*ModelInfo {
	seen := make(map[string]bool)
	var all []*ModelInfo
	for _, m := range append(staticWorkBuddyIntlModels, staticWorkBuddyCNModels...) {
		if m != nil && !seen[m.ID] {
			seen[m.ID] = true
			all = append(all, m)
		}
	}
	return cloneModelInfos(all)
}

// LookupWorkBuddyModel finds a WorkBuddy model by ID and optional realm.
func LookupWorkBuddyModel(modelID, realm string) *ModelInfo {
	clean := strings.ToLower(strings.TrimSpace(modelID))
	clean = strings.TrimPrefix(clean, "workbuddy/")
	if clean == "" {
		return nil
	}

	models := GetWorkBuddyModels(realm)

	// First pass: exact match (ensures -free alias returns its own dedicated entry)
	for _, m := range models {
		if m != nil && strings.ToLower(m.ID) == clean {
			return cloneModelInfo(m)
		}
	}

	// Second pass: fallback without -free suffix if only base model is registered
	// Note: CN realm has no -free tier/models, so fallback without -free suffix should not apply to CN.
	if !strings.EqualFold(strings.TrimSpace(realm), "cn") {
		target := strings.TrimSuffix(clean, "-free")
		if target != clean {
			for _, m := range models {
				if m != nil && strings.ToLower(m.ID) == target {
					return cloneModelInfo(m)
				}
			}
		}
	}

	return nil
}
