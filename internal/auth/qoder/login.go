// Package qoder provides authentication, COSY request signing, and token management
// for Qoder Global and Qoder Mainland China APIs.
package qoder

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	// DeviceTokenPollPath polls for OAuth device authorization completion.
	DeviceTokenPollPath = "/api/v1/deviceToken/poll"
	// DeviceTokenRefreshPath refreshes OAuth device access tokens.
	DeviceTokenRefreshPath = "/api/v1/deviceToken/refresh"
	// JobTokenPath trades a device token for a short-lived job token.
	JobTokenPath = "/api/v1/me/jobToken"

	// LoginPendingCode is returned while awaiting user browser authorization.
	LoginPendingCode = 11217
	// LoginPollInterval is the delay between polling cycles.
	LoginPollInterval = 2 * time.Second
	// LoginTimeout is the maximum duration for an interactive device login session.
	LoginTimeout = 10 * time.Minute
)

// AuthSession holds the initiated device login session.
type AuthSession struct {
	AuthURL   string `json:"auth_url"`
	Nonce     string `json:"nonce"`
	Verifier  string `json:"verifier"`
	MachineID string `json:"machine_id"`
	Realm     string `json:"realm"`
}

// TokenBundle contains the authorized tokens and identity.
type TokenBundle struct {
	Realm         string `json:"realm"`
	AccessToken   string `json:"access_token"`
	RefreshToken  string `json:"refresh_token"`
	DeviceToken   string `json:"device_token"`
	DeviceRefresh string `json:"device_refresh"`
	JobToken      string `json:"job_token,omitempty"`
	JobRefresh    string `json:"job_refresh,omitempty"`
	ExpiresAt     int64  `json:"expires_at"`
	UserID        string `json:"user_id"`
	Nickname      string `json:"nickname"`
	Email         string `json:"email,omitempty"`
	MachineID     string `json:"machine_id"`
	BaseURL       string `json:"base_url"`
	OpenAPIBase   string `json:"openapi_base"`
	DeviceChat    bool   `json:"device_chat,omitempty"`
}

// UserInfo holds user identity retrieved from the Qoder OpenAPI account endpoint.
type UserInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	Avatar      string `json:"avatar,omitempty"`
	Source      string `json:"source,omitempty"`
	OrgName     string `json:"organization_name,omitempty"`
	HighestTier bool   `json:"is_highest_tier,omitempty"`
}

// generatePKCE generates a PKCE code verifier and S256 challenge.
func generatePKCE() (verifier, challenge string) {
	b := NewRandomBytes(32)
	verifier = base64.RawURLEncoding.EncodeToString(b)
	hash := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(hash[:])
	return verifier, challenge
}

// StartLogin initiates a Qoder device authorization session and returns the AuthURL and session context.
func StartLogin(realm string) (*AuthSession, error) {
	if strings.TrimSpace(realm) == "" {
		realm = RealmIntl
	}
	authWeb := ResolveAuthWebURL(realm)
	clientID := ResolveClientID(realm)
	machineID := uuid.New().String()
	nonce := hex.EncodeToString(NewRandomBytes(16))
	verifier, challenge := generatePKCE()

	authURL := fmt.Sprintf(
		"%s/device/selectAccounts?challenge=%s&challenge_method=S256&nonce=%s&machine_id=%s&client_id=%s",
		authWeb,
		url.QueryEscape(challenge),
		url.QueryEscape(nonce),
		url.QueryEscape(machineID),
		url.QueryEscape(clientID),
	)

	return &AuthSession{
		AuthURL:   authURL,
		Nonce:     nonce,
		Verifier:  verifier,
		MachineID: machineID,
		Realm:     realm,
	}, nil
}

