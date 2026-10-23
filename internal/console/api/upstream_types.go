package api

import "github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"

// CpaAuthFilesResponse represents the JSON returned by CPA /v0/management/auth-files
type CpaAuthFilesResponse struct {
	Files []CpaAuthFile `json:"files"`
}

// CpaAuthFile represents a single credential file managed by CPA
type CpaAuthFile struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Account       string              `json:"account"`
	AccountType   string              `json:"account_type"`
	AuthIndex     string              `json:"auth_index"`
	Label         string              `json:"label"`
	Email         string              `json:"email"`
	Nickname      string              `json:"nickname"`
	NickName      string              `json:"nick_name"`
	Provider      string              `json:"provider"`
	Type          string              `json:"type"`
	Status        string              `json:"status"`
	StatusMessage string              `json:"status_message"`
	Disabled      bool                `json:"disabled"`
	Unavailable   bool                `json:"unavailable"`
	Priority      *int                `json:"priority"`
	Weight        *int                `json:"weight"`
	Note          string              `json:"note"`
	Cooldowns     []CpaCooldownRecord `json:"cooldowns"`
	Quota         *CpaFileQuota       `json:"quota"`
	SupportsQuota bool                `json:"supports_quota"`
	QuotaProvider string              `json:"quota_provider"`
	QuotaPluginID string              `json:"quota_plugin_id"`
	ProjectID     string              `json:"project_id"`
	CreatedAt     string              `json:"created_at"`
	UpdatedAt     string              `json:"updated_at"`
}

// CpaCooldownRecord represents an active cooldown entry on a credential
type CpaCooldownRecord struct {
	Scope            string `json:"scope"`
	ModelKey         string `json:"model_key"`
	Reason           string `json:"reason"`
	RetryAt          string `json:"retry_at"`
	RemainingSeconds int    `json:"remaining_seconds"`
	BackoffLevel     *int   `json:"backoff_level"`
	HTTPStatus       *int   `json:"http_status"`
}

// CpaFileQuota contains parsed or native quota details
type CpaFileQuota struct {
	Signals map[string]interface{} `json:"signals"`
	Summary []CpaQuotaSummaryItem  `json:"summary"`
	Groups  []CpaQuotaGroup        `json:"groups"`
}

// CpaQuotaSummaryItem describes a summary item (points, expires_at, etc.)
type CpaQuotaSummaryItem struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
	Unit  string      `json:"unit"`
}

// CpaQuotaGroup represents a group of quota buckets
type CpaQuotaGroup struct {
	GroupID     string           `json:"group_id,omitempty"`
	DisplayName string           `json:"displayName"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Buckets     []CpaQuotaBucket `json:"buckets"`
}

// CpaQuotaBucket represents a specific rate limit or token bucket
type CpaQuotaBucket struct {
	BucketID          string   `json:"bucket_id"`
	Window            string   `json:"window"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Unit              string   `json:"unit,omitempty"`
	TotalCapacity     *float64 `json:"total_capacity,omitempty"`
	RemainCapacity    *float64 `json:"remain_capacity,omitempty"`
	UsedCapacity      *float64 `json:"used_capacity,omitempty"`
	RemainingFraction *float64 `json:"remainingFraction,omitempty"`
	RemainingPercent  *int     `json:"remaining_percent,omitempty"`
	ResetTime         string   `json:"resetTime,omitempty"`
	ResetTimeSnake    string   `json:"reset_time,omitempty"`
	ResetCountdown    string   `json:"reset_countdown,omitempty"`
	Disabled          bool     `json:"disabled"`
	UsedPercent       *float64 `json:"used_percent,omitempty"`
	ExpiresAt         string   `json:"expires_at,omitempty"`
}

// CpaOpenAICompatResponse represents CPA /v0/management/openai-compatibility
type CpaOpenAICompatResponse struct {
	Compatibility []CpaOpenAICompatProvider `json:"openai-compatibility"`
}

// CpaOpenAICompatProvider defines a custom upstream compatible provider
type CpaOpenAICompatProvider struct {
	Name          string                    `json:"name"`
	Disabled      bool                      `json:"disabled"`
	BaseURL       string                    `json:"base-url"`
	APIKeyEntries []CpaOpenAICompatKeyEntry `json:"api-key-entries"`
	Models        []CpaOpenAICompatModel    `json:"models"`
	AuthIndex     string                    `json:"auth-index,omitempty"`
}

