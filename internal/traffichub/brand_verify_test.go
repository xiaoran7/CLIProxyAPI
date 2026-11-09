package traffichub

import "testing"

// 后端 InferChannel 的全部输出，必须都能被前端 getBrandNodeVisual 的品牌分支命中。
func TestInferChannelOutputsAreAllBranded(t *testing.T) {
	frontend := map[string][]string{
		"claude":      {"claude", "anthropic"},
		"openai":      {"openai", "codex", "gpt"},
		"antigravity": {"antigravity"},
		"gemini":      {"gemini", "google", "vertex"},
		"deepseek":    {"deepseek"},
		"qoder":       {"qoder"},
		"zcode":       {"zcode", "glm", "bigmodel"},
		"workbuddy":   {"workbuddy"},
		"kimi":        {"kimi", "moonshot"},
		"minimax":     {"minimax", "abab"},
		"stepfun":     {"stepfun", "step"},
		"gateway":     {"probe", "unspecified"},
		"xai":         {"xai", "grok"},
		"openrouter":  {"openrouter"},
		"opencode":    {"opencode"},
	}

	cases := []struct{ model, hint, want string }{
		{"claude-sonnet-4", "", "claude"},
		{"gpt-4o", "", "openai"},
		{"gemini-3.8-flash-high", "", "gemini"},
		{"deepseek-v3", "", "deepseek"},
		{"qwen-max", "", "qoder"},
		{"glm-4.6", "", "zcode"},
		{"workbuddy-cn-x", "", "workbuddy"},
		{"kimi-k2", "", "kimi"},
		{"minimax-m2", "", "minimax"},
		{"step-5-preview", "", "stepfun"},
		{"grok-4", "", "xai"},
		{"probe:ping", "", "gateway"},
	}

	for _, c := range cases {
		got := InferChannel(c.model, c.hint)
		if got != c.want {
			t.Errorf("InferChannel(%q) = %q, want %q", c.model, got, c.want)
		}
		if _, ok := frontend[got]; !ok {
			t.Errorf("channel %q (from model %q) has no frontend brand branch -> renders as gray placeholder", got, c.model)
		}
	}
}
