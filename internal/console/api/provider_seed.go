// 提供商注册中心种子数据与内置规则 —— 拆自 provider_registry.go
package api

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
			{P: "argon", Anchor: ""},
			{P: "gemini-4", Anchor: "^"},
			{P: "gemini-3.8", Anchor: "^"},
			{P: "gemini-3.5", Anchor: "^"},
			{P: "gemini-3.1", Anchor: "^"},
			{P: "gemini-3-flash", Anchor: "^"},
			{P: "claude-opus-4-6", Anchor: "^"},
			{P: "claude-sonnet-4-6", Anchor: "^"},
			{P: "claude-opus-5-5-high", Anchor: "^"},
			{P: "claude-sonnet-5-5-high", Anchor: "^"},
			{P: "gpt-oss-120b", Anchor: "^"},
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
			{P: "workbuddy-cn/", Anchor: "^"},
			{P: "codebuddy/", Anchor: "^"},
			{P: "wb-cn/", Anchor: "^"},
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
			{P: "workbuddy/", Anchor: "^"},
			{P: "workbuddy-intl/", Anchor: "^"},
			{P: "wb/", Anchor: "^"},
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
		ProviderKey: "zcode-cn",
		Label:       "BigModel (智谱开放平台)",
		ModelPatterns: []PatternItem{
			{P: "zcode-cn", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "zcode-cn", Anchor: ""},
			{P: "zcode-bigmodel", Anchor: ""},
			{P: "bigmodel", Anchor: ""},
		},
		SortOrder: 76,
	},
	{
		ProviderKey: "zcode",
		Label:       "ZCode (Z.ai 国际版)",
		ModelPatterns: []PatternItem{
			{P: "-trial", Anchor: ""},
			{P: "zcode", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "zcode", Anchor: ""},
			{P: "zcode-zai", Anchor: ""},
		},
		SortOrder: 77,
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
	{
		ProviderKey: "meta",
		Label:       "Meta (Llama)",
		ModelPatterns: []PatternItem{
			{P: "llama", Anchor: ""},
			{P: "meta/", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "meta", Anchor: ""},
			{P: "llama", Anchor: ""},
		},
		SortOrder: 120,
	},
	{
		ProviderKey: "xai",
		Label:       "xAI (Grok)",
		ModelPatterns: []PatternItem{
			{P: "grok", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "xai", Anchor: ""},
			{P: "grok", Anchor: ""},
		},
		SortOrder: 130,
	},
	{
		ProviderKey: "mistral",
		Label:       "Mistral AI",
		ModelPatterns: []PatternItem{
			{P: "mistral", Anchor: ""},
			{P: "mixtral", Anchor: ""},
			{P: "codestral", Anchor: ""},
			{P: "pixtral", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "mistral", Anchor: ""},
		},
		SortOrder: 140,
	},
	{
		ProviderKey: "minimax",
		Label:       "MiniMax 海螺",
		ModelPatterns: []PatternItem{
			{P: "minimax", Anchor: ""},
			{P: "abab", Anchor: ""},
		},
		AuthPatterns: []PatternItem{
			{P: "minimax", Anchor: ""},
		},
		SortOrder: 150,
	},
}
