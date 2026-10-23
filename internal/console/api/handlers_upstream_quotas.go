package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// HandleGetUpstreamQuotas returns all upstream credentials and quota status
func HandleGetUpstreamQuotas(eng *engine.Engine, cpa *CpaClient, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		force := c.Query("refresh") == "1" || c.Request.Method == http.MethodPost
		response, err := FetchUpstreamAccounts(ctx, eng, cpa, providers, force)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, response)
	}
}

// isSupportedQuotaProvider returns true if the provider supports quota probing or has quota groups.
func isSupportedQuotaProvider(prov string, f CpaAuthFile) bool {
	p := strings.ToLower(strings.TrimSpace(prov))
	switch p {
	case "antigravity", "codex", "claude", "workbuddy", "workbuddy-cn", "copilot-tencent",
		"qoder", "qoder-cn", "qoder-intl", "qoder-en", "kimi", "xai", "devin", "zcode", "zcode-cn":
		return true
	}
	if strings.HasPrefix(p, "workbuddy") || strings.HasPrefix(p, "qoder") || strings.HasPrefix(p, "zcode") {
		return true
	}
	if f.SupportsQuota || f.QuotaProvider != "" || f.QuotaPluginID != "" {
		return true
	}
	if f.Quota != nil && len(f.Quota.Groups) > 0 {
		return true
	}
	return false
}

// extractAccountCredits aggregates point capacity metrics across quota groups and buckets.
func extractAccountCredits(groups []CpaQuotaGroup) (*float64, *float64, *float64, string) {
	var totCreds, remCreds, usedCreds *float64
	var expAt string
	for _, g := range groups {
		for _, b := range g.Buckets {
			if b.TotalCapacity != nil || b.RemainCapacity != nil || b.UsedCapacity != nil {
				if totCreds == nil {
					z1, z2, z3 := 0.0, 0.0, 0.0
					totCreds, remCreds, usedCreds = &z1, &z2, &z3
				}
				if b.TotalCapacity != nil {
					*totCreds += *b.TotalCapacity
				}
				if b.RemainCapacity != nil {
					*remCreds += *b.RemainCapacity
				}
				if b.UsedCapacity != nil {
					*usedCreds += *b.UsedCapacity
				}
			}
			if b.ExpiresAt != "" && expAt == "" {
				expAt = b.ExpiresAt
			}
		}
	}
	return totCreds, remCreds, usedCreds, expAt
}

