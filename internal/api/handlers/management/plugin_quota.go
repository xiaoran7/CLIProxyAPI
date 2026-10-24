package management

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/auth/qoder"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/auth/workbuddy"
	zcodeauth "github.com/router-for-me/CLIProxyAPI/v8/internal/auth/zcode"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/clienterror"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	log "github.com/sirupsen/logrus"
)

type credentialQuotaRequest struct {
	AuthIndexSnake  *string `json:"auth_index"`
	AuthIndexCamel  *string `json:"authIndex"`
	AuthIndexPascal *string `json:"AuthIndex"`
	PluginID        string  `json:"plugin_id"`
	Provider        string  `json:"provider"`
}

func (r credentialQuotaRequest) resolveAuthIndex() string {
	if r.AuthIndexSnake != nil && strings.TrimSpace(*r.AuthIndexSnake) != "" {
		return strings.TrimSpace(*r.AuthIndexSnake)
	}
	if r.AuthIndexCamel != nil && strings.TrimSpace(*r.AuthIndexCamel) != "" {
		return strings.TrimSpace(*r.AuthIndexCamel)
	}
	if r.AuthIndexPascal != nil && strings.TrimSpace(*r.AuthIndexPascal) != "" {
		return strings.TrimSpace(*r.AuthIndexPascal)
	}
	return ""
}

// GetQuotaProviders returns the list of registered quota providers.
func (h *Handler) GetQuotaProviders(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusOK, gin.H{"providers": []any{}})
		return
	}
	h.mu.Lock()
	host := h.pluginHost
	h.mu.Unlock()
	if host == nil {
		c.JSON(http.StatusOK, gin.H{"providers": []any{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"providers": host.QuotaProviders(c.Request.Context())})
}

// FetchCredentialQuota retrieves normalized quota for a credential via its quota provider or declarative probe.
func (h *Handler) FetchCredentialQuota(c *gin.Context) {
	var body credentialQuotaRequest
	if errBind := c.ShouldBindJSON(&body); errBind != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	authIndex := body.resolveAuthIndex()
	if authIndex == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "auth_index is required"})
		return
	}

	auth := h.authByIndex(authIndex)
	if auth == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "auth not found"})
		return
	}

	h.mu.Lock()
	host := h.pluginHost
	h.mu.Unlock()

	pluginID := strings.TrimSpace(body.PluginID)
	provider := strings.TrimSpace(body.Provider)
	if provider == "" {
		provider = auth.Provider
	}

	if host != nil {
		req := pluginapi.QuotaFetchRequest{
			AuthIndex:  auth.Index,
			AuthID:     auth.ID,
			Provider:   provider,
			Metadata:   auth.Metadata,
			Attributes: auth.Attributes,
		}
		var quotaResp pluginapi.QuotaFetchResponse
		var handled bool
		var errFetch error
		if pluginID != "" && host.HasQuotaProviderForPlugin(pluginID) {
			quotaResp, handled, errFetch = host.FetchQuotaByPlugin(c.Request.Context(), pluginID, req)
		} else if pluginID == "" && host.HasQuotaProvider(provider) {
			quotaResp, handled, errFetch = host.FetchQuota(c.Request.Context(), req)
		}
		if handled {
			if errFetch != nil {
				log.WithError(errFetch).Warnf("failed to fetch quota for credential %s", auth.Index)
				c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to fetch quota: %v", errFetch)})
				return
			}
			c.JSON(http.StatusOK, quotaResp)
			return
		}
	}

	// Native WorkBuddy / CodeBuddy quota provider
	pLower := strings.ToLower(provider)
	if strings.HasPrefix(pLower, "workbuddy") || pLower == "copilot-tencent" || strings.HasPrefix(strings.ToLower(pluginID), "workbuddy") {
		quotaResp, errQuota := workbuddy.QueryQuota(c.Request.Context(), nil, auth)
		if errQuota != nil {
			log.WithError(errQuota).Warnf("failed to query native workbuddy quota for credential %s", auth.Index)
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to fetch quota: %v", errQuota)})
			return
		}
		c.JSON(http.StatusOK, quotaResp)
		return
	}

	// Native Qoder quota provider
	if strings.HasPrefix(pLower, "qoder") || strings.HasPrefix(strings.ToLower(pluginID), "qoder") {
		quotaResp, errQuota := qoder.QueryQuota(c.Request.Context(), nil, auth)
		if errQuota != nil {
			log.WithError(errQuota).Warnf("failed to query native qoder quota for credential %s", auth.Index)
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to fetch quota: %v", errQuota)})
			return
		}
		c.JSON(http.StatusOK, quotaResp)
		return
	}

	// Native ZCode / BigModel quota provider
	if strings.HasPrefix(pLower, "zcode") || strings.HasPrefix(strings.ToLower(pluginID), "zcode") || strings.HasPrefix(pLower, "bigmodel") || strings.HasPrefix(strings.ToLower(pluginID), "bigmodel") {
		quotaResp, errQuota := zcodeauth.QueryQuota(c.Request.Context(), nil, auth)
		if errQuota != nil {
			log.WithError(errQuota).Warnf("failed to query native zcode quota for credential %s", auth.Index)
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to fetch quota: %v", errQuota)})
			return
		}
		c.JSON(http.StatusOK, quotaResp)
		return
	}

	// Fallback to declarative quota probe if configured in metadata
	if auth.Metadata != nil {
		if rawProbe, okProbe := auth.Metadata["quota_probe"]; okProbe && rawProbe != nil {
			if probeMap, okMap := rawProbe.(map[string]any); okMap {
				quotaResp, handledProbe, errProbe := h.executeQuotaProbe(c, auth, probeMap)
				if handledProbe {
					if errProbe != nil {
						c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("quota probe failed: %v", errProbe)})
						return
					}
					c.JSON(http.StatusOK, quotaResp)
					return
				}
			}
		}
	}

	c.JSON(http.StatusNotImplemented, gin.H{"error": "no quota provider available for credential"})
}

