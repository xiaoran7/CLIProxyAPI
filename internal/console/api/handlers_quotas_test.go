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
