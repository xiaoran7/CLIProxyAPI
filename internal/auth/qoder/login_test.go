package qoder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestStartLogin(t *testing.T) {
	sessionIntl, errIntl := StartLogin(RealmIntl)
	if errIntl != nil {
		t.Fatalf("StartLogin(intl) failed: %v", errIntl)
	}
	if !strings.HasPrefix(sessionIntl.AuthURL, AuthWebURLIntl+"/device/selectAccounts?") {
		t.Errorf("StartLogin(intl) unexpected AuthURL: %s", sessionIntl.AuthURL)
	}
	if sessionIntl.Nonce == "" || sessionIntl.Verifier == "" || sessionIntl.MachineID == "" {
		t.Errorf("StartLogin(intl) missing session parameters: %+v", sessionIntl)
	}
	uIntl, errParse := url.Parse(sessionIntl.AuthURL)
	if errParse != nil {
		t.Fatalf("failed to parse auth URL: %v", errParse)
	}
	qIntl := uIntl.Query()
	if qIntl.Get("client_id") != DefaultClientIDIntl {
		t.Errorf("expected client_id %s, got %s", DefaultClientIDIntl, qIntl.Get("client_id"))
	}
	if qIntl.Get("challenge_method") != "S256" || qIntl.Get("challenge") == "" {
		t.Errorf("missing or invalid PKCE parameters in URL: %s", sessionIntl.AuthURL)
	}

	sessionCN, errCN := StartLogin(RealmCN)
	if errCN != nil {
		t.Fatalf("StartLogin(cn) failed: %v", errCN)
	}
	if !strings.HasPrefix(sessionCN.AuthURL, AuthWebURLCN+"/device/selectAccounts?") {
		t.Errorf("StartLogin(cn) unexpected AuthURL: %s", sessionCN.AuthURL)
	}
	uCN, _ := url.Parse(sessionCN.AuthURL)
	if uCN.Query().Get("client_id") != DefaultClientIDCN {
		t.Errorf("expected CN client_id %s, got %s", DefaultClientIDCN, uCN.Query().Get("client_id"))
	}
}

func TestPollTokenPendingAndSuccess(t *testing.T) {
	var pollHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case DeviceTokenPollPath:
			pollHits++
			if pollHits == 1 {
				// Simulating pending state: Qoder returns HTTP 404 with NotFound
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"errorCode":    "NotFound",
					"errorMessage": "Not found",
				})
				return
			}
			// Simulating user approved authorization: Qoder returns HTTP 200 with root-level token
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":         "dt-test-token-12345",
				"device_token":  "dt-test-token-12345",
				"refresh_token": "drt-test-refresh-67890",
				"user_id":       "uid-user-1",
				"user_name":     "Test Qoder User",
				"expires_in":    2592000,
			})
		case JobTokenPath:
			if r.Header.Get("Authorization") != "Bearer dt-test-token-12345" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":         "jt-job-token-abcdef",
				"refresh_token": "jrt-refresh-ghijkl",
				"expires_in":    3600000,
			})
		case UserInfoPath:
			if r.Header.Get("Authorization") != "Bearer dt-test-token-12345" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "uid-user-1",
				"name":     "Test Qoder User",
				"username": "qoder_dev",
				"email":    "dev@qoder.test",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Override OpenAPI base URL via env for test
	t.Setenv("QODER_INTL_OPENAPI", srv.URL)

	ctx := context.Background()
	client := srv.Client()

	// 1st Poll: Pending (HTTP 404)
	bundle, isPending, err := PollToken(ctx, client, RealmIntl, "test-nonce", "test-verifier", "test-machine")
	if err != nil {
		t.Fatalf("first poll returned unexpected error: %v", err)
	}
	if !isPending {
		t.Errorf("expected isPending=true on 404, got false")
	}
	if bundle != nil {
		t.Errorf("expected bundle=nil on pending, got %+v", bundle)
	}

	// 2nd Poll: Authorized (HTTP 200) -> trades for JobToken and enriches UserInfo
	bundle, isPending, err = PollToken(ctx, client, RealmIntl, "test-nonce", "test-verifier", "test-machine")
	if err != nil {
		t.Fatalf("second poll failed: %v", err)
	}
	if isPending {
		t.Errorf("expected isPending=false on success, got true")
	}
	if bundle == nil {
		t.Fatal("expected non-nil bundle on success")
	}
	if bundle.DeviceToken != "dt-test-token-12345" {
		t.Errorf("expected DeviceToken dt-test-token-12345, got %s", bundle.DeviceToken)
	}
	if bundle.JobToken != "jt-job-token-abcdef" {
		t.Errorf("expected JobToken jt-job-token-abcdef, got %s", bundle.JobToken)
	}
	if bundle.AccessToken != "jt-job-token-abcdef" {
		t.Errorf("expected AccessToken jt-job-token-abcdef, got %s", bundle.AccessToken)
	}
	if bundle.Email != "dev@qoder.test" {
		t.Errorf("expected Email dev@qoder.test, got %s", bundle.Email)
	}
	if bundle.Nickname != "Test Qoder User" {
		t.Errorf("expected Nickname 'Test Qoder User', got %s", bundle.Nickname)
	}
	if bundle.DeviceChat {
		t.Errorf("expected DeviceChat=false when JobToken trade succeeds, got true")
	}
}

func TestPollTokenDeviceChatFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case DeviceTokenPollPath:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":         "dt-cn-direct",
				"refresh_token": "drt-cn-refresh",
				"user_id":       "cn-user-88",
				"user_name":     "CN Developer",
				"expires_in":    2592000,
			})
		case JobTokenPath:
			// Simulating site refusing job token trade (as Qoder CN CLI does)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    400,
				"message": "not supported",
			})
		case UserInfoPath:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "cn-user-88",
				"name":  "CN Developer",
				"email": "dev@qoder.cn",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("QODER_CN_OPENAPI", srv.URL)

	ctx := context.Background()
	client := srv.Client()

	bundle, isPending, err := PollToken(ctx, client, RealmCN, "cn-nonce", "cn-verifier", "cn-machine")
	if err != nil {
		t.Fatalf("poll failed: %v", err)
	}
	if isPending {
		t.Errorf("expected isPending=false, got true")
	}
	if bundle == nil {
		t.Fatal("expected non-nil bundle")
	}
	// Fallback should use device token directly for active chat
	if bundle.AccessToken != "dt-cn-direct" {
		t.Errorf("expected AccessToken to fall back to dt-cn-direct, got %s", bundle.AccessToken)
	}
	if !bundle.DeviceChat {
		t.Errorf("expected DeviceChat=true on jobToken fallback, got false")
	}
	if bundle.Email != "dev@qoder.cn" {
		t.Errorf("expected Email dev@qoder.cn, got %s", bundle.Email)
	}
}
