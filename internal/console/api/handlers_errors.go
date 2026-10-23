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

// HandleGetErrors returns paginated error events and circuit-breaker telemetry.
func HandleGetErrors(eng *engine.Engine, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		database := eng.DB()

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

		timeRange := strings.TrimSpace(c.Query("time_range"))
		sinceParam := strings.TrimSpace(c.Query("since"))
		untilParam := strings.TrimSpace(c.Query("until"))
		sincePtr, untilPtr := resolveTimeRange(timeRange, sinceParam, untilParam)

		var beforeIDVal int64
		if beforeIDPtr != nil {
			beforeIDVal = *beforeIDPtr
		}

		params := db.ErrorFilterParams{
			Page:     page,
			PageSize: pageSize,
			KeyID:    keyIDPtr,
			Since:    sincePtr,
			Until:    untilPtr,
			BeforeID: beforeIDVal,
		}

		events, err := database.SelectErrorsFilteredPaged(ctx, params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query error events: %v", err)})
			return
		}

		total, err := database.CountErrorsFiltered(ctx, params)
		if err != nil {
			total = int64(len(events))
		}

		todayTotal, _ := database.CountTodayErrors(ctx, 0, keyIDPtr)

		baselines, _ := database.SelectBaselinesAll(ctx)
		baselineMap := make(map[string]string)
		for _, b := range baselines {
			if b.Name != nil && *b.Name != "" {
				baselineMap[b.AuthIndex] = strings.TrimSuffix(*b.Name, ".json")
			}
		}

		nowMs := time.Now().UnixMilli()

		type ErrorItemResponse struct {
			ID                  int64   `json:"id"`
			TS                  int64   `json:"ts"`
			FormattedTime       string  `json:"formatted_time"`
			KeyID               *int64  `json:"key_id"`
			KeyAlias            string  `json:"key_alias"`
			DisplayKey          string  `json:"display_key"`
			AuthIndex           *string `json:"auth_index"`
			AuthName            string  `json:"auth_name"`
			Provider            string  `json:"provider"`
			ProviderLabel       string  `json:"provider_label"`
			Model               *string `json:"model"`
			Status              int     `json:"status"`
			ErrorType           *string `json:"error_type"`
			ErrorBody           *string `json:"error_body"`
			RetryAfterMs        int64   `json:"retry_after_ms"`
			RecoverAt           int64   `json:"recover_at"`
			IsActive            bool    `json:"is_active"`
			RemainingRecoverSec int64   `json:"remaining_recover_sec"`
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
			if a != "" {
				if name, ok := baselineMap[a]; ok && name != "" {
					authName = name
				} else if acc, ok := providers.GetAuthAccount(a); ok && acc.Name != "" {
					authName = acc.Name
				} else {
					shortAuth := a
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

			isActive := e.RecoverAt > nowMs
			remainingSec := int64(0)
			if isActive {
				remainingSec = int64(math.Ceil(float64(e.RecoverAt-nowMs) / 1000.0))
			}

			t := time.Unix(e.TS, 0).In(cstZone)

			items = append(items, ErrorItemResponse{
				ID:                  e.ID,
				TS:                  e.TS * 1000,
				FormattedTime:       t.Format("2006-01-02 15:04:05"),
				KeyID:               e.KeyID,
				KeyAlias:            keyAlias,
				DisplayKey:          displayKey,
				AuthIndex:           e.AuthIndex,
				AuthName:            authName,
				Provider:            provRes.Key,
				ProviderLabel:       provRes.Label,
				Model:               e.Model,
				Status:              e.Status,
				ErrorType:           e.ErrorType,
				ErrorBody:           e.ErrorBody,
				RetryAfterMs:        e.RetryAfterMs,
				RecoverAt:           e.RecoverAt,
				IsActive:            isActive,
				RemainingRecoverSec: remainingSec,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"errors":      items,
			"total":       total,
			"today_total": todayTotal,
			"page":        page,
			"page_size":   pageSize,
		})
	}
}
