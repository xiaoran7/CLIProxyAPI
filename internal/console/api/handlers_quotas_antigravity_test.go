// Antigravity 与 Codex 额度解析及冷却对齐测试 —— 拆自 handlers_quotas_test.go
package api

import (
	"testing"
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

	t.Run("ArgonModelCooldown_SynthesizesArgonGroup_WhenMissingFromUpstream", func(t *testing.T) {
		groups := makeGroups() // Contains only Gemini and Claude groups
		cooldowns := []CpaCooldownRecord{
			{
				Scope:            "model",
				ModelKey:         "gemini-4-argon-high",
				RemainingSeconds: 595000,
				RetryAt:          "2026-11-09T12:00:00Z",
			},
		}

		reconciled := reconcileAntigravityQuotaGroups(groups, cooldowns)
		if len(reconciled) != 3 {
			t.Fatalf("expected 3 groups (Argon synthesized), got %d", len(reconciled))
		}

		// Gemini and Claude must remain 100%
		for _, b := range reconciled[0].Buckets {
			if b.RemainingPercent == nil || *b.RemainingPercent != 100 {
				t.Errorf("gemini bucket %v remainingPercent = %v, want 100", b.BucketID, b.RemainingPercent)
			}
		}
		for _, b := range reconciled[1].Buckets {
			if b.RemainingPercent == nil || *b.RemainingPercent != 100 {
				t.Errorf("claude bucket %v remainingPercent = %v, want 100", b.BucketID, b.RemainingPercent)
			}
		}

		// Synthesized Argon group must be 0% with reset time
		argonGrp := reconciled[2]
		if argonGrp.DisplayName != "Gemini 4 Argon" {
			t.Errorf("expected group DisplayName 'Gemini 4 Argon', got %q", argonGrp.DisplayName)
		}
		if len(argonGrp.Buckets) != 1 {
			t.Fatalf("expected 1 bucket in synthesized Argon group, got %d", len(argonGrp.Buckets))
		}
		bArgon := argonGrp.Buckets[0]
		if bArgon.RemainingPercent == nil || *bArgon.RemainingPercent != 0 {
			t.Errorf("argon bucket remainingPercent = %v, want 0", bArgon.RemainingPercent)
		}
		if bArgon.ResetTime != "2026-11-09T12:00:00Z" {
			t.Errorf("argon bucket resetTime = %v, want 2026-11-09T12:00:00Z", bArgon.ResetTime)
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

	// used_percent carries 0-100, the unit its name states. The codex backend
	// really reports whole percentages here (observed: 9, 12, 1, 2).
	rawJSON := []byte(`{
		"plan_type": "plus",
		"rate_limit": {
			"primary_window": {
				"used_percent": 35,
				"reset_after_seconds": 3600
			},
			"secondary_window": {
				"used_percent": 80,
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

	// Primary window: 100 - 35 = 65% remaining
	b5h := groups[0].Buckets[0]
	if b5h.RemainingPercent == nil || *b5h.RemainingPercent != 65 {
		t.Errorf("5h remainingPercent = %v, want 65", b5h.RemainingPercent)
	}

	// Secondary window: 100 - 80 = 20% remaining
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
