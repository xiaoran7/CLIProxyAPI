// Claude/插件/快照注水及健康状态额度测试 —— 拆自 handlers_quotas_test.go
package api

import (
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
)

func TestComputeHealthModelCooldownNotBlockingCredential(t *testing.T) {
	t.Parallel()

	// Model-scoped cooldown should NOT set Blocked: true
	fileModelCooling := CpaAuthFile{
		Name:     "antigravity-01.json",
		Provider: "antigravity",
		Status:   "active",
		Cooldowns: []CpaCooldownRecord{
			{
				Scope:            "models",
				ModelKey:         "claude-3-5-sonnet",
				RemainingSeconds: 3600,
			},
		},
	}
	hModel := computeHealth(fileModelCooling)
	if hModel.Blocked {
		t.Errorf("model-scoped cooldown should NOT mark entire credential as blocked")
	}
	if hModel.Cooldown == nil || hModel.Cooldown.Scope != "models" {
		t.Errorf("expected Cooldown info to be populated with scope 'models'")
	}
	if hModel.Label != "部分模型冷却" {
		t.Errorf("expected Label '部分模型冷却', got %q", hModel.Label)
	}
	if !strings.Contains(hModel.Detail, "claude-3-5-sonnet") {
		t.Errorf("expected Detail to mention 'claude-3-5-sonnet', got %q", hModel.Detail)
	}

	// Credential-scoped cooldown MUST set Blocked: true
	fileCredCooling := CpaAuthFile{
		Name:     "antigravity-02.json",
		Provider: "antigravity",
		Status:   "active",
		Cooldowns: []CpaCooldownRecord{
			{
				Scope:            "credential",
				RemainingSeconds: 7200,
			},
		},
	}
	hCred := computeHealth(fileCredCooling)
	if !hCred.Blocked {
		t.Errorf("credential-scoped cooldown MUST mark credential as blocked")
	}
	if hCred.Label != "冷却中" {
		t.Errorf("expected Label '冷却中', got %q", hCred.Label)
	}
}

func TestParseClaudeQuotaGroups(t *testing.T) {
	t.Parallel()

	rawJSON := []byte(`{
		"five_hour": {
			"utilization": 0.25,
			"resets_at": "2026-10-22T18:00:00Z"
		},
		"seven_day": {
			"utilization": 0.60,
			"resets_at": "2026-10-28T00:00:00Z"
		},
		"seven_day_sonnet": {
			"utilization": 0.10,
			"resets_at": "2026-10-28T00:00:00Z"
		}
	}`)

	groups := parseClaudeQuotaGroups(rawJSON)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].DisplayName != "Claude 模型" {
		t.Errorf("expected Claude 模型, got %s", groups[0].DisplayName)
	}
	if len(groups[0].Buckets) != 3 {
		t.Fatalf("expected 3 buckets, got %d", len(groups[0].Buckets))
	}

	// 5h bucket: 1.0 - 0.25 = 75%
	b5h := groups[0].Buckets[0]
	if b5h.Window != "5h" {
		t.Errorf("expected window '5h', got %s", b5h.Window)
	}
	if b5h.RemainingPercent == nil || *b5h.RemainingPercent != 75 {
		t.Errorf("expected 75%% remaining, got %v", b5h.RemainingPercent)
	}

	// 7d bucket: 1.0 - 0.60 = 40%
	b7d := groups[0].Buckets[1]
	if b7d.Window != "weekly" {
		t.Errorf("expected window 'weekly', got %s", b7d.Window)
	}
	if b7d.RemainingPercent == nil || *b7d.RemainingPercent != 40 {
		t.Errorf("expected 40%% remaining, got %v", b7d.RemainingPercent)
	}
}

