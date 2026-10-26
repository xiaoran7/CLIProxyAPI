package api

import (
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
)

func TestReconcileAntigravityQuotaGroups(t *testing.T) {
	t.Parallel()

	oneFrac := 1.0
	hundredPct := 100

	makeGroups := func() []CpaQuotaGroup {
		return []CpaQuotaGroup{
			{
				DisplayName: "GEMINI Models",
				Name:        "models/gemini",
				Buckets: []CpaQuotaBucket{
					{
						BucketID:          "gemini-5h",
						Name:              "5-Hour Limit",
						Window:            "5h",
						RemainingFraction: &oneFrac,
						RemainingPercent:  &hundredPct,
						ResetTime:         "2026-10-22T17:00:00Z",
					},
					{
						BucketID:          "gemini-week",
						Name:              "Weekly Limit",
						Window:            "weekly",
						RemainingFraction: &oneFrac,
						RemainingPercent:  &hundredPct,
						ResetTime:         "2026-10-29T12:00:00Z",
					},
				},
			},
			{
				DisplayName: "CLAUDE and GPT Models",
				Name:        "models/claude",
				Buckets: []CpaQuotaBucket{
					{
						BucketID:          "claude-5h",
						Name:              "5-Hour Limit",
						Window:            "5h",
						RemainingFraction: &oneFrac,
						RemainingPercent:  &hundredPct,
						ResetTime:         "2026-10-22T17:00:00Z",
					},
					{
						BucketID:          "claude-week",
						Name:              "Weekly Limit",
						Window:            "weekly",
						RemainingFraction: &oneFrac,
						RemainingPercent:  &hundredPct,
						ResetTime:         "2026-10-29T12:00:00Z",
					},
				},
			},
		}
	}

	t.Run("ClaudeModelCooldown45h_OverridesWeeklyAnd5hToZero", func(t *testing.T) {
		groups := makeGroups()
		cooldowns := []CpaCooldownRecord{
			{
				Scope:            "model",
				ModelKey:         "claude-3-5-sonnet",
				RemainingSeconds: 162000,
				RetryAt:          "2026-10-24T02:00:00Z",
			},
		}

		reconciled := reconcileAntigravityQuotaGroups(groups, cooldowns)
		if len(reconciled) != 2 {
			t.Fatalf("expected 2 groups, got %d", len(reconciled))
		}

		// Gemini group must remain 100%
		for _, b := range reconciled[0].Buckets {
			if b.RemainingFraction == nil || *b.RemainingFraction != 1.0 {
				t.Errorf("gemini bucket %v remainingFraction = %v, want 1.0", b.BucketID, b.RemainingFraction)
			}
			if b.RemainingPercent == nil || *b.RemainingPercent != 100 {
				t.Errorf("gemini bucket %v remainingPercent = %v, want 100", b.BucketID, b.RemainingPercent)
			}
		}

		// Claude group: both 5h and weekly must be 0% with 45h reset time
		for _, b := range reconciled[1].Buckets {
			if b.RemainingFraction == nil || *b.RemainingFraction != 0.0 {
				t.Errorf("claude bucket %v remainingFraction = %v, want 0.0", b.BucketID, b.RemainingFraction)
			}
			if b.RemainingPercent == nil || *b.RemainingPercent != 0 {
				t.Errorf("claude bucket %v remainingPercent = %v, want 0", b.BucketID, b.RemainingPercent)
			}
			if b.ResetTime != "2026-10-24T02:00:00Z" {
				t.Errorf("claude bucket %v resetTime = %v, want 2026-10-24T02:00:00Z", b.BucketID, b.ResetTime)
			}
		}
	})

	t.Run("ArgonModelCooldown_OverridesOnlyArgon_LeavesGeminiUntouched", func(t *testing.T) {
		groups := append(makeGroups(), CpaQuotaGroup{
			DisplayName: "Gemini 4 Argon",
			Name:        "models/gemini4_argon",
			Buckets: []CpaQuotaBucket{
				{
					BucketID:          "gemini-argon-wtu-weekly",
					Name:              "Gemini 4 Argon Weekly Limit",
					Window:            "weekly",
					RemainingFraction: &oneFrac,
					RemainingPercent:  &hundredPct,
					ResetTime:         "2026-10-29T12:00:00Z",
				},
			},
		})
		cooldowns := []CpaCooldownRecord{
			{
				Scope:            "model",
				ModelKey:         "gemini-4-argon-high",
				RemainingSeconds: 397000,
				RetryAt:          "2026-10-31T04:54:50Z",
			},
		}

		reconciled := reconcileAntigravityQuotaGroups(groups, cooldowns)
		if len(reconciled) != 3 {
			t.Fatalf("expected 3 groups, got %d", len(reconciled))
		}

		// Gemini group must remain 100%
		for _, b := range reconciled[0].Buckets {
			if b.RemainingFraction == nil || *b.RemainingFraction != 1.0 {
				t.Errorf("gemini bucket %v remainingFraction = %v, want 1.0", b.BucketID, b.RemainingFraction)
			}
			if b.RemainingPercent == nil || *b.RemainingPercent != 100 {
				t.Errorf("gemini bucket %v remainingPercent = %v, want 100", b.BucketID, b.RemainingPercent)
			}
		}

		// Claude group must remain 100%
		for _, b := range reconciled[1].Buckets {
			if b.RemainingFraction == nil || *b.RemainingFraction != 1.0 {
				t.Errorf("claude bucket %v remainingFraction = %v, want 1.0", b.BucketID, b.RemainingFraction)
			}
		}

		// Argon group must be reconciled to 0% with 2026-10-31T04:54:50Z reset time
		for _, b := range reconciled[2].Buckets {
			if b.RemainingFraction == nil || *b.RemainingFraction != 0.0 {
				t.Errorf("argon bucket %v remainingFraction = %v, want 0.0", b.BucketID, b.RemainingFraction)
			}
			if b.RemainingPercent == nil || *b.RemainingPercent != 0 {
				t.Errorf("argon bucket %v remainingPercent = %v, want 0", b.BucketID, b.RemainingPercent)
			}
			if b.ResetTime != "2026-10-31T04:54:50Z" {
				t.Errorf("argon bucket %v resetTime = %v, want 2026-10-31T04:54:50Z", b.BucketID, b.ResetTime)
			}
		}
	})

	t.Run("CredentialLevelCooldown_OverridesAllGroups", func(t *testing.T) {
		groups := makeGroups()
		cooldowns := []CpaCooldownRecord{
			{
				Scope:            "credential",
				RemainingSeconds: 7200,
				RetryAt:          "2026-10-22T14:00:00Z",
			},
		}

		reconciled := reconcileAntigravityQuotaGroups(groups, cooldowns)
		for _, g := range reconciled {
			for _, b := range g.Buckets {
				if b.RemainingFraction == nil || *b.RemainingFraction != 0.0 {
					t.Errorf("bucket %v remainingFraction = %v, want 0.0", b.BucketID, b.RemainingFraction)
				}
				if b.RemainingPercent == nil || *b.RemainingPercent != 0 {
					t.Errorf("bucket %v remainingPercent = %v, want 0", b.BucketID, b.RemainingPercent)
				}
				if b.ResetTime != "2026-10-22T14:00:00Z" {
					t.Errorf("bucket %v resetTime = %v, want 2026-10-22T14:00:00Z", b.BucketID, b.ResetTime)
				}
			}
		}
	})

	t.Run("NoCooldown_Unchanged", func(t *testing.T) {
		groups := makeGroups()
		reconciled := reconcileAntigravityQuotaGroups(groups, nil)
		if len(reconciled) != len(groups) {
			t.Fatalf("length changed")
		}
		for i, g := range reconciled {
			for j, b := range g.Buckets {
				orig := groups[i].Buckets[j]
				if *b.RemainingFraction != *orig.RemainingFraction {
					t.Errorf("unexpected change in bucket %v", b.BucketID)
				}
			}
		}
	})
}