// ResetCredentialQuota resets quota or usage for a credential.
func (h *Handler) ResetCredentialQuota(c *gin.Context) {
	var body credentialQuotaRequest
	if errBind := c.ShouldBindJSON(&body); errBind != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	authIndex := body.resolveAuthIndex()
	if authIndex == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "auth_index is required"})
		return
	}

	auth := h.authByIndex(authIndex)
	if auth == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "auth not found"})
		return
	}

	h.mu.Lock()
	host := h.pluginHost
	h.mu.Unlock()

	if host == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "plugin host unavailable"})
		return
	}

	pluginID := strings.TrimSpace(body.PluginID)
	provider := strings.TrimSpace(body.Provider)
	if provider == "" {
		provider = auth.Provider
	}

	req := pluginapi.QuotaResetRequest{
		AuthIndex:  auth.Index,
		AuthID:     auth.ID,
		Provider:   provider,
		Metadata:   auth.Metadata,
		Attributes: auth.Attributes,
	}

	var resetResp pluginapi.QuotaResetResponse
	var handled bool
	var errReset error

	if pluginID != "" {
		if !host.HasQuotaProviderForPlugin(pluginID) {
			c.JSON(http.StatusNotFound, gin.H{"error": "quota provider not found for plugin"})
			return
		}
		resetResp, handled, errReset = host.ResetQuotaByPlugin(c.Request.Context(), pluginID, req)
		if !handled {
			c.JSON(http.StatusNotFound, gin.H{"error": "quota provider not found for plugin"})
			return
		}
	} else {
		if !host.HasQuotaProviderContext(c.Request.Context(), provider) {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "no quota provider available for credential to reset"})
			return
		}
		resetResp, handled, errReset = host.ResetQuota(c.Request.Context(), req)
		if !handled {
			c.JSON(http.StatusBadGateway, gin.H{"error": "quota provider did not handle reset request"})
			return
		}
	}

	if errReset != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("plugin quota reset failed: %v", errReset)})
		return
	}
	if !resetResp.Success {
		msg := resetResp.Message
		if msg == "" {
			msg = "quota reset rejected by provider"
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": msg})
		return
	}

	if h.authManager != nil {
		updated, _, errResetCore := h.authManager.ResetQuota(c.Request.Context(), auth.ID)
		if errResetCore != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to reset routing quota: %v", errResetCore)})
			return
		}
		if updated != nil {
			updated.EnsureIndex()
		}
	}

	resp := gin.H{
		"status":     "ok",
		"auth_index": auth.Index,
	}
	if resetResp.Message != "" {
		resp["message"] = resetResp.Message
	}
	c.JSON(http.StatusOK, resp)
}

