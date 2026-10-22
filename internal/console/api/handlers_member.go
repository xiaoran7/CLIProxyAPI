package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/middleware"
)

// MemberAuthMiddleware verifies that the request carries a valid active member API key.
func MemberAuthMiddleware(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawKey := middleware.ExtractRawKey(c)
		if rawKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized: missing API key",
			})
			return
		}

		keyID, exists := eng.ResolveKeyID(rawKey)
		if !exists || keyID <= 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized: invalid or deactivated API key",
			})
			return
		}

		c.Set(middleware.ContextKeyID, keyID)
		c.Next()
	}
}

// HandleMemberInfo returns personal usage statistics and quota rule for the authenticated member.
func HandleMemberInfo(eng *engine.Engine, pricing *PricingEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		val, exists := c.Get(middleware.ContextKeyID)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		keyID := val.(int64)

		ctx := c.Request.Context()
		database := eng.DB()

		keyRow, err := database.SelectKeyByID(ctx, keyID)
		if err != nil || keyRow == nil || keyRow.Active == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "API Key not found or inactive"})
			return
		}

		ruleRow, _ := database.SelectQuotaRuleByKeyID(ctx, keyID)

		today := engine.ShanghaiDate(time.Now())
		dailySummary, _ := database.SelectDailyForDateKey(ctx, today, keyID)
		if dailySummary == nil {
			dailySummary = &db.DailyStatsSummary{Date: today}
		}

		// Merge live counters
		liveTokens := eng.Memory().GetTodayUsageForKey(keyID)
		liveRequests := eng.Memory().GetTodayRequestsForKey(keyID)
		if liveTokens > dailySummary.TotalTokens {
			dailySummary.TotalTokens = liveTokens
		}
		if liveRequests > dailySummary.Requests {
			dailySummary.Requests = liveRequests
		}

		// Calculate all-time totals
		allTimeList, _ := database.SelectDailyRangeByKey(ctx, "2000-01-01", keyID)
		allTimeTokens := int64(0)
		allTimeRequests := int64(0)
		for _, item := range allTimeList {
			allTimeTokens += item.TotalTokens
			allTimeRequests += item.Requests
		}

		pBase := dailySummary.PromptTokens
		if dailySummary.CachedTokens > pBase {
			pBase = dailySummary.CachedTokens
		}
		hitRate := 0.0
		if pBase > 0 {
			hitRate = math.Round((float64(dailySummary.CachedTokens)/float64(pBase))*1000.0) / 10.0
		}

		cost := pricing.CalculateModelCost("default", dailySummary.PromptTokens, dailySummary.CompletionTokens, dailySummary.CachedTokens)

		var whitelist []string
		enabled := false
		dailyLimit := int64(0)
		if ruleRow != nil {
			enabled = ruleRow.Enabled == 1
			if ruleRow.DailyLimit != nil {
				dailyLimit = *ruleRow.DailyLimit
			}
			if strings.TrimSpace(ruleRow.ModelWhitelist) != "" {
				_ = json.Unmarshal([]byte(ruleRow.ModelWhitelist), &whitelist)
			}
		}
		if whitelist == nil {
			whitelist = []string{}
		}

		c.JSON(http.StatusOK, gin.H{
			"id":                      keyRow.ID,
			"key_alias":               keyRow.Alias,
			"alias":                   keyRow.Alias,
			"active":                  keyRow.Active,
			"enabled":                 enabled,
			"daily_limit":             dailyLimit,
			"model_whitelist":         whitelist,
			"today_tokens":            dailySummary.TotalTokens,
			"today_prompt_tokens":     dailySummary.PromptTokens,
			"today_completion_tokens": dailySummary.CompletionTokens,
			"today_cached_tokens":     dailySummary.CachedTokens,
			"today_requests":          dailySummary.Requests,
			"today_cost_usd":          cost.CostUSD,
			"cache_hit_rate":          hitRate,
			"all_time_tokens":         allTimeTokens,
			"all_time_requests":       allTimeRequests,
			"created_at":              keyRow.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
}

// HandleMemberStatusSummary returns a sanitized system health overview for members.
func HandleMemberStatusSummary(eng *engine.Engine, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		val, exists := c.Get(middleware.ContextKeyID)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		keyID := val.(int64)

		ctx := c.Request.Context()
		database := eng.DB()

		todayTokens := eng.Memory().GetTodayUsageForKey(keyID)
		todayRequests, _ := database.CountTodayEvents(ctx, 0, &keyID)
		todayErrors, _ := database.CountTodayErrors(ctx, 0, &keyID)

		// 7-day window for Uptime Kuma heartbeats and provider groups
		sinceTs := time.Now().Unix() - 7*86400
		rows, _ := database.SelectEventsForStatus(ctx, sinceTs, 3000)

		nowMs := time.Now().UnixMilli()
		overallScope := buildStatusScope(rows, nowMs)
		providerGroups := buildProviderGroups(rows, providers, nowMs)

		c.JSON(http.StatusOK, gin.H{
			"status":          "ok",
			"window_days":     7,
			"heartbeat_count": 60,
			"overall":         overallScope,
			"provider_groups": providerGroups,
			"today_requests":  todayRequests,
			"today_errors":    todayErrors,
			"member_stats": gin.H{
				"today_tokens":   todayTokens,
				"today_requests": todayRequests,
				"today_errors":   todayErrors,
			},
			"generated_at": nowMs,
			"updated_at":   nowMs,
		})
	}
}

