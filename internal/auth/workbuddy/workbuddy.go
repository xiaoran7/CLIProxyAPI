// Package workbuddy provides authentication and token management for WorkBuddy and CodeBuddy API.
package workbuddy

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
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

	// DefaultUserAgentIntl is the client User-Agent matching the international desktop app (5.8.4 capture).
	DefaultUserAgentIntl = "WorkBuddy/5.8.4 workbuddy-ai/5.8.4 CLI/2.161.0"
	// DefaultUserAgentCN is the client User-Agent matching mainland China desktop app.
	DefaultUserAgentCN = "WorkBuddy/5.5.6 WorkBuddy/5.5.6 CLI/2.137.1"
	// DefaultUserAgent matches the default international desktop app.
	DefaultUserAgent = DefaultUserAgentIntl

	// DefaultIDEVersionIntl is the international desktop client version.
	DefaultIDEVersionIntl = "5.8.4"
	// DefaultIDEVersionCN is the mainland China desktop client version.
	DefaultIDEVersionCN = "5.5.6"

	// DefaultIDEName is the X-IDE-Name header value.
	DefaultIDEName = "WorkBuddy"
	// DefaultIDEType is the X-IDE-Type header value.
	DefaultIDEType = "WorkBuddy"
	// DefaultProduct is the X-Product header value ("SaaS" per desktop 5.8.4 capture).
	DefaultProduct = "SaaS"
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

// DeriveUUID computes MD5(salt + ":" + uid) formatted as a deterministic UUIDv4-like string,
// matching the hardware/session UUID format used by the desktop client.
func DeriveUUID(salt, uid string) string {
	sum := md5.Sum([]byte(salt + ":" + uid))
	h := hex.EncodeToString(sum[:])
	h = h[:12] + "4" + h[13:16] + "a" + h[17:]
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

func newHexID(byteLen int) string {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprintf("%016x", time.Now().UnixNano())))[:byteLen*2]
	}
	return hex.EncodeToString(b)
}

func newUUIDv7OrV4() string {
	if u, err := uuid.NewV7(); err == nil {
		return u.String()
	}
	return uuid.New().String()
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

	ua := DefaultUserAgentIntl
	ideVer := DefaultIDEVersionIntl
	if realm == RealmCN {
		ua = DefaultUserAgentCN
		ideVer = DefaultIDEVersionCN
	}

	convID := newUUIDv7OrV4()
	convReqID := newHexID(16) // 32 hex chars
	msgID := newHexID(16)     // 32 hex chars
	traceID := newHexID(16)   // 32 hex chars
	spanID := newHexID(8)     // 16 hex chars

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", ua)
	req.Header.Set("X-IDE-Name", DefaultIDEName)
	req.Header.Set("X-IDE-Type", DefaultIDEType)
	req.Header.Set("X-IDE-Version", ideVer)
	req.Header.Set("X-Product", DefaultProduct)
	req.Header.Set("X-Domain", domain)
	req.Header.Set("X-Agent-Intent", "craft")
	req.Header.Set("X-Agent-Purpose", "conversation")
	req.Header.Set("X-Agent-Type", "main")
	req.Header.Set("X-Private-Data", "true")

	// Conversation header family: required by upstream WAF / channel validation to avoid 400 / 11-128
	req.Header.Set("X-Conversation-ID", convID)
	req.Header.Set("X-Conversation-Request-ID", convReqID)
	req.Header.Set("X-Root-Request-ID", convReqID)
	req.Header.Set("X-Conversation-Message-ID", msgID)
	req.Header.Set("X-Request-ID", msgID)

	// Distributed tracing headers (matching desktop 5.8.4 capture)
	req.Header.Set("X-Trace-ID", traceID)
	req.Header.Set("traceparent", fmt.Sprintf("00-%s-%s-01", traceID, spanID))
	req.Header.Set("b3", fmt.Sprintf("%s-%s-1", traceID, spanID))
	req.Header.Set("X-B3-TraceId", traceID)
	req.Header.Set("X-B3-SpanId", spanID)
	req.Header.Set("X-B3-Sampled", "1")

	// Machine / session fingerprint headers
	machineID := DeriveID("machine", uid)
	sessionID := DeriveID("session", uid)
	req.Header.Set("X-Machine-ID", machineID)
	req.Header.Set("X-Session-ID", sessionID)
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

		ua := DefaultUserAgentIntl
		if realm == RealmCN {
			ua = DefaultUserAgentCN
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("User-Agent", ua)
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
