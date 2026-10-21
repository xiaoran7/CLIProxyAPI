package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var safeAuthFilenameRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@-]{0,127}\.json$`)

func isSafeAuthFilename(name string) bool {
	return safeAuthFilenameRegex.MatchString(name) && !strings.Contains(name, "..")
}

// RegisterCpaRoutes mounts all CPA loopback management endpoints.
func RegisterCpaRoutes(rg *gin.RouterGroup, cpa *CpaClient) {
	// Config YAML
	rg.GET("/config-yaml", func(c *gin.Context) {
		resp, err := cpa.Request(c.Request.Context(), http.MethodGet, "/config.yaml", nil, "application/yaml", "v0")
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Type", "application/yaml; charset=utf-8")
		if resp.ETag != "" {
			c.Header("ETag", fmt.Sprintf("\"%s\"", resp.ETag))
		}
		c.Header("Cache-Control", "no-store")
		c.Data(resp.StatusCode, "application/yaml; charset=utf-8", resp.Bytes)
	})

	rg.PUT("/config-yaml", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
			return
		}

		expected := strings.Trim(c.GetHeader("If-Match"), "\"")
		if expected != "" {
			currentResp, err := cpa.Request(c.Request.Context(), http.MethodGet, "/config.yaml", nil, "application/yaml", "v0")
			if err == nil && currentResp.ETag != "" && currentResp.ETag != expected {
				c.JSON(http.StatusConflict, gin.H{"error": "CPA configuration has changed, please reload before saving"})
				return
			}
		}

		resp, err := cpa.Request(c.Request.Context(), http.MethodPut, "/config.yaml", body, "application/yaml", "v0")
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.Data(resp.StatusCode, "application/json; charset=utf-8", resp.Bytes)
	})

	rg.GET("/config", func(c *gin.Context) {
		forwardCpa(c, cpa, http.MethodGet, "/config", nil, "v0")
	})

	rg.GET("/latest-version", func(c *gin.Context) {
		forwardCpa(c, cpa, http.MethodGet, "/latest-version", nil, "v0")
	})

	// Auth files
	rg.GET("/auth-files", func(c *gin.Context) {
		forwardCpa(c, cpa, http.MethodGet, "/auth-files", nil, "v0")
	})

	rg.POST("/auth-files", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
			return
		}

		var payload struct {
			Name    string      `json:"name"`
			Content interface{} `json:"content"`
		}
		_ = json.Unmarshal(body, &payload)
		if !isSafeAuthFilename(payload.Name) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid auth filename"})
			return
		}

		targetURL := fmt.Sprintf("/auth-files?name=%s", payload.Name)
		contentBytes, _ := json.Marshal(payload.Content)
		forwardCpa(c, cpa, http.MethodPost, targetURL, contentBytes, "v0")
	})

	rg.DELETE("/auth-files", func(c *gin.Context) {
		name := c.Query("name")
		if !isSafeAuthFilename(name) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid auth filename"})
			return
		}
		targetURL := fmt.Sprintf("/auth-files?name=%s", name)
		forwardCpa(c, cpa, http.MethodDelete, targetURL, nil, "v0")
	})

	rg.GET("/auth-files/download", func(c *gin.Context) {
		name := c.Query("name")
		if !isSafeAuthFilename(name) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid auth filename"})
			return
		}
		targetURL := fmt.Sprintf("/auth-files/download?name=%s", name)
		resp, err := cpa.Request(c.Request.Context(), http.MethodGet, targetURL, nil, "application/json", "v0")
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", name))
		c.Data(resp.StatusCode, "application/json", resp.Bytes)
	})

	rg.GET("/auth-files/models", func(c *gin.Context) {
		name := c.Query("name")
		if !isSafeAuthFilename(name) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid auth filename"})
			return
		}
		targetURL := fmt.Sprintf("/auth-files/models?name=%s", name)
		forwardCpa(c, cpa, http.MethodGet, targetURL, nil, "v0")
	})

	rg.POST("/auth-files/refresh", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		forwardCpa(c, cpa, http.MethodPost, "/auth-files/refresh", body, "v0")
	})

	rg.PATCH("/auth-files/fields", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		forwardCpa(c, cpa, http.MethodPatch, "/auth-files/fields", body, "v0")
	})

	rg.PATCH("/auth-files/status", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		forwardCpa(c, cpa, http.MethodPatch, "/auth-files/status", body, "v0")
	})

	// OAuth routes
	oauthResources := map[string]string{
		"codex":        "/codex-auth-url",
		"claude":       "/anthropic-auth-url",
		"antigravity":  "/antigravity-auth-url",
		"kimi":         "/kimi-auth-url",
		"xai":          "/xai-auth-url",
		"devin":        "/devin-auth-url",
		"meta":         "/meta-auth-url",
		"workbuddy":    "/workbuddy-auth-url",
		"workbuddy-cn": "/workbuddy-cn-auth-url",
		"qoder":        "/qoder-auth-url",
		"qoder-cn":     "/qoder-cn-auth-url",
	}

	rg.POST("/oauth/:provider/start", func(c *gin.Context) {
		provider := c.Param("provider")
		resPath, ok := oauthResources[provider]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported oauth provider"})
			return
		}
		forwardCpa(c, cpa, http.MethodGet, resPath+"?is_webui=true", nil, "v0")
	})

	rg.GET("/oauth/status", func(c *gin.Context) {
		state := c.Query("state")
		targetURL := fmt.Sprintf("/get-auth-status?state=%s", state)
		forwardCpa(c, cpa, http.MethodGet, targetURL, nil, "v0")
	})

	rg.POST("/oauth/callback", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		forwardCpa(c, cpa, http.MethodPost, "/oauth-callback", body, "v0")
	})

	rg.POST("/oauth/cancel", func(c *gin.Context) {
		state := c.Query("state")
		targetURL := fmt.Sprintf("/oauth-session?state=%s", state)
		forwardCpa(c, cpa, http.MethodDelete, targetURL, nil, "v0")
	})

	// V8 endpoints forwarding
	rg.Any("/v8/*subpath", func(c *gin.Context) {
		subpath := c.Param("subpath")
		body, _ := io.ReadAll(c.Request.Body)
		forwardCpa(c, cpa, c.Request.Method, subpath, body, "v8")
	})

	// Logs
	rg.GET("/logs", func(c *gin.Context) {
		after := c.Query("after")
		if after == "" {
			after = strconv.FormatInt(time.Now().Unix()-3600, 10)
		}
		targetURL := fmt.Sprintf("/logs?after=%s", after)
		resp, err := cpa.Request(c.Request.Context(), http.MethodGet, targetURL, nil, "application/json", "v0")
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}

		if resp.Truncated {
			c.Header("X-Logs-Truncated", "true")
		}
		c.Data(resp.StatusCode, "application/json; charset=utf-8", resp.Bytes)
	})

	rg.DELETE("/logs", func(c *gin.Context) {
		forwardCpa(c, cpa, http.MethodDelete, "/logs", nil, "v0")
	})

	// Error logs
	rg.GET("/request-error-logs", func(c *gin.Context) {
		forwardCpa(c, cpa, http.MethodGet, "/request-error-logs", nil, "v0")
	})

	rg.GET("/request-error-logs/preview", func(c *gin.Context) {
		name := c.Query("name")
		targetURL := fmt.Sprintf("/request-error-logs/%s", name)
		resp, err := cpa.Request(c.Request.Context(), http.MethodGet, targetURL, nil, "text/plain", "v0")
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.Data(resp.StatusCode, "text/plain; charset=utf-8", resp.Bytes)
	})

	// Quota reset & cooldown
	rg.POST("/reset-quota", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		forwardCpa(c, cpa, http.MethodPost, "/reset-quota", body, "v0")
	})

	rg.POST("/cooldown/reset", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		forwardCpa(c, cpa, http.MethodPost, "/routing/cooldown/reset", body, "v0")
	})

	rg.POST("/cooldown/reset-all", func(c *gin.Context) {
		forwardCpa(c, cpa, http.MethodPost, "/routing/cooldown/reset", []byte("{}"), "v0")
	})

	// Providers & Settings
	rg.GET("/providers/:provider", func(c *gin.Context) {
		prov := c.Param("provider")
		forwardCpa(c, cpa, http.MethodGet, "/"+prov, nil, "v0")
	})

	rg.PUT("/providers/:provider", func(c *gin.Context) {
		prov := c.Param("provider")
		body, _ := io.ReadAll(c.Request.Body)
		forwardCpa(c, cpa, http.MethodPut, "/"+prov, body, "v0")
	})

	rg.GET("/settings/:setting", func(c *gin.Context) {
		st := c.Param("setting")
		forwardCpa(c, cpa, http.MethodGet, "/"+st, nil, "v0")
	})

	rg.PATCH("/settings/:setting", func(c *gin.Context) {
		st := c.Param("setting")
		body, _ := io.ReadAll(c.Request.Body)
		forwardCpa(c, cpa, http.MethodPatch, "/"+st, body, "v0")
	})

	// Runtime config snapshot
	rg.GET("/runtime-config", func(c *gin.Context) {
		ctx := c.Request.Context()
		var configData interface{}
		var routingData interface{}
		var versionData interface{}

		_ = cpa.RequestJSON(ctx, http.MethodGet, "/config", nil, "v0", &configData)
		_ = cpa.RequestJSON(ctx, http.MethodGet, "/routing", nil, "v0", &routingData)
		_ = cpa.RequestJSON(ctx, http.MethodGet, "/latest-version", nil, "v0", &versionData)

		c.JSON(http.StatusOK, gin.H{
			"config":  configData,
			"routing": routingData,
			"version": versionData,
		})
	})
}

func forwardCpa(c *gin.Context, cpa *CpaClient, method, path string, body []byte, apiVersion string) {
	resp, err := cpa.Request(c.Request.Context(), method, path, body, "application/json", apiVersion)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	contentType := resp.ContentType
	if contentType == "" {
		contentType = "application/json; charset=utf-8"
	}
	c.Data(resp.StatusCode, contentType, resp.Bytes)
}

func computeETag(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
