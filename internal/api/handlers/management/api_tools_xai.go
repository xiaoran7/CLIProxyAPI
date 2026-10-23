package management

import (
	"context"
	"fmt"
	"strings"
	"time"

	xaiauth "github.com/router-for-me/CLIProxyAPI/v8/internal/auth/xai"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

func (h *Handler) resolveXAIToken(ctx context.Context, auth *coreauth.Auth, requestProxyURL string, force bool) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if auth == nil {
		return "", nil
	}

	current := xaiTokenValue(auth)
	if !force && current != "" && !xaiOAuthTokenNeedsRefresh(auth) {
		return current, nil
	}

	refreshToken := xaiRefreshToken(auth)
	if refreshToken == "" {
		return current, nil
	}

	proxyURL := strings.TrimSpace(requestProxyURL)
	if proxyURL == "" {
		proxyURL = strings.TrimSpace(auth.ProxyURL)
	}
	var cfg *config.Config
	if h != nil {
		cfg = h.cfg
	}
	svc := xaiauth.NewXAIAuthWithProxyURL(cfg, proxyURL)
	tokenEndpoint := xaiTokenEndpoint(auth)
	td, errRefresh := svc.RefreshTokens(ctx, refreshToken, tokenEndpoint)
	if errRefresh != nil {
		return "", errRefresh
	}
	if td == nil || strings.TrimSpace(td.AccessToken) == "" {
		return "", fmt.Errorf("xai oauth token refresh returned empty access_token")
	}

	now := time.Now()
	metadata := make(map[string]any, len(auth.Metadata)+10)
	if auth.Metadata != nil {
		for k, v := range auth.Metadata {
			metadata[k] = v
		}
	}
	metadata["type"] = "xai"
	metadata["auth_kind"] = "oauth"
	metadata["access_token"] = strings.TrimSpace(td.AccessToken)
	if strings.TrimSpace(td.RefreshToken) != "" {
		metadata["refresh_token"] = strings.TrimSpace(td.RefreshToken)
	}
	if strings.TrimSpace(td.IDToken) != "" {
		metadata["id_token"] = strings.TrimSpace(td.IDToken)
	}
	if strings.TrimSpace(td.TokenType) != "" {
		metadata["token_type"] = strings.TrimSpace(td.TokenType)
	}
	if td.ExpiresIn > 0 {
		metadata["expires_in"] = td.ExpiresIn
	}
	if strings.TrimSpace(td.Expire) != "" {
		metadata["expired"] = strings.TrimSpace(td.Expire)
	}
	if strings.TrimSpace(td.Email) != "" {
		metadata["email"] = strings.TrimSpace(td.Email)
	}
	if strings.TrimSpace(td.Subject) != "" {
		metadata["sub"] = strings.TrimSpace(td.Subject)
	}
	if tokenEndpoint != "" {
		metadata["token_endpoint"] = tokenEndpoint
	}
	if stringValue(metadata, "base_url") == "" {
		metadata["base_url"] = xaiauth.DefaultAPIBaseURL
	}
	metadata["last_refresh"] = now.UTC().Format(time.RFC3339)
	auth.Metadata = metadata

	attributes := make(map[string]string, len(auth.Attributes)+2)
	if auth.Attributes != nil {
		for k, v := range auth.Attributes {
			attributes[k] = v
		}
	}
	attributes["auth_kind"] = "oauth"
	if strings.TrimSpace(attributes["base_url"]) == "" {
		attributes["base_url"] = xaiauth.DefaultAPIBaseURL
	}
	auth.Attributes = attributes

	if ts := xaiTokenStorage(auth); ts != nil {
		copyStorage := *ts
		copyStorage.AccessToken = strings.TrimSpace(td.AccessToken)
		if strings.TrimSpace(td.RefreshToken) != "" {
			copyStorage.RefreshToken = strings.TrimSpace(td.RefreshToken)
		}
		if strings.TrimSpace(td.IDToken) != "" {
			copyStorage.IDToken = strings.TrimSpace(td.IDToken)
		}
		if strings.TrimSpace(td.TokenType) != "" {
			copyStorage.TokenType = strings.TrimSpace(td.TokenType)
		}
		if td.ExpiresIn > 0 {
			copyStorage.ExpiresIn = td.ExpiresIn
		}
		if strings.TrimSpace(td.Expire) != "" {
			copyStorage.Expire = strings.TrimSpace(td.Expire)
		}
		if strings.TrimSpace(td.Email) != "" {
			copyStorage.Email = strings.TrimSpace(td.Email)
		}
		if strings.TrimSpace(td.Subject) != "" {
			copyStorage.Subject = strings.TrimSpace(td.Subject)
		}
		copyStorage.LastRefresh = now.UTC().Format(time.RFC3339)
		auth.Storage = &copyStorage
	}

	if store := h.tokenStoreWithBaseDir(); store != nil && !coreauth.IsConfigAPIKeyAuth(auth) {
		if _, errSave := store.Save(ctx, auth); errSave != nil {
			log.WithError(errSave).Warn("management APICall: failed to persist refreshed xai oauth token")
		}
	}
	if h != nil && h.authManager != nil {
		auth.LastRefreshedAt = now
		auth.UpdatedAt = now
		if _, errUpdate := h.authManager.Update(ctx, auth); errUpdate != nil {
			log.WithError(errUpdate).Warn("management APICall: failed to update refreshed xai oauth credential in manager")
		}
	}

	return strings.TrimSpace(td.AccessToken), nil
}

