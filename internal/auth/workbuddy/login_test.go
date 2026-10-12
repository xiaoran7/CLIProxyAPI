package workbuddy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseJWTClaims(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadData, _ := json.Marshal(map[string]any{
		"uid": "u-test-999",
		"exp": 1893456000,
	})
	payload := base64.RawURLEncoding.EncodeToString(payloadData)
	token := header + "." + payload + ".signature"

	uid, exp := ParseJWTClaims(token)
	if uid != "u-test-999" {
		t.Errorf("ParseJWTClaims uid = %q, want u-test-999", uid)
	}
	if exp != 1893456000 {
		t.Errorf("ParseJWTClaims exp = %d, want 1893456000", exp)
	}
}

func TestStartLoginAndPoll(t *testing.T) {
	var pollCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case AuthStatePath:
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			_, _ = w.Write([]byte(`{
				"code": 0,
				"msg": "ok",
				"data": {
					"state": "mock-state-123",
					"authUrl": "https://www.workbuddy.ai/auth/test"
				}
			}`))
		case AuthTokenPath:
			pollCount++
			if pollCount == 1 {
				// first poll: pending
				_, _ = w.Write([]byte(`{
					"code": 11217,
					"msg": "waiting for user login"
				}`))
			} else {
				// second poll: completed
				jwtPayload, _ := json.Marshal(map[string]any{
					"uid": "u-real-888",
					"exp": time.Now().Add(24 * time.Hour).Unix(),
				})
				tok := "header." + base64.RawURLEncoding.EncodeToString(jwtPayload) + ".sig"
				respJSON, _ := json.Marshal(map[string]any{
					"code": 0,
					"data": map[string]any{
						"accessToken":  tok,
						"refreshToken": "ref-real-888",
						"domain":       "www.workbuddy.ai",
						"expiresAt":    1893456000,
					},
				})
				_, _ = w.Write(respJSON)
			}
		case LoginAccountPath:
			_, _ = w.Write([]byte(`{
				"code": 0,
				"data": {
					"nickname": "TestPilot"
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Test StartLogin with custom client redirecting to mock server
	client := server.Client()
	client.Transport = customTransportRoundTripper{
		server: server,
		base:   http.DefaultTransport,
	}

	session, err := StartLogin(context.Background(), client, RealmIntl, "CLI")
	if err != nil {
		t.Fatalf("StartLogin failed: %v", err)
	}
	if session.State != "mock-state-123" {
		t.Errorf("session.State = %q, want mock-state-123", session.State)
	}

	// First poll should be pending
	bundle, isPending, err := PollToken(context.Background(), client, RealmIntl, session.State)
	if err != nil {
		t.Fatalf("PollToken first try failed: %v", err)
	}
	if !isPending {
		t.Error("expected first poll to be pending")
	}
	if bundle != nil {
		t.Error("expected nil bundle on pending poll")
	}

	// Second poll should succeed
	bundle, isPending, err = PollToken(context.Background(), client, RealmIntl, session.State)
	if err != nil {
		t.Fatalf("PollToken second try failed: %v", err)
	}
	if isPending {
		t.Error("expected second poll not to be pending")
	}
	if bundle == nil {
		t.Fatal("expected non-nil bundle on second poll")
	}
	if bundle.UserID != "u-real-888" {
		t.Errorf("bundle.UserID = %q, want u-real-888", bundle.UserID)
	}
	if bundle.Nickname != "TestPilot" {
		t.Errorf("bundle.Nickname = %q, want TestPilot", bundle.Nickname)
	}
	if bundle.RefreshToken != "ref-real-888" {
		t.Errorf("bundle.RefreshToken = %q, want ref-real-888", bundle.RefreshToken)
	}
}

type customTransportRoundTripper struct {
	server *httptest.Server
	base   http.RoundTripper
}

func (c customTransportRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = c.server.Listener.Addr().String()
	return c.base.RoundTrip(req)
}
