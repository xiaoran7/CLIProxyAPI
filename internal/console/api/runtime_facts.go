package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// RuntimeFactsWindowPreset defines time window configuration for telemetry breakdown.
type RuntimeFactsWindowPreset struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	ShortLabel    string `json:"short_label"`
	WindowMinutes int    `json:"window_minutes"`
	BucketMinutes int    `json:"bucket_minutes"`
	BucketCount   int    `json:"bucket_count"`
}

var (
	chinaTzLocation = time.FixedZone("CST", 8*3600)
	chinaTzOffset   = int64(8 * 3600)
)

var runtimeFactsWindows = []RuntimeFactsWindowPreset{
	{ID: "1h", Label: "近 1 小时", ShortLabel: "1h", WindowMinutes: 60, BucketMinutes: 5, BucketCount: 12},
	{ID: "6h", Label: "近 6 小时", ShortLabel: "6h", WindowMinutes: 360, BucketMinutes: 15, BucketCount: 24},
	{ID: "1d", Label: "近 1 天", ShortLabel: "1d", WindowMinutes: 1440, BucketMinutes: 60, BucketCount: 24},
	{ID: "3d", Label: "近 3 天", ShortLabel: "3d", WindowMinutes: 4320, BucketMinutes: 180, BucketCount: 24},
	{ID: "7d", Label: "近 7 天", ShortLabel: "7d", WindowMinutes: 10080, BucketMinutes: 360, BucketCount: 28},
	{ID: "30d", Label: "近 30 天", ShortLabel: "30d", WindowMinutes: 43200, BucketMinutes: 1440, BucketCount: 30},
}

// legacyWindowPresets maintains backward compatibility for historical presets.
var legacyWindowPresets = []RuntimeFactsWindowPreset{
	{ID: "3h20m", Label: "近 3 小时 20 分", ShortLabel: "3h20m", WindowMinutes: 200, BucketMinutes: 10, BucketCount: 20},
	{ID: "5h", Label: "近 5 小时", ShortLabel: "5h", WindowMinutes: 300, BucketMinutes: 10, BucketCount: 30},
}

func resolveWindowPreset(windowParam string) RuntimeFactsWindowPreset {
	norm := strings.ToLower(strings.TrimSpace(windowParam))
	switch norm {
	case "12h", "24h":
		norm = "1d"
	}
	for _, w := range runtimeFactsWindows {
		if strings.EqualFold(w.ID, norm) {
			return w
		}
	}
	for _, w := range legacyWindowPresets {
		if strings.EqualFold(w.ID, norm) {
			return w
		}
	}
	return runtimeFactsWindows[0]
}

// RecentBucket represents a time-slotted request count entry.
type RecentBucket struct {
	Time    string `json:"time"`
	Success int64  `json:"success"`
	Failed  int64  `json:"failed"`
	Start   int64  `json:"start,omitempty"`
	End     int64  `json:"end,omitempty"`
}

func formatWindowBucketTime(startSec, endSec int64, bucketMinutes int) string {
	t1 := time.Unix(startSec, 0).In(chinaTzLocation)
	t2 := time.Unix(endSec, 0).In(chinaTzLocation)

	if bucketMinutes < 60 {
		return fmt.Sprintf("%02d:%02d-%02d:%02d", t1.Hour(), t1.Minute(), t2.Hour(), t2.Minute())
	}
	if bucketMinutes >= 1440 {
		return fmt.Sprintf("%02d-%02d", t1.Month(), t1.Day())
	}
	if t1.Month() == t2.Month() && t1.Day() == t2.Day() {
		return fmt.Sprintf("%02d-%02d %02d:%02d-%02d:%02d", t1.Month(), t1.Day(), t1.Hour(), t1.Minute(), t2.Hour(), t2.Minute())
	}
	return fmt.Sprintf("%02d-%02d %02d:%02d-%02d-%02d %02d:%02d", t1.Month(), t1.Day(), t1.Hour(), t1.Minute(), t2.Month(), t2.Day(), t2.Hour(), t2.Minute())
}

