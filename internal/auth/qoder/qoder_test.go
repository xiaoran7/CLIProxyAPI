package qoder

import (
	"testing"
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestResolveRealm(t *testing.T) {
	tests := []struct {
		name     string
		auth     *cliproxyauth.Auth
		expected string
	}{
		{
			name:     "nil auth defaults to intl",
			auth:     nil,
			expected: RealmIntl,
		},
		{
			name: "provider qoder-cn resolves to cn",
			auth: &cliproxyauth.Auth{
				Provider: "qoder-cn",
			},
			expected: RealmCN,
		},
		{
			name: "provider qoder_cn resolves to cn",
			auth: &cliproxyauth.Auth{
				Provider: "qoder_cn",
			},
			expected: RealmCN,
		},
		{
			name: "provider qoder resolves to intl",
			auth: &cliproxyauth.Auth{
				Provider: "qoder",
			},
			expected: RealmIntl,
		},
		{
			name: "attribute realm cn resolves to cn",
			auth: &cliproxyauth.Auth{
				Provider:   "qoder",
				Attributes: map[string]string{"realm": "cn"},
			},
			expected: RealmCN,
		},
		{
			name: "metadata realm cn resolves to cn",
			auth: &cliproxyauth.Auth{
				Provider: "qoder",
				Metadata: map[string]any{"realm": "cn"},
			},
			expected: RealmCN,
		},
		{
			name: "base_url containing qoder.com.cn resolves to cn",
			auth: &cliproxyauth.Auth{
				Provider:   "qoder",
				Attributes: map[string]string{"base_url": "https://gateway.qoder.com.cn"},
			},
			expected: RealmCN,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveRealm(tt.auth)
			if got != tt.expected {
				t.Errorf("ResolveRealm() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestResolveBaseURL(t *testing.T) {
	if got := ResolveBaseURL(RealmIntl); got != BaseURLIntl {
		t.Errorf("ResolveBaseURL(intl) = %s, expected %s", got, BaseURLIntl)
	}
	if got := ResolveBaseURL(RealmCN); got != BaseURLCN {
		t.Errorf("ResolveBaseURL(cn) = %s, expected %s", got, BaseURLCN)
	}
}

func TestResolveOpenAPIBaseURL(t *testing.T) {
	if got := ResolveOpenAPIBaseURL(RealmIntl); got != OpenAPIBaseURLIntl {
		t.Errorf("ResolveOpenAPIBaseURL(intl) = %s, expected %s", got, OpenAPIBaseURLIntl)
	}
	if got := ResolveOpenAPIBaseURL(RealmCN); got != OpenAPIBaseURLCN {
		t.Errorf("ResolveOpenAPIBaseURL(cn) = %s, expected %s", got, OpenAPIBaseURLCN)
	}
}

func TestTokenExtraction(t *testing.T) {
	auth := &cliproxyauth.Auth{
		Attributes: map[string]string{
			"personal_token": "pt-test-123",
			"device_token":   "dt-test-789",
			"device_refresh": "dr-test-111",
			"job_refresh":    "jr-test-222",
		},
		Metadata: map[string]any{
			"job_token": "jt-test-456",
		},
	}

	if pat := PersonalAccessToken(auth); pat != "pt-test-123" {
		t.Errorf("PersonalAccessToken() = %s, expected pt-test-123", pat)
	}
	if token := AccessToken(auth); token != "jt-test-456" {
		t.Errorf("AccessToken() = %s, expected jt-test-456", token)
	}
	if dt := DeviceToken(auth); dt != "dt-test-789" {
		t.Errorf("DeviceToken() = %s, expected dt-test-789", dt)
	}
	if drt := DeviceRefreshToken(auth); drt != "dr-test-111" {
		t.Errorf("DeviceRefreshToken() = %s, expected dr-test-111", drt)
	}
	if jrt := JobRefreshToken(auth); jrt != "jr-test-222" {
		t.Errorf("JobRefreshToken() = %s, expected jr-test-222", jrt)
	}
}

func TestIsTokenExpired(t *testing.T) {
	// 1. Nil auth
	if IsTokenExpired(nil) {
		t.Errorf("IsTokenExpired(nil) should be false")
	}

	// 2. Future expiration (RFC3339 string)
	authFuture := &cliproxyauth.Auth{
		Metadata: map[string]any{
			"expired": time.Now().Add(10 * time.Minute).Format(time.RFC3339),
		},
	}
	if IsTokenExpired(authFuture) {
		t.Errorf("IsTokenExpired(future) should be false")
	}

	// 3. Past expiration (RFC3339 string)
	authPast := &cliproxyauth.Auth{
		Metadata: map[string]any{
			"expired": time.Now().Add(-1 * time.Minute).Format(time.RFC3339),
		},
	}
	if !IsTokenExpired(authPast) {
		t.Errorf("IsTokenExpired(past) should be true")
	}

	// 4. Within 2-min threshold
	authClose := &cliproxyauth.Auth{
		Metadata: map[string]any{
			"expired": time.Now().Add(1 * time.Minute).Format(time.RFC3339),
		},
	}
	if !IsTokenExpired(authClose) {
		t.Errorf("IsTokenExpired(close) should be true (needs refresh buffer)")
	}

	// 5. Unix timestamp expires_at
	authUnixPast := &cliproxyauth.Auth{
		Metadata: map[string]any{
			"expires_at": time.Now().Add(-10 * time.Second).Unix(),
		},
	}
	if !IsTokenExpired(authUnixPast) {
		t.Errorf("IsTokenExpired(unix past) should be true")
	}
}
