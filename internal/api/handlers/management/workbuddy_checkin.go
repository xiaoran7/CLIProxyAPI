package management

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/auth/workbuddy"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

// GetWorkBuddyCheckin returns the current daily checkin records.
func (h *Handler) GetWorkBuddyCheckin(c *gin.Context) {
	engine := workbuddy.GetGlobalCheckinEngine()
	if engine == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
			"today":   workbuddy.TodayBeijing(),
			"records": map[string]any{},
		})
		return
	}

	authID := strings.TrimSpace(c.Query("auth_id"))
	if authID != "" {
		record, found := engine.GetStatus(authID)
		if !found {
			c.JSON(http.StatusOK, gin.H{
				"enabled": true,
				"today":   workbuddy.TodayBeijing(),
				"found":   false,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"enabled": true,
			"today":   workbuddy.TodayBeijing(),
			"found":   true,
			"record":  record,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled": true,
		"today":   workbuddy.TodayBeijing(),
		"records": engine.GetAllRecords(),
	})
}

// PostWorkBuddyCheckin triggers an immediate checkin for one or all WorkBuddy accounts.
func (h *Handler) PostWorkBuddyCheckin(c *gin.Context) {
	engine := workbuddy.GetGlobalCheckinEngine()
	authID := strings.TrimSpace(c.Query("auth_id"))
	if authID == "" {
		var req struct {
			AuthID string `json:"auth_id"`
		}
		if err := c.ShouldBindJSON(&req); err == nil {
			authID = strings.TrimSpace(req.AuthID)
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	if authID != "" {
		var targetAuth *cliproxyauth.Auth
		if h.authManager != nil {
			targetAuth, _ = h.authManager.GetByID(authID)
		}
		if targetAuth == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "auth not found"})
			return
		}

		res, err := workbuddy.Checkin(ctx, http.DefaultClient, targetAuth)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  err.Error(),
				"result": res,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"today":  workbuddy.TodayBeijing(),
			"result": res,
		})
		return
	}

	if engine != nil {
		results := engine.RunCheckinAll(ctx)
		c.JSON(http.StatusOK, gin.H{
			"today":   workbuddy.TodayBeijing(),
			"results": results,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"today":   workbuddy.TodayBeijing(),
		"results": map[string]any{},
	})
}
