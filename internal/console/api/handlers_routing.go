package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// UpstreamRoutingConfig holds routing configuration fields.
type UpstreamRoutingConfig struct {
	Strategy           string `json:"strategy"`
	SessionAffinity    bool   `json:"session_affinity"`
	SessionAffinityTTL string `json:"session_affinity_ttl"`
}

func normalizeRoutingStrategy(val string) string {
	s := strings.ToLower(strings.TrimSpace(val))
	switch s {
	case "", "round-robin", "roundrobin", "rr":
		return "round-robin"
	case "weighted-round-robin", "weightedroundrobin", "wrr":
		return "weighted-round-robin"
	case "fill-first", "fillfirst", "ff":
		return "fill-first"
	case "smart", "smart-routing":
		return "smart"
	case "pace", "pace-routing":
		return "pace"
	case "priority":
		return "priority"
	default:
		return ""
	}
}

func normalizeRoutingTTL(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return ""
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n >= 0 {
			return strconv.FormatInt(n, 10)
		}
		return ""
	case float64:
		if val >= 0 {
			return strconv.FormatInt(int64(val), 10)
		}
		return ""
	case int:
		if val >= 0 {
			return strconv.Itoa(val)
		}
		return ""
	case int64:
		if val >= 0 {
			return strconv.FormatInt(val, 10)
		}
		return ""
	default:
		return ""
	}
}

func readRoutingConfig(ctx context.Context, cpa *CpaClient) (*UpstreamRoutingConfig, error) {
	var configResp map[string]interface{}
	if err := cpa.RequestJSON(ctx, http.MethodGet, "/config", nil, "v0", &configResp); err != nil {
		return nil, err
	}
	res := &UpstreamRoutingConfig{
		Strategy:           "round-robin",
		SessionAffinity:    false,
		SessionAffinityTTL: "",
	}
	if rRaw, ok := configResp["routing"].(map[string]interface{}); ok && rRaw != nil {
		if strat, ok := rRaw["strategy"].(string); ok {
			if norm := normalizeRoutingStrategy(strat); norm != "" {
				res.Strategy = norm
			}
		}
		if aff, ok := rRaw["session-affinity"].(bool); ok {
			res.SessionAffinity = aff
		} else if aff, ok := rRaw["session_affinity"].(bool); ok {
			res.SessionAffinity = aff
		}
		if ttl, exists := rRaw["session-affinity-ttl"]; exists {
			res.SessionAffinityTTL = normalizeRoutingTTL(ttl)
		} else if ttl, exists := rRaw["session_affinity_ttl"]; exists {
			res.SessionAffinityTTL = normalizeRoutingTTL(ttl)
		}
	}
	return res, nil
}

// HandleGetRuntimeConfig returns the snapshot of runtime config, routing, version, and usage stats wrapped in ok/value envelopes.
func HandleGetRuntimeConfig(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		var configData interface{}
		configErr := cpa.RequestJSON(ctx, http.MethodGet, "/config", nil, "v0", &configData)

		routingData, routingErr := readRoutingConfig(ctx, cpa)

		var versionData interface{}
		versionErr := cpa.RequestJSON(ctx, http.MethodGet, "/latest-version", nil, "v0", &versionData)

		var usageStatsData interface{}
		usageErr := cpa.RequestJSON(ctx, http.MethodGet, "/usage-statistics-enabled", nil, "v0", &usageStatsData)
		if usageErr != nil {
			if cfgMap, ok := configData.(map[string]interface{}); ok {
				if val, exists := cfgMap["usage-statistics-enabled"]; exists {
					usageStatsData = val
					usageErr = nil
				}
			}
		}

		configWrap := gin.H{"ok": configErr == nil}
		if configErr == nil {
			configWrap["value"] = configData
		} else {
			configWrap["error"] = configErr.Error()
		}

		routingWrap := gin.H{"ok": routingErr == nil}
		if routingErr == nil {
			routingWrap["value"] = routingData
		} else {
			routingWrap["error"] = routingErr.Error()
		}

		versionWrap := gin.H{"ok": versionErr == nil}
		if versionErr == nil {
			versionWrap["value"] = versionData
		} else {
			versionWrap["error"] = versionErr.Error()
		}

		usageWrap := gin.H{"ok": usageErr == nil}
		if usageErr == nil {
			usageWrap["value"] = usageStatsData
		} else {
			usageWrap["error"] = usageErr.Error()
		}

		c.JSON(http.StatusOK, gin.H{
			"config":           configWrap,
			"routing":          routingWrap,
			"version":          versionWrap,
			"usage_statistics": usageWrap,
		})
	}
}

// HandleGetUpstreamRouting reads upstream routing configuration from CPA.
func HandleGetUpstreamRouting(cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		routingData, err := readRoutingConfig(ctx, cpa)
		if err != nil {
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
			Strategy           *string     `json:"strategy"`
			SessionAffinity    *bool       `json:"session_affinity"`
			SessionAffinityTTL interface{} `json:"session_affinity_ttl"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		patch := make(map[string]interface{})
		if payload.Strategy != nil {
			norm := normalizeRoutingStrategy(*payload.Strategy)
			if norm == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid routing strategy: must be round-robin, weighted-round-robin, fill-first, smart, pace, or priority"})
				return
			}
			patch["strategy"] = norm
		}
		if payload.SessionAffinity != nil {
			patch["session-affinity"] = *payload.SessionAffinity
		}
		if payload.SessionAffinityTTL != nil {
			patch["session-affinity-ttl"] = normalizeRoutingTTL(payload.SessionAffinityTTL)
		}

		if len(patch) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No routing fields to update"})
			return
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

		routingData, err := readRoutingConfig(ctx, cpa)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"routing": patch})
			return
		}
		c.JSON(http.StatusOK, gin.H{"routing": routingData})
	}
}
