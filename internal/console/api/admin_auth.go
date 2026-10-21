package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AdminAuthMiddleware validates the management secret for console admin endpoints.
// It checks Authorization: Bearer <secret> or x-admin-key: <secret> using constant-time comparison.
func AdminAuthMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		trimmedSecret := strings.TrimSpace(secret)
		if trimmedSecret == "" {
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

		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(trimmedSecret)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized: invalid or missing management secret",
			})
			return
		}

		c.Next()
	}
}