func TestParsePluginQuotaResponse(t *testing.T) {
	t.Parallel()

	rawJSON := []byte(`{
		"subscription": {
			"plan": "Qoder Global (en) · Pro"
		},
		"summary": [
			{"key": "points_remain", "label": "Available Points", "value": 850.5},
			{"key": "points_total", "label": "Total Quota", "value": 1000.0},
			{"key": "points_used", "label": "Used Quota", "value": 149.5}
		],
		"groups": [
			{
				"displayName": "Qoder Resource Quota",
				"buckets": [
					{
						"window": "monthly",
						"remainingFraction": 0.8505,
						"description": "Monthly Resource Quota",
						"resetTime": "2026-11-01T00:00:00Z"
					}
				]
			}
		]
	}`)

	groups, planType, planName := parsePluginQuotaResponse(rawJSON, "qoder")
	if planType != "pro" {
		t.Errorf("expected planType 'pro', got %q", planType)
	}
	if planName != "Qoder Global (en) · Pro" {
		t.Errorf("expected planName, got %q", planName)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].Buckets) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(groups[0].Buckets))
	}

	b := groups[0].Buckets[0]
	if b.RemainingPercent == nil || *b.RemainingPercent != 85 {
		t.Errorf("expected 85%% remaining, got %v", b.RemainingPercent)
	}
	if b.TotalCapacity == nil || *b.TotalCapacity != 1000.0 {
		t.Errorf("expected total capacity 1000.0, got %v", b.TotalCapacity)
	}
	if b.RemainCapacity == nil || *b.RemainCapacity != 850.5 {
		t.Errorf("expected remain capacity 850.5, got %v", b.RemainCapacity)
	}

	tot, rem, used, exp := extractAccountCredits(groups)
	if tot == nil || *tot != 1000.0 {
		t.Errorf("expected 1000 total credits, got %v", tot)
	}
	if rem == nil || *rem != 850.5 {
		t.Errorf("expected 850.5 remaining credits, got %v", rem)
	}
	if used == nil || *used != 149.5 {
		t.Errorf("expected 149.5 used credits, got %v", used)
	}
	if exp != "2026-11-01T00:00:00Z" {
		t.Errorf("expected expiration timestamp, got %q", exp)
	}
}

func TestParsePluginQuotaResponse_WorkBuddy(t *testing.T) {
	t.Parallel()

	rawJSON := []byte(`{
		"summary": [
			{"key": "points_remain", "label": "可用点数", "value": 350.0, "unit": "点"},
			{"key": "points_total", "label": "总点数", "value": 350.0, "unit": "点"},
			{"key": "points_used", "label": "已用点数", "value": 0.0, "unit": "点"},
			{"key": "runs_out_points", "label": "即将过期点数", "value": 250.0, "unit": "点"},
			{"key": "runs_out_at", "label": "最近过期时间", "value": 1793279074000, "unit": "2026-10-29 21:04:34 CST"}
		],
		"groups": [
			{
				"groupId": "workbuddy-resource-quota",
				"displayName": "WorkBuddy Resource Quota",
				"buckets": [
					{
						"bucketId": "workbuddy-points",
						"name": "可用点数",
						"window": "points",
						"remainingFraction": 1.0,
						"description": "350 可用 / 350 总点数"
					},
					{
						"bucketId": "TCACA_code_123",
						"name": "套餐包",
						"window": "points",
						"remainingFraction": 1.0,
						"description": "套餐包 (250 点)"
					}
				]
			}
		]
	}`)

	groups, planType, planName := parsePluginQuotaResponse(rawJSON, "workbuddy")
	if planType != "standard" {
		t.Errorf("expected planType 'standard', got %q", planType)
	}
	if planName != "标准包" {
		t.Errorf("expected planName '标准包', got %q", planName)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].Buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(groups[0].Buckets))
	}

	// Verify primary bucket holds points
	b0 := groups[0].Buckets[0]
	if b0.TotalCapacity == nil || *b0.TotalCapacity != 350.0 {
		t.Errorf("expected b0 total capacity 350, got %v", b0.TotalCapacity)
	}
	if b0.RemainCapacity == nil || *b0.RemainCapacity != 350.0 {
		t.Errorf("expected b0 remain capacity 350, got %v", b0.RemainCapacity)
	}
	if b0.ExpiresAt != "2026-10-29 21:04:34 CST" {
		t.Errorf("expected b0 expiresAt '2026-10-29 21:04:34 CST', got %q", b0.ExpiresAt)
	}

	// Verify secondary bucket does NOT duplicate points to avoid multiplier bugs
	b1 := groups[0].Buckets[1]
	if b1.TotalCapacity != nil {
		t.Errorf("secondary bucket must not duplicate TotalCapacity, got %v", b1.TotalCapacity)
	}

	// Verify extracted account credits are not duplicated across multiple buckets
	tot, rem, used, exp := extractAccountCredits(groups)
	if tot == nil || *tot != 350.0 {
		t.Errorf("expected 350 total credits, got %v", tot)
	}
	if rem == nil || *rem != 350.0 {
		t.Errorf("expected 350 remaining credits, got %v", rem)
	}
	if used == nil || *used != 0.0 {
		t.Errorf("expected 0 used credits, got %v", used)
	}
	if exp != "2026-10-29 21:04:34 CST" {
		t.Errorf("expected expiration timestamp '2026-10-29 21:04:34 CST', got %q", exp)
	}
}

