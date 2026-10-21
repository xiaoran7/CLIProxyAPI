package db

import "time"

// KeyRow represents a row in the keys table.
type KeyRow struct {
	ID        int64     `json:"id"`
	APIKey    string    `json:"api_key"`
	Alias     string    `json:"alias"`
	Active    int       `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// QuotaRuleRow represents a row in the quota_rules table.
type QuotaRuleRow struct {
	KeyID          int64     `json:"key_id"`
	Enabled        int       `json:"enabled"`
	DailyLimit     *int64    `json:"daily_limit"`
	HardRatio      float64   `json:"hard_ratio"`
	RPMLimit       int       `json:"rpm_limit"`
	ModelWhitelist string    `json:"model_whitelist"`
	ModelGroup     string    `json:"model_group"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// UsageDailyRow represents a row in the usage_daily table.
type UsageDailyRow struct {
	Date             string `json:"date"`
	KeyID            int64  `json:"key_id"`
	Model            string `json:"model"`
	Requests         int64  `json:"requests"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	CachedTokens     int64  `json:"cached_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	EstimatedTokens  int64  `json:"estimated_tokens"`
	SuccessCount     int64  `json:"success_count"`
	FailureCount     int64  `json:"failure_count"`
}

// RequestEventRow represents a row in the request_events or usage_outbox table.
type RequestEventRow struct {
	ID               int64   `json:"id"`
	TS               int64   `json:"ts"`
	KeyID            *int64  `json:"key_id"`
	Model            string  `json:"model"`
	Status           int     `json:"status"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	LatencyMs        int64   `json:"latency_ms"`
	TtftMs           *int64  `json:"ttft_ms"`
	Stream           int     `json:"stream"`
	Estimated        int     `json:"estimated"`
	ErrorType        *string `json:"error_type"`
	AuthIndex        string  `json:"auth_index"`
	ReasoningEffort  string  `json:"reasoning_effort"`
	TokensPerSec     float64 `json:"tokens_per_sec"`
}

// ErrorEventRow represents a row in the error_events table.
type ErrorEventRow struct {
	ID           int64   `json:"id"`
	TS           int64   `json:"ts"`
	KeyID        *int64  `json:"key_id"`
	AuthIndex    *string `json:"auth_index"`
	Provider     *string `json:"provider"`
	Model        *string `json:"model"`
	Status       int     `json:"status"`
	ErrorType    *string `json:"error_type"`
	ErrorBody    *string `json:"error_body"`
	RetryAfterMs int64   `json:"retry_after_ms"`
	RecoverAt    int64   `json:"recover_at"`
}

// ModelPriceRow represents a row in the model_prices table.
type ModelPriceRow struct {
	ID              int64     `json:"id"`
	Pattern         string    `json:"pattern"`
	PromptPrice     float64   `json:"prompt_price"`
	CompletionPrice float64   `json:"completion_price"`
	CacheReadPrice  float64   `json:"cache_read_price"`
	Multiplier      float64   `json:"multiplier"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// QuotaSnapshotRow represents a row in the quota_snapshots table.
type QuotaSnapshotRow struct {
	ID               int64   `json:"id"`
	TS               int64   `json:"ts"`
	AuthIndex        string  `json:"auth_index"`
	Provider         string  `json:"provider"`
	AccountEmail     *string `json:"account_email"`
	WindowName       *string `json:"window_name"`
	RemainingPercent float64 `json:"remaining_percent"`
	ResetAt          *int64  `json:"reset_at"`
}

// UpstreamAuthBaselineRow represents a row in the upstream_auth_baselines table.
type UpstreamAuthBaselineRow struct {
	AuthIndex            string    `json:"auth_index"`
	Name                 *string   `json:"name"`
	Provider             *string   `json:"provider"`
	BaseRequests         int64     `json:"base_requests"`
	BaseSuccessCount     int64     `json:"base_success_count"`
	BaseFailureCount     int64     `json:"base_failure_count"`
	BasePromptTokens     int64     `json:"base_prompt_tokens"`
	BaseCompletionTokens int64     `json:"base_completion_tokens"`
	BaseCachedTokens     int64     `json:"base_cached_tokens"`
	BaseTotalTokens      int64     `json:"base_total_tokens"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// UpstreamDailyStatRow represents a row in the upstream_daily_stats table.
type UpstreamDailyStatRow struct {
	Date             string `json:"date"`
	AuthIndex        string `json:"auth_index"`
	Provider         string `json:"provider"`
	Requests         int64  `json:"requests"`
	SuccessCount     int64  `json:"success_count"`
	FailureCount     int64  `json:"failure_count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	CachedTokens     int64  `json:"cached_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

// UpstreamValidationRow represents a row in the upstream_validations table.
type UpstreamValidationRow struct {
	ID            int64   `json:"id"`
	TS            int64   `json:"ts"`
	AuthIndex     string  `json:"auth_index"`
	Provider      string  `json:"provider"`
	AccountName   *string `json:"account_name"`
	ValidationURL string  `json:"validation_url"`
	ErrorMessage  *string `json:"error_message"`
	RawError      *string `json:"raw_error"`
	Status        string  `json:"status"`
	UpdatedAt     int64   `json:"updated_at"`
}

// ProviderRegistryRow represents a row in the provider_registry table.
type ProviderRegistryRow struct {
	ProviderKey   string    `json:"provider_key"`
	Label         string    `json:"label"`
	ModelPatterns string    `json:"model_patterns"`
	AuthPatterns  string    `json:"auth_patterns"`
	Source        string    `json:"source"`
	Enabled       int       `json:"enabled"`
	SortOrder     int       `json:"sort_order"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// DailyStatsSummary represents aggregated stats over a date range.
type DailyStatsSummary struct {
	Date             string `json:"date"`
	Requests         int64  `json:"requests"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	CachedTokens     int64  `json:"cached_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	SuccessCount     int64  `json:"success_count"`
	FailureCount     int64  `json:"failure_count"`
}

// ModelStatsSummary represents aggregated model stats over a date range.
type ModelStatsSummary struct {
	Model            string `json:"model"`
	Requests         int64  `json:"requests"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	CachedTokens     int64  `json:"cached_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	SuccessCount     int64  `json:"success_count"`
	FailureCount     int64  `json:"failure_count"`
}