// FetchUpstreamAccounts retrieves active and configured upstream accounts with quota groups and cached stats.
func FetchUpstreamAccounts(ctx context.Context, eng *engine.Engine, cpa *CpaClient, providers *ProviderRegistryEngine, force bool) (*UpstreamQuotasResponse, error) {
	if !force {
		upstreamCacheMu.RLock()
		if cachedUpstreamQuotasResp != nil && time.Since(cachedUpstreamAt) < 10*time.Second {
			resp := cachedUpstreamQuotasResp
			upstreamCacheMu.RUnlock()
			return resp, nil
		}
		upstreamCacheMu.RUnlock()
	}

	// 1. Fetch auth files from CPA loopback
	var authFilesResp CpaAuthFilesResponse
	if cpa != nil {
		_ = cpa.RequestJSON(ctx, http.MethodGet, "/auth-files", nil, "v0", &authFilesResp)
	}

	// 2. Fetch openai-compatibility providers from CPA loopback
	var compatResp CpaOpenAICompatResponse
	if cpa != nil {
		_ = cpa.RequestJSON(ctx, http.MethodGet, "/openai-compatibility", nil, "v0", &compatResp)
	}

	// Synchronize credentials to in-memory provider registry and persist baselines to SQLite
	SyncAuthFilesToBaselines(ctx, eng, providers, authFilesResp.Files, compatResp.Compatibility)

	// If refresh was requested, concurrently probe active credentials (with 10s per-credential throttle)
	if force && eng != nil && cpa != nil {
		var wg sync.WaitGroup
		sem := make(chan struct{}, 6)
		for _, f := range authFilesResp.Files {
			prov := strings.ToLower(strings.TrimSpace(f.Provider))
			if prov == "" {
				prov = strings.ToLower(strings.TrimSpace(f.Type))
			}
			if !isSupportedQuotaProvider(prov, f) {
				continue
			}
			if f.Disabled || f.Status == "disabled" {
				continue
			}
			key := f.AuthIndex
			if key == "" {
				key = f.Name
			}
			if !shouldProbeAccount(key, 10*time.Second) {
				continue
			}
			wg.Add(1)
			go func(file CpaAuthFile, p string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				fetchCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer cancel()
				_, _, _, _ = fetchSingleAccountQuota(fetchCtx, eng, cpa, file.AuthIndex, p, file.ProjectID, file.Email, file.Name)
			}(f, prov)
		}
		wg.Wait()
	}

	// 3. Hydrate quota groups from SQLite quota_snapshots if available
	var dbHydratedMap map[string][]CpaQuotaGroup
	var dbHydratedTimes map[string]int64
	if eng != nil && eng.DB() != nil {
		since48h := time.Now().Add(-48 * time.Hour).Unix()
		if snapshots, err := eng.DB().SelectLatestQuotaSnapshots(ctx, since48h); err == nil && len(snapshots) > 0 {
			dbHydratedMap, dbHydratedTimes = hydrateQuotaGroupsFromSnapshots(snapshots)
		}
	}

	// 4. Background probe for active credentials lacking recent quotas (non-blocking)
	if !force && eng != nil {
		var probeFiles []CpaAuthFile
		for _, f := range authFilesResp.Files {
			prov := strings.ToLower(strings.TrimSpace(f.Provider))
			if prov == "" {
				prov = strings.ToLower(strings.TrimSpace(f.Type))
			}
			if !isSupportedQuotaProvider(prov, f) {
				continue
			}
			if f.Disabled || f.Status == "disabled" {
				continue
			}
			accountQuotaCacheMu.RLock()
			cached1, found1 := accountQuotaCache[f.AuthIndex]
			cached2, found2 := accountQuotaCache[f.Name]
			accountQuotaCacheMu.RUnlock()

			isStale := false
			if !found1 && !found2 {
				isStale = true
			} else {
				var lastUpdate time.Time
				if found1 && cached1.UpdatedAt.After(lastUpdate) {
					lastUpdate = cached1.UpdatedAt
				}
				if found2 && cached2.UpdatedAt.After(lastUpdate) {
					lastUpdate = cached2.UpdatedAt
				}
				if lastUpdate.IsZero() || time.Since(lastUpdate) > 60*time.Second {
					isStale = true
				}
			}
			key := f.AuthIndex
			if key == "" {
				key = f.Name
			}
			if isStale && shouldProbeAccount(key, 30*time.Second) {
				probeFiles = append(probeFiles, f)
			}
		}
		if len(probeFiles) > 0 {
			go func(pList []CpaAuthFile) {
				sem := make(chan struct{}, 4)
				var pWg sync.WaitGroup
				for _, file := range pList {
					pWg.Add(1)
					go func(fl CpaAuthFile) {
						defer pWg.Done()
						sem <- struct{}{}
						defer func() { <-sem }()
						pCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
						defer cancel()
						p := strings.ToLower(strings.TrimSpace(fl.Provider))
						if p == "" {
							p = strings.ToLower(strings.TrimSpace(fl.Type))
						}
						_, _, _, _ = fetchSingleAccountQuota(pCtx, eng, cpa, fl.AuthIndex, p, fl.ProjectID, fl.Email, fl.Name)
					}(file)
				}
				pWg.Wait()
				InvalidateUpstreamQuotaCache()
			}(probeFiles)
		}
	}

	// 5. Fetch authentic local usage statistics from SQLite
	todayDate := engine.ShanghaiDate(time.Now())
	todayStats, err := eng.DB().SelectUpstreamDailyForDate(ctx, todayDate)
	if err != nil {
		todayStats = []db.UpstreamDailyStatRow{}
	}
	todayStatsMap := make(map[string]db.UpstreamDailyStatRow)
	for _, s := range todayStats {
		todayStatsMap[s.AuthIndex] = s
	}

	fiveHoursAgo := time.Now().Add(-5 * time.Hour).Unix()
	windowStatsMap, err := eng.DB().SelectUpstreamWindowStats(ctx, fiveHoursAgo)
	if err != nil {
		windowStatsMap = make(map[string]db.UpstreamWindowStat)
	}

	lifetimeStats, err := eng.DB().SelectUpstreamDailyAgg(ctx)
	if err != nil {
		lifetimeStats = []db.UpstreamDailyStatRow{}
	}
	lifetimeStatsMap := make(map[string]db.UpstreamDailyStatRow)
	for _, s := range lifetimeStats {
		lifetimeStatsMap[s.AuthIndex] = s
	}

	populateLocalStats := func(item *UpstreamAccountItem, key, altKey string) {
		if stat, ok := todayStatsMap[key]; ok {
			item.TodayTokens = stat.TotalTokens
			item.TodayRequests = stat.Requests
		} else if altKey != "" {
			if stat, ok := todayStatsMap[altKey]; ok {
				item.TodayTokens = stat.TotalTokens
				item.TodayRequests = stat.Requests
			}
		}

		if wStat, ok := windowStatsMap[key]; ok {
			item.FiveHourTokens = wStat.Tokens
			item.FiveHourRequests = wStat.Requests
		} else if altKey != "" {
			if wStat, ok := windowStatsMap[altKey]; ok {
				item.FiveHourTokens = wStat.Tokens
				item.FiveHourRequests = wStat.Requests
			}
		}

		if lStat, ok := lifetimeStatsMap[key]; ok {
			item.LifetimeTokens = lStat.TotalTokens
			item.LifetimeRequests = lStat.Requests
		} else if altKey != "" {
			if lStat, ok := lifetimeStatsMap[altKey]; ok {
				item.LifetimeTokens = lStat.TotalTokens
				item.LifetimeRequests = lStat.Requests
			}
		}
	}

	// 6. Fetch pending validation challenges from SQLite
	pendingVals, err := eng.DB().SelectValidations(ctx, "pending")
	if err != nil {
		pendingVals = []db.UpstreamValidationRow{}
	}
	valMap := make(map[string]db.UpstreamValidationRow)
	for _, v := range pendingVals {
		valMap[v.AuthIndex] = v
	}

	nowMs := time.Now().UnixMilli()
	var accounts []UpstreamAccountItem

	// Process native auth-files
	for _, f := range authFilesResp.Files {
		fname := f.Name
		if fname == "" {
			fname = f.ID
		}
		prov := strings.ToLower(strings.TrimSpace(f.Provider))
		if prov == "" {
			prov = strings.ToLower(strings.TrimSpace(f.Type))
		}
		if prov == "" {
			prov = "unknown"
		}

		prio := defaultAuthPriority
		if f.Priority != nil && *f.Priority >= 0 && *f.Priority <= 100 {
			prio = *f.Priority
		}

		weight := defaultAuthWeight
		if f.Weight != nil && *f.Weight > 0 && *f.Weight <= 1000 {
			weight = *f.Weight
		}

		status := strings.TrimSpace(f.Status)
		if status == "" {
			if f.Disabled {
				status = "disabled"
			} else {
				status = "active"
			}
		}

		health := computeHealth(f)
		dispName := resolveDisplayName(f)

		email := strings.TrimSpace(f.Email)
		if email == "" || isGenericIdentifier(email, prov) {
			email = dispName
		}

		nick := strings.TrimSpace(f.Nickname)
		if nick == "" {
			nick = strings.TrimSpace(f.NickName)
		}

		planType := "free"
		planName := "免费版"
		var groups []CpaQuotaGroup
		var cachedCredits struct {
			hasCredits bool
			tot        *float64
			rem        *float64
			used       *float64
			expAt      string
		}

		// 1. Prioritize live-probed accountQuotaCache
		accountQuotaCacheMu.RLock()
		cached, found := accountQuotaCache[f.AuthIndex]
		if !found && fname != "" {
			cached, found = accountQuotaCache[fname]
		}
		if found {
			if len(cached.Groups) > 0 {
				groups = cached.Groups
			}
			if cached.PlanType != "" {
				planType = cached.PlanType
			}
			if cached.PlanName != "" {
				planName = cached.PlanName
			}
			if cached.TotalCredits != nil || cached.RemainingCredits != nil || cached.UsedCredits != nil || cached.ExpiresAt != "" {
				cachedCredits.hasCredits = true
				cachedCredits.tot = cached.TotalCredits
				cachedCredits.rem = cached.RemainingCredits
				cachedCredits.used = cached.UsedCredits
				cachedCredits.expAt = cached.ExpiresAt
			}
		}
		accountQuotaCacheMu.RUnlock()

		// 2. Fallback to auth-file quota groups if live cache is empty
		if len(groups) == 0 && f.Quota != nil && len(f.Quota.Groups) > 0 {
			groups = f.Quota.Groups
			planType = "standard"
			planName = "标准包"
		}

		// 3. Fallback to SQLite hydrated quota snapshots
		if len(groups) == 0 && dbHydratedMap != nil {
			if hGroups, found := dbHydratedMap[f.AuthIndex]; found && len(hGroups) > 0 {
				groups = hGroups
				planType = "standard"
				planName = "标准包"
				var snapTime time.Time
				if ts, ok := dbHydratedTimes[f.AuthIndex]; ok && ts > 0 {
					snapTime = time.Unix(ts, 0)
				} else {
					snapTime = time.Now().Add(-2 * time.Hour)
				}
				totCreds, remCreds, usedCreds, expAt := extractAccountCredits(groups)
				accountQuotaCacheMu.Lock()
				accountQuotaCache[f.AuthIndex] = CachedAccountQuota{
					Groups:           groups,
					PlanType:         planType,
					PlanName:         planName,
					UpdatedAt:        snapTime,
					TotalCredits:     totCreds,
					RemainingCredits: remCreds,
					UsedCredits:      usedCreds,
					ExpiresAt:        expAt,
				}
				if fname != "" {
					accountQuotaCache[fname] = accountQuotaCache[f.AuthIndex]
				}
				accountQuotaCacheMu.Unlock()
			} else if hGroups, found := dbHydratedMap[fname]; found && len(hGroups) > 0 {
				groups = hGroups
				planType = "standard"
				planName = "标准包"
				var snapTime time.Time
				if ts, ok := dbHydratedTimes[fname]; ok && ts > 0 {
					snapTime = time.Unix(ts, 0)
				} else {
					snapTime = time.Now().Add(-2 * time.Hour)
				}
				totCreds, remCreds, usedCreds, expAt := extractAccountCredits(groups)
				accountQuotaCacheMu.Lock()
				accountQuotaCache[fname] = CachedAccountQuota{
					Groups:           groups,
					PlanType:         planType,
					PlanName:         planName,
					UpdatedAt:        snapTime,
					TotalCredits:     totCreds,
					RemainingCredits: remCreds,
					UsedCredits:      usedCreds,
					ExpiresAt:        expAt,
				}
				accountQuotaCacheMu.Unlock()
			}
		}

		if prov == "antigravity" && len(f.Cooldowns) > 0 && len(groups) > 0 {
			groups = reconcileAntigravityQuotaGroups(groups, f.Cooldowns)
		}

		item := UpstreamAccountItem{
			ID:          fname,
			Filename:    fname,
			DisplayName: dispName,
			Nickname:    nick,
			Email:       email,
			Provider:    prov,
			Status:      status,
			Disabled:    f.Disabled || status == "disabled",
			Health:      health,
			Priority:    prio,
			Weight:      weight,
			Note:        f.Note,
			AuthIndex:   f.AuthIndex,
			PlanType:    planType,
			PlanName:    planName,
			Groups:      groups,
			UpdatedAt:   nowMs,
		}

		populateLocalStats(&item, f.AuthIndex, fname)
		if cachedCredits.hasCredits {
			item.TotalCredits = cachedCredits.tot
			item.RemainingCredits = cachedCredits.rem
			item.UsedCredits = cachedCredits.used
			if cachedCredits.expAt != "" {
				item.ExpiresAt = cachedCredits.expAt
			}
		} else {
			totCreds, remCreds, usedCreds, expAt := extractAccountCredits(item.Groups)
			item.TotalCredits = totCreds
			item.RemainingCredits = remCreds
			item.UsedCredits = usedCreds
			item.ExpiresAt = expAt
		}
		if (item.TotalCredits == nil && item.RemainingCredits == nil) && f.Quota != nil && len(f.Quota.Summary) > 0 {
			for _, m := range f.Quota.Summary {
				v := 0.0
				switch val := m.Value.(type) {
				case float64:
					v = val
				case int:
					v = float64(val)
				case int64:
					v = float64(val)
				case string:
					if fv, err := strconv.ParseFloat(val, 64); err == nil {
						v = fv
					}
				}
				switch m.Key {
				case "points_total", "total", "quota":
					item.TotalCredits = &v
				case "points_used", "used":
					item.UsedCredits = &v
				case "points_remain", "remaining", "balance", "points":
					item.RemainingCredits = &v
				case "expires_at", "runs_out_at":
					if m.Unit != "" && item.ExpiresAt == "" {
						item.ExpiresAt = m.Unit
					}
				}
			}
		}

		if val, ok := valMap[f.AuthIndex]; ok {
			vCopy := val
			item.Validation = &vCopy
		}

		accounts = append(accounts, item)
	}

	// Process openai-compatibility providers (OpenCode, OpenRouter, StepFun, etc.)
	for _, comp := range compatResp.Compatibility {
		provName := strings.ToLower(strings.TrimSpace(comp.Name))
		for idx, keyEntry := range comp.APIKeyEntries {
			authIdx := keyEntry.AuthIndex
			if authIdx == "" {
				authIdx = fmt.Sprintf("%s_%d", provName, idx)
			}
			fname := fmt.Sprintf("%s-%s", provName, authIdx)

			prio := defaultAuthPriority
			if keyEntry.Priority != nil && *keyEntry.Priority >= 0 && *keyEntry.Priority <= 100 {
				prio = *keyEntry.Priority
			}

			weight := defaultAuthWeight
			if keyEntry.Weight != nil && *keyEntry.Weight > 0 && *keyEntry.Weight <= 1000 {
				weight = *keyEntry.Weight
			}

			status := "active"
			disabled := comp.Disabled
			if disabled {
				status = "disabled"
			}

			dispName := keyEntry.Note
			if dispName == "" {
				prefixLen := 8
				if len(authIdx) < prefixLen {
					prefixLen = len(authIdx)
				}
				dispName = fmt.Sprintf("%s (%s)", provName, authIdx[:prefixLen])
			}

			healthLabel := "正常"
			if disabled {
				healthLabel = "已停用"
			}

			compPlanType := "standard"
			compPlanName := provName
			var compGroups []CpaQuotaGroup
			accountQuotaCacheMu.RLock()
			if cached, found := accountQuotaCache[authIdx]; found && len(cached.Groups) > 0 {
				compGroups = cached.Groups
				if cached.PlanType != "" {
					compPlanType = cached.PlanType
				}
				if cached.PlanName != "" {
					compPlanName = cached.PlanName
				}
			} else if cached, found := accountQuotaCache[fname]; found && len(cached.Groups) > 0 {
				compGroups = cached.Groups
				if cached.PlanType != "" {
					compPlanType = cached.PlanType
				}
				if cached.PlanName != "" {
					compPlanName = cached.PlanName
				}
			}
			accountQuotaCacheMu.RUnlock()

			if len(compGroups) == 0 && dbHydratedMap != nil {
				if hGroups, found := dbHydratedMap[authIdx]; found && len(hGroups) > 0 {
					compGroups = hGroups
				} else if hGroups, found := dbHydratedMap[fname]; found && len(hGroups) > 0 {
					compGroups = hGroups
				}
			}

			item := UpstreamAccountItem{
				ID:          fname,
				Filename:    fname,
				DisplayName: dispName,
				Nickname:    "",
				Email:       maskSecret(keyEntry.APIKey),
				Provider:    provName,
				Status:      status,
				Disabled:    disabled,
				Health: UpstreamAccountHealth{
					State:   status,
					Label:   healthLabel,
					Detail:  "",
					Blocked: disabled,
				},
				Priority:  prio,
				Weight:    weight,
				Note:      keyEntry.Note,
				AuthIndex: authIdx,
				PlanType:  compPlanType,
				PlanName:  compPlanName,
				Groups:    compGroups,
				UpdatedAt: nowMs,
			}

			populateLocalStats(&item, authIdx, fname)
			totCreds, remCreds, usedCreds, expAt := extractAccountCredits(item.Groups)
			item.TotalCredits = totCreds
			item.RemainingCredits = remCreds
			item.UsedCredits = usedCreds
			item.ExpiresAt = expAt

			if val, ok := valMap[authIdx]; ok {
				vCopy := val
				item.Validation = &vCopy
			}

			accounts = append(accounts, item)
		}
	}

	// 5. If no accounts were discovered (e.g. CPA is offline), gracefully fallback to SQLite baselines
	if len(accounts) == 0 {
		baselines, err := eng.DB().SelectBaselinesAll(ctx)
		if err == nil {
			for _, b := range baselines {
				name := b.AuthIndex
				if b.Name != nil && *b.Name != "" {
					name = *b.Name
				}
				prov := ""
				if b.Provider != nil && *b.Provider != "" {
					prov = *b.Provider
				}
				provRes := providers.InferProvider("", b.AuthIndex)
				if prov == "" {
					prov = provRes.Key
				}
				var bGroups []CpaQuotaGroup
				if dbHydratedMap != nil {
					if hGroups, found := dbHydratedMap[b.AuthIndex]; found && len(hGroups) > 0 {
						bGroups = hGroups
					}
				}
				item := UpstreamAccountItem{
					ID:          b.AuthIndex,
					Filename:    b.AuthIndex,
					DisplayName: name,
					Email:       name,
					Provider:    prov,
					Status:      "active",
					Disabled:    false,
					Priority:    defaultAuthPriority,
					Weight:      defaultAuthWeight,
					AuthIndex:   b.AuthIndex,
					PlanType:    "standard",
					PlanName:    prov,
					Groups:      bGroups,
					UpdatedAt:   nowMs,
				}
				populateLocalStats(&item, b.AuthIndex, "")
				totCreds, remCreds, usedCreds, expAt := extractAccountCredits(item.Groups)
				item.TotalCredits = totCreds
				item.RemainingCredits = remCreds
				item.UsedCredits = usedCreds
				item.ExpiresAt = expAt
				if val, ok := valMap[b.AuthIndex]; ok {
					vCopy := val
					item.Validation = &vCopy
				}
				accounts = append(accounts, item)
			}
		}
	}

	// 6. Merge local OpenRouter accounts from data/openrouter-accounts.json
	orAccounts := getOpenRouterAccountsList()
	for _, orAcc := range orAccounts {
		accStatus := "active"
		if !orAcc.Enabled {
			accStatus = "disabled"
		}
		item := UpstreamAccountItem{
			ID:          "openrouter-" + orAcc.ID,
			Filename:    "openrouter-" + orAcc.ID,
			DisplayName: orAcc.Name,
			Email:       orAcc.APIKeyMasked,
			Provider:    "openrouter",
			Status:      accStatus,
			Disabled:    !orAcc.Enabled,
			Priority:    orAcc.Priority,
			Weight:      orAcc.Weight,
			AuthIndex:   "openrouter-" + orAcc.ID,
			PlanType:    "standard",
			PlanName:    "OpenRouter",
			Groups:      []CpaQuotaGroup{},
			UpdatedAt:   nowMs,
		}
		populateLocalStats(&item, item.AuthIndex, "")
		accounts = append(accounts, item)
	}

	// Sort accounts: Priority DESC, then AuthIndex ASC
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].Priority != accounts[j].Priority {
			return accounts[i].Priority > accounts[j].Priority
		}
		return accounts[i].AuthIndex < accounts[j].AuthIndex
	})

	response := &UpstreamQuotasResponse{
		Accounts:           accounts,
		PendingValidations: pendingVals,
		DefaultPriority:    defaultAuthPriority,
		LastUpdated:        nowMs,
		IsLoading:          false,
		Error:              nil,
	}

	upstreamCacheMu.Lock()
	cachedUpstreamQuotasResp = response
	cachedUpstreamAt = time.Now()
	upstreamCacheMu.Unlock()

	return response, nil
}