// FetchUserInfo retrieves user identity info from /api/v1/userinfo using a device token.
func FetchUserInfo(ctx context.Context, httpClient *http.Client, realm, deviceToken string) (*UserInfo, error) {
	if strings.TrimSpace(deviceToken) == "" {
		return nil, errors.New("qoder: missing device token")
	}
	openAPIBase := ResolveOpenAPIBaseURL(realm)
	reqURL := openAPIBase + UserInfoPath

	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if errReq != nil {
		return nil, fmt.Errorf("qoder: create userinfo request: %w", errReq)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	req.Header.Set("User-Agent", DefaultUserAgent)

	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, errDo := client.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("qoder: userinfo request failed: %w", errDo)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, errRead := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if errRead != nil {
		return nil, fmt.Errorf("qoder: read userinfo response: %w", errRead)
	}

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("qoder: userinfo returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var u UserInfo
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("qoder: decode userinfo: %w", err)
	}
	return &u, nil
}

// TradeJobToken trades a device token for a short-lived job token for model inference.
func TradeJobToken(ctx context.Context, httpClient *http.Client, realm, deviceToken string) (*JobTokenResponse, error) {
	if strings.TrimSpace(deviceToken) == "" {
		return nil, errors.New("qoder: device token is empty")
	}
	openAPIBase := ResolveOpenAPIBaseURL(realm)
	clientID := ResolveClientID(realm)
	reqURL := openAPIBase + JobTokenPath

	payloadBytes, errMarshal := json.Marshal(map[string]string{
		"clientId": clientID,
	})
	if errMarshal != nil {
		return nil, fmt.Errorf("qoder: marshal jobToken payload: %w", errMarshal)
	}

	req, errReq := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payloadBytes))
	if errReq != nil {
		return nil, fmt.Errorf("qoder: create jobToken request: %w", errReq)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	req.Header.Set("User-Agent", DefaultUserAgent)

	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, errDo := client.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("qoder: trade job token request failed: %w", errDo)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, errRead := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if errRead != nil {
		return nil, fmt.Errorf("qoder: read jobToken response: %w", errRead)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qoder: jobToken returned status %d: %s", resp.StatusCode, string(body))
	}

	parsed := gjson.ParseBytes(body)
	token := parsed.Get("token").String()
	if token == "" {
		token = parsed.Get("job_token").String()
	}
	if token == "" {
		token = parsed.Get("data.token").String()
	}
	if token == "" {
		token = parsed.Get("data.job_token").String()
	}
	if token == "" {
		return nil, fmt.Errorf("qoder: jobToken response missing token: %s", string(body))
	}

	refreshToken := parsed.Get("refresh_token").String()
	if refreshToken == "" {
		refreshToken = parsed.Get("data.refresh_token").String()
	}
	expiresIn := parsed.Get("expires_in").Int()
	if expiresIn <= 0 {
		expiresIn = parsed.Get("data.expires_in").Int()
	}

	return &JobTokenResponse{
		JobToken:     token,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
	}, nil
}

