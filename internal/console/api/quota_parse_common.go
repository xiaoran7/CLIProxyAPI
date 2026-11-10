// 上游额度解析通用辅助与快照注水 —— 拆自 quota_parse.go
package api

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
)

func formatResetCountdown(isoTime string) string {
	if strings.TrimSpace(isoTime) == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, isoTime)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, isoTime)
		if err != nil {
			return ""
		}
	}
	diff := time.Until(t)
	if diff <= 0 {
		return "即将重置"
	}
	days := int(diff.Hours()) / 24
	hours := int(diff.Hours()) % 24
	mins := int(diff.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%d 天 %d 小时", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%d 小时 %d 分", hours, mins)
	}
	if mins > 0 {
		return fmt.Sprintf("%d 分钟", mins)
	}
	return "即将重置"
}

func hydrateQuotaGroupsFromSnapshots(snapshots []db.QuotaSnapshotRow) (map[string][]CpaQuotaGroup, map[string]int64) {
	authMap := make(map[string][]db.QuotaSnapshotRow)
	latestTSMap := make(map[string]int64)
	for _, s := range snapshots {
		authMap[s.AuthIndex] = append(authMap[s.AuthIndex], s)
		if s.TS > latestTSMap[s.AuthIndex] {
			latestTSMap[s.AuthIndex] = s.TS
		}
	}

	res := make(map[string][]CpaQuotaGroup)
	for authIdx, rows := range authMap {
		groupBuckets := make(map[string][]CpaQuotaBucket)
		groupDispNames := make(map[string]string)
		// seenBucket dedupes on the canonical group plus window: the same window
		// can arrive under both name shapes ("Gemini Models:5h" and
		// "Gemini Models - Five Hour Limit Remaining"), and appending both
		// rendered a stale duplicate meter beside the real one.
		seenBucket := make(map[string]bool, len(rows))

		for _, r := range rows {
			winName := ""
			if r.WindowName != nil {
				winName = *r.WindowName
			}
			var grpName, win string
			if strings.Contains(winName, ":") {
				parts := strings.SplitN(winName, ":", 2)
				grpName = strings.TrimSpace(parts[0])
				win = strings.TrimSpace(parts[1])
			} else if strings.Contains(winName, " - ") {
				parts := strings.SplitN(winName, " - ", 2)
				grpName = strings.TrimSpace(parts[0])
				win = strings.TrimSpace(parts[1])
			} else {
				win = winName
			}

			if grpName == "" || grpName == "<nil>" || grpName == "Quota" {
				wLower := strings.ToLower(winName)
				if strings.Contains(wLower, "argon") || strings.Contains(wLower, "gemini 4") ||
					strings.Contains(wLower, "gemini-4") || strings.Contains(wLower, "gemini4") {
					grpName = "Gemini 4 Argon"
				} else if strings.Contains(wLower, "gemini") {
					grpName = "Gemini Models"
				} else if strings.Contains(wLower, "claude") || strings.Contains(wLower, "gpt") || strings.Contains(wLower, "3p") {
					grpName = "Claude Models"
				} else if strings.Contains(wLower, "codex") || r.Provider == "codex" {
					grpName = "Codex"
				} else {
					grpName = "Standard"
				}
			}

			dispName := grpName
			gLower := strings.ToLower(grpName)
			canonicalName := grpName
			// Argon is its own pool and must be canonicalized before the plain
			// Gemini check: "Gemini 4 Argon" contains "gemini", so collapsing it
			// into models/gemini made the dedup below keep whichever row came
			// first and report the exhausted Argon window as the Gemini window's
			// remaining quota (observed: Argon 0% shown for a Gemini pool at 88%).
			if strings.Contains(gLower, "argon") || strings.Contains(gLower, "gemini 4") ||
				strings.Contains(gLower, "gemini-4") || strings.Contains(gLower, "gemini4") {
				canonicalName = "models/gemini4_argon"
				dispName = "Gemini 4 Argon"
			} else if strings.Contains(gLower, "gemini") {
				canonicalName = "models/gemini"
				dispName = "Gemini Models"
			} else if strings.Contains(gLower, "claude") || strings.Contains(gLower, "gpt") || strings.Contains(gLower, "3p") {
				canonicalName = "models/claude"
				dispName = "Claude Models"
			} else if strings.Contains(gLower, "codex") {
				canonicalName = "codex"
				dispName = "Codex"
			}

			remPct := int(math.Round(r.RemainingPercent))
			if remPct < 0 {
				remPct = 0
			}
			if remPct > 100 {
				remPct = 100
			}
			frac := float64(remPct) / 100.0

			var resetTimeStr string
			if r.ResetAt != nil && *r.ResetAt > 0 {
				resetTimeStr = time.Unix(*r.ResetAt, 0).UTC().Format(time.RFC3339)
			}
			countdown := formatResetCountdown(resetTimeStr)

			wLower := strings.ToLower(win)
			isWeekly := strings.Contains(wLower, "week")
			isMonthly := strings.Contains(wLower, "month")
			is5h := strings.Contains(wLower, "5h") || strings.Contains(wLower, "five hour") || strings.Contains(wLower, "5 hour") || strings.Contains(wLower, "five-hour") || strings.Contains(wLower, "5-hour")

			bucketName := win
			if isWeekly {
				win = "weekly"
				bucketName = "Weekly Limit Remaining"
			} else if isMonthly {
				win = "monthly"
				bucketName = "Monthly Limit Remaining"
			} else if is5h || win == "" {
				win = "5h"
				bucketName = "5-Hour Limit"
			}

			bkt := CpaQuotaBucket{
				BucketID:          fmt.Sprintf("%s:%s", canonicalName, win),
				Window:            win,
				Name:              bucketName,
				RemainingFraction: &frac,
				RemainingPercent:  &remPct,
				ResetTime:         resetTimeStr,
				ResetTimeSnake:    resetTimeStr,
				ResetCountdown:    countdown,
				Disabled:          remPct <= 0,
			}
			bucketKey := canonicalName + "|" + win
			if seenBucket[bucketKey] {
				continue
			}
			seenBucket[bucketKey] = true
			groupBuckets[canonicalName] = append(groupBuckets[canonicalName], bkt)
			groupDispNames[canonicalName] = dispName
		}

		var groups []CpaQuotaGroup
		for cName, bkts := range groupBuckets {
			sort.Slice(bkts, func(i, j int) bool {
				iIs5h := bkts[i].Window == "5h" || strings.Contains(strings.ToLower(bkts[i].Name), "5h") || strings.Contains(strings.ToLower(bkts[i].Name), "five")
				jIs5h := bkts[j].Window == "5h" || strings.Contains(strings.ToLower(bkts[j].Name), "5h") || strings.Contains(strings.ToLower(bkts[j].Name), "five")
				if iIs5h && !jIs5h {
					return true
				}
				if !iIs5h && jIs5h {
					return false
				}
				return bkts[i].Window < bkts[j].Window
			})
			groups = append(groups, CpaQuotaGroup{
				Name:        cName,
				DisplayName: groupDispNames[cName],
				Buckets:     bkts,
			})
		}
		sort.Slice(groups, func(i, j int) bool {
			iIsGemini := strings.Contains(strings.ToLower(groups[i].Name), "gemini") || strings.Contains(strings.ToLower(groups[i].DisplayName), "gemini")
			jIsGemini := strings.Contains(strings.ToLower(groups[j].Name), "gemini") || strings.Contains(strings.ToLower(groups[j].DisplayName), "gemini")
			if iIsGemini && !jIsGemini {
				return true
			}
			if !iIsGemini && jIsGemini {
				return false
			}
			return groups[i].DisplayName < groups[j].DisplayName
		})
		res[authIdx] = groups
	}
	return res, latestTSMap
}

