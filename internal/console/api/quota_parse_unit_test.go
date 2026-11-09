package api

import "testing"

// TestNormalizeUsedPercentUnitDoesNotGuess pins the unit of a used_percent
// value. The old code treated any value <= 1 as a fraction, so an upstream
// reporting "1" (one percent used) was read as "100% used" and the surface
// showed 0% remaining. Codex reports secondary_window.used_percent=1 on a
// fresh weekly quota, which made an untouched week look exhausted.
func TestNormalizeUsedPercentUnitDoesNotGuess(t *testing.T) {
	for _, tc := range []struct {
		used    float64
		wantPct int
	}{
		{0, 100},
		{1, 99}, // was 0 before the fix
		{7, 93},
		{9, 91},
		{50, 50},
		{99, 1},
		{100, 0},
	} {
		_, pct := normalizeUsedPercent(tc.used)
		if pct == nil {
			t.Fatalf("used=%v: nil percent", tc.used)
		}
		if *pct != tc.wantPct {
			t.Errorf("normalizeUsedPercent(%v) remaining = %d%%, want %d%%", tc.used, *pct, tc.wantPct)
		}
	}
}

// TestNormalizeUsedFractionStaysFractional is the mirror case: a field named
// *_fraction really does carry 0..1, where 1 means fully used.
func TestNormalizeUsedFractionStaysFractional(t *testing.T) {
	for _, tc := range []struct {
		used    float64
		wantPct int
	}{
		{0, 100},
		{0.01, 99},
		{0.5, 50},
		{1, 0},
	} {
		_, pct := normalizeUsedFraction(tc.used)
		if pct == nil {
			t.Fatalf("used=%v: nil percent", tc.used)
		}
		if *pct != tc.wantPct {
			t.Errorf("normalizeUsedFraction(%v) remaining = %d%%, want %d%%", tc.used, *pct, tc.wantPct)
		}
	}
}

// TestParseCodexQuotaGroupsUsesRealUpstreamShape feeds the exact payload the
// codex backend returns, so the reported remaining percentages cannot drift
// from what the upstream said.
func TestParseCodexQuotaGroupsUsesRealUpstreamShape(t *testing.T) {
	raw := []byte(`{
      "plan_type": "plus",
      "rate_limit": {
        "allowed": true,
        "limit_reached": false,
        "primary_window":   {"used_percent": 9, "limit_window_seconds": 18000, "reset_after_seconds": 12805, "reset_at": 1794214231},
        "secondary_window": {"used_percent": 1, "limit_window_seconds": 604800, "reset_after_seconds": 551917, "reset_at": 1794753343}
      }
    }`)
	groups, planType, _ := parseCodexQuotaGroups(raw)
	if planType != "plus" {
		t.Errorf("planType = %q, want plus", planType)
	}
	if len(groups) != 1 || len(groups[0].Buckets) != 2 {
		t.Fatalf("got %d groups / %d buckets, want 1 / 2", len(groups), len(groups[0].Buckets))
	}
	want := map[string]int{"codex-5h": 91, "codex-weekly": 99}
	for _, b := range groups[0].Buckets {
		if b.RemainingPercent == nil {
			t.Fatalf("%s: nil remaining", b.BucketID)
		}
		if *b.RemainingPercent != want[b.BucketID] {
			t.Errorf("%s remaining = %d%%, want %d%%", b.BucketID, *b.RemainingPercent, want[b.BucketID])
		}
	}
}
