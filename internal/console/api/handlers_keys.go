package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// GenerateRandomAPIKey generates a secure random API key prefixed with sk-cpa-.
func GenerateRandomAPIKey() string {
	bytes := make([]byte, 24)
	_, _ = rand.Read(bytes)
	return "sk-cpa-" + hex.EncodeToString(bytes)
}

// MaskAPIKey masks sensitive key material for display.
func MaskAPIKey(raw string) string {
	if len(raw) <= 10 {
		return "sk-***"
	}
	prefixLen := 6
	if len(raw) < 14 {
		prefixLen = 3
	}
	return raw[:prefixLen] + "..." + raw[len(raw)-4:]
}

// HandleGetKeys returns a list of all API keys with active quotas and live usage stats.
func HandleGetKeys(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		database := eng.DB()

		keys, err := database.SelectAllKeys(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query keys: %v", err)})
			return
		}

		rules, err := database.SelectAllQuotaRules(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query quota rules: %v", err)})
			return
		}

		ruleMap := make(map[int64]db.QuotaRuleRow)
		for _, r := range rules {
			ruleMap[r.KeyID] = r
		}

		allTimeStats, err := database.SelectDailyAllTimeByAllKeys(ctx)
		if err != nil {
			allTimeStats = make(map[int64]*db.DailyStatsSummary)
		}

		type KeyItemResponse struct {
			ID         int64  `json:"id"`
			DisplayKey string `json:"display_key"`
			Alias      string `json:"alias"`
			Active     int    `json:"active"`
			CreatedAt  string `json:"created_at"`
			UpdatedAt  string `json:"updated_at"`
			Quota      gin.H  `json:"quota"`
			TodayUsage gin.H  `json:"today_usage"`
			AllTime    gin.H  `json:"all_time"`
		}

		var items []KeyItemResponse
		for _, k := range keys {
			r, hasRule := ruleMap[k.ID]
			var whitelist []string
			if hasRule && strings.TrimSpace(r.ModelWhitelist) != "" {
				_ = json.Unmarshal([]byte(r.ModelWhitelist), &whitelist)
			}
			if whitelist == nil {
				whitelist = []string{}
			}

			todayTokens := eng.Memory().GetTodayUsageForKey(k.ID)
			todayRequests := eng.Memory().GetTodayRequestsForKey(k.ID)

			allTimeTokens := int64(0)
			allTimeRequests := int64(0)
			if stat, ok := allTimeStats[k.ID]; ok && stat != nil {
				allTimeTokens = stat.TotalTokens
				allTimeRequests = stat.Requests
			}

			hardRatio := 1.2
			rpmLimit := 0
			var dailyLimit *int64
			enabled := false
			modelGroup := ""
			if hasRule {
				enabled = r.Enabled == 1
				dailyLimit = r.DailyLimit
				if r.HardRatio > 0 {
					hardRatio = r.HardRatio
				}
				rpmLimit = r.RPMLimit
				modelGroup = r.ModelGroup
			}

			items = append(items, KeyItemResponse{
				ID:         k.ID,
				DisplayKey: MaskAPIKey(k.APIKey),
				Alias:      k.Alias,
				Active:     k.Active,
				CreatedAt:  k.CreatedAt.Format("2006-01-02 15:04:05"),
				UpdatedAt:  k.UpdatedAt.Format("2006-01-02 15:04:05"),
				Quota: gin.H{
					"enabled":         enabled,
					"daily_limit":     dailyLimit,
					"hard_ratio":      hardRatio,
					"rpm_limit":       rpmLimit,
					"model_whitelist": whitelist,
					"model_group":     modelGroup,
				},
				TodayUsage: gin.H{
					"tokens":   todayTokens,
					"requests": todayRequests,
				},
				AllTime: gin.H{
					"tokens":   allTimeTokens,
					"requests": allTimeRequests,
				},
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"keys":  items,
			"total": len(items),
		})
	}
}

// CreateKeyPayload defines the JSON body for creating a key.
type CreateKeyPayload struct {
	APIKey         string      `json:"api_key"`
	Alias          string      `json:"alias"`
	Enabled        *bool       `json:"enabled"`
	DailyLimit     *int64      `json:"daily_limit"`
	HardRatio      *float64    `json:"hard_ratio"`
	RPMLimit       *int        `json:"rpm_limit"`
	ModelWhitelist interface{} `json:"model_whitelist"`
	ModelGroup     string      `json:"model_group"`
}

// HandleCreateKey creates a new API key and associated quota rule.
func HandleCreateKey(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload CreateKeyPayload
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request payload: %v", err)})
			return
		}

		rawKey := strings.TrimSpace(payload.APIKey)
		if rawKey == "" {
			rawKey = GenerateRandomAPIKey()
		}

		alias := strings.TrimSpace(payload.Alias)
		if alias == "" {
			alias = "API Key"
		}

		ctx := c.Request.Context()
		database := eng.DB()

		keyID, err := database.InsertKey(ctx, rawKey, alias)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to insert key: %v", err)})
			return
		}

		// Configure initial quota rule
		enabledInt := 0
		if payload.Enabled != nil && *payload.Enabled {
			enabledInt = 1
		}
		hardRatio := 1.2
		if payload.HardRatio != nil && *payload.HardRatio > 0 {
			hardRatio = *payload.HardRatio
		}
		rpmLimit := 0
		if payload.RPMLimit != nil && *payload.RPMLimit >= 0 {
			rpmLimit = *payload.RPMLimit
		}

		whitelistStr := "[]"
		if payload.ModelWhitelist != nil {
			switch v := payload.ModelWhitelist.(type) {
			case []interface{}:
				var strs []string
				for _, item := range v {
					if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
						strs = append(strs, strings.TrimSpace(s))
					}
				}
				b, _ := json.Marshal(strs)
				whitelistStr = string(b)
			case string:
				whitelistStr = v
			}
		}

		rule := &db.QuotaRuleRow{
			KeyID:          keyID,
			Enabled:        enabledInt,
			DailyLimit:     payload.DailyLimit,
			HardRatio:      hardRatio,
			RPMLimit:       rpmLimit,
			ModelWhitelist: whitelistStr,
			ModelGroup:     payload.ModelGroup,
		}
		_ = database.UpsertQuotaRule(ctx, rule)

		_ = eng.ReloadState(ctx)

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"key_id":  keyID,
			"api_key": rawKey,
			"alias":   alias,
		})
	}
}

