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
func HandleMemberStatusSummary(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		val, exists := c.Get(middleware.ContextKeyID)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		keyID := val.(int64)

		todayTokens := eng.Memory().GetTodayUsageForKey(keyID)
		todayRequests := eng.Memory().GetTodayRequestsForKey(keyID)

		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"member_stats": gin.H{
				"today_tokens":   todayTokens,
				"today_requests": todayRequests,
			},
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

		limitStr := c.DefaultQuery("limit", "50")
		if ps := c.Query("page_size"); ps != "" {
			limitStr = ps
		}
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 || limit > 200 {
			limit = 50
		}

		beforeID := int64(0)
		if bid := c.Query("before_id"); bid != "" {
			if id, err := strconv.ParseInt(bid, 10, 64); err == nil {
				beforeID = id
			}
		}

		ctx := c.Request.Context()
		events, err := eng.DB().SelectEventsPaged(ctx, beforeID, limit, &keyID, "", nil, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query events: %v", err)})
			return
		}

		type MemberEventItem struct {
			ID               int64   `json:"id"`
			TS               int64   `json:"ts"`
			FormattedTime    string  `json:"formatted_time"`
			Model            string  `json:"model"`
			Provider         string  `json:"provider"`
			Status           int     `json:"status"`
			PromptTokens     int64   `json:"prompt_tokens"`
			CompletionTokens int64   `json:"completion_tokens"`
			CachedTokens     int64   `json:"cached_tokens"`
			TotalTokens      int64   `json:"total_tokens"`
			LatencyMs        int64   `json:"latency_ms"`
			TtftMs           *int64  `json:"ttft_ms"`
			Stream           int     `json:"stream"`
			CostUSD          float64 `json:"cost_usd"`
		}

		var items []MemberEventItem
		for _, e := range events {
			provRes := providers.InferProvider(e.Model, "")
			cost := pricing.CalculateModelCost(e.Model, e.PromptTokens, e.CompletionTokens, e.CachedTokens)
			t := time.Unix(e.TS, 0).In(cstZone)

			items = append(items, MemberEventItem{
				ID:               e.ID,
				TS:               e.TS,
				FormattedTime:    t.Format("2006-01-02 15:04:05"),
				Model:            e.Model,
				Provider:         provRes.Key,
				Status:           e.Status,
				PromptTokens:     e.PromptTokens,
				CompletionTokens: e.CompletionTokens,
				CachedTokens:     e.CachedTokens,
				TotalTokens:      e.TotalTokens,
				LatencyMs:        e.LatencyMs,
				TtftMs:           e.TtftMs,
				Stream:           e.Stream,
				CostUSD:          cost.CostUSD,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"requests": items,
			"total":    len(items),
		})
	}
}
