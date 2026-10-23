package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/middleware"
)

// HandleGetValidations returns pending and historical upstream validation challenges.
func HandleGetValidations(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		database := eng.DB()

		pending, err := database.SelectValidations(ctx, "pending")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query pending validations: %v", err)})
			return
		}

		history, err := database.SelectValidations(ctx, "")
		if err != nil {
			history = []db.UpstreamValidationRow{}
		}

		seen := make(map[int64]bool)
		var combined []db.UpstreamValidationRow

		for _, item := range pending {
			seen[item.ID] = true
			combined = append(combined, item)
		}
		for _, item := range history {
			if !seen[item.ID] {
				seen[item.ID] = true
				combined = append(combined, item)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"validations":   combined,
			"pending_count": len(pending),
		})
	}
}

// HandleResolveValidation marks an upstream validation challenge as resolved.
func HandleResolveValidation(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid validation id"})
			return
		}

		if err := eng.DB().ResolveValidation(c.Request.Context(), id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to resolve validation: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// HandleDismissValidation marks an upstream validation challenge as dismissed.
func HandleDismissValidation(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid validation id"})
			return
		}

		if err := eng.DB().DismissValidation(c.Request.Context(), id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to dismiss validation: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// HandleDeleteValidation permanently removes a validation challenge.
func HandleDeleteValidation(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid validation id"})
			return
		}

		if err := eng.DB().DeleteValidation(c.Request.Context(), id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to delete validation: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// HandleManualValidation allows manual insertion of an upstream validation challenge URL.
func HandleManualValidation(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			AuthIndex     string `json:"auth_index"`
			Provider      string `json:"provider"`
			AccountName   string `json:"account_name"`
			ValidationURL string `json:"validation_url"`
			ErrorMessage  string `json:"error_message"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		rawURL := strings.TrimSpace(payload.ValidationURL)
		if rawURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "validation_url is required"})
			return
		}

		valURL := middleware.ExtractGoogleValidationURL(rawURL)
		if valURL == "" {
			valURL = rawURL
		}
		if !strings.Contains(valURL, "accounts.google.com") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Please provide a valid Google validation URL (must contain accounts.google.com)"})
			return
		}

		authIndex := strings.TrimSpace(payload.AuthIndex)
		if authIndex == "" {
			authIndex = "manual"
		}
		provider := strings.TrimSpace(payload.Provider)
		if provider == "" {
			provider = "antigravity"
		}
		acctName := strings.TrimSpace(payload.AccountName)
		if acctName == "" {
			acctName = "Manual Verification"
		}
		errMsg := strings.TrimSpace(payload.ErrorMessage)
		if errMsg == "" {
			errMsg = "Manual entry validation challenge"
		}

		now := time.Now().Unix()
		row := &db.UpstreamValidationRow{
			TS:            now,
			AuthIndex:     authIndex,
			Provider:      provider,
			AccountName:   &acctName,
			ValidationURL: valURL,
			ErrorMessage:  &errMsg,
			Status:        "pending",
			UpdatedAt:     now,
		}

		id, err := eng.DB().InsertValidation(c.Request.Context(), row)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to record validation: %v", err)})
			return
		}

		row.ID = id
		c.JSON(http.StatusOK, gin.H{
			"success":        true,
			"record":         row,
			"validation_url": valURL,
		})
	}
}

// HandleGetQuotaSnapshots returns historical quota window percentages.
func HandleGetQuotaSnapshots(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		authIndex := strings.TrimSpace(c.Query("auth_index"))
		hoursStr := c.DefaultQuery("hours", "24")
		hours, err := strconv.Atoi(hoursStr)
		if err != nil || hours <= 0 {
			hours = 24
		}

		sinceTS := time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
		snapshots, err := eng.DB().SelectQuotaSnapshotsByAuth(c.Request.Context(), authIndex, sinceTS)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query quota snapshots: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"snapshots":  snapshots,
			"auth_index": authIndex,
			"hours":      hours,
		})
	}
}
