package registry

import "testing"

func TestGlobalExcludedModelsFiltering(t *testing.T) {
	r := newTestModelRegistry()
	r.RegisterClient("client-1", "OpenAI", []*ModelInfo{
		{ID: "deepseek-v4.1-flash", DisplayName: "DeepSeek Flash"},
		{ID: "deepseek-v4.1-flash-free", DisplayName: "DeepSeek Flash Free"},
		{ID: "glm-5.3-flash-free", DisplayName: "GLM Flash Free"},
		{ID: "gemini-2.5-pro", DisplayName: "Gemini Pro"},
	})

	// Initial state: all 4 models available
	initial := r.GetAvailableModels("openai")
	if len(initial) != 4 {
		t.Fatalf("expected 4 available models, got %d", len(initial))
	}
	if r.GetModelCount("deepseek-v4.1-flash-free") != 1 {
		t.Fatalf("expected model count 1 for deepseek-v4.1-flash-free")
	}

	// Apply exact exclusion
	r.SetGlobalExcludedModels([]string{"deepseek-v4.1-flash-free"})
	filtered := r.GetAvailableModels("openai")
	if len(filtered) != 3 {
		t.Fatalf("expected 3 models after exact exclusion, got %d", len(filtered))
	}
	for _, m := range filtered {
		if m["id"] == "deepseek-v4.1-flash-free" {
			t.Fatalf("expected deepseek-v4.1-flash-free to be excluded")
		}
	}
	if r.GetModelCount("deepseek-v4.1-flash-free") != 0 {
		t.Fatalf("expected model count 0 for excluded model")
	}
	if r.GetModelInfo("deepseek-v4.1-flash-free", "") != nil {
		t.Fatalf("expected nil ModelInfo for excluded model")
	}
	if r.GetModelProviders("deepseek-v4.1-flash-free") != nil {
		t.Fatalf("expected nil providers for excluded model")
	}

	// Apply wildcard exclusion
	r.SetGlobalExcludedModels([]string{"*-free"})
	wildcardFiltered := r.GetAvailableModels("openai")
	if len(wildcardFiltered) != 2 {
		t.Fatalf("expected 2 models after wildcard *-free exclusion, got %d", len(wildcardFiltered))
	}
	for _, m := range wildcardFiltered {
		id := m["id"].(string)
		if id == "deepseek-v4.1-flash-free" || id == "glm-5.3-flash-free" {
			t.Fatalf("expected %s to be excluded by wildcard", id)
		}
	}

	// Clear exclusion
	r.SetGlobalExcludedModels(nil)
	restored := r.GetAvailableModels("openai")
	if len(restored) != 4 {
		t.Fatalf("expected all 4 models restored, got %d", len(restored))
	}
}
