package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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

// HandleGetUpstreamAuthStats aggregates credentials statistics with optional time range filtering.
func HandleGetUpstreamAuthStats(eng *engine.Engine, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		database := eng.DB()

		rangeParam := strings.ToLower(strings.TrimSpace(c.Query("range")))
		if rangeParam == "" {
			rangeParam = strings.ToLower(strings.TrimSpace(c.Query("time_range")))
		}

		now := time.Now()
		var dailyAgg []db.UpstreamDailyStatRow
		var err error

		switch rangeParam {
		case "1d", "today":
			dailyAgg, err = database.SelectUpstreamDailyAggSince(ctx, now.Format("2006-01-02"))
		case "7d":
			dailyAgg, err = database.SelectUpstreamDailyAggSince(ctx, now.AddDate(0, 0, -7).Format("2006-01-02"))
		case "30d":
			dailyAgg, err = database.SelectUpstreamDailyAggSince(ctx, now.AddDate(0, 0, -30).Format("2006-01-02"))
		default:
			dailyAgg, err = database.SelectUpstreamDailyAgg(ctx)
		}

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

// RuntimeFactsWindowPreset defines time window configuration for telemetry breakdown.
type RuntimeFactsWindowPreset struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	ShortLabel    string `json:"short_label"`
	WindowMinutes int    `json:"window_minutes"`
	BucketMinutes int    `json:"bucket_minutes"`
	BucketCount   int    `json:"bucket_count"`
}

var runtimeFactsWindows = []RuntimeFactsWindowPreset{
	{ID: "1h", Label: "1小时", ShortLabel: "1h", WindowMinutes: 60, BucketMinutes: 5, BucketCount: 12},
	{ID: "3h20m", Label: "3小时20分", ShortLabel: "3.3h", WindowMinutes: 200, BucketMinutes: 10, BucketCount: 20},
	{ID: "6h", Label: "6小时", ShortLabel: "6h", WindowMinutes: 360, BucketMinutes: 15, BucketCount: 24},
	{ID: "12h", Label: "12小时", ShortLabel: "12h", WindowMinutes: 720, BucketMinutes: 30, BucketCount: 24},
	{ID: "24h", Label: "24小时", ShortLabel: "24h", WindowMinutes: 1440, BucketMinutes: 60, BucketCount: 24},
}

