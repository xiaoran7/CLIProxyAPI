package cliproxy

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/auth/workbuddy"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

// workbuddyCreditsMaxBody caps how much of the catalogue response is read.
const workbuddyCreditsMaxBody = 4 << 20

// workbuddyCreditsFetchTimeout bounds credential acquisition and the catalogue
// request. It is deliberately short: a slow catalogue must never stall model
// registration.
const workbuddyCreditsFetchTimeout = 20 * time.Second

var (
	workbuddyCreditsFetcherOnce sync.Once

	// workbuddyCreditsClientFactory builds the HTTP client used for the
	// catalogue request. Tests replace it to avoid live network access.
	workbuddyCreditsClientFactory = func() *http.Client {
		return &http.Client{Timeout: workbuddyCreditsFetchTimeout}
	}
)

// initWorkBuddyCreditsFetcher installs the process-wide catalogue transport.
//
// The transport needs a live credential because the catalogue is credentialed.
// The resolver is provided by the service so the registry package stays free of
// auth-manager dependencies. Only the first Service to register a WorkBuddy
// account installs it, and an already-installed transport (for example a test
// stub) is never replaced.
func initWorkBuddyCreditsFetcher(resolve func(realm string) *coreauth.Auth) {
	workbuddyCreditsFetcherOnce.Do(func() {
		registry.InstallWorkBuddyCreditsHTTPGetterIfAbsent(func(url string) ([]byte, bool) {
			return fetchWorkBuddyCreditsCatalogue(url, resolve)
		})
	})
}

// authForWorkBuddyCreditsRealm picks a usable credential for a realm. Realm
// membership is resolved from the auth itself, so an account only ever reads the
// catalogue of the realm it belongs to.
func authForWorkBuddyCreditsRealm(auths []*coreauth.Auth, realm string) *coreauth.Auth {
	for _, a := range auths {
		if a == nil || a.Disabled {
			continue
		}
		if !workbuddyAuthIsUsable(a) {
			continue
		}
		if !strings.EqualFold(workbuddy.ResolveRealm(a), realm) {
			continue
		}
		if workbuddy.AccessToken(a) != "" {
			return a
		}
	}
	return nil
}

// workbuddyAuthIsUsable reports whether an auth belongs to a WorkBuddy provider.
func workbuddyAuthIsUsable(a *coreauth.Auth) bool {
	if a == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(a.Provider)) {
	case "workbuddy", "workbuddy-intl", "workbuddy-cn", "copilot-tencent":
		return true
	}
	if a.Attributes != nil && strings.EqualFold(strings.TrimSpace(a.Attributes["auth_kind"]), "workbuddy") {
		return true
	}
	return false
}

// warmWorkBuddyCreditsForAuth refreshes a realm's rate catalogue if it is stale.
//
// A stale catalogue is served while the refresh runs in the background, so hot
// paths never block on the network; only a cold cache waits, and only for a
// bounded window. Model registration is unaffected either way: rates are an
// additive annotation and a failed fetch simply leaves them unknown.
func (s *Service) warmWorkBuddyCreditsForAuth(ctx context.Context, a *coreauth.Auth, realm string) {
	if a == nil || s == nil || s.coreManager == nil {
		return
	}
	initWorkBuddyCreditsFetcher(func(target string) *coreauth.Auth {
		if s.coreManager == nil {
			return nil
		}
		auth := authForWorkBuddyCreditsRealm(s.coreManager.List(), target)
		if auth == nil && a != nil && workbuddyAuthIsUsable(a) && strings.EqualFold(workbuddy.ResolveRealm(a), target) && workbuddy.AccessToken(a) != "" {
			return a
		}
		return auth
	})

	// WarmWorkBuddyCredits is a no-op while the cached catalogue is fresh and
	// otherwise refreshes in the background, serving the previous rates in the
	// meantime. A cold cache fetches inline so the first listing already carries
	// rates.
	registry.WarmWorkBuddyCredits(realm)
}

// fetchWorkBuddyCreditsCatalogue performs the credentialed GET /v3/config call.
func fetchWorkBuddyCreditsCatalogue(url string, resolve func(realm string) *coreauth.Auth) (body []byte, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("workbuddy credits: fetch panic recovered: %v", r)
			body = nil
			ok = false
		}
	}()

	realm := "intl"
	if strings.Contains(strings.ToLower(url), "tencent.com") || strings.Contains(strings.ToLower(url), "codebuddy.cn") {
		realm = "cn"
	}
	if resolve == nil {
		return nil, false
	}
	auth := resolve(realm)
	if auth == nil {
		return nil, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), workbuddyCreditsFetchTimeout)
	defer cancel()

	token := workbuddy.AccessToken(auth)
	if token == "" {
		return nil, false
	}

	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if errReq != nil {
		log.Debugf("workbuddy credits: build request: %v", errReq)
		return nil, false
	}
	workbuddy.ApplyCatalogHeaders(req, auth, token)

	client := workbuddyCreditsClientFactory()
	if client == nil {
		return nil, false
	}
	resp, errDo := client.Do(req)
	if errDo != nil {
		log.Debugf("workbuddy credits: fetch %s: %v", realm, errDo)
		return nil, false
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Debugf("workbuddy credits: close body: %v", errClose)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		log.Debugf("workbuddy credits: fetch %s: HTTP %d", realm, resp.StatusCode)
		return nil, false
	}

	read, errRead := io.ReadAll(io.LimitReader(resp.Body, workbuddyCreditsMaxBody))
	if errRead != nil {
		log.Debugf("workbuddy credits: read %s: %v", realm, errRead)
		return nil, false
	}
	return read, true
}
