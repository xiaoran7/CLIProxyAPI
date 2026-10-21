package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

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
	DisplayName string           `json:"displayName"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Buckets     []CpaQuotaBucket `json:"buckets"`
}

// CpaQuotaBucket represents a specific rate limit or token bucket
type CpaQuotaBucket struct {
	BucketID          string   `json:"bucket_id"`
	Window            string   `json:"window"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Unit              string   `json:"unit"`
	TotalCapacity     *float64 `json:"total_capacity"`
	RemainCapacity    *float64 `json:"remain_capacity"`
	UsedCapacity      *float64 `json:"used_capacity"`
	RemainingFraction *float64 `json:"remainingFraction"`
	RemainingPercent  *int     `json:"remaining_percent"`
	ResetTime         string   `json:"resetTime"`
	ExpiresAt         string   `json:"expires_at"`
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
	RawError          *string                   `json:"raw_error"`
	UpdatedAt         int64                     `json:"updated_at"`
	Validation        *db.UpstreamValidationRow `json:"validation,omitempty"`
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

var (
	upstreamCacheMu          sync.RWMutex
	cachedUpstreamQuotasResp *UpstreamQuotasResponse
	cachedUpstreamAt         time.Time
)

// InvalidateUpstreamQuotaCache forces the next fetch to query fresh data
func InvalidateUpstreamQuotaCache() {
	upstreamCacheMu.Lock()
	defer upstreamCacheMu.Unlock()
	cachedUpstreamQuotasResp = nil
	cachedUpstreamAt = time.Time{}
}

const (
	defaultAuthPriority = 0
	defaultAuthWeight   = 100
)

var cooldownReasonLabels = map[string]string{
	"quota":                "额度限制",
	"credential_quota":     "凭证额度限制",
	"cloudflare_challenge": "Cloudflare 验证",
	"invalid_grant":        "授权无效",
	"unauthorized":         "未获授权",
	"payment_required":     "访问受限",
	"not_found":            "资源未找到",
	"model_not_supported":  "模型不受支持",
	"transient_error":      "上游暂时出错",
}

func formatCooldownRemaining(seconds int) string {
	if seconds <= 0 {
		return "计时已到期"
	}
	if seconds < 60 {
		return fmt.Sprintf("%d 秒", seconds)
	}
	minutes := (seconds + 59) / 60
	if minutes < 60 {
		return fmt.Sprintf("%d 分钟", minutes)
	}
	hours := minutes / 60
	restMin := minutes % 60
	if hours >= 24 {
		return fmt.Sprintf("%d 天 %d 小时", hours/24, hours%24)
	}
	if restMin > 0 {
		return fmt.Sprintf("%d 小时 %d 分钟", hours, restMin)
	}
	return fmt.Sprintf("%d 小时", hours)
}