func TestParseAntigravityQuotaGroups(t *testing.T) {
	t.Parallel()

	rawJSON := []byte(`{
		"groups": [
			{
				"name": "models/gemini",
				"displayName": "GEMINI Models",
				"buckets": [
					{
						"bucketId": "gemini-5h",
						"displayName": "5-Hour Limit",
						"window": "5h",
						"remainingFraction": 0.5,
						"disabled": false,
						"resetTime": "2026-10-22T18:00:00Z"
					},
					{
						"bucketId": "gemini-week",
						"displayName": "Weekly Limit",
						"window": "weekly",
						"remainingFraction": 1.0,
						"disabled": false,
						"resetTime": "2026-10-29T12:00:00Z"
					}
				]
			},
			{
				"name": "models/claude",
				"displayName": "CLAUDE Models",
				"buckets": [
					{
						"bucketId": "claude-5h",
						"displayName": "5-Hour Limit",
						"window": "5h",
						"remainingFraction": 1.0,
						"disabled": true,
						"resetTime": "2026-10-22T18:00:00Z"
					}
				]
			}
		]
	}`)

	groups := parseAntigravityQuotaGroups(rawJSON)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// Verify Gemini 5h is 50%
	if len(groups[0].Buckets) != 2 {
		t.Fatalf("expected 2 buckets in gemini, got %d", len(groups[0].Buckets))
	}
	b0 := groups[0].Buckets[0]
	if b0.RemainingPercent == nil || *b0.RemainingPercent != 50 {
		t.Errorf("gemini 5h remainingPercent = %v, want 50", b0.RemainingPercent)
	}
	if b0.Disabled {
		t.Errorf("gemini 5h should not be disabled")
	}

	// Verify Claude 5h is disabled: true
	if len(groups[1].Buckets) != 1 {
		t.Fatalf("expected 1 bucket in claude, got %d", len(groups[1].Buckets))
	}
	bClaude := groups[1].Buckets[0]
	if !bClaude.Disabled {
		t.Errorf("claude 5h should be marked disabled: true")
	}
}

