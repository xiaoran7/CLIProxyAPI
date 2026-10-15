// Package workbuddy provides authentication and token management for WorkBuddy and CodeBuddy API.
package workbuddy

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"golang.org/x/sync/singleflight"
)

const (
	// RealmIntl identifies the international realm (workbuddy.ai).
	RealmIntl = "intl"
	// RealmCN identifies the mainland China realm (copilot.tencent.com).
	RealmCN = "cn"

	// BaseURLIntl is the API origin for international WorkBuddy.
	BaseURLIntl = "https://www.workbuddy.ai"
	// BaseURLCN is the API origin for mainland China CodeBuddy / Copilot.
	BaseURLCN = "https://copilot.tencent.com"

	// DomainIntl is the X-Domain header value for international WorkBuddy.
	DomainIntl = "www.workbuddy.ai"
	// DomainCN is the X-Domain header value for mainland China CodeBuddy.
	DomainCN = "copilot.tencent.com"

	// ChatCompletionsPath is the chat completions endpoint path.
	ChatCompletionsPath = "/v2/chat/completions"
	// TokenRefreshPath is the token refresh endpoint path.
	TokenRefreshPath = "/v2/plugin/auth/token/refresh"

	// DefaultUserAgent is the client User-Agent matching the desktop app.
	DefaultUserAgent = "WorkBuddy/5.5.2 WorkBuddy AI/5.5.2 CLI/5.5.2"
	// DefaultIDEName is the X-IDE-Name header value.
	DefaultIDEName = "WorkBuddy"
	// DefaultIDEType is the X-IDE-Type header value.
	DefaultIDEType = "WorkBuddy"
	// DefaultProduct is the X-Product header value.
	DefaultProduct = "WorkBuddy"
)

var refreshGroup singleflight.Group

// TokenData holds the rotated token payload from WorkBuddy.
type TokenData struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// DeriveID computes MD5(salt + ":" + uid) as a 32-character lowercase hex string.
func DeriveID(salt, uid string) string {
	sum := md5.Sum([]byte(salt + ":" + uid))
	return hex.EncodeToString(sum[:])
}

// ResolveRealm determines whether the auth belongs to intl or cn.
func ResolveRealm(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return RealmIntl
	}
	if auth.Attributes != nil {
		if r, ok := auth.Attributes["realm"]; ok && strings.TrimSpace(r) != "" {
			if strings.EqualFold(r, RealmCN) {
				return RealmCN
			}
			return RealmIntl
		}
	}
	if auth.Metadata != nil {
		if r, ok := auth.Metadata["realm"].(string); ok && strings.TrimSpace(r) != "" {
			if strings.EqualFold(r, RealmCN) {
				return RealmCN
			}
			return RealmIntl
		}
	}
	p := strings.ToLower(strings.TrimSpace(auth.Provider))
	if p == "workbuddy-cn" || p == "copilot-tencent" {
		return RealmCN
	}
	if auth.Attributes != nil {
		bu := strings.ToLower(auth.Attributes["base_url"])
		if strings.Contains(bu, "tencent.com") || strings.Contains(bu, "codebuddy.cn") {
			return RealmCN
		}
	}
	return RealmIntl
}

// ResolveBaseURL returns the base server URL for the given realm.
func ResolveBaseURL(realm string) string {
	if strings.EqualFold(realm, RealmCN) {
		return BaseURLCN
	}
	return BaseURLIntl
}

// ResolveDomain returns the X-Domain header value for the given realm.
func ResolveDomain(realm string) string {
	if strings.EqualFold(realm, RealmCN) {
		return DomainCN
	}
	return DomainIntl
}

// ApplyHeaders injects WorkBuddy device fingerprint and client headers.
// AccessToken returns the bearer token held by an auth object, or an empty
// string when the auth carries no usable token.
func AccessToken(auth *cliproxyauth.Auth) string {
	return extractAccessToken(auth)
}

// ResolveUserID returns the upstream user identifier used by the desktop client
// headers, falling back to the auth ID and then "anonymous".
func ResolveUserID(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return "anonymous"
	}
	if auth.Metadata != nil {
		if u, ok := auth.Metadata["user_id"].(string); ok && u != "" {
			return u
		}
		if u, ok := auth.Metadata["uid"].(string); ok && u != "" {
			return u
		}
	}
	if auth.ID != "" {
		return auth.ID
	}
	return "anonymous"
}

