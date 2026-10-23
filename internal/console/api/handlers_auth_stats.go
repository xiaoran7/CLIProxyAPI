package api

import (
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// AuthStatQuotaInfo describes 5-hour or weekly quota limit for a credential.
type AuthStatQuotaInfo struct {
	Name             string   `json:"name"`
	GroupName        string   `json:"group_name"`
	RemainingPercent float64  `json:"remaining_percent"`
	UsedPercent      *float64 `json:"used_percent,omitempty"`
	ResetTime        string   `json:"reset_time,omitempty"`
	ResetCountdown   string   `json:"reset_countdown,omitempty"`
}

// AuthStatPeriodStats holds segmented token and request counts (5h, 24h, all).
type AuthStatPeriodStats struct {
	Tokens5h    int64 `json:"tokens_5h"`
	Tokens24h   int64 `json:"tokens_24h"`
	TokensAll   int64 `json:"tokens_all"`
	Requests5h  int64 `json:"requests_5h"`
	Requests24h int64 `json:"requests_24h"`
	RequestsAll int64 `json:"requests_all"`
}

// AuthStatResponseItem represents telemetry and quota status for an upstream credential.
type AuthStatResponseItem struct {
	AuthIndex         string              `json:"auth_index"`
	Name              string              `json:"name"`
	DisplayName       string              `json:"display_name"`
	Nickname          string              `json:"nickname"`
	Email             string              `json:"email"`
	Provider          string              `json:"provider"`
	ProviderLabel     string              `json:"provider_label"`
	Priority          int                 `json:"priority"`
	Status            string              `json:"status"`
	Disabled          bool                `json:"disabled"`
	IsDeleted         bool                `json:"is_deleted"`
	IsPlaceholderName bool                `json:"is_placeholder_name"`
	RuntimeSource     string              `json:"runtime_source"`
	LastModel         string              `json:"last_model"`
	TotalRequests     int64               `json:"total_requests"`
	Requests          int64               `json:"requests"`
	SuccessCount      int64               `json:"success_count"`
	FailureCount      int64               `json:"failure_count"`
	SuccessRate       float64             `json:"success_rate"`
	PromptTokens      int64               `json:"prompt_tokens"`
	CompletionTokens  int64               `json:"completion_tokens"`
	CachedTokens      int64               `json:"cached_tokens"`
	TotalTokens       int64               `json:"total_tokens"`
	CacheRate         float64             `json:"cache_rate"`
	Quota5h           *AuthStatQuotaInfo  `json:"quota_5h,omitempty"`
	QuotaWeekly       *AuthStatQuotaInfo  `json:"quota_weekly,omitempty"`
	PeriodStats       AuthStatPeriodStats `json:"period_stats"`
	TotalCredits      *float64            `json:"total_credits,omitempty"`
	RemainingCredits  *float64            `json:"remaining_credits,omitempty"`
	UsedCredits       *float64            `json:"used_credits,omitempty"`
	ExpiresAt         string              `json:"expires_at,omitempty"`
	ResetTime         string              `json:"reset_time,omitempty"`
	ResetCountdown    string              `json:"reset_countdown,omitempty"`
	TodayTokens       int64               `json:"today_tokens"`
	TodayRequests     int64               `json:"today_requests"`
	DailyLimit        *int64              `json:"daily_limit,omitempty"`
	RemainingRequests *int64              `json:"remaining_requests,omitempty"`
}

// HandleGetUpstreamAuthStats aggregates credentials statistics with optional time range filtering.
func HandleGetUpstreamAuthStats(eng *engine.Engine, cpa *CpaClient, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		database := eng.DB()

		rangeParam := strings.ToLower(strings.TrimSpace(c.Query("range")))
		if rangeParam == "" {
			rangeParam = strings.ToLower(strings.TrimSpace(c.Query("time_range")))
		}
		if rangeParam == "" {
			rangeParam = "all"
		}

		now := time.Now()

		// 1. Fetch active accounts from CPA and SQLite (fallback)
		var activeAccounts []UpstreamAccountItem
		if quotasResp, err := FetchUpstreamAccounts(ctx, eng, cpa, providers, false); err == nil && quotasResp != nil {
			activeAccounts = quotasResp.Accounts
		}

		activeMap := make(map[string]UpstreamAccountItem)
		for _, acc := range activeAccounts {
			if acc.AuthIndex != "" {
				activeMap[acc.AuthIndex] = acc
			}
			if acc.Filename != "" && acc.Filename != acc.AuthIndex {
				activeMap[acc.Filename] = acc
			}
			if acc.ID != "" && acc.ID != acc.AuthIndex && acc.ID != acc.Filename {
				activeMap[acc.ID] = acc
			}
		}

		// 2. Fetch baselines from SQLite
		baselines, err := database.SelectBaselinesAll(ctx)
		if err != nil {
			baselines = []db.UpstreamAuthBaselineRow{}
		}
		baseMap := make(map[string]db.UpstreamAuthBaselineRow)
		for _, b := range baselines {
			baseMap[b.AuthIndex] = b
		}

		// 3. Fetch period stats (5h and 24h) from request_events
		since5h := now.Add(-5 * time.Hour).Unix()
		win5h, _ := database.SelectUpstreamWindowStats(ctx, since5h)
		since24h := now.Add(-24 * time.Hour).Unix()
		win24h, _ := database.SelectUpstreamWindowStats(ctx, since24h)

		// 4. Fetch latest model per auth_index
		latestModels, _ := database.SelectLatestModelByAuth(ctx)

		// 5. Query traffic metrics based on rangeParam
		type trafficMetrics struct {
			requests         int64
			successCount     int64
			failureCount     int64
			promptTokens     int64
			completionTokens int64
			cachedTokens     int64
			totalTokens      int64
		}
		trafficMap := make(map[string]trafficMetrics)
		isAll := rangeParam == "all"
		runtimeSource := "cpa_runtime"

		if rangeParam == "5h" {
			if evs, err := database.SelectUpstreamEventAggSince(ctx, since5h); err == nil {
				for _, ev := range evs {
					trafficMap[ev.AuthIndex] = trafficMetrics{
						requests:         ev.Requests,
						successCount:     ev.SuccessCount,
						failureCount:     ev.FailureCount,
						promptTokens:     ev.PromptTokens,
						completionTokens: ev.CompletionTokens,
						cachedTokens:     ev.CachedTokens,
						totalTokens:      ev.TotalTokens,
					}
				}
			}
		} else if rangeParam == "24h" || rangeParam == "1d" || rangeParam == "today" {
			if evs, err := database.SelectUpstreamEventAggSince(ctx, since24h); err == nil {
				for _, ev := range evs {
					trafficMap[ev.AuthIndex] = trafficMetrics{
						requests:         ev.Requests,
						successCount:     ev.SuccessCount,
						failureCount:     ev.FailureCount,
						promptTokens:     ev.PromptTokens,
						completionTokens: ev.CompletionTokens,
						cachedTokens:     ev.CachedTokens,
						totalTokens:      ev.TotalTokens,
					}
				}
			}
		} else if rangeParam == "7d" {
			sinceDate := now.AddDate(0, 0, -7).Format("2006-01-02")
			if dailyAgg, err := database.SelectUpstreamDailyAggSince(ctx, sinceDate); err == nil {
				for _, d := range dailyAgg {
					trafficMap[d.AuthIndex] = trafficMetrics{
						requests:         d.Requests,
						successCount:     d.SuccessCount,
						failureCount:     d.FailureCount,
						promptTokens:     d.PromptTokens,
						completionTokens: d.CompletionTokens,
						cachedTokens:     d.CachedTokens,
						totalTokens:      d.TotalTokens,
					}
				}
			}
		} else if rangeParam == "30d" {
			sinceDate := now.AddDate(0, 0, -30).Format("2006-01-02")
			if dailyAgg, err := database.SelectUpstreamDailyAggSince(ctx, sinceDate); err == nil {
				for _, d := range dailyAgg {
					trafficMap[d.AuthIndex] = trafficMetrics{
						requests:         d.Requests,
						successCount:     d.SuccessCount,
						failureCount:     d.FailureCount,
						promptTokens:     d.PromptTokens,
						completionTokens: d.CompletionTokens,
						cachedTokens:     d.CachedTokens,
						totalTokens:      d.TotalTokens,
					}
				}
			}
		} else { // "all"
			runtimeSource = "local_estimate"
			if dailyAgg, err := database.SelectUpstreamDailyAgg(ctx); err == nil {
				for _, d := range dailyAgg {
					trafficMap[d.AuthIndex] = trafficMetrics{
						requests:         d.Requests,
						successCount:     d.SuccessCount,
						failureCount:     d.FailureCount,
						promptTokens:     d.PromptTokens,
						completionTokens: d.CompletionTokens,
						cachedTokens:     d.CachedTokens,
						totalTokens:      d.TotalTokens,
					}
				}
			}
		}

		// 6. Gather all unique auth_index keys
		allIndexesSet := make(map[string]struct{})
		for _, acc := range activeAccounts {
			if acc.AuthIndex != "" {
				allIndexesSet[acc.AuthIndex] = struct{}{}
			}
		}
		for idx := range baseMap {
			if idx != "" {
				allIndexesSet[idx] = struct{}{}
			}
		}
		for idx := range trafficMap {
			if idx != "" {
				allIndexesSet[idx] = struct{}{}
			}
		}
		for idx := range win5h {
			if idx != "" {
				allIndexesSet[idx] = struct{}{}
			}
		}
		for idx := range win24h {
			if idx != "" {
				allIndexesSet[idx] = struct{}{}
			}
		}

		hasUpstreamCatalog := len(activeAccounts) > 0
		var items []AuthStatResponseItem

		for authIndex := range allIndexesSet {
			var acc *UpstreamAccountItem
			if a, ok := activeMap[authIndex]; ok {
				acc = &a
			} else {
				for _, a := range activeAccounts {
					if a.AuthIndex == authIndex || a.Filename == authIndex || a.ID == authIndex {
						acc = &a
						break
					}
				}
			}

			base, hasBase := baseMap[authIndex]
			traf := trafficMap[authIndex]

			isDeleted := false
			if hasUpstreamCatalog {
				isDeleted = (acc == nil)
			} else {
				isDeleted = (acc == nil && !hasBase)
			}

			displayName := ""
			nickname := ""
			email := ""
			provider := ""
			priority := 10
			status := "active"
			disabled := false

			if acc != nil {
				displayName = acc.DisplayName
				if displayName == "" {
					displayName = acc.Filename
				}
				nickname = acc.Nickname
				email = acc.Email
				provider = acc.Provider
				priority = acc.Priority
				status = acc.Status
				disabled = acc.Disabled
			} else if hasBase {
				if base.Name != nil && *base.Name != "" {
					displayName = *base.Name
				}
				if base.Provider != nil && *base.Provider != "" {
					provider = *base.Provider
				}
			}

			if acc == nil {
				if regAcc, ok := providers.GetAuthAccount(authIndex); ok {
					if displayName == "" && regAcc.Name != "" {
						displayName = regAcc.Name
					}
					if provider == "" && regAcc.ProviderKey != "" {
						provider = regAcc.ProviderKey
					}
				}
			}

			if provider == "" {
				provRes := providers.InferProvider("", authIndex)
				if provRes.Key != "" {
					provider = provRes.Key
				} else if lm, ok := latestModels[authIndex]; ok && lm != "" {
					provResModel := providers.InferProvider(lm, "")
					if provResModel.Key != "" {
						provider = provResModel.Key
					}
				}
			}
			if provider == "" {
				provider = "other"
			}

			provLabel := providers.GetProviderLabel(provider)
			if provLabel == "" {
				provLabel = UnknownProviderLabel
			}

			isPlaceholderName := false
			if displayName == "" {
				isPlaceholderName = true
				if len(authIndex) >= 8 {
					displayName = "已删除账号 " + authIndex[:8]
				} else {
					displayName = "已删除账号 " + authIndex
				}
			} else if strings.HasPrefix(displayName, "已删除账号") || displayName == authIndex {
				isPlaceholderName = true
			}

			// Compute request and token counts
			var totalReqs, succCount, failCount, pTokens, cTokens, cachedToks, totTokens int64
			if isAll {
				if hasBase {
					totalReqs += base.BaseRequests
					succCount += base.BaseSuccessCount
					failCount += base.BaseFailureCount
					pTokens += base.BasePromptTokens
					cTokens += base.BaseCompletionTokens
					cachedToks += base.BaseCachedTokens
					totTokens += base.BaseTotalTokens
				}
				totalReqs += traf.requests
				succCount += traf.successCount
				failCount += traf.failureCount
				pTokens += traf.promptTokens
				cTokens += traf.completionTokens
				cachedToks += traf.cachedTokens
				totTokens += traf.totalTokens
			} else {
				totalReqs = traf.requests
				succCount = traf.successCount
				failCount = traf.failureCount
				pTokens = traf.promptTokens
				cTokens = traf.completionTokens
				cachedToks = traf.cachedTokens
				totTokens = traf.totalTokens
			}

			successRate := 100.0
			if totalReqs > 0 {
				successRate = math.Round((float64(succCount)/float64(totalReqs))*10000) / 100
			}

			promptBase := pTokens
			if cachedToks > promptBase {
				promptBase = cachedToks
			}
			cacheRate := 0.0
			if promptBase > 0 {
				cacheRate = math.Round((float64(cachedToks)/float64(promptBase))*10000) / 100
			}

			// Extract Quota 5h and Quota Weekly from acc.Groups
			var quota5h *AuthStatQuotaInfo
			var quotaWeekly *AuthStatQuotaInfo
			var totalCredits, remainingCredits, usedCredits *float64
			var expiresAt string
			var resetTimeStr, resetCountdownStr string

			if acc != nil {
				for _, grp := range acc.Groups {
					gNameLower := strings.ToLower(grp.DisplayName + " " + grp.Name)
					isClaudeGroup := strings.Contains(gNameLower, "claude") || strings.Contains(gNameLower, "gpt")
					for _, bkt := range grp.Buckets {
						wLower := strings.ToLower(bkt.Window + " " + bkt.Name + " " + bkt.BucketID)
						is5h := bkt.Window == "5h" || strings.Contains(wLower, "5h") || strings.Contains(wLower, "five")
						isWeek := bkt.Window == "weekly" || strings.Contains(wLower, "week")

						if is5h && bkt.RemainingPercent != nil {
							remPct := float64(*bkt.RemainingPercent)
							bktName := bkt.Name
							if bktName == "" {
								if isClaudeGroup {
									bktName = "Claude 5 小时周期"
								} else {
									bktName = "5 小时周期"
								}
							}
							if quota5h == nil || remPct < quota5h.RemainingPercent || (remPct == quota5h.RemainingPercent && isClaudeGroup) {
								quota5h = &AuthStatQuotaInfo{
									Name:             bktName,
									GroupName:        grp.DisplayName,
									RemainingPercent: remPct,
									UsedPercent:      bkt.UsedPercent,
									ResetTime:        bkt.ResetTime,
									ResetCountdown:   bkt.ResetCountdown,
								}
							}
						}

						if isWeek && bkt.RemainingPercent != nil {
							remPct := float64(*bkt.RemainingPercent)
							bktName := bkt.Name
							if bktName == "" {
								if isClaudeGroup {
									bktName = "Claude 周限额"
								} else {
									bktName = "周限额"
								}
							}
							if quotaWeekly == nil || remPct < quotaWeekly.RemainingPercent || (remPct == quotaWeekly.RemainingPercent && isClaudeGroup) {
								quotaWeekly = &AuthStatQuotaInfo{
									Name:             bktName,
									GroupName:        grp.DisplayName,
									RemainingPercent: remPct,
									UsedPercent:      bkt.UsedPercent,
									ResetTime:        bkt.ResetTime,
									ResetCountdown:   bkt.ResetCountdown,
								}
							}
						}

						// Accumulate credits for WorkBuddy
						if bkt.TotalCapacity != nil || bkt.RemainCapacity != nil {
							if totalCredits == nil {
								zero := 0.0
								totalCredits = &zero
								remainingCredits = &zero
								usedCredits = &zero
							}
							if bkt.TotalCapacity != nil {
								*totalCredits += *bkt.TotalCapacity
							}
							if bkt.RemainCapacity != nil {
								*remainingCredits += *bkt.RemainCapacity
							}
							if bkt.UsedCapacity != nil {
								*usedCredits += *bkt.UsedCapacity
							}
						}

						if bkt.ExpiresAt != "" && expiresAt == "" {
							expiresAt = bkt.ExpiresAt
						}
						if bkt.ResetTime != "" && resetTimeStr == "" {
							resetTimeStr = bkt.ResetTime
							resetCountdownStr = bkt.ResetCountdown
						}
					}
				}
			}

			// Period stats
			w5 := win5h[authIndex]
			w24 := win24h[authIndex]
			periodStats := AuthStatPeriodStats{
				Tokens5h:    w5.Tokens,
				Requests5h:  w5.Requests,
				Tokens24h:   w24.Tokens,
				Requests24h: w24.Requests,
				TokensAll:   totTokens,
				RequestsAll: totalReqs,
			}

			// OpenRouter & OpenCode metrics
			var todayToks, todayReqs int64
			var dailyLim, remReqs *int64
			if acc != nil {
				todayToks = acc.TodayTokens
				todayReqs = acc.TodayRequests
			}
			if strings.Contains(strings.ToLower(provider), "openrouter") {
				lim := int64(800)
				dailyLim = &lim
				rem := lim - todayReqs
				if rem < 0 {
					rem = 0
				}
				remReqs = &rem
			}

			lastModel := latestModels[authIndex]

			items = append(items, AuthStatResponseItem{
				AuthIndex:         authIndex,
				Name:              displayName,
				DisplayName:       displayName,
				Nickname:          nickname,
				Email:             email,
				Provider:          provider,
				ProviderLabel:     provLabel,
				Priority:          priority,
				Status:            status,
				Disabled:          disabled,
				IsDeleted:         isDeleted,
				IsPlaceholderName: isPlaceholderName,
				RuntimeSource:     runtimeSource,
				LastModel:         lastModel,
				TotalRequests:     totalReqs,
				Requests:          totalReqs,
				SuccessCount:      succCount,
				FailureCount:      failCount,
				SuccessRate:       successRate,
				PromptTokens:      pTokens,
				CompletionTokens:  cTokens,
				CachedTokens:      cachedToks,
				TotalTokens:       totTokens,
				CacheRate:         cacheRate,
				Quota5h:           quota5h,
				QuotaWeekly:       quotaWeekly,
				PeriodStats:       periodStats,
				TotalCredits:      totalCredits,
				RemainingCredits:  remainingCredits,
				UsedCredits:       usedCredits,
				ExpiresAt:         expiresAt,
				ResetTime:         resetTimeStr,
				ResetCountdown:    resetCountdownStr,
				TodayTokens:       todayToks,
				TodayRequests:     todayReqs,
				DailyLimit:        dailyLim,
				RemainingRequests: remReqs,
			})
		}

		// Sort items: Active first (by Priority DESC, then TotalRequests DESC, then TotalTokens DESC),
		// Deleted items sorted by TotalRequests DESC
		sort.Slice(items, func(i, j int) bool {
			if items[i].IsDeleted != items[j].IsDeleted {
				return !items[i].IsDeleted // active items first
			}
			if !items[i].IsDeleted {
				if items[i].Priority != items[j].Priority {
					return items[i].Priority > items[j].Priority
				}
			}
			if items[i].TotalRequests != items[j].TotalRequests {
				return items[i].TotalRequests > items[j].TotalRequests
			}
			return items[i].TotalTokens > items[j].TotalTokens
		})

		// Summary calculations
		var sumRequests, sumSuccess, sumFailure, sumPrompt, sumCompletion, sumCached, sumTokens int64
		for _, it := range items {
			sumRequests += it.TotalRequests
			sumSuccess += it.SuccessCount
			sumFailure += it.FailureCount
			sumPrompt += it.PromptTokens
			sumCompletion += it.CompletionTokens
			sumCached += it.CachedTokens
			sumTokens += it.TotalTokens
		}

		overallPromptBase := sumPrompt
		if sumCached > overallPromptBase {
			overallPromptBase = sumCached
		}
		overallCacheRate := 0.0
		if overallPromptBase > 0 {
			overallCacheRate = math.Round((float64(sumCached)/float64(overallPromptBase))*10000) / 100
		}
		overallSuccessRate := 100.0
		if sumRequests > 0 {
			overallSuccessRate = math.Round((float64(sumSuccess)/float64(sumRequests))*10000) / 100
		}

		summary := gin.H{
			"total_accounts":    len(items),
			"total_requests":    sumRequests,
			"total_success":     sumSuccess,
			"total_failure":     sumFailure,
			"success_rate":      overallSuccessRate,
			"prompt_tokens":     sumPrompt,
			"completion_tokens": sumCompletion,
			"cached_tokens":     sumCached,
			"total_tokens":      sumTokens,
			"cache_rate":        overallCacheRate,
			"request_source":    runtimeSource,
			"token_source":      "local_metered",
		}

		c.JSON(http.StatusOK, gin.H{
			"time_range": rangeParam,
			"items":      items,
			"auth_stats": items,
			"total":      len(items),
			"summary":    summary,
			"source": gin.H{
				"requests": runtimeSource,
				"tokens":   "local_metered",
			},
			"updated_at": now.UnixMilli(),
		})
	}
}
