package management

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/traffichub"
)

// GetTraffic handles GET /v0/management/traffic, returning real-time traffic records,
// active client IP statistics, and multi-stage topology flow data.
func (h *Handler) GetTraffic(c *gin.Context) {
	if c == nil {
		return
	}
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	snapshot := traffichub.Default().GetSnapshot()
	c.JSON(http.StatusOK, snapshot)
}