func ApplyHeaders(req *http.Request, auth *cliproxyauth.Auth, token string) {
	if req == nil {
		return
	}
	realm := ResolveRealm(auth)
	domain := ResolveDomain(realm)

	uid := ResolveUserID(auth)

	machineID := DeriveID("machine", uid)
	sessionID := DeriveID("session", uid)
	requestID := DeriveID("req", uid) + "-" + fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("X-IDE-Name", DefaultIDEName)
	req.Header.Set("X-IDE-Type", DefaultIDEType)
	req.Header.Set("X-Product", DefaultProduct)
	req.Header.Set("X-Domain", domain)
	req.Header.Set("X-Machine-ID", machineID)
	req.Header.Set("X-Session-ID", sessionID)
	req.Header.Set("X-Request-ID", requestID)
}

// ApplyCatalogHeaders applies the headers the desktop client sends when reading
// its configuration catalogue. ApplyHeaders covers ordinary API traffic; the
// catalogue additionally identifies the account.
func ApplyCatalogHeaders(req *http.Request, auth *cliproxyauth.Auth, token string) {
	ApplyHeaders(req, auth, token)
	if req == nil || auth == nil {
		return
	}
	uid := ResolveUserID(auth)
	if uid != "" && uid != "anonymous" {
		req.Header.Set("X-User-Id", uid)
	}
}

// RefreshToken executes token refresh against /v2/plugin/auth/token/refresh with singleflight protection.
func RefreshToken(ctx context.Context, httpClient *http.Client, realm, refreshToken, uid string) (*TokenData, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, errors.New("workbuddy: refresh token is empty")
	}

	key := realm + ":" + refreshToken
	res, err, _ := refreshGroup.Do(key, func() (any, error) {
		baseURL := ResolveBaseURL(realm)
		domain := ResolveDomain(realm)
		refreshURL := baseURL + TokenRefreshPath

		req, errReq := http.NewRequestWithContext(ctx, http.MethodPost, refreshURL, bytes.NewReader([]byte("{}")))
		if errReq != nil {
			return nil, fmt.Errorf("workbuddy: create refresh request failed: %w", errReq)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("User-Agent", DefaultUserAgent)
		req.Header.Set("Origin", baseURL)
		req.Header.Set("Referer", baseURL+"/")
		req.Header.Set("X-Refresh-Token", refreshToken)
		req.Header.Set("X-Auth-Refresh-Source", "plugin")
		req.Header.Set("X-User-Id", uid)
		req.Header.Set("X-Domain", domain)
		req.Header.Set("X-CodeBuddy-Request", "1")
		if realm == RealmIntl {
			req.Header.Set("Accept-Language", "en-US")
		} else {
			req.Header.Set("Accept-Language", "zh-CN")
		}

		client := httpClient
		if client == nil {
			client = http.DefaultClient
		}

		resp, errDo := client.Do(req)
		if errDo != nil {
			return nil, fmt.Errorf("workbuddy: execute refresh request failed: %w", errDo)
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		body, errRead := io.ReadAll(resp.Body)
		if errRead != nil {
			return nil, fmt.Errorf("workbuddy: read refresh response failed: %w", errRead)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("workbuddy: refresh returned status %d: %s", resp.StatusCode, string(body))
		}

		// Handle flat response or Tencent envelope (code/msg/data)
		var envelope struct {
			Code    int             `json:"code"`
			Message string          `json:"msg"`
			Data    json.RawMessage `json:"data"`
		}

		var tokenResp struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			Token        string `json:"access_token"`
			RefToken     string `json:"refresh_token"`
			ExpiresIn    int64  `json:"expires_in"`
		}

		if errUnmarshal := json.Unmarshal(body, &envelope); errUnmarshal == nil && len(envelope.Data) > 0 {
			if errData := json.Unmarshal(envelope.Data, &tokenResp); errData != nil {
				_ = json.Unmarshal(body, &tokenResp)
			}
		} else {
			_ = json.Unmarshal(body, &tokenResp)
		}

		accessToken := tokenResp.AccessToken
		if accessToken == "" {
			accessToken = tokenResp.Token
		}
		newRefreshToken := tokenResp.RefreshToken
		if newRefreshToken == "" {
			newRefreshToken = tokenResp.RefToken
		}
		if newRefreshToken == "" {
			newRefreshToken = refreshToken
		}

		if accessToken == "" {
			return nil, fmt.Errorf("workbuddy: refresh response contained no access token: %s", string(body))
		}

		expiresIn := tokenResp.ExpiresIn
		if expiresIn <= 0 {
			expiresIn = 7200
		}

		return &TokenData{
			AccessToken:  accessToken,
			RefreshToken: newRefreshToken,
			ExpiresIn:    expiresIn,
			TokenType:    "Bearer",
		}, nil
	})

	if err != nil {
		return nil, err
	}
	return res.(*TokenData), nil
}