// GetPluginQuota handles GET /v0/management/plugins/:id/quota?auth_index=...
func (h *Handler) GetPluginQuota(c *gin.Context) {
	pluginID := strings.TrimSpace(c.Param("id"))
	authIndex := strings.TrimSpace(c.Query("auth_index"))
	if authIndex == "" {
		authIndex = strings.TrimSpace(c.Query("authIndex"))
	}
	if authIndex == "" {
		pLower := strings.ToLower(pluginID)
		if strings.HasPrefix(pLower, "workbuddy") {
			h.listWorkBuddyQuotas(c)
			return
		}
		if strings.HasPrefix(pLower, "qoder") {
			h.listQoderQuotas(c)
			return
		}
		if strings.HasPrefix(pLower, "zcode") {
			h.listZCodeQuotas(c)
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "auth_index is required"})
		return
	}
	h.fetchQuotaForPlugin(c, pluginID, authIndex)
}

// FetchPluginQuota handles POST /v0/management/plugins/:id/quota
func (h *Handler) FetchPluginQuota(c *gin.Context) {
	pluginID := strings.TrimSpace(c.Param("id"))
	var body credentialQuotaRequest
	if errBind := c.ShouldBindJSON(&body); errBind != nil {
		c.JSON(clienterror.HTTPStatusFromErrorOr(errBind, http.StatusBadRequest), gin.H{"error": "invalid request body"})
		return
	}
	authIndex := body.resolveAuthIndex()
	if authIndex == "" {
		pLower := strings.ToLower(pluginID)
		if strings.HasPrefix(pLower, "workbuddy") {
			h.listWorkBuddyQuotas(c)
			return
		}
		if strings.HasPrefix(pLower, "qoder") {
			h.listQoderQuotas(c)
			return
		}
		if strings.HasPrefix(pLower, "zcode") {
			h.listZCodeQuotas(c)
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "auth_index is required"})
		return
	}
	h.fetchQuotaForPlugin(c, pluginID, authIndex)
}

