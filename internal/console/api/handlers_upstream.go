package api

import (
	"encoding/json"
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

// HandleGetUpstreamAuthStats aggregates credentials statistics.
func HandleGetUpstreamAuthStats(eng *engine.Engine, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		database := eng.DB()

		dailyAgg, err := database.SelectUpstreamDailyAgg(ctx)
		if err != nil {
			dailyAgg = []db.UpstreamDailyStatRow{}
		}

		baselines, err := database.SelectBaselinesAll(ctx)
		if err != nil {
			baselines = []db.UpstreamAuthBaselineRow{}
		}

		type AuthStatResponseItem struct {
			AuthIndex        string `json:"auth_index"`
			Provider         string `json:"provider"`
			ProviderLabel    string `json:"provider_label"`
			Name             string `json:"name"`
			Requests         int64  `json:"requests"`
			SuccessCount     int64  `json:"success_count"`
			FailureCount     int64  `json:"failure_count"`
			PromptTokens     int64  `json:"prompt_tokens"`
			CompletionTokens int64  `json:"completion_tokens"`
			CachedTokens     int64  `json:"cached_tokens"`
			TotalTokens      int64  `json:"total_tokens"`
		}

		baseMap := make(map[string]db.UpstreamAuthBaselineRow)
		for _, b := range baselines {
			baseMap[b.AuthIndex] = b
		}

		var items []AuthStatResponseItem
		for _, d := range dailyAgg {
			name := d.AuthIndex
			prov := d.Provider
			if b, ok := baseMap[d.AuthIndex]; ok {
				if b.Name != nil && *b.Name != "" {
					name = *b.Name
				}
				if b.Provider != nil && *b.Provider != "" {
					prov = *b.Provider
				}
			}

			provRes := providers.InferProvider("", d.AuthIndex)
			if prov == "" {
				prov = provRes.Key
			}
			provLabel := provRes.Label
			if provLabel == "" {
				provLabel = providers.GetProviderLabel(prov)
			}

			items = append(items, AuthStatResponseItem{
				AuthIndex:        d.AuthIndex,
				Provider:         prov,
				ProviderLabel:    provLabel,
				Name:             name,
				Requests:         d.Requests,
				SuccessCount:     d.SuccessCount,
				FailureCount:     d.FailureCount,
				PromptTokens:     d.PromptTokens,
				CompletionTokens: d.CompletionTokens,
				CachedTokens:     d.CachedTokens,
				TotalTokens:      d.TotalTokens,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"auth_stats": items,
			"total":      len(items),
		})
	}
}