// PollToken polls the deviceToken/poll endpoint once.
// Returns (bundle, isPending, error).
func PollToken(ctx context.Context, httpClient *http.Client, realm, nonce, verifier, machineID string) (*TokenBundle, bool, error) {
	if strings.TrimSpace(nonce) == "" || strings.TrimSpace(verifier) == "" {
		return nil, false, errors.New("qoder: nonce and verifier are required")
	}
	openAPIBase := ResolveOpenAPIBaseURL(realm)
	reqURL := fmt.Sprintf(
		"%s%s?nonce=%s&verifier=%s&challenge_method=S256",
		openAPIBase,
		DeviceTokenPollPath,
		url.QueryEscape(nonce),
		url.QueryEscape(verifier),
	)

	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if errReq != nil {
		return nil, false, fmt.Errorf("qoder: create poll request: %w", errReq)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", DefaultUserAgent)

	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, errDo := client.Do(req)
	if errDo != nil {
		return nil, true, fmt.Errorf("qoder: poll request: %w", errDo)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, errRead := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if errRead != nil {
		return nil, true, fmt.Errorf("qoder: read poll response: %w", errRead)
	}

	// 1. Check HTTP status code for pending states:
	// Qoder returns HTTP 404 {"errorCode":"NotFound","errorMessage":"Not found"} or 400/403 while waiting for user authorization.
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusForbidden {
		parsedErr := gjson.ParseBytes(body)
		code := parsedErr.Get("code").Int()
		errCode := parsedErr.Get("errorCode").String()
		msg := strings.ToLower(parsedErr.Get("errorMessage").String() + " " + parsedErr.Get("message").String() + " " + parsedErr.Get("msg").String() + " " + errCode)
		if resp.StatusCode == http.StatusNotFound || strings.Contains(msg, "not found") || strings.Contains(msg, "notfound") || strings.Contains(msg, "pending") || strings.Contains(msg, "waiting") || code == LoginPendingCode || code == 10001 {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("qoder: poll returned status %d: %s", resp.StatusCode, string(body))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("qoder: poll returned status %d: %s", resp.StatusCode, string(body))
	}

	// 2. Parse successful HTTP 200 response
	parsed := gjson.ParseBytes(body)
	code := parsed.Get("code").Int()

	// Check if body signals pending despite HTTP 200
	if code == LoginPendingCode || code == 10001 || parsed.Get("status").String() == "pending" {
		return nil, true, nil
	}

	// Extract device token: Qoder returns {"token":"dt-...", "device_token":"dt-...", ...}
	deviceToken := parsed.Get("token").String()
	if deviceToken == "" {
		deviceToken = parsed.Get("device_token").String()
	}
	if deviceToken == "" {
		deviceToken = parsed.Get("data.token").String()
	}
	if deviceToken == "" {
		deviceToken = parsed.Get("data.device_token").String()
	}
	if deviceToken == "" {
		deviceToken = parsed.Get("data.access_token").String()
	}
	if deviceToken == "" {
		deviceToken = parsed.Get("access_token").String()
	}
	if deviceToken == "" {
		// No token extracted yet, treat as pending
		return nil, true, nil
	}

	deviceRefresh := parsed.Get("refresh_token").String()
	if deviceRefresh == "" {
		deviceRefresh = parsed.Get("data.refresh_token").String()
	}
	userID := parsed.Get("user_id").String()
	if userID == "" {
		userID = parsed.Get("data.user_id").String()
	}
	userName := parsed.Get("user_name").String()
	if userName == "" {
		userName = parsed.Get("name").String()
	}
	if userName == "" {
		userName = parsed.Get("data.user_name").String()
	}
	if userName == "" {
		userName = parsed.Get("data.name").String()
	}

	expiresIn := parsed.Get("expires_in").Int()
	if expiresIn <= 0 {
		expiresIn = parsed.Get("data.expires_in").Int()
	}
	if expiresIn <= 0 {
		expiresIn = 30 * 24 * 3600 // Default 30 days for Qoder device token
	}
	expiresAt := time.Now().Unix() + expiresIn

	userEmail := ""
	// Enrich user identity via /api/v1/userinfo
	if userInfo, errUser := FetchUserInfo(ctx, client, realm, deviceToken); errUser == nil && userInfo != nil {
		if strings.TrimSpace(userInfo.Name) != "" {
			userName = strings.TrimSpace(userInfo.Name)
		} else if strings.TrimSpace(userInfo.Username) != "" && userName == "" {
			userName = strings.TrimSpace(userInfo.Username)
		}
		if strings.TrimSpace(userInfo.Email) != "" {
			userEmail = strings.TrimSpace(userInfo.Email)
		}
		if strings.TrimSpace(userInfo.ID) != "" && userID == "" {
			userID = strings.TrimSpace(userInfo.ID)
		}
	}

	activeToken := deviceToken
	activeRefresh := deviceRefresh
	jobToken := ""
	jobRefresh := ""
	deviceChat := true

	// Attempt to trade device token for job token (Global realm uses jt-... tokens; CN CLI falls back to dt-...)
	if jtResp, errJob := TradeJobToken(ctx, client, realm, deviceToken); errJob == nil && jtResp != nil && strings.TrimSpace(jtResp.JobToken) != "" {
		jobToken = jtResp.JobToken
		jobRefresh = jtResp.RefreshToken
		activeToken = jobToken
		if jobRefresh != "" {
			activeRefresh = jobRefresh
		}
		deviceChat = false
		if jtResp.ExpiresIn > 0 {
			if jtResp.ExpiresIn > 1000000 {
				expiresAt = time.Now().Unix() + (jtResp.ExpiresIn / 1000)
			} else {
				expiresAt = time.Now().Unix() + jtResp.ExpiresIn
			}
		}
	}

	return &TokenBundle{
		Realm:         realm,
		AccessToken:   activeToken,
		RefreshToken:  activeRefresh,
		DeviceToken:   deviceToken,
		DeviceRefresh: deviceRefresh,
		JobToken:      jobToken,
		JobRefresh:    jobRefresh,
		ExpiresAt:     expiresAt,
		UserID:        userID,
		Nickname:      userName,
		Email:         userEmail,
		MachineID:     machineID,
		BaseURL:       ResolveBaseURL(realm),
		OpenAPIBase:   openAPIBase,
		DeviceChat:    deviceChat,
	}, false, nil
}

// WaitForAuthorization continuously polls until the user approves device login or context expires.
func WaitForAuthorization(ctx context.Context, httpClient *http.Client, realm, nonce, verifier, machineID string) (*TokenBundle, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, LoginTimeout)
		defer cancel()
	}

	ticker := time.NewTicker(LoginPollInterval)
	defer ticker.Stop()

	var consecutiveErrors int
	const maxConsecutiveErrors = 15

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			bundle, isPending, err := PollToken(ctx, httpClient, realm, nonce, verifier, machineID)
			if err != nil {
				consecutiveErrors++
				if consecutiveErrors >= maxConsecutiveErrors {
					return nil, fmt.Errorf("qoder: polling aborted after %d consecutive errors: %w", consecutiveErrors, err)
				}
				continue
			}
			consecutiveErrors = 0
			if isPending {
				continue
			}
			if bundle != nil {
				return bundle, nil
			}
		}
	}
}