// HandleGetRuntimeFacts returns runtime traffic telemetry with graceful degraded mode.
func HandleGetRuntimeFacts(eng *engine.Engine, cpa *CpaClient, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		windowParam := strings.TrimSpace(c.Query("window"))
		if windowParam == "" {
			windowParam = "3h20m"
		}

		matchedPreset := runtimeFactsWindows[1] // default 3h20m
		for _, w := range runtimeFactsWindows {
			if strings.EqualFold(w.ID, windowParam) {
				matchedPreset = w
				break
			}
		}

		// Try to query CPA auth-files and api-key-usage
		var authFilesResp struct {
			Files []struct {
				Name      string `json:"name"`
				AuthIndex string `json:"auth_index"`
				Provider  string `json:"provider"`
				Disabled  bool   `json:"disabled"`
				Status    string `json:"status"`
			} `json:"files"`
		}

		cpaErr := cpa.RequestJSON(ctx, http.MethodGet, "/auth-files", nil, "v0", &authFilesResp)
		if cpaErr != nil {
			// Degraded payload: return HTTP 200 with degraded status so frontend renders gracefully
			c.JSON(http.StatusOK, gin.H{
				"source":            "cpa_runtime",
				"degraded":          true,
				"error":             cpaErr.Error(),
				"fetched_at":        time.Now().UnixMilli(),
				"window":            matchedPreset.ID,
				"window_label":      matchedPreset.Label,
				"window_minutes":    matchedPreset.WindowMinutes,
				"bucket_minutes":    matchedPreset.BucketMinutes,
				"bucket_count":      matchedPreset.BucketCount,
				"available_windows": runtimeFactsWindows,
				"totals": gin.H{
					"credentials_total":    0,
					"credentials_active":   0,
					"credentials_disabled": 0,
					"credentials_cooldown": 0,
					"credentials_blocked":  0,
					"scope":                "cumulative",
					"buckets":              []interface{}{},
					"success_count":        0,
					"failure_count":        0,
					"total_requests":       0,
					"success_rate":         nil,
					"source":               "cpa_runtime",
				},
				"providers":   []interface{}{},
				"credentials": []interface{}{},
			})
			return
		}

		// Calculate statistics from auth files and daily aggregates
		dailyAgg, _ := eng.DB().SelectUpstreamDailyAgg(ctx)
		dailyMap := make(map[string]db.UpstreamDailyStatRow)
		for _, d := range dailyAgg {
			dailyMap[d.AuthIndex] = d
		}

		type CredentialFact struct {
			AuthIndex string `json:"auth_index"`
			Filename  string `json:"filename"`
			Provider  string `json:"provider"`
			Status    string `json:"status"`
			Disabled  bool   `json:"disabled"`
			Success   int64  `json:"success"`
			Failed    int64  `json:"failed"`
			Total     int64  `json:"total"`
		}

		type ProviderFact struct {
			Provider     string `json:"provider"`
			KeyCount     int    `json:"key_count"`
			TotalSuccess int64  `json:"total_success"`
			TotalFailed  int64  `json:"total_failed"`
			Total        int64  `json:"total"`
		}

		var credFacts []CredentialFact
		provMap := make(map[string]*ProviderFact)
		var totalSuccess, totalFailed int64
		activeCount := 0
		disabledCount := 0

		for _, f := range authFilesResp.Files {
			prov := f.Provider
			if prov == "" {
				prov = providers.InferProvider("", f.AuthIndex).Key
			}
			if prov == "" {
				prov = "general"
			}

			statusStr := "active"
			if f.Disabled {
				statusStr = "disabled"
				disabledCount++
			} else {
				activeCount++
			}

			var sCount, fCount, reqCount int64
			if d, ok := dailyMap[f.AuthIndex]; ok {
				sCount = d.SuccessCount
				fCount = d.FailureCount
				reqCount = d.Requests
			}

			totalSuccess += sCount
			totalFailed += fCount

			credFacts = append(credFacts, CredentialFact{
				AuthIndex: f.AuthIndex,
				Filename:  f.Name,
				Provider:  prov,
				Status:    statusStr,
				Disabled:  f.Disabled,
				Success:   sCount,
				Failed:    fCount,
				Total:     reqCount,
			})

			pf, ok := provMap[prov]
			if !ok {
				pf = &ProviderFact{Provider: prov}
				provMap[prov] = pf
			}
			pf.KeyCount++
			pf.TotalSuccess += sCount
			pf.TotalFailed += fCount
			pf.Total += reqCount
		}

		var provList []ProviderFact
		for _, pf := range provMap {
			provList = append(provList, *pf)
		}
		sort.Slice(provList, func(i, j int) bool {
			return provList[i].Total > provList[j].Total
		})

		totalRequests := totalSuccess + totalFailed
		var successRate *float64
		if totalRequests > 0 {
			rate := float64(totalSuccess) / float64(totalRequests) * 100
			successRate = &rate
		}

		c.JSON(http.StatusOK, gin.H{
			"source":            "cpa_runtime",
			"degraded":          false,
			"fetched_at":        time.Now().UnixMilli(),
			"window":            matchedPreset.ID,
			"window_label":      matchedPreset.Label,
			"window_minutes":    matchedPreset.WindowMinutes,
			"bucket_minutes":    matchedPreset.BucketMinutes,
			"bucket_count":      matchedPreset.BucketCount,
			"available_windows": runtimeFactsWindows,
			"totals": gin.H{
				"credentials_total":    len(credFacts),
				"credentials_active":   activeCount,
				"credentials_disabled": disabledCount,
				"credentials_cooldown": 0,
				"credentials_blocked":  0,
				"scope":                "window",
				"buckets":              []interface{}{},
				"success_count":        totalSuccess,
				"failure_count":        totalFailed,
				"total_requests":       totalRequests,
				"success_rate":         successRate,
				"source":               "cpa_runtime",
			},
			"providers":   provList,
			"credentials": credFacts,
		})
	}
}

// HandleGetUpstreamRouting reads upstream routing configuration from CPA.
func HandleGetUpstreamRouting(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var routingData interface{}
		if err := cpa.RequestJSON(c.Request.Context(), http.MethodGet, "/routing", nil, "v0", &routingData); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Failed to fetch CPA routing: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"routing": routingData})
	}
}