// CpaOpenAICompatKeyEntry represents an API key entry under a compatible provider
type CpaOpenAICompatKeyEntry struct {
	APIKey    string `json:"api-key"`
	AuthIndex string `json:"auth-index"`
	Priority  *int   `json:"priority"`
	Weight    *int   `json:"weight"`
	Note      string `json:"note"`
}

// CpaOpenAICompatModel defines a model provided by a compatible provider
type CpaOpenAICompatModel struct {
	Name  string `json:"name"`
	Alias string `json:"alias"`
}

// UpstreamAccountItem is the normalized account record for the frontend
type UpstreamAccountItem struct {
	ID                string                    `json:"id"`
	Filename          string                    `json:"filename"`
	DisplayName       string                    `json:"display_name"`
	Nickname          string                    `json:"nickname"`
	Email             string                    `json:"email"`
	Provider          string                    `json:"provider"`
	Status            string                    `json:"status"`
	Disabled          bool                      `json:"disabled"`
	Health            UpstreamAccountHealth     `json:"health"`
	Priority          int                       `json:"priority"`
	Weight            int                       `json:"weight"`
	Note              string                    `json:"note"`
	AuthIndex         string                    `json:"auth_index"`
	PlanType          string                    `json:"plan_type"`
	PlanName          string                    `json:"plan_name"`
	SubscriptionUntil *string                   `json:"subscription_until"`
	Groups            []CpaQuotaGroup           `json:"groups"`
	TodayTokens       int64                     `json:"today_tokens"`
	TodayRequests     int64                     `json:"today_requests"`
	FiveHourTokens    int64                     `json:"five_hour_tokens"`
	FiveHourRequests  int64                     `json:"five_hour_requests"`
	LifetimeTokens    int64                     `json:"lifetime_tokens"`
	LifetimeRequests  int64                     `json:"lifetime_requests"`
	RawError          *string                   `json:"raw_error"`
	UpdatedAt         int64                     `json:"updated_at"`
	Validation        *db.UpstreamValidationRow `json:"validation,omitempty"`
	TotalCredits      *float64                  `json:"total_credits,omitempty"`
	RemainingCredits  *float64                  `json:"remaining_credits,omitempty"`
	UsedCredits       *float64                  `json:"used_credits,omitempty"`
	ExpiresAt         string                    `json:"expires_at,omitempty"`
}

// UpstreamAccountHealth indicates real-time operational status
type UpstreamAccountHealth struct {
	State    string                   `json:"state"`
	Label    string                   `json:"label"`
	Detail   string                   `json:"detail"`
	Blocked  bool                     `json:"blocked"`
	Cooldown *UpstreamCooldownSummary `json:"cooldown"`
}

// UpstreamCooldownSummary encapsulates cooldown details for UI cards
type UpstreamCooldownSummary struct {
	Scope            string   `json:"scope"`
	ScopeLabel       string   `json:"scope_label"`
	ModelCount       int      `json:"model_count"`
	Reason           string   `json:"reason"`
	ReasonLabel      string   `json:"reason_label"`
	RemainingSeconds int      `json:"remaining_seconds"`
	RemainingText    string   `json:"remaining_text"`
	EarliestRetryAt  *int64   `json:"earliest_retry_at"`
	HTTPStatus       *int     `json:"http_status"`
	BackoffLevel     *int     `json:"backoff_level"`
	ObservedAt       int64    `json:"observed_at"`
	Reasons          []string `json:"reasons"`
	ModelKeys        []string `json:"model_keys"`
}

// UpstreamQuotasResponse is the top-level payload for /console/api/upstream-quotas
type UpstreamQuotasResponse struct {
	Accounts           []UpstreamAccountItem      `json:"accounts"`
	PendingValidations []db.UpstreamValidationRow `json:"pending_validations"`
	DefaultPriority    int                        `json:"default_priority"`
	LastUpdated        int64                      `json:"last_updated"`
	IsLoading          bool                       `json:"is_loading"`
	Error              *string                    `json:"error"`
}
