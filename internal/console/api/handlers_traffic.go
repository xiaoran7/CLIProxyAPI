package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/traffichub"
)

// HandleGetTraffic returns the real-time visual gateway topology and stream snapshot.
func HandleGetTraffic() gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot := traffichub.Default().GetSnapshot()
		c.JSON(http.StatusOK, snapshot)
	}
}
