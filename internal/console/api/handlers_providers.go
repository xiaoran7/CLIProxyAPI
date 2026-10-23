package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// HandleGetProviderRegistry returns all AI provider registry entries.
func HandleGetProviderRegistry(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := eng.DB().SelectProviderRegistry(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query provider registry: %v", err)})
			return
		}

		type ProviderItemResponse struct {
			ProviderKey   string        `json:"provider_key"`
			Label         string        `json:"label"`
			ModelPatterns []PatternItem `json:"model_patterns"`
			AuthPatterns  []PatternItem `json:"auth_patterns"`
			Source        string        `json:"source"`
			Enabled       bool          `json:"enabled"`
			SortOrder     int           `json:"sort_order"`
			CreatedAt     string        `json:"created_at"`
			UpdatedAt     string        `json:"updated_at"`
		}

		var items []ProviderItemResponse
		for _, r := range rows {
			var mps []PatternItem
			if r.ModelPatterns != "" {
				_ = json.Unmarshal([]byte(r.ModelPatterns), &mps)
			}
			var aps []PatternItem
			if r.AuthPatterns != "" {
				_ = json.Unmarshal([]byte(r.AuthPatterns), &aps)
			}

			items = append(items, ProviderItemResponse{
				ProviderKey:   r.ProviderKey,
				Label:         r.Label,
				ModelPatterns: mps,
				AuthPatterns:  aps,
				Source:        r.Source,
				Enabled:       r.Enabled == 1,
				SortOrder:     r.SortOrder,
				CreatedAt:     r.CreatedAt.Format("2006-01-02 15:04:05"),
				UpdatedAt:     r.UpdatedAt.Format("2006-01-02 15:04:05"),
			})
		}

		c.JSON(http.StatusOK, gin.H{"providers": items})
	}
}

// UpsertProviderPayload defines the JSON body for upserting a provider entry.
type UpsertProviderPayload struct {
	ProviderKey   string      `json:"provider_key"`
	Label         string      `json:"label"`
	ModelPatterns interface{} `json:"model_patterns"`
	AuthPatterns  interface{} `json:"auth_patterns"`
	Source        string      `json:"source"`
	Enabled       *bool       `json:"enabled"`
	SortOrder     *int        `json:"sort_order"`
}

// HandleUpsertProviderRegistry creates or updates an AI provider registry entry.
func HandleUpsertProviderRegistry(eng *engine.Engine, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload UpsertProviderPayload
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request payload: %v", err)})
			return
		}

		key := strings.ToLower(strings.TrimSpace(payload.ProviderKey))
		label := strings.TrimSpace(payload.Label)

		if err := ValidateProviderKey(key); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if label == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "label cannot be empty"})
			return
		}

		source := strings.TrimSpace(payload.Source)
		if source == "" {
			source = "manual"
		}

		enabledInt := 1
		if payload.Enabled != nil && !*payload.Enabled {
			enabledInt = 0
		}

		sortOrder := 100
		if payload.SortOrder != nil && *payload.SortOrder > 0 {
			sortOrder = *payload.SortOrder
		}

		mpStr := "[]"
		if payload.ModelPatterns != nil {
			b, _ := json.Marshal(payload.ModelPatterns)
			mpStr = string(b)
		}

		apStr := "[]"
		if payload.AuthPatterns != nil {
			b, _ := json.Marshal(payload.AuthPatterns)
			apStr = string(b)
		}

		ctx := c.Request.Context()
		row := &db.ProviderRegistryRow{
			ProviderKey:   key,
			Label:         label,
			ModelPatterns: mpStr,
			AuthPatterns:  apStr,
			Source:        source,
			Enabled:       enabledInt,
			SortOrder:     sortOrder,
		}

		if err := eng.DB().UpsertProviderRegistry(ctx, row); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to upsert provider: %v", err)})
			return
		}

		_ = providers.InitAndReload(ctx)

		c.JSON(http.StatusOK, gin.H{
			"success":      true,
			"provider_key": key,
		})
	}
}

// HandleDeleteProviderRegistry deletes a custom provider from the registry.
func HandleDeleteProviderRegistry(eng *engine.Engine, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.ToLower(strings.TrimSpace(c.Param("key")))
		if key == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provider_key is required"})
			return
		}

		ctx := c.Request.Context()
		if err := eng.DB().DeleteProviderRegistry(ctx, key); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to delete provider: %v", err)})
			return
		}

		_ = providers.InitAndReload(ctx)

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}
