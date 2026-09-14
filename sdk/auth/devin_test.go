package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	devinauth "github.com/router-for-me/CLIProxyAPI/v7/internal/auth/devin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func TestDevinAuthenticatorProviderAndRefreshLead(t *testing.T) {
	authenticator := NewDevinAuthenticator()
	if authenticator.Provider() != "devin" {
		t.Fatalf("Provider() = %q, want devin", authenticator.Provider())
	}
	lead := authenticator.RefreshLead()
	if lead != nil {
		t.Fatalf("RefreshLead() = %v, want nil for permanent tokens", lead)
	}
}

func TestDevinAuthenticatorHeadlessManualTokenLogin(t *testing.T) {
	authenticator := NewDevinAuthenticator()
	cfg := &config.Config{}

	mockPrompt := func(prompt string) (string, error) {
		return "devin-session-token$eyJmock.session.token", nil
	}

	opts := &LoginOptions{
		NoBrowser: true,
		Prompt:    mockPrompt,
	}

	auth, err := authenticator.Login(context.Background(), cfg, opts)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if auth.Provider != "devin" {
		t.Errorf("auth.Provider = %q, want devin", auth.Provider)
	}
	if auth.Attributes["api_key"] != "devin-session-token$eyJmock.session.token" {
		t.Errorf("api_key = %q, want expected", auth.Attributes["api_key"])
	}
}

func TestDevinAuthenticatorHeadlessManualCodeLogin(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/cli/token":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if body["code"] != "devin-cli-auth-code-123" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"invalid_code"}`))
				return
			}
			if body["code_verifier"] == "" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"missing_verifier"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"token":"eyJtest.manual.code.token"}`))

		case "/v3/self":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"user_name":"test-user","user_id":"uid-123","org_id":"org-456"}`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	authSvc := devinauth.NewDevinAuthService(mockServer.Client())
	authSvc.SetAPIBaseURL(mockServer.URL)

	authenticator := NewDevinAuthenticator()
	authenticator.AuthService = authSvc
	cfg := &config.Config{}

	mockPrompt := func(prompt string) (string, error) {
		return "devin-cli-auth-code-123", nil
	}

	opts := &LoginOptions{
		NoBrowser: true,
		Prompt:    mockPrompt,
	}

	auth, err := authenticator.Login(context.Background(), cfg, opts)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if auth.Provider != "devin" {
		t.Errorf("auth.Provider = %q, want devin", auth.Provider)
	}
	expectedToken := "devin-session-token$eyJtest.manual.code.token"
	if auth.Attributes["api_key"] != expectedToken {
		t.Errorf("api_key = %q, want %q", auth.Attributes["api_key"], expectedToken)
	}
	expectedLabel := "Devin (test-user)"
	if auth.Label != expectedLabel {
		t.Errorf("auth.Label = %q, want %q", auth.Label, expectedLabel)
	}
}

func TestDevinAuthenticatorHeadlessManualCallbackURLLogin(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/cli/token":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if body["code"] != "parsed-callback-code" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"invalid_code"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"token":"eyJtest.callback.token"}`))

		case "/v3/self":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"user_name":"url-user","user_id":"uid-url","org_id":"org-url"}`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	authSvc := devinauth.NewDevinAuthService(mockServer.Client())
	authSvc.SetAPIBaseURL(mockServer.URL)

	authenticator := NewDevinAuthenticator()
	authenticator.AuthService = authSvc
	cfg := &config.Config{}

	mockPrompt := func(prompt string) (string, error) {
		// User accidentally pastes the full callback URL from browser address bar
		return "http://127.0.0.1:12345/callback?code=parsed-callback-code", nil
	}

	opts := &LoginOptions{
		NoBrowser: true,
		Prompt:    mockPrompt,
	}

	auth, err := authenticator.Login(context.Background(), cfg, opts)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedToken := "devin-session-token$eyJtest.callback.token"
	if auth.Attributes["api_key"] != expectedToken {
		t.Errorf("api_key = %q, want %q", auth.Attributes["api_key"], expectedToken)
	}
	expectedLabel := "Devin (url-user)"
	if auth.Label != expectedLabel {
		t.Errorf("auth.Label = %q, want %q", auth.Label, expectedLabel)
	}
}

func TestDevinAuthenticatorHeadlessEmptyInputAborts(t *testing.T) {
	authenticator := NewDevinAuthenticator()
	cfg := &config.Config{}

	mockPrompt := func(prompt string) (string, error) {
		return "", nil
	}

	opts := &LoginOptions{
		NoBrowser: true,
		Prompt:    mockPrompt,
	}

	_, err := authenticator.Login(context.Background(), cfg, opts)
	if err == nil {
		t.Fatal("expected error on empty input, got nil")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Errorf("expected cancellation error message, got: %v", err)
	}
}
