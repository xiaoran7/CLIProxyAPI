package api

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// HandleGetStats returns daily token and request aggregates over a time window.
func HandleGetStats(eng *engine.Engine, pricing *PricingEngine, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		database := eng.DB()

		// Flush outbox and live counters so latest events are committed to SQLite
		_ = eng.Flusher().FlushUsage(ctx)

		daysParam := strings.TrimSpace(c.DefaultQuery("days", "14"))
		isAllTime := strings.ToLower(daysParam) == "all" || daysParam == "0"
		dayCount := 14
		if !isAllTime {
			if d, err := strconv.Atoi(daysParam); err == nil && d > 0 && d <= 365 {
				dayCount = d
			}
		}

		var keyIDPtr *int64
		if keyIDStr := c.Query("key_id"); keyIDStr != "" {
			if id, err := strconv.ParseInt(keyIDStr, 10, 64); err == nil && id > 0 {
				keyIDPtr = &id
			}
		}

		now := time.Now().In(cstZone)
		sinceDate := ""
		sinceTs := int64(0)
		if !isAllTime {
			sinceTime := now.AddDate(0, 0, -(dayCount - 1))
			sinceDate = sinceTime.Format("2006-01-02")
			if t0, err := time.ParseInLocation("2006-01-02", sinceDate, cstZone); err == nil {
				sinceTs = t0.Unix()
			}
		}

		var dailyList []db.DailyStatsSummary
		var modelList []db.ModelStatsSummary
		var dailyModels []db.DailyModelStatSummary
		var err error

		if keyIDPtr != nil {
			dailyList, err = database.SelectDailyRangeByKey(ctx, sinceDate, *keyIDPtr)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query daily range: %v", err)})
				return
			}
			modelList, err = database.SelectModelRangeByKey(ctx, sinceDate, *keyIDPtr)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query model range: %v", err)})
				return
			}
			dailyModels, _ = database.SelectDailyModelsRange(ctx, sinceDate, keyIDPtr)
		} else {
			dailyList, err = database.SelectDailyRange(ctx, sinceDate)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query daily range: %v", err)})
				return
			}
			modelList, err = database.SelectModelRange(ctx, sinceDate)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to query model range: %v", err)})
				return
			}
			dailyModels, _ = database.SelectDailyModelsRange(ctx, sinceDate, nil)
		}

		// 1. Calculate real daily costs and savings per model
		dailyCostMap := make(map[string]float64)
		dailySavingsMap := make(map[string]float64)
		for _, dm := range dailyModels {
			costObj := pricing.CalculateModelCost(dm.Model, dm.PromptTokens, dm.CompletionTokens, dm.CachedTokens)
			dailyCostMap[dm.Date] += costObj.CostUSD
			dailySavingsMap[dm.Date] += costObj.SavingsUSD
		}

		type DailyResponseItem struct {
			Date             string  `json:"date"`
			Requests         int64   `json:"requests"`
			SuccessCount     int64   `json:"success_count"`
			FailureCount     int64   `json:"failure_count"`
			SuccessRate      float64 `json:"success_rate"`
			PromptTokens     int64   `json:"prompt_tokens"`
			CompletionTokens int64   `json:"completion_tokens"`
			CachedTokens     int64   `json:"cached_tokens"`
			TotalTokens      int64   `json:"total_tokens"`
			CacheHitRate     float64 `json:"cache_hit_rate"`
			CostUSD          float64 `json:"cost_usd"`
			SavingsUSD       float64 `json:"savings_usd"`
		}

		dailyMap := make(map[string]*DailyResponseItem)
		for _, d := range dailyList {
			pBase := d.PromptTokens
			if d.CachedTokens > pBase {
				pBase = d.CachedTokens
			}
			hitRate := 0.0
			if pBase > 0 {
				hitRate = math.Round((float64(d.CachedTokens)/float64(pBase))*1000.0) / 10.0
			}
			costUSD := math.Round(dailyCostMap[d.Date]*10000.0) / 10000.0
			savingsUSD := math.Round(dailySavingsMap[d.Date]*10000.0) / 10000.0
			successRate := 100.0
			if d.Requests > 0 {
				successRate = math.Round((float64(d.SuccessCount)/float64(d.Requests))*1000.0) / 10.0
			}

			dailyMap[d.Date] = &DailyResponseItem{
				Date:             d.Date,
				Requests:         d.Requests,
				SuccessCount:     d.SuccessCount,
				FailureCount:     d.FailureCount,
				SuccessRate:      successRate,
				PromptTokens:     d.PromptTokens,
				CompletionTokens: d.CompletionTokens,
				CachedTokens:     d.CachedTokens,
				TotalTokens:      d.TotalTokens,
				CacheHitRate:     hitRate,
				CostUSD:          costUSD,
				SavingsUSD:       savingsUSD,
			}
		}

		// 2. Accumulate today's committed + live usage
		todayStr := engine.ShanghaiDate(time.Now())
		todayRows, _ := database.SelectDailyForDate(ctx, todayStr)
		todayTokens := int64(0)
		todayRequests := int64(0)
		todayPrompt := int64(0)
		todayCompletion := int64(0)
		todayCached := int64(0)
		todaySuccess := int64(0)
		todayFailure := int64(0)
		todayCost := 0.0
		todaySavings := 0.0

		for _, row := range todayRows {
			if keyIDPtr != nil && row.KeyID != *keyIDPtr {
				continue
			}
			todayTokens += row.TotalTokens
			todayRequests += row.Requests
			todayPrompt += row.PromptTokens
			todayCompletion += row.CompletionTokens
			todayCached += row.CachedTokens
			todaySuccess += row.SuccessCount
			todayFailure += row.FailureCount
			costObj := pricing.CalculateModelCost(row.Model, row.PromptTokens, row.CompletionTokens, row.CachedTokens)
			todayCost += costObj.CostUSD
			todaySavings += costObj.SavingsUSD
		}

		todayCost = math.Round(todayCost*10000.0) / 10000.0
		todaySavings = math.Round(todaySavings*10000.0) / 10000.0
		todayPBase := todayPrompt
		if todayCached > todayPBase {
			todayPBase = todayCached
		}
		todayHitRate := 0.0
		if todayPBase > 0 {
			todayHitRate = math.Round((float64(todayCached)/float64(todayPBase))*1000.0) / 10.0
		}
		todaySuccessRate := 100.0
		if todayRequests > 0 {
			todaySuccessRate = math.Round((float64(todaySuccess)/float64(todayRequests))*1000.0) / 10.0
		}

		if todayTokens > 0 || todayRequests > 0 {
			dailyMap[todayStr] = &DailyResponseItem{
				Date:             todayStr,
				Requests:         todayRequests,
				SuccessCount:     todaySuccess,
				FailureCount:     todayFailure,
				SuccessRate:      todaySuccessRate,
				PromptTokens:     todayPrompt,
				CompletionTokens: todayCompletion,
				CachedTokens:     todayCached,
				TotalTokens:      todayTokens,
				CacheHitRate:     todayHitRate,
				CostUSD:          todayCost,
				SavingsUSD:       todaySavings,
			}
		}

		var formattedDaily []DailyResponseItem
		for _, item := range dailyMap {
			formattedDaily = append(formattedDaily, *item)
		}
		sort.Slice(formattedDaily, func(i, j int) bool {
			return formattedDaily[i].Date < formattedDaily[j].Date
		})

		// 3. Dominant provider identification per model from auth_index & model aggregations
		authModelAggs, _ := database.SelectAuthModelAggRange(ctx, sinceTs, keyIDPtr)

		type dominantProv struct {
			key    string
			label  string
			tokens int64
		}
		modelDominant := make(map[string]dominantProv)
		modelAccountedTokens := make(map[string]int64)

		for _, item := range authModelAggs {
			modelAccountedTokens[item.Model] += item.TotalTokens
			provRes := providers.InferProvider(item.Model, item.AuthIndex)
			pKey := provRes.Key
			if pKey == "" || pKey == "unknown" {
				pKey = UnknownProviderKey
			}
			pLabel := provRes.Label
			if pLabel == "" {
				pLabel = providers.GetProviderLabel(pKey)
			}
			if pLabel == "" {
				pLabel = UnknownProviderLabel
			}

			prev, exists := modelDominant[item.Model]
			if !exists || item.TotalTokens > prev.tokens {
				modelDominant[item.Model] = dominantProv{
					key:    pKey,
					label:  pLabel,
					tokens: item.TotalTokens,
				}
			}
		}

		// Calculate total period tokens
		periodTotalTokens := int64(0)
		for _, m := range modelList {
			periodTotalTokens += m.TotalTokens
		}

		type ModelResponseItem struct {
			Model            string  `json:"model"`
			Provider         string  `json:"provider"`
			Requests         int64   `json:"requests"`
			SuccessCount     int64   `json:"success_count"`
			FailureCount     int64   `json:"failure_count"`
			SuccessRate      float64 `json:"success_rate"`
			PromptTokens     int64   `json:"prompt_tokens"`
			CompletionTokens int64   `json:"completion_tokens"`
			CachedTokens     int64   `json:"cached_tokens"`
			TotalTokens      int64   `json:"total_tokens"`
			TokensPercent    float64 `json:"tokens_percent"`
			CacheHitRate     float64 `json:"cache_hit_rate"`
			CostUSD          float64 `json:"cost_usd"`
			SavingsUSD       float64 `json:"savings_usd"`
		}

		var formattedModels []ModelResponseItem
		for _, m := range modelList {
			cost := pricing.CalculateModelCost(m.Model, m.PromptTokens, m.CompletionTokens, m.CachedTokens)
			pBase := m.PromptTokens
			if m.CachedTokens > pBase {
				pBase = m.CachedTokens
			}
			hitRate := 0.0
			if pBase > 0 {
				hitRate = math.Round((float64(m.CachedTokens)/float64(pBase))*1000.0) / 10.0
			}
			successRate := 100.0
			if m.Requests > 0 {
				successRate = math.Round((float64(m.SuccessCount)/float64(m.Requests))*1000.0) / 10.0
			}
			tokensPct := 0.0
			if periodTotalTokens > 0 {
				tokensPct = math.Round((float64(m.TotalTokens)/float64(periodTotalTokens))*1000.0) / 10.0
			}

			provLabel := ""
			if dom, ok := modelDominant[m.Model]; ok && dom.label != "" {
				provLabel = dom.label
			} else {
				provLabel = providers.InferModelProviderLabel(m.Model)
			}

			formattedModels = append(formattedModels, ModelResponseItem{
				Model:            m.Model,
				Provider:         provLabel,
				Requests:         m.Requests,
				SuccessCount:     m.SuccessCount,
				FailureCount:     m.FailureCount,
				SuccessRate:      successRate,
				PromptTokens:     m.PromptTokens,
				CompletionTokens: m.CompletionTokens,
				CachedTokens:     m.CachedTokens,
				TotalTokens:      m.TotalTokens,
				TokensPercent:    tokensPct,
				CacheHitRate:     hitRate,
				CostUSD:          cost.CostUSD,
				SavingsUSD:       cost.SavingsUSD,
			})
		}
		sort.Slice(formattedModels, func(i, j int) bool {
			return formattedModels[i].TotalTokens > formattedModels[j].TotalTokens
		})

		// 4. Provider Breakdown & Reconciliation
		type providerStatAccumulator struct {
			provider         string
			displayName      string
			requests         int64
			totalTokens      int64
			promptTokens     int64
			completionTokens int64
			cachedTokens     int64
			costUSD          float64
			savingsUSD       float64
			modelsSet        map[string]struct{}
		}

		providerMap := make(map[string]*providerStatAccumulator)
		getOrCreateProvider := func(provKey, provLabel string) *providerStatAccumulator {
			if stat, ok := providerMap[provKey]; ok {
				return stat
			}
			stat := &providerStatAccumulator{
				provider:    provKey,
				displayName: provLabel,
				modelsSet:   make(map[string]struct{}),
			}
			providerMap[provKey] = stat
			return stat
		}

		if len(authModelAggs) == 0 {
			// Fallback: Infer providers from model list if no request_events exist
			for _, m := range formattedModels {
				inferred := providers.InferProvider(m.Model, "")
				provKey := inferred.Key
				if provKey == "" || provKey == "unknown" {
					provKey = UnknownProviderKey
				}
				provLabel := inferred.Label
				if provLabel == "" {
					provLabel = providers.GetProviderLabel(provKey)
				}
				if provLabel == "" {
					provLabel = OtherModelLabel
				}
				pStat := getOrCreateProvider(provKey, provLabel)
				pStat.requests += m.Requests
				pStat.totalTokens += m.TotalTokens
				pStat.promptTokens += m.PromptTokens
				pStat.completionTokens += m.CompletionTokens
				pStat.cachedTokens += m.CachedTokens
				pStat.costUSD += m.CostUSD
				pStat.savingsUSD += m.SavingsUSD
				pStat.modelsSet[m.Model] = struct{}{}
			}
		} else {
			// Accurate credential-level accumulation
			for _, item := range authModelAggs {
				provRes := providers.InferProvider(item.Model, item.AuthIndex)
				provKey := provRes.Key
				if provKey == "" || provKey == "unknown" {
					provKey = UnknownProviderKey
				}
				provLabel := provRes.Label
				if provLabel == "" {
					provLabel = providers.GetProviderLabel(provKey)
				}
				if provLabel == "" {
					provLabel = UnknownProviderLabel
				}
				pStat := getOrCreateProvider(provKey, provLabel)
				pStat.requests += item.Requests
				pStat.totalTokens += item.TotalTokens
				pStat.promptTokens += item.PromptTokens
				pStat.completionTokens += item.CompletionTokens
				pStat.cachedTokens += item.CachedTokens

				costObj := pricing.CalculateModelCost(item.Model, item.PromptTokens, item.CompletionTokens, item.CachedTokens)
				pStat.costUSD += costObj.CostUSD
				pStat.savingsUSD += costObj.SavingsUSD
				pStat.modelsSet[item.Model] = struct{}{}
			}

			// Reconciliation against periodTotalTokens
			for _, m := range formattedModels {
				accounted := modelAccountedTokens[m.Model]
				if m.TotalTokens > accounted {
					diffTokens := m.TotalTokens - accounted
					ratio := 0.0
					if m.TotalTokens > 0 {
						ratio = float64(accounted) / float64(m.TotalTokens)
					}
					diffPrompt := int64(math.Max(0, float64(m.PromptTokens)-math.Round(float64(m.PromptTokens)*ratio)))
					diffCompletion := int64(math.Max(0, float64(m.CompletionTokens)-math.Round(float64(m.CompletionTokens)*ratio)))
					diffCached := int64(math.Max(0, float64(m.CachedTokens)-math.Round(float64(m.CachedTokens)*ratio)))
					diffRequests := int64(math.Max(0, float64(m.Requests)-math.Round(float64(m.Requests)*ratio)))

					provKey := UnknownProviderKey
					provLabel := UnknownProviderLabel
					if dom, ok := modelDominant[m.Model]; ok && dom.key != "" {
						provKey = dom.key
						provLabel = dom.label
					} else {
						inferred := providers.InferProvider(m.Model, "")
						if inferred.Key != "" {
							provKey = inferred.Key
							provLabel = inferred.Label
						}
					}

					pStat := getOrCreateProvider(provKey, provLabel)
					pStat.requests += diffRequests
					pStat.totalTokens += diffTokens
					pStat.promptTokens += diffPrompt
					pStat.completionTokens += diffCompletion
					pStat.cachedTokens += diffCached

					costObj := pricing.CalculateModelCost(m.Model, diffPrompt, diffCompletion, diffCached)
					pStat.costUSD += costObj.CostUSD
					pStat.savingsUSD += costObj.SavingsUSD
					pStat.modelsSet[m.Model] = struct{}{}
				}
			}
		}

		type ProviderResponseItem struct {
			Provider            string  `json:"provider"`
			DisplayName         string  `json:"display_name"`
			Requests            int64   `json:"requests"`
			TotalTokens         int64   `json:"total_tokens"`
			PromptTokens        int64   `json:"prompt_tokens"`
			CompletionTokens    int64   `json:"completion_tokens"`
			CachedTokens        int64   `json:"cached_tokens"`
			ModelsCount         int     `json:"models_count"`
			CostUSD             float64 `json:"cost_usd"`
			SavingsUSD          float64 `json:"savings_usd"`
			CacheHitRate        float64 `json:"cache_hit_rate"`
			TokensPercent       float64 `json:"tokens_percent"`
			AvgTokensPerRequest int64   `json:"avg_tokens_per_request"`
		}

		var formattedProviders []ProviderResponseItem
		for _, p := range providerMap {
			pBase := p.promptTokens
			if p.cachedTokens > pBase {
				pBase = p.cachedTokens
			}
			hitRate := 0.0
			if pBase > 0 {
				hitRate = math.Round((float64(p.cachedTokens)/float64(pBase))*1000.0) / 10.0
			}
			tokensPct := 0.0
			if periodTotalTokens > 0 {
				tokensPct = math.Round((float64(p.totalTokens)/float64(periodTotalTokens))*1000.0) / 10.0
			}
			avgTokens := int64(0)
			if p.requests > 0 {
				avgTokens = p.totalTokens / p.requests
			}

			formattedProviders = append(formattedProviders, ProviderResponseItem{
				Provider:            p.provider,
				DisplayName:         p.displayName,
				Requests:            p.requests,
				TotalTokens:         p.totalTokens,
				PromptTokens:        p.promptTokens,
				CompletionTokens:    p.completionTokens,
				CachedTokens:        p.cachedTokens,
				ModelsCount:         len(p.modelsSet),
				CostUSD:             math.Round(p.costUSD*10000.0) / 10000.0,
				SavingsUSD:          math.Round(p.savingsUSD*10000.0) / 10000.0,
				CacheHitRate:        hitRate,
				TokensPercent:       tokensPct,
				AvgTokensPerRequest: avgTokens,
			})
		}
		sort.Slice(formattedProviders, func(i, j int) bool {
			return formattedProviders[i].TotalTokens > formattedProviders[j].TotalTokens
		})

		// 5. Hourly Breakdown for days=1
		type HourlyResponseItem struct {
			Hour             string  `json:"hour"`
			Date             string  `json:"date"`
			Requests         int64   `json:"requests"`
			SuccessCount     int64   `json:"success_count"`
			FailureCount     int64   `json:"failure_count"`
			PromptTokens     int64   `json:"prompt_tokens"`
			CompletionTokens int64   `json:"completion_tokens"`
			CachedTokens     int64   `json:"cached_tokens"`
			TotalTokens      int64   `json:"total_tokens"`
			CostUSD          float64 `json:"cost_usd"`
			SavingsUSD       float64 `json:"savings_usd"`
			CacheHitRate     float64 `json:"cache_hit_rate"`
		}

		var hourly []HourlyResponseItem
		if dayCount == 1 && !isAllTime {
			t0, _ := time.ParseInLocation("2006-01-02", todayStr, cstZone)
			todayStartTs := t0.Unix()
			todayEndTs := todayStartTs + 86400

			hourlyRows, _ := database.SelectHourlyRange(ctx, todayStartTs, todayEndTs, keyIDPtr)

			hourBuckets := make(map[string]*HourlyResponseItem)
			for h := 0; h < 24; h++ {
				hStr := fmt.Sprintf("%02d", h)
				hourBuckets[hStr] = &HourlyResponseItem{
					Hour: fmt.Sprintf("%s:00", hStr),
					Date: fmt.Sprintf("%s:00", hStr),
				}
			}

			for _, r := range hourlyRows {
				if b, ok := hourBuckets[r.Hour]; ok {
					b.Requests += r.Requests
					b.SuccessCount += r.SuccessCount
					b.FailureCount += r.FailureCount
					b.PromptTokens += r.PromptTokens
					b.CompletionTokens += r.CompletionTokens
					b.CachedTokens += r.CachedTokens
					b.TotalTokens += r.TotalTokens
					costObj := pricing.CalculateModelCost(r.Model, r.PromptTokens, r.CompletionTokens, r.CachedTokens)
					b.CostUSD += costObj.CostUSD
					b.SavingsUSD += costObj.SavingsUSD
				}
			}

			for h := 0; h < 24; h++ {
				hStr := fmt.Sprintf("%02d", h)
				b := hourBuckets[hStr]
				b.CostUSD = math.Round(b.CostUSD*10000.0) / 10000.0
				b.SavingsUSD = math.Round(b.SavingsUSD*10000.0) / 10000.0
				pBase := b.PromptTokens
				if b.CachedTokens > pBase {
					pBase = b.CachedTokens
				}
				if pBase > 0 {
					b.CacheHitRate = math.Round((float64(b.CachedTokens)/float64(pBase))*1000.0) / 10.0
				}
				hourly = append(hourly, *b)
			}
		}

		// 6. Selected period metrics
		periodTokens := int64(0)
		periodRequests := int64(0)
		periodPrompt := int64(0)
		periodCompletion := int64(0)
		periodCached := int64(0)
		periodSuccess := int64(0)
		periodFailure := int64(0)
		periodCost := 0.0
		periodSavings := 0.0

		for _, day := range formattedDaily {
			periodTokens += day.TotalTokens
			periodRequests += day.Requests
			periodPrompt += day.PromptTokens
			periodCompletion += day.CompletionTokens
			periodCached += day.CachedTokens
			periodSuccess += day.SuccessCount
			periodFailure += day.FailureCount
			periodCost += day.CostUSD
			periodSavings += day.SavingsUSD
		}
		periodCost = math.Round(periodCost*10000.0) / 10000.0
		periodSavings = math.Round(periodSavings*10000.0) / 10000.0
		periodPBase := periodPrompt
		if periodCached > periodPBase {
			periodPBase = periodCached
		}
		periodHitRate := 0.0
		if periodPBase > 0 {
			periodHitRate = math.Round((float64(periodCached)/float64(periodPBase))*1000.0) / 10.0
		}
		periodSuccessRate := 100.0
		if periodRequests > 0 {
			periodSuccessRate = math.Round((float64(periodSuccess)/float64(periodRequests))*1000.0) / 10.0
		}
		periodTokensPerReq := int64(0)
		if periodRequests > 0 {
			periodTokensPerReq = periodTokens / periodRequests
		}

		// 7. All-Time Totals
		allTimeRow, _ := database.SelectDailyAllTimeDetailed(ctx, keyIDPtr)
		allTimeTokens := int64(0)
		allTimeRequests := int64(0)
		allTimePrompt := int64(0)
		allTimeCompletion := int64(0)
		allTimeCached := int64(0)
		allTimeSuccess := int64(0)
		allTimeFailure := int64(0)

		if allTimeRow != nil {
			allTimeTokens = allTimeRow.TotalTokens
			allTimeRequests = allTimeRow.Requests
			allTimePrompt = allTimeRow.PromptTokens
			allTimeCompletion = allTimeRow.CompletionTokens
			allTimeCached = allTimeRow.CachedTokens
			allTimeSuccess = allTimeRow.SuccessCount
			allTimeFailure = allTimeRow.FailureCount
		}

		allTimeModelRows, _ := database.SelectModelAllTimeDetailed(ctx, keyIDPtr)
		allTimeCost := 0.0
		allTimeSavings := 0.0
		for _, r := range allTimeModelRows {
			costObj := pricing.CalculateModelCost(r.Model, r.PromptTokens, r.CompletionTokens, r.CachedTokens)
			allTimeCost += costObj.CostUSD
			allTimeSavings += costObj.SavingsUSD
		}
		allTimeCost = math.Round(allTimeCost*10000.0) / 10000.0
		allTimeSavings = math.Round(allTimeSavings*10000.0) / 10000.0
		allTimePBase := allTimePrompt
		if allTimeCached > allTimePBase {
			allTimePBase = allTimeCached
		}
		allTimeHitRate := 0.0
		if allTimePBase > 0 {
			allTimeHitRate = math.Round((float64(allTimeCached)/float64(allTimePBase))*1000.0) / 10.0
		}
		allTimeSuccessRate := 100.0
		if allTimeRequests > 0 {
			allTimeSuccessRate = math.Round((float64(allTimeSuccess)/float64(allTimeRequests))*1000.0) / 10.0
		}
		allTimeTokensPerReq := int64(0)
		if allTimeRequests > 0 {
			allTimeTokensPerReq = allTimeTokens / allTimeRequests
		}

		// 8. Key status breakdown
		allKeys, _ := database.SelectAllKeys(ctx)
		rules, _ := database.SelectAllQuotaRules(ctx)
		ruleMap := make(map[int64]db.QuotaRuleRow)
		for _, r := range rules {
			ruleMap[r.KeyID] = r
		}

		activeKeys := 0
		blockedKeys := 0
		warnedKeys := 0
		for _, k := range allKeys {
			if k.Active == 1 {
				activeKeys++
			}
			if r, ok := ruleMap[k.ID]; ok && r.Enabled == 1 && r.DailyLimit != nil && *r.DailyLimit > 0 {
				used := eng.Memory().GetTodayUsageForKey(k.ID)
				hardRatio := r.HardRatio
				if hardRatio <= 0 {
					hardRatio = 1.2
				}
				hardLimit := int64(float64(*r.DailyLimit) * hardRatio)
				if used >= hardLimit {
					blockedKeys++
				} else if float64(used) >= float64(*r.DailyLimit)*0.85 {
					warnedKeys++
				}
			}
		}

		effTokens := periodTokens
		effRequests := periodRequests
		effCost := periodCost
		effSavings := periodSavings
		effPrompt := periodPrompt
		effCompletion := periodCompletion
		effCached := periodCached
		effHitRate := periodHitRate
		effSuccessRate := periodSuccessRate
		if isAllTime {
			effTokens = allTimeTokens
			effRequests = allTimeRequests
			effCost = allTimeCost
			effSavings = allTimeSavings
			effPrompt = allTimePrompt
			effCompletion = allTimeCompletion
			effCached = allTimeCached
			effHitRate = allTimeHitRate
			effSuccessRate = allTimeSuccessRate
		}

		todayTokensPerReq := int64(0)
		if todayRequests > 0 {
			todayTokensPerReq = todayTokens / todayRequests
		}

		summary := gin.H{
			"period_days":              daysParam,
			"period_tokens":            effTokens,
			"period_total_tokens":      effTokens,
			"period_requests":          effRequests,
			"period_prompt_tokens":     effPrompt,
			"period_completion_tokens": effCompletion,
			"period_cached_tokens":     effCached,
			"period_cache_hit_rate":    effHitRate,
			"period_cost_usd":          effCost,
			"period_savings_usd":       effSavings,
			"period_success_count":     periodSuccess,
			"period_failure_count":     periodFailure,
			"period_success_rate":      effSuccessRate,
			"period_tokens_per_req":    periodTokensPerReq,

			"today_tokens":            todayTokens,
			"today_requests":          todayRequests,
			"today_success_count":     todaySuccess,
			"today_failure_count":     todayFailure,
			"today_success_rate":      todaySuccessRate,
			"today_prompt_tokens":     todayPrompt,
			"today_completion_tokens": todayCompletion,
			"today_cached_tokens":     todayCached,
			"today_cache_hit_rate":    todayHitRate,
			"today_cost_usd":          todayCost,
			"today_savings_usd":       todaySavings,
			"today_tokens_per_req":    todayTokensPerReq,

			"all_time_tokens":            allTimeTokens,
			"all_time_requests":          allTimeRequests,
			"all_time_prompt_tokens":     allTimePrompt,
			"all_time_completion_tokens": allTimeCompletion,
			"all_time_cached_tokens":     allTimeCached,
			"all_time_success_count":     allTimeSuccess,
			"all_time_failure_count":     allTimeFailure,
			"all_time_cost_usd":          allTimeCost,
			"all_time_savings_usd":       allTimeSavings,
			"all_time_cache_hit_rate":    allTimeHitRate,
			"all_time_success_rate":      allTimeSuccessRate,
			"all_time_tokens_per_req":    allTimeTokensPerReq,

			"blocked_keys": blockedKeys,
			"warned_keys":  warnedKeys,
			"active_keys":  activeKeys,

			// Legacy and fallback aliases
			"total_tokens":   effTokens,
			"requests":       effRequests,
			"cost_usd":       effCost,
			"savings_usd":    effSavings,
			"prompt_tokens":  effPrompt,
			"cached_tokens":  effCached,
			"cache_hit_rate": effHitRate,
			"success_rate":   effSuccessRate,
			"total_requests": effRequests,
			"total_cost_usd": effCost,
			"success_count":  periodSuccess,
			"failure_count":  periodFailure,
		}

		c.JSON(http.StatusOK, gin.H{
			"daily":     formattedDaily,
			"hourly":    hourly,
			"models":    formattedModels,
			"providers": formattedProviders,
			"today":     summary,
			"summary":   summary,
		})
	}
}
