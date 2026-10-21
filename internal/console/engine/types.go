package engine

// KeyMeta holds public/cached metadata for a registered key.
type KeyMeta struct {
	ID         int64  `json:"id"`
	APIKey     string `json:"-"`
	DisplayKey string `json:"display_key"`
	Alias      string `json:"alias"`
	Active     bool   `json:"active"`
}

// QuotaRule defines the quota, rate-limit, and allowlist rules for a key.
type QuotaRule struct {
	KeyID          int64    `json:"key_id"`
	Enabled        bool     `json:"enabled"`
	DailyLimit     int64    `json:"daily_limit"`
	HardRatio      float64  `json:"hard_ratio"`
	RPMLimit       int      `json:"rpm_limit"`
	ModelWhitelist string   `json:"model_whitelist"`
	ModelGroup     string   `json:"model_group"`
	AllowedModels  []string `json:"-"`
}

// UsageAgg aggregates live tokens and requests for a specific date-key-model bucket in memory.
type UsageAgg struct {
	Requests         int64 `json:"requests"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	CachedTokens     int64 `json:"cached_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	EstimatedTokens  int64 `json:"estimated_tokens"`
}

// RequestInfo carries telemetry data from an executed LLM request to the outbox.
type RequestInfo struct {
	KeyID            *int64  `json:"key_id"`
	RawKey           string  `json:"raw_key"`
	Model            string  `json:"model"`
	Status           int     `json:"status"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	LatencyMs        int64   `json:"latency_ms"`
	TtftMs           *int64  `json:"ttft_ms"`
	Stream           bool    `json:"stream"`
	Estimated        bool    `json:"estimated"`
	ErrorType        string  `json:"error_type"`
	AuthIndex        string  `json:"auth_index"`
	ReasoningEffort  string  `json:"reasoning_effort"`
	TokensPerSec     float64 `json:"tokens_per_sec"`
}

// CheckResult represents the verdict of a quota, RPM, or model authorization check.
type CheckResult struct {
	Allowed   bool   `json:"allowed"`
	Reason    string `json:"reason,omitempty"`
	Usage     int64  `json:"usage,omitempty"`
	Limit     int64  `json:"limit,omitempty"`
	HardLimit int64  `json:"hard_limit,omitempty"`
	RPMLimit  int    `json:"rpm_limit,omitempty"`
}