func (h *Handler) fetchQuotaForPlugin(c *gin.Context, pluginID, authIndex string) {
	auth := h.authByIndex(authIndex)
	if auth == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "auth not found"})
		return
	}

	h.mu.Lock()
	host := h.pluginHost
	h.mu.Unlock()

	if host != nil && host.HasQuotaProviderForPlugin(pluginID) {
		quotaResp, handled, errFetch := host.FetchQuotaByPlugin(c.Request.Context(), pluginID, pluginapi.QuotaFetchRequest{
			AuthIndex:  auth.Index,
			AuthID:     auth.ID,
			Provider:   auth.Provider,
			Metadata:   auth.Metadata,
			Attributes: auth.Attributes,
		})
		if handled {
			if errFetch != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to fetch quota: %v", errFetch)})
				return
			}
			c.JSON(http.StatusOK, quotaResp)
			return
		}
	}

	pLower := strings.ToLower(strings.TrimSpace(pluginID))
	authPLower := strings.ToLower(strings.TrimSpace(auth.Provider))
	if strings.HasPrefix(pLower, "workbuddy") || strings.HasPrefix(authPLower, "workbuddy") || authPLower == "copilot-tencent" {
		quotaResp, errQuota := workbuddy.QueryQuota(c.Request.Context(), nil, auth)
		if errQuota != nil {
			log.WithError(errQuota).Warnf("failed to query native workbuddy quota for credential %s", auth.Index)
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to fetch quota: %v", errQuota)})
			return
		}
		c.JSON(http.StatusOK, quotaResp)
		return
	}

	if strings.HasPrefix(pLower, "qoder") || strings.HasPrefix(authPLower, "qoder") {
		quotaResp, errQuota := qoder.QueryQuota(c.Request.Context(), nil, auth)
		if errQuota != nil {
			log.WithError(errQuota).Warnf("failed to query native qoder quota for credential %s", auth.Index)
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to fetch quota: %v", errQuota)})
			return
		}
		c.JSON(http.StatusOK, quotaResp)
		return
	}

	if strings.HasPrefix(pLower, "zcode") || strings.HasPrefix(authPLower, "zcode") {
		quotaResp, errQuota := zcodeauth.QueryQuota(c.Request.Context(), nil, auth)
		if errQuota != nil {
			log.WithError(errQuota).Warnf("failed to query native zcode quota for credential %s", auth.Index)
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to fetch quota: %v", errQuota)})
			return
		}
		c.JSON(http.StatusOK, quotaResp)
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "quota provider not found for plugin"})
}

// ResetPluginQuota handles DELETE /v0/management/plugins/:id/quota and POST /v0/management/plugins/:id/quota/reset
func (h *Handler) ResetPluginQuota(c *gin.Context) {
	pluginID := strings.TrimSpace(c.Param("id"))
	authIndex := strings.TrimSpace(c.Query("auth_index"))
	if authIndex == "" {
		authIndex = strings.TrimSpace(c.Query("authIndex"))
	}
	if authIndex == "" {
		var body credentialQuotaRequest
		if errBind := c.ShouldBindJSON(&body); errBind != nil {
			if status := clienterror.HTTPStatusFromError(errBind); status == http.StatusRequestEntityTooLarge {
				c.JSON(status, gin.H{"error": errBind.Error()})
				return
			}
		}
		authIndex = body.resolveAuthIndex()
	}
	if authIndex == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "auth_index is required"})
		return
	}

	auth := h.authByIndex(authIndex)
	if auth == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "auth not found"})
		return
	}

	h.mu.Lock()
	host := h.pluginHost
	h.mu.Unlock()

	if host == nil || !host.HasQuotaProviderForPlugin(pluginID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "quota provider not found for plugin"})
		return
	}

	resetResp, handled, errReset := host.ResetQuotaByPlugin(c.Request.Context(), pluginID, pluginapi.QuotaResetRequest{
		AuthIndex:  auth.Index,
		AuthID:     auth.ID,
		Provider:   auth.Provider,
		Metadata:   auth.Metadata,
		Attributes: auth.Attributes,
	})
	if !handled {
		c.JSON(http.StatusNotFound, gin.H{"error": "quota provider not found for plugin"})
		return
	}
	if errReset != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to reset quota: %v", errReset)})
		return
	}
	if !resetResp.Success {
		msg := resetResp.Message
		if msg == "" {
			msg = "quota reset rejected by plugin"
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": msg})
		return
	}

	if h.authManager != nil {
		updated, _, errResetCore := h.authManager.ResetQuota(c.Request.Context(), auth.ID)
		if errResetCore != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to reset routing quota: %v", errResetCore)})
			return
		}
		if updated != nil {
			updated.EnsureIndex()
		}
	}

	resp := gin.H{
		"status":     "ok",
		"auth_index": auth.Index,
	}
	if resetResp.Message != "" {
		resp["message"] = resetResp.Message
	}
	c.JSON(http.StatusOK, resp)
}
