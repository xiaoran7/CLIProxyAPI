// Package qoder provides authentication, COSY request signing, and token management
// for Qoder Global and Qoder Mainland China APIs.
package qoder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/misc"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/tidwall/gjson"
	"golang.org/x/sync/singleflight"
)

const (
	// JobTokenExchangePath exchanges a Personal Access Token (PAT) for a short-lived Job Token.
	JobTokenExchangePath = "/api/v1/jobToken/exchange"
	// JobTokenRefreshPath refreshes an active Job Token using a job refresh token.
	JobTokenRefreshPath = "/api/v1/jobToken/refresh"
)

var exchangeGroup singleflight.Group

// TokenData holds the persisted token state for a Qoder account.
type TokenData struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	JobToken     string `json:"job_token,omitempty"`
	JobRefresh   string `json:"job_refresh,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
	Realm        string `json:"realm,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	Nickname     string `json:"nickname,omitempty"`
}

// QoderTokenStorage matches the auth json storage schema in auths/ directory.
type QoderTokenStorage struct {
	Type         string         `json:"type"`
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token,omitempty"`
	JobToken     string         `json:"job_token,omitempty"`
	JobRefresh   string         `json:"job_refresh,omitempty"`
	Realm        string         `json:"realm"`
	BaseURL      string         `json:"base_url"`
	OpenAPIBase  string         `json:"openapi_base"`
	UserID       string         `json:"user_id,omitempty"`
	Nickname     string         `json:"nickname,omitempty"`
	MachineID    string         `json:"machine_id,omitempty"`
	Expired      string         `json:"expired,omitempty"`
	Metadata     map[string]any `json:"-"`
}

// SetMetadata allows external callers to inject metadata into the storage before saving.
func (ts *QoderTokenStorage) SetMetadata(meta map[string]any) {
	ts.Metadata = meta
}