func alignAndSum(sequences [][]RecentBucket, bucketCount int) []RecentBucket {
	result := make([]RecentBucket, bucketCount)
	var nonEmpty [][]RecentBucket
	maxWidth := 0
	for _, seq := range sequences {
		if len(seq) > 0 {
			nonEmpty = append(nonEmpty, seq)
			if len(seq) > maxWidth {
				maxWidth = len(seq)
			}
		}
	}
	if len(nonEmpty) == 0 {
		return result
	}
	if maxWidth > bucketCount {
		maxWidth = bucketCount
	}
	offset := bucketCount - maxWidth
	for _, seq := range nonEmpty {
		seqLen := len(seq)
		if seqLen > bucketCount {
			seq = seq[seqLen-bucketCount:]
			seqLen = bucketCount
		}
		seqOffset := offset + (maxWidth - seqLen)
		for i, b := range seq {
			idx := seqOffset + i
			if idx >= 0 && idx < bucketCount {
				result[idx].Success += b.Success
				result[idx].Failed += b.Failed
				if result[idx].Time == "" && b.Time != "" {
					result[idx].Time = b.Time
				}
				if result[idx].Start == 0 && b.Start != 0 {
					result[idx].Start = b.Start
					result[idx].End = b.End
				}
			}
		}
	}
	return result
}

func getLocalTrafficBuckets(ctx context.Context, eng *engine.Engine, preset RuntimeFactsWindowPreset, nowSec int64) []RecentBucket {
	bucketSeconds := int64(preset.BucketMinutes * 60)
	localCurrentSec := nowSec + chinaTzOffset
	localCurrentSlot := localCurrentSec / bucketSeconds
	minSlot := localCurrentSlot - int64(preset.BucketCount) + 1
	minTs := minSlot*bucketSeconds - chinaTzOffset
	maxTs := (localCurrentSlot+1)*bucketSeconds - chinaTzOffset

	type slotEntry struct {
		success int64
		failed  int64
	}
	countMap := make(map[int64]*slotEntry)

	// 1. Query SQLite window aggregate
	if eng != nil && eng.DB() != nil {
		rows, err := eng.DB().SelectEventsWindowAggregate(ctx, chinaTzOffset, bucketSeconds, minTs, maxTs)
		if err == nil {
			for _, r := range rows {
				countMap[r.SlotStart] = &slotEntry{
					success: r.Success,
					failed:  r.Failed,
				}
			}
		}
	}

	// 2. Overlay unflushed events in usage_outbox
	if eng != nil && eng.DB() != nil {
		outbox, err := eng.DB().SelectAllOutbox(ctx)
		if err == nil {
			for _, ev := range outbox {
				if ev.TS >= minTs && ev.TS < maxTs {
					localTs := ev.TS + chinaTzOffset
					slotStart := (localTs/bucketSeconds)*bucketSeconds - chinaTzOffset
					entry := countMap[slotStart]
					if entry == nil {
						entry = &slotEntry{}
						countMap[slotStart] = entry
					}
					if ev.Status >= 200 && ev.Status < 400 {
						entry.success++
					} else {
						entry.failed++
					}
				}
			}
		}
	}

	// 3. Overlay in-memory pending events
	if eng != nil {
		pending := eng.PeekPendingEvents()
		for _, ev := range pending {
			if ev != nil && ev.TS >= minTs && ev.TS < maxTs {
				localTs := ev.TS + chinaTzOffset
				slotStart := (localTs/bucketSeconds)*bucketSeconds - chinaTzOffset
				entry := countMap[slotStart]
				if entry == nil {
					entry = &slotEntry{}
					countMap[slotStart] = entry
				}
				if ev.Status >= 200 && ev.Status < 400 {
					entry.success++
				} else {
					entry.failed++
				}
			}
		}
	}

	// 4. Build contiguous buckets slice
	buckets := make([]RecentBucket, preset.BucketCount)
	for i := 0; i < preset.BucketCount; i++ {
		slot := minSlot + int64(i)
		startSec := slot*bucketSeconds - chinaTzOffset
		endSec := (slot+1)*bucketSeconds - chinaTzOffset
		var s, f int64
		if entry := countMap[startSec]; entry != nil {
			s = entry.success
			f = entry.failed
		}
		buckets[i] = RecentBucket{
			Time:    formatWindowBucketTime(startSec, endSec, preset.BucketMinutes),
			Success: s,
			Failed:  f,
			Start:   startSec,
			End:     endSec,
		}
	}

	return buckets
}

