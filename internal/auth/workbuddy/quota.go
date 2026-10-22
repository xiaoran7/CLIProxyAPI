// Package workbuddy provides authentication and token management for WorkBuddy and CodeBuddy API.
package workbuddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"github.com/tidwall/gjson"
)

const (
	// UserPointsPath queries account quota points and VIP tier status.
	UserPointsPath = "/v2/plugin/user/points"
	// UserProfilePath queries user profile details.
	UserProfilePath = "/v2/plugin/user/profile"

	// MeterUserResourceSummaryPath is the resource summary endpoint on /billing/meter without /v2 gateway prefix.
	MeterUserResourceSummaryPath = "/billing/meter/get-user-resource-summary"
	// MeterEnterpriseUsagePath is the enterprise user usage endpoint for team/enterprise accounts.
	MeterEnterpriseUsagePath = "/billing/meter/get-enterprise-user-usage"
	// MeterPaidPackagesPath is the paid packages endpoint with expiration info.
	MeterPaidPackagesPath = "/billing/meter/get-user-resource-paid-packages"
	// MeterFreePackagesPath is the free packages endpoint with slice and expiration info.
	MeterFreePackagesPath = "/billing/meter/get-user-resource-free-packages"

	// DailySlicePackageCode is the daily package refreshed every midnight.
	DailySlicePackageCode = "TCACA_code_001_PqouKr6QWV"
	// SliceCapacityType is the capacity type for daily slice packages.
	SliceCapacityType = "4"
)

var (
	paidPackageCodes = []string{
		"TCACA_code_002_AkiJS3ZHF5",
		"TCACA_code_005_maRGyrHhw1",
		"TCACA_code_003_FAnt7lcmRT",
		"TCACA_code_023_4xbGhMrE6q",
		"TCACA_code_026_BaESVICNoi",
		"TCACA_code_027_0FCGVA6vSa",
		"TCACA_code_009_0XmEQc2xOf",
		"TCACA_code_038_OhvqZtiPKr",
		"TCACA_code_036_lupO5WgNdG",
	}

	freePackageCodes = []string{
		DailySlicePackageCode,
		"TCACA_code_008_cfWoLwvjU4",
		"TCACA_code_035_ArVxJcGDsm",
		"TCACA_code_006_DbXS0lrypC",
		"TCACA_code_039_KRcQj7wUat",
		"TCACA_code_040_mi9rCYg46x",
		"TCACA_code_007_nzdH5h4Nl0",
		"TCACA_code_028_NtpWi0jzXs",
		"TCACA_code_037_WxOD3MpI2o",
		"TCACA_code_029_6wCGEWquYy",
		"TCACA_code_030_BjSt89qTvr",
	}

	beijingLocation = time.FixedZone("CST", 8*3600)
)

// parseCapacity parses a number or string capacity ("438.88" or 438.88) to float64.
func parseCapacity(val gjson.Result) float64 {
	if !val.Exists() {
		return 0
	}
	switch val.Type {
	case gjson.Number:
		v := val.Float()
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0
		}
		return v
	case gjson.String:
		s := strings.TrimSpace(val.String())
		if s == "" {
			return 0
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0
		}
		return n
	default:
		return 0
	}
}

// parseBeijingTime parses "2006-01-02 15:04:05" in CST (UTC+8).
func parseBeijingTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, beijingLocation); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

