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
	snapshot := traffichub.Default().GetSnapshot()
	c.JSON(http.StatusOK, snapshot)
}