func TestIsSupportedQuotaProvider(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		prov     string
		file     CpaAuthFile
		expected bool
	}{
		{"antigravity", CpaAuthFile{}, true},
		{"codex", CpaAuthFile{}, true},
		{"claude", CpaAuthFile{}, true},
		{"workbuddy", CpaAuthFile{}, true},
		{"workbuddy-cn", CpaAuthFile{}, true},
		{"qoder", CpaAuthFile{}, true},
		{"qoder-cn", CpaAuthFile{}, true},
		{"qoder-intl", CpaAuthFile{}, true},
		{"copilot-tencent", CpaAuthFile{}, true},
		{"kimi", CpaAuthFile{}, true},
		{"xai", CpaAuthFile{}, true},
		{"devin", CpaAuthFile{}, true},
		{"generic", CpaAuthFile{SupportsQuota: true}, true},
		{"custom", CpaAuthFile{QuotaProvider: "plugin-xyz"}, true},
		{"unknown", CpaAuthFile{}, false},
	}

	for _, tc := range testCases {
		res := isSupportedQuotaProvider(tc.prov, tc.file)
		if res != tc.expected {
			t.Errorf("provider %q expected %v, got %v", tc.prov, tc.expected, res)
		}
	}
}

func TestHydrateQuotaGroupsFromSnapshots(t *testing.T) {
	t.Parallel()

	resetTS := time.Now().Add(1 * time.Hour).Unix()
	geminiWin := "models/gemini:5h"
	claudeWin := "models/claude:weekly"
	snapshots := []db.QuotaSnapshotRow{
		{
			AuthIndex:        "antigravity-01",
			Provider:         "antigravity",
			WindowName:       &geminiWin,
			RemainingPercent: 80,
			ResetAt:          &resetTS,
			TS:               1729600000,
		},
		{
			AuthIndex:        "antigravity-01",
			Provider:         "antigravity",
			WindowName:       &claudeWin,
			RemainingPercent: 45,
			ResetAt:          &resetTS,
			TS:               1729600000,
		},
	}

	resultMap, latestTSMap := hydrateQuotaGroupsFromSnapshots(snapshots)
	if latestTSMap["antigravity-01"] != 1729600000 {
		t.Errorf("expected latest TS 1729600000, got %v", latestTSMap["antigravity-01"])
	}
	groups, ok := resultMap["antigravity-01"]
	if !ok || len(groups) != 2 {
		t.Fatalf("expected 2 groups for antigravity-01, got %v", len(groups))
	}

	geminiFound := false
	claudeFound := false
	for _, g := range groups {
		if g.Name == "models/gemini" {
			geminiFound = true
			if len(g.Buckets) != 1 || *g.Buckets[0].RemainingPercent != 80 {
				t.Errorf("expected 80%% for gemini bucket, got %v", g.Buckets[0].RemainingPercent)
			}
		}
		if g.Name == "models/claude" {
			claudeFound = true
			if len(g.Buckets) != 1 || *g.Buckets[0].RemainingPercent != 45 {
				t.Errorf("expected 45%% for claude bucket, got %v", g.Buckets[0].RemainingPercent)
			}
		}
	}

	if !geminiFound || !claudeFound {
		t.Errorf("expected both gemini and claude groups to be hydrated, got gemini=%v, claude=%v", geminiFound, claudeFound)
	}
}

