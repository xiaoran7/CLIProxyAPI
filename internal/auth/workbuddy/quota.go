// Package workbuddy provides authentication and token management for WorkBuddy and CodeBuddy API.
package workbuddy

import (
	"context"
	"encoding/json"
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
	// UserPointsPath queries account quota points and VIP tier status.
	UserPointsPath = "/v2/plugin/user/points"
	// UserProfilePath queries user profile details.
	UserProfilePath = "/v2/plugin/user/profile"
)

// QueryQuota queries WorkBuddy / CodeBuddy account points and subscription status.
func QueryQuota(ctx context.Context, client *http.Client, auth *cliproxyauth.Auth) (pluginapi.QuotaFetchResponse, error) {
	if auth == nil {
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("workbuddy: auth is nil")
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	token := extractAccessToken(auth)
	if token == "" {
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("workbuddy: missing access token")
	}

	realm := ResolveRealm(auth)
	baseURL := ResolveBaseURL(realm)
	if auth.Attributes != nil {
		if bu := strings.TrimSpace(auth.Attributes["base_url"]); bu != "" {
			baseURL = strings.TrimRight(bu, "/")
		}
	}

	editionLabel := "WorkBuddy (en)"
	if strings.EqualFold(realm, RealmCN) {
		editionLabel = "WorkBuddy (cn)"
	}

	// Query points endpoint first; fall back to profile/account info if points endpoint returns 404 or non-standard shape
	pointsURL := baseURL + UserPointsPath
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, pointsURL, nil)
	if errReq != nil {
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("workbuddy: create quota request: %w", errReq)
	}
	ApplyHeaders(req, auth, token)

	var respBytes []byte
	var statusCode int
	resp, errDo := client.Do(req)
	if errDo == nil {
		defer func() {
			_ = resp.Body.Close()
		}()
		statusCode = resp.StatusCode
		respBytes, _ = io.ReadAll(resp.Body)
	}

	now := time.Now().UTC()
	nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)

	var planName = editionLabel + " · Daily Free Tier"
	var remainingFrac = 1.0
	var usedPoints, totalPoints, remainPoints float64
	var hasExplicitPoints bool

	if statusCode >= 200 && statusCode < 300 && len(respBytes) > 0 && json.Valid(respBytes) {
		data := gjson.GetBytes(respBytes, "data")
		if !data.Exists() {
			data = gjson.ParseBytes(respBytes)
		}

		if vip := strings.TrimSpace(data.Get("vip_type").String()); vip != "" {
			planName = fmt.Sprintf("%s · %s", editionLabel, vip)
		} else if plan := strings.TrimSpace(data.Get("plan_name").String()); plan != "" {
			planName = fmt.Sprintf("%s · %s", editionLabel, plan)
		} else if level := strings.TrimSpace(data.Get("user_level").String()); level != "" {
			planName = fmt.Sprintf("%s · %s", editionLabel, level)
		}

		// Parse total / remaining / used points across possible field names
		for _, k := range []string{"total_points", "total_quota", "limit", "daily_limit", "total"} {
			if v := data.Get(k); v.Exists() && v.Type == gjson.Number && v.Float() > 0 {
				totalPoints = v.Float()
				hasExplicitPoints = true
				break
			}
		}
		for _, k := range []string{"remaining_points", "remain_points", "available_points", "balance", "remaining", "left_points"} {
			if v := data.Get(k); v.Exists() && v.Type == gjson.Number {
				remainPoints = v.Float()
				hasExplicitPoints = true
				break
			}
		}
		for _, k := range []string{"used_points", "used_quota", "used", "consumed_points"} {
			if v := data.Get(k); v.Exists() && v.Type == gjson.Number {
				usedPoints = v.Float()
				hasExplicitPoints = true
				break
			}
		}

		if hasExplicitPoints {
			if totalPoints > 0 {
				if remainPoints == 0 && usedPoints > 0 && usedPoints <= totalPoints {
					remainPoints = totalPoints - usedPoints
				}
				remainingFrac = remainPoints / totalPoints
				if remainingFrac < 0 {
					remainingFrac = 0
				} else if remainingFrac > 1 {
					remainingFrac = 1
				}
			} else if remainPoints > 0 {
				remainingFrac = 1.0
			}
		}
	}

	// Build summary metrics
	var summary []pluginapi.QuotaMetric
	if hasExplicitPoints {
		if totalPoints > 0 {
			summary = append(summary, pluginapi.QuotaMetric{
				Key:    "points_remain",
				Label:  "Available Points",
				Value:  remainPoints,
				Unit:   "pts",
				Format: "number",
			}, pluginapi.QuotaMetric{
				Key:    "points_total",
				Label:  "Total Quota",
				Value:  totalPoints,
				Unit:   "pts",
				Format: "number",
			})
		} else {
			summary = append(summary, pluginapi.QuotaMetric{
				Key:    "points_remain",
				Label:  "Available Points",
				Value:  remainPoints,
				Unit:   "pts",
				Format: "number",
			})
		}
	} else {
		summary = append(summary, pluginapi.QuotaMetric{
			Key:    "free_tier_status",
			Label:  "Daily Free Tier",
			Value:  1,
			Unit:   "active",
			Format: "number",
		})
	}

	return pluginapi.QuotaFetchResponse{
		Subscription: &pluginapi.QuotaSubscription{
			Plan: planName,
		},
		Groups: []pluginapi.QuotaGroup{
			{
				DisplayName: "Daily Quota & Free 1M Models",
				Buckets: []pluginapi.QuotaBucket{
					{
						Window:            "daily",
						RemainingFraction: remainingFrac,
						ResetTime:         nextMidnight,
						Description:       "Tencent WorkBuddy / CodeBuddy Daily Quota Bucket",
					},
				},
			},
		},
		Summary: summary,
	}, nil
}

func extractAccessToken(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	if auth.Metadata != nil {
		if t, ok := auth.Metadata["access_token"].(string); ok && strings.TrimSpace(t) != "" {
			return strings.TrimSpace(t)
		}
		if t, ok := auth.Metadata["token"].(string); ok && strings.TrimSpace(t) != "" {
			return strings.TrimSpace(t)
		}
	}
	if auth.Attributes != nil {
		if t := strings.TrimSpace(auth.Attributes["api_key"]); t != "" {
			return t
		}
		if t := strings.TrimSpace(auth.Attributes["access_token"]); t != "" {
			return t
		}
	}
	return ""
}

// ExtractNickname retrieves the display name, email, or user ID from an auth object.
func ExtractNickname(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	if auth.Metadata != nil {
		for _, k := range []string{"nickname", "email", "user_id"} {
			if v, ok := auth.Metadata[k].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

func extractNickname(auth *cliproxyauth.Auth) string {
	return ExtractNickname(auth)
}
