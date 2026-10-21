package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

var cstZone = time.FixedZone("CST", 8*3600)

// HandleGetStats returns daily token and request aggregates over a time window.
func HandleGetStats(eng *engine.Engine, pricing *PricingEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		database := eng.DB()

		daysParam := c.DefaultQuery("days", "14")
		days, err := strconv.Atoi(daysParam)
		if err != nil || days <= 0 || days > 365 {
			days = 14
		}

		var keyIDPtr *int64
		if keyIDStr := c.Query("key_id"); keyIDStr != "" {
			if id, err := strconv.ParseInt(keyIDStr, 10, 64); err == nil && id > 0 {
				keyIDPtr = &id
			}
		}

		now := time.Now().In(cstZone)
		sinceTime := now.AddDate(0, 0, -days)
		sinceDate := sinceTime.Format("2006-01-02")

		var dailyList []db.DailyStatsSummary
		var modelList []db.ModelStatsSummary

		if keyIDPtr != nil {
			dailyList, err = database.SelectDailyRangeByKey(ctx, sinceDate, *keyIDPtr)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query daily range: %v", err)})
				return
			}
			modelList, err = database.SelectModelRangeByKey(ctx, sinceDate, *keyIDPtr)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query model range: %v", err)})
				return
			}
		} else {
			dailyList, err = database.SelectDailyRange(ctx, sinceDate)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query daily range: %v", err)})
				return
			}
			modelList, err = database.SelectModelRange(ctx, sinceDate)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query model range: %v", err)})
				return
			}
		}

		totalTokens := int64(0)
		totalRequests := int64(0)
		totalSuccess := int64(0)
		totalFailure := int64(0)
		totalCostUSD := 0.0

		type DailyResponseItem struct {
			Date             string  `json:"date"`
			Requests         int64   `json:"requests"`
			PromptTokens     int64   `json:"prompt_tokens"`
			CompletionTokens int64   `json:"completion_tokens"`
			CachedTokens     int64   `json:"cached_tokens"`
			TotalTokens      int64   `json:"total_tokens"`
			SuccessCount     int64   `json:"success_count"`
			FailureCount     int64   `json:"failure_count"`
			CostUSD          float64 `json:"cost_usd"`
		}

		var formattedDaily []DailyResponseItem
		for _, d := range dailyList {
			totalTokens += d.TotalTokens
			totalRequests += d.Requests
			totalSuccess += d.SuccessCount
			totalFailure += d.FailureCount

			cost := pricing.CalculateModelCost("default", d.PromptTokens, d.CompletionTokens, d.CachedTokens)
			totalCostUSD += cost.CostUSD

			formattedDaily = append(formattedDaily, DailyResponseItem{
				Date:             d.Date,
				Requests:         d.Requests,
				PromptTokens:     d.PromptTokens,
				CompletionTokens: d.CompletionTokens,
				CachedTokens:     d.CachedTokens,
				TotalTokens:      d.TotalTokens,
				SuccessCount:     d.SuccessCount,
				FailureCount:     d.FailureCount,
				CostUSD:          cost.CostUSD,
			})
		}

		type ModelResponseItem struct {
			Model            string  `json:"model"`
			Requests         int64   `json:"requests"`
			PromptTokens     int64   `json:"prompt_tokens"`
			CompletionTokens int64   `json:"completion_tokens"`
			CachedTokens     int64   `json:"cached_tokens"`
			TotalTokens      int64   `json:"total_tokens"`
			CostUSD          float64 `json:"cost_usd"`
		}

		var formattedModels []ModelResponseItem
		for _, m := range modelList {
			cost := pricing.CalculateModelCost(m.Model, m.PromptTokens, m.CompletionTokens, m.CachedTokens)
			formattedModels = append(formattedModels, ModelResponseItem{
				Model:            m.Model,
				Requests:         m.Requests,
				PromptTokens:     m.PromptTokens,
				CompletionTokens: m.CompletionTokens,
				CachedTokens:     m.CachedTokens,
				TotalTokens:      m.TotalTokens,
				CostUSD:          cost.CostUSD,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"daily":  formattedDaily,
			"models": formattedModels,
			"summary": gin.H{
				"days":           days,
				"total_requests": totalRequests,
				"total_tokens":   totalTokens,
				"success_count":  totalSuccess,
				"failure_count":  totalFailure,
				"total_cost_usd": totalCostUSD,
			},
		})
	}
}