func TestHydrateQuotaGroupsFromSnapshots_HyphenDelimitedAndGeminiFirst(t *testing.T) {
	t.Parallel()

	resetTS := time.Now().Add(2 * time.Hour).Unix()
	gemini5h := "Gemini Models - Five Hour Limit Remaining"
	geminiWeek := "Gemini Models - Weekly Limit Remaining"
	claude5h := "Claude and GPT models - Five Hour Limit Remaining"
	claudeWeek := "Claude and GPT models - Weekly Limit Remaining"

	snapshots := []db.QuotaSnapshotRow{
		{
			AuthIndex:        "antigravity-hyphen",
			Provider:         "antigravity",
			WindowName:       &claude5h,
			RemainingPercent: 100,
			ResetAt:          &resetTS,
			TS:               1729600010,
		},
		{
			AuthIndex:        "antigravity-hyphen",
			Provider:         "antigravity",
			WindowName:       &geminiWeek,
			RemainingPercent: 90,
			ResetAt:          &resetTS,
			TS:               1729600010,
		},
		{
			AuthIndex:        "antigravity-hyphen",
			Provider:         "antigravity",
			WindowName:       &gemini5h,
			RemainingPercent: 54,
			ResetAt:          &resetTS,
			TS:               1729600010,
		},
		{
			AuthIndex:        "antigravity-hyphen",
			Provider:         "antigravity",
			WindowName:       &claudeWeek,
			RemainingPercent: 95,
			ResetAt:          &resetTS,
			TS:               1729600010,
		},
	}

	resultMap, _ := hydrateQuotaGroupsFromSnapshots(snapshots)
	groups, ok := resultMap["antigravity-hyphen"]
	if !ok || len(groups) != 2 {
		t.Fatalf("expected 2 groups for antigravity-hyphen, got %v", len(groups))
	}

	var geminiGroup, claudeGroup *CpaQuotaGroup
	for i := range groups {
		if groups[i].Name == "models/gemini" {
			geminiGroup = &groups[i]
		} else if groups[i].Name == "models/claude" {
			claudeGroup = &groups[i]
		}
	}

	if geminiGroup == nil {
		t.Fatalf("expected models/gemini group to be present")
	}
	if claudeGroup == nil {
		t.Fatalf("expected models/claude group to be present")
	}

	// 2. Bucket 0 in Gemini MUST be 5h
	if len(geminiGroup.Buckets) != 2 {
		t.Fatalf("expected 2 buckets in gemini group, got %d", len(geminiGroup.Buckets))
	}
	if geminiGroup.Buckets[0].Window != "5h" {
		t.Errorf("expected gemini bucket 0 to be 5h, got %q", geminiGroup.Buckets[0].Window)
	}
	if *geminiGroup.Buckets[0].RemainingPercent != 54 {
		t.Errorf("expected gemini 5h remainingPercent = 54, got %v", *geminiGroup.Buckets[0].RemainingPercent)
	}
	if geminiGroup.Buckets[1].Window != "weekly" {
		t.Errorf("expected gemini bucket 1 to be weekly, got %q", geminiGroup.Buckets[1].Window)
	}
	if *geminiGroup.Buckets[1].RemainingPercent != 90 {
		t.Errorf("expected gemini weekly remainingPercent = 90, got %v", *geminiGroup.Buckets[1].RemainingPercent)
	}

	// 3. Bucket 0 in Claude MUST be 5h
	if len(claudeGroup.Buckets) != 2 {
		t.Fatalf("expected 2 buckets in claude group, got %d", len(claudeGroup.Buckets))
	}
	if claudeGroup.Buckets[0].Window != "5h" {
		t.Errorf("expected claude bucket 0 to be 5h, got %q", claudeGroup.Buckets[0].Window)
	}
	if *claudeGroup.Buckets[0].RemainingPercent != 100 {
		t.Errorf("expected claude 5h remainingPercent = 100, got %v", *claudeGroup.Buckets[0].RemainingPercent)
	}
}
