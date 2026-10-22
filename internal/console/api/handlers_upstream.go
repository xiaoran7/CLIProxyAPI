package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/middleware"
)

// HandleGetModelPrices returns all model pricing rules.
func HandleGetModelPrices(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		prices, err := eng.DB().SelectAllModelPrices(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query model prices: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"prices": prices})
	}
}

// UpsertModelPricePayload defines the payload for creating or modifying a model price.
type UpsertModelPricePayload struct {
	Pattern         string   `json:"pattern"`
	ModelPattern    string   `json:"model_pattern"`
	PromptPrice     *float64 `json:"prompt_price"`
	CompletionPrice *float64 `json:"completion_price"`
	CacheReadPrice  *float64 `json:"cache_read_price"`
	Multiplier      *float64 `json:"multiplier"`
}

// HandleUpsertModelPrice inserts or updates a model pricing rule.
func HandleUpsertModelPrice(eng *engine.Engine, pricing *PricingEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload UpsertModelPricePayload
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request payload: %v", err)})
			return
		}

		pat := strings.TrimSpace(payload.Pattern)
		if pat == "" {
			pat = strings.TrimSpace(payload.ModelPattern)
		}
		if pat == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pattern is required"})
			return
		}

		promptPrice := 0.50
		if payload.PromptPrice != nil && *payload.PromptPrice >= 0 {
			promptPrice = *payload.PromptPrice
		}
		completionPrice := 1.50
		if payload.CompletionPrice != nil && *payload.CompletionPrice >= 0 {
			completionPrice = *payload.CompletionPrice
		}
		cacheReadPrice := 0.10
		if payload.CacheReadPrice != nil && *payload.CacheReadPrice >= 0 {
			cacheReadPrice = *payload.CacheReadPrice
		}
		multiplier := 1.0
		if payload.Multiplier != nil && *payload.Multiplier > 0 {
			multiplier = *payload.Multiplier
		}

		ctx := c.Request.Context()
		if err := eng.DB().UpsertModelPrice(ctx, pat, promptPrice, completionPrice, cacheReadPrice, multiplier); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to upsert model price: %v", err)})
			return
		}

		_ = pricing.InitAndReload(ctx)

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// HandleDeleteModelPrice deletes a model pricing rule by ID.
func HandleDeleteModelPrice(eng *engine.Engine, pricing *PricingEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid price id"})
			return
		}

		ctx := c.Request.Context()
		if err := eng.DB().DeleteModelPrice(ctx, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to delete model price: %v", err)})
			return
		}

		_ = pricing.InitAndReload(ctx)

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// HandleGetProviderRegistry returns all AI provider registry entries.
func HandleGetProviderRegistry(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := eng.DB().SelectProviderRegistry(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query provider registry: %v", err)})
			return
		}

		type ProviderItemResponse struct {
			ProviderKey   string        `json:"provider_key"`
			Label         string        `json:"label"`
			ModelPatterns []PatternItem `json:"model_patterns"`
			AuthPatterns  []PatternItem `json:"auth_patterns"`
			Source        string        `json:"source"`
			Enabled       bool          `json:"enabled"`
			SortOrder     int           `json:"sort_order"`
			CreatedAt     string        `json:"created_at"`
			UpdatedAt     string        `json:"updated_at"`
		}

		var items []ProviderItemResponse
		for _, r := range rows {
			var mps []PatternItem
			if r.ModelPatterns != "" {
				_ = json.Unmarshal([]byte(r.ModelPatterns), &mps)
			}
			var aps []PatternItem
			if r.AuthPatterns != "" {
				_ = json.Unmarshal([]byte(r.AuthPatterns), &aps)
			}

			items = append(items, ProviderItemResponse{
				ProviderKey:   r.ProviderKey,
				Label:         r.Label,
				ModelPatterns: mps,
				AuthPatterns:  aps,
				Source:        r.Source,
				Enabled:       r.Enabled == 1,
				SortOrder:     r.SortOrder,
				CreatedAt:     r.CreatedAt.Format("2006-01-02 15:04:05"),
				UpdatedAt:     r.UpdatedAt.Format("2006-01-02 15:04:05"),
			})
		}

		c.JSON(http.StatusOK, gin.H{"providers": items})
	}
}

// UpsertProviderPayload defines the JSON body for upserting a provider entry.
type UpsertProviderPayload struct {
	ProviderKey   string      `json:"provider_key"`
	Label         string      `json:"label"`
	ModelPatterns interface{} `json:"model_patterns"`
	AuthPatterns  interface{} `json:"auth_patterns"`
	Source        string      `json:"source"`
	Enabled       *bool       `json:"enabled"`
	SortOrder     *int        `json:"sort_order"`
}

// HandleUpsertProviderRegistry creates or updates an AI provider registry entry.
func HandleUpsertProviderRegistry(eng *engine.Engine, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload UpsertProviderPayload
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request payload: %v", err)})
			return
		}

		key := strings.ToLower(strings.TrimSpace(payload.ProviderKey))
		label := strings.TrimSpace(payload.Label)

		if err := ValidateProviderKey(key); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if label == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "label cannot be empty"})
			return
		}

		source := strings.TrimSpace(payload.Source)
		if source == "" {
			source = "manual"
		}

		enabledInt := 1
		if payload.Enabled != nil && !*payload.Enabled {
			enabledInt = 0
		}

		sortOrder := 100
		if payload.SortOrder != nil && *payload.SortOrder > 0 {
			sortOrder = *payload.SortOrder
		}

		mpStr := "[]"
		if payload.ModelPatterns != nil {
			b, _ := json.Marshal(payload.ModelPatterns)
			mpStr = string(b)
		}

		apStr := "[]"
		if payload.AuthPatterns != nil {
			b, _ := json.Marshal(payload.AuthPatterns)
			apStr = string(b)
		}

		ctx := c.Request.Context()
		row := &db.ProviderRegistryRow{
			ProviderKey:   key,
			Label:         label,
			ModelPatterns: mpStr,
			AuthPatterns:  apStr,
			Source:        source,
			Enabled:       enabledInt,
			SortOrder:     sortOrder,
		}

		if err := eng.DB().UpsertProviderRegistry(ctx, row); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to upsert provider: %v", err)})
			return
		}

		_ = providers.InitAndReload(ctx)

		c.JSON(http.StatusOK, gin.H{
			"success":      true,
			"provider_key": key,
		})
	}
}

// HandleDeleteProviderRegistry deletes a custom provider from the registry.
func HandleDeleteProviderRegistry(eng *engine.Engine, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.ToLower(strings.TrimSpace(c.Param("key")))
		if key == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provider_key is required"})
			return
		}

		ctx := c.Request.Context()
		if err := eng.DB().DeleteProviderRegistry(ctx, key); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to delete provider: %v", err)})
			return
		}

		_ = providers.InitAndReload(ctx)

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// HandleGetValidations returns pending and historical upstream validation challenges.
func HandleGetValidations(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		database := eng.DB()

		pending, err := database.SelectValidations(ctx, "pending")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query pending validations: %v", err)})
			return
		}

		history, err := database.SelectValidations(ctx, "")
		if err != nil {
			history = []db.UpstreamValidationRow{}
		}

		seen := make(map[int64]bool)
		var combined []db.UpstreamValidationRow

		for _, item := range pending {
			seen[item.ID] = true
			combined = append(combined, item)
		}
		for _, item := range history {
			if !seen[item.ID] {
				seen[item.ID] = true
				combined = append(combined, item)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"validations":   combined,
			"pending_count": len(pending),
		})
	}
}

