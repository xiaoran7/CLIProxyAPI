package management

import (
	"context"
	"fmt"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/runtime/executor"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func metaTokenFromAuth(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	if auth.Metadata != nil {
		if k, ok := auth.Metadata["api_key"].(string); ok && strings.TrimSpace(k) != "" && !strings.HasPrefix(strings.TrimSpace(k), "dca:") {
			return strings.TrimSpace(k)
		}
		if t, ok := auth.Metadata["access_token"].(string); ok && strings.TrimSpace(t) != "" && !strings.HasPrefix(strings.TrimSpace(t), "dca:") {
			return strings.TrimSpace(t)
		}
	}
	if auth.Attributes != nil {
		if k := strings.TrimSpace(auth.Attributes["api_key"]); k != "" && !strings.HasPrefix(k, "dca:") {
			return k
		}
		if t := strings.TrimSpace(auth.Attributes["access_token"]); t != "" && !strings.HasPrefix(t, "dca:") {
			return t
		}
	}
	return ""
}

// metaManagementPreparer applies the tool's proxy override only to acquisition.
// The saved credential retains its configured proxy.
type metaManagementPreparer struct {
	executor *executor.MetaExecutor
	proxyURL string
}

func (p metaManagementPreparer) ShouldPrepareRequestAuth(auth *coreauth.Auth) bool {
	return p.executor.ShouldPrepareRequestAuth(auth)
}

func (p metaManagementPreparer) PrepareRequestAuth(ctx context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	proxyURL := auth.ProxyURL
	if strings.TrimSpace(p.proxyURL) != "" {
		auth.ProxyURL = p.proxyURL
	}
	updated, err := p.executor.PrepareRequestAuth(ctx, auth)
	if updated != nil {
		updated.ProxyURL = proxyURL
	}
	return updated, err
}

func (h *Handler) resolveMetaToken(ctx context.Context, auth *coreauth.Auth, requestProxyURL string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if auth == nil {
		return "", nil
	}
	if token := metaTokenFromAuth(auth); token != "" {
		return token, nil
	}
	var cfg *config.Config
	if h != nil {
		cfg = h.cfg
	}
	preparer := metaManagementPreparer{executor: executor.NewMetaExecutor(cfg), proxyURL: requestProxyURL}
	if !preparer.ShouldPrepareRequestAuth(auth) {
		return "", nil
	}
	if h == nil || h.authManager == nil || auth.ID == "" {
		return "", fmt.Errorf("meta token mint requires a registered credential")
	}
	updated, err := h.authManager.PrepareRequestAuth(ctx, preparer, auth)
	if err != nil {
		return "", err
	}
	return metaTokenFromAuth(updated), nil
}
