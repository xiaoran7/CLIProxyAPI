// Package registry provides centralized model management for all AI service providers.
package registry

// ZCode's GLM models, as ZCode's own provider configuration lists them.
//
// The Start Plan serves every model but GLM-5.3; the coding plans serve all of
// them. A catalog entry whose name ends in -Trial spends the Start Plan's gift
// quota instead of the account's coding plan.
var staticZCodeModels = []*ModelInfo{
	{
		ID:                  "GLM-5.3",
		Object:              "model",
		OwnedBy:             "zhipu",
		Type:                "zcode",
		DisplayName:         "GLM-5.3",
		ContextLength:       1000000,
		MaxCompletionTokens: 128000,
		InputTokenLimit:     1000000,
		OutputTokenLimit:    128000,
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "high", "max"},
		},
	},
	{
		ID:                  "GLM-5.3-Flash",
		Object:              "model",
		OwnedBy:             "zhipu",
		Type:                "zcode",
		DisplayName:         "GLM-5.3 Flash",
		ContextLength:       1000000,
		MaxCompletionTokens: 128000,
		InputTokenLimit:     1000000,
		OutputTokenLimit:    128000,
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "high", "max"},
		},
	},
	{
		ID:                  "GLM-5.2",
		Object:              "model",
		OwnedBy:             "zhipu",
		Type:                "zcode",
		DisplayName:         "GLM-5.2",
		ContextLength:       1000000,
		MaxCompletionTokens: 128000,
		InputTokenLimit:     1000000,
		OutputTokenLimit:    128000,
		Thinking: &ThinkingSupport{
			Levels: []string{"none", "high", "max"},
		},
	},
	{
		ID:                  "GLM-5-Turbo",
		Object:              "model",
		OwnedBy:             "zhipu",
		Type:                "zcode",
		DisplayName:         "GLM-5 Turbo",
		ContextLength:       200000,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     200000,
		OutputTokenLimit:    64000,
		Thinking: &ThinkingSupport{
			Levels: []string{"none", "high"},
		},
	},
}

// zcodeTrialModels are the models the Start Plan serves.
var zcodeTrialModels = []*ModelInfo{
	{
		ID:                  "GLM-5.3-Flash-Trial",
		Object:              "model",
		OwnedBy:             "zhipu",
		Type:                "zcode",
		DisplayName:         "GLM-5.3 Flash (Start Plan)",
		ContextLength:       1000000,
		MaxCompletionTokens: 128000,
		InputTokenLimit:     1000000,
		OutputTokenLimit:    128000,
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "high", "max"},
		},
	},
	{
		ID:                  "GLM-5.2-Trial",
		Object:              "model",
		OwnedBy:             "zhipu",
		Type:                "zcode",
		DisplayName:         "GLM-5.2 (Start Plan)",
		ContextLength:       1000000,
		MaxCompletionTokens: 128000,
		InputTokenLimit:     1000000,
		OutputTokenLimit:    128000,
		Thinking: &ThinkingSupport{
			Levels: []string{"none", "high", "max"},
		},
	},
	{
		ID:                  "GLM-5-Turbo-Trial",
		Object:              "model",
		OwnedBy:             "zhipu",
		Type:                "zcode",
		DisplayName:         "GLM-5 Turbo (Start Plan)",
		ContextLength:       200000,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     200000,
		OutputTokenLimit:    64000,
		Thinking: &ThinkingSupport{
			Levels: []string{"none", "high"},
		},
	},
}

// GetZCodeModels returns the ZCode catalog: the coding plans' models plus the
// Start Plan's trial entries.
func GetZCodeModels() []*ModelInfo {
	models := make([]*ModelInfo, 0, len(staticZCodeModels)+len(zcodeTrialModels))
	for _, model := range staticZCodeModels {
		models = append(models, cloneModelInfo(model))
	}
	for _, model := range zcodeTrialModels {
		models = append(models, cloneModelInfo(model))
	}
	return models
}

// GetZCodeCodingPlanModels returns only the models a coding plan serves.
func GetZCodeCodingPlanModels() []*ModelInfo {
	models := make([]*ModelInfo, 0, len(staticZCodeModels))
	for _, model := range staticZCodeModels {
		models = append(models, cloneModelInfo(model))
	}
	return models
}
