package qoder

import (
	"testing"

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
}