// HandleResolveValidation marks an upstream validation challenge as resolved.
func HandleResolveValidation(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid validation id"})
			return
		}

		if err := eng.DB().ResolveValidation(c.Request.Context(), id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to resolve validation: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// HandleDismissValidation marks an upstream validation challenge as dismissed.
func HandleDismissValidation(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid validation id"})
			return
		}

		if err := eng.DB().DismissValidation(c.Request.Context(), id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to dismiss validation: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// HandleDeleteValidation permanently removes a validation challenge.
func HandleDeleteValidation(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid validation id"})
			return
		}

		if err := eng.DB().DeleteValidation(c.Request.Context(), id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to delete validation: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// HandleManualValidation allows manual insertion of an upstream validation challenge URL.
func HandleManualValidation(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			AuthIndex     string `json:"auth_index"`
			Provider      string `json:"provider"`
			AccountName   string `json:"account_name"`
			ValidationURL string `json:"validation_url"`
			ErrorMessage  string `json:"error_message"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		rawURL := strings.TrimSpace(payload.ValidationURL)
		if rawURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "validation_url is required"})
			return
		}

		valURL := middleware.ExtractGoogleValidationURL(rawURL)
		if valURL == "" {
			valURL = rawURL
		}
		if !strings.Contains(valURL, "accounts.google.com") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Please provide a valid Google validation URL (must contain accounts.google.com)"})
			return
		}

		authIndex := strings.TrimSpace(payload.AuthIndex)
		if authIndex == "" {
			authIndex = "manual"
		}
		provider := strings.TrimSpace(payload.Provider)
		if provider == "" {
			provider = "antigravity"
		}
		acctName := strings.TrimSpace(payload.AccountName)
		if acctName == "" {
			acctName = "Manual Verification"
		}
		errMsg := strings.TrimSpace(payload.ErrorMessage)
		if errMsg == "" {
			errMsg = "Manual entry validation challenge"
		}

		now := time.Now().Unix()
		row := &db.UpstreamValidationRow{
			TS:            now,
			AuthIndex:     authIndex,
			Provider:      provider,
			AccountName:   &acctName,
			ValidationURL: valURL,
			ErrorMessage:  &errMsg,
			Status:        "pending",
			UpdatedAt:     now,
		}

		id, err := eng.DB().InsertValidation(c.Request.Context(), row)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to record validation: %v", err)})
			return
		}

		row.ID = id
		c.JSON(http.StatusOK, gin.H{
			"success":        true,
			"record":         row,
			"validation_url": valURL,
		})
	}
}

// HandleGetQuotaSnapshots returns historical quota window percentages.
func HandleGetQuotaSnapshots(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		authIndex := strings.TrimSpace(c.Query("auth_index"))
		hoursStr := c.DefaultQuery("hours", "24")
		hours, err := strconv.Atoi(hoursStr)
		if err != nil || hours <= 0 {
			hours = 24
		}

		sinceTS := time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
		snapshots, err := eng.DB().SelectQuotaSnapshotsByAuth(c.Request.Context(), authIndex, sinceTS)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query quota snapshots: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"snapshots":  snapshots,
			"auth_index": authIndex,
			"hours":      hours,
		})
	}
}

// AuthStatQuotaInfo describes 5-hour or weekly quota limit for a credential.
type AuthStatQuotaInfo struct {
	Name             string   `json:"name"`
	GroupName        string   `json:"group_name"`
	RemainingPercent float64  `json:"remaining_percent"`
	UsedPercent      *float64 `json:"used_percent,omitempty"`
	ResetTime        string   `json:"reset_time,omitempty"`
	ResetCountdown   string   `json:"reset_countdown,omitempty"`
}

// AuthStatPeriodStats holds segmented token and request counts (5h, 24h, all).
type AuthStatPeriodStats struct {
	Tokens5h    int64 `json:"tokens_5h"`
	Tokens24h   int64 `json:"tokens_24h"`
	TokensAll   int64 `json:"tokens_all"`
	Requests5h  int64 `json:"requests_5h"`
	Requests24h int64 `json:"requests_24h"`
	RequestsAll int64 `json:"requests_all"`
}

// AuthStatResponseItem represents telemetry and quota status for an upstream credential.
type AuthStatResponseItem struct {
	AuthIndex         string              `json:"auth_index"`
	Name              string              `json:"name"`
	DisplayName       string              `json:"display_name"`
	Nickname          string              `json:"nickname"`
	Email             string              `json:"email"`
	Provider          string              `json:"provider"`
	ProviderLabel     string              `json:"provider_label"`
	Priority          int                 `json:"priority"`
	Status            string              `json:"status"`
	Disabled          bool                `json:"disabled"`
	IsDeleted         bool                `json:"is_deleted"`
	IsPlaceholderName bool                `json:"is_placeholder_name"`
	RuntimeSource     string              `json:"runtime_source"`
	LastModel         string              `json:"last_model"`
	TotalRequests     int64               `json:"total_requests"`
	Requests          int64               `json:"requests"`
	SuccessCount      int64               `json:"success_count"`
	FailureCount      int64               `json:"failure_count"`
	SuccessRate       float64             `json:"success_rate"`
	PromptTokens      int64               `json:"prompt_tokens"`
	CompletionTokens  int64               `json:"completion_tokens"`
	CachedTokens      int64               `json:"cached_tokens"`
	TotalTokens       int64               `json:"total_tokens"`
	CacheRate         float64             `json:"cache_rate"`
	Quota5h           *AuthStatQuotaInfo  `json:"quota_5h,omitempty"`
	QuotaWeekly       *AuthStatQuotaInfo  `json:"quota_weekly,omitempty"`
	PeriodStats       AuthStatPeriodStats `json:"period_stats"`
	TotalCredits      *float64            `json:"total_credits,omitempty"`
	RemainingCredits  *float64            `json:"remaining_credits,omitempty"`
	UsedCredits       *float64            `json:"used_credits,omitempty"`
	ExpiresAt         string              `json:"expires_at,omitempty"`
	ResetTime         string              `json:"reset_time,omitempty"`
	ResetCountdown    string              `json:"reset_countdown,omitempty"`
	TodayTokens       int64               `json:"today_tokens"`
	TodayRequests     int64               `json:"today_requests"`
	DailyLimit        *int64              `json:"daily_limit,omitempty"`
	RemainingRequests *int64              `json:"remaining_requests,omitempty"`
}

// HandleGetUpstreamAuthStats aggregates credentials statistics with optional time range filtering.
func HandleGetUpstreamAuthStats(eng *engine.Engine, cpa *CpaClient, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		database := eng.DB()

		rangeParam := strings.ToLower(strings.TrimSpace(c.Query("range")))
		if rangeParam == "" {
			rangeParam = strings.ToLower(strings.TrimSpace(c.Query("time_range")))
		}
		if rangeParam == "" {
			rangeParam = "all"
		}

		now := time.Now()

		// 1. Fetch active accounts from CPA and SQLite (fallback)
		var activeAccounts []UpstreamAccountItem
		if quotasResp, err := FetchUpstreamAccounts(ctx, eng, cpa, providers, false); err == nil && quotasResp != nil {
			activeAccounts = quotasResp.Accounts
		}

		activeMap := make(map[string]UpstreamAccountItem)
		for _, acc := range activeAccounts {
			if acc.AuthIndex != "" {
				activeMap[acc.AuthIndex] = acc
			}
			if acc.Filename != "" && acc.Filename != acc.AuthIndex {
				activeMap[acc.Filename] = acc
			}
			if acc.ID != "" && acc.ID != acc.AuthIndex && acc.ID != acc.Filename {
				activeMap[acc.ID] = acc
			}
		}

		// 2. Fetch baselines from SQLite
		baselines, err := database.SelectBaselinesAll(ctx)
		if err != nil {
			baselines = []db.UpstreamAuthBaselineRow{}
		}
		baseMap := make(map[string]db.UpstreamAuthBaselineRow)
		for _, b := range baselines {
			baseMap[b.AuthIndex] = b
		}

		// 3. Fetch period stats (5h and 24h) from request_events
		since5h := now.Add(-5 * time.Hour).Unix()
		win5h, _ := database.SelectUpstreamWindowStats(ctx, since5h)
		since24h := now.Add(-24 * time.Hour).Unix()
		win24h, _ := database.SelectUpstreamWindowStats(ctx, since24h)

		// 4. Fetch latest model per auth_index
		latestModels, _ := database.SelectLatestModelByAuth(ctx)

		// 5. Query traffic metrics based on rangeParam
		type trafficMetrics struct {
			requests         int64
			successCount     int64
			failureCount     int64
			promptTokens     int64
			completionTokens int64
			cachedTokens     int64
			totalTokens      int64
		}
		trafficMap := make(map[string]trafficMetrics)
		isAll := rangeParam == "all"
		runtimeSource := "cpa_runtime"

		if rangeParam == "5h" {
			if evs, err := database.SelectUpstreamEventAggSince(ctx, since5h); err == nil {
				for _, ev := range evs {
					trafficMap[ev.AuthIndex] = trafficMetrics{
						requests:         ev.Requests,
						successCount:     ev.SuccessCount,
						failureCount:     ev.FailureCount,
						promptTokens:     ev.PromptTokens,
						completionTokens: ev.CompletionTokens,
						cachedTokens:     ev.CachedTokens,
						totalTokens:      ev.TotalTokens,
					}
				}
			}
		} else if rangeParam == "24h" || rangeParam == "1d" || rangeParam == "today" {
			if evs, err := database.SelectUpstreamEventAggSince(ctx, since24h); err == nil {
				for _, ev := range evs {
					trafficMap[ev.AuthIndex] = trafficMetrics{
						requests:         ev.Requests,
						successCount:     ev.SuccessCount,
						failureCount:     ev.FailureCount,
						promptTokens:     ev.PromptTokens,
						completionTokens: ev.CompletionTokens,
						cachedTokens:     ev.CachedTokens,
						totalTokens:      ev.TotalTokens,
					}
				}
			}
		} else if rangeParam == "7d" {
			sinceDate := now.AddDate(0, 0, -7).Format("2006-01-02")
			if dailyAgg, err := database.SelectUpstreamDailyAggSince(ctx, sinceDate); err == nil {
				for _, d := range dailyAgg {
					trafficMap[d.AuthIndex] = trafficMetrics{
						requests:         d.Requests,
						successCount:     d.SuccessCount,
						failureCount:     d.FailureCount,
						promptTokens:     d.PromptTokens,
						completionTokens: d.CompletionTokens,
						cachedTokens:     d.CachedTokens,
						totalTokens:      d.TotalTokens,
					}
				}
			}
		} else if rangeParam == "30d" {
			sinceDate := now.AddDate(0, 0, -30).Format("2006-01-02")
			if dailyAgg, err := database.SelectUpstreamDailyAggSince(ctx, sinceDate); err == nil {
				for _, d := range dailyAgg {
					trafficMap[d.AuthIndex] = trafficMetrics{
						requests:         d.Requests,
						successCount:     d.SuccessCount,
						failureCount:     d.FailureCount,
						promptTokens:     d.PromptTokens,
						completionTokens: d.CompletionTokens,
						cachedTokens:     d.CachedTokens,
						totalTokens:      d.TotalTokens,
					}
				}
			}
		} else { // "all"
			runtimeSource = "local_estimate"
			if dailyAgg, err := database.SelectUpstreamDailyAgg(ctx); err == nil {
				for _, d := range dailyAgg {
					trafficMap[d.AuthIndex] = trafficMetrics{
						requests:         d.Requests,
						successCount:     d.SuccessCount,
						failureCount:     d.FailureCount,
						promptTokens:     d.PromptTokens,
						completionTokens: d.CompletionTokens,
						cachedTokens:     d.CachedTokens,
						totalTokens:      d.TotalTokens,
					}
				}
			}
		}

		// 6. Gather all unique auth_index keys
		allIndexesSet := make(map[string]struct{})
		for _, acc := range activeAccounts {
			if acc.AuthIndex != "" {
				allIndexesSet[acc.AuthIndex] = struct{}{}
			}
		}
		for idx := range baseMap {
			if idx != "" {
				allIndexesSet[idx] = struct{}{}
			}
		}
		for idx := range trafficMap {
			if idx != "" {
				allIndexesSet[idx] = struct{}{}
			}
		}
		for idx := range win5h {
			if idx != "" {
				allIndexesSet[idx] = struct{}{}
			}
		}
		for idx := range win24h {
			if idx != "" {
				allIndexesSet[idx] = struct{}{}
			}
		}

		hasUpstreamCatalog := len(activeAccounts) > 0
		var items []AuthStatResponseItem

		for authIndex := range allIndexesSet {
			var acc *UpstreamAccountItem
			if a, ok := activeMap[authIndex]; ok {
				acc = &a
			} else {
				for _, a := range activeAccounts {
					if a.AuthIndex == authIndex || a.Filename == authIndex || a.ID == authIndex {
						acc = &a
						break
					}
				}
			}

			base, hasBase := baseMap[authIndex]
			traf := trafficMap[authIndex]

			isDeleted := false
			if hasUpstreamCatalog {
				isDeleted = (acc == nil)
			} else {
				isDeleted = (acc == nil && !hasBase)
			}

			displayName := ""
			nickname := ""
			email := ""
			provider := ""
			priority := 10
			status := "active"
			disabled := false

			if acc != nil {
				displayName = acc.DisplayName
				if displayName == "" {
					displayName = acc.Filename
				}
				nickname = acc.Nickname
				email = acc.Email
				provider = acc.Provider
				priority = acc.Priority
				status = acc.Status
				disabled = acc.Disabled
			} else if hasBase {
				if base.Name != nil && *base.Name != "" {
					displayName = *base.Name
				}
				if base.Provider != nil && *base.Provider != "" {
					provider = *base.Provider
				}
			}

			if acc == nil {
				if regAcc, ok := providers.GetAuthAccount(authIndex); ok {
					if displayName == "" && regAcc.Name != "" {
						displayName = regAcc.Name
					}
					if provider == "" && regAcc.ProviderKey != "" {
						provider = regAcc.ProviderKey
					}
				}
			}

			if provider == "" {
				provRes := providers.InferProvider("", authIndex)
				if provRes.Key != "" {
					provider = provRes.Key
				} else if lm, ok := latestModels[authIndex]; ok && lm != "" {
					provResModel := providers.InferProvider(lm, "")
					if provResModel.Key != "" {
						provider = provResModel.Key
					}
				}
			}
			if provider == "" {
				provider = "other"
			}

			provLabel := providers.GetProviderLabel(provider)
			if provLabel == "" {
				provLabel = UnknownProviderLabel
			}

			isPlaceholderName := false
			if displayName == "" {
				isPlaceholderName = true
				if len(authIndex) >= 8 {
					displayName = "已删除账号 " + authIndex[:8]
				} else {
					displayName = "已删除账号 " + authIndex
				}
			} else if strings.HasPrefix(displayName, "已删除账号") || displayName == authIndex {
				isPlaceholderName = true
			}

			// Compute request and token counts
			var totalReqs, succCount, failCount, pTokens, cTokens, cachedToks, totTokens int64
			if isAll {
				if hasBase {
					totalReqs += base.BaseRequests
					succCount += base.BaseSuccessCount
					failCount += base.BaseFailureCount
					pTokens += base.BasePromptTokens
					cTokens += base.BaseCompletionTokens
					cachedToks += base.BaseCachedTokens
					totTokens += base.BaseTotalTokens
				}
				totalReqs += traf.requests
				succCount += traf.successCount
				failCount += traf.failureCount
				pTokens += traf.promptTokens
				cTokens += traf.completionTokens
				cachedToks += traf.cachedTokens
				totTokens += traf.totalTokens
			} else {
				totalReqs = traf.requests
				succCount = traf.successCount
				failCount = traf.failureCount
				pTokens = traf.promptTokens
				cTokens = traf.completionTokens
				cachedToks = traf.cachedTokens
				totTokens = traf.totalTokens
			}

			successRate := 100.0
			if totalReqs > 0 {
				successRate = math.Round((float64(succCount)/float64(totalReqs))*10000) / 100
			}

			promptBase := pTokens
			if cachedToks > promptBase {
				promptBase = cachedToks
			}
			cacheRate := 0.0
			if promptBase > 0 {
				cacheRate = math.Round((float64(cachedToks)/float64(promptBase))*10000) / 100
			}

			// Extract Quota 5h and Quota Weekly from acc.Groups
			var quota5h *AuthStatQuotaInfo
			var quotaWeekly *AuthStatQuotaInfo
			var totalCredits, remainingCredits, usedCredits *float64
			var expiresAt string
			var resetTimeStr, resetCountdownStr string

			if acc != nil {
				for _, grp := range acc.Groups {
					gNameLower := strings.ToLower(grp.DisplayName + " " + grp.Name)
					isClaudeGroup := strings.Contains(gNameLower, "claude") || strings.Contains(gNameLower, "gpt")
					for _, bkt := range grp.Buckets {
						wLower := strings.ToLower(bkt.Window + " " + bkt.Name + " " + bkt.BucketID)
						is5h := bkt.Window == "5h" || strings.Contains(wLower, "5h") || strings.Contains(wLower, "five")
						isWeek := bkt.Window == "weekly" || strings.Contains(wLower, "week")

						if is5h && bkt.RemainingPercent != nil {
							remPct := float64(*bkt.RemainingPercent)
							bktName := bkt.Name
							if bktName == "" {
								if isClaudeGroup {
									bktName = "Claude 5 小时周期"
								} else {
									bktName = "5 小时周期"
								}
							}
							if quota5h == nil || remPct < quota5h.RemainingPercent || (remPct == quota5h.RemainingPercent && isClaudeGroup) {
								quota5h = &AuthStatQuotaInfo{
									Name:             bktName,
									GroupName:        grp.DisplayName,
									RemainingPercent: remPct,
									UsedPercent:      bkt.UsedPercent,
									ResetTime:        bkt.ResetTime,
									ResetCountdown:   bkt.ResetCountdown,
								}
							}
						}

						if isWeek && bkt.RemainingPercent != nil {
							remPct := float64(*bkt.RemainingPercent)
							bktName := bkt.Name
							if bktName == "" {
								if isClaudeGroup {
									bktName = "Claude 周限额"
								} else {
									bktName = "周限额"
								}
							}
							if quotaWeekly == nil || remPct < quotaWeekly.RemainingPercent || (remPct == quotaWeekly.RemainingPercent && isClaudeGroup) {
								quotaWeekly = &AuthStatQuotaInfo{
									Name:             bktName,
									GroupName:        grp.DisplayName,
									RemainingPercent: remPct,
									UsedPercent:      bkt.UsedPercent,
									ResetTime:        bkt.ResetTime,
									ResetCountdown:   bkt.ResetCountdown,
								}
							}
						}

						// Accumulate credits for WorkBuddy
						if bkt.TotalCapacity != nil || bkt.RemainCapacity != nil {
							if totalCredits == nil {
								zero := 0.0
								totalCredits = &zero
								remainingCredits = &zero
								usedCredits = &zero
							}
							if bkt.TotalCapacity != nil {
								*totalCredits += *bkt.TotalCapacity
							}
							if bkt.RemainCapacity != nil {
								*remainingCredits += *bkt.RemainCapacity
							}
							if bkt.UsedCapacity != nil {
								*usedCredits += *bkt.UsedCapacity
							}
						}

						if bkt.ExpiresAt != "" && expiresAt == "" {
							expiresAt = bkt.ExpiresAt
						}
						if bkt.ResetTime != "" && resetTimeStr == "" {
							resetTimeStr = bkt.ResetTime
							resetCountdownStr = bkt.ResetCountdown
						}
					}
				}
			}

			// Period stats
			w5 := win5h[authIndex]
			w24 := win24h[authIndex]
			periodStats := AuthStatPeriodStats{
				Tokens5h:    w5.Tokens,
				Requests5h:  w5.Requests,
				Tokens24h:   w24.Tokens,
				Requests24h: w24.Requests,
				TokensAll:   totTokens,
				RequestsAll: totalReqs,
			}

			// OpenRouter & OpenCode metrics
			var todayToks, todayReqs int64
			var dailyLim, remReqs *int64
			if acc != nil {
				todayToks = acc.TodayTokens
				todayReqs = acc.TodayRequests
			}
			if strings.Contains(strings.ToLower(provider), "openrouter") {
				lim := int64(800)
				dailyLim = &lim
				rem := lim - todayReqs
				if rem < 0 {
					rem = 0
				}
				remReqs = &rem
			}

			lastModel := latestModels[authIndex]

			items = append(items, AuthStatResponseItem{
				AuthIndex:         authIndex,
				Name:              displayName,
				DisplayName:       displayName,
				Nickname:          nickname,
				Email:             email,
				Provider:          provider,
				ProviderLabel:     provLabel,
				Priority:          priority,
				Status:            status,
				Disabled:          disabled,
				IsDeleted:         isDeleted,
				IsPlaceholderName: isPlaceholderName,
				RuntimeSource:     runtimeSource,
				LastModel:         lastModel,
				TotalRequests:     totalReqs,
				Requests:          totalReqs,
				SuccessCount:      succCount,
				FailureCount:      failCount,
				SuccessRate:       successRate,
				PromptTokens:      pTokens,
				CompletionTokens:  cTokens,
				CachedTokens:      cachedToks,
				TotalTokens:       totTokens,
				CacheRate:         cacheRate,
				Quota5h:           quota5h,
				QuotaWeekly:       quotaWeekly,
				PeriodStats:       periodStats,
				TotalCredits:      totalCredits,
				RemainingCredits:  remainingCredits,
				UsedCredits:       usedCredits,
				ExpiresAt:         expiresAt,
				ResetTime:         resetTimeStr,
				ResetCountdown:    resetCountdownStr,
				TodayTokens:       todayToks,
				TodayRequests:     todayReqs,
				DailyLimit:        dailyLim,
				RemainingRequests: remReqs,
			})
		}

		// Sort items: Active first (by Priority DESC, then TotalRequests DESC, then TotalTokens DESC),
		// Deleted items sorted by TotalRequests DESC
		sort.Slice(items, func(i, j int) bool {
			if items[i].IsDeleted != items[j].IsDeleted {
				return !items[i].IsDeleted // active items first
			}
			if !items[i].IsDeleted {
				if items[i].Priority != items[j].Priority {
					return items[i].Priority > items[j].Priority
				}
			}
			if items[i].TotalRequests != items[j].TotalRequests {
				return items[i].TotalRequests > items[j].TotalRequests
			}
			return items[i].TotalTokens > items[j].TotalTokens
		})

		// Summary calculations
		var sumRequests, sumSuccess, sumFailure, sumPrompt, sumCompletion, sumCached, sumTokens int64
		for _, it := range items {
			sumRequests += it.TotalRequests
			sumSuccess += it.SuccessCount
			sumFailure += it.FailureCount
			sumPrompt += it.PromptTokens
			sumCompletion += it.CompletionTokens
			sumCached += it.CachedTokens
			sumTokens += it.TotalTokens
		}

		overallPromptBase := sumPrompt
		if sumCached > overallPromptBase {
			overallPromptBase = sumCached
		}
		overallCacheRate := 0.0
		if overallPromptBase > 0 {
			overallCacheRate = math.Round((float64(sumCached)/float64(overallPromptBase))*10000) / 100
		}
		overallSuccessRate := 100.0
		if sumRequests > 0 {
			overallSuccessRate = math.Round((float64(sumSuccess)/float64(sumRequests))*10000) / 100
		}

		summary := gin.H{
			"total_accounts":    len(items),
			"total_requests":    sumRequests,
			"total_success":     sumSuccess,
			"total_failure":     sumFailure,
			"success_rate":      overallSuccessRate,
			"prompt_tokens":     sumPrompt,
			"completion_tokens": sumCompletion,
			"cached_tokens":     sumCached,
			"total_tokens":      sumTokens,
			"cache_rate":        overallCacheRate,
			"request_source":    runtimeSource,
			"token_source":      "local_metered",
		}

		c.JSON(http.StatusOK, gin.H{
			"time_range": rangeParam,
			"items":      items,
			"auth_stats": items,
			"total":      len(items),
			"summary":    summary,
			"source": gin.H{
				"requests": runtimeSource,
				"tokens":   "local_metered",
			},
			"updated_at": now.UnixMilli(),
		})
	}
}

// RuntimeFactsWindowPreset defines time window configuration for telemetry breakdown.
type RuntimeFactsWindowPreset struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	ShortLabel    string `json:"short_label"`
	WindowMinutes int    `json:"window_minutes"`
	BucketMinutes int    `json:"bucket_minutes"`
	BucketCount   int    `json:"bucket_count"`
}

var (
	chinaTzLocation = time.FixedZone("CST", 8*3600)
	chinaTzOffset   = int64(8 * 3600)
)

var runtimeFactsWindows = []RuntimeFactsWindowPreset{
	{ID: "1h", Label: "近 1 小时", ShortLabel: "1h", WindowMinutes: 60, BucketMinutes: 5, BucketCount: 12},
	{ID: "6h", Label: "近 6 小时", ShortLabel: "6h", WindowMinutes: 360, BucketMinutes: 15, BucketCount: 24},
	{ID: "1d", Label: "近 1 天", ShortLabel: "1d", WindowMinutes: 1440, BucketMinutes: 60, BucketCount: 24},
	{ID: "3d", Label: "近 3 天", ShortLabel: "3d", WindowMinutes: 4320, BucketMinutes: 180, BucketCount: 24},
	{ID: "7d", Label: "近 7 天", ShortLabel: "7d", WindowMinutes: 10080, BucketMinutes: 360, BucketCount: 28},
	{ID: "30d", Label: "近 30 天", ShortLabel: "30d", WindowMinutes: 43200, BucketMinutes: 1440, BucketCount: 30},
}

// legacyWindowPresets maintains backward compatibility for historical presets.
var legacyWindowPresets = []RuntimeFactsWindowPreset{
	{ID: "3h20m", Label: "近 3 小时 20 分", ShortLabel: "3h20m", WindowMinutes: 200, BucketMinutes: 10, BucketCount: 20},
	{ID: "5h", Label: "近 5 小时", ShortLabel: "5h", WindowMinutes: 300, BucketMinutes: 10, BucketCount: 30},
}

func resolveWindowPreset(windowParam string) RuntimeFactsWindowPreset {
	norm := strings.ToLower(strings.TrimSpace(windowParam))
	switch norm {
	case "12h", "24h":
		norm = "1d"
	}
	for _, w := range runtimeFactsWindows {
		if strings.EqualFold(w.ID, norm) {
			return w
		}
	}
	for _, w := range legacyWindowPresets {
		if strings.EqualFold(w.ID, norm) {
			return w
		}
	}
	return runtimeFactsWindows[0]
}

// RecentBucket represents a time-slotted request count entry.
type RecentBucket struct {
	Time    string `json:"time"`
	Success int64  `json:"success"`
	Failed  int64  `json:"failed"`
	Start   int64  `json:"start,omitempty"`
	End     int64  `json:"end,omitempty"`
}

func formatWindowBucketTime(startSec, endSec int64, bucketMinutes int) string {
	t1 := time.Unix(startSec, 0).In(chinaTzLocation)
	t2 := time.Unix(endSec, 0).In(chinaTzLocation)

	if bucketMinutes < 60 {
		return fmt.Sprintf("%02d:%02d-%02d:%02d", t1.Hour(), t1.Minute(), t2.Hour(), t2.Minute())
	}
	if bucketMinutes >= 1440 {
		return fmt.Sprintf("%02d-%02d", t1.Month(), t1.Day())
	}
	if t1.Month() == t2.Month() && t1.Day() == t2.Day() {
		return fmt.Sprintf("%02d-%02d %02d:%02d-%02d:%02d", t1.Month(), t1.Day(), t1.Hour(), t1.Minute(), t2.Hour(), t2.Minute())
	}
	return fmt.Sprintf("%02d-%02d %02d:%02d-%02d-%02d %02d:%02d", t1.Month(), t1.Day(), t1.Hour(), t1.Minute(), t2.Month(), t2.Day(), t2.Hour(), t2.Minute())
}

func alignAndSum(sequences [][]RecentBucket, bucketCount int) []RecentBucket {
	result := make([]RecentBucket, bucketCount)
	var nonEmpty [][]RecentBucket
	maxWidth := 0
	for _, seq := range sequences {
		if len(seq) > 0 {
			nonEmpty = append(nonEmpty, seq)
			if len(seq) > maxWidth {
				maxWidth = len(seq)
			}
		}
	}
	if len(nonEmpty) == 0 {
		return result
	}
	if maxWidth > bucketCount {
		maxWidth = bucketCount
	}
	offset := bucketCount - maxWidth
	for _, seq := range nonEmpty {
		seqLen := len(seq)
		if seqLen > bucketCount {
			seq = seq[seqLen-bucketCount:]
			seqLen = bucketCount
		}
		seqOffset := offset + (maxWidth - seqLen)
		for i, b := range seq {
			idx := seqOffset + i
			if idx >= 0 && idx < bucketCount {
				result[idx].Success += b.Success
				result[idx].Failed += b.Failed
				if result[idx].Time == "" && b.Time != "" {
					result[idx].Time = b.Time
				}
				if result[idx].Start == 0 && b.Start != 0 {
					result[idx].Start = b.Start
					result[idx].End = b.End
				}
			}
		}
	}
	return result
}

func getLocalTrafficBuckets(ctx context.Context, eng *engine.Engine, preset RuntimeFactsWindowPreset, nowSec int64) []RecentBucket {
	bucketSeconds := int64(preset.BucketMinutes * 60)
	localCurrentSec := nowSec + chinaTzOffset
	localCurrentSlot := localCurrentSec / bucketSeconds
	minSlot := localCurrentSlot - int64(preset.BucketCount) + 1
	minTs := minSlot*bucketSeconds - chinaTzOffset
	maxTs := (localCurrentSlot+1)*bucketSeconds - chinaTzOffset

	type slotEntry struct {
		success int64
		failed  int64
	}
	countMap := make(map[int64]*slotEntry)

	// 1. Query SQLite window aggregate
	if eng != nil && eng.DB() != nil {
		rows, err := eng.DB().SelectEventsWindowAggregate(ctx, chinaTzOffset, bucketSeconds, minTs, maxTs)
		if err == nil {
			for _, r := range rows {
				countMap[r.SlotStart] = &slotEntry{
					success: r.Success,
					failed:  r.Failed,
				}
			}
		}
	}

	// 2. Overlay unflushed events in usage_outbox
	if eng != nil && eng.DB() != nil {
		outbox, err := eng.DB().SelectAllOutbox(ctx)
		if err == nil {
			for _, ev := range outbox {
				if ev.TS >= minTs && ev.TS < maxTs {
					localTs := ev.TS + chinaTzOffset
					slotStart := (localTs/bucketSeconds)*bucketSeconds - chinaTzOffset
					entry := countMap[slotStart]
					if entry == nil {
						entry = &slotEntry{}
						countMap[slotStart] = entry
					}
					if ev.Status >= 200 && ev.Status < 400 {
						entry.success++
					} else {
						entry.failed++
					}
				}
			}
		}
	}

	// 3. Overlay in-memory pending events
	if eng != nil {
		pending := eng.PeekPendingEvents()
		for _, ev := range pending {
			if ev != nil && ev.TS >= minTs && ev.TS < maxTs {
				localTs := ev.TS + chinaTzOffset
				slotStart := (localTs/bucketSeconds)*bucketSeconds - chinaTzOffset
				entry := countMap[slotStart]
				if entry == nil {
					entry = &slotEntry{}
					countMap[slotStart] = entry
				}
				if ev.Status >= 200 && ev.Status < 400 {
					entry.success++
				} else {
					entry.failed++
				}
			}
		}
	}

	// 4. Build contiguous buckets slice
	buckets := make([]RecentBucket, preset.BucketCount)
	for i := 0; i < preset.BucketCount; i++ {
		slot := minSlot + int64(i)
		startSec := slot*bucketSeconds - chinaTzOffset
		endSec := (slot+1)*bucketSeconds - chinaTzOffset
		var s, f int64
		if entry := countMap[startSec]; entry != nil {
			s = entry.success
			f = entry.failed
		}
		buckets[i] = RecentBucket{
			Time:    formatWindowBucketTime(startSec, endSec, preset.BucketMinutes),
			Success: s,
			Failed:  f,
			Start:   startSec,
			End:     endSec,
		}
	}

	return buckets
}

type rawCooldownRecord struct {
	Scope            string    `json:"scope"`
	ModelKey         string    `json:"model_key"`
	Reason           string    `json:"reason"`
	RetryAt          time.Time `json:"retry_at"`
	RemainingSeconds int64     `json:"remaining_seconds"`
	BackoffLevel     *int      `json:"backoff_level"`
	HTTPStatus       *int      `json:"http_status"`
}

// CooldownSummary holds normalized aggregate cooldown data.
type CooldownSummary struct {
	Scope            string   `json:"scope"`
	ScopeLabel       string   `json:"scope_label"`
	ModelCount       int      `json:"model_count"`
	Reason           string   `json:"reason"`
	ReasonLabel      string   `json:"reason_label"`
	RemainingSeconds int64    `json:"remaining_seconds"`
	RemainingText    string   `json:"remaining_text"`
	EarliestRetryAt  *int64   `json:"earliest_retry_at"`
	HTTPStatus       *int     `json:"http_status"`
	BackoffLevel     *int     `json:"backoff_level"`
	ObservedAt       int64    `json:"observed_at"`
	Reasons          []string `json:"reasons"`
	ModelKeys        []string `json:"model_keys"`
}

// AuthHealthInfo holds classified operational health for an upstream credential.
type AuthHealthInfo struct {
	State    string           `json:"state"`
	Label    string           `json:"label"`
	Detail   string           `json:"detail"`
	Blocked  bool             `json:"blocked"`
	Cooldown *CooldownSummary `json:"cooldown"`
}

func formatCooldownRemaining(seconds int64) string {
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
	restMinutes := minutes % 60
	if hours >= 24 {
		days := hours / 24
		return fmt.Sprintf("%d 天 %d 小时", days, hours%24)
	}
	if restMinutes > 0 {
		return fmt.Sprintf("%d 小时 %d 分钟", hours, restMinutes)
	}
	return fmt.Sprintf("%d 小时", hours)
}

func interpretCooldowns(rawList []rawCooldownRecord, observedAtMs int64, nowMs int64) *CooldownSummary {
	if len(rawList) == 0 {
		return nil
	}
	elapsedSec := (nowMs - observedAtMs) / 1000
	if elapsedSec < 0 {
		elapsedSec = 0
	}

	var active []rawCooldownRecord
	var activeRemaining []int64

	for _, r := range rawList {
		rem := r.RemainingSeconds - elapsedSec
		if rem > 0 {
			active = append(active, r)
			activeRemaining = append(activeRemaining, rem)
		}
	}
	if len(active) == 0 {
		return nil
	}

	modelCount := 0
	credentialWide := false
	modelKeyMap := make(map[string]bool)
	reasonMap := make(map[string]bool)

	var earliestRetryAt *int64
	var firstHTTPStatus *int
	var maxBackoffLevel *int
	minRemaining := activeRemaining[0]

	for i, r := range active {
		rem := activeRemaining[i]
		if rem < minRemaining {
			minRemaining = rem
		}
		if r.Scope == "credential" {
			credentialWide = true
		} else if r.Scope == "model" {
			modelCount++
			if r.ModelKey != "" {
				modelKeyMap[r.ModelKey] = true
			}
		}
		if r.Reason != "" {
			reasonMap[r.Reason] = true
		}
		if !r.RetryAt.IsZero() {
			ms := r.RetryAt.UnixMilli()
			if earliestRetryAt == nil || ms < *earliestRetryAt {
				earliestRetryAt = &ms
			}
		}
		if firstHTTPStatus == nil && r.HTTPStatus != nil {
			firstHTTPStatus = r.HTTPStatus
		}
		if r.BackoffLevel != nil {
			if maxBackoffLevel == nil || *r.BackoffLevel > *maxBackoffLevel {
				maxBackoffLevel = r.BackoffLevel
			}
		}
	}

	scope := "models"
	scopeLabel := fmt.Sprintf("%d 个模型冷却", modelCount)
	if credentialWide && modelCount > 0 {
		scope = "credential_and_models"
		scopeLabel = fmt.Sprintf("凭证及 %d 个模型冷却", modelCount)
	} else if credentialWide {
		scope = "credential"
		scopeLabel = "整个凭证"
	}

	severityRank := map[string]int{
		"quota":                1,
		"credential_quota":     2,
		"payment_required":     3,
		"cloudflare_challenge": 4,
		"invalid_grant":        5,
		"unauthorized":         6,
		"model_not_supported":  7,
		"not_found":            8,
		"transient_error":      9,
	}

	reasons := make([]string, 0, len(reasonMap))
	for k := range reasonMap {
		reasons = append(reasons, k)
	}
	sort.Slice(reasons, func(i, j int) bool {
		rI := severityRank[reasons[i]]
		if rI == 0 {
			rI = 99
		}
		rJ := severityRank[reasons[j]]
		if rJ == 0 {
			rJ = 99
		}
		return rI < rJ
	})
	primaryReason := "unknown"
	if len(reasons) > 0 {
		primaryReason = reasons[0]
	}

	reasonLabels := map[string]string{
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
	reasonLabel := reasonLabels[primaryReason]
	if reasonLabel == "" {
		reasonLabel = "其他或未知原因"
	}

	modelKeys := make([]string, 0, len(modelKeyMap))
	for k := range modelKeyMap {
		modelKeys = append(modelKeys, k)
	}
	sort.Strings(modelKeys)

	return &CooldownSummary{
		Scope:            scope,
		ScopeLabel:       scopeLabel,
		ModelCount:       modelCount,
		Reason:           primaryReason,
		ReasonLabel:      reasonLabel,
		RemainingSeconds: minRemaining,
		RemainingText:    formatCooldownRemaining(minRemaining),
		EarliestRetryAt:  earliestRetryAt,
		HTTPStatus:       firstHTTPStatus,
		BackoffLevel:     maxBackoffLevel,
		ObservedAt:       observedAtMs,
		Reasons:          reasons,
		ModelKeys:        modelKeys,
	}
}

func getAuthHealth(cooldown *CooldownSummary, disabled bool, unavailable bool, status string, statusMsg string) AuthHealthInfo {
	if cooldown != nil {
		return AuthHealthInfo{
			State:    "cooldown",
			Label:    "冷却中",
			Detail:   fmt.Sprintf("%s · %s · 最早结束约 %s", cooldown.ReasonLabel, cooldown.ScopeLabel, cooldown.RemainingText),
			Blocked:  true,
			Cooldown: cooldown,
		}
	}
	statusLower := strings.ToLower(strings.TrimSpace(status))
	msgLower := strings.ToLower(strings.TrimSpace(statusMsg))
	if strings.Contains(msgLower, "token expired") || strings.Contains(msgLower, "credential expired") || strings.Contains(msgLower, "credentials expired") || strings.Contains(msgLower, "令牌已过期") || strings.Contains(msgLower, "凭据已过期") || statusLower == "expired" {
		return AuthHealthInfo{
			State:   "expired",
			Label:   "Token 已过期",
			Detail:  "CPA 报告 token expired，请重新授权或刷新凭据",
			Blocked: true,
		}
	}
	if disabled || statusLower == "disabled" {
		return AuthHealthInfo{
			State:   "disabled",
			Label:   "已停用",
			Detail:  "CPA 已停用此凭据",
			Blocked: true,
		}
	}
	if statusLower == "error" || statusLower == "invalid" || statusLower == "failed" {
		return AuthHealthInfo{
			State:   "error",
			Label:   "凭据异常",
			Detail:  "CPA 报告凭据错误，请检查认证状态",
			Blocked: true,
		}
	}
	if unavailable || statusLower == "unavailable" {
		return AuthHealthInfo{
			State:   "unavailable",
			Label:   "暂不可用",
			Detail:  "CPA 当前标记此凭据不可用",
			Blocked: true,
		}
	}
	return AuthHealthInfo{
		State:   "ready",
		Label:   "可用",
		Detail:  "",
		Blocked: false,
	}
}

func resolveAuthAccountDisplayName(name, authIndex, email, label, nickname string) string {
	nick := strings.TrimSpace(nickname)
	if nick != "" {
		return nick
	}
	rawLabel := strings.TrimSpace(label)
	if rawLabel != "" {
		if idx := strings.LastIndex(rawLabel, "("); idx != -1 && strings.HasSuffix(rawLabel, ")") {
			inner := strings.TrimSpace(rawLabel[idx+1 : len(rawLabel)-1])
			innerLower := strings.ToLower(inner)
			if inner != "" && innerLower != "cn" && innerLower != "intl" && innerLower != "global" && innerLower != "default" && innerLower != "api" {
				return inner
			}
		}
		if !strings.EqualFold(rawLabel, name) {
			return rawLabel
		}
	}
	rawEmail := strings.TrimSpace(email)
	if rawEmail != "" && !strings.EqualFold(rawEmail, name) {
		return rawEmail
	}
	cleanName := strings.TrimSuffix(strings.TrimSpace(name), ".json")
	if cleanName != "" {
		return cleanName
	}
	return authIndex
}

// HandleGetRuntimeFacts returns runtime traffic telemetry with graceful degraded mode.
func HandleGetRuntimeFacts(eng *engine.Engine, cpa *CpaClient, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		now := time.Now()
		nowSec := now.Unix()
		nowMs := now.UnixMilli()

		windowParam := strings.TrimSpace(c.Query("window"))
		matchedPreset := resolveWindowPreset(windowParam)

		// 1. Fetch CPA auth-files
		var authFilesResp struct {
			ObservedAt time.Time `json:"observed_at"`
			Files      []struct {
				Name           string              `json:"name"`
				AuthIndex      string              `json:"auth_index"`
				Provider       string              `json:"provider"`
				Disabled       bool                `json:"disabled"`
				Unavailable    bool                `json:"unavailable"`
				Status         string              `json:"status"`
				StatusMessage  string              `json:"status_message"`
				Email          string              `json:"email"`
				Label          string              `json:"label"`
				Nickname       string              `json:"nickname"`
				Priority       int                 `json:"priority"`
				Success        int64               `json:"success"`
				Failed         int64               `json:"failed"`
				Cooldowns      []rawCooldownRecord `json:"cooldowns"`
				RecentRequests []RecentBucket      `json:"recent_requests"`
			} `json:"files"`
		}

		cpaErr := cpa.RequestJSON(ctx, http.MethodGet, "/auth-files", nil, "v0", &authFilesResp)
		if cpaErr != nil {
			// Degraded payload: return HTTP 200 with degraded status so frontend renders gracefully
			c.JSON(http.StatusOK, gin.H{
				"source":            "cpa_runtime",
				"degraded":          true,
				"error":             cpaErr.Error(),
				"fetched_at":        nowMs,
				"window":            matchedPreset.ID,
				"window_label":      matchedPreset.Label,
				"window_minutes":    matchedPreset.WindowMinutes,
				"bucket_minutes":    matchedPreset.BucketMinutes,
				"bucket_count":      matchedPreset.BucketCount,
				"available_windows": runtimeFactsWindows,
				"totals": gin.H{
					"credentials_total":    0,
					"credentials_active":   0,
					"credentials_disabled": 0,
					"credentials_cooldown": 0,
					"credentials_blocked":  0,
					"scope":                "cumulative",
					"buckets":              []interface{}{},
					"success_count":        0,
					"failure_count":        0,
					"total_requests":       0,
					"success_rate":         nil,
					"window_minutes":       matchedPreset.WindowMinutes,
					"bucket_minutes":       matchedPreset.BucketMinutes,
					"bucket_count":         matchedPreset.BucketCount,
					"source":               "cpa_runtime",
				},
				"providers":   []interface{}{},
				"credentials": []interface{}{},
			})
			return
		}

		// 2. Fetch CPA api-key-usage
		var apiKeyUsageResp map[string]map[string]struct {
			Success        int64          `json:"success"`
			Failed         int64          `json:"failed"`
			RecentRequests []RecentBucket `json:"recent_requests"`
		}
		_ = cpa.RequestJSON(ctx, http.MethodGet, "/api-key-usage", nil, "v0", &apiKeyUsageResp)

		observedAtMs := nowMs
		if !authFilesResp.ObservedAt.IsZero() {
			observedAtMs = authFilesResp.ObservedAt.UnixMilli()
		}

		// 3. Query native gate local buckets (authoritative dual-track SQLite + Outbox + Memory)
		localBuckets := getLocalTrafficBuckets(ctx, eng, matchedPreset, nowSec)
		hasLocalTraffic := false
		for _, b := range localBuckets {
			if b.Success > 0 || b.Failed > 0 {
				hasLocalTraffic = true
				break
			}
		}

		type CredentialFact struct {
			AuthIndex     string         `json:"auth_index"`
			Name          string         `json:"name"`
			DisplayName   string         `json:"display_name"`
			Nickname      string         `json:"nickname"`
			Email         string         `json:"email"`
			Provider      string         `json:"provider"`
			Priority      int            `json:"priority"`
			Status        string         `json:"status"`
			Disabled      bool           `json:"disabled"`
			Unavailable   bool           `json:"unavailable"`
			Success       int64          `json:"success"`
			Failed        int64          `json:"failed"`
			Health        AuthHealthInfo `json:"health"`
			Buckets       []RecentBucket `json:"buckets"`
			WindowMinutes int            `json:"window_minutes"`
		}

		type ProviderFact struct {
			Provider      string         `json:"provider"`
			KeyCount      int            `json:"key_count"`
			TotalSuccess  int64          `json:"total_success"`
			TotalFailed   int64          `json:"total_failed"`
			Total         int64          `json:"total"`
			SuccessRate   *float64       `json:"success_rate"`
			Buckets       []RecentBucket `json:"buckets"`
			WindowMinutes int            `json:"window_minutes"`
		}

		type providerCollector struct {
			provider     string
			keyCount     int
			totalSuccess int64
			totalFailed  int64
			bucketSeqs   [][]RecentBucket
		}
		provCollectorMap := make(map[string]*providerCollector)

		var credFacts []CredentialFact
		var credBucketSeqs [][]RecentBucket
		var cumulativeSuccess, cumulativeFailed int64
		activeCount := 0
		disabledCount := 0
		cooldownCount := 0
		blockedCount := 0
		hasWindowTraffic := false

		for _, f := range authFilesResp.Files {
			prov := strings.ToLower(strings.TrimSpace(f.Provider))
			if prov == "" {
				prov = strings.ToLower(providers.InferProvider("", f.AuthIndex).Key)
			}
			if prov == "" {
				prov = "general"
			}

			cooldown := interpretCooldowns(f.Cooldowns, observedAtMs, nowMs)
			health := getAuthHealth(cooldown, f.Disabled, f.Unavailable, f.Status, f.StatusMessage)

			if f.Disabled {
				disabledCount++
			} else {
				activeCount++
			}
			if health.State == "cooldown" {
				cooldownCount++
			}
			if health.Blocked {
				blockedCount++
			}

			cumulativeSuccess += f.Success
			cumulativeFailed += f.Failed

			displayName := resolveAuthAccountDisplayName(f.Name, f.AuthIndex, f.Email, f.Label, f.Nickname)
			credBuckets := f.RecentRequests
			if len(credBuckets) > 0 {
				credBucketSeqs = append(credBucketSeqs, credBuckets)
				if !hasWindowTraffic {
					for _, b := range credBuckets {
						if b.Success > 0 || b.Failed > 0 {
							hasWindowTraffic = true
							break
						}
					}
				}
			}

			credFacts = append(credFacts, CredentialFact{
				AuthIndex:     f.AuthIndex,
				Name:          displayName,
				DisplayName:   displayName,
				Nickname:      f.Nickname,
				Email:         f.Email,
				Provider:      prov,
				Priority:      f.Priority,
				Status:        f.Status,
				Disabled:      f.Disabled,
				Unavailable:   f.Unavailable,
				Success:       f.Success,
				Failed:        f.Failed,
				Health:        health,
				Buckets:       credBuckets,
				WindowMinutes: matchedPreset.WindowMinutes,
			})

			pCol := provCollectorMap[prov]
			if pCol == nil {
				pCol = &providerCollector{provider: prov}
				provCollectorMap[prov] = pCol
			}
			pCol.keyCount++
			pCol.totalSuccess += f.Success
			pCol.totalFailed += f.Failed
			if len(credBuckets) > 0 {
				pCol.bucketSeqs = append(pCol.bucketSeqs, credBuckets)
			}
		}

		// Merge API key usage providers
		for pRaw, keyMap := range apiKeyUsageResp {
			pNorm := strings.ToLower(strings.TrimSpace(pRaw))
			if pNorm == "" {
				pNorm = "unknown"
			}
			pCol := provCollectorMap[pNorm]
			if pCol == nil {
				pCol = &providerCollector{provider: pNorm}
				provCollectorMap[pNorm] = pCol
			}
			for _, stats := range keyMap {
				pCol.keyCount++
				pCol.totalSuccess += stats.Success
				pCol.totalFailed += stats.Failed
				cumulativeSuccess += stats.Success
				cumulativeFailed += stats.Failed
				if len(stats.RecentRequests) > 0 {
					pCol.bucketSeqs = append(pCol.bucketSeqs, stats.RecentRequests)
					credBucketSeqs = append(credBucketSeqs, stats.RecentRequests)
					if !hasWindowTraffic {
						for _, b := range stats.RecentRequests {
							if b.Success > 0 || b.Failed > 0 {
								hasWindowTraffic = true
								break
							}
						}
					}
				}
			}
		}

		var provList []ProviderFact
		for _, pCol := range provCollectorMap {
			total := pCol.totalSuccess + pCol.totalFailed
			var sRate *float64
			if total > 0 {
				r := float64(pCol.totalSuccess) / float64(total) * 100
				r = float64(int(r*100+0.5)) / 100
				sRate = &r
			}
			pBuckets := alignAndSum(pCol.bucketSeqs, matchedPreset.BucketCount)
			provList = append(provList, ProviderFact{
				Provider:      pCol.provider,
				KeyCount:      pCol.keyCount,
				TotalSuccess:  pCol.totalSuccess,
				TotalFailed:   pCol.totalFailed,
				Total:         total,
				SuccessRate:   sRate,
				Buckets:       pBuckets,
				WindowMinutes: matchedPreset.WindowMinutes,
			})
		}
		sort.Slice(provList, func(i, j int) bool {
			if provList[i].Total != provList[j].Total {
				return provList[i].Total > provList[j].Total
			}
			return provList[i].Provider < provList[j].Provider
		})

		// Decide throughput buckets:
		// Gate local buckets are authoritative; if empty and CPA has window traffic, fall back to CPA ring buffer when bucket granularity matches
		useLocal := hasLocalTraffic || !hasWindowTraffic || matchedPreset.BucketMinutes != 10
		var finalBuckets []RecentBucket
		var finalSource string
		if useLocal {
			finalBuckets = localBuckets
			if hasLocalTraffic {
				finalSource = "gate_local"
			} else {
				finalSource = "cpa_runtime"
			}
		} else {
			finalBuckets = alignAndSum(credBucketSeqs, matchedPreset.BucketCount)
			finalSource = "cpa_runtime"
		}

		var windowSuccess, windowFailed int64
		for _, b := range finalBuckets {
			windowSuccess += b.Success
			windowFailed += b.Failed
		}

		effectiveTraffic := hasLocalTraffic || hasWindowTraffic
		var finalSuccess, finalFailed int64
		var scope string
		if effectiveTraffic {
			finalSuccess = windowSuccess
			finalFailed = windowFailed
			scope = "window"
		} else {
			finalSuccess = cumulativeSuccess
			finalFailed = cumulativeFailed
			scope = "cumulative"
		}

		totalReqs := finalSuccess + finalFailed
		var successRate *float64
		if totalReqs > 0 {
			rate := float64(finalSuccess) / float64(totalReqs) * 100
			rate = float64(int(rate*100+0.5)) / 100
			successRate = &rate
		}

		c.JSON(http.StatusOK, gin.H{
			"source":            finalSource,
			"degraded":          false,
			"fetched_at":        nowMs,
			"window":            matchedPreset.ID,
			"window_label":      matchedPreset.Label,
			"window_minutes":    matchedPreset.WindowMinutes,
			"bucket_minutes":    matchedPreset.BucketMinutes,
			"bucket_count":      matchedPreset.BucketCount,
			"available_windows": runtimeFactsWindows,
			"totals": gin.H{
				"credentials_total":    len(credFacts),
				"credentials_active":   activeCount,
				"credentials_disabled": disabledCount,
				"credentials_cooldown": cooldownCount,
				"credentials_blocked":  blockedCount,
				"scope":                scope,
				"buckets":              finalBuckets,
				"success_count":        finalSuccess,
				"failure_count":        finalFailed,
				"total_requests":       totalReqs,
				"success_rate":         successRate,
				"window_minutes":       matchedPreset.WindowMinutes,
				"bucket_minutes":       matchedPreset.BucketMinutes,
				"bucket_count":         matchedPreset.BucketCount,
				"source":               finalSource,
			},
			"providers":   provList,
			"credentials": credFacts,
		})
	}
}

// UpstreamRoutingConfig holds routing configuration fields.
type UpstreamRoutingConfig struct {
	Strategy           string `json:"strategy"`
	SessionAffinity    bool   `json:"session_affinity"`
	SessionAffinityTTL string `json:"session_affinity_ttl"`
}

func normalizeRoutingStrategy(val string) string {
	s := strings.ToLower(strings.TrimSpace(val))
	switch s {
	case "", "round-robin", "roundrobin", "rr":
		return "round-robin"
	case "weighted-round-robin", "weightedroundrobin", "wrr":
		return "weighted-round-robin"
	case "fill-first", "fillfirst", "ff":
		return "fill-first"
	case "smart", "smart-routing":
		return "smart"
	case "pace", "pace-routing":
		return "pace"
	case "priority":
		return "priority"
	default:
		return ""
	}
}

func normalizeRoutingTTL(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return ""
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n >= 0 {
			return strconv.FormatInt(n, 10)
		}
		return ""
	case float64:
		if val >= 0 {
			return strconv.FormatInt(int64(val), 10)
		}
		return ""
	case int:
		if val >= 0 {
			return strconv.Itoa(val)
		}
		return ""
	case int64:
		if val >= 0 {
			return strconv.FormatInt(val, 10)
		}
		return ""
	default:
		return ""
	}
}

func readRoutingConfig(ctx context.Context, cpa *CpaClient) (*UpstreamRoutingConfig, error) {
	var configResp map[string]interface{}
	if err := cpa.RequestJSON(ctx, http.MethodGet, "/config", nil, "v0", &configResp); err != nil {
		return nil, err
	}
	res := &UpstreamRoutingConfig{
		Strategy:           "round-robin",
		SessionAffinity:    false,
		SessionAffinityTTL: "",
	}
	if rRaw, ok := configResp["routing"].(map[string]interface{}); ok && rRaw != nil {
		if strat, ok := rRaw["strategy"].(string); ok {
			if norm := normalizeRoutingStrategy(strat); norm != "" {
				res.Strategy = norm
			}
		}
		if aff, ok := rRaw["session-affinity"].(bool); ok {
			res.SessionAffinity = aff
		} else if aff, ok := rRaw["session_affinity"].(bool); ok {
			res.SessionAffinity = aff
		}
		if ttl, exists := rRaw["session-affinity-ttl"]; exists {
			res.SessionAffinityTTL = normalizeRoutingTTL(ttl)
		} else if ttl, exists := rRaw["session_affinity_ttl"]; exists {
			res.SessionAffinityTTL = normalizeRoutingTTL(ttl)
		}
	}
	return res, nil
}

// HandleGetRuntimeConfig returns the snapshot of runtime config, routing, version, and usage stats wrapped in ok/value envelopes.
func HandleGetRuntimeConfig(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		var configData interface{}
		configErr := cpa.RequestJSON(ctx, http.MethodGet, "/config", nil, "v0", &configData)

		routingData, routingErr := readRoutingConfig(ctx, cpa)

		var versionData interface{}
		versionErr := cpa.RequestJSON(ctx, http.MethodGet, "/latest-version", nil, "v0", &versionData)

		var usageStatsData interface{}
		usageErr := cpa.RequestJSON(ctx, http.MethodGet, "/usage-statistics-enabled", nil, "v0", &usageStatsData)
		if usageErr != nil {
			if cfgMap, ok := configData.(map[string]interface{}); ok {
				if val, exists := cfgMap["usage-statistics-enabled"]; exists {
					usageStatsData = val
					usageErr = nil
				}
			}
		}

		configWrap := gin.H{"ok": configErr == nil}
		if configErr == nil {
			configWrap["value"] = configData
		} else {
			configWrap["error"] = configErr.Error()
		}

		routingWrap := gin.H{"ok": routingErr == nil}
		if routingErr == nil {
			routingWrap["value"] = routingData
		} else {
			routingWrap["error"] = routingErr.Error()
		}

		versionWrap := gin.H{"ok": versionErr == nil}
		if versionErr == nil {
			versionWrap["value"] = versionData
		} else {
			versionWrap["error"] = versionErr.Error()
		}

		usageWrap := gin.H{"ok": usageErr == nil}
		if usageErr == nil {
			usageWrap["value"] = usageStatsData
		} else {
			usageWrap["error"] = usageErr.Error()
		}

		c.JSON(http.StatusOK, gin.H{
			"config":           configWrap,
			"routing":          routingWrap,
			"version":          versionWrap,
			"usage_statistics": usageWrap,
		})
	}
}

// HandleGetUpstreamRouting reads upstream routing configuration from CPA.
func HandleGetUpstreamRouting(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		routingData, err := readRoutingConfig(ctx, cpa)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Failed to fetch CPA routing: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"routing": routingData})
	}
}

// HandlePutUpstreamRouting updates upstream routing configuration in CPA.
func HandlePutUpstreamRouting(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Strategy           *string     `json:"strategy"`
			SessionAffinity    *bool       `json:"session_affinity"`
			SessionAffinityTTL interface{} `json:"session_affinity_ttl"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		patch := make(map[string]interface{})
		if payload.Strategy != nil {
			norm := normalizeRoutingStrategy(*payload.Strategy)
			if norm == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid routing strategy: must be round-robin, weighted-round-robin, fill-first, smart, pace, or priority"})
				return
			}
			patch["strategy"] = norm
		}
		if payload.SessionAffinity != nil {
			patch["session-affinity"] = *payload.SessionAffinity
		}
		if payload.SessionAffinityTTL != nil {
			patch["session-affinity-ttl"] = normalizeRoutingTTL(payload.SessionAffinityTTL)
		}

		if len(patch) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No routing fields to update"})
			return
		}

		ctx := c.Request.Context()
		cpaPayload := map[string]interface{}{
			"routing": patch,
		}
		cpaBody, _ := json.Marshal(cpaPayload)
		resp, err := cpa.Request(ctx, http.MethodPatch, "/config", cpaBody, "application/json", "v0")
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Failed to update CPA routing config: %v", err)})
			return
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			c.JSON(resp.StatusCode, gin.H{"error": fmt.Sprintf("CPA returned %d: %s", resp.StatusCode, string(resp.Bytes))})
			return
		}

		routingData, err := readRoutingConfig(ctx, cpa)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"routing": patch})
			return
		}
		c.JSON(http.StatusOK, gin.H{"routing": routingData})
	}
}

