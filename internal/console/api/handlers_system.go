package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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
		outboxCount, _ := eng.DB().CountOutbox(ctx)
		validations, _ := eng.DB().SelectValidations(ctx, "pending")

		status := "healthy"
		if outboxCount > 5000 || len(validations) > 20 {
			status = "warning"
		}

		c.JSON(http.StatusOK, gin.H{
			"status":              status,
			"outbox_count":        outboxCount,
			"pending_validations": len(validations),
			"timestamp":           time.Now().Unix(),
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