func xaiTokenValue(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	if auth.Attributes != nil {
		if v := strings.TrimSpace(auth.Attributes["api_key"]); v != "" {
			return v
		}
	}
	if auth.Metadata != nil {
		if v := stringValue(auth.Metadata, "api_key"); v != "" {
			return v
		}
		if v := stringValue(auth.Metadata, "access_token"); v != "" {
			return v
		}
		if v := stringValue(auth.Metadata, "accessToken"); v != "" {
			return v
		}
	}
	if ts := xaiTokenStorage(auth); ts != nil {
		if v := strings.TrimSpace(ts.AccessToken); v != "" {
			return v
		}
	}
	return ""
}

func xaiRefreshToken(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	if v := stringValue(auth.Metadata, "refresh_token"); v != "" {
		return v
	}
	if ts := xaiTokenStorage(auth); ts != nil {
		return strings.TrimSpace(ts.RefreshToken)
	}
	return ""
}

func xaiTokenEndpoint(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	if v := stringValue(auth.Metadata, "token_endpoint"); v != "" {
		return v
	}
	if ts := xaiTokenStorage(auth); ts != nil {
		return strings.TrimSpace(ts.TokenEndpoint)
	}
	return ""
}

func xaiTokenStorage(auth *coreauth.Auth) *xaiauth.TokenStorage {
	if auth == nil {
		return nil
	}
	ts, ok := auth.Storage.(*xaiauth.TokenStorage)
	if !ok {
		return nil
	}
	return ts
}

func xaiOAuthTokenNeedsRefresh(auth *coreauth.Auth) bool {
	if auth == nil {
		return true
	}
	tokenStr := xaiTokenValue(auth)
	if tokenStr == "" {
		return true
	}

	lead := xaiauth.RefreshLead()
	now := time.Now()
	if lead > 0 {
		now = now.Add(lead)
	}

	temp := auth.Clone()
	if temp.Metadata == nil {
		temp.Metadata = make(map[string]any)
	}
	if stringValue(temp.Metadata, "access_token") == "" {
		temp.Metadata["access_token"] = tokenStr
	}
	if ts := xaiTokenStorage(auth); ts != nil {
		if stringValue(temp.Metadata, "expired") == "" && strings.TrimSpace(ts.Expire) != "" {
			temp.Metadata["expired"] = strings.TrimSpace(ts.Expire)
		}
		if _, ok := temp.Metadata["expires_in"]; !ok && ts.ExpiresIn > 0 {
			temp.Metadata["expires_in"] = ts.ExpiresIn
		}
		if stringValue(temp.Metadata, "last_refresh") == "" && strings.TrimSpace(ts.LastRefresh) != "" {
			temp.Metadata["last_refresh"] = strings.TrimSpace(ts.LastRefresh)
		}
	}

	return !temp.HasValidAccessToken(now)
}
