// Antigravity 上游额度解析与冷却对齐 —— 拆自 quota_parse.go
package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

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

// reconcileAntigravityQuotaGroups reconciles raw Antigravity upstream quota groups
// with active conductor cooldowns.
func reconcileAntigravityQuotaGroups(groups []CpaQuotaGroup, cooldowns []CpaCooldownRecord) []CpaQuotaGroup {
	var activeCds []CpaCooldownRecord
	hasCredCd := false
	var claudeCds []CpaCooldownRecord
	var geminiCds []CpaCooldownRecord
	var gemini4Cds []CpaCooldownRecord

	for _, cd := range cooldowns {
		if cd.RemainingSeconds <= 0 {
			continue
		}
		activeCds = append(activeCds, cd)
		if cd.Scope == "credential" {
			hasCredCd = true
		} else if cd.Scope == "model" {
			mk := strings.ToLower(strings.TrimSpace(cd.ModelKey))
			isGemini4 := strings.Contains(mk, "argon") || strings.Contains(mk, "gemini-4") ||
				strings.Contains(mk, "gemini4") || strings.Contains(mk, "gemini 4")
			isClaude := strings.Contains(mk, "claude") || strings.Contains(mk, "gpt") ||
				strings.Contains(mk, "sonnet") || strings.Contains(mk, "opus") ||
				strings.Contains(mk, "haiku")
			isGemini := !isGemini4 && (strings.Contains(mk, "gemini") || strings.Contains(mk, "flash") ||
				(strings.Contains(mk, "pro") && !strings.Contains(mk, "claude")))

			if isGemini4 {
				gemini4Cds = append(gemini4Cds, cd)
			} else if isClaude {
				claudeCds = append(claudeCds, cd)
			} else if isGemini {
				geminiCds = append(geminiCds, cd)
			} else {
				// All non-Gemini non-Argon models (Claude, GPT, DeepSeek, Llama, Qwen, etc.) share the Antigravity 3P quota pool
				claudeCds = append(claudeCds, cd)
			}
		}
	}

	if !hasCredCd && len(claudeCds) == 0 && len(geminiCds) == 0 && len(gemini4Cds) == 0 {
		return groups
	}

	reconciled := make([]CpaQuotaGroup, len(groups))
	hasArgonGroup := false
	for i, grp := range groups {
		gCopy := grp
		gName := strings.ToLower(fmt.Sprintf("%s %s %s", grp.GroupID, grp.DisplayName, grp.Name))
		isGemini4 := strings.Contains(gName, "argon") || strings.Contains(gName, "gemini 4") ||
			strings.Contains(gName, "gemini-4") || strings.Contains(gName, "gemini4")
		if isGemini4 {
			hasArgonGroup = true
		}
		isClaude := strings.Contains(gName, "claude") || strings.Contains(gName, "gpt") || strings.Contains(gName, "3p")
		isGemini := !isGemini4 && strings.Contains(gName, "gemini")

		var targetCds []CpaCooldownRecord
		if hasCredCd {
			targetCds = activeCds
		} else if isGemini4 {
			targetCds = gemini4Cds
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

	if len(gemini4Cds) > 0 && !hasArgonGroup {
		zeroFrac := float64(0)
		zeroPct := 0
		var latestRetryStr string
		for _, cd := range gemini4Cds {
			if cd.RetryAt != "" && cd.RetryAt > latestRetryStr {
				latestRetryStr = cd.RetryAt
			}
		}
		argonGrp := CpaQuotaGroup{
			Name:        "models/gemini4_argon",
			DisplayName: "Gemini 4 Argon",
			Description: "",
			Buckets: []CpaQuotaBucket{
				{
					BucketID:          "gemini-argon-wtu-weekly",
					Name:              "Gemini 4 Argon Weekly Limit",
					Window:            "weekly",
					RemainingFraction: &zeroFrac,
					RemainingPercent:  &zeroPct,
					ResetTime:         latestRetryStr,
				},
			},
		}
		reconciled = append(reconciled, argonGrp)
	}

	return reconciled
}