// HandlePutUpstreamAuthBatch processes batch actions on upstream credentials.
func HandlePutUpstreamAuthBatch(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Names    []string    `json:"names"`
			Action   string      `json:"action"`
			Value    interface{} `json:"value"`
			Priority *int        `json:"priority"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		if len(payload.Names) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Please select at least one auth file"})
			return
		}
		if len(payload.Names) > 200 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot batch process more than 200 files at once"})
			return
		}

		validActions := map[string]bool{
			"enable": true, "disable": true, "priority": true, "reset-cooldown": true, "delete": true,
		}
		if !validActions[payload.Action] {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Unsupported batch action: %s", payload.Action)})
			return
		}

		ctx := c.Request.Context()
		type BatchResultItem struct {
			Name  string `json:"name"`
			OK    bool   `json:"ok"`
			Error string `json:"error,omitempty"`
		}

		var results []BatchResultItem
		succeeded := 0

		authIndexMap := make(map[string]string)
		if payload.Action == "reset-cooldown" {
			var authFilesResp struct {
				Files []struct {
					Name      string `json:"name"`
					AuthIndex string `json:"auth_index"`
				} `json:"files"`
			}
			if err := cpa.RequestJSON(ctx, http.MethodGet, "/auth-files", nil, "v0", &authFilesResp); err == nil {
				for _, f := range authFilesResp.Files {
					if f.Name != "" && f.AuthIndex != "" {
						authIndexMap[f.Name] = f.AuthIndex
					}
				}
			}
		}

		for _, name := range payload.Names {
			if !isSafeAuthFilename(name) {
				results = append(results, BatchResultItem{Name: name, OK: false, Error: "Invalid auth filename"})
				continue
			}

			var err error
			var resp *CpaResponse
			switch payload.Action {
			case "enable":
				b, _ := json.Marshal(map[string]interface{}{"name": name, "disabled": false})
				resp, err = cpa.Request(ctx, http.MethodPatch, "/auth-files/status", b, "application/json", "v0")
			case "disable":
				b, _ := json.Marshal(map[string]interface{}{"name": name, "disabled": true})
				resp, err = cpa.Request(ctx, http.MethodPatch, "/auth-files/status", b, "application/json", "v0")
			case "priority":
				prio := 0
				if payload.Priority != nil {
					prio = *payload.Priority
				} else if payload.Value != nil {
					switch v := payload.Value.(type) {
					case float64:
						prio = int(v)
					case string:
						prio, _ = strconv.Atoi(v)
					}
				}
				b, _ := json.Marshal(map[string]interface{}{"name": name, "priority": prio})
				resp, err = cpa.Request(ctx, http.MethodPatch, "/auth-files/fields", b, "application/json", "v0")
			case "reset-cooldown":
				authIdx := authIndexMap[name]
				if authIdx == "" {
					authIdx = name
				}
				b, _ := json.Marshal(map[string]string{"auth_index": authIdx})
				resp, err = cpa.Request(ctx, http.MethodPost, "/reset-quota", b, "application/json", "v0")
			case "delete":
				targetURL := fmt.Sprintf("/auth-files?name=%s", name)
				resp, err = cpa.Request(ctx, http.MethodDelete, targetURL, nil, "application/json", "v0")
			}

			if err != nil {
				results = append(results, BatchResultItem{Name: name, OK: false, Error: err.Error()})
			} else if resp != nil && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
				errMsg := fmt.Sprintf("status %d", resp.StatusCode)
				var errResp map[string]interface{}
				if json.Unmarshal(resp.Bytes, &errResp) == nil && errResp["error"] != nil {
					errMsg = fmt.Sprintf("%v", errResp["error"])
				} else if len(resp.Bytes) > 0 {
					errMsg = string(resp.Bytes)
				}
				results = append(results, BatchResultItem{Name: name, OK: false, Error: errMsg})
			} else {
				results = append(results, BatchResultItem{Name: name, OK: true})
				succeeded++
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"results":   results,
			"succeeded": succeeded,
			"total":     len(payload.Names),
		})
	}
}

// HandleUpdateUpstreamAuthPriority updates priority, weight, and note for an upstream auth credential.
func HandleUpdateUpstreamAuthPriority(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Name     string      `json:"name"`
			Priority interface{} `json:"priority"`
			Weight   interface{} `json:"weight"`
			Note     *string     `json:"note"`
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
		if !isSafeAuthFilename(name) && !strings.HasPrefix(name, "openrouter-") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的凭据文件名"})
			return
		}

		prio := 0
		if payload.Priority != nil {
			switch v := payload.Priority.(type) {
			case float64:
				prio = int(v)
			case string:
				prio, _ = strconv.Atoi(v)
			}
		}

		cpaPayload := map[string]interface{}{
			"name":     name,
			"priority": prio,
		}
		if payload.Weight != nil {
			switch v := payload.Weight.(type) {
			case float64:
				cpaPayload["weight"] = int(v)
			case string:
				if w, err := strconv.Atoi(v); err == nil {
					cpaPayload["weight"] = w
				}
			}
		}
		if payload.Note != nil {
			cpaPayload["note"] = strings.TrimSpace(*payload.Note)
		}

		if !strings.HasPrefix(name, "openrouter-") {
			b, _ := json.Marshal(cpaPayload)
			if resp, err := cpa.Request(c.Request.Context(), http.MethodPatch, "/auth-files/fields", b, "application/json", "v0"); err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Failed to update CPA auth fields: %v", err)})
				return
			} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				c.JSON(resp.StatusCode, gin.H{"error": fmt.Sprintf("CPA returned %d: %s", resp.StatusCode, string(resp.Bytes))})
				return
			}
		}

		InvalidateUpstreamQuotaCache()

		c.JSON(http.StatusOK, gin.H{
			"status":   "ok",
			"name":     name,
			"priority": prio,
		})
	}
}

// HandleUpdateUpstreamAuthStatus enables or disables an upstream auth credential.
func HandleUpdateUpstreamAuthStatus(cpa *CpaClient) gin.HandlerFunc {
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
		if !isSafeAuthFilename(name) && !strings.HasPrefix(name, "openrouter-") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的凭据文件名"})
			return
		}

		if !strings.HasPrefix(name, "openrouter-") {
			b, _ := json.Marshal(map[string]interface{}{
				"name":     name,
				"disabled": payload.Disabled,
			})
			if resp, err := cpa.Request(c.Request.Context(), http.MethodPatch, "/auth-files/status", b, "application/json", "v0"); err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Failed to update CPA auth status: %v", err)})
				return
			} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				c.JSON(resp.StatusCode, gin.H{"error": fmt.Sprintf("CPA returned %d: %s", resp.StatusCode, string(resp.Bytes))})
				return
			}
		}

		InvalidateUpstreamQuotaCache()

		c.JSON(http.StatusOK, gin.H{
			"status":   "ok",
			"name":     name,
			"disabled": payload.Disabled,
		})
	}
}

// HandleActivateUpstreamQuotaWindow triggers upstream quota refresh/probe for a credential.
func HandleActivateUpstreamQuotaWindow(cpa *CpaClient, eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			AuthIndex string `json:"authIndex"`
			Filename  string `json:"filename"`
			Provider  string `json:"provider"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		authIndex := strings.TrimSpace(payload.AuthIndex)
		filename := strings.TrimSpace(payload.Filename)
		if authIndex == "" && filename == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "必须指定上游凭证 auth_index 或 filename"})
			return
		}

		// Invalidate cached quotas to force fresh query
		InvalidateUpstreamQuotaCache()

		c.JSON(http.StatusOK, gin.H{
			"status":     "ok",
			"message":    "激活探测完成",
			"auth_index": authIndex,
			"filename":   filename,
		})
	}
}
