package qoder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestTodayBeijing(t *testing.T) {
	today := TodayBeijing()
	if len(today) != 10 || today[4] != '-' || today[7] != '-' {
		t.Fatalf("unexpected TodayBeijing format: %s", today)
	}
}

func TestQoderCheckin_ClaimSuccess(t *testing.T) {
	mux := http.NewServeMux()

	// Campaigns list
	mux.HandleFunc(CampaignsPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-qoder-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Cosy-Clienttype") != CosyClientTypeDesktop {
			t.Errorf("expected Cosy-Clienttype %s, got %s", CosyClientTypeDesktop, r.Header.Get("Cosy-Clienttype"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"msg":  "ok",
			"data": map[string]any{
				"campaigns": []map[string]any{
					{
						"id":      "camp-daily-100",
						"name":    "Daily 100 Credits",
						"status":  "available",
						"claimed": false,
					},
				},
			},
		})
	})

	// Campaign claim
	claimPath := fmt.Sprintf(CampaignClaimPath, "camp-daily-100")
	mux.HandleFunc(claimPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"msg":  "claim success",
			"data": map[string]any{
				"points": 100,
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	auth := &cliproxyauth.Auth{
		ID:       "qoder-test-1",
		Provider: "qoder",
		Metadata: map[string]any{
			"access_token": "test-qoder-token",
			"openapi_base": server.URL,
			"machine_id":   "mock-machine-id",
		},
	}

	res, err := Checkin(context.Background(), server.Client(), auth)
	if err != nil {
		t.Fatalf("unexpected checkin error: %v", err)
	}
	if res.Status != CheckinClaimed || res.Outcome != CheckinClaimed {
		t.Fatalf("expected outcome %q, got %q", CheckinClaimed, res.Outcome)
	}
	if res.Points != 100 || res.Credit != 100 {
		t.Fatalf("expected 100 points, got %d", res.Points)
	}
}

func TestQoderCheckin_AlreadyDone(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc(CampaignsPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"msg":  "ok",
			"data": map[string]any{
				"campaigns": []map[string]any{
					{
						"id":      "camp-daily-100",
						"name":    "Daily 100 Credits",
						"status":  "claimed",
						"claimed": true,
					},
				},
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	auth := &cliproxyauth.Auth{
		ID:       "qoder-test-2",
		Provider: "qoder-cn",
		Metadata: map[string]any{
			"access_token": "test-qoder-token",
			"openapi_base": server.URL,
		},
	}

	res, err := Checkin(context.Background(), server.Client(), auth)
	if err != nil {
		t.Fatalf("unexpected checkin error: %v", err)
	}
	if res.Outcome != CheckinDone {
		t.Fatalf("expected outcome %q, got %q", CheckinDone, res.Outcome)
	}
}

func TestQoderCheckin_DirectFallback(t *testing.T) {
	mux := http.NewServeMux()

	// Campaigns endpoint returns 404
	mux.HandleFunc(CampaignsPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Direct check-in claim endpoint succeeds
	mux.HandleFunc(DailyCheckinClaimPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 200,
			"msg":  "每日签到成功",
			"data": map[string]any{
				"credits": 100,
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	auth := &cliproxyauth.Auth{
		ID:       "qoder-test-3",
		Provider: "qoder",
		Metadata: map[string]any{
			"access_token": "test-qoder-token",
			"openapi_base": server.URL,
		},
	}

	res, err := Checkin(context.Background(), server.Client(), auth)
	if err != nil {
		t.Fatalf("unexpected checkin error: %v", err)
	}
	if res.Outcome != CheckinClaimed {
		t.Fatalf("expected outcome %q, got %q", CheckinClaimed, res.Outcome)
	}
	if res.Points != 100 {
		t.Fatalf("expected 100 points, got %d", res.Points)
	}
}

func TestQoderCheckin_Unauthorized(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(CampaignsPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	mux.HandleFunc(DailyCheckinClaimPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	mux.HandleFunc(DailyCheckinAltPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	auth := &cliproxyauth.Auth{
		ID:       "qoder-test-4",
		Provider: "qoder",
		Metadata: map[string]any{
			"access_token": "expired-token",
			"openapi_base": server.URL,
		},
	}

	res, err := Checkin(context.Background(), server.Client(), auth)
	if err == nil {
		t.Fatal("expected error on unauthorized checkin, got nil")
	}
	if res.Outcome != CheckinFailed {
		t.Fatalf("expected outcome %q, got %q", CheckinFailed, res.Outcome)
	}
}

func TestCheckinEngine_PersistenceAndDeduplication(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "qoder_checkin_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	callCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc(CampaignsPath, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"msg":  "ok",
			"data": map[string]any{
				"campaigns": []map[string]any{
					{
						"id":      "camp-1",
						"claimed": true,
						"status":  "claimed",
					},
				},
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	authList := []*cliproxyauth.Auth{
		{
			ID:       "acc1",
			Provider: "qoder",
			Metadata: map[string]any{
				"access_token": "tok1",
				"openapi_base": server.URL,
			},
		},
	}

	engine := NewCheckinEngine(tmpDir, func() []*cliproxyauth.Auth {
		return authList
	})

	// First execution should call server
	results := engine.RunCheckinAll(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if callCount != 1 {
		t.Fatalf("expected 1 server call, got %d", callCount)
	}

	// Verify persistence to disk
	storePath := filepath.Join(tmpDir, "qoder_checkins.json")
	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		t.Fatalf("expected %s to exist", storePath)
	}

	// Second execution on same day should deduplicate and skip server call
	results2 := engine.RunCheckinAll(context.Background())
	if len(results2) != 1 {
		t.Fatalf("expected 1 result from cache, got %d", len(results2))
	}
	if callCount != 1 {
		t.Fatalf("expected call count to remain 1 due to deduplication, got %d", callCount)
	}

	// Recreate engine from disk and verify status is restored
	engineReloaded := NewCheckinEngine(tmpDir, nil)
	status, found := engineReloaded.GetStatus("acc1")
	if !found {
		t.Fatal("expected status for acc1 to be found after reload")
	}
	if status.Outcome != CheckinDone {
		t.Fatalf("expected outcome %q, got %q", CheckinDone, status.Outcome)
	}
}

func TestCheckinEngine_EnabledToggle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "qoder_toggle_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	engine := NewCheckinEngine(tmpDir, nil)
	if !engine.IsEnabled() {
		t.Fatal("expected engine to be enabled by default")
	}

	engine.SetEnabled(false)
	if engine.IsEnabled() {
		t.Fatal("expected engine to be disabled after SetEnabled(false)")
	}

	// Verify persistence of enabled state across restarts
	engineReloaded := NewCheckinEngine(tmpDir, nil)
	if engineReloaded.IsEnabled() {
		t.Fatal("expected reloaded engine to retain disabled state")
	}

	engineReloaded.SetEnabled(true)
	if !engineReloaded.IsEnabled() {
		t.Fatal("expected reloaded engine to become enabled")
	}

	engineReloaded2 := NewCheckinEngine(tmpDir, nil)
	if !engineReloaded2.IsEnabled() {
		t.Fatal("expected reloaded engine 2 to retain enabled state")
	}
}
