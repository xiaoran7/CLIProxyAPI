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
	Type          string         `json:"type"`
	AccessToken   string         `json:"access_token"`
	RefreshToken  string         `json:"refresh_token,omitempty"`
	DeviceToken   string         `json:"device_token,omitempty"`
	DeviceRefresh string         `json:"device_refresh,omitempty"`
	JobToken      string         `json:"job_token,omitempty"`
	JobRefresh    string         `json:"job_refresh,omitempty"`
	Realm         string         `json:"realm"`
	BaseURL       string         `json:"base_url"`
	OpenAPIBase   string         `json:"openapi_base"`
	UserID        string         `json:"user_id,omitempty"`
	Nickname      string         `json:"nickname,omitempty"`
	Email         string         `json:"email,omitempty"`
	MachineID     string         `json:"machine_id,omitempty"`
	DeviceChat    bool           `json:"device_chat,omitempty"`
	Expired       string         `json:"expired,omitempty"`
	Metadata      map[string]any `json:"-"`
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
	return ExchangeJobTokenWithBase(ctx, httpClient, ResolveOpenAPIBaseURL(realm), pat)
}

// ExchangeJobTokenWithBase exchanges a Personal Access Token (PAT) for a Job Token using an explicit openapi base URL.
func ExchangeJobTokenWithBase(ctx context.Context, httpClient *http.Client, openAPIBase, pat string) (*JobTokenResponse, error) {
	if strings.TrimSpace(pat) == "" {
		return nil, errors.New("qoder: personal token is empty")
	}
	if openAPIBase == "" {
		openAPIBase = OpenAPIBaseURLIntl
	}
	openAPIBase = strings.TrimRight(openAPIBase, "/")
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
	return RefreshJobTokenWithBase(ctx, httpClient, ResolveOpenAPIBaseURL(realm), jrt)
}

