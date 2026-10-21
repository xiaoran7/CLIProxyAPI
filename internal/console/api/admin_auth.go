package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AdminAuthMiddleware validates the management secret for console admin endpoints.
// It checks Authorization: Bearer <secret>, custom headers (x-admin-key, x-management-key),
// the GateAdminTokenCookie, or query token, supporting constant-time string comparison and bcrypt hashes.
func AdminAuthMiddleware(adminSecret string, cpaSecret ...string) gin.HandlerFunc {
	secondSecret := ""
	if len(cpaSecret) > 0 {
		secondSecret = cpaSecret[0]
	}

	return func(c *gin.Context) {
		trimmedAdmin := strings.TrimSpace(adminSecret)
		trimmedCpa := strings.TrimSpace(secondSecret)

		if trimmedAdmin == "" && trimmedCpa == "" {
			// If no management secret is configured, allow requests from loopback/localhost only
			clientIP := c.ClientIP()
			if clientIP == "127.0.0.1" || clientIP == "::1" || clientIP == "localhost" {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized: management secret is not configured and non-local access is restricted",
			})
			return
		}

		token := ""
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				token = strings.TrimSpace(authHeader[7:])
			} else {
				token = strings.TrimSpace(authHeader)
			}
		}

		if token == "" {
			token = strings.TrimSpace(c.GetHeader("x-admin-key"))
		}
		if token == "" {
			token = strings.TrimSpace(c.GetHeader("x-management-key"))
		}

		if token == "" {
			if cookie, err := c.Cookie(GateAdminTokenCookie); err == nil && cookie != "" {
				token = strings.TrimSpace(cookie)
			}
		}

		if token == "" {
			if qToken := strings.TrimSpace(c.Query("token")); qToken != "" {
				token = qToken
			}
		}

		if token == "" || !verifySecretCandidate(token, trimmedAdmin, trimmedCpa) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized: invalid or missing management secret",
			})
			return
		}

		c.Next()
	}
}
