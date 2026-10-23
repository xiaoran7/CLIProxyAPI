package management

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

func tokenValueForAuth(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	if v := tokenValueFromMetadata(auth.Metadata); v != "" {
		return v
	}
	if auth.Attributes != nil {
		if v := strings.TrimSpace(auth.Attributes["api_key"]); v != "" {
			return v
		}
		if v := strings.TrimSpace(auth.Attributes["session_token"]); v != "" {
			return v
		}
	}
	return ""
}

func (h *Handler) resolveTokenForAuth(ctx context.Context, auth *coreauth.Auth, requestProxyURL string) (string, error) {
	return h.resolveTokenForAuthWithForce(ctx, auth, requestProxyURL, false)
}

func (h *Handler) resolveTokenForAuthWithForce(ctx context.Context, auth *coreauth.Auth, requestProxyURL string, force bool) (string, error) {
	if auth == nil {
		return "", nil
	}

	if strings.EqualFold(strings.TrimSpace(auth.Provider), "antigravity") {
		token, errToken := h.refreshAntigravityOAuthAccessToken(ctx, auth, requestProxyURL, force)
		return token, errToken
	}

	if strings.EqualFold(strings.TrimSpace(auth.Provider), "meta") {
		token, errToken := h.resolveMetaToken(ctx, auth, requestProxyURL)
		return token, errToken
	}

	if strings.EqualFold(strings.TrimSpace(auth.Provider), "xai") {
		token, errToken := h.resolveXAIToken(ctx, auth, requestProxyURL, force)
		return token, errToken
	}

	return tokenValueForAuth(auth), nil
}

var antigravityOAuthTokenURL = "https://oauth2.googleapis.com/token"

func antigravityTokenNeedsRefresh(metadata map[string]any) bool {
	// Refresh a bit early to avoid requests racing token expiry.
	const skew = 30 * time.Second

	if metadata == nil {
		return true
	}
	if expStr, ok := metadata["expired"].(string); ok {
		if ts, errParse := time.Parse(time.RFC3339, strings.TrimSpace(expStr)); errParse == nil {
			return !ts.After(time.Now().Add(skew))
		}
	}
	expiresIn := int64Value(metadata["expires_in"])
	timestampMs := int64Value(metadata["timestamp"])
	if expiresIn > 0 && timestampMs > 0 {
		exp := time.UnixMilli(timestampMs).Add(time.Duration(expiresIn) * time.Second)
		return !exp.After(time.Now().Add(skew))
	}
	return true
}

func (h *Handler) refreshAntigravityOAuthAccessToken(ctx context.Context, auth *coreauth.Auth, requestProxyURL string, force bool) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if auth == nil {
		return "", nil
	}

	metadata := auth.Metadata
	if len(metadata) == 0 {
		return "", fmt.Errorf("antigravity oauth metadata missing")
	}

	current := strings.TrimSpace(tokenValueFromMetadata(metadata))
	if !force && current != "" && !antigravityTokenNeedsRefresh(metadata) {
		return current, nil
	}

	refreshToken := stringValue(metadata, "refresh_token")
	if refreshToken == "" {
		return "", fmt.Errorf("antigravity refresh token missing")
	}

	tokenURL := strings.TrimSpace(antigravityOAuthTokenURL)
	if tokenURL == "" {
		tokenURL = "https://oauth2.googleapis.com/token"
	}
	form := url.Values{}
	form.Set("client_id", antigravityOAuthClientID)
	form.Set("client_secret", antigravityOAuthClientSecret)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	req, errReq := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if errReq != nil {
		return "", errReq
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{
		Timeout:   defaultAPICallTimeout,
		Transport: h.apiCallTransport(auth, requestProxyURL),
	}
	resp, errDo := httpClient.Do(req)
	if errDo != nil {
		return "", errDo
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Errorf("response body close error: %v", errClose)
		}
	}()

	bodyBytes, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return "", errRead
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("antigravity oauth token refresh failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if errUnmarshal := json.Unmarshal(bodyBytes, &tokenResp); errUnmarshal != nil {
		return "", errUnmarshal
	}

	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return "", fmt.Errorf("antigravity oauth token refresh returned empty access_token")
	}

	if auth.Metadata == nil {
		auth.Metadata = make(map[string]any)
	}
	now := time.Now()
	auth.Metadata["access_token"] = strings.TrimSpace(tokenResp.AccessToken)
	if strings.TrimSpace(tokenResp.RefreshToken) != "" {
		auth.Metadata["refresh_token"] = strings.TrimSpace(tokenResp.RefreshToken)
	}
	if tokenResp.ExpiresIn > 0 {
		auth.Metadata["expires_in"] = tokenResp.ExpiresIn
		auth.Metadata["timestamp"] = now.UnixMilli()
		auth.Metadata["expired"] = now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339)
	}
	auth.Metadata["type"] = "antigravity"

	if store := h.tokenStoreWithBaseDir(); store != nil && !coreauth.IsConfigAPIKeyAuth(auth) {
		if _, errSave := store.Save(ctx, auth); errSave != nil {
			log.WithError(errSave).Warn("management APICall: failed to persist refreshed antigravity oauth token")
		}
	}
	if h != nil && h.authManager != nil {
		auth.LastRefreshedAt = now
		auth.UpdatedAt = now
		if _, errUpdate := h.authManager.Update(ctx, auth); errUpdate != nil {
			log.WithError(errUpdate).Warn("management APICall: failed to update refreshed antigravity oauth credential in manager")
		}
	}

	return strings.TrimSpace(tokenResp.AccessToken), nil
}