// HandleGetRuntimeFacts returns runtime traffic telemetry with graceful degraded mode.
func HandleGetRuntimeFacts(eng *engine.Engine, cpa *CpaClient, providers *ProviderRegistryEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		now := time.Now()
		nowSec := now.Unix()
		nowMs := now.UnixMilli()

		windowParam := strings.TrimSpace(c.Query("window"))
		matchedPreset := resolveWindowPreset(windowParam)

		// 1. Fetch CPA auth-files
		var authFilesResp struct {
			ObservedAt time.Time `json:"observed_at"`
			Files      []struct {
				Name           string              `json:"name"`
				AuthIndex      string              `json:"auth_index"`
				Provider       string              `json:"provider"`
				Disabled       bool                `json:"disabled"`
				Unavailable    bool                `json:"unavailable"`
				Status         string              `json:"status"`
				StatusMessage  string              `json:"status_message"`
				Email          string              `json:"email"`
				Label          string              `json:"label"`
				Nickname       string              `json:"nickname"`
				Priority       int                 `json:"priority"`
				Success        int64               `json:"success"`
				Failed         int64               `json:"failed"`
				Cooldowns      []rawCooldownRecord `json:"cooldowns"`
				RecentRequests []RecentBucket      `json:"recent_requests"`
			} `json:"files"`
		}

		cpaErr := cpa.RequestJSON(ctx, http.MethodGet, "/auth-files", nil, "v0", &authFilesResp)
		if cpaErr != nil {
			// Degraded payload: return HTTP 200 with degraded status so frontend renders gracefully
			c.JSON(http.StatusOK, gin.H{
				"source":            "cpa_runtime",
				"degraded":          true,
				"error":             cpaErr.Error(),
				"fetched_at":        nowMs,
				"window":            matchedPreset.ID,
				"window_label":      matchedPreset.Label,
				"window_minutes":    matchedPreset.WindowMinutes,
				"bucket_minutes":    matchedPreset.BucketMinutes,
				"bucket_count":      matchedPreset.BucketCount,
				"available_windows": runtimeFactsWindows,
				"totals": gin.H{
					"credentials_total":    0,
					"credentials_active":   0,
					"credentials_disabled": 0,
					"credentials_cooldown": 0,
					"credentials_blocked":  0,
					"scope":                "cumulative",
					"buckets":              []interface{}{},
					"success_count":        0,
					"failure_count":        0,
					"total_requests":       0,
					"success_rate":         nil,
					"window_minutes":       matchedPreset.WindowMinutes,
					"bucket_minutes":       matchedPreset.BucketMinutes,
					"bucket_count":         matchedPreset.BucketCount,
					"source":               "cpa_runtime",
				},
				"providers":   []interface{}{},
				"credentials": []interface{}{},
			})
			return
		}

		// 2. Fetch CPA api-key-usage
		var apiKeyUsageResp map[string]map[string]struct {
			Success        int64          `json:"success"`
			Failed         int64          `json:"failed"`
			RecentRequests []RecentBucket `json:"recent_requests"`
		}
		_ = cpa.RequestJSON(ctx, http.MethodGet, "/api-key-usage", nil, "v0", &apiKeyUsageResp)

		observedAtMs := nowMs
		if !authFilesResp.ObservedAt.IsZero() {
			observedAtMs = authFilesResp.ObservedAt.UnixMilli()
		}

		// 3. Query native gate local buckets (authoritative dual-track SQLite + Outbox + Memory)
		localBuckets := getLocalTrafficBuckets(ctx, eng, matchedPreset, nowSec)
		hasLocalTraffic := false
		for _, b := range localBuckets {
			if b.Success > 0 || b.Failed > 0 {
				hasLocalTraffic = true
				break
			}
		}

		type CredentialFact struct {
			AuthIndex     string         `json:"auth_index"`
			Name          string         `json:"name"`
			DisplayName   string         `json:"display_name"`
			Nickname      string         `json:"nickname"`
			Email         string         `json:"email"`
			Provider      string         `json:"provider"`
			Priority      int            `json:"priority"`
			Status        string         `json:"status"`
			Disabled      bool           `json:"disabled"`
			Unavailable   bool           `json:"unavailable"`
			Success       int64          `json:"success"`
			Failed        int64          `json:"failed"`
			Health        AuthHealthInfo `json:"health"`
			Buckets       []RecentBucket `json:"buckets"`
			WindowMinutes int            `json:"window_minutes"`
		}

		type ProviderFact struct {
			Provider      string         `json:"provider"`
			KeyCount      int            `json:"key_count"`
			TotalSuccess  int64          `json:"total_success"`
			TotalFailed   int64          `json:"total_failed"`
			Total         int64          `json:"total"`
			SuccessRate   *float64       `json:"success_rate"`
			Buckets       []RecentBucket `json:"buckets"`
			WindowMinutes int            `json:"window_minutes"`
		}

		type providerCollector struct {
			provider     string
			keyCount     int
			totalSuccess int64
			totalFailed  int64
			bucketSeqs   [][]RecentBucket
		}
		provCollectorMap := make(map[string]*providerCollector)

		var credFacts []CredentialFact
		var credBucketSeqs [][]RecentBucket
		var cumulativeSuccess, cumulativeFailed int64
		activeCount := 0
		disabledCount := 0
		cooldownCount := 0
		blockedCount := 0
		hasWindowTraffic := false

		for _, f := range authFilesResp.Files {
			prov := strings.ToLower(strings.TrimSpace(f.Provider))
			if prov == "" {
				prov = strings.ToLower(providers.InferProvider("", f.AuthIndex).Key)
			}
			if prov == "" {
				prov = "general"
			}

			cooldown := interpretCooldowns(f.Cooldowns, observedAtMs, nowMs)
			health := getAuthHealth(cooldown, f.Disabled, f.Unavailable, f.Status, f.StatusMessage)

			if f.Disabled {
				disabledCount++
			} else {
				activeCount++
			}
			if health.State == "cooldown" {
				cooldownCount++
			}
			if health.Blocked {
				blockedCount++
			}

			cumulativeSuccess += f.Success
			cumulativeFailed += f.Failed

			displayName := resolveAuthAccountDisplayName(f.Name, f.AuthIndex, f.Email, f.Label, f.Nickname)
			credBuckets := f.RecentRequests
			if len(credBuckets) > 0 {
				credBucketSeqs = append(credBucketSeqs, credBuckets)
				if !hasWindowTraffic {
					for _, b := range credBuckets {
						if b.Success > 0 || b.Failed > 0 {
							hasWindowTraffic = true
							break
						}
					}
				}
			}

			credFacts = append(credFacts, CredentialFact{
				AuthIndex:     f.AuthIndex,
				Name:          displayName,
				DisplayName:   displayName,
				Nickname:      f.Nickname,
				Email:         f.Email,
				Provider:      prov,
				Priority:      f.Priority,
				Status:        f.Status,
				Disabled:      f.Disabled,
				Unavailable:   f.Unavailable,
				Success:       f.Success,
				Failed:        f.Failed,
				Health:        health,
				Buckets:       credBuckets,
				WindowMinutes: matchedPreset.WindowMinutes,
			})

			pCol := provCollectorMap[prov]
			if pCol == nil {
				pCol = &providerCollector{provider: prov}
				provCollectorMap[prov] = pCol
			}
			pCol.keyCount++
			pCol.totalSuccess += f.Success
			pCol.totalFailed += f.Failed
			if len(credBuckets) > 0 {
				pCol.bucketSeqs = append(pCol.bucketSeqs, credBuckets)
			}
		}

		// Merge API key usage providers
		for pRaw, keyMap := range apiKeyUsageResp {
			pNorm := strings.ToLower(strings.TrimSpace(pRaw))
			if pNorm == "" {
				pNorm = "unknown"
			}
			pCol := provCollectorMap[pNorm]
			if pCol == nil {
				pCol = &providerCollector{provider: pNorm}
				provCollectorMap[pNorm] = pCol
			}
			for _, stats := range keyMap {
				pCol.keyCount++
				pCol.totalSuccess += stats.Success
				pCol.totalFailed += stats.Failed
				cumulativeSuccess += stats.Success
				cumulativeFailed += stats.Failed
				if len(stats.RecentRequests) > 0 {
					pCol.bucketSeqs = append(pCol.bucketSeqs, stats.RecentRequests)
					credBucketSeqs = append(credBucketSeqs, stats.RecentRequests)
					if !hasWindowTraffic {
						for _, b := range stats.RecentRequests {
							if b.Success > 0 || b.Failed > 0 {
								hasWindowTraffic = true
								break
							}
						}
					}
				}
			}
		}

		var provList []ProviderFact
		for _, pCol := range provCollectorMap {
			total := pCol.totalSuccess + pCol.totalFailed
			var sRate *float64
			if total > 0 {
				r := float64(pCol.totalSuccess) / float64(total) * 100
				r = float64(int(r*100+0.5)) / 100
				sRate = &r
			}
			pBuckets := alignAndSum(pCol.bucketSeqs, matchedPreset.BucketCount)
			provList = append(provList, ProviderFact{
				Provider:      pCol.provider,
				KeyCount:      pCol.keyCount,
				TotalSuccess:  pCol.totalSuccess,
				TotalFailed:   pCol.totalFailed,
				Total:         total,
				SuccessRate:   sRate,
				Buckets:       pBuckets,
				WindowMinutes: matchedPreset.WindowMinutes,
			})
		}
		sort.Slice(provList, func(i, j int) bool {
			if provList[i].Total != provList[j].Total {
				return provList[i].Total > provList[j].Total
			}
			return provList[i].Provider < provList[j].Provider
		})

		// Decide throughput buckets:
		// Gate local buckets are authoritative; if empty and CPA has window traffic, fall back to CPA ring buffer when bucket granularity matches
		useLocal := hasLocalTraffic || !hasWindowTraffic || matchedPreset.BucketMinutes != 10
		var finalBuckets []RecentBucket
		var finalSource string
		if useLocal {
			finalBuckets = localBuckets
			if hasLocalTraffic {
				finalSource = "gate_local"
			} else {
				finalSource = "cpa_runtime"
			}
		} else {
			finalBuckets = alignAndSum(credBucketSeqs, matchedPreset.BucketCount)
			finalSource = "cpa_runtime"
		}

		var windowSuccess, windowFailed int64
		for _, b := range finalBuckets {
			windowSuccess += b.Success
			windowFailed += b.Failed
		}

		effectiveTraffic := hasLocalTraffic || hasWindowTraffic
		var finalSuccess, finalFailed int64
		var scope string
		if effectiveTraffic {
			finalSuccess = windowSuccess
			finalFailed = windowFailed
			scope = "window"
		} else {
			finalSuccess = cumulativeSuccess
			finalFailed = cumulativeFailed
			scope = "cumulative"
		}

		totalReqs := finalSuccess + finalFailed
		var successRate *float64
		if totalReqs > 0 {
			rate := float64(finalSuccess) / float64(totalReqs) * 100
			rate = float64(int(rate*100+0.5)) / 100
			successRate = &rate
		}

		c.JSON(http.StatusOK, gin.H{
			"source":            finalSource,
			"degraded":          false,
			"fetched_at":        nowMs,
			"window":            matchedPreset.ID,
			"window_label":      matchedPreset.Label,
			"window_minutes":    matchedPreset.WindowMinutes,
			"bucket_minutes":    matchedPreset.BucketMinutes,
			"bucket_count":      matchedPreset.BucketCount,
			"available_windows": runtimeFactsWindows,
			"totals": gin.H{
				"credentials_total":    len(credFacts),
				"credentials_active":   activeCount,
				"credentials_disabled": disabledCount,
				"credentials_cooldown": cooldownCount,
				"credentials_blocked":  blockedCount,
				"scope":                scope,
				"buckets":              finalBuckets,
				"success_count":        finalSuccess,
				"failure_count":        finalFailed,
				"total_requests":       totalReqs,
				"success_rate":         successRate,
				"window_minutes":       matchedPreset.WindowMinutes,
				"bucket_minutes":       matchedPreset.BucketMinutes,
				"bucket_count":         matchedPreset.BucketCount,
				"source":               finalSource,
			},
			"providers":   provList,
			"credentials": credFacts,
		})
	}
}
