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
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"github.com/tidwall/gjson"
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
				if strings.Contains(wLower, "gemini") {
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
			if strings.Contains(gLower, "gemini") {
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
			is5h := strings.Contains(wLower, "5h") || strings.Contains(wLower, "five") || strings.Contains(wLower, "hour")

			bucketName := win
			if isWeekly {
				win = "weekly"
				bucketName = "Weekly Limit Remaining"
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

func normalizeUsedPercent(used float64) (*float64, *int) {
	if used > 1.0 {
		rem := 100.0 - used
		return normalizePercentAndFraction(rem)
	}
	remFrac := 1.0 - used
	return normalizeFractionAndPercent(remFrac)
}

func normalizeUsedFraction(used float64) (*float64, *int) {
	return normalizeUsedPercent(used)
}

func parseAntigravityQuotaGroups(raw []byte) []CpaQuotaGroup {
	if len(raw) == 0 {
		return nil
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil
	}
	rawGroups, ok := root["groups"].([]any)
	if !ok || len(rawGroups) == 0 {
		return nil
	}

	var res []CpaQuotaGroup
	for _, rg := range rawGroups {
		grpMap, ok := rg.(map[string]any)
		if !ok {
			continue
		}

		var gName, gDisp string
		if v, ok := grpMap["displayName"].(string); ok && strings.TrimSpace(v) != "" {
			gDisp = strings.TrimSpace(v)
		}
		if v, ok := grpMap["name"].(string); ok && strings.TrimSpace(v) != "" {
			gName = strings.TrimSpace(v)
		}
		if (gName == "" || gName == "<nil>") && gDisp != "" && gDisp != "<nil>" {
			gName = gDisp
		}
		if (gDisp == "" || gDisp == "<nil>") && gName != "" && gName != "<nil>" {
			gDisp = gName
		}

		gLower := strings.ToLower(fmt.Sprintf("%s %s", gName, gDisp))
		if strings.Contains(gLower, "argon") || strings.Contains(gLower, "gemini 4") || strings.Contains(gLower, "gemini-4") || strings.Contains(gLower, "gemini4") {
			if gName == "" || gName == "<nil>" || gName == "Model Group" {
				gName = "models/gemini4_argon"
			}
			if gDisp == "" || gDisp == "<nil>" {
				gDisp = "Gemini 4 Argon"
			}
		} else if strings.Contains(gLower, "gemini") {
			if gName == "" || gName == "<nil>" || gName == "Model Group" {
				gName = "models/gemini"
			}
			if gDisp == "" || gDisp == "<nil>" {
				gDisp = "Gemini Models"
			}
		} else if strings.Contains(gLower, "claude") || strings.Contains(gLower, "gpt") || strings.Contains(gLower, "3p") {
			if gName == "" || gName == "<nil>" || gName == "Model Group" {
				gName = "models/claude"
			}
			if gDisp == "" || gDisp == "<nil>" {
				gDisp = "Claude and GPT models"
			}
		}

		gDesc := ""
		if v, ok := grpMap["description"].(string); ok {
			gDesc = v
		}

		grp := CpaQuotaGroup{
			Name:        gName,
			DisplayName: gDisp,
			Description: gDesc,
		}

		rawBuckets, _ := grpMap["buckets"].([]any)
		for _, rb := range rawBuckets {
			bMap, ok := rb.(map[string]any)
			if !ok {
				continue
			}

			var bID string
			if v, ok := bMap["bucketId"].(string); ok && strings.TrimSpace(v) != "" {
				bID = strings.TrimSpace(v)
			} else if v, ok := bMap["bucket_id"].(string); ok && strings.TrimSpace(v) != "" {
				bID = strings.TrimSpace(v)
			}

			var bDisp string
			if v, ok := bMap["displayName"].(string); ok && strings.TrimSpace(v) != "" {
				bDisp = strings.TrimSpace(v)
			} else if v, ok := bMap["name"].(string); ok && strings.TrimSpace(v) != "" {
				bDisp = strings.TrimSpace(v)
			}

			var win string
			if v, ok := bMap["window"].(string); ok && strings.TrimSpace(v) != "" {
				win = strings.TrimSpace(v)
			}

			bLower := strings.ToLower(fmt.Sprintf("%s %s %s", bID, bDisp, win))
			if strings.Contains(bLower, "weekly") || strings.Contains(bLower, "week") {
				win = "weekly"
				if bDisp == "" {
					bDisp = "Weekly Limit"
				}
				if bID == "" {
					bID = fmt.Sprintf("%s-week", gName)
				}
			} else {
				win = "5h"
				if bDisp == "" {
					bDisp = "5-Hour Limit"
				}
				if bID == "" {
					bID = fmt.Sprintf("%s-5h", gName)
				}
			}

			disabled := false
			if d, ok := bMap["disabled"].(bool); ok {
				disabled = d
			}

			resetTime := ""
			if rt, ok := bMap["resetTime"].(string); ok && rt != "" {
				resetTime = rt
			} else if rt, ok := bMap["reset_time"].(string); ok && rt != "" {
				resetTime = rt
			} else if rt, ok := bMap["resetTimeSnake"].(string); ok && rt != "" {
				resetTime = rt
			} else if f, ok := extractFloat(bMap["resetTime"]); ok && f > 1000000000 {
				resetTime = time.Unix(int64(f), 0).UTC().Format(time.RFC3339)
			} else if f, ok := extractFloat(bMap["reset_time"]); ok && f > 1000000000 {
				resetTime = time.Unix(int64(f), 0).UTC().Format(time.RFC3339)
			}

			var remFrac *float64
			var remPct *int

			if f, ok := extractFloat(bMap["remainingFraction"]); ok {
				remFrac, remPct = normalizeFractionAndPercent(f)
			} else if f, ok := extractFloat(bMap["remaining_fraction"]); ok {
				remFrac, remPct = normalizeFractionAndPercent(f)
			} else if f, ok := extractFloat(bMap["remainingPercent"]); ok {
				remFrac, remPct = normalizePercentAndFraction(f)
			} else if f, ok := extractFloat(bMap["remaining_percent"]); ok {
				remFrac, remPct = normalizePercentAndFraction(f)
			} else if f, ok := extractFloat(bMap["usedPercent"]); ok {
				remFrac, remPct = normalizeUsedPercent(f)
			} else if f, ok := extractFloat(bMap["used_percent"]); ok {
				remFrac, remPct = normalizeUsedPercent(f)
			} else if f, ok := extractFloat(bMap["usedFraction"]); ok {
				remFrac, remPct = normalizeUsedFraction(f)
			} else if f, ok := extractFloat(bMap["used_fraction"]); ok {
				remFrac, remPct = normalizeUsedFraction(f)
			}

			countdown := formatResetCountdown(resetTime)

			grp.Buckets = append(grp.Buckets, CpaQuotaBucket{
				BucketID:          bID,
				Window:            win,
				Name:              bDisp,
				RemainingFraction: remFrac,
				RemainingPercent:  remPct,
				ResetTime:         resetTime,
				ResetTimeSnake:    resetTime,
				ResetCountdown:    countdown,
				Disabled:          disabled,
			})
		}
		if len(grp.Buckets) > 0 {
			sort.Slice(grp.Buckets, func(i, j int) bool {
				iIs5h := grp.Buckets[i].Window == "5h" || strings.Contains(strings.ToLower(grp.Buckets[i].Name), "5h") || strings.Contains(strings.ToLower(grp.Buckets[i].Name), "five")
				jIs5h := grp.Buckets[j].Window == "5h" || strings.Contains(strings.ToLower(grp.Buckets[j].Name), "5h") || strings.Contains(strings.ToLower(grp.Buckets[j].Name), "five")
				if iIs5h && !jIs5h {
					return true
				}
				if !iIs5h && jIs5h {
					return false
				}
				return grp.Buckets[i].Window < grp.Buckets[j].Window
			})
			res = append(res, grp)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		iIsGemini := strings.Contains(strings.ToLower(res[i].Name), "gemini") || strings.Contains(strings.ToLower(res[i].DisplayName), "gemini")
		jIsGemini := strings.Contains(strings.ToLower(res[j].Name), "gemini") || strings.Contains(strings.ToLower(res[j].DisplayName), "gemini")
		if iIsGemini && !jIsGemini {
			return true
		}
		if !iIsGemini && jIsGemini {
			return false
		}
		return res[i].DisplayName < res[j].DisplayName
	})
	return res
}

func parseCodexQuotaGroups(raw []byte) ([]CpaQuotaGroup, string, string) {
	if len(raw) == 0 {
		return nil, "free", "免费版"
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, "free", "免费版"
	}

	planType := "plus"
	if pt, ok := root["plan_type"].(string); ok && pt != "" {
		planType = strings.ToLower(pt)
	} else if pt, ok := root["planType"].(string); ok && pt != "" {
		planType = strings.ToLower(pt)
	}
	planName := strings.ToUpper(planType)

	var primaryMap, secondaryMap map[string]any
	findWindows := func(m map[string]any) (map[string]any, map[string]any) {
		if m == nil {
			return nil, nil
		}
		var p, s map[string]any
		for _, k := range []string{"primary_window", "primaryWindow", "primary", "five_hour", "5h", "rolling"} {
			if sub, ok := m[k].(map[string]any); ok {
				p = sub
				break
			}
		}
		for _, k := range []string{"secondary_window", "secondaryWindow", "secondary", "weekly", "week"} {
			if sub, ok := m[k].(map[string]any); ok {
				s = sub
				break
			}
		}
		return p, s
	}

	if rl, ok := root["rate_limit"].(map[string]any); ok {
		primaryMap, secondaryMap = findWindows(rl)
	} else if rl, ok := root["rate_limits"].(map[string]any); ok {
		primaryMap, secondaryMap = findWindows(rl)
	}
	if primaryMap == nil && secondaryMap == nil {
		primaryMap, secondaryMap = findWindows(root)
	}

	parseWindow := func(wMap map[string]any, defaultID, defaultWindow, defaultName string) *CpaQuotaBucket {
		if wMap == nil {
			return nil
		}
		var remFrac *float64
		var remPct *int

		if f, ok := extractFloat(wMap["remaining_percent"]); ok {
			remFrac, remPct = normalizePercentAndFraction(f)
		} else if f, ok := extractFloat(wMap["remainingPercent"]); ok {
			remFrac, remPct = normalizePercentAndFraction(f)
		} else if f, ok := extractFloat(wMap["remaining_fraction"]); ok {
			remFrac, remPct = normalizeFractionAndPercent(f)
		} else if f, ok := extractFloat(wMap["remainingFraction"]); ok {
			remFrac, remPct = normalizeFractionAndPercent(f)
		} else if f, ok := extractFloat(wMap["used_percent"]); ok {
			remFrac, remPct = normalizeUsedPercent(f)
		} else if f, ok := extractFloat(wMap["usedPercent"]); ok {
			remFrac, remPct = normalizeUsedPercent(f)
		} else if f, ok := extractFloat(wMap["used_fraction"]); ok {
			remFrac, remPct = normalizeUsedFraction(f)
		} else if f, ok := extractFloat(wMap["usedFraction"]); ok {
			remFrac, remPct = normalizeUsedFraction(f)
		} else {
			defaultFrac, defaultPct := 1.0, 100
			remFrac, remPct = &defaultFrac, &defaultPct
		}

		var resetTimeStr string
		if resetAt, ok := extractFloat(wMap["reset_at"]); ok && resetAt > 0 {
			resetTimeStr = time.Unix(int64(resetAt), 0).UTC().Format(time.RFC3339)
		} else if resetAt, ok := extractFloat(wMap["resetAt"]); ok && resetAt > 0 {
			resetTimeStr = time.Unix(int64(resetAt), 0).UTC().Format(time.RFC3339)
		} else if resetSec, ok := extractFloat(wMap["reset_after_seconds"]); ok && resetSec > 0 {
			resetTimeStr = time.Now().UTC().Add(time.Duration(resetSec) * time.Second).Format(time.RFC3339)
		} else if resetSec, ok := extractFloat(wMap["resetAfterSeconds"]); ok && resetSec > 0 {
			resetTimeStr = time.Now().UTC().Add(time.Duration(resetSec) * time.Second).Format(time.RFC3339)
		} else if rt, ok := wMap["reset_time"].(string); ok && rt != "" {
			resetTimeStr = rt
		} else if rt, ok := wMap["resetTime"].(string); ok && rt != "" {
			resetTimeStr = rt
		}

		countdown := formatResetCountdown(resetTimeStr)

		disabled := false
		if remPct != nil && *remPct <= 0 {
			disabled = true
		}

		return &CpaQuotaBucket{
			BucketID:          defaultID,
			Window:            defaultWindow,
			Name:              defaultName,
			RemainingFraction: remFrac,
			RemainingPercent:  remPct,
			ResetTime:         resetTimeStr,
			ResetTimeSnake:    resetTimeStr,
			ResetCountdown:    countdown,
			Disabled:          disabled,
		}
	}

	var buckets []CpaQuotaBucket
	if b := parseWindow(primaryMap, "codex-5h", "5h", "5 Hour Limit Remaining"); b != nil {
		buckets = append(buckets, *b)
	}
	if b := parseWindow(secondaryMap, "codex-weekly", "weekly", "Weekly Limit Remaining"); b != nil {
		buckets = append(buckets, *b)
	}

	if len(buckets) == 0 {
		return nil, planType, planName
	}

	return []CpaQuotaGroup{
		{
			Name:        "codex",
			DisplayName: "Codex",
			Buckets:     buckets,
		},
	}, planType, planName
}

// parseClaudeQuotaGroups converts Anthropic OAuth usage JSON to normalized quota groups.
func parseClaudeQuotaGroups(raw []byte) []CpaQuotaGroup {
	if len(raw) == 0 {
		return nil
	}
	parsed := gjson.ParseBytes(raw)
	var buckets []CpaQuotaBucket
	addBucket := func(key, window, name, desc string) {
		node := parsed.Get(key)
		if !node.Exists() {
			return
		}
		uNode := node.Get("utilization")
		if !uNode.Exists() {
			return
		}
		uVal := uNode.Float()
		remPct := int(math.Round((1.0 - uVal) * 100.0))
		if remPct < 0 {
			remPct = 0
		} else if remPct > 100 {
			remPct = 100
		}
		remFrac := float64(remPct) / 100.0
		resetsAt := node.Get("resets_at").String()
		countdown := formatResetCountdown(resetsAt)
		buckets = append(buckets, CpaQuotaBucket{
			BucketID:          fmt.Sprintf("claude-%s", window),
			Window:            window,
			Name:              name,
			Description:       desc,
			RemainingFraction: &remFrac,
			RemainingPercent:  &remPct,
			ResetTime:         resetsAt,
			ResetTimeSnake:    resetsAt,
			ResetCountdown:    countdown,
			Disabled:          remPct <= 0,
		})
	}
	addBucket("five_hour", "5h", "5-Hour Limit", "5小时滚动限制")
	addBucket("seven_day", "weekly", "Weekly Limit Remaining", "7天总限制")
	addBucket("seven_day_sonnet", "7d_sonnet", "Sonnet 7-Day Limit", "7天 Sonnet 限制")
	addBucket("seven_day_opus", "7d_opus", "Opus 7-Day Limit", "7天 Opus 限制")

	if len(buckets) == 0 {
		return nil
	}
	return []CpaQuotaGroup{{
		GroupID:     "models/claude",
		Name:        "models/claude",
		DisplayName: "Claude 模型",
		Buckets:     buckets,
	}}
}

// parsePluginQuotaResponse maps pluginapi.QuotaFetchResponse JSON into CpaQuotaGroup, planType, planName.
func parsePluginQuotaResponse(raw []byte, prov string) ([]CpaQuotaGroup, string, string) {
	if len(raw) == 0 {
		return nil, "standard", "标准包"
	}
	var resp pluginapi.QuotaFetchResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, "standard", "标准包"
	}

	planType := "standard"
	planName := "标准包"
	if resp.Subscription != nil {
		if resp.Subscription.Plan != "" {
			planName = resp.Subscription.Plan
		} else if resp.Subscription.TierName != "" {
			planName = resp.Subscription.TierName
		}
		pLower := strings.ToLower(planName)
		if strings.Contains(pLower, "pro") || strings.Contains(pLower, "plus") || strings.Contains(pLower, "team") || strings.Contains(pLower, "max") {
			planType = "pro"
		} else if strings.Contains(pLower, "free") {
			planType = "free"
		}
	}

	var totalPoints, usedPoints, remainPoints *float64
	var expAtStr string
	for _, m := range resp.Summary {
		v := m.Value
		switch m.Key {
		case "points_total", "total", "quota":
			totalPoints = &v
		case "points_used", "used":
			usedPoints = &v
		case "points_remain", "remaining", "balance", "points":
			remainPoints = &v
		case "expires_at", "runs_out_at":
			if m.Unit != "" && expAtStr == "" {
				expAtStr = m.Unit
			}
		}
	}

	var groups []CpaQuotaGroup
	for idx, g := range resp.Groups {
		dispName := g.DisplayName
		if dispName == "" {
			dispName = fmt.Sprintf("配额池 #%d", idx+1)
		}
		var buckets []CpaQuotaBucket
		for bIdx, b := range g.Buckets {
			remFrac := b.RemainingFraction
			if remFrac < 0 {
				remFrac = 0
			} else if remFrac > 1 {
				remFrac = 1
			}
			remPct := int(math.Round(remFrac * 100.0))
			countdown := formatResetCountdown(b.ResetTime)
			bName := b.Description
			if bName == "" {
				bName = b.Window
			}
			if bName == "" {
				bName = fmt.Sprintf("额度窗口 #%d", bIdx+1)
			}
			bkt := CpaQuotaBucket{
				BucketID:          fmt.Sprintf("%s-%s-%d", prov, b.Window, bIdx),
				Window:            b.Window,
				Name:              bName,
				Description:       b.Description,
				RemainingFraction: &remFrac,
				RemainingPercent:  &remPct,
				ResetTime:         b.ResetTime,
				ResetTimeSnake:    b.ResetTime,
				ResetCountdown:    countdown,
				Disabled:          remPct <= 0,
			}
			// Only attach account-wide points to the primary bucket (first bucket of first group)
			// to avoid duplicating points when summing bucket capacities across buckets.
			if idx == 0 && bIdx == 0 {
				if totalPoints != nil {
					bkt.TotalCapacity = totalPoints
				}
				if remainPoints != nil {
					bkt.RemainCapacity = remainPoints
				}
				if usedPoints != nil {
					bkt.UsedCapacity = usedPoints
				}
			}
			if expAtStr != "" {
				bkt.ExpiresAt = expAtStr
			} else if b.ResetTime != "" {
				bkt.ExpiresAt = b.ResetTime
			}
			buckets = append(buckets, bkt)
		}
		groups = append(groups, CpaQuotaGroup{
			GroupID:     fmt.Sprintf("%s-%d", prov, idx),
			Name:        dispName,
			DisplayName: dispName,
			Buckets:     buckets,
		})
	}

	// If no groups but summary has remaining points, construct a synthetic group
	if len(groups) == 0 && remainPoints != nil {
		remFrac := 1.0
		if totalPoints != nil && *totalPoints > 0 {
			remFrac = *remainPoints / *totalPoints
			if remFrac < 0 {
				remFrac = 0
			} else if remFrac > 1 {
				remFrac = 1
			}
		}
		remPct := int(math.Round(remFrac * 100.0))
		bkt := CpaQuotaBucket{
			BucketID:          fmt.Sprintf("%s-points", prov),
			Window:            "points",
			Name:              "资源额度点数",
			Description:       "可用资源额度",
			RemainingFraction: &remFrac,
			RemainingPercent:  &remPct,
			TotalCapacity:     totalPoints,
			RemainCapacity:    remainPoints,
			UsedCapacity:      usedPoints,
			Disabled:          remPct <= 0,
			ExpiresAt:         expAtStr,
		}
		groups = append(groups, CpaQuotaGroup{
			GroupID:     fmt.Sprintf("%s-quota", prov),
			Name:        "资源点数",
			DisplayName: "资源点数",
			Buckets:     []CpaQuotaBucket{bkt},
		})
	}

	return groups, planType, planName
}

// reconcileAntigravityQuotaGroups reconciles raw Antigravity upstream quota groups
// with active conductor cooldowns.
func reconcileAntigravityQuotaGroups(groups []CpaQuotaGroup, cooldowns []CpaCooldownRecord) []CpaQuotaGroup {
	var activeCds []CpaCooldownRecord
	hasCredCd := false
	var claudeCds []CpaCooldownRecord
	var geminiCds []CpaCooldownRecord

	for _, cd := range cooldowns {
		if cd.RemainingSeconds <= 0 {
			continue
		}
		activeCds = append(activeCds, cd)
		if cd.Scope == "credential" {
			hasCredCd = true
		} else if cd.Scope == "model" {
			mk := strings.ToLower(strings.TrimSpace(cd.ModelKey))
			isGemini := strings.Contains(mk, "gemini") || strings.Contains(mk, "flash") ||
				(strings.Contains(mk, "pro") && !strings.Contains(mk, "claude"))
			if isGemini {
				geminiCds = append(geminiCds, cd)
			} else {
				// All non-Gemini models (Claude, GPT, DeepSeek, Llama, Qwen, etc.) share the Antigravity 3P quota pool
				claudeCds = append(claudeCds, cd)
			}
		}
	}

	if !hasCredCd && len(claudeCds) == 0 && len(geminiCds) == 0 {
		return groups
	}

	reconciled := make([]CpaQuotaGroup, len(groups))
	for i, grp := range groups {
		gCopy := grp
		gName := strings.ToLower(fmt.Sprintf("%s %s %s", grp.GroupID, grp.DisplayName, grp.Name))
		isGemini := strings.Contains(gName, "gemini")
		isClaude := !isGemini || strings.Contains(gName, "claude") || strings.Contains(gName, "gpt")

		var targetCds []CpaCooldownRecord
		if hasCredCd {
			targetCds = activeCds
		} else if isGemini {
			targetCds = geminiCds
		} else if isClaude {
			targetCds = claudeCds
		}

		if len(targetCds) == 0 {
			reconciled[i] = gCopy
			continue
		}

		maxRem := 0
		var latestRetryStr string
		for _, cd := range targetCds {
			if cd.RemainingSeconds > maxRem {
				maxRem = cd.RemainingSeconds
			}
			if cd.RetryAt != "" && cd.RetryAt > latestRetryStr {
				latestRetryStr = cd.RetryAt
			}
		}

		bktCopies := make([]CpaQuotaBucket, len(grp.Buckets))
		for j, bkt := range grp.Buckets {
			bCopy := bkt
			bName := strings.ToLower(fmt.Sprintf("%s %s %s %s", bkt.BucketID, bkt.Name, bkt.Description, bkt.Window))
			is5h := strings.Contains(bName, "5h") || strings.Contains(bName, "5 hour") || strings.Contains(bName, "five")
			isWeekly := strings.Contains(bName, "weekly") || strings.Contains(bName, "week")

			isBlocked := false
			if hasCredCd {
				isBlocked = true
			} else if maxRem > 5*3600 {
				// Cooldown > 5h implies weekly rate limit exhaustion. All buckets blocked.
				isBlocked = true
			} else if is5h || !isWeekly {
				isBlocked = true
			}

			if isBlocked {
				zeroFrac := float64(0)
				zeroPct := 0
				bCopy.RemainingFraction = &zeroFrac
				bCopy.RemainingPercent = &zeroPct
				if latestRetryStr != "" {
					bCopy.ResetTime = latestRetryStr
				}
			}
			bktCopies[j] = bCopy
		}
		gCopy.Buckets = bktCopies
		reconciled[i] = gCopy
	}
	return reconciled
}
