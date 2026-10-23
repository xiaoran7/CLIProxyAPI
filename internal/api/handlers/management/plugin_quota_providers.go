package management

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/auth/qoder"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/auth/workbuddy"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

func (h *Handler) listWorkBuddyQuotas(c *gin.Context) {
	if h.authManager == nil {
		c.JSON(http.StatusOK, gin.H{"accounts": []any{}})
		return
	}

	auths := h.authManager.List()
	type accountRateRange struct {
		FreeCount int     `json:"free_count"`
		PaidCount int     `json:"paid_count"`
		Min       float64 `json:"min"`
		Max       float64 `json:"max"`
	}
	type accountQuota struct {
		AuthID        string  `json:"auth_id"`
		User          string  `json:"user"`
		Points        float64 `json:"points"`
		Balance       float64 `json:"balance"`
		TotalPoints   float64 `json:"total_points,omitempty"`
		UsedPoints    float64 `json:"used_points,omitempty"`
		ExpiresAt     string  `json:"expires_at,omitempty"`
		RunsOutAt     string  `json:"runs_out_at,omitempty"`
		RunsOutPoints float64 `json:"runs_out_points,omitempty"`
		VIPLevel      string  `json:"vip_level"`
		FreeTier      bool    `json:"free_tier"`
		Realm         string  `json:"realm"`
		// Multipliers describes the credit rates published for this realm, so
		// the panel can show what a call actually costs. It is null when the
		// catalogue is unavailable, which must read as "unknown", not "free".
		Multipliers *accountRateRange `json:"multipliers,omitempty"`
		// FreeModels and PaidModels list the free / paid model IDs so a client
		// can explain the multiplier range without a second request.
		FreeModels []string           `json:"free_models,omitempty"`
		PaidModels []string           `json:"paid_models,omitempty"`
		ModelRates map[string]float64 `json:"model_rates,omitempty"`
	}

	var targets []*coreauth.Auth
	for _, a := range auths {
		if a == nil || isInternalStorageFile(a.ID) || isInternalStorageFile(a.FileName) || strings.HasSuffix(strings.ToLower(a.ID), "checkins.json") || a.ID == "enabled" || a.ID == "records" {
			continue
		}
		path := strings.TrimSpace(authAttribute(a, "path"))
		if path != "" && isInternalStorageFile(path) {
			continue
		}
		provider := strings.ToLower(strings.TrimSpace(a.Provider))
		authKind := strings.ToLower(strings.TrimSpace(a.Attributes["auth_kind"]))
		isWorkbuddy := provider == "workbuddy" || provider == "workbuddy-cn" || authKind == "workbuddy" || provider == "copilot-tencent"
		if isWorkbuddy {
			targets = append(targets, a)
		}
	}

	if len(targets) == 0 {
		c.JSON(http.StatusOK, gin.H{"accounts": []accountQuota{}})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	results := make([]accountQuota, len(targets))
	var wg sync.WaitGroup
	for idx, a := range targets {
		wg.Add(1)
		go func(i int, auth *coreauth.Auth) {
			defer wg.Done()
			realm := workbuddy.ResolveRealm(auth)
			user := workbuddy.ExtractNickname(auth)
			if user == "" {
				user = auth.ID
			}
			aq := accountQuota{
				AuthID:   auth.ID,
				User:     user,
				VIPLevel: "Standard",
				Realm:    realm,
			}

			// Report the credit multipliers actually published for the realm's
			// catalogue. free_tier is derived from those rates instead of being
			// asserted unconditionally; when the catalogue is unknown it stays
			// false and Multipliers is null.
			credits := registry.GetWorkBuddyCreditsSummary(realm)
			if credits.HasRates {
				aq.Multipliers = &accountRateRange{
					FreeCount: credits.FreeCount,
					PaidCount: credits.PaidCount,
					Min:       credits.MinMultiplier,
					Max:       credits.MaxMultiplier,
				}
				aq.FreeTier = credits.FreeCount > 0
				aq.ModelRates = make(map[string]float64)
				for id, rate := range registry.WorkBuddyCreditsModelRates(realm) {
					if rate.Free {
						aq.FreeModels = append(aq.FreeModels, id)
						aq.ModelRates[id] = 0.0
						continue
					}
					aq.PaidModels = append(aq.PaidModels, id)
					aq.ModelRates[id] = rate.Multiplier
				}
				sort.Strings(aq.FreeModels)
				sort.Strings(aq.PaidModels)
			}

			quotaResp, errQuota := workbuddy.QueryQuota(ctx, nil, auth)
			if errQuota == nil {
				if quotaResp.Subscription != nil && quotaResp.Subscription.Plan != "" {
					aq.VIPLevel = quotaResp.Subscription.Plan
				}
				for _, g := range quotaResp.Groups {
					for _, b := range g.Buckets {
						if b.ResetTime != "" && aq.RunsOutAt == "" {
							aq.RunsOutAt = b.ResetTime
						}
					}
				}
				for _, m := range quotaResp.Summary {
					switch m.Key {
					case "points_remain":
						aq.Points = m.Value
						aq.Balance = m.Value
					case "points_total":
						aq.TotalPoints = m.Value
					case "points_used":
						aq.UsedPoints = m.Value
					case "runs_out_points":
						aq.RunsOutPoints = m.Value
					case "expires_at":
						aq.ExpiresAt = m.Unit
					case "runs_out_at":
						aq.RunsOutAt = m.Unit
					}
				}
			}
			results[i] = aq
		}(idx, a)
	}
	wg.Wait()

	c.JSON(http.StatusOK, gin.H{
		"accounts": results,
	})
}

func (h *Handler) listQoderQuotas(c *gin.Context) {
	if h.authManager == nil {
		c.JSON(http.StatusOK, gin.H{"accounts": []any{}})
		return
	}

	auths := h.authManager.List()
	type accountQuota struct {
		AuthID      string  `json:"auth_id"`
		User        string  `json:"user"`
		Points      float64 `json:"points"`
		Balance     float64 `json:"balance"`
		TotalPoints float64 `json:"total_points,omitempty"`
		UsedPoints  float64 `json:"used_points,omitempty"`
		ExpiresAt   string  `json:"expires_at,omitempty"`
		VIPLevel    string  `json:"vip_level"`
		FreeTier    bool    `json:"free_tier"`
		Realm       string  `json:"realm"`
	}

	var targets []*coreauth.Auth
	for _, a := range auths {
		if a == nil || isInternalStorageFile(a.ID) || isInternalStorageFile(a.FileName) || strings.HasSuffix(strings.ToLower(a.ID), "checkins.json") || a.ID == "enabled" || a.ID == "records" {
			continue
		}
		path := strings.TrimSpace(authAttribute(a, "path"))
		if path != "" && isInternalStorageFile(path) {
			continue
		}
		provider := strings.ToLower(strings.TrimSpace(a.Provider))
		authKind := strings.ToLower(strings.TrimSpace(a.Attributes["auth_kind"]))
		isQoder := provider == "qoder" || provider == "qoder-cn" || provider == "qoder-intl" || provider == "qoder-en" || authKind == "qoder"
		if isQoder {
			targets = append(targets, a)
		}
	}

	if len(targets) == 0 {
		c.JSON(http.StatusOK, gin.H{"accounts": []accountQuota{}})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	results := make([]accountQuota, len(targets))
	var wg sync.WaitGroup
	for idx, a := range targets {
		wg.Add(1)
		go func(i int, auth *coreauth.Auth) {
			defer wg.Done()
			realm := qoder.ResolveRealm(auth)
			user := qoder.ResolveUserID(auth)
			if auth.Metadata != nil {
				if nick, ok := auth.Metadata["nickname"].(string); ok && strings.TrimSpace(nick) != "" {
					user = strings.TrimSpace(nick)
				}
			}
			if user == "" {
				user = auth.ID
			}

			aq := accountQuota{
				AuthID:   auth.ID,
				User:     user,
				VIPLevel: "Standard",
				Realm:    realm,
				FreeTier: true,
			}

			client := &http.Client{Timeout: 10 * time.Second}
			quotaResp, err := qoder.QueryQuota(ctx, client, auth)
			if err == nil {
				if quotaResp.Subscription != nil && quotaResp.Subscription.Plan != "" {
					aq.VIPLevel = quotaResp.Subscription.Plan
					if strings.Contains(strings.ToLower(quotaResp.Subscription.Plan), "pro") || strings.Contains(strings.ToLower(quotaResp.Subscription.Plan), "vip") {
						aq.FreeTier = false
					}
				}
				for _, metric := range quotaResp.Summary {
					switch metric.Key {
					case "points_remain":
						aq.Points = metric.Value
						aq.Balance = metric.Value
					case "points_total":
						aq.TotalPoints = metric.Value
					case "points_used":
						aq.UsedPoints = metric.Value
					}
				}
				if len(quotaResp.Groups) > 0 && len(quotaResp.Groups[0].Buckets) > 0 {
					aq.ExpiresAt = quotaResp.Groups[0].Buckets[0].ResetTime
				}
			} else {
				log.WithError(err).Warnf("failed to query qoder quota for auth %s", auth.ID)
			}
			results[i] = aq
		}(idx, a)
	}
	wg.Wait()

	c.JSON(http.StatusOK, gin.H{
		"accounts": results,
	})
}
