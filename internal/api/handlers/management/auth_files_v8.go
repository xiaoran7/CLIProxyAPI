package management

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// StartOAuthV8 dispatches provider login through the v8 OAuth namespace.
func (h *Handler) StartOAuthV8(c *gin.Context) {
	switch c.Param("provider") {
	case "claude":
		h.RequestAnthropicToken(c)
	case "codex":
		h.RequestCodexToken(c)
	case "antigravity":
		h.RequestAntigravityToken(c)
	case "kimi":
		h.RequestKimiToken(c)
	case "kimi-ai":
		h.RequestKimiAIToken(c)
	case "xai":
		h.RequestXAIToken(c)
	case "devin":
		h.RequestDevinToken(c)
	case "meta":
		h.RequestMetaToken(c)
	default:
		if !h.ServePluginAuthURL(c) {
			c.JSON(http.StatusNotFound, gin.H{"error": "provider_not_found"})
		}
	}
}
