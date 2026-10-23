package zcode

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// QueryQuota reads a ZCode credential's allowance in the gateway's normalized
// quota shape.
//
// A team seat reports the team plan's windows, a credential with a coding plan
// key reports the plan's windows plus the Start Plan's buckets when the account
// also holds the gift, and a Start-Plan-only account reports its buckets alone.
func QueryQuota(ctx context.Context, httpClient *http.Client, auth *cliproxyauth.Auth) (pluginapi.QuotaFetchResponse, error) {
	if auth == nil {
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("zcode: auth is nil")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	site := ResolveSite(auth)
	key := AccessKey(auth)
	jwt := StartToken(auth)

	var (
		usage *Usage
		err   error
	)
	switch {
	case IsTeamSeat(auth):
		usage, err = QueryTeamPlan(ctx, httpClient, ResolveBaseURL(auth), site, key, SiteToken(auth), TeamOrg(auth), TeamProject(auth))
	case key != "":
		usage, err = QueryCodingPlan(ctx, httpClient, ResolveBaseURL(auth), key)
	default:
		usage, err = QueryStartPlan(ctx, httpClient, jwt, DeviceID(auth))
	}
	if err != nil {
		return pluginapi.QuotaFetchResponse{}, err
	}

	// A coding plan account may also hold the gift; its buckets are shown beside
	// the plan's windows so an operator can see both pools.
	if key != "" && jwt != "" && !IsTeamSeat(auth) && !JWTExpired(jwt, nowFunc()) {
		if gift, errGift := QueryStartPlan(ctx, httpClient, jwt, DeviceID(auth)); errGift == nil && gift != nil && len(gift.Windows) > 0 {
			usage.Windows = append(usage.Windows, gift.Windows...)
		}
	}

	if usage.Error != "" && len(usage.Windows) == 0 {
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("zcode: %s", usage.Error)
	}
	return buildQuotaResponse(auth, usage), nil
}

// buildQuotaResponse maps a Usage onto the normalized quota contract.
func buildQuotaResponse(auth *cliproxyauth.Auth, usage *Usage) pluginapi.QuotaFetchResponse {
	plan := strings.TrimSpace(usage.Plan)
	if plan == "" {
		plan = PlanName(auth)
	}
	if plan == "" {
		plan = "ZCode"
	}

	resp := pluginapi.QuotaFetchResponse{
		Subscription: &pluginapi.QuotaSubscription{
			Plan:     plan,
			TierName: plan,
			TierID:   strings.ToLower(strings.ReplaceAll(plan, " ", "-")),
		},
	}

	summary := []pluginapi.QuotaMetric{}
	if !usage.Until.IsZero() {
		summary = append(summary, pluginapi.QuotaMetric{
			Key:    "expires_at",
			Label:  "Plan Ends",
			Value:  float64(usage.Until.Unix()),
			Unit:   usage.Until.Format(time.RFC3339),
			Format: "number",
		})
	}
	if usage.Renew != "" {
		renew := 0.0
		if usage.Renew == "auto" {
			renew = 1
		}
		summary = append(summary, pluginapi.QuotaMetric{
			Key:    "auto_renew",
			Label:  "Auto Renew",
			Value:  renew,
			Format: "number",
		})
	}
	if usage.Resets != nil && usage.Resets.Count > 0 {
		summary = append(summary, pluginapi.QuotaMetric{
			Key:    "resets_available",
			Label:  "Resets Left",
			Value:  float64(usage.Resets.Count),
			Format: "number",
		})
	}
	resp.Summary = summary

	// Coding plan windows and gift buckets are separate pools: a request spends
	// one or the other, never both.
	var planBuckets []pluginapi.QuotaBucket
	var giftBuckets []pluginapi.QuotaBucket
	for _, window := range usage.Windows {
		if window.Aside {
			continue
		}
		bucket := pluginapi.QuotaBucket{
			Window:            window.Name,
			RemainingFraction: remainingFraction(window.UsedPercent),
			Description:       windowLabel(window),
		}
		if !window.ResetsAt.IsZero() {
			bucket.ResetTime = window.ResetsAt.UTC().Format(time.RFC3339)
		}
		if window.Gift {
			giftBuckets = append(giftBuckets, bucket)
			continue
		}
		planBuckets = append(planBuckets, bucket)
	}

	planLabel := plan
	if IsTeamSeat(auth) {
		planLabel = plan + " (Team)"
	}
	if len(planBuckets) > 0 {
		resp.Groups = append(resp.Groups, pluginapi.QuotaGroup{
			DisplayName: planLabel,
			Buckets:     planBuckets,
		})
	}
	if len(giftBuckets) > 0 {
		// A gift-only account's plan is the Start Plan itself; an account that
		// holds both pools names the gift separately so the two are told apart.
		giftLabel := "ZCode Start Plan"
		if len(planBuckets) == 0 {
			giftLabel = planLabel
		}
		resp.Groups = append(resp.Groups, pluginapi.QuotaGroup{
			DisplayName: giftLabel,
			Buckets:     giftBuckets,
		})
	}

	// A reset allowance is worth showing even when no window could be read.
	if usage.Resets != nil && usage.Resets.Count > 0 && len(resp.Groups) == 0 {
		bucket := pluginapi.QuotaBucket{
			Window:            "Resets",
			RemainingFraction: 1,
			Description:       fmt.Sprintf("%d resets left", usage.Resets.Count),
		}
		if !usage.Resets.Until.IsZero() {
			bucket.ResetTime = usage.Resets.Until.UTC().Format(time.RFC3339)
		}
		resp.Groups = append(resp.Groups, pluginapi.QuotaGroup{
			DisplayName: planLabel,
			Buckets:     []pluginapi.QuotaBucket{bucket},
		})
	}

	if len(resp.Groups) == 0 {
		resp.Groups = []pluginapi.QuotaGroup{{
			DisplayName: planLabel,
			Buckets: []pluginapi.QuotaBucket{{
				Window:            "Allowance",
				RemainingFraction: 1,
				Description:       "no window reported by upstream",
			}},
		}}
	}
	return resp
}

// remainingFraction turns a used percentage into the contract's remaining share.
func remainingFraction(usedPercent float64) float64 {
	remaining := 1 - usedPercent/100
	if remaining < 0 {
		return 0
	}
	if remaining > 1 {
		return 1
	}
	return remaining
}

// windowLabel renders a window's own count beside its name, when it has one.
func windowLabel(window Window) string {
	label := window.Name
	if window.HasCount {
		label = fmt.Sprintf("%s · %s", window.Name, window.Display)
	}
	if len(window.Models) > 0 {
		models := append([]string(nil), window.Models...)
		sort.Strings(models)
		label = fmt.Sprintf("%s (%s)", label, strings.Join(models, ", "))
	}
	return label
}
