// Package workbuddy provides authentication and token management for WorkBuddy and CodeBuddy API.
package workbuddy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// AuthStatePath initiates an interactive login session.
	AuthStatePath = "/v2/plugin/auth/state"
	// AuthTokenPath polls for authorization completion.
	AuthTokenPath = "/v2/plugin/auth/token"
	// LoginAccountPath queries account profile info.
	LoginAccountPath = "/v2/plugin/login/account"

	// LoginPendingCode is the response code returned by WorkBuddy while awaiting user authorization.
	LoginPendingCode = 11217
	// LoginTTL is the expiration window for an initiated login session.
	LoginTTL = 10 * time.Minute
)

// AuthSession holds the initiated authentication session from WorkBuddy.
type AuthSession struct {
	State    string `json:"state"`
	AuthURL  string `json:"auth_url"`
	Realm    string `json:"realm"`
	Platform string `json:"platform"`
}

// TokenBundle contains the authorized tokens and user identity.
type TokenBundle struct {
	Realm        string `json:"realm"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	UserID       string `json:"user_id"`
	Nickname     string `json:"nickname"`
	Domain       string `json:"domain"`
	BaseURL      string `json:"base_url"`
}

// StartLogin initiates a WorkBuddy login session and returns state and auth URL.
func StartLogin(ctx context.Context, httpClient *http.Client, realm, platform string) (*AuthSession, error) {
	if strings.TrimSpace(realm) == "" {
		realm = RealmIntl
	}
	if strings.TrimSpace(platform) == "" {
		platform = "CLI"
	}

	baseURL := ResolveBaseURL(realm)
	origin := baseURL
	if strings.EqualFold(realm, RealmCN) {
		origin = "https://www.codebuddy.cn"
	}

	reqURL := fmt.Sprintf("%s%s?platform=%s", baseURL, AuthStatePath, url.QueryEscape(platform))
	req, errReq := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader([]byte("{}")))
	if errReq != nil {
		return nil, fmt.Errorf("workbuddy: create start login request failed: %w", errReq)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", origin+"/")

	client := httpClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, errDo := client.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("workbuddy: start login request failed: %w", errDo)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return nil, fmt.Errorf("workbuddy: read start login response failed: %w", errRead)
	}

	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			State   string `json:"state"`
			AuthURL string `json:"authUrl"`
		} `json:"data"`
	}
	if errJSON := json.Unmarshal(body, &parsed); errJSON != nil {
		return nil, fmt.Errorf("workbuddy: parse start login response failed: %w (body: %s)", errJSON, string(body))
	}
	if parsed.Code != 0 {
		return nil, fmt.Errorf("workbuddy: start login returned code %d: %s", parsed.Code, parsed.Msg)
	}
	if strings.TrimSpace(parsed.Data.State) == "" || strings.TrimSpace(parsed.Data.AuthURL) == "" {
		return nil, fmt.Errorf("workbuddy: start login returned empty state or authUrl: %s", string(body))
	}

	return &AuthSession{
		State:    parsed.Data.State,
		AuthURL:  parsed.Data.AuthURL,
		Realm:    realm,
		Platform: platform,
	}, nil
}

// PollToken polls the WorkBuddy authorization endpoint once.
// Returns (bundle, isPending, error).
func PollToken(ctx context.Context, httpClient *http.Client, realm, state string) (*TokenBundle, bool, error) {
	if strings.TrimSpace(state) == "" {
		return nil, false, errors.New("workbuddy: state is required")
	}
	if strings.TrimSpace(realm) == "" {
		realm = RealmIntl
	}

	baseURL := ResolveBaseURL(realm)
	origin := baseURL
	if strings.EqualFold(realm, RealmCN) {
		origin = "https://www.codebuddy.cn"
	}

	reqURL := fmt.Sprintf("%s%s?state=%s", baseURL, AuthTokenPath, url.QueryEscape(state))
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if errReq != nil {
		return nil, false, fmt.Errorf("workbuddy: create poll token request failed: %w", errReq)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", origin+"/")

	client := httpClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, errDo := client.Do(req)
	if errDo != nil {
		return nil, true, fmt.Errorf("workbuddy: poll token request failed: %w", errDo)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return nil, true, fmt.Errorf("workbuddy: read poll token response failed: %w", errRead)
	}

	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			Domain       string `json:"domain"`
			ExpiresAt    any    `json:"expiresAt"`
		} `json:"data"`
	}
	if errJSON := json.Unmarshal(body, &parsed); errJSON != nil {
		return nil, true, fmt.Errorf("workbuddy: parse poll token response failed: %w", errJSON)
	}

	if parsed.Code == LoginPendingCode {
		return nil, true, nil
	}
	if parsed.Code != 0 {
		return nil, false, fmt.Errorf("workbuddy: login failed with code %d: %s", parsed.Code, parsed.Msg)
	}
	if strings.TrimSpace(parsed.Data.AccessToken) == "" {
		return nil, true, nil
	}

	token := strings.TrimSpace(parsed.Data.AccessToken)
	uid, exp := ParseJWTClaims(token)

	// If expiresAt was provided in response payload, use it if valid
	var expiresAtSec int64 = exp
	switch v := parsed.Data.ExpiresAt.(type) {
	case float64:
		if v > 0 {
			if v > 1e11 {
				expiresAtSec = int64(v / 1000)
			} else {
				expiresAtSec = int64(v)
			}
		}
	case int64:
		if v > 0 {
			if v > 1e11 {
				expiresAtSec = v / 1000
			} else {
				expiresAtSec = v
			}
		}
	}

	domain := strings.TrimSpace(parsed.Data.Domain)
	if domain == "" {
		domain = ResolveDomain(realm)
	}

	nickname := fetchNickname(ctx, client, baseURL, origin, state, token)

	return &TokenBundle{
		Realm:        realm,
		AccessToken:  token,
		RefreshToken: strings.TrimSpace(parsed.Data.RefreshToken),
		ExpiresAt:    expiresAtSec,
		UserID:       uid,
		Nickname:     nickname,
		Domain:       domain,
		BaseURL:      baseURL,
	}, false, nil
}

// WaitForAuthorization continuously polls until the token is ready, context canceled, or timeout.
func WaitForAuthorization(ctx context.Context, httpClient *http.Client, realm, state string) (*TokenBundle, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			bundle, isPending, err := PollToken(ctx, httpClient, realm, state)
			if err != nil {
				if isPending {
					// Transient network/poll error, continue waiting
					continue
				}
				return nil, err
			}
			if !isPending && bundle != nil {
				return bundle, nil
			}
		}
	}
}

// fetchNickname attempts to query user profile for display name.
func fetchNickname(ctx context.Context, client *http.Client, baseURL, origin, state, token string) string {
	acctURL := fmt.Sprintf("%s%s?state=%s", baseURL, LoginAccountPath, url.QueryEscape(state))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, acctURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", origin+"/")

	resp, errDo := client.Do(req)
	if errDo != nil {
		return ""
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return ""
	}

	var parsed struct {
		Code int `json:"code"`
		Data struct {
			Nickname string `json:"nickname"`
		} `json:"data"`
	}
	if errJSON := json.Unmarshal(body, &parsed); errJSON == nil && parsed.Code == 0 {
		return strings.TrimSpace(parsed.Data.Nickname)
	}
	return ""
}

// ParseJWTClaims extracts user ID and expiration epoch seconds from standard JWT token without verifying signature.
func ParseJWTClaims(token string) (uid string, exp int64) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return "", 0
	}
	seg := parts[1]
	// Pad base64 segment if needed
	switch len(seg) % 4 {
	case 2:
		seg += "=="
	case 3:
		seg += "="
	}

	data, err := base64.URLEncoding.DecodeString(seg)
	if err != nil {
		data, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return "", 0
		}
	}

	var claims map[string]any
	if errUnmarshal := json.Unmarshal(data, &claims); errUnmarshal != nil {
		return "", 0
	}

	for _, key := range []string{"uid", "user_id", "sub", "id"} {
		if val, ok := claims[key].(string); ok && strings.TrimSpace(val) != "" {
			uid = strings.TrimSpace(val)
			break
		}
	}

	if expVal, ok := claims["exp"]; ok {
		switch v := expVal.(type) {
		case float64:
			exp = int64(v)
		case int64:
			exp = v
		}
	}

	return uid, exp
}
