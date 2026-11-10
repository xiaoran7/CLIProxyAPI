// Claude 及通用插件上游额度解析 —— 拆自 quota_parse.go
package api

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"github.com/tidwall/gjson"
)

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