// parseEndOfPackage determines the expiration time in milliseconds for a package.
func parseEndOfPackage(pkg gjson.Result) int64 {
	if ms := pkg.Get("DeductionEndTime").Int(); ms > 0 {
		return ms
	}
	for _, k := range []string{"ExpiredTime", "CycleEndTime"} {
		if v := strings.TrimSpace(pkg.Get(k).String()); v != "" {
			if t := parseBeijingTime(v); !t.IsZero() {
				return t.UnixMilli()
			}
		}
	}
	return 0
}

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

	now := time.Now().UTC()
	nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)

	var planName = editionLabel + " · Daily Free Tier"
	var remainingFrac = 1.0
	var usedPoints, totalPoints, remainPoints float64
	var hasExplicitPoints bool
	var expiresAt string
	var runsOutAt string
	var runsOutPoints float64
	var hasRunsOut bool

	// Helper for POST requests to /billing/meter/...
	postMeter := func(path string, reqBody any) ([]byte, int, error) {
		bodyBytes, _ := json.Marshal(reqBody)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, 0, err
		}
		ApplyHeaders(req, auth, token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("X-Product", "SaaS")
		req.Header.Set("X-IDE-Type", "WorkBuddy")

		resp, err := client.Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer func() { _ = resp.Body.Close() }()
		respBytes, errRead := io.ReadAll(resp.Body)
		return respBytes, resp.StatusCode, errRead
	}

	// 1. Primary path: query /billing/meter/get-user-resource-summary
	summaryBytes, summaryStatus, errSummary := postMeter(MeterUserResourceSummaryPath, map[string]any{})
	if errSummary == nil && summaryStatus == http.StatusOK && json.Valid(summaryBytes) {
		data := gjson.GetBytes(summaryBytes, "data")
		if !data.Exists() {
			data = gjson.ParseBytes(summaryBytes)
		}

		isPaidUser := data.Get("IsPaidUser").Bool()
		if isPaidUser {
			planName = editionLabel + " · Pro"
		} else {
			planName = editionLabel + " · Free"
		}

		pkgs := data.Get("Packages")
		if pkgs.IsArray() && len(pkgs.Array()) > 0 {
			countedCodes := make(map[string]bool)
			for _, p := range pkgs.Array() {
				code := p.Get("PackageCode").String()
				if code != "" {
					countedCodes[code] = true
				}
				tot := parseCapacity(p.Get("CycleTotalCapacity"))
				used := parseCapacity(p.Get("CycleUsedCapacity"))
				rem := parseCapacity(p.Get("CycleRemainCapacity"))
				if rem == 0 && tot > used {
					rem = tot - used
				}
				totalPoints += tot
				usedPoints += used
				remainPoints += rem
			}
			hasExplicitPoints = true

			// Fetch package details to discover earliest expiration / cutoff date
			nowBeijing := time.Now().In(beijingLocation)
			dayStr := nowBeijing.Format("2006-01-02")
			nowMs := now.UnixMilli()

			// Dynamically combine builtin package codes with codes active on the account
			activePaidCodes := make([]string, 0, len(paidPackageCodes)+len(countedCodes))
			activePaidCodes = append(activePaidCodes, paidPackageCodes...)
			activePaidSet := make(map[string]bool, len(paidPackageCodes))
			for _, c := range paidPackageCodes {
				activePaidSet[c] = true
			}

			activeFreeCodes := make([]string, 0, len(freePackageCodes)+len(countedCodes))
			activeFreeCodes = append(activeFreeCodes, freePackageCodes...)
			activeFreeSet := make(map[string]bool, len(freePackageCodes))
			for _, c := range freePackageCodes {
				activeFreeSet[c] = true
			}

			for code := range countedCodes {
				if !activePaidSet[code] && !activeFreeSet[code] {
					activePaidCodes = append(activePaidCodes, code)
					activePaidSet[code] = true
					activeFreeCodes = append(activeFreeCodes, code)
					activeFreeSet[code] = true
				}
			}

			type meterPackageCall struct {
				path  string
				extra map[string]any
			}
			calls := []meterPackageCall{
				{
					path: MeterPaidPackagesPath,
					extra: map[string]any{
						"PackageCodes":  activePaidCodes,
						"NeedRenewInfo": true,
					},
				},
				{
					path: MeterFreePackagesPath,
					extra: map[string]any{
						"PackageCodes":         activeFreeCodes,
						"SlicePeriodStartTime": dayStr + " 00:00:00",
						"SlicePeriodEndTime":   dayStr + " 23:59:59",
					},
				},
			}

			var earliestAt int64
			var earliestAmount float64

			for _, c := range calls {
				for page := 1; page <= 5; page++ {
					bodyMap := map[string]any{
						"PageNumber": page,
						"PageSize":   200,
						"Status":     []int{0, 3},
					}
					for k, v := range c.extra {
						bodyMap[k] = v
					}
					pkgBytes, pkgStatus, errPkg := postMeter(c.path, bodyMap)
					if errPkg != nil || pkgStatus != http.StatusOK || !json.Valid(pkgBytes) {
						break
					}
					pkgData := gjson.GetBytes(pkgBytes, "data")
					if !pkgData.Exists() {
						pkgData = gjson.ParseBytes(pkgBytes)
					}
					accounts := pkgData.Get("Accounts")
					if !accounts.IsArray() || len(accounts.Array()) == 0 {
						break
					}
					for _, p := range accounts.Array() {
						pCode := p.Get("PackageCode").String()
						capType := strings.TrimSpace(p.Get("CapacityType").String())
						if !countedCodes[pCode] || pCode == DailySlicePackageCode || capType == SliceCapacityType {
							continue
						}
						remPrecise := p.Get("CycleCapacityRemainPrecise")
						if !remPrecise.Exists() {
							remPrecise = p.Get("CycleCapacityRemain")
						}
						left := parseCapacity(remPrecise)
						end := parseEndOfPackage(p)
						if left <= 0 || end <= nowMs {
							continue
						}
						if earliestAt == 0 || end < earliestAt {
							earliestAt = end
							earliestAmount = left
						} else if end == earliestAt {
							earliestAmount += left
						}
					}
					totalCount := pkgData.Get("TotalCount").Int()
					if int64(len(accounts.Array())) < 200 || int64(page*200) >= totalCount {
						break
					}
				}
			}

			if earliestAt > 0 {
				earliestTime := time.UnixMilli(earliestAt).UTC()
				runsOutAt = earliestTime.Format(time.RFC3339)
				expiresAt = earliestTime.In(beijingLocation).Format("2006-01-02 15:04:05")
				runsOutPoints = earliestAmount
				hasRunsOut = true
			}
		} else {
			// Packages array is empty: query enterprise usage endpoint
			entBytes, entStatus, errEnt := postMeter(MeterEnterpriseUsagePath, map[string]any{})
			if errEnt == nil && entStatus == http.StatusOK && json.Valid(entBytes) {
				entData := gjson.GetBytes(entBytes, "data")
				if !entData.Exists() {
					entData = gjson.ParseBytes(entBytes)
				}
				limit := parseCapacity(entData.Get("limitNum"))
				if limit > 0 {
					planName = editionLabel + " · Enterprise"
					credit := parseCapacity(entData.Get("credit"))
					totalPoints = limit
					usedPoints = credit
					if limit >= credit {
						remainPoints = limit - credit
					}
					hasExplicitPoints = true

					resetTimeStr := strings.TrimSpace(entData.Get("cycleResetTime").String())
					if resetTimeStr != "" {
						if t := parseBeijingTime(resetTimeStr); !t.IsZero() {
							runsOutAt = t.UTC().Format(time.RFC3339)
							expiresAt = t.In(beijingLocation).Format("2006-01-02 15:04:05")
							runsOutPoints = remainPoints
							hasRunsOut = true
						}
					}
				}
			}
		}
	}

	// 2. Fallback to /v2/plugin/user/points if meter endpoints didn't yield explicit points
	if !hasExplicitPoints {
		pointsURL := baseURL + UserPointsPath
		req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, pointsURL, nil)
		if errReq == nil {
			ApplyHeaders(req, auth, token)
			resp, errDo := client.Do(req)
			if errDo == nil {
				defer func() { _ = resp.Body.Close() }()
				if respBytes, errRead := io.ReadAll(resp.Body); errRead == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && json.Valid(respBytes) {
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

					for _, k := range []string{"total_points", "total_quota", "limit", "daily_limit", "total"} {
						if v := data.Get(k); v.Exists() && parseCapacity(v) > 0 {
							totalPoints = parseCapacity(v)
							hasExplicitPoints = true
							break
						}
					}
					for _, k := range []string{"remaining_points", "remain_points", "available_points", "balance", "remaining", "left_points"} {
						if v := data.Get(k); v.Exists() {
							remainPoints = parseCapacity(v)
							hasExplicitPoints = true
							break
						}
					}
					for _, k := range []string{"used_points", "used_quota", "used", "consumed_points"} {
						if v := data.Get(k); v.Exists() {
							usedPoints = parseCapacity(v)
							hasExplicitPoints = true
							break
						}
					}
				}
			}
		}
	}

	// 3. Fallback to /v2/billing/meter/checkin-activity-status if still no explicit points
	if !hasExplicitPoints {
		statusURL := baseURL + "/v2/billing/meter/checkin-activity-status"
		reqActivity, errActivityReq := http.NewRequestWithContext(ctx, http.MethodPost, statusURL, bytes.NewReader([]byte("{}")))
		if errActivityReq == nil {
			ApplyHeaders(reqActivity, auth, token)
			reqActivity.Header.Set("Content-Type", "application/json")
			respActivity, errActivityDo := client.Do(reqActivity)
			if errActivityDo == nil {
				defer func() { _ = respActivity.Body.Close() }()
				bodyActivity, _ := io.ReadAll(respActivity.Body)
				if respActivity.StatusCode == http.StatusOK && json.Valid(bodyActivity) {
					data := gjson.GetBytes(bodyActivity, "data")
					if data.Exists() {
						if totalCredits := data.Get("total_credits"); totalCredits.Exists() {
							remainPoints = parseCapacity(totalCredits)
							hasExplicitPoints = true
						}
						if dailyCredit := data.Get("daily_credit"); dailyCredit.Exists() && parseCapacity(dailyCredit) > 0 {
							totalPoints = parseCapacity(dailyCredit)
						}
						theme := strings.TrimSpace(data.Get("theme_name").String())
						season := data.Get("season").Int()
						if theme != "" {
							if season > 0 {
								planName = fmt.Sprintf("%s · %s (Season %d)", editionLabel, theme, season)
							} else {
								planName = fmt.Sprintf("%s · %s", editionLabel, theme)
							}
						}
					}
				}
			}
		}
	}

	// Calculate remaining fraction
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

	// Build summary metrics
	var summary []pluginapi.QuotaMetric
	if hasExplicitPoints {
		summary = append(summary, pluginapi.QuotaMetric{
			Key:    "points_remain",
			Label:  "Available Points",
			Value:  remainPoints,
			Unit:   "pts",
			Format: "number",
		})
		if totalPoints > 0 {
			summary = append(summary, pluginapi.QuotaMetric{
				Key:    "points_total",
				Label:  "Total Quota",
				Value:  totalPoints,
				Unit:   "pts",
				Format: "number",
			}, pluginapi.QuotaMetric{
				Key:    "points_used",
				Label:  "Used Quota",
				Value:  usedPoints,
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

	if hasRunsOut {
		summary = append(summary, pluginapi.QuotaMetric{
			Key:    "runs_out_points",
			Label:  "Soonest Expiring Points",
			Value:  runsOutPoints,
			Unit:   "pts",
			Format: "number",
		})
		if expiresAt != "" {
			summary = append(summary, pluginapi.QuotaMetric{
				Key:    "expires_at",
				Label:  "Expiration Date (CST)",
				Value:  0,
				Unit:   expiresAt,
				Format: "text",
			})
		}
		if runsOutAt != "" {
			summary = append(summary, pluginapi.QuotaMetric{
				Key:    "runs_out_at",
				Label:  "Expiration Timestamp (UTC)",
				Value:  0,
				Unit:   runsOutAt,
				Format: "text",
			})
		}
	}

	// Report the real credit multipliers the catalogue publishes for this realm
	summary = append(summary, workbuddyCreditMetrics(realm)...)

	bucketResetTime := nextMidnight
	if hasRunsOut && runsOutAt != "" {
		bucketResetTime = runsOutAt
	}

	quotaBucket := pluginapi.QuotaBucket{
		Window:            "daily",
		RemainingFraction: remainingFrac,
		ResetTime:         bucketResetTime,
		Description:       "Tencent WorkBuddy / CodeBuddy Quota Bucket",
	}
	if hasRunsOut && expiresAt != "" {
		quotaBucket.Description = fmt.Sprintf("Tencent WorkBuddy / CodeBuddy (Expires at %s CST)", expiresAt)
	}

	return pluginapi.QuotaFetchResponse{
		Subscription: &pluginapi.QuotaSubscription{
			Plan: planName,
		},
		Groups: []pluginapi.QuotaGroup{
			{
				DisplayName: "WorkBuddy Resource Quota",
				Buckets: []pluginapi.QuotaBucket{
					quotaBucket,
				},
			},
		},
		Summary: summary,
	}, nil
}

// workbuddyCreditMetrics summarizes the realm's published credit multipliers.
func workbuddyCreditMetrics(realm string) []pluginapi.QuotaMetric {
	summary := registry.GetWorkBuddyCreditsSummary(realm)
	if !summary.HasRates {
		return nil
	}
	metrics := []pluginapi.QuotaMetric{
		{
			Key:    "credits_free_count",
			Label:  "Free Models (0x)",
			Value:  float64(summary.FreeCount),
			Unit:   "models",
			Format: "number",
		},
	}
	if summary.PaidCount > 0 {
		metrics = append(metrics, pluginapi.QuotaMetric{
			Key:    "credits_paid_count",
			Label:  "Paid Models",
			Value:  float64(summary.PaidCount),
			Unit:   "models",
			Format: "number",
		}, pluginapi.QuotaMetric{
			Key:    "credits_rate_min",
			Label:  "Lowest Credit Rate",
			Value:  summary.MinMultiplier,
			Unit:   "x",
			Format: "number",
		}, pluginapi.QuotaMetric{
			Key:    "credits_rate_max",
			Label:  "Highest Credit Rate",
			Value:  summary.MaxMultiplier,
			Unit:   "x",
			Format: "number",
		})
	}
	return metrics
}

func extractAccessToken(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	keys := []string{"access_token", "token", "api_key", "personal_token"}
	if auth.Metadata != nil {
		for _, k := range keys {
			if v, ok := auth.Metadata[k]; ok {
				if t, ok := v.(string); ok && strings.TrimSpace(t) != "" {
					t = strings.TrimSpace(t)
					t = strings.TrimPrefix(t, "Bearer ")
					t = strings.TrimPrefix(t, "bearer ")
					return strings.TrimSpace(t)
				}
			}
		}
	}
	if auth.Attributes != nil {
		for _, k := range keys {
			if t := strings.TrimSpace(auth.Attributes[k]); t != "" {
				t = strings.TrimPrefix(t, "Bearer ")
				t = strings.TrimPrefix(t, "bearer ")
				return strings.TrimSpace(t)
			}
		}
	}
	return ""
}

// ExtractNickname retrieves the display name, email, or user ID from an auth object.
func ExtractNickname(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	keys := []string{"nickname", "nick_name", "username", "user_name", "name", "email", "user_id"}
	if auth.Metadata != nil {
		for _, k := range keys {
			if v, ok := auth.Metadata[k].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	if auth.Attributes != nil {
		for _, k := range keys {
			if v := strings.TrimSpace(auth.Attributes[k]); v != "" {
				return v
			}
		}
	}
	if strings.TrimSpace(auth.Label) != "" {
		return strings.TrimSpace(auth.Label)
	}
	return ""
}

func extractNickname(auth *cliproxyauth.Auth) string {
	return ExtractNickname(auth)
}