// HandleGetRequests returns recent or paged request events.
func HandleGetRequests(eng *engine.Engine, pricing *PricingEngine, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		database := eng.DB()

		var keyIDPtr *int64
		if keyIDStr := c.Query("key_id"); keyIDStr != "" {
			if id, err := strconv.ParseInt(keyIDStr, 10, 64); err == nil && id > 0 {
				keyIDPtr = &id
			}
		}

		limitStr := c.DefaultQuery("limit", "100")
		if ps := c.Query("page_size"); ps != "" {
			limitStr = ps
		}
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 || limit > 500 {
			limit = 100
		}

		beforeID := int64(0)
		if bid := c.Query("before_id"); bid != "" {
			if id, err := strconv.ParseInt(bid, 10, 64); err == nil {
				beforeID = id
			}
		}

		modelFilter := strings.TrimSpace(c.Query("model"))
		authIndexFilter := strings.TrimSpace(c.Query("auth_index"))
		var statusPtr *int
		if st := c.Query("status"); st != "" {
			if s, err := strconv.Atoi(st); err == nil {
				statusPtr = &s
			}
		}

		var events []db.RequestEventRow
		if beforeID > 0 || keyIDPtr != nil || modelFilter != "" || authIndexFilter != "" || statusPtr != nil {
			events, err = database.SelectEventsPaged(ctx, beforeID, limit, keyIDPtr, modelFilter, statusPtr, authIndexFilter)
		} else {
			events, err = database.SelectRecentEvents(ctx, limit)
		}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query request events: %v", err)})
			return
		}

		type EventItemResponse struct {
			ID               int64   `json:"id"`
			TS               int64   `json:"ts"`
			FormattedTime    string  `json:"formatted_time"`
			KeyID            *int64  `json:"key_id"`
			Model            string  `json:"model"`
			Provider         string  `json:"provider"`
			ProviderLabel    string  `json:"provider_label"`
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
			TokensPerSec     float64 `json:"tokens_per_sec"`
			CostUSD          float64 `json:"cost_usd"`
		}

		var items []EventItemResponse
		for _, e := range events {
			provRes := providers.InferProvider(e.Model, e.AuthIndex)
			cost := pricing.CalculateModelCost(e.Model, e.PromptTokens, e.CompletionTokens, e.CachedTokens)

			t := time.Unix(e.TS, 0).In(cstZone)

			items = append(items, EventItemResponse{
				ID:               e.ID,
				TS:               e.TS,
				FormattedTime:    t.Format("2006-01-02 15:04:05"),
				KeyID:            e.KeyID,
				Model:            e.Model,
				Provider:         provRes.Key,
				ProviderLabel:    provRes.Label,
				Status:           e.Status,
				PromptTokens:     e.PromptTokens,
				CompletionTokens: e.CompletionTokens,
				CachedTokens:     e.CachedTokens,
				TotalTokens:      e.TotalTokens,
				LatencyMs:        e.LatencyMs,
				TtftMs:           e.TtftMs,
				Stream:           e.Stream,
				Estimated:        e.Estimated,
				ErrorType:        e.ErrorType,
				AuthIndex:        e.AuthIndex,
				TokensPerSec:     e.TokensPerSec,
				CostUSD:          cost.CostUSD,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"requests": items,
			"total":    len(items),
		})
	}
}

// HandleGetErrors returns recent error events.
func HandleGetErrors(eng *engine.Engine, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		limitStr := c.DefaultQuery("limit", "100")
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 || limit > 500 {
			limit = 100
		}

		events, err := eng.DB().SelectRecentErrorEvents(ctx, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query error events: %v", err)})
			return
		}

		type ErrorItemResponse struct {
			ID            int64   `json:"id"`
			TS            int64   `json:"ts"`
			FormattedTime string  `json:"formatted_time"`
			KeyID         *int64  `json:"key_id"`
			AuthIndex     *string `json:"auth_index"`
			Provider      string  `json:"provider"`
			ProviderLabel string  `json:"provider_label"`
			Model         *string `json:"model"`
			Status        int     `json:"status"`
			ErrorType     *string `json:"error_type"`
			ErrorBody     *string `json:"error_body"`
			RetryAfterMs  int64   `json:"retry_after_ms"`
			RecoverAt     int64   `json:"recover_at"`
		}

		var items []ErrorItemResponse
		for _, e := range events {
			m := ""
			if e.Model != nil {
				m = *e.Model
			}
			a := ""
			if e.AuthIndex != nil {
				a = *e.AuthIndex
			}
			provRes := providers.InferProvider(m, a)

			t := time.Unix(e.TS, 0).In(cstZone)

			items = append(items, ErrorItemResponse{
				ID:            e.ID,
				TS:            e.TS,
				FormattedTime: t.Format("2006-01-02 15:04:05"),
				KeyID:         e.KeyID,
				AuthIndex:     e.AuthIndex,
				Provider:      provRes.Key,
				ProviderLabel: provRes.Label,
				Model:         e.Model,
				Status:        e.Status,
				ErrorType:     e.ErrorType,
				ErrorBody:     e.ErrorBody,
				RetryAfterMs:  e.RetryAfterMs,
				RecoverAt:     e.RecoverAt,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"errors": items,
			"total":  len(items),
		})
	}
}

// HandleGetStatusSummary returns a comprehensive operational summary of the console gateway.
func HandleGetStatusSummary(eng *engine.Engine, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		database := eng.DB()

		outboxCount, _ := database.CountOutbox(ctx)
		validations, _ := database.SelectValidations(ctx, "pending")

		today := engine.ShanghaiDate(time.Now())
		dailyToday, _ := database.SelectDailyForDate(ctx, today)

		todayTokens := int64(0)
		todayRequests := int64(0)
		todaySuccess := int64(0)
		todayFailure := int64(0)

		for _, row := range dailyToday {
			todayTokens += row.TotalTokens
			todayRequests += row.Requests
			todaySuccess += row.SuccessCount
			todayFailure += row.FailureCount
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"health": gin.H{
				"outbox_count":        outboxCount,
				"pending_validations": len(validations),
			},
			"today": gin.H{
				"date":          today,
				"tokens":        todayTokens,
				"requests":      todayRequests,
				"success_count": todaySuccess,
				"failure_count": todayFailure,
			},
		})
	}
}
