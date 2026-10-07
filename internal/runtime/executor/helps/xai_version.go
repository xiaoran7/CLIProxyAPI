package helps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

const (
	// DefaultXAIFallbackClientVersion is the fallback Grok CLI version when npm registry resolution fails.
	DefaultXAIFallbackClientVersion = "1.0.50"
	// XAIVersionRefreshInterval is the periodic interval to check for Grok CLI updates from npm.
	XAIVersionRefreshInterval = 3 * time.Hour
	// XAIVersionFetchTimeout is the maximum duration for a single npm registry lookup.
	XAIVersionFetchTimeout = 10 * time.Second
)

var (
	xaiNPMRegistryURL = "https://registry.npmjs.org/@xai-official/grok/latest"
)

var (
	cachedXAIClientVersion = DefaultXAIFallbackClientVersion
	xaiClientVersionMu     sync.RWMutex
	xaiVersionUpdaterOnce  sync.Once
)

// GetXAIClientVersion returns the current Grok CLI client version.
// If the background updater has fetched a newer version from npm, it returns that version;
// otherwise it returns DefaultXAIFallbackClientVersion.
func GetXAIClientVersion() string {
	xaiClientVersionMu.RLock()
	defer xaiClientVersionMu.RUnlock()
	return cachedXAIClientVersion
}

// StartXAIVersionUpdater starts a background goroutine that periodically refreshes the Grok CLI version from npm.
// It executes a single fetch on startup and then polls every 3 hours without multiple retries on failure.
func StartXAIVersionUpdater(ctx context.Context) {
	xaiVersionUpdaterOnce.Do(func() {
		go runXAIVersionUpdater(ctx)
	})
}

func runXAIVersionUpdater(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Fetch once on startup
	refreshXAIClientVersion(ctx)

	ticker := time.NewTicker(XAIVersionRefreshInterval)
	defer ticker.Stop()

	log.Infof("periodic Grok CLI version refresh started (interval=%s)", XAIVersionRefreshInterval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshXAIClientVersion(ctx)
		}
	}
}

func refreshXAIClientVersion(ctx context.Context) {
	version, errFetch := FetchXAINPMLatestVersion(ctx, nil)
	if errFetch != nil {
		log.WithError(errFetch).Warn("failed to fetch latest Grok CLI version from npm, keeping fallback/cached version")
		return
	}

	if version == "" {
		log.Warn("fetched empty Grok CLI version from npm, keeping fallback/cached version")
		return
	}

	xaiClientVersionMu.Lock()
	cachedXAIClientVersion = version
	xaiClientVersionMu.Unlock()

	log.WithField("version", version).Info("updated Grok CLI client version from npm")
}

// FetchXAINPMLatestVersion performs a single request to the npm registry to query the latest version of @xai-official/grok.
func FetchXAINPMLatestVersion(ctx context.Context, client *http.Client) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	fetchCtx, cancel := context.WithTimeout(ctx, XAIVersionFetchTimeout)
	defer cancel()

	xaiClientVersionMu.RLock()
	registryURL := xaiNPMRegistryURL
	xaiClientVersionMu.RUnlock()

	req, errReq := http.NewRequestWithContext(fetchCtx, http.MethodGet, registryURL, nil)
	if errReq != nil {
		return "", fmt.Errorf("create npm registry request: %w", errReq)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CLIProxyAPI")

	if client == nil {
		client = &http.Client{Timeout: XAIVersionFetchTimeout}
	}

	resp, errDo := client.Do(req)
	if errDo != nil {
		return "", fmt.Errorf("npm registry request failed: %w", errDo)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Errorf("failed to close npm response body: %v", errClose)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm registry returned HTTP %d", resp.StatusCode)
	}

	body, errRead := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if errRead != nil {
		return "", fmt.Errorf("read npm registry response: %w", errRead)
	}

	version := strings.TrimSpace(gjson.GetBytes(body, "version").String())
	if version == "" {
		return "", errors.New("version not found in npm response")
	}

	return version, nil
}

// OverrideXAINPMRegistryURLForTest overrides the npm registry URL for testing purposes.
func OverrideXAINPMRegistryURLForTest(t *testing.T, url string) func() {
	xaiClientVersionMu.Lock()
	oldURL := xaiNPMRegistryURL
	xaiNPMRegistryURL = url
	xaiClientVersionMu.Unlock()
	return func() {
		xaiClientVersionMu.Lock()
		xaiNPMRegistryURL = oldURL
		xaiClientVersionMu.Unlock()
	}
}

// SetXAIClientVersionForTest sets the cached Grok CLI version directly for testing purposes.
func SetXAIClientVersionForTest(t *testing.T, version string) func() {
	xaiClientVersionMu.Lock()
	old := cachedXAIClientVersion
	cachedXAIClientVersion = version
	xaiClientVersionMu.Unlock()
	return func() {
		xaiClientVersionMu.Lock()
		cachedXAIClientVersion = old
		xaiClientVersionMu.Unlock()
	}
}

// ResetXAIVersionUpdaterOnceForTest resets the sync.Once for testing updater initialization.
func ResetXAIVersionUpdaterOnceForTest(t *testing.T) {
	xaiClientVersionMu.Lock()
	xaiVersionUpdaterOnce = sync.Once{}
	xaiClientVersionMu.Unlock()
}
