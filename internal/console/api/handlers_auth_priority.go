package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// HandlePutUpstreamAuthPriority updates priority, weight, and note for a credential
func HandlePutUpstreamAuthPriority(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Name     string `json:"name"`
			Priority int    `json:"priority"`
			Weight   *int   `json:"weight"`
			Note     string `json:"note"`
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

		if payload.Priority < 0 || payload.Priority > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "优先级 priority 必须在 0 到 100 之间"})
			return
		}

		weight := defaultAuthWeight
		if payload.Weight != nil {
			if *payload.Weight < 1 || *payload.Weight > 1000 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "权重 weight 必须在 1 到 1000 之间"})
				return
			}
			weight = *payload.Weight
		}

		ctx := c.Request.Context()
		bodyBytes, _ := json.Marshal(map[string]interface{}{
			"name":     name,
			"priority": payload.Priority,
			"weight":   weight,
			"note":     payload.Note,
		})

		if !strings.HasPrefix(name, "openrouter-") {
			resp, err := cpa.Request(ctx, http.MethodPatch, "/auth-files/fields", bodyBytes, "application/json", "v0")
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Failed to update CPA auth fields: %v", err)})
				return
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				var errResp map[string]interface{}
				if json.Unmarshal(resp.Bytes, &errResp) == nil && errResp["error"] != nil {
					c.JSON(resp.StatusCode, gin.H{"error": fmt.Sprintf("%v", errResp["error"])})
				} else {
					c.JSON(resp.StatusCode, gin.H{"error": fmt.Sprintf("core auth manager returned %d: %s", resp.StatusCode, string(resp.Bytes))})
				}
				return
			}
		}

		InvalidateUpstreamQuotaCache()

		c.JSON(http.StatusOK, gin.H{
			"status":   "ok",
			"name":     name,
			"priority": payload.Priority,
			"weight":   weight,
		})
	}
}

// HandlePutUpstreamAuthStatus enables or disables an upstream credential
func HandlePutUpstreamAuthStatus(cpa *CpaClient) gin.HandlerFunc {
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

		ctx := c.Request.Context()
		bodyBytes, _ := json.Marshal(map[string]interface{}{
			"name":     name,
			"disabled": payload.Disabled,
		})

		if !strings.HasPrefix(name, "openrouter-") {
			resp, err := cpa.Request(ctx, http.MethodPatch, "/auth-files/status", bodyBytes, "application/json", "v0")
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Failed to update CPA auth status: %v", err)})
				return
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				var errResp map[string]interface{}
				if json.Unmarshal(resp.Bytes, &errResp) == nil && errResp["error"] != nil {
					c.JSON(resp.StatusCode, gin.H{"error": fmt.Sprintf("%v", errResp["error"])})
				} else {
					c.JSON(resp.StatusCode, gin.H{"error": fmt.Sprintf("core auth manager returned %d: %s", resp.StatusCode, string(resp.Bytes))})
				}
				return
			}
		}

		InvalidateUpstreamQuotaCache()

		statusStr := "active"
		if payload.Disabled {
			statusStr = "disabled"
		}

		c.JSON(http.StatusOK, gin.H{
			"status":      "ok",
			"name":        name,
			"disabled":    payload.Disabled,
			"auth_status": statusStr,
		})
	}
}

// HandlePostUpstreamAuthActivateWindow triggers on-demand quota probe for an upstream credential
func HandlePostUpstreamAuthActivateWindow(eng *engine.Engine, cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Filename  string `json:"filename"`
			Provider  string `json:"provider"`
			AuthIndex string `json:"authIndex"`
			AuthIdx   string `json:"auth_index"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		authIdx := strings.TrimSpace(payload.AuthIndex)
		if authIdx == "" {
			authIdx = strings.TrimSpace(payload.AuthIdx)
		}
		fname := strings.TrimSpace(payload.Filename)
		prov := strings.ToLower(strings.TrimSpace(payload.Provider))

		if authIdx == "" && fname == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "必须指定上游凭证 auth_index 或 filename"})
			return
		}

		ctx := c.Request.Context()

		// Resolve missing auth_index or provider from /auth-files if possible
		if authIdx == "" && fname != "" {
			var authFilesResp CpaAuthFilesResponse
			if err := cpa.RequestJSON(ctx, http.MethodGet, "/auth-files", nil, "v0", &authFilesResp); err == nil {
				for _, f := range authFilesResp.Files {
					if strings.EqualFold(f.Name, fname) || strings.EqualFold(f.ID, fname) {
						if f.AuthIndex != "" {
							authIdx = strings.TrimSpace(f.AuthIndex)
						}
						if prov == "" {
							p := f.Provider
							if p == "" {
								p = f.Type
							}
							prov = strings.ToLower(strings.TrimSpace(p))
						}
						break
					}
				}
			}
		}

		if authIdx != "" {
			resetBody, _ := json.Marshal(map[string]string{"auth_index": authIdx})
			_, _ = cpa.Request(ctx, http.MethodPost, "/reset-quota", resetBody, "application/json", "v0")
		}
		if fname != "" {
			refreshBody, _ := json.Marshal(map[string]string{"name": fname})
			_, _ = cpa.Request(ctx, http.MethodPost, "/auth-files/refresh", refreshBody, "application/json", "v0")
		}

		groups, planType, planName, err := fetchSingleAccountQuota(ctx, eng, cpa, authIdx, prov, "", "", fname)
		InvalidateUpstreamQuotaCache()

		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"error":   err.Error(),
				"message": fmt.Sprintf("拉取上游额度失败: %v", err),
			})
			return
		}

		totCreds, remCreds, usedCreds, expAt := extractAccountCredits(groups)
		c.JSON(http.StatusOK, gin.H{
			"success":           true,
			"message":           "已成功拉取上游额度",
			"groups":            groups,
			"plan_type":         planType,
			"plan_name":         planName,
			"total_credits":     totCreds,
			"remaining_credits": remCreds,
			"used_credits":      usedCreds,
			"expires_at":        expAt,
		})
	}
}