func TestParseAntigravityQuotaGroups_MissingWindowAndNameDefaultsTo5h(t *testing.T) {
	t.Parallel()

	// Real Google Cloud Code PA responses omit displayName and window for the 5h bucket
	rawJSON := []byte(`{
		"groups": [
			{
				"name": "models/gemini",
				"displayName": "GEMINI Models",
				"buckets": [
					{
						"remainingFraction": 0.75,
						"resetTime": "2026-10-22T17:00:00Z"
					},
					{
						"displayName": "Weekly Limit",
						"remainingFraction": 1.0,
						"resetTime": "2026-10-29T12:00:00Z"
					}
				]
			}
		]
	}`)

	groups := parseAntigravityQuotaGroups(rawJSON)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].Buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(groups[0].Buckets))
	}

	b5h := groups[0].Buckets[0]
	if b5h.Window != "5h" {
		t.Errorf("expected window '5h', got %q", b5h.Window)
	}
	if b5h.Name != "5-Hour Limit" {
		t.Errorf("expected name '5-Hour Limit', got %q", b5h.Name)
	}
	if b5h.RemainingPercent == nil || *b5h.RemainingPercent != 75 {
		t.Errorf("expected 75%% remaining, got %v", b5h.RemainingPercent)
	}
	if b5h.BucketID != "models/gemini-5h" {
		t.Errorf("expected bucketId 'models/gemini-5h', got %q", b5h.BucketID)
	}

	bWeekly := groups[0].Buckets[1]
	if bWeekly.Window != "weekly" {
		t.Errorf("expected window 'weekly', got %q", bWeekly.Window)
	}
}

func TestParseCodexQuotaGroups(t *testing.T) {
	t.Parallel()

	rawJSON := []byte(`{
		"plan_type": "plus",
		"rate_limit": {
			"primary_window": {
				"used_percent": 0.35,
				"reset_after_seconds": 3600
			},
			"secondary_window": {
				"used_percent": 0.8,
				"reset_after_seconds": 86400
			}
		}
	}`)

	groups, planType, planName := parseCodexQuotaGroups(rawJSON)
	if planType != "plus" || planName != "PLUS" {
		t.Errorf("expected plus/PLUS, got %s/%s", planType, planName)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].Buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(groups[0].Buckets))
	}

	// Primary window: 1.0 - 0.35 = 65% remaining
	b5h := groups[0].Buckets[0]
	if b5h.RemainingPercent == nil || *b5h.RemainingPercent != 65 {
		t.Errorf("5h remainingPercent = %v, want 65", b5h.RemainingPercent)
	}

	// Secondary window: 1.0 - 0.8 = 20% remaining
	bWeekly := groups[0].Buckets[1]
	if bWeekly.RemainingPercent == nil || *bWeekly.RemainingPercent != 20 {
		t.Errorf("weekly remainingPercent = %v, want 20", bWeekly.RemainingPercent)
	}
}

func TestParseCodexQuotaGroups_PercentageScale100(t *testing.T) {
	t.Parallel()

	// Some upstreams return 0-100 used_percent instead of 0-1 fraction
	rawJSON := []byte(`{
		"plan_type": "pro",
		"rate_limit": {
			"primary_window": {
				"used_percent": 25.0,
				"reset_after_seconds": 3600
			},
			"secondary_window": {
				"used_percent": 75.0,
				"reset_after_seconds": 86400
			}
		}
	}`)

	groups, planType, planName := parseCodexQuotaGroups(rawJSON)
	if planType != "pro" || planName != "PRO" {
		t.Errorf("expected pro/PRO, got %s/%s", planType, planName)
	}
	if len(groups) != 1 || len(groups[0].Buckets) != 2 {
		t.Fatalf("expected 1 group with 2 buckets, got %v", groups)
	}

	// 100 - 25 = 75%
	if groups[0].Buckets[0].RemainingPercent == nil || *groups[0].Buckets[0].RemainingPercent != 75 {
		t.Errorf("expected 75%% remaining, got %v", groups[0].Buckets[0].RemainingPercent)
	}
	// 100 - 75 = 25%
	if groups[0].Buckets[1].RemainingPercent == nil || *groups[0].Buckets[1].RemainingPercent != 25 {
		t.Errorf("expected 25%% remaining, got %v", groups[0].Buckets[1].RemainingPercent)
	}
}

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
