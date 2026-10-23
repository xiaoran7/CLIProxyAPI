package management

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/clienterror"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/proxyutil"
	log "github.com/sirupsen/logrus"
)

const defaultAPICallTimeout = 60 * time.Second

const (
	antigravityOAuthClientID     = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	antigravityOAuthClientSecret = "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"
)

type apiCallRequest struct {
	AuthIndexSnake    *string           `json:"auth_index"`
	AuthIndexCamel    *string           `json:"authIndex"`
	AuthIndexPascal   *string           `json:"AuthIndex"`
	Method            string            `json:"method"`
	URL               string            `json:"url"`
	ProxyURL          string            `json:"proxy_url"`
	Header            map[string]string `json:"header"`
	Data              string            `json:"data"`
	ForceRefreshSnake *bool             `json:"force_refresh"`
	ForceRefreshCamel *bool             `json:"forceRefresh"`
}

type apiCallResponse struct {
	StatusCode int                 `json:"status_code"`
	Header     map[string][]string `json:"header"`
	Body       string              `json:"body"`
}

// APICall makes a generic HTTP request on behalf of the management API caller.
// It is protected by the management middleware.
//
// Endpoint:
//
//	POST /v0/management/api-call
//
// Authentication:
//
//	Same as other management APIs (requires a management key and remote-management rules).
//	You can provide the key via:
//	- Authorization: Bearer <key>
//	- X-Management-Key: <key>
//
// Request JSON:
//   - auth_index / authIndex / AuthIndex (optional):
//     The credential "auth_index" from GET /v0/management/auth-files (or other endpoints returning it).
//     If omitted, credential-specific proxy selection is skipped.
//     If "$TOKEN$" is present and the credential or token cannot be resolved, the request fails with HTTP 400.
//   - method (required): HTTP method, e.g. GET, POST, PUT, PATCH, DELETE.
//   - url (required): Absolute URL including scheme and host, e.g. "https://api.example.com/v1/ping".
//   - proxy_url (optional): Proxy used for this request. Supports HTTP, HTTPS, SOCKS5, SOCKS5H,
//     and "direct"/"none" to explicitly bypass proxies. When set, credential and global proxies are ignored.
//   - header (optional): Request headers map.
//     Supports magic variable "$TOKEN$" which is replaced using the selected credential:
//     1) metadata.access_token
//     2) attributes.api_key
//     3) metadata.token / metadata.id_token / metadata.cookie
//     Example: {"Authorization":"Bearer $TOKEN$"}.
//     Note: if you need to override the HTTP Host header, set header["Host"].
//   - data (optional): Raw request body as string (useful for POST/PUT/PATCH).
//
// Proxy selection (highest priority first):
//  1. Request proxy_url (when set, lower-priority proxy settings are ignored)
//  2. Selected credential proxy_url
//  3. Global config proxy-url
//  4. Direct connect (environment proxies are not used)
//
// Response JSON (returned with HTTP 200 when the APICall itself succeeds):
//   - status_code: Upstream HTTP status code.
//   - header: Upstream response headers.
//   - body: Upstream response body as string.
//
// Example:
//
//	curl -sS -X POST "http://127.0.0.1:8317/v0/management/api-call" \
//	  -H "Authorization: Bearer <MANAGEMENT_KEY>" \
//	  -H "Content-Type: application/json" \
//	  -d '{"auth_index":"<AUTH_INDEX>","method":"GET","url":"https://api.example.com/v1/ping","header":{"Authorization":"Bearer $TOKEN$"}}'
//
//	curl -sS -X POST "http://127.0.0.1:8317/v0/management/api-call" \
//	  -H "Authorization: Bearer 831227" \
//	  -H "Content-Type: application/json" \
//	  -d '{"auth_index":"<AUTH_INDEX>","method":"POST","url":"https://api.example.com/v1/fetchAvailableModels","header":{"Authorization":"Bearer $TOKEN$","Content-Type":"application/json","User-Agent":"cliproxyapi"},"data":"{}"}'
func (h *Handler) APICall(c *gin.Context) {
	var body apiCallRequest
	if errBindJSON := c.ShouldBindJSON(&body); errBindJSON != nil {
		c.JSON(clienterror.HTTPStatusFromErrorOr(errBindJSON, http.StatusBadRequest), gin.H{"error": "invalid body"})
		return
	}

	method := strings.ToUpper(strings.TrimSpace(body.Method))
	if method == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing method"})
		return
	}

	urlStr := strings.TrimSpace(body.URL)
	if urlStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing url"})
		return
	}
	parsedURL, errParseURL := url.Parse(urlStr)
	if errParseURL != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid url"})
		return
	}

	requestProxyURL := strings.TrimSpace(body.ProxyURL)
	if requestProxyURL != "" {
		if _, errParseProxy := proxyutil.Parse(requestProxyURL); errParseProxy != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy_url"})
			return
		}
	}

	authIndex := firstNonEmptyString(body.AuthIndexSnake, body.AuthIndexCamel, body.AuthIndexPascal)
	auth := h.authByIndex(authIndex)

	queryRefresh := c.Query("refresh") == "1" ||
		strings.EqualFold(c.Query("refresh"), "true") ||
		c.Query("force_refresh") == "1" ||
		strings.EqualFold(c.Query("force_refresh"), "true") ||
		c.Query("forceRefresh") == "1" ||
		strings.EqualFold(c.Query("forceRefresh"), "true")

	forceRefresh := (body.ForceRefreshSnake != nil && *body.ForceRefreshSnake) ||
		(body.ForceRefreshCamel != nil && *body.ForceRefreshCamel) ||
		queryRefresh

	origHeaders := body.Header
	if origHeaders == nil {
		origHeaders = map[string]string{}
	}
	origData := body.Data

	hasTokenPlaceholder := false
	for _, value := range origHeaders {
		if strings.Contains(value, "$TOKEN$") {
			hasTokenPlaceholder = true
			break
		}
	}
	if !hasTokenPlaceholder && strings.Contains(origData, "$TOKEN$") {
		hasTokenPlaceholder = true
	}

	var token string
	var tokenResolved bool
	var tokenErr error

	resolveToken := func(force bool) error {
		if !tokenResolved || force {
			token, tokenErr = h.resolveTokenForAuthWithForce(c.Request.Context(), auth, requestProxyURL, force)
			tokenResolved = true
		}
		if token == "" {
			if tokenErr != nil {
				return errors.New("auth token refresh failed")
			}
			if authIndex != "" && auth == nil {
				return errors.New("auth credential not found for auth_index")
			}
			return errors.New("auth token not found")
		}
		return nil
	}

	if hasTokenPlaceholder {
		if errToken := resolveToken(forceRefresh); errToken != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": errToken.Error()})
			return
		}
	}

	buildRequest := func(curToken string) (*http.Request, error) {
		reqHeaders := make(map[string]string, len(origHeaders))
		for k, v := range origHeaders {
			if strings.Contains(v, "$TOKEN$") {
				reqHeaders[k] = strings.ReplaceAll(v, "$TOKEN$", curToken)
			} else {
				reqHeaders[k] = v
			}
		}

		dataStr := origData
		if strings.Contains(dataStr, "$TOKEN$") {
			replacement := curToken
			if json.Valid([]byte(dataStr)) && strings.ContainsAny(curToken, "\"\\\r\n\t") {
				if b, errMarshal := json.Marshal(curToken); errMarshal == nil && len(b) >= 2 {
					replacement = string(b[1 : len(b)-1])
				}
			}
			dataStr = strings.ReplaceAll(dataStr, "$TOKEN$", replacement)
		}

		var requestBody io.Reader
		if dataStr != "" {
			requestBody = strings.NewReader(dataStr)
		}

		req, errNewRequest := http.NewRequestWithContext(c.Request.Context(), method, urlStr, requestBody)
		if errNewRequest != nil {
			return nil, errNewRequest
		}

		hostOverride := ""
		for key, value := range reqHeaders {
			if strings.EqualFold(key, "host") {
				hostOverride = strings.TrimSpace(value)
				continue
			}
			req.Header.Set(key, value)
		}
		if hostOverride != "" {
			req.Host = hostOverride
		}
		return req, nil
	}

	req, errBuild := buildRequest(token)
	if errBuild != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to build request"})
		return
	}

	httpClient := &http.Client{
		Timeout: defaultAPICallTimeout,
	}
	httpClient.Transport = h.apiCallTransport(auth, requestProxyURL)

	resp, errDo := httpClient.Do(req)
	if errDo != nil {
		log.WithError(errDo).Debug("management APICall request failed")
		c.JSON(http.StatusBadGateway, gin.H{"error": "request failed"})
		return
	}

	respBody, errReadAll := io.ReadAll(resp.Body)
	if errClose := resp.Body.Close(); errClose != nil {
		log.Errorf("response body close error: %v", errClose)
	}
	if errReadAll != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read response"})
		return
	}

	// If upstream returned 401 Unauthorized, and we used a credential token placeholder ($TOKEN$),
	// attempt an automatic forced refresh once and retry the request.
	if resp.StatusCode == http.StatusUnauthorized && hasTokenPlaceholder && auth != nil && !forceRefresh {
		if errRefresh := resolveToken(true); errRefresh == nil && token != "" {
			if retryReq, errRetryBuild := buildRequest(token); errRetryBuild == nil {
				if retryResp, errRetryDo := httpClient.Do(retryReq); errRetryDo == nil {
					retryBody, errRetryRead := io.ReadAll(retryResp.Body)
					if errRetryClose := retryResp.Body.Close(); errRetryClose != nil {
						log.Errorf("retry response body close error: %v", errRetryClose)
					}
					if errRetryRead == nil {
						resp = retryResp
						respBody = retryBody
					}
				} else {
					log.WithError(errRetryDo).Debug("management APICall retry request failed")
				}
			}
		}
	}

	if auth != nil && strings.EqualFold(strings.TrimSpace(auth.Provider), "antigravity") &&
		strings.Contains(urlStr, "retrieveUserQuotaSummary") && resp.StatusCode == http.StatusOK {
		respBody = reconcileAntigravityQuotaResponse(auth, respBody, time.Now().UTC())
	}

	c.JSON(http.StatusOK, apiCallResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       string(respBody),
	})
}