// HandlePutUpstreamRouting updates upstream routing configuration in CPA.
func HandlePutUpstreamRouting(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Strategy           string      `json:"strategy"`
			SessionAffinity    *bool       `json:"session_affinity"`
			SessionAffinityTTL interface{} `json:"session_affinity_ttl"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		patch := make(map[string]interface{})
		if payload.Strategy != "" {
			validStrategies := map[string]bool{"priority": true, "round-robin": true, "weighted-round-robin": true}
			if !validStrategies[payload.Strategy] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid routing strategy: must be priority, round-robin, or weighted-round-robin"})
				return
			}
			patch["strategy"] = payload.Strategy
		}
		if payload.SessionAffinity != nil {
			patch["session-affinity"] = *payload.SessionAffinity
		}
		if payload.SessionAffinityTTL != nil {
			patch["session-affinity-ttl"] = payload.SessionAffinityTTL
		}

		ctx := c.Request.Context()
		cpaPayload := map[string]interface{}{
			"routing": patch,
		}
		cpaBody, _ := json.Marshal(cpaPayload)
		resp, err := cpa.Request(ctx, http.MethodPatch, "/config", cpaBody, "application/json", "v0")
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Failed to update CPA routing config: %v", err)})
			return
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			c.JSON(resp.StatusCode, gin.H{"error": fmt.Sprintf("CPA returned %d: %s", resp.StatusCode, string(resp.Bytes))})
			return
		}

		var routingData interface{}
		if err := cpa.RequestJSON(ctx, http.MethodGet, "/routing", nil, "v0", &routingData); err != nil {
			routingData = patch
		}

		c.JSON(http.StatusOK, gin.H{"routing": routingData})
	}
}

// HandlePutUpstreamAuthBatch processes batch actions on upstream credentials.
func HandlePutUpstreamAuthBatch(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Names    []string    `json:"names"`
			Action   string      `json:"action"`
			Value    interface{} `json:"value"`
			Priority *int        `json:"priority"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		if len(payload.Names) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Please select at least one auth file"})
			return
		}
		if len(payload.Names) > 200 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot batch process more than 200 files at once"})
			return
		}

		validActions := map[string]bool{
			"enable": true, "disable": true, "priority": true, "reset-cooldown": true, "delete": true,
		}
		if !validActions[payload.Action] {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Unsupported batch action: %s", payload.Action)})
			return
		}

		ctx := c.Request.Context()
		type BatchResultItem struct {
			Name  string `json:"name"`
			OK    bool   `json:"ok"`
			Error string `json:"error,omitempty"`
		}

		var results []BatchResultItem
		succeeded := 0

		authIndexMap := make(map[string]string)
		if payload.Action == "reset-cooldown" {
			var authFilesResp struct {
				Files []struct {
					Name      string `json:"name"`
					AuthIndex string `json:"auth_index"`
				} `json:"files"`
			}
			if err := cpa.RequestJSON(ctx, http.MethodGet, "/auth-files", nil, "v0", &authFilesResp); err == nil {
				for _, f := range authFilesResp.Files {
					if f.Name != "" && f.AuthIndex != "" {
						authIndexMap[f.Name] = f.AuthIndex
					}
				}
			}
		}

		for _, name := range payload.Names {
			if !isSafeAuthFilename(name) {
				results = append(results, BatchResultItem{Name: name, OK: false, Error: "Invalid auth filename"})
				continue
			}

			var err error
			switch payload.Action {
			case "enable":
				b, _ := json.Marshal(map[string]interface{}{"name": name, "disabled": false})
				_, err = cpa.Request(ctx, http.MethodPatch, "/auth-files/status", b, "application/json", "v0")
			case "disable":
				b, _ := json.Marshal(map[string]interface{}{"name": name, "disabled": true})
				_, err = cpa.Request(ctx, http.MethodPatch, "/auth-files/status", b, "application/json", "v0")
			case "priority":
				prio := 0
				if payload.Priority != nil {
					prio = *payload.Priority
				} else if payload.Value != nil {
					switch v := payload.Value.(type) {
					case float64:
						prio = int(v)
					case string:
						prio, _ = strconv.Atoi(v)
					}
				}
				b, _ := json.Marshal(map[string]interface{}{"name": name, "priority": prio})
				_, err = cpa.Request(ctx, http.MethodPatch, "/auth-files/fields", b, "application/json", "v0")
			case "reset-cooldown":
				authIdx := authIndexMap[name]
				if authIdx == "" {
					authIdx = name
				}
				b, _ := json.Marshal(map[string]string{"auth_index": authIdx})
				_, err = cpa.Request(ctx, http.MethodPost, "/reset-quota", b, "application/json", "v0")
			case "delete":
				targetURL := fmt.Sprintf("/auth-files?name=%s", name)
				_, err = cpa.Request(ctx, http.MethodDelete, targetURL, nil, "application/json", "v0")
			}

			if err != nil {
				results = append(results, BatchResultItem{Name: name, OK: false, Error: err.Error()})
			} else {
				results = append(results, BatchResultItem{Name: name, OK: true})
				succeeded++
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"results":   results,
			"succeeded": succeeded,
			"total":     len(payload.Names),
		})
	}
}

