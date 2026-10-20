// Package qoder provides authentication, COSY request signing, and token management
// for Qoder Global and Qoder Mainland China APIs.
package qoder

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"github.com/tidwall/gjson"
)

const (
	// QuotaUsagePath queries account usage and remaining capacity.
	QuotaUsagePath = "/api/v2/quota/usage"
	// UserInfoPath queries account profile and tier information.
	UserInfoPath = "/api/v1/userinfo"
)

// QueryQuota queries account quota points and subscription status for a Qoder credential.
func QueryQuota(ctx context.Context, httpClient *http.Client, auth *cliproxyauth.Auth) (pluginapi.QuotaFetchResponse, error) {
	if auth == nil {
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("qoder: auth is nil")
	}
	realm := ResolveRealm(auth)
	token := AccessToken(auth)
	if token == "" {
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("qoder: no access token in auth")
	}

	editionLabel := "Qoder Global (en)"
	if realm == RealmCN {
		editionLabel = "Qoder Mainland (cn)"
	}

	openAPIBase := ResolveOpenAPIBaseURL(realm)
	reqURL := openAPIBase + QuotaUsagePath

	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if errReq != nil {
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("qoder: create quota request: %w", errReq)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", DefaultUserAgent)

	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	resp, errDo := client.Do(req)
	if errDo != nil {
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("qoder: quota request failed: %w", errDo)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("qoder: read quota response: %w", errRead)
	}

	parsed := gjson.ParseBytes(body)
	code := parsed.Get("code").Int()
	if code != 0 && code != 200 {
		msg := parsed.Get("msg").String()
		if msg == "" {
			msg = parsed.Get("message").String()
		}
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("qoder: quota error code %d: %s", code, msg)
	}

	data := parsed.Get("data")
	total := data.Get("total").Float()
	used := data.Get("used").Float()
	remaining := data.Get("remaining").Float()
	if remaining == 0 && total > 0 {
		remaining = total - used
		if remaining < 0 {
			remaining = 0
		}
	}

	tier := data.Get("tier").String()
	if tier == "" {
		tier = data.Get("level").String()
	}
	if tier == "" {
		tier = "Free"
	}

	planName := fmt.Sprintf("%s · %s", editionLabel, tier)

	var summary []pluginapi.QuotaMetric
	summary = append(summary, pluginapi.QuotaMetric{
		Key:    "points_remain",
		Label:  "Available Points",
		Value:  remaining,
		Unit:   "pts",
		Format: "number",
	})
	if total > 0 {
		summary = append(summary, pluginapi.QuotaMetric{
			Key:    "points_total",
			Label:  "Total Quota",
			Value:  total,
			Unit:   "pts",
			Format: "number",
		}, pluginapi.QuotaMetric{
			Key:    "points_used",
			Label:  "Used Quota",
			Value:  used,
			Unit:   "pts",
			Format: "number",
		})
	}

	remainingFrac := 1.0
	if total > 0 {
		remainingFrac = remaining / total
		if remainingFrac < 0 {
			remainingFrac = 0
		} else if remainingFrac > 1 {
			remainingFrac = 1
		}
	}

	var resetTime string
	expireSec := data.Get("expire_at").Int()
	if expireSec > 0 {
		resetTime = time.Unix(expireSec, 0).UTC().Format(time.RFC3339)
	}

	quotaBucket := pluginapi.QuotaBucket{
		Window:            "monthly",
		RemainingFraction: remainingFrac,
		ResetTime:         resetTime,
		Description:       fmt.Sprintf("%s Resource Quota", editionLabel),
	}

	return pluginapi.QuotaFetchResponse{
		Subscription: &pluginapi.QuotaSubscription{
			Plan: planName,
		},
		Groups: []pluginapi.QuotaGroup{
			{
				DisplayName: fmt.Sprintf("%s Quota", editionLabel),
				Buckets: []pluginapi.QuotaBucket{
					quotaBucket,
				},
			},
		},
		Summary: summary,
	}, nil
}