// SaveTokenToFile serializes the Qoder token storage to a JSON file.
func (ts *QoderTokenStorage) SaveTokenToFile(authFilePath string) error {
	misc.LogSavingCredentials(authFilePath)
	if ts.Type == "" {
		ts.Type = "qoder"
	}
	if ts.Realm == "" {
		ts.Realm = RealmIntl
	}
	if ts.BaseURL == "" {
		ts.BaseURL = ResolveBaseURL(ts.Realm)
	}
	if ts.OpenAPIBase == "" {
		ts.OpenAPIBase = ResolveOpenAPIBaseURL(ts.Realm)
	}

	if err := os.MkdirAll(filepath.Dir(authFilePath), 0o700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, errMerge := misc.MergeMetadata(ts, ts.Metadata)
	if errMerge != nil {
		return fmt.Errorf("failed to merge metadata: %w", errMerge)
	}

	f, err := os.Create(authFilePath)
	if err != nil {
		return fmt.Errorf("failed to create token file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}
	return nil
}

// JobTokenResponse holds the response from the jobToken exchange or refresh endpoint.
type JobTokenResponse struct {
	JobToken     string `json:"job_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// ExchangeJobToken exchanges a Personal Access Token (PAT) for a Job Token.
func ExchangeJobToken(ctx context.Context, httpClient *http.Client, realm, pat string) (*JobTokenResponse, error) {
	if strings.TrimSpace(pat) == "" {
		return nil, errors.New("qoder: personal token is empty")
	}
	openAPIBase := ResolveOpenAPIBaseURL(realm)
	reqURL := openAPIBase + JobTokenExchangePath

	payloadBytes, errMarshal := json.Marshal(map[string]string{
		"personal_token": pat,
	})
	if errMarshal != nil {
		return nil, fmt.Errorf("qoder: marshal exchange payload: %w", errMarshal)
	}

	req, errReq := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payloadBytes))
	if errReq != nil {
		return nil, fmt.Errorf("qoder: build exchange request: %w", errReq)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", DefaultUserAgent)

	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, errDo := client.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("qoder: exchange request failed: %w", errDo)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return nil, fmt.Errorf("qoder: read exchange response: %w", errRead)
	}

	parsed := gjson.ParseBytes(body)
	if code := parsed.Get("code").Int(); code != 0 && code != 200 {
		msg := parsed.Get("msg").String()
		if msg == "" {
			msg = parsed.Get("message").String()
		}
		return nil, fmt.Errorf("qoder: exchange failed with code %d: %s", code, msg)
	}

	jobToken := parsed.Get("data.job_token").String()
	if jobToken == "" {
		jobToken = parsed.Get("data.jobToken").String()
	}
	if jobToken == "" {
		jobToken = parsed.Get("job_token").String()
	}
	if jobToken == "" {
		return nil, fmt.Errorf("qoder: exchange response missing job token: %s", string(body))
	}

	refreshToken := parsed.Get("data.refresh_token").String()
	if refreshToken == "" {
		refreshToken = parsed.Get("data.refreshToken").String()
	}
	expiresIn := parsed.Get("data.expires_in").Int()

	return &JobTokenResponse{
		JobToken:     jobToken,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
	}, nil
}

// RefreshJobToken refreshes an existing Job Token using the job refresh token.
func RefreshJobToken(ctx context.Context, httpClient *http.Client, realm, jrt string) (*JobTokenResponse, error) {
	if strings.TrimSpace(jrt) == "" {
		return nil, errors.New("qoder: job refresh token is empty")
	}
	openAPIBase := ResolveOpenAPIBaseURL(realm)
	reqURL := openAPIBase + JobTokenRefreshPath

	payloadBytes, errMarshal := json.Marshal(map[string]string{
		"refresh_token": jrt,
	})
	if errMarshal != nil {
		return nil, fmt.Errorf("qoder: marshal refresh payload: %w", errMarshal)
	}

	req, errReq := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payloadBytes))
	if errReq != nil {
		return nil, fmt.Errorf("qoder: build refresh request: %w", errReq)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", DefaultUserAgent)

	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, errDo := client.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("qoder: refresh request failed: %w", errDo)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return nil, fmt.Errorf("qoder: read refresh response: %w", errRead)
	}

	parsed := gjson.ParseBytes(body)
	if code := parsed.Get("code").Int(); code != 0 && code != 200 {
		msg := parsed.Get("msg").String()
		return nil, fmt.Errorf("qoder: refresh failed with code %d: %s", code, msg)
	}

	jobToken := parsed.Get("data.job_token").String()
	if jobToken == "" {
		jobToken = parsed.Get("data.jobToken").String()
	}
	if jobToken == "" {
		return nil, fmt.Errorf("qoder: refresh response missing job token: %s", string(body))
	}

	refreshToken := parsed.Get("data.refresh_token").String()
	expiresIn := parsed.Get("data.expires_in").Int()

	return &JobTokenResponse{
		JobToken:     jobToken,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
	}, nil
}

// EnsureValidJobToken acquires or ensures an active Job Token for the given auth object.
// If the auth object has a direct job token (jt-...) it uses it; otherwise if it has a PAT (pt-...)
// it exchanges it via SingleFlight to prevent concurrent redundant calls.
func EnsureValidJobToken(ctx context.Context, httpClient *http.Client, auth *cliproxyauth.Auth) (string, error) {
	if auth == nil {
		return "", errors.New("qoder: auth is nil")
	}

	// 1. Check if auth directly provides an active job token
	token := AccessToken(auth)
	if strings.HasPrefix(token, "jt-") {
		return token, nil
	}

	// 2. Check if a PAT exists to exchange
	pat := PersonalAccessToken(auth)
	if pat == "" && strings.HasPrefix(token, "pt-") {
		pat = token
	}

	realm := ResolveRealm(auth)
	if pat != "" {
		key := fmt.Sprintf("%s:%s", realm, pat)
		res, err, _ := exchangeGroup.Do(key, func() (any, error) {
			resp, errExchange := ExchangeJobToken(ctx, httpClient, realm, pat)
			if errExchange != nil {
				return "", errExchange
			}
			// Update auth metadata in-place
			if auth.Metadata == nil {
				auth.Metadata = make(map[string]any)
			}
			auth.Metadata["job_token"] = resp.JobToken
			if resp.RefreshToken != "" {
				auth.Metadata["job_refresh"] = resp.RefreshToken
			}
			return resp.JobToken, nil
		})
		if err != nil {
			return "", err
		}
		if str, ok := res.(string); ok && str != "" {
			return str, nil
		}
	}

	// If token was already a non-prefixed bearer token (from OAuth device flow), return it
	if token != "" {
		return token, nil
	}

	return "", errors.New("qoder: no usable token or credentials in auth")
}