// RefreshJobTokenWithBase refreshes an existing Job Token using an explicit openapi base URL.
func RefreshJobTokenWithBase(ctx context.Context, httpClient *http.Client, openAPIBase, jrt string) (*JobTokenResponse, error) {
	if strings.TrimSpace(jrt) == "" {
		return nil, errors.New("qoder: job refresh token is empty")
	}
	if openAPIBase == "" {
		openAPIBase = OpenAPIBaseURLIntl
	}
	openAPIBase = strings.TrimRight(openAPIBase, "/")
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

// DeviceTokenResponse holds the response from the deviceToken refresh endpoint.
type DeviceTokenResponse struct {
	Token        string `json:"token"`
	DeviceToken  string `json:"device_token,omitempty"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// RefreshDeviceToken refreshes an active device token using the device refresh token.
func RefreshDeviceToken(ctx context.Context, httpClient *http.Client, realm, drt string) (*DeviceTokenResponse, error) {
	return RefreshDeviceTokenWithBase(ctx, httpClient, ResolveOpenAPIBaseURL(realm), drt)
}

// RefreshDeviceTokenWithBase refreshes an active device token using an explicit openapi base URL.
func RefreshDeviceTokenWithBase(ctx context.Context, httpClient *http.Client, openAPIBase, drt string) (*DeviceTokenResponse, error) {
	if strings.TrimSpace(drt) == "" {
		return nil, errors.New("qoder: device refresh token is empty")
	}
	if openAPIBase == "" {
		openAPIBase = OpenAPIBaseURLIntl
	}
	openAPIBase = strings.TrimRight(openAPIBase, "/")
	reqURL := openAPIBase + "/api/v1/deviceToken/refresh"

	payloadBytes, errMarshal := json.Marshal(map[string]string{
		"refresh_token": drt,
	})
	if errMarshal != nil {
		return nil, fmt.Errorf("qoder: marshal device refresh payload: %w", errMarshal)
	}

	req, errReq := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payloadBytes))
	if errReq != nil {
		return nil, fmt.Errorf("qoder: build device refresh request: %w", errReq)
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
		return nil, fmt.Errorf("qoder: device refresh request failed: %w", errDo)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return nil, fmt.Errorf("qoder: read device refresh response: %w", errRead)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qoder: device refresh returned status %d: %s", resp.StatusCode, string(body))
	}

	parsed := gjson.ParseBytes(body)
	token := parsed.Get("token").String()
	if token == "" {
		token = parsed.Get("device_token").String()
	}
	if token == "" {
		token = parsed.Get("data.token").String()
	}
	if token == "" {
		token = parsed.Get("data.device_token").String()
	}
	if token == "" {
		return nil, fmt.Errorf("qoder: device refresh response missing token: %s", string(body))
	}

	refreshToken := parsed.Get("refresh_token").String()
	if refreshToken == "" {
		refreshToken = parsed.Get("data.refresh_token").String()
	}
	expiresIn := parsed.Get("expires_in").Int()
	if expiresIn <= 0 {
		expiresIn = parsed.Get("data.expires_in").Int()
	}

	return &DeviceTokenResponse{
		Token:        token,
		DeviceToken:  token,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
	}, nil
}

// JobRefreshToken returns the job refresh token string if present in auth metadata/attributes.
func JobRefreshToken(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	if auth.Metadata != nil {
		if t, ok := auth.Metadata["job_refresh"].(string); ok && strings.TrimSpace(t) != "" {
			return strings.TrimSpace(t)
		}
	}
	if auth.Attributes != nil {
		if t := strings.TrimSpace(auth.Attributes["job_refresh"]); t != "" {
			return t
		}
	}
	return ""
}

// DeviceRefreshToken returns the device refresh token string if present in auth metadata/attributes.
func DeviceRefreshToken(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	if auth.Metadata != nil {
		if t, ok := auth.Metadata["device_refresh"].(string); ok && strings.TrimSpace(t) != "" {
			return strings.TrimSpace(t)
		}
		if t, ok := auth.Metadata["refresh_token"].(string); ok && strings.TrimSpace(t) != "" {
			return strings.TrimSpace(t)
		}
	}
	if auth.Attributes != nil {
		if t := strings.TrimSpace(auth.Attributes["device_refresh"]); t != "" {
			return t
		}
		if t := strings.TrimSpace(auth.Attributes["refresh_token"]); t != "" {
			return t
		}
	}
	return ""
}

// ResolveRefreshToken returns the refresh token string (device or job refresh token) from auth.
func ResolveRefreshToken(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	if rt := DeviceRefreshToken(auth); rt != "" {
		return rt
	}
	if rt := JobRefreshToken(auth); rt != "" {
		return rt
	}
	return ""
}

// DeviceToken returns the raw device token (dt-...) if present in auth metadata/attributes.
func DeviceToken(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	if auth.Metadata != nil {
		if t, ok := auth.Metadata["device_token"].(string); ok && strings.TrimSpace(t) != "" {
			return strings.TrimSpace(t)
		}
	}
	if auth.Attributes != nil {
		if t := strings.TrimSpace(auth.Attributes["device_token"]); t != "" {
			return t
		}
	}
	return ""
}

// IsTokenExpired checks whether the token in the given auth object is expired or within 2 minutes of expiration.
func IsTokenExpired(auth *cliproxyauth.Auth) bool {
	if auth == nil {
		return false
	}
	if auth.Metadata != nil {
		if expStr, ok := auth.Metadata["expired"].(string); ok && strings.TrimSpace(expStr) != "" {
			if t, err := time.Parse(time.RFC3339, expStr); err == nil {
				return time.Now().Add(2 * time.Minute).After(t)
			}
		}
		if expAt, ok := auth.Metadata["expires_at"].(int64); ok && expAt > 0 {
			return time.Now().Add(2*time.Minute).Unix() >= expAt
		}
		if expAt, ok := auth.Metadata["expires_at"].(float64); ok && expAt > 0 {
			return time.Now().Add(2*time.Minute).Unix() >= int64(expAt)
		}
	}
	if auth.Attributes != nil {
		if expStr := strings.TrimSpace(auth.Attributes["expired"]); expStr != "" {
			if t, err := time.Parse(time.RFC3339, expStr); err == nil {
				return time.Now().Add(2 * time.Minute).After(t)
			}
		}
	}
	return false
}

// EnsureValidJobToken acquires or ensures an active Job Token for the given auth object.
// If the auth object has a direct job token (jt-...) it checks validity and refreshes if needed;
// otherwise if it has a PAT (pt-...) it exchanges it via SingleFlight to prevent concurrent redundant calls.
func EnsureValidJobToken(ctx context.Context, httpClient *http.Client, auth *cliproxyauth.Auth) (string, error) {
	if auth == nil {
		return "", errors.New("qoder: auth is nil")
	}

	realm := ResolveRealm(auth)
	openAPIBase := ResolveOpenAPIBaseURLForAuth(realm, auth)
	token := AccessToken(auth)

	// 1. Check if auth directly provides an active job token
	if strings.HasPrefix(token, "jt-") {
		// If token is not expired, return directly
		if !IsTokenExpired(auth) {
			return token, nil
		}
		// If expired and job_refresh exists, attempt proactive refresh via singleflight
		if jrt := JobRefreshToken(auth); jrt != "" {
			key := fmt.Sprintf("job_refresh:%s:%s", realm, jrt)
			res, err, _ := exchangeGroup.Do(key, func() (any, error) {
				resp, errRefresh := RefreshJobTokenWithBase(ctx, httpClient, openAPIBase, jrt)
				if errRefresh != nil {
					return "", errRefresh
				}
				if auth.Metadata == nil {
					auth.Metadata = make(map[string]any)
				}
				auth.Metadata["job_token"] = resp.JobToken
				auth.Metadata["access_token"] = resp.JobToken
				if resp.RefreshToken != "" {
					auth.Metadata["job_refresh"] = resp.RefreshToken
				}
				if resp.ExpiresIn > 0 {
					var expSec int64
					if resp.ExpiresIn > 1000000 {
						expSec = resp.ExpiresIn / 1000
					} else {
						expSec = resp.ExpiresIn
					}
					expTime := time.Now().Add(time.Duration(expSec) * time.Second)
					auth.Metadata["expired"] = expTime.UTC().Format(time.RFC3339)
					auth.Metadata["expires_at"] = expTime.Unix()
				}
				return resp.JobToken, nil
			})
			if err == nil {
				if str, ok := res.(string); ok && str != "" {
					return str, nil
				}
			}
		}
	}

	// 2. Check if a device refresh token exists to refresh and acquire/trade for a job token
	if drt := DeviceRefreshToken(auth); drt != "" && (IsTokenExpired(auth) || !strings.HasPrefix(token, "jt-")) {
		key := fmt.Sprintf("device_refresh:%s:%s", realm, drt)
		res, err, _ := exchangeGroup.Do(key, func() (any, error) {
			dtResp, errDrt := RefreshDeviceTokenWithBase(ctx, httpClient, openAPIBase, drt)
			if errDrt != nil {
				return "", errDrt
			}
			if auth.Metadata == nil {
				auth.Metadata = make(map[string]any)
			}
			newDeviceToken := dtResp.Token
			if newDeviceToken == "" {
				newDeviceToken = dtResp.DeviceToken
			}
			auth.Metadata["device_token"] = newDeviceToken
			if dtResp.RefreshToken != "" {
				auth.Metadata["device_refresh"] = dtResp.RefreshToken
			}
			if ts, ok := auth.Storage.(*QoderTokenStorage); ok && ts != nil {
				ts.DeviceToken = newDeviceToken
				if dtResp.RefreshToken != "" {
					ts.DeviceRefresh = dtResp.RefreshToken
				}
			}

			// Trade newly refreshed device token for job token
			jtResp, errTrade := TradeJobTokenWithBase(ctx, httpClient, openAPIBase, realm, newDeviceToken)
			if errTrade == nil && jtResp != nil && strings.TrimSpace(jtResp.JobToken) != "" {
				auth.Metadata["job_token"] = jtResp.JobToken
				auth.Metadata["access_token"] = jtResp.JobToken
				if jtResp.RefreshToken != "" {
					auth.Metadata["job_refresh"] = jtResp.RefreshToken
				}
				if jtResp.ExpiresIn > 0 {
					var expSec int64
					if jtResp.ExpiresIn > 1000000 {
						expSec = jtResp.ExpiresIn / 1000
					} else {
						expSec = jtResp.ExpiresIn
					}
					expTime := time.Now().Add(time.Duration(expSec) * time.Second)
					auth.Metadata["expired"] = expTime.UTC().Format(time.RFC3339)
					auth.Metadata["expires_at"] = expTime.Unix()
				}
				if ts, ok := auth.Storage.(*QoderTokenStorage); ok && ts != nil {
					ts.JobToken = jtResp.JobToken
					ts.AccessToken = jtResp.JobToken
					if jtResp.RefreshToken != "" {
						ts.JobRefresh = jtResp.RefreshToken
					}
				}
				return jtResp.JobToken, nil
			}

			// If trade fails (e.g. CN realm where device token is directly used), use device token
			auth.Metadata["access_token"] = newDeviceToken
			if ts, ok := auth.Storage.(*QoderTokenStorage); ok && ts != nil {
				ts.AccessToken = newDeviceToken
			}
			return newDeviceToken, nil
		})
		if err == nil {
			if str, ok := res.(string); ok && str != "" {
				return str, nil
			}
		}
	}

	// 3. Check if an active device token exists directly to trade for a job token
	if dt := DeviceToken(auth); dt != "" && (IsTokenExpired(auth) || !strings.HasPrefix(token, "jt-")) {
		key := fmt.Sprintf("device_trade:%s:%s", realm, dt)
		res, err, _ := exchangeGroup.Do(key, func() (any, error) {
			jtResp, errTrade := TradeJobTokenWithBase(ctx, httpClient, openAPIBase, realm, dt)
			if errTrade == nil && jtResp != nil && strings.TrimSpace(jtResp.JobToken) != "" {
				if auth.Metadata == nil {
					auth.Metadata = make(map[string]any)
				}
				auth.Metadata["job_token"] = jtResp.JobToken
				auth.Metadata["access_token"] = jtResp.JobToken
				if jtResp.RefreshToken != "" {
					auth.Metadata["job_refresh"] = jtResp.RefreshToken
				}
				if jtResp.ExpiresIn > 0 {
					var expSec int64
					if jtResp.ExpiresIn > 1000000 {
						expSec = jtResp.ExpiresIn / 1000
					} else {
						expSec = jtResp.ExpiresIn
					}
					expTime := time.Now().Add(time.Duration(expSec) * time.Second)
					auth.Metadata["expired"] = expTime.UTC().Format(time.RFC3339)
					auth.Metadata["expires_at"] = expTime.Unix()
				}
				if ts, ok := auth.Storage.(*QoderTokenStorage); ok && ts != nil {
					ts.JobToken = jtResp.JobToken
					ts.AccessToken = jtResp.JobToken
					if jtResp.RefreshToken != "" {
						ts.JobRefresh = jtResp.RefreshToken
					}
				}
				return jtResp.JobToken, nil
			}
			return dt, nil
		})
		if err == nil {
			if str, ok := res.(string); ok && str != "" {
				return str, nil
			}
		}
	}

	// 4. Check if a PAT exists to exchange
	pat := PersonalAccessToken(auth)
	if pat == "" && strings.HasPrefix(token, "pt-") {
		pat = token
	}

	if pat != "" {
		key := fmt.Sprintf("pat:%s:%s", realm, pat)
		res, err, _ := exchangeGroup.Do(key, func() (any, error) {
			resp, errExchange := ExchangeJobTokenWithBase(ctx, httpClient, openAPIBase, pat)
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
