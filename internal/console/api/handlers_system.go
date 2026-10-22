package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// HandleHealthz responds with health status.
func HandleHealthz() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "console",
			"now":     time.Now().UnixMilli(),
		})
	}
}

// HandleReadyz validates gateway readiness (database connectivity and outbox health).
func HandleReadyz(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		outboxCount, err := eng.DB().CountOutbox(ctx)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not_ready",
				"error":  fmt.Sprintf("database ping failed: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
			"checks": gin.H{
				"outbox_count": outboxCount,
			},
		})
	}
}

// HandleStatusQuota returns quota system health for monitoring.
func HandleStatusQuota(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		outboxCount, outboxErr := eng.DB().CountOutbox(ctx)

		keys, _ := eng.DB().SelectAllKeys(ctx)
		rules, _ := eng.DB().SelectAllQuotaRules(ctx)

		ruleMap := make(map[int64]db.QuotaRuleRow, len(rules))
		for _, r := range rules {
			ruleMap[r.KeyID] = r
		}

		blockedDetails := make([]gin.H, 0)
		warningDetails := make([]gin.H, 0)

		for _, k := range keys {
			r, hasRule := ruleMap[k.ID]
			if !hasRule || r.Enabled != 1 || r.DailyLimit == nil || *r.DailyLimit <= 0 {
				continue
			}

			dailyLimit := *r.DailyLimit
			hardRatio := r.HardRatio
			if hardRatio <= 0 {
				hardRatio = 1.2
			}
			hardLimit := int64(float64(dailyLimit) * hardRatio)
			used := eng.Memory().GetTodayUsageForKey(k.ID)

			percent := 0
			if dailyLimit > 0 {
				percent = int(float64(used) * 100 / float64(dailyLimit))
			}

			alias := k.Alias
			if strings.TrimSpace(alias) == "" {
				alias = k.APIKey
			}
			if len(alias) > 14 {
				alias = alias[:10] + "…" + alias[len(alias)-4:]
			}

			item := gin.H{
				"id":         k.ID,
				"alias":      alias,
				"name":       alias,
				"used":       used,
				"limit":      dailyLimit,
				"hard_limit": hardLimit,
				"percent":    percent,
			}

			if used >= hardLimit {
				blockedDetails = append(blockedDetails, item)
			} else if used >= int64(float64(dailyLimit)*0.85) {
				warningDetails = append(warningDetails, item)
			}
		}

		isDegraded := outboxCount > 5000 || outboxErr != nil
		isBlocked := len(blockedDetails) > 0
		isWarning := len(warningDetails) > 0

		status := "ok"
		httpCode := http.StatusOK
		message := "All quotas and gateway operations healthy"

		if isDegraded {
			status = "degraded"
			httpCode = http.StatusServiceUnavailable
			message = fmt.Sprintf("Gateway storage degraded: outbox queue %d", outboxCount)
		} else if isBlocked {
			status = "blocked"
			httpCode = http.StatusTooManyRequests
			message = fmt.Sprintf("%d key(s) blocked by daily token limit", len(blockedDetails))
		} else if isWarning {
			status = "warning"
			httpCode = http.StatusOK
			message = fmt.Sprintf("%d key(s) exceeded 85%% warning threshold", len(warningDetails))
		}

		keysOk := len(keys) - len(blockedDetails) - len(warningDetails)
		if keysOk < 0 {
			keysOk = 0
		}

		summary := gin.H{
			"total_keys":         len(keys),
			"keys_ok":            keysOk,
			"keys_warning":       len(warningDetails),
			"warning_keys":       len(warningDetails),
			"keys_blocked":       len(blockedDetails),
			"blocked_keys":       len(blockedDetails),
			"outbox_count":       outboxCount,
			"outbox_queue_depth": outboxCount,
			"flush_failures":     0,
		}

		c.JSON(httpCode, gin.H{
			"status":          status,
			"httpCode":        httpCode,
			"http_code":       httpCode,
			"message":         message,
			"summary":         summary,
			"blocked_details": blockedDetails,
			"warning_details": warningDetails,
			"check_time":      time.Now().UnixMilli(),
			"timestamp":       time.Now().Unix(),
		})
	}
}

// HandleMetrics outputs Prometheus-compatible gateway metrics.
func HandleMetrics(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		outboxCount, _ := eng.DB().CountOutbox(ctx)
		validations, _ := eng.DB().SelectValidations(ctx, "pending")

		var lines []string
		lines = append(lines, fmt.Sprintf("console_quota_pending_outbox %d", outboxCount))
		lines = append(lines, fmt.Sprintf("console_pending_validations %d", len(validations)))

		// Compatibility aliases for existing scrapers
		lines = append(lines, fmt.Sprintf("gate_quota_pending_outbox %d", outboxCount))
		lines = append(lines, fmt.Sprintf("cpa_guard_quota_pending_outbox %d", outboxCount))

		c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(strings.Join(lines, "\n")+"\n"))
	}
}
