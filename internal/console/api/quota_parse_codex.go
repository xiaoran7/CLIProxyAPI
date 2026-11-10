// Codex 上游额度解析 —— 拆自 quota_parse.go
package api

import (
	"encoding/json"
	"strings"
	"time"
)

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
	if secondaryMap != nil {
		secWindowSec, _ := extractFloat(secondaryMap["limit_window_seconds"])
		if secWindowSec == 0 {
			secWindowSec, _ = extractFloat(secondaryMap["limitWindowSeconds"])
		}
		isMonthly := secWindowSec >= 2419200 // 28 days or longer
		secID := "codex-weekly"
		secWin := "weekly"
		secName := "Weekly Limit Remaining"
		if isMonthly {
			secID = "codex-monthly"
			secWin = "monthly"
			secName = "Monthly Limit Remaining"
		}
		if b := parseWindow(secondaryMap, secID, secWin, secName); b != nil {
			buckets = append(buckets, *b)
		}
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
