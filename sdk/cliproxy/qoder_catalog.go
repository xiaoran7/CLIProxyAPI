package cliproxy

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/auth/qoder"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

const qoderCatalogFetchTimeout = 20 * time.Second

var (
	qoderCatalogClientFactory = func() *http.Client {
		return &http.Client{Timeout: qoderCatalogFetchTimeout}
	}
	qoderCatalogFetchMu sync.Mutex
)

// qoderAuthIsUsable reports whether an auth belongs to a Qoder provider.
func qoderAuthIsUsable(a *coreauth.Auth) bool {
	if a == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(a.Provider)) {
	case "qoder", "qoder-intl", "qoder-en", "qoder-cn":
		return true
	}
	if a.Attributes != nil && strings.EqualFold(strings.TrimSpace(a.Attributes["auth_kind"]), "qoder") {
		return true
	}
	return false
}

// warmQoderCatalogForAuth discovers dynamic models from upstream Qoder backends.
// Models are NEVER hardcoded: a cold cache fetches inline so the first listing carries
// live models, while subsequent refreshes execute in the background once stale.
func (s *Service) warmQoderCatalogForAuth(ctx context.Context, a *coreauth.Auth, realm string) {
	if a == nil || s == nil {
		return
	}
	if !qoderAuthIsUsable(a) {
		return
	}

	cleanRealm := strings.ToLower(strings.TrimSpace(realm))
	if cleanRealm == "" {
		cleanRealm = qoder.ResolveRealm(a)
	}

	if registry.IsQoderCatalogFresh(cleanRealm) {
		return
	}

	existing := registry.GetQoderDynamicModels(cleanRealm)
	isCold := len(existing) == 0

	fetchFunc := func() {
		qoderCatalogFetchMu.Lock()
		defer qoderCatalogFetchMu.Unlock()

		if registry.IsQoderCatalogFresh(cleanRealm) {
			return
		}

		fetchCtx, cancel := context.WithTimeout(context.Background(), qoderCatalogFetchTimeout)
		defer cancel()

		client := qoderCatalogClientFactory()
		body, err := qoder.FetchUpstreamModels(fetchCtx, client, a)
		if err != nil {
			log.Debugf("qoder catalog: fetch upstream models (%s) failed: %v", cleanRealm, err)
			registry.MarkQoderCatalogFetched(cleanRealm, true)
			return
		}

		models := registry.ParseQoderCatalogModels(body, cleanRealm)
		if len(models) == 0 {
			log.Debugf("qoder catalog: upstream returned 0 valid models for %s", cleanRealm)
			registry.MarkQoderCatalogFetched(cleanRealm, true)
			return
		}

		registry.SetQoderCatalogModels(cleanRealm, models)
		log.Infof("qoder catalog: discovered %d dynamic models for %s", len(models), cleanRealm)
	}

	if isCold {
		// Cold cache: fetch synchronously so initial model list is populated
		fetchFunc()
	} else {
		// Stale cache: refresh in background while serving previous models
		go fetchFunc()
	}
}