// reconcileAntigravityQuotaResponse reconciles optimistic or stale upstream Google PA
// quota summaries with the runtime conductor's ground truth (active 429 / cooldown state).
func reconcileAntigravityQuotaResponse(auth *coreauth.Auth, respBody []byte, now time.Time) []byte {
	if auth == nil || len(respBody) == 0 {
		return respBody
	}
	cooldowns := coreauth.CooldownSnapshotForAuth(auth, now)
	if len(cooldowns) == 0 {
		return respBody
	}

	var quotaResp map[string]any
	if err := json.Unmarshal(respBody, &quotaResp); err != nil {
		return respBody
	}

	groups, ok := quotaResp["groups"].([]any)
	if !ok || len(groups) == 0 {
		return respBody
	}

	hasCredCooldown := false
	var credRetryAt time.Time
	var credRemainingSec int64

	var claudeCooldowns []coreauth.CooldownView
	var geminiCooldowns []coreauth.CooldownView

	for _, cd := range cooldowns {
		if cd.RemainingSeconds <= 0 && (cd.RetryAt.IsZero() || !cd.RetryAt.After(now)) {
			continue
		}
		if cd.Scope == "credential" {
			hasCredCooldown = true
			if cd.RetryAt.After(credRetryAt) {
				credRetryAt = cd.RetryAt
				credRemainingSec = cd.RemainingSeconds
			}
		} else if cd.Scope == "model" {
			mKey := strings.ToLower(strings.TrimSpace(cd.ModelKey))
			if strings.Contains(mKey, "claude") || strings.Contains(mKey, "gpt") ||
				strings.Contains(mKey, "sonnet") || strings.Contains(mKey, "opus") ||
				strings.Contains(mKey, "haiku") {
				claudeCooldowns = append(claudeCooldowns, cd)
			}
			if strings.Contains(mKey, "gemini") || strings.Contains(mKey, "flash") ||
				(strings.Contains(mKey, "pro") && !strings.Contains(mKey, "claude")) {
				geminiCooldowns = append(geminiCooldowns, cd)
			}
		}
	}

	if !hasCredCooldown && len(claudeCooldowns) == 0 && len(geminiCooldowns) == 0 {
		return respBody
	}

	for _, rawGrp := range groups {
		grp, okGrp := rawGrp.(map[string]any)
		if !okGrp {
			continue
		}
		gName := strings.ToLower(fmt.Sprintf("%v %v %v %v", grp["label"], grp["displayName"], grp["name"], grp["id"]))
		isClaude := strings.Contains(gName, "claude") || strings.Contains(gName, "gpt")
		isGemini := strings.Contains(gName, "gemini")

		var targetCooldowns []coreauth.CooldownView
		if hasCredCooldown {
			targetCooldowns = append(targetCooldowns, coreauth.CooldownView{
				Scope:            "credential",
				RetryAt:          credRetryAt,
				RemainingSeconds: credRemainingSec,
			})
		} else if isClaude {
			targetCooldowns = claudeCooldowns
		} else if isGemini {
			targetCooldowns = geminiCooldowns
		}

		if len(targetCooldowns) == 0 {
			continue
		}

		var maxRemaining int64
		var latestRetry time.Time
		for _, cd := range targetCooldowns {
			if cd.RemainingSeconds > maxRemaining {
				maxRemaining = cd.RemainingSeconds
			}
			if cd.RetryAt.After(latestRetry) {
				latestRetry = cd.RetryAt
			}
		}
		if latestRetry.IsZero() && maxRemaining > 0 {
			latestRetry = now.Add(time.Duration(maxRemaining) * time.Second)
		}
		retryAtStr := latestRetry.UTC().Format(time.RFC3339)

		buckets, okBuckets := grp["buckets"].([]any)
		if !okBuckets {
			continue
		}

		for _, rawBkt := range buckets {
			bkt, okBkt := rawBkt.(map[string]any)
			if !okBkt {
				continue
			}
			bName := strings.ToLower(fmt.Sprintf("%v %v %v %v %v %v", bkt["label"], bkt["displayName"], bkt["name"], bkt["id"], bkt["window"], bkt["description"]))
			isWeekly := strings.Contains(bName, "weekly") || strings.Contains(bName, "week")
			is5h := strings.Contains(bName, "5h") || strings.Contains(bName, "5 hour") || strings.Contains(bName, "five")

			if hasCredCooldown {
				bkt["remainingFraction"] = float64(0)
				bkt["remaining_fraction"] = float64(0)
				if retryAtStr != "" {
					bkt["resetTime"] = retryAtStr
					bkt["reset_time"] = retryAtStr
				}
			} else if maxRemaining > 5*3600 {
				// Cooldown > 5 hours implies weekly limit exhaustion (e.g. 45h 429 rate limit).
				// All buckets for this model family are blocked.
				bkt["remainingFraction"] = float64(0)
				bkt["remaining_fraction"] = float64(0)
				if retryAtStr != "" {
					bkt["resetTime"] = retryAtStr
					bkt["reset_time"] = retryAtStr
				}
			} else if is5h || !isWeekly {
				bkt["remainingFraction"] = float64(0)
				bkt["remaining_fraction"] = float64(0)
				if retryAtStr != "" {
					bkt["resetTime"] = retryAtStr
					bkt["reset_time"] = retryAtStr
				}
			}
		}
	}
	reconciled, errMarshal := json.Marshal(quotaResp)
	if errMarshal != nil {
		return respBody
	}
	return reconciled
}

