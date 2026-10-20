// Package qoder provides authentication, COSY request signing, and token management
// for Qoder Global and Qoder Mainland China APIs.
package qoder

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	Realm        string `json:"realm"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	UserID       string `json:"user_id"`
	Nickname     string `json:"nickname"`
	MachineID    string `json:"machine_id"`
	BaseURL      string `json:"base_url"`
	OpenAPIBase  string `json:"openapi_base"`
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

	body, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return nil, true, fmt.Errorf("qoder: read poll response: %w", errRead)
	}

	parsed := gjson.ParseBytes(body)
	code := parsed.Get("code").Int()

	// Handle pending authorization states
	if code == LoginPendingCode || code == 10001 || parsed.Get("status").String() == "pending" {
		return nil, true, nil
	}
	if code != 0 && code != 200 {
		msg := parsed.Get("msg").String()
		if msg == "" {
			msg = parsed.Get("message").String()
		}
		if strings.Contains(strings.ToLower(msg), "pending") || strings.Contains(strings.ToLower(msg), "waiting") {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("qoder: login returned code %d: %s", code, msg)
	}

	accessToken := parsed.Get("data.access_token").String()
	if accessToken == "" {
		accessToken = parsed.Get("data.accessToken").String()
	}
	if accessToken == "" {
		accessToken = parsed.Get("access_token").String()
	}
	if accessToken == "" {
		return nil, true, nil
	}

	refreshToken := parsed.Get("data.refresh_token").String()
	if refreshToken == "" {
		refreshToken = parsed.Get("data.refreshToken").String()
	}
	userID := parsed.Get("data.user_id").String()
	if userID == "" {
		userID = parsed.Get("data.userId").String()
	}
	nickname := parsed.Get("data.nickname").String()
	if nickname == "" {
		nickname = parsed.Get("data.name").String()
	}

	expiresIn := parsed.Get("data.expires_in").Int()
	var expiresAt int64
	if expiresIn > 0 {
		expiresAt = time.Now().Unix() + expiresIn
	}

	return &TokenBundle{
		Realm:        realm,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		UserID:       userID,
		Nickname:     nickname,
		MachineID:    machineID,
		BaseURL:      ResolveBaseURL(realm),
		OpenAPIBase:  openAPIBase,
	}, false, nil
}

// WaitForAuthorization continuously polls until the user approves device login or context expires.
func WaitForAuthorization(ctx context.Context, httpClient *http.Client, realm, nonce, verifier, machineID string) (*TokenBundle, error) {
	ticker := time.NewTicker(LoginPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			bundle, isPending, err := PollToken(ctx, httpClient, realm, nonce, verifier, machineID)
			if err != nil {
				// Temporary network hiccups during polling are tolerated
				continue
			}
			if isPending {
				continue
			}
			if bundle != nil {
				return bundle, nil
			}
		}
	}
}