func extractFloat(val any) (float64, bool) {
	if val == nil {
		return 0, false
	}
	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f, true
		}
	case string:
		v = strings.TrimSpace(v)
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func normalizeFractionAndPercent(f float64) (*float64, *int) {
	if f > 1.0 {
		return normalizePercentAndFraction(f)
	}
	if f < 0 {
		f = 0
	}
	pct := int(math.Round(f * 100.0))
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return &f, &pct
}

func normalizePercentAndFraction(p float64) (*float64, *int) {
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	pct := int(math.Round(p))
	frac := p / 100.0
	return &frac, &pct
}

// normalizeUsedPercent converts a used percentage (0-100) to the remaining
// percentage and fraction. The field name states the unit, so no heuristic is
// applied: a used_percent of 1 means "1% used", leaving 99%. Guessing from the
// magnitude treated 1 as a fraction (100% used) and reported a fresh weekly
// quota as fully exhausted.
func normalizeUsedPercent(used float64) (*float64, *int) {
	return normalizePercentAndFraction(100.0 - used)
}

// normalizeUsedFraction converts a used fraction (0-1) to the remaining
// percentage and fraction. As above, the field name carries the unit.
func normalizeUsedFraction(used float64) (*float64, *int) {
	if used < 0 {
		used = 0
	}
	if used > 1.0 {
		used = 1.0
	}
	remFrac := 1.0 - used
	return normalizeFractionAndPercent(remFrac)
}