func (h *Handler) authByIndex(authIndex string) *coreauth.Auth {
	authIndex = strings.TrimSpace(authIndex)
	if authIndex == "" || h == nil || h.authManager == nil {
		return nil
	}
	auths := h.authManager.List()
	for _, auth := range auths {
		if auth == nil {
			continue
		}
		auth.EnsureIndex()
		if auth.Index == authIndex {
			return auth
		}
	}
	return nil
}

func (h *Handler) apiCallTransport(auth *coreauth.Auth, requestProxyURL string) http.RoundTripper {
	if proxyStr := strings.TrimSpace(requestProxyURL); proxyStr != "" {
		if transport := buildProxyTransport(proxyStr); transport != nil {
			return transport
		}
		return directAPICallTransport()
	}

	var proxyCandidates []string
	if auth != nil {
		if proxyStr := strings.TrimSpace(auth.ProxyURL); proxyStr != "" {
			proxyCandidates = append(proxyCandidates, proxyStr)
		}
		if h != nil && h.cfg != nil {
			if proxyStr := strings.TrimSpace(proxyURLFromAPIKeyConfig(h.cfg, auth)); proxyStr != "" {
				proxyCandidates = append(proxyCandidates, proxyStr)
			}
		}
	}
	if h != nil && h.cfg != nil {
		if proxyStr := strings.TrimSpace(h.cfg.ProxyURL); proxyStr != "" {
			proxyCandidates = append(proxyCandidates, proxyStr)
		}
	}

	for _, proxyStr := range proxyCandidates {
		if transport := buildProxyTransport(proxyStr); transport != nil {
			return transport
		}
	}

	return directAPICallTransport()
}

func directAPICallTransport() http.RoundTripper {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok || transport == nil {
		return &http.Transport{Proxy: nil}
	}
	clone := transport.Clone()
	clone.Proxy = nil
	return clone
}

func firstNonEmptyString(values ...*string) string {
	for _, v := range values {
		if v == nil {
			continue
		}
		if out := strings.TrimSpace(*v); out != "" {
			return out
		}
	}
	return ""
}
