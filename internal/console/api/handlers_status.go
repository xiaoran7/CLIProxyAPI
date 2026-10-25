package api

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// HandleGetStatusSummary returns a comprehensive operational summary of the console gateway.
func HandleGetStatusSummary(eng *engine.Engine, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		database := eng.DB()

		outboxCount, _ := database.CountOutbox(ctx)
		validations, _ := database.SelectValidations(ctx, "pending")

		today := engine.ShanghaiDate(time.Now())
		dailyToday, _ := database.SelectDailyForDate(ctx, today)

		todayTokens := int64(0)
		for _, row := range dailyToday {
			todayTokens += row.TotalTokens
		}

		todayRequests, _ := database.CountTodayEvents(ctx, 0, nil)
		todayErrors, _ := database.CountTodayErrors(ctx, 0, nil)

		// 7-day window for Uptime Kuma heartbeats and provider groups
		sinceTs := time.Now().Unix() - 7*86400
		rows, _ := database.SelectEventsForStatus(ctx, sinceTs, 3000)

		nowMs := time.Now().UnixMilli()
		overallScope := buildStatusScope(rows, nowMs)
		providerGroups := buildProviderGroups(rows, providers, nowMs)

		c.JSON(http.StatusOK, gin.H{
			"status":          "ok",
			"window_days":     7,
			"heartbeat_count": 60,
			"overall":         overallScope,
			"provider_groups": providerGroups,
			"today_requests":  todayRequests,
			"today_errors":    todayErrors,
			"today": gin.H{
				"date":     today,
				"tokens":   todayTokens,
				"requests": todayRequests,
			},
			"health": gin.H{
				"outbox_count":        outboxCount,
				"pending_validations": len(validations),
			},
			"updated_at": nowMs,
		})
	}
}

type statusScopeResult struct {
	Status         string          `json:"status"`
	HealthDot      string          `json:"health_dot"`
	HealthTitle    string          `json:"health_title"`
	Bars           []statusBarItem `json:"bars"`
	LatencyAvgMs   int64           `json:"latency_avg_ms"`
	TtftAvgMs      int64           `json:"ttft_avg_ms"`
	Availability7D *float64        `json:"availability_7d"`
	Requests7D     int64           `json:"requests_7d"`
	Requests24H    int64           `json:"requests_24h"`
	LastRequestTS  *int64          `json:"last_request_ts"`
}

type statusBarItem struct {
	TS           int64  `json:"ts"`
	Status       int    `json:"status"`
	LatencyMs    int64  `json:"latency_ms"`
	TtftMs       int64  `json:"ttft_ms"`
	Model        string `json:"model"`
	Requests     int64  `json:"requests"`
	SuccessCount int64  `json:"success_count"`
	Stream       bool   `json:"stream"`
}

type providerGroupResult struct {
	GroupID          string          `json:"group_id"`
	Name             string          `json:"name"`
	Kind             string          `json:"kind"`
	Provider         string          `json:"provider"`
	ProviderType     string          `json:"provider_type"`
	Accounts         int             `json:"accounts"`
	DisabledAccounts int             `json:"disabled_accounts"`
	Enabled          bool            `json:"enabled"`
	Status           string          `json:"status"`
	HealthDot        string          `json:"health_dot"`
	HealthTitle      string          `json:"health_title"`
	Bars             []statusBarItem `json:"bars"`
	LatencyAvgMs     int64           `json:"latency_avg_ms"`
	TtftAvgMs        int64           `json:"ttft_avg_ms"`
	Availability7D   *float64        `json:"availability_7d"`
	Requests7D       int64           `json:"requests_7d"`
	Requests24H      int64           `json:"requests_24h"`
	LastRequestTS    *int64          `json:"last_request_ts"`
}