func computeHealth(file CpaAuthFile) UpstreamAccountHealth {
	var activeCooldowns []CpaCooldownRecord
	for _, cd := range file.Cooldowns {
		if cd.RemainingSeconds > 0 {
			activeCooldowns = append(activeCooldowns, cd)
		}
	}

	if len(activeCooldowns) > 0 {
		modelCount := 0
		hasCredentialScope := false
		var reasons []string
		var modelKeys []string
		minRemaining := activeCooldowns[0].RemainingSeconds
		primaryReason := activeCooldowns[0].Reason

		for _, cd := range activeCooldowns {
			if cd.Scope == "credential" {
				hasCredentialScope = true
			} else if cd.Scope == "model" {
				modelCount++
				if cd.ModelKey != "" {
					modelKeys = append(modelKeys, cd.ModelKey)
				}
			}
			if cd.RemainingSeconds < minRemaining {
				minRemaining = cd.RemainingSeconds
			}
			reasons = append(reasons, cd.Reason)
		}

		scopeStr := "models"
		scopeLabel := fmt.Sprintf("%d 个模型冷却", modelCount)
		if hasCredentialScope {
			if modelCount > 0 {
				scopeStr = "credential_and_models"
				scopeLabel = fmt.Sprintf("凭证及 %d 个模型冷却", modelCount)
			} else {
				scopeStr = "credential"
				scopeLabel = "整个凭证"
			}
		}

		reasonLabel := cooldownReasonLabels[primaryReason]
		if reasonLabel == "" {
			reasonLabel = "冷却限制"
		}

		remText := formatCooldownRemaining(minRemaining)
		detail := fmt.Sprintf("%s · %s · 最早结束约 %s", reasonLabel, scopeLabel, remText)

		cdSummary := &UpstreamCooldownSummary{
			Scope:            scopeStr,
			ScopeLabel:       scopeLabel,
			ModelCount:       modelCount,
			Reason:           primaryReason,
			ReasonLabel:      reasonLabel,
			RemainingSeconds: minRemaining,
			RemainingText:    remText,
			ObservedAt:       time.Now().UnixMilli(),
			Reasons:          reasons,
			ModelKeys:        modelKeys,
		}

		return UpstreamAccountHealth{
			State:    "cooldown",
			Label:    "冷却中",
			Detail:   detail,
			Blocked:  true,
			Cooldown: cdSummary,
		}
	}

	statusLower := strings.ToLower(strings.TrimSpace(file.Status))
	msgLower := strings.ToLower(strings.TrimSpace(file.StatusMessage))

	if strings.Contains(statusLower, "expired") || strings.Contains(msgLower, "expired") || strings.Contains(msgLower, "过期") {
		return UpstreamAccountHealth{
			State:   "expired",
			Label:   "Token 已过期",
			Detail:  "CPA 报告 token expired，请重新授权或刷新凭据",
			Blocked: true,
		}
	}

	if file.Disabled || statusLower == "disabled" {
		return UpstreamAccountHealth{
			State:   "disabled",
			Label:   "已停用",
			Detail:  "CPA 已停用此凭据",
			Blocked: true,
		}
	}

	if statusLower == "error" || statusLower == "invalid" || statusLower == "failed" {
		return UpstreamAccountHealth{
			State:   "error",
			Label:   "凭据异常",
			Detail:  "CPA 报告凭据错误，请检查认证状态",
			Blocked: true,
		}
	}

	if file.Unavailable || statusLower == "unavailable" {
		return UpstreamAccountHealth{
			State:   "unavailable",
			Label:   "暂不可用",
			Detail:  "CPA 当前标记此凭据不可用",
			Blocked: true,
		}
	}

	return UpstreamAccountHealth{
		State:   "ready",
		Label:   "可用",
		Detail:  "",
		Blocked: false,
	}
}

func isGenericIdentifier(val, provider string) bool {
	v := strings.ToLower(strings.TrimSpace(val))
	if v == "" {
		return true
	}
	p := strings.ToLower(strings.TrimSpace(provider))
	return v == p || v == "default" || v == "account" || v == "user" || v == "api" || v == "auth"
}

func resolveDisplayName(file CpaAuthFile) string {
	nick := strings.TrimSpace(file.Nickname)
	if nick == "" {
		nick = strings.TrimSpace(file.NickName)
	}
	if nick != "" {
		return nick
	}

	rawLabel := strings.TrimSpace(file.Label)
	if rawLabel != "" && strings.Contains(rawLabel, "(") && strings.HasSuffix(rawLabel, ")") {
		start := strings.LastIndex(rawLabel, "(")
		end := strings.LastIndex(rawLabel, ")")
		if start >= 0 && end > start {
			extracted := strings.TrimSpace(rawLabel[start+1 : end])
			extLower := strings.ToLower(extracted)
			if extLower != "cn" && extLower != "intl" && extLower != "global" && extLower != "default" && extLower != "api" {
				return extracted
			}
		}
	}

	email := strings.TrimSpace(file.Email)
	cleanName := strings.TrimSuffix(strings.TrimSpace(file.Name), ".json")

	if email != "" && !isGenericIdentifier(email, file.Provider) && email != cleanName {
		return email
	}

	if rawLabel != "" && !isGenericIdentifier(rawLabel, file.Provider) && rawLabel != cleanName {
		return rawLabel
	}

	if cleanName != "" {
		return cleanName
	}

	return file.AuthIndex
}

