package helps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetXAIClientVersionDefault(t *testing.T) {
	restore := SetXAIClientVersionForTest(t, DefaultXAIFallbackClientVersion)
	defer restore()

	if got := GetXAIClientVersion(); got != DefaultXAIFallbackClientVersion {
		t.Fatalf("GetXAIClientVersion() = %q, want %q", got, DefaultXAIFallbackClientVersion)
	}
}

func TestFetchXAINPMLatestVersionSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept header = %q, want application/json", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"@xai-official/grok","version":"1.0.52"}`))
	}))
	defer server.Close()

	restore := OverrideXAINPMRegistryURLForTest(t, server.URL)
	defer restore()

	version, err := FetchXAINPMLatestVersion(context.Background(), server.Client())
	if err != nil {
		t.Fatalf("FetchXAINPMLatestVersion() error = %v", err)
	}
	if version != "1.0.52" {
		t.Fatalf("FetchXAINPMLatestVersion() = %q, want 1.0.52", version)
	}
}

func TestFetchXAINPMLatestVersionHttpError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	restore := OverrideXAINPMRegistryURLForTest(t, server.URL)
	defer restore()

	_, err := FetchXAINPMLatestVersion(context.Background(), server.Client())
	if err == nil {
		t.Fatal("FetchXAINPMLatestVersion() expected error for HTTP 500, got nil")
	}
}

func TestFetchXAINPMLatestVersionMissingVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"@xai-official/grok"}`))
	}))
	defer server.Close()

	restore := OverrideXAINPMRegistryURLForTest(t, server.URL)
	defer restore()

	_, err := FetchXAINPMLatestVersion(context.Background(), server.Client())
	if err == nil {
		t.Fatal("FetchXAINPMLatestVersion() expected error for missing version, got nil")
	}
}

func TestRefreshXAIClientVersionKeepsFallbackOnError(t *testing.T) {
	restoreVersion := SetXAIClientVersionForTest(t, "1.0.50")
	defer restoreVersion()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "offline", http.StatusBadGateway)
	}))
	defer server.Close()

	restoreURL := OverrideXAINPMRegistryURLForTest(t, server.URL)
	defer restoreURL()

	refreshXAIClientVersion(context.Background())

	if got := GetXAIClientVersion(); got != "1.0.50" {
		t.Fatalf("GetXAIClientVersion() = %q, want fallback 1.0.50 preserved on error", got)
	}
}

func TestRefreshXAIClientVersionUpdatesOnSuccess(t *testing.T) {
	restoreVersion := SetXAIClientVersionForTest(t, "1.0.50")
	defer restoreVersion()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"@xai-official/grok","version":"1.0.55"}`))
	}))
	defer server.Close()

	restoreURL := OverrideXAINPMRegistryURLForTest(t, server.URL)
	defer restoreURL()

	refreshXAIClientVersion(context.Background())

	if got := GetXAIClientVersion(); got != "1.0.55" {
		t.Fatalf("GetXAIClientVersion() = %q, want updated version 1.0.55", got)
	}
}

func TestStartXAIVersionUpdaterLifecycle(t *testing.T) {
	ResetXAIVersionUpdaterOnceForTest(t)
	restoreVersion := SetXAIClientVersionForTest(t, "1.0.50")
	defer restoreVersion()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"@xai-official/grok","version":"1.0.60"}`))
	}))
	defer server.Close()

	restoreURL := OverrideXAINPMRegistryURLForTest(t, server.URL)
	defer restoreURL()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartXAIVersionUpdater(ctx)

	// Wait briefly for the startup fetch to settle
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if GetXAIClientVersion() == "1.0.60" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if got := GetXAIClientVersion(); got != "1.0.60" {
		t.Fatalf("GetXAIClientVersion() = %q, want 1.0.60 after StartXAIVersionUpdater", got)
	}
}
