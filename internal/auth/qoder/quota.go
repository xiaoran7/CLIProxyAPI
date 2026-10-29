// Package qoder provides authentication, COSY request signing, and token management
// for Qoder Global and Qoder Mainland China APIs.
package qoder

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)
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
	if codeRes := parsed.Get("code"); codeRes.Exists() {
		code := codeRes.Int()
		if code != 0 && code != 200 {
			msg := parsed.Get("msg").String()
			if msg == "" {
				msg = parsed.Get("message").String()
			}
			return pluginapi.QuotaFetchResponse{}, fmt.Errorf("qoder: quota error code %d: %s", code, msg)
		}
	}

	root := parsed
	if d := parsed.Get("data"); d.IsObject() {
		if !parsed.Get("userQuota").Exists() && !parsed.Get("userId").Exists() && !parsed.Get("userType").Exists() {
			root = d
		} else if d.Get("userQuota").Exists() {
			root = d
		}
	}

	var total, used, remaining float64
	var userTotal, userUsed, userRemaining float64
	var addOnTotal, addOnUsed, addOnRemaining float64
	hasStructuredQuota := false

	if uq := root.Get("userQuota"); uq.IsObject() {
		userTotal = uq.Get("total").Float()
		userUsed = uq.Get("used").Float()
		userRemaining = uq.Get("remaining").Float()
		hasStructuredQuota = true
	}
	if aq := root.Get("addOnQuota"); aq.IsObject() {
		addOnTotal = aq.Get("total").Float()
		addOnUsed = aq.Get("used").Float()
		addOnRemaining = aq.Get("remaining").Float()
		hasStructuredQuota = true
	}

	if hasStructuredQuota {
		total = userTotal + addOnTotal
		used = userUsed + addOnUsed
		remaining = userRemaining + addOnRemaining
		if remaining == 0 && total > 0 && used > 0 {
			remaining = total - used
			if remaining < 0 {
				remaining = 0
			}
		}
	} else {
		total = root.Get("total").Float()
		if total == 0 {
			total = root.Get("total_points").Float()
		}
		if total == 0 {
			total = root.Get("quota").Float()
		}
		used = root.Get("used").Float()
		if used == 0 {
			used = root.Get("used_points").Float()
		}
		remaining = root.Get("remaining").Float()
		if remaining == 0 {
			remaining = root.Get("remaining_points").Float()
		}
		if remaining == 0 {
			remaining = root.Get("points").Float()
		}
		if remaining == 0 {
			remaining = root.Get("balance").Float()
		}
		if remaining == 0 && total > 0 && used > 0 {
			remaining = total - used
			if remaining < 0 {
				remaining = 0
			}
		}
	}

	userTypeRaw := root.Get("userType").String()
	if userTypeRaw == "" {
		userTypeRaw = root.Get("user_type").String()
	}
	if userTypeRaw == "" {
		userTypeRaw = root.Get("tier").String()
	}
	if userTypeRaw == "" {
		userTypeRaw = root.Get("level").String()
	}

	var tier string
	switch strings.ToLower(strings.TrimSpace(userTypeRaw)) {
	case "personal_professional_trial", "pro_trial", "trial":
		tier = "Pro Trial"
	case "personal_professional", "professional", "pro":
		tier = "Pro"
	case "personal_standard", "standard":
		tier = "Standard"
	case "personal_free", "free":
		tier = "Free"
	case "team", "enterprise":
		tier = "Enterprise"
	default:
		if userTypeRaw != "" {
			tier = userTypeRaw
		} else {
			tier = "Free"
		}
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
	if hasStructuredQuota && userTotal > 0 {
		summary = append(summary, pluginapi.QuotaMetric{
			Key:    "user_quota_remain",
			Label:  "Monthly Allowance",
			Value:  userRemaining,
			Unit:   "pts",
			Format: "number",
		})
	}
	if hasStructuredQuota && addOnTotal > 0 {
		summary = append(summary, pluginapi.QuotaMetric{
			Key:    "addon_quota_remain",
			Label:  "Bonus / Check-in Quota",
			Value:  addOnRemaining,
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
	expireSec := root.Get("expiresAt").Int()
	if expireSec == 0 {
		expireSec = root.Get("expires_at").Int()
	}
	if expireSec == 0 {
		expireSec = root.Get("expire_at").Int()
	}
	if expireSec == 0 {
		expireSec = root.Get("expire_time").Int()
	}
	if expireSec > 1e11 {
		expireSec /= 1000
	}
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