// HandleGetUpstreamQuotas returns all upstream credentials and quota status
func HandleGetUpstreamQuotas(eng *engine.Engine, cpa *CpaClient, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		force := c.Query("refresh") == "1" || c.Request.Method == http.MethodPost

		if !force {
			upstreamCacheMu.RLock()
			if cachedUpstreamQuotasResp != nil && time.Since(cachedUpstreamAt) < 30*time.Second {
				resp := cachedUpstreamQuotasResp
				upstreamCacheMu.RUnlock()
				c.JSON(http.StatusOK, resp)
				return
			}
			upstreamCacheMu.RUnlock()
		}

		// 1. Fetch auth files from CPA loopback
		var authFilesResp CpaAuthFilesResponse
		_ = cpa.RequestJSON(ctx, http.MethodGet, "/auth-files", nil, "v0", &authFilesResp)

		// 2. Fetch openai-compatibility providers from CPA loopback
		var compatResp CpaOpenAICompatResponse
		_ = cpa.RequestJSON(ctx, http.MethodGet, "/openai-compatibility", nil, "v0", &compatResp)

		// 3. Fetch today's aggregated usage stats from SQLite
		dailyStats, err := eng.DB().SelectUpstreamDailyAgg(ctx)
		if err != nil {
			dailyStats = []db.UpstreamDailyStatRow{}
		}
		statsMap := make(map[string]db.UpstreamDailyStatRow)
		for _, s := range dailyStats {
			statsMap[s.AuthIndex] = s
		}

		// 4. Fetch pending validation challenges from SQLite
		pendingVals, err := eng.DB().SelectValidations(ctx, "pending")
		if err != nil {
			pendingVals = []db.UpstreamValidationRow{}
		}
		valMap := make(map[string]db.UpstreamValidationRow)
		for _, v := range pendingVals {
			valMap[v.AuthIndex] = v
		}

		nowMs := time.Now().UnixMilli()
		var accounts []UpstreamAccountItem

		// Process native auth-files
		for _, f := range authFilesResp.Files {
			fname := f.Name
			if fname == "" {
				fname = f.ID
			}
			prov := strings.ToLower(strings.TrimSpace(f.Provider))
			if prov == "" {
				prov = strings.ToLower(strings.TrimSpace(f.Type))
			}
			if prov == "" {
				prov = "unknown"
			}

			prio := defaultAuthPriority
			if f.Priority != nil && *f.Priority >= 0 && *f.Priority <= 100 {
				prio = *f.Priority
			}

			weight := defaultAuthWeight
			if f.Weight != nil && *f.Weight > 0 && *f.Weight <= 1000 {
				weight = *f.Weight
			}

			status := strings.TrimSpace(f.Status)
			if status == "" {
				if f.Disabled {
					status = "disabled"
				} else {
					status = "active"
				}
			}

			health := computeHealth(f)
			dispName := resolveDisplayName(f)

			email := strings.TrimSpace(f.Email)
			if email == "" || isGenericIdentifier(email, prov) {
				email = dispName
			}

			nick := strings.TrimSpace(f.Nickname)
			if nick == "" {
				nick = strings.TrimSpace(f.NickName)
			}

			planType := "free"
			planName := "免费版"
			var groups []CpaQuotaGroup
			if f.Quota != nil && len(f.Quota.Groups) > 0 {
				groups = f.Quota.Groups
				planType = "standard"
				planName = "标准包"
			}

			item := UpstreamAccountItem{
				ID:          fname,
				Filename:    fname,
				DisplayName: dispName,
				Nickname:    nick,
				Email:       email,
				Provider:    prov,
				Status:      status,
				Disabled:    f.Disabled || status == "disabled",
				Health:      health,
				Priority:    prio,
				Weight:      weight,
				Note:        f.Note,
				AuthIndex:   f.AuthIndex,
				PlanType:    planType,
				PlanName:    planName,
				Groups:      groups,
				UpdatedAt:   nowMs,
			}

			if stat, ok := statsMap[f.AuthIndex]; ok {
				item.TodayTokens = stat.TotalTokens
				item.TodayRequests = stat.Requests
			}

			if val, ok := valMap[f.AuthIndex]; ok {
				vCopy := val
				item.Validation = &vCopy
			}

			accounts = append(accounts, item)
		}

		// Process openai-compatibility providers (OpenCode, OpenRouter, StepFun, etc.)
		for _, comp := range compatResp.Compatibility {
			provName := strings.ToLower(strings.TrimSpace(comp.Name))
			for idx, keyEntry := range comp.APIKeyEntries {
				authIdx := keyEntry.AuthIndex
				if authIdx == "" {
					authIdx = fmt.Sprintf("%s_%d", provName, idx)
				}
				fname := fmt.Sprintf("%s-%s", provName, authIdx)

				prio := defaultAuthPriority
				if keyEntry.Priority != nil && *keyEntry.Priority >= 0 && *keyEntry.Priority <= 100 {
					prio = *keyEntry.Priority
				}

				weight := defaultAuthWeight
				if keyEntry.Weight != nil && *keyEntry.Weight > 0 && *keyEntry.Weight <= 1000 {
					weight = *keyEntry.Weight
				}

				status := "active"
				disabled := comp.Disabled
				if disabled {
					status = "disabled"
				}

				dispName := keyEntry.Note
				if dispName == "" {
					prefixLen := 8
					if len(authIdx) < prefixLen {
						prefixLen = len(authIdx)
					}
					dispName = fmt.Sprintf("%s (%s)", provName, authIdx[:prefixLen])
				}

				healthLabel := "正常"
				if disabled {
					healthLabel = "已停用"
				}

				item := UpstreamAccountItem{
					ID:          fname,
					Filename:    fname,
					DisplayName: dispName,
					Nickname:    "",
					Email:       maskSecret(keyEntry.APIKey),
					Provider:    provName,
					Status:      status,
					Disabled:    disabled,
					Health: UpstreamAccountHealth{
						State:   status,
						Label:   healthLabel,
						Detail:  "",
						Blocked: disabled,
					},
					Priority:  prio,
					Weight:    weight,
					Note:      keyEntry.Note,
					AuthIndex: authIdx,
					PlanType:  "standard",
					PlanName:  provName,
					Groups:    []CpaQuotaGroup{},
					UpdatedAt: nowMs,
				}

				if stat, ok := statsMap[authIdx]; ok {
					item.TodayTokens = stat.TotalTokens
					item.TodayRequests = stat.Requests
				}

				if val, ok := valMap[authIdx]; ok {
					vCopy := val
					item.Validation = &vCopy
				}

				accounts = append(accounts, item)
			}
		}

		// 5. If no accounts were discovered (e.g. CPA is offline), gracefully fallback to SQLite baselines
		if len(accounts) == 0 {
			baselines, err := eng.DB().SelectBaselinesAll(ctx)
			if err == nil {
				for _, b := range baselines {
					name := b.AuthIndex
					if b.Name != nil && *b.Name != "" {
						name = *b.Name
					}
					prov := ""
					if b.Provider != nil && *b.Provider != "" {
						prov = *b.Provider
					}
					provRes := providers.InferProvider("", b.AuthIndex)
					if prov == "" {
						prov = provRes.Key
					}
					item := UpstreamAccountItem{
						ID:          b.AuthIndex,
						Filename:    b.AuthIndex,
						DisplayName: name,
						Email:       name,
						Provider:    prov,
						Status:      "active",
						Disabled:    false,
						Priority:    defaultAuthPriority,
						Weight:      defaultAuthWeight,
						AuthIndex:   b.AuthIndex,
						PlanType:    "standard",
						PlanName:    prov,
						Groups:      []CpaQuotaGroup{},
						UpdatedAt:   nowMs,
					}
					if stat, ok := statsMap[b.AuthIndex]; ok {
						item.TodayTokens = stat.TotalTokens
						item.TodayRequests = stat.Requests
					}
					if val, ok := valMap[b.AuthIndex]; ok {
						vCopy := val
						item.Validation = &vCopy
					}
					accounts = append(accounts, item)
				}
			}
		}

		// 6. Merge local OpenRouter accounts from data/openrouter-accounts.json
		orAccounts := getOpenRouterAccountsList()
		for _, orAcc := range orAccounts {
			accStatus := "active"
			if !orAcc.Enabled {
				accStatus = "disabled"
			}
			item := UpstreamAccountItem{
				ID:          "openrouter-" + orAcc.ID,
				Filename:    "openrouter-" + orAcc.ID,
				DisplayName: orAcc.Name,
				Email:       orAcc.APIKeyMasked,
				Provider:    "openrouter",
				Status:      accStatus,
				Disabled:    !orAcc.Enabled,
				Priority:    orAcc.Priority,
				Weight:      orAcc.Weight,
				AuthIndex:   "openrouter-" + orAcc.ID,
				PlanType:    "standard",
				PlanName:    "OpenRouter",
				Groups:      []CpaQuotaGroup{},
				UpdatedAt:   nowMs,
			}
			accounts = append(accounts, item)
		}

		// Sort accounts: Priority DESC, then AuthIndex ASC
		sort.Slice(accounts, func(i, j int) bool {
			if accounts[i].Priority != accounts[j].Priority {
				return accounts[i].Priority > accounts[j].Priority
			}
			return accounts[i].AuthIndex < accounts[j].AuthIndex
		})

		response := &UpstreamQuotasResponse{
			Accounts:           accounts,
			PendingValidations: pendingVals,
			DefaultPriority:    defaultAuthPriority,
			LastUpdated:        nowMs,
			IsLoading:          false,
			Error:              nil,
		}

		upstreamCacheMu.Lock()
		cachedUpstreamQuotasResp = response
		cachedUpstreamAt = time.Now()
		upstreamCacheMu.Unlock()

		c.JSON(http.StatusOK, response)
	}
}