func buildStatusScope(rows []db.RequestEventRow, nowMs int64) statusScopeResult {
	const slotCount = 60
	currentMinuteTs := (nowMs / 60000) * 60
	startMinuteTs := currentMinuteTs - int64(slotCount-1)*60

	type slotAccumulator struct {
		ts           int64
		requests     int64
		successCount int64
		hasErr       bool
		hasWarn      bool
		latencySum   int64
		latencyCount int64
		ttftSum      int64
		ttftCount    int64
		models       map[string]struct{}
	}

	slots := make([]slotAccumulator, slotCount)
	for i := 0; i < slotCount; i++ {
		minTs := startMinuteTs + int64(i)*60
		slots[i] = slotAccumulator{
			ts:     minTs * 1000,
			models: make(map[string]struct{}),
		}
	}

	totalLatSum := int64(0)
	totalLatCount := int64(0)
	totalTtftSum := int64(0)
	totalTtftCount := int64(0)
	okCount := int64(0)
	reqs24h := int64(0)
	dayAgoTs := (nowMs - 86400000) / 1000

	for _, r := range rows {
		if r.TS >= dayAgoTs {
			reqs24h++
		}
		if r.Status >= 200 && r.Status < 400 {
			okCount++
		}
		if r.LatencyMs > 0 {
			totalLatSum += r.LatencyMs
			totalLatCount++
		}
		if r.TtftMs != nil && *r.TtftMs > 0 {
			totalTtftSum += *r.TtftMs
			totalTtftCount++
		}

		if r.TS < startMinuteTs || r.TS >= currentMinuteTs+60 {
			continue
		}
		slotIdx := int((r.TS - startMinuteTs) / 60)
		if slotIdx >= 0 && slotIdx < slotCount {
			s := &slots[slotIdx]
			s.requests++
			if r.Status >= 200 && r.Status < 400 {
				s.successCount++
			} else if r.Status >= 500 || r.Status == 0 {
				s.hasErr = true
			} else if r.Status >= 400 {
				s.hasWarn = true
			}
			if r.LatencyMs > 0 {
				s.latencySum += r.LatencyMs
				s.latencyCount++
			}
			if r.TtftMs != nil && *r.TtftMs > 0 {
				s.ttftSum += *r.TtftMs
				s.ttftCount++
			}
			if r.Model != "" {
				s.models[r.Model] = struct{}{}
			}
		}
	}

	bars := make([]statusBarItem, slotCount)
	for i := 0; i < slotCount; i++ {
		s := &slots[i]
		status := 200
		if s.requests > 0 {
			if s.hasErr {
				status = 500
			} else if s.hasWarn {
				status = 429
			}
		}
		avgLat := int64(0)
		if s.latencyCount > 0 {
			avgLat = s.latencySum / s.latencyCount
		}
		avgTtft := int64(0)
		if s.ttftCount > 0 {
			avgTtft = s.ttftSum / s.ttftCount
		}

		var modelNames []string
		for m := range s.models {
			modelNames = append(modelNames, m)
			if len(modelNames) >= 2 {
				break
			}
		}

		bars[i] = statusBarItem{
			TS:           s.ts,
			Status:       status,
			LatencyMs:    avgLat,
			TtftMs:       avgTtft,
			Model:        strings.Join(modelNames, ", "),
			Requests:     s.requests,
			SuccessCount: s.successCount,
		}
	}

	var avail7d *float64
	if len(rows) > 0 {
		v := math.Round((float64(okCount)/float64(len(rows)))*10000.0) / 100.0
		avail7d = &v
	}

	avgLatTotal := int64(0)
	if totalLatCount > 0 {
		avgLatTotal = totalLatSum / totalLatCount
	}
	avgTtftTotal := int64(0)
	if totalTtftCount > 0 {
		avgTtftTotal = totalTtftSum / totalTtftCount
	}

	var lastReqTS *int64
	if len(rows) > 0 {
		tsMs := rows[0].TS * 1000
		lastReqTS = &tsMs
	}

	health := deriveGroupHealth(bars, avail7d, int64(len(rows)))
	scopeStatus := "operational"
	switch health.dot {
	case "err":
		scopeStatus = "major_outage"
	case "warn":
		scopeStatus = "degraded"
	case "idle":
		scopeStatus = "idle"
	case "":
		scopeStatus = "no_data"
	}

	return statusScopeResult{
		Status:         scopeStatus,
		HealthDot:      health.dot,
		HealthTitle:    health.title,
		Bars:           bars,
		LatencyAvgMs:   avgLatTotal,
		TtftAvgMs:      avgTtftTotal,
		Availability7D: avail7d,
		Requests7D:     int64(len(rows)),
		Requests24H:    reqs24h,
		LastRequestTS:  lastReqTS,
	}
}

type healthDecision struct {
	dot   string
	title string
}

