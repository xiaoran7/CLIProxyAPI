package registry

import (
	"testing"
)

func TestOpenCodeModelsCatalog(t *testing.T) {
	models := GetOpenCodeModels()
	if len(models) == 0 {
		t.Fatalf("expected non-empty OpenCode models catalog")
	}

	testCases := []struct {
		id            string
		expectedCtx   int
		hasThinking   bool
		expectedOwner string
	}{
		{"glm-5.3-flash-free", 1048576, true, "zhipu"},
		{"beam-free", 1048576, false, "opencode"},
		{"nemotron-3.5-super-vl-free", 1048576, false, "nvidia"},
		{"minimax-m3.1-flash-free", 1048576, false, "minimax"},
		{"big-pickle", 200000, false, "opencode"},
		{"cat-jam-free", 262144, true, "opencode"},
		{"deepseek-v4.1-flash-free", 1048576, true, "deepseek"},
		{"claude-sonnet-5-5", 1000000, true, "anthropic"},
	}

	for _, tc := range testCases {
		m := LookupOpenCodeModel(tc.id)
		if m == nil {
			t.Fatalf("model %s not found in catalog", tc.id)
		}
		if m.ContextLength != tc.expectedCtx {
			t.Errorf("model %s context length = %d, expected %d", tc.id, m.ContextLength, tc.expectedCtx)
		}
		if tc.hasThinking && (m.Thinking == nil || len(m.Thinking.Levels) == 0) {
			t.Errorf("model %s expected thinking levels, got none", tc.id)
		}
		if m.OwnedBy != tc.expectedOwner {
			t.Errorf("model %s owned by %s, expected %s", tc.id, m.OwnedBy, tc.expectedOwner)
		}
		if !IsOpenCodeModel(tc.id) {
			t.Errorf("expected IsOpenCodeModel(%q) to be true", tc.id)
		}
	}

	// Verify channel resolution
	channelModels := GetStaticModelDefinitionsByChannel("opencode")
	if len(channelModels) == 0 {
		t.Fatalf("expected channel models for opencode to be non-empty")
	}
}