// HandlePutUpstreamAuthPriority updates priority, weight, and note for a credential
func HandlePutUpstreamAuthPriority(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Name     string `json:"name"`
			Priority int    `json:"priority"`
			Weight   *int   `json:"weight"`
			Note     string `json:"note"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		name := strings.TrimSpace(payload.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "凭证名称 name 不能为空"})
			return
		}

		if payload.Priority < 0 || payload.Priority > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "优先级 priority 必须在 0 到 100 之间"})
			return
		}

		weight := defaultAuthWeight
		if payload.Weight != nil {
			if *payload.Weight < 1 || *payload.Weight > 1000 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "权重 weight 必须在 1 到 1000 之间"})
				return
			}
			weight = *payload.Weight
		}

		ctx := c.Request.Context()
		bodyBytes, _ := json.Marshal(map[string]interface{}{
			"name":     name,
			"priority": payload.Priority,
			"weight":   weight,
			"note":     payload.Note,
		})

		if !strings.HasPrefix(name, "openrouter-") {
			_, _ = cpa.Request(ctx, http.MethodPatch, "/auth-files/fields", bodyBytes, "application/json", "v0")
		}

		InvalidateUpstreamQuotaCache()

		c.JSON(http.StatusOK, gin.H{
			"status":   "ok",
			"name":     name,
			"priority": payload.Priority,
			"weight":   weight,
		})
	}
}

// HandlePutUpstreamAuthStatus enables or disables an upstream credential
func HandlePutUpstreamAuthStatus(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Name     string `json:"name"`
			Disabled bool   `json:"disabled"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		name := strings.TrimSpace(payload.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "凭证名称 name 不能为空"})
			return
		}

		ctx := c.Request.Context()
		bodyBytes, _ := json.Marshal(map[string]interface{}{
			"name":     name,
			"disabled": payload.Disabled,
		})

		if !strings.HasPrefix(name, "openrouter-") {
			_, _ = cpa.Request(ctx, http.MethodPatch, "/auth-files/status", bodyBytes, "application/json", "v0")
		}

		InvalidateUpstreamQuotaCache()

		statusStr := "active"
		if payload.Disabled {
			statusStr = "disabled"
		}

		c.JSON(http.StatusOK, gin.H{
			"status":      "ok",
			"name":        name,
			"disabled":    payload.Disabled,
			"auth_status": statusStr,
		})
	}
}

