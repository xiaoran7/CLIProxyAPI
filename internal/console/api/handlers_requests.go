package api

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// HandleGetRequests returns filtered and paginated request events with full metadata.
func HandleGetRequests(eng *engine.Engine, pricing *PricingEngine, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		var keyIDPtr *int64
		if keyIDStr := c.Query("key_id"); keyIDStr != "" && keyIDStr != "all" {
			if id, err := strconv.ParseInt(keyIDStr, 10, 64); err == nil && id > 0 {
				keyIDPtr = &id
			}
		}

		var beforeIDPtr *int64
		if bid := c.Query("before_id"); bid != "" {
			if id, err := strconv.ParseInt(bid, 10, 64); err == nil && id > 0 {
				beforeIDPtr = &id
			}
		}

		filter := strings.TrimSpace(c.Query("filter"))
		authIndex := strings.TrimSpace(c.Query("auth_index"))
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
			KeyID:         keyIDPtr,
			AuthIndex:     authIndex,
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

		todayTotal, _ := database.CountTodayEvents(ctx, 0, keyIDPtr)

		// Cache baselines for quick account resolution
		baselines, _ := database.SelectBaselinesAll(ctx)
		baselineMap := make(map[string]string)
		for _, b := range baselines {
			if b.Name != nil && *b.Name != "" {
				baselineMap[b.AuthIndex] = strings.TrimSuffix(*b.Name, ".json")
			}
		}

		type EventItemResponse struct {
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
			AuthIndex        string  `json:"auth_index"`
			AuthName         string  `json:"auth_name"`
			ReasoningEffort  string  `json:"reasoning_effort"`
			TokensPerSec     float64 `json:"tokens_per_sec"`
		}

		var items []EventItemResponse
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

			keyAlias := ""
			displayKey := "未知 Key"
			if e.KeyID != nil {
				if meta := eng.GetKeyMeta(*e.KeyID); meta != nil {
					keyAlias = meta.Alias
					if meta.DisplayKey != "" {
						displayKey = meta.DisplayKey
					} else {
						displayKey = fmt.Sprintf("Key #%d", *e.KeyID)
					}
				} else {
					displayKey = fmt.Sprintf("Key #%d", *e.KeyID)
				}
			}

			authName := ""
			if e.AuthIndex != "" {
				if name, ok := baselineMap[e.AuthIndex]; ok && name != "" {
					authName = name
				} else if acc, ok := providers.GetAuthAccount(e.AuthIndex); ok && acc.Name != "" {
					authName = acc.Name
				} else {
					shortAuth := e.AuthIndex
					if len(shortAuth) > 8 {
						shortAuth = shortAuth[:8]
					}
					if provRes.Key != "" && provRes.Key != UnknownProviderKey {
						authName = fmt.Sprintf("%s 渠道 %s", provRes.Label, shortAuth)
					} else {
						authName = fmt.Sprintf("已删除账号 %s", shortAuth)
					}
				}
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

			items = append(items, EventItemResponse{
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
				AuthIndex:        e.AuthIndex,
				AuthName:         authName,
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
