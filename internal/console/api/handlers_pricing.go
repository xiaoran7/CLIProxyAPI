package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// HandleGetModelPrices returns all model pricing rules.
func HandleGetModelPrices(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		prices, err := eng.DB().SelectAllModelPrices(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query model prices: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"prices": prices})
	}
}

// UpsertModelPricePayload defines the payload for creating or modifying a model price.
type UpsertModelPricePayload struct {
	Pattern         string   `json:"pattern"`
	ModelPattern    string   `json:"model_pattern"`
	PromptPrice     *float64 `json:"prompt_price"`
	CompletionPrice *float64 `json:"completion_price"`
	CacheReadPrice  *float64 `json:"cache_read_price"`
	Multiplier      *float64 `json:"multiplier"`
}

// HandleUpsertModelPrice inserts or updates a model pricing rule.
func HandleUpsertModelPrice(eng *engine.Engine, pricing *PricingEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload UpsertModelPricePayload
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request payload: %v", err)})
			return
		}

		pat := strings.TrimSpace(payload.Pattern)
		if pat == "" {
			pat = strings.TrimSpace(payload.ModelPattern)
		}
		if pat == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pattern is required"})
			return
		}

		promptPrice := 0.50
		if payload.PromptPrice != nil && *payload.PromptPrice >= 0 {
			promptPrice = *payload.PromptPrice
		}
		completionPrice := 1.50
		if payload.CompletionPrice != nil && *payload.CompletionPrice >= 0 {
			completionPrice = *payload.CompletionPrice
		}
		cacheReadPrice := 0.10
		if payload.CacheReadPrice != nil && *payload.CacheReadPrice >= 0 {
			cacheReadPrice = *payload.CacheReadPrice
		}
		multiplier := 1.0
		if payload.Multiplier != nil && *payload.Multiplier > 0 {
			multiplier = *payload.Multiplier
		}

		ctx := c.Request.Context()
		if err := eng.DB().UpsertModelPrice(ctx, pat, promptPrice, completionPrice, cacheReadPrice, multiplier); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to upsert model price: %v", err)})
			return
		}

		_ = pricing.InitAndReload(ctx)

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// HandleDeleteModelPrice deletes a model pricing rule by ID.
func HandleDeleteModelPrice(eng *engine.Engine, pricing *PricingEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid price id"})
			return
		}

		ctx := c.Request.Context()
		if err := eng.DB().DeleteModelPrice(ctx, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to delete model price: %v", err)})
			return
		}

		_ = pricing.InitAndReload(ctx)

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}