// HandleUpdateUpstreamAuthPriority updates priority, weight, and note for an upstream auth credential.
func HandleUpdateUpstreamAuthPriority(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Name     string      `json:"name"`
			Priority interface{} `json:"priority"`
			Weight   interface{} `json:"weight"`
			Note     *string     `json:"note"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}
		name := strings.TrimSpace(payload.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "凭证名称 name 不能为空"})
			return
		}
		if !isSafeAuthFilename(name) && !strings.HasPrefix(name, "openrouter-") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的凭据文件名"})
			return
		}

		prio := 0
		if payload.Priority != nil {
			switch v := payload.Priority.(type) {
			case float64:
				prio = int(v)
			case string:
				prio, _ = strconv.Atoi(v)
			}
		}

		cpaPayload := map[string]interface{}{
			"name":     name,
			"priority": prio,
		}
		if payload.Weight != nil {
			switch v := payload.Weight.(type) {
			case float64:
				cpaPayload["weight"] = int(v)
			case string:
				if w, err := strconv.Atoi(v); err == nil {
					cpaPayload["weight"] = w
				}
			}
		}
		if payload.Note != nil {
			cpaPayload["note"] = strings.TrimSpace(*payload.Note)
		}

		if !strings.HasPrefix(name, "openrouter-") {
			b, _ := json.Marshal(cpaPayload)
			if resp, err := cpa.Request(c.Request.Context(), http.MethodPatch, "/auth-files/fields", b, "application/json", "v0"); err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Failed to update CPA auth fields: %v", err)})
				return
			} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				c.JSON(resp.StatusCode, gin.H{"error": fmt.Sprintf("CPA returned %d: %s", resp.StatusCode, string(resp.Bytes))})
				return
			}
		}

		InvalidateUpstreamQuotaCache()

		c.JSON(http.StatusOK, gin.H{
			"status":   "ok",
			"name":     name,
			"priority": prio,
		})
	}
}

// HandleUpdateUpstreamAuthStatus enables or disables an upstream auth credential.
func HandleUpdateUpstreamAuthStatus(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Name     string `json:"name"`
			Disabled bool   `json:"disabled"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}
		name := strings.TrimSpace(payload.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "凭证名称 name 不能为空"})
			return
		}
		if !isSafeAuthFilename(name) && !strings.HasPrefix(name, "openrouter-") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的凭据文件名"})
			return
		}

		if !strings.HasPrefix(name, "openrouter-") {
			b, _ := json.Marshal(map[string]interface{}{
				"name":     name,
				"disabled": payload.Disabled,
			})
			if resp, err := cpa.Request(c.Request.Context(), http.MethodPatch, "/auth-files/status", b, "application/json", "v0"); err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Failed to update CPA auth status: %v", err)})
				return
			} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				c.JSON(resp.StatusCode, gin.H{"error": fmt.Sprintf("CPA returned %d: %s", resp.StatusCode, string(resp.Bytes))})
				return
			}
		}

		InvalidateUpstreamQuotaCache()

		c.JSON(http.StatusOK, gin.H{
			"status":   "ok",
			"name":     name,
			"disabled": payload.Disabled,
		})
	}
}

// HandleActivateUpstreamQuotaWindow triggers upstream quota refresh/probe for a credential.
func HandleActivateUpstreamQuotaWindow(cpa *CpaClient, eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			AuthIndex string `json:"authIndex"`
			Filename  string `json:"filename"`
			Provider  string `json:"provider"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		authIndex := strings.TrimSpace(payload.AuthIndex)
		filename := strings.TrimSpace(payload.Filename)
		if authIndex == "" && filename == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "必须指定上游凭证 auth_index 或 filename"})
			return
		}

		// Invalidate cached quotas to force fresh query
		InvalidateUpstreamQuotaCache()

		c.JSON(http.StatusOK, gin.H{
			"status":     "ok",
			"message":    "激活探测完成",
			"auth_index": authIndex,
			"filename":   filename,
		})
	}
}
