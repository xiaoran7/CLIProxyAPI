package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

const (
	ContextKeyID     = "console_key_id"
	ContextRawKey    = "console_raw_key"
	ContextModel     = "console_model"
	ContextReasoning = "console_reasoning_effort"
	ContextAuthIndex = "console_auth_index"
)

// ExtractRawKey extracts the plaintext API key from multiple standard protocol headers and query parameters:
// 1. Authorization: Bearer <key>
// 2. x-api-key: <key> (Anthropic)
// 3. api-key: <key> (Azure OpenAI)
// 4. x-goog-api-key: <key> (Google Gemini)
// 5. Query parameter ?key=<key> (Gemini REST)
func ExtractRawKey(c *gin.Context) string {
	// 1. Authorization: Bearer <key>
	if auth := c.GetHeader("Authorization"); auth != "" {
		trimmed := strings.TrimSpace(auth)
		if strings.HasPrefix(strings.ToLower(trimmed), "bearer ") {
			return strings.TrimSpace(trimmed[7:])
		}
		return trimmed
	}

	// 2. Anthropic x-api-key: <key>
	if k := c.GetHeader("x-api-key"); k != "" {
		return strings.TrimSpace(k)
	}

	// 3. Azure OpenAI api-key: <key>
	if k := c.GetHeader("api-key"); k != "" {
		return strings.TrimSpace(k)
	}

	// 4. Google Gemini x-goog-api-key: <key>
	if k := c.GetHeader("x-goog-api-key"); k != "" {
		return strings.TrimSpace(k)
	}

	// 5. URL query ?key=<key>
	if k := c.Query("key"); k != "" {
		return strings.TrimSpace(k)
	}

	return ""
}

// KeyAuthMiddleware enforces multi-tenant API key authentication when keys are registered in Console.
func KeyAuthMiddleware(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		// Skip Console management routes, native CPA management, OAuth, and public system endpoints
		if ShouldSkipGateway(path) {
			c.Next()
			return
		}

		rawKey := ExtractRawKey(c)

		// If Console has active registered keys, enforce key verification
		if eng.HasActiveKeys() {
			if rawKey == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": gin.H{
						"message": "Unauthorized: invalid or missing API Key",
						"type":    "authentication_error",
						"code":    "invalid_api_key",
					},
				})
				return
			}

			keyID, ok := eng.ResolveKeyID(rawKey)
			if !ok {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": gin.H{
						"message": "Unauthorized: invalid or inactive API Key",
						"type":    "authentication_error",
						"code":    "invalid_api_key",
					},
				})
				return
			}

			c.Set(ContextKeyID, keyID)
			c.Set(ContextRawKey, rawKey)
		} else if rawKey != "" {
			// If no keys registered yet, optionally associate if rawKey is known
			if keyID, ok := eng.ResolveKeyID(rawKey); ok {
				c.Set(ContextKeyID, keyID)
				c.Set(ContextRawKey, rawKey)
			}
		}

		c.Next()
	}
}
