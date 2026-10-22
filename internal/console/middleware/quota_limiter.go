package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// QuotaLimiterMiddleware enforces 60s sliding window RPM limits and daily token quotas.
func QuotaLimiterMiddleware(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Discovery, utility, and preflight endpoints do not consume RPM or token quota
		if IsDiscoveryOrUtilityRequest(c.Request.Method, path) {
			c.Next()
			return
		}

		val, exists := c.Get(ContextKeyID)
		if !exists {
			c.Next()
			return
		}

		keyID, ok := val.(int64)
		if !ok || keyID <= 0 {
			c.Next()
			return
		}

		// 1. Sliding window RPM check (evaluated before quota)
		rpmRes := eng.CheckRateLimit(keyID)
		if !rpmRes.Allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"message":   rpmRes.Reason,
					"type":      "rate_limit_error",
					"code":      "rate_limit_exceeded",
					"rpm_limit": rpmRes.RPMLimit,
				},
			})
			return
		}

		// 2. Daily token quota check (enforcing hard_ratio burst allowance)
		quotaRes := eng.CheckQuota(keyID)
		if !quotaRes.Allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"message":    quotaRes.Reason,
					"type":       "insufficient_quota",
					"code":       "quota_exceeded",
					"usage":      quotaRes.Usage,
					"limit":      quotaRes.Limit,
					"hard_limit": quotaRes.HardLimit,
				},
			})
			return
		}

		c.Next()
	}
}