func tokenValueFromMetadata(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	if v, ok := metadata["accessToken"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v, ok := metadata["access_token"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if tokenRaw, ok := metadata["token"]; ok && tokenRaw != nil {
		switch typed := tokenRaw.(type) {
		case string:
			if v := strings.TrimSpace(typed); v != "" {
				return v
			}
		case map[string]any:
			if v, ok := typed["access_token"].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
			if v, ok := typed["accessToken"].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case map[string]string:
			if v := strings.TrimSpace(typed["access_token"]); v != "" {
				return v
			}
			if v := strings.TrimSpace(typed["accessToken"]); v != "" {
				return v
			}
		}
	}
	if v, ok := metadata["token"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v, ok := metadata["id_token"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v, ok := metadata["api_key"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v, ok := metadata["session_token"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v, ok := metadata["cookie"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return ""
}

func int64Value(raw any) int64 {
	switch typed := raw.(type) {
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint64:
		if typed > uint64(^uint64(0)>>1) {
			return 0
		}
		return int64(typed)
	case float32:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		if i, errParse := typed.Int64(); errParse == nil {
			return i
		}
	case string:
		if s := strings.TrimSpace(typed); s != "" {
			if i, errParse := json.Number(s).Int64(); errParse == nil {
				return i
			}
		}
	}
	return 0
}

func stringValue(metadata map[string]any, key string) string {
	if len(metadata) == 0 || key == "" {
		return ""
	}
	if v, ok := metadata[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
