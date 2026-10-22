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