// HandlePostUpstreamAuthActivateWindow triggers on-demand quota probe for an upstream credential
func HandlePostUpstreamAuthActivateWindow(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Filename  string `json:"filename"`
			Provider  string `json:"provider"`
			AuthIndex string `json:"authIndex"`
			AuthIdx   string `json:"auth_index"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		authIdx := strings.TrimSpace(payload.AuthIndex)
		if authIdx == "" {
			authIdx = strings.TrimSpace(payload.AuthIdx)
		}
		fname := strings.TrimSpace(payload.Filename)
		prov := strings.ToLower(strings.TrimSpace(payload.Provider))

		if authIdx == "" && fname == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "必须指定上游凭证 auth_index 或 filename"})
			return
		}

		ctx := c.Request.Context()
		if authIdx != "" {
			resetBody, _ := json.Marshal(map[string]string{"auth_index": authIdx})
			_, _ = cpa.Request(ctx, http.MethodPost, "/reset-quota", resetBody, "application/json", "v0")
		}
		if fname != "" {
			refreshBody, _ := json.Marshal(map[string]string{"name": fname})
			_, _ = cpa.Request(ctx, http.MethodPost, "/auth-files/refresh", refreshBody, "application/json", "v0")
		}

		if prov == "workbuddy" || prov == "workbuddy-cn" {
			reqBody, _ := json.Marshal(map[string]string{"authIndex": authIdx})
			_, _ = cpa.Request(ctx, http.MethodPost, "/quota/fetch", reqBody, "application/json", "v0")
		} else if prov == "codex" {
			apiCallBody, _ := json.Marshal(map[string]interface{}{
				"authIndex": authIdx,
				"method":    "GET",
				"url":       "https://chatgpt.com/backend-api/wham/usage",
				"header": map[string]string{
					"Authorization": "Bearer $TOKEN$",
					"Content-Type":  "application/json",
					"User-Agent":    "codex-tui/0.149.1 (Mac OS 24.1.0; arm64)",
				},
			})
			_, _ = cpa.Request(ctx, http.MethodPost, "/api-call", apiCallBody, "application/json", "v0")
		} else if prov == "antigravity" {
			apiCallBody, _ := json.Marshal(map[string]interface{}{
				"authIndex": authIdx,
				"method":    "POST",
				"url":       "https://daily-cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary",
				"header": map[string]string{
					"Authorization": "Bearer $TOKEN$",
					"Content-Type":  "application/json",
					"User-Agent":    "antigravity/cli/1.3.6 (aidev_client; os_type=darwin; arch=arm64)",
				},
				"data": `{"project":"aicode-consumers"}`,
			})
			_, _ = cpa.Request(ctx, http.MethodPost, "/api-call", apiCallBody, "application/json", "v0")
		}

		InvalidateUpstreamQuotaCache()

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "已向上游查询额度；5 小时窗口由实际模型请求决定",
		})
	}
}