// HandleRevealKey returns the plaintext raw API key for inspection.
func HandleRevealKey(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		keyID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || keyID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key id"})
			return
		}

		keyRow, err := eng.DB().SelectKeyByID(c.Request.Context(), keyID)
		if err != nil || keyRow == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Key not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"key_id":  keyID,
			"api_key": keyRow.APIKey,
			"alias":   keyRow.Alias,
		})
	}
}

// HandleRotateKey rotates the API key secret value and updates in-memory mapping.
func HandleRotateKey(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		keyID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || keyID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key id"})
			return
		}

		newKey := GenerateRandomAPIKey()
		ctx := c.Request.Context()

		if err := eng.DB().UpdateKeyRaw(ctx, keyID, newKey); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to rotate key: %v", err)})
			return
		}

		_ = eng.ReloadState(ctx)

		c.JSON(http.StatusOK, gin.H{
			"success":     true,
			"key_id":      keyID,
			"new_api_key": newKey,
		})
	}
}

// HandleUpdateKeyAlias updates the user-facing alias of an API key.
func HandleUpdateKeyAlias(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		keyID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || keyID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key id"})
			return
		}

		var payload struct {
			Alias string `json:"alias"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		alias := strings.TrimSpace(payload.Alias)
		if alias == "" {
			alias = "API Key"
		}

		ctx := c.Request.Context()
		if err := eng.DB().UpdateAlias(ctx, keyID, alias); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update alias: %v", err)})
			return
		}

		_ = eng.ReloadState(ctx)

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"key_id":  keyID,
			"alias":   alias,
		})
	}
}

// HandleDeleteKey permanently deletes an API key and associated quota rules.
func HandleDeleteKey(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		keyID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || keyID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key id"})
			return
		}

		ctx := c.Request.Context()
		if err := eng.DB().DeleteKey(ctx, keyID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to delete key: %v", err)})
			return
		}

		_ = eng.ReloadState(ctx)

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"key_id":  keyID,
		})
	}
}

// QuotaUpdatePayload defines the JSON body for updating a quota rule.
type QuotaUpdatePayload struct {
	Enabled        *bool       `json:"enabled"`
	DailyLimit     *int64      `json:"daily_limit"`
	HardRatio      *float64    `json:"hard_ratio"`
	RPMLimit       *int        `json:"rpm_limit"`
	ModelWhitelist interface{} `json:"model_whitelist"`
	ModelGroup     string      `json:"model_group"`
}

// HandleUpdateQuotaRule updates or creates the quota rule for a specific key.
func HandleUpdateQuotaRule(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		keyID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || keyID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key id"})
			return
		}

		var payload QuotaUpdatePayload
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request payload: %v", err)})
			return
		}

		enabledInt := 0
		if payload.Enabled != nil && *payload.Enabled {
			enabledInt = 1
		}

		hardRatio := 1.2
		if payload.HardRatio != nil && *payload.HardRatio > 0 {
			hardRatio = *payload.HardRatio
		}

		rpmLimit := 0
		if payload.RPMLimit != nil && *payload.RPMLimit >= 0 {
			rpmLimit = *payload.RPMLimit
		}

		whitelistStr := "[]"
		if payload.ModelWhitelist != nil {
			switch v := payload.ModelWhitelist.(type) {
			case []interface{}:
				var strs []string
				for _, item := range v {
					if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
						strs = append(strs, strings.TrimSpace(s))
					}
				}
				b, _ := json.Marshal(strs)
				whitelistStr = string(b)
			case string:
				whitelistStr = v
			}
		}

		rule := &db.QuotaRuleRow{
			KeyID:          keyID,
			Enabled:        enabledInt,
			DailyLimit:     payload.DailyLimit,
			HardRatio:      hardRatio,
			RPMLimit:       rpmLimit,
			ModelWhitelist: whitelistStr,
			ModelGroup:     payload.ModelGroup,
		}

		ctx := c.Request.Context()
		if err := eng.DB().UpsertQuotaRule(ctx, rule); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update quota rule: %v", err)})
			return
		}

		_ = eng.ReloadState(ctx)

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"key_id":  keyID,
		})
	}
}

// HandleDeleteQuotaRule deletes the quota rule for a specific key.
func HandleDeleteQuotaRule(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		keyID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || keyID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid key id"})
			return
		}

		ctx := c.Request.Context()
		if err := eng.DB().DeleteQuotaRule(ctx, keyID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to delete quota rule: %v", err)})
			return
		}

		_ = eng.ReloadState(ctx)

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"key_id":  keyID,
		})
	}
}