func deriveGroupHealth(bars []statusBarItem, availability *float64, recentRequests int64) healthDecision {
	if len(bars) == 0 {
		return healthDecision{dot: "", title: "暂无数据"}
	}
	windowRequests := int64(0)
	for _, b := range bars {
		windowRequests += b.Requests
	}
	if windowRequests == 0 {
		if availability != nil && recentRequests > 0 {
			return healthDecision{dot: "idle", title: "近期无流量"}
		}
		return healthDecision{dot: "", title: "暂无数据"}
	}

	var activeBars []statusBarItem
	for _, b := range bars {
		if b.Requests > 0 {
			activeBars = append(activeBars, b)
		}
	}
	evalBars := bars
	if len(activeBars) > 0 {
		evalBars = activeBars
	}
	if len(evalBars) > 3 {
		evalBars = evalBars[len(evalBars)-3:]
	}

	recentBad := false
	recentWarn := false
	for _, b := range evalBars {
		if b.Status >= 500 || b.Status == 0 {
			recentBad = true
		} else if b.Status >= 400 && b.Status < 500 {
			recentWarn = true
		}
	}

	if recentBad {
		return healthDecision{dot: "err", title: "组内当前存在失败请求"}
	}
	if recentWarn {
		return healthDecision{dot: "warn", title: "组内当前存在 4xx 异常"}
	}
	if availability != nil && recentRequests >= 30 && *availability < 99.0 {
		return healthDecision{dot: "warn", title: "近期整体可用率偏低"}
	}
	return healthDecision{dot: "ok", title: "运行正常"}
}

func buildProviderGroups(rows []db.RequestEventRow, providers *ProviderRegistryEngine, nowMs int64) []providerGroupResult {
	type groupEntry struct {
		key      string
		name     string
		rows     []db.RequestEventRow
		accounts map[string]struct{}
	}

	groups := make(map[string]*groupEntry)
	for _, r := range rows {
		authClean := strings.TrimSpace(r.AuthIndex)
		// An event with no auth_index cannot be attributed to any credential, so
		// it belongs to no provider group. InferProvider would otherwise fall back
		// to model-name patterns and invent phantom groups: a stray "glm-5.3"
		// landed in "zhipu" and a "glm-5.3-flash-trial" in "zcode" beside the real
		// "zcode-cn" group, showing one upstream as three.
		if authClean == "" {
			continue
		}
		provRes := providers.InferProvider(r.Model, authClean)
		provKey := provRes.Key
		provLabel := provRes.Label

		// If unclassified or generic 'other', resolve the provider from the credential itself.
		if provKey == "" || provKey == "unknown" || provKey == UnknownProviderKey {
			if acc, ok := providers.GetAuthAccount(authClean); ok && (acc.ProviderKey != "" || acc.Name != "") {
				if acc.ProviderKey != "" && acc.ProviderKey != "openai-compatibility" && acc.ProviderKey != UnknownProviderKey {
					provKey = acc.ProviderKey
				} else if acc.Name != "" {
					provKey = fmt.Sprintf("compat:%s", strings.ToLower(strings.ReplaceAll(acc.Name, " ", "_")))
				} else {
					prefix := authClean
					if len(prefix) > 8 {
						prefix = prefix[:8]
					}
					provKey = fmt.Sprintf("upstream:%s", prefix)
				}
				if provLabel == "" {
					provLabel = acc.Name
				}
			} else {
				prefix := authClean
				if len(prefix) > 8 {
					prefix = prefix[:8]
				}
				provKey = fmt.Sprintf("upstream:%s", prefix)
				provLabel = fmt.Sprintf("上游渠道 %s", prefix)
			}
		}

		g, exists := groups[provKey]
		if !exists {
			label := providers.GetProviderGroupLabel(provKey, provLabel)
			g = &groupEntry{
				key:      provKey,
				name:     label,
				accounts: make(map[string]struct{}),
			}
			groups[provKey] = g
		}
		g.rows = append(g.rows, r)
		if r.AuthIndex != "" {
			g.accounts[r.AuthIndex] = struct{}{}
		}
	}

	var result []providerGroupResult
	for _, g := range groups {
		scope := buildStatusScope(g.rows, nowMs)
		groupStatus := scope.Status
		if len(g.accounts) == 0 && len(g.rows) == 0 {
			groupStatus = "no_data"
		}

		result = append(result, providerGroupResult{
			GroupID:          g.key,
			Name:             g.name,
			Kind:             "cpa",
			Provider:         g.key,
			ProviderType:     g.key,
			Accounts:         len(g.accounts),
			DisabledAccounts: 0,
			Enabled:          true,
			Status:           groupStatus,
			HealthDot:        scope.HealthDot,
			HealthTitle:      scope.HealthTitle,
			Bars:             scope.Bars,
			LatencyAvgMs:     scope.LatencyAvgMs,
			TtftAvgMs:        scope.TtftAvgMs,
			Availability7D:   scope.Availability7D,
			Requests7D:       scope.Requests7D,
			Requests24H:      scope.Requests24H,
			LastRequestTS:    scope.LastRequestTS,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Requests7D > result[j].Requests7D
	})

	return result
}