// HandleMemberRequests returns request events restricted to the authenticated member's key.
func HandleMemberRequests(eng *engine.Engine, pricing *PricingEngine, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		val, exists := c.Get(middleware.ContextKeyID)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		keyID := val.(int64)

		ctx := c.Request.Context()
		database := eng.DB()

		// Real-time outbox flushing so in-flight usage is persisted to SQLite
		_ = eng.Flusher().FlushUsage(ctx)

		page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
		if err != nil || page < 1 {
			page = 1
		}

		pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "25"))
		if err != nil || pageSize < 1 {
			pageSize = 25
		}
		if ps := c.Query("limit"); ps != "" && c.Query("page_size") == "" {
			if l, err := strconv.Atoi(ps); err == nil && l > 0 {
				pageSize = l
			}
		}
		if pageSize > 100 {
			pageSize = 100
		}

		var beforeIDPtr *int64
		if bid := c.Query("before_id"); bid != "" {
			if id, err := strconv.ParseInt(bid, 10, 64); err == nil && id > 0 {
				beforeIDPtr = &id
			}
		}

		filter := strings.TrimSpace(c.Query("filter"))
		search := strings.TrimSpace(c.Query("search"))
		timeRange := strings.TrimSpace(c.Query("time_range"))
		sinceParam := strings.TrimSpace(c.Query("since"))
		untilParam := strings.TrimSpace(c.Query("until"))
		sincePtr, untilPtr := resolveTimeRange(timeRange, sinceParam, untilParam)

		cleanProvider := strings.ToLower(strings.TrimSpace(c.Query("provider")))
		var modelPatterns, authPatterns []string
		var providerAuths, excludeAuths []string
		if cleanProvider != "" && cleanProvider != "all" {
			modelPatterns, authPatterns = providers.GetProviderPatterns(cleanProvider)
			providerAuths = providers.GetProviderAuthIndexes(cleanProvider)
			excludeAuths = providers.GetExcludeAuthIndexes(cleanProvider)
		}

		var beforeIDVal int64
		if beforeIDPtr != nil {
			beforeIDVal = *beforeIDPtr
		}

		params := db.EventFilterParams{
			Page:          page,
			PageSize:      pageSize,
			KeyID:         &keyID,
			Provider:      cleanProvider,
			ProviderAuths: providerAuths,
			ExcludeAuths:  excludeAuths,
			ModelPatterns: modelPatterns,
			AuthPatterns:  authPatterns,
			Filter:        filter,
			Search:        search,
			Since:         sincePtr,
			Until:         untilPtr,
			BeforeID:      beforeIDVal,
		}

		events, err := database.SelectEventsFilteredPaged(ctx, params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query events: %v", err)})
			return
		}

		total, err := database.CountEventsFiltered(ctx, params)
		if err != nil {
			total = int64(len(events))
		}

		todayTotal, _ := database.CountTodayEvents(ctx, 0, &keyID)

		keyAlias := ""
		displayKey := "当前 Key"
		if meta := eng.GetKeyMeta(keyID); meta != nil {
			keyAlias = meta.Alias
			if meta.DisplayKey != "" {
				displayKey = meta.DisplayKey
			} else {
				displayKey = fmt.Sprintf("Key #%d", keyID)
			}
		}

		type MemberEventItemResponse struct {
			ID               int64   `json:"id"`
			TS               int64   `json:"ts"`
			FormattedTime    string  `json:"formatted_time"`
			KeyID            *int64  `json:"key_id"`
			KeyAlias         string  `json:"key_alias"`
			DisplayKey       string  `json:"display_key"`
			Model            string  `json:"model"`
			Provider         string  `json:"provider"`
			ProviderLabel    string  `json:"provider_label"`
			Status           int     `json:"status"`
			PromptTokens     int64   `json:"prompt_tokens"`
			CompletionTokens int64   `json:"completion_tokens"`
			CachedTokens     int64   `json:"cached_tokens"`
			TotalTokens      int64   `json:"total_tokens"`
			CacheHitRate     float64 `json:"cache_hit_rate"`
			CostUSD          float64 `json:"cost_usd"`
			SavingsUSD       float64 `json:"savings_usd"`
			LatencyMs        int64   `json:"latency_ms"`
			TtftMs           *int64  `json:"ttft_ms"`
			Stream           bool    `json:"stream"`
			Estimated        bool    `json:"estimated"`
			ErrorType        *string `json:"error_type"`
			ReasoningEffort  string  `json:"reasoning_effort"`
			TokensPerSec     float64 `json:"tokens_per_sec"`
		}

		var items []MemberEventItemResponse
		for _, e := range events {
			provRes := providers.InferProvider(e.Model, e.AuthIndex)
			cost := pricing.CalculateModelCost(e.Model, e.PromptTokens, e.CompletionTokens, e.CachedTokens)

			pBase := e.PromptTokens
			if e.CachedTokens > pBase {
				pBase = e.CachedTokens
			}
			hitRate := 0.0
			if pBase > 0 {
				hitRate = math.Round((float64(e.CachedTokens)/float64(pBase))*1000.0) / 10.0
			}

			tps := e.TokensPerSec
			if tps <= 0 {
				var ttftVal int64
				if e.TtftMs != nil {
					ttftVal = *e.TtftMs
				}
				tps = engine.CalculateTokensPerSec(e.CompletionTokens, e.LatencyMs, ttftVal, e.Stream == 1)
			}

			t := time.Unix(e.TS, 0).In(cstZone)

			items = append(items, MemberEventItemResponse{
				ID:               e.ID,
				TS:               e.TS * 1000,
				FormattedTime:    t.Format("2006-01-02 15:04:05"),
				KeyID:            e.KeyID,
				KeyAlias:         keyAlias,
				DisplayKey:       displayKey,
				Model:            e.Model,
				Provider:         provRes.Key,
				ProviderLabel:    provRes.Label,
				Status:           e.Status,
				PromptTokens:     e.PromptTokens,
				CompletionTokens: e.CompletionTokens,
				CachedTokens:     e.CachedTokens,
				TotalTokens:      e.TotalTokens,
				CacheHitRate:     hitRate,
				CostUSD:          cost.CostUSD,
				SavingsUSD:       cost.SavingsUSD,
				LatencyMs:        e.LatencyMs,
				TtftMs:           e.TtftMs,
				Stream:           e.Stream == 1,
				Estimated:        e.Estimated == 1,
				ErrorType:        e.ErrorType,
				ReasoningEffort:  resolveReasoningEffort(e.ReasoningEffort, e.Model),
				TokensPerSec:     tps,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"events":      items,
			"requests":    items,
			"total":       total,
			"today_total": todayTotal,
			"page":        page,
			"page_size":   pageSize,
		})
	}
}
