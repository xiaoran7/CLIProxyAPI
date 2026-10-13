package workbuddy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestWorkBuddyCheckin_IntlSkipped(t *testing.T) {
	auth := &cliproxyauth.Auth{
		ID:       "wb-intl",
		Provider: "workbuddy",
		Attributes: map[string]string{
			"realm": RealmIntl,
		},
	}
	res, err := Checkin(context.Background(), http.DefaultClient, auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Outcome != CheckinInactive {
		t.Fatalf("expected outcome %q, got %q", CheckinInactive, res.Outcome)
	}
}

func TestWorkBuddyCheckin_ClaimSuccess(t *testing.T) {
	statusCalled := false
	claimCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/billing/meter/checkin-activity-status":
			statusCalled = true
			resp := map[string]any{
				"code": 0,
				"msg":  "ok",
				"data": map[string]any{
					"active":           true,
					"today_checked_in": false,
					"streak_days":      5,
					"daily_credit":     10.0,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "/v2/billing/meter/daily-checkin":
			claimCalled = true
			resp := map[string]any{
				"code": 0,
				"msg":  "claim success",
				"data": map[string]any{
					"credit":      10.0,
					"streak_days": 6,
					"message":     "恭喜获得10算力",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	auth := &cliproxyauth.Auth{
		ID:       "wb-cn-test",
		Provider: "workbuddy-cn",
		Attributes: map[string]string{
			"realm":    RealmCN,
			"base_url": server.URL,
		},
		Metadata: map[string]any{
			"access_token": "valid-token-123",
			"nickname":     "test_user",
		},
	}

	res, err := Checkin(context.Background(), server.Client(), auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !statusCalled || !claimCalled {
		t.Fatalf("expected both status and claim called, status=%v claim=%v", statusCalled, claimCalled)
	}
	if res.Outcome != CheckinClaimed {
		t.Fatalf("expected outcome %q, got %q", CheckinClaimed, res.Outcome)
	}
	if res.Credit != 10.0 || res.Streak != 6 {
		t.Fatalf("expected 10.0 credit and 6 streak, got credit=%f streak=%d", res.Credit, res.Streak)
	}
}

func TestWorkBuddyCheckin_AlreadyDone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/billing/meter/checkin-activity-status" {
			resp := map[string]any{
				"code": 0,
				"msg":  "ok",
				"data": map[string]any{
					"active":           true,
					"today_checked_in": true,
					"streak_days":      7,
					"today_credit":     10.0,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	auth := &cliproxyauth.Auth{
		ID:       "wb-cn-already",
		Provider: "workbuddy-cn",
		Attributes: map[string]string{
			"realm":    RealmCN,
			"base_url": server.URL,
		},
		Metadata: map[string]any{
			"access_token": "valid-token-123",
		},
	}

	res, err := Checkin(context.Background(), server.Client(), auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Outcome != CheckinDone {
		t.Fatalf("expected outcome %q, got %q", CheckinDone, res.Outcome)
	}
	if res.Streak != 7 || res.Credit != 10.0 {
		t.Fatalf("expected 7 streak and 10.0 credit, got streak=%d credit=%f", res.Streak, res.Credit)
	}
}

func TestCheckinEngine_PersistenceAndDeduplication(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "wb-checkin-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Path == "/v2/billing/meter/checkin-activity-status" {
			resp := map[string]any{
				"code": 0,
				"msg":  "ok",
				"data": map[string]any{
					"active":           true,
					"today_checked_in": true,
					"streak_days":      3,
					"today_credit":     5.0,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	testAuth := &cliproxyauth.Auth{
		ID:       "wb-test-persist",
		Provider: "workbuddy-cn",
		Attributes: map[string]string{
			"realm":    RealmCN,
			"base_url": server.URL,
		},
		Metadata: map[string]any{
			"access_token": "token-xyz",
		},
	}

	engine := NewCheckinEngine(tmpDir, func() []*cliproxyauth.Auth {
		return []*cliproxyauth.Auth{testAuth}
	})
	engine.client = server.Client()

	// First run
	results := engine.RunCheckinAll(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call, got %d", callCount)
	}

	// Verify file was written
	storeFile := filepath.Join(tmpDir, "workbuddy_checkins.json")
	if _, errStat := os.Stat(storeFile); errStat != nil {
		t.Fatalf("expected store file to exist: %v", errStat)
	}

	// Second run should deduplicate without calling upstream
	results2 := engine.RunCheckinAll(context.Background())
	if len(results2) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results2))
	}
	if callCount != 1 {
		t.Fatalf("expected call count to remain 1 due to dedup, got %d", callCount)
	}
}

func TestWorkBuddyCheckin_InactiveCampaign(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/billing/meter/checkin-activity-status" {
			resp := map[string]any{
				"code": 0,
				"msg":  "ok",
				"data": map[string]any{
					"active": false,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	auth := &cliproxyauth.Auth{
		ID:       "wb-cn-inactive",
		Provider: "workbuddy-cn",
		Attributes: map[string]string{
			"realm":    RealmCN,
			"base_url": server.URL,
		},
		Metadata: map[string]any{
			"access_token": "valid-token-123",
		},
	}

	res, err := Checkin(context.Background(), server.Client(), auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Outcome != CheckinInactive {
		t.Fatalf("expected outcome %q, got %q", CheckinInactive, res.Outcome)
	}
}
