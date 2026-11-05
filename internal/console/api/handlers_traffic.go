package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/traffichub"
)

// HandleGetTraffic returns the real-time visual gateway topology and stream snapshot.
func HandleGetTraffic() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")

		snapshot := traffichub.Default().GetSnapshot()
		c.JSON(http.StatusOK, snapshot)
	}
}
