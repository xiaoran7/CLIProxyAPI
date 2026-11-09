package api

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
)

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

// TestHydrateKeepsArgonSeparateFromGemini reproduces the snapshot rows observed
// in production for one antigravity credential: the plain Gemini pool has quota
// left while the Gemini 4 Argon pool is exhausted. Canonicalizing "Gemini 4
// Argon" into models/gemini collapsed both onto one dedup key, so whichever row
// the query returned first won: an exhausted Argon row made the surface report
// 0% for a Gemini pool that still had quota. Both orderings are covered because
// the collapse only misreported when the Argon row came first.
func TestHydrateKeepsArgonSeparateFromGemini(t *testing.T) {
	str := func(s string) *string { return &s }
	row := func(window string, pct float64) db.QuotaSnapshotRow {
		return db.QuotaSnapshotRow{AuthIndex: "idx", Provider: "antigravity", WindowName: str(window), RemainingPercent: pct}
	}
	geminiRows := []db.QuotaSnapshotRow{
		row("Gemini Models:5h", 100),
		row("Gemini Models:weekly", 88),
	}
	argonRows := []db.QuotaSnapshotRow{
		row("Gemini 4 Argon:weekly", 0),
	}
	claudeRows := []db.QuotaSnapshotRow{
		row("Claude and GPT models:weekly", 100),
		row("Claude and GPT models:5h", 100),
	}

	cases := map[string][]db.QuotaSnapshotRow{
		"gemini first": append(append(append([]db.QuotaSnapshotRow{}, geminiRows...), argonRows...), claudeRows...),
		"argon first":  append(append(append([]db.QuotaSnapshotRow{}, argonRows...), geminiRows...), claudeRows...),
	}

	for name, rows := range cases {
		t.Run(name, func(t *testing.T) {
			groups, _ := hydrateQuotaGroupsFromSnapshots(rows)
			got, ok := groups["idx"]
			if !ok || len(got) == 0 {
				t.Fatalf("hydrate returned no groups for idx")
			}

			byGroup := map[string]map[string]int{}
			for _, g := range got {
				byGroup[g.Name] = map[string]int{}
				for _, b := range g.Buckets {
					if b.RemainingPercent != nil {
						byGroup[g.Name][b.Window] = *b.RemainingPercent
					}
				}
			}
			if v, ok := byGroup["models/gemini"]["weekly"]; !ok || v != 88 {
				t.Errorf("models/gemini weekly = %d (present=%v), want 88: the exhausted Argon pool overwrote it", v, ok)
			}
			if v, ok := byGroup["models/gemini4_argon"]["weekly"]; !ok || v != 0 {
				t.Errorf("models/gemini4_argon weekly = %d (present=%v), want 0", v, ok)
			}
			if _, ok := byGroup["models/claude"]; !ok {
				t.Errorf("models/claude group missing, got groups %v", keysOf(byGroup))
			}
		})
	}
}

func keysOf(m map[string]map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestHydrateKeepsArgonSeparateFromGeminiWithoutDelimiters guards against the bug
// where raw window names like "gemini-argon-wtu-weekly" had no ":" or " - " delimiter,
// leaving grpName empty and causing the fallback strings.Contains(wLower, "gemini")
// to incorrectly fold Argon into models/gemini.
func TestHydrateKeepsArgonSeparateFromGeminiWithoutDelimiters(t *testing.T) {
	str := func(s string) *string { return &s }
	rows := []db.QuotaSnapshotRow{
		{AuthIndex: "idx", Provider: "antigravity", WindowName: str("gemini-argon-wtu-weekly"), RemainingPercent: 0},
		{AuthIndex: "idx", Provider: "antigravity", WindowName: str("Gemini Models:weekly"), RemainingPercent: 88},
	}
	groups, _ := hydrateQuotaGroupsFromSnapshots(rows)
	byGroup := map[string]map[string]int{}
	for _, g := range groups["idx"] {
		byGroup[g.Name] = map[string]int{}
		for _, b := range g.Buckets {
			if b.RemainingPercent != nil {
				byGroup[g.Name][b.Window] = *b.RemainingPercent
			}
		}
	}
	if v, ok := byGroup["models/gemini4_argon"]["weekly"]; !ok || v != 0 {
		t.Errorf("expected models/gemini4_argon weekly=0, got groups: %v", byGroup)
	}
	if v, ok := byGroup["models/gemini"]["weekly"]; !ok || v != 88 {
		t.Errorf("expected models/gemini weekly=88, got groups: %v", byGroup)
	}
}

// TestHydrateDoesNotTreatArbitraryHourAs5h verifies that non-5h windows containing
// "hour" (like "hourly" or "1 hour") do not get collapsed into "5-Hour Limit".
func TestHydrateDoesNotTreatArbitraryHourAs5h(t *testing.T) {
	str := func(s string) *string { return &s }
	rows := []db.QuotaSnapshotRow{
		{AuthIndex: "idx", Provider: "custom", WindowName: str("custom:hourly"), RemainingPercent: 50},
		{AuthIndex: "idx", Provider: "custom", WindowName: str("custom:5h"), RemainingPercent: 75},
	}
	groups, _ := hydrateQuotaGroupsFromSnapshots(rows)
	bkts := groups["idx"][0].Buckets
	if len(bkts) != 2 {
		t.Fatalf("expected 2 distinct buckets, got %d", len(bkts))
	}
	var foundHourly, found5h bool
	for _, b := range bkts {
		if b.Window == "hourly" && b.Name == "hourly" {
			foundHourly = true
		}
		if b.Window == "5h" && b.Name == "5-Hour Limit" {
			found5h = true
		}
	}
	if !foundHourly || !found5h {
		t.Errorf("expected distinct hourly and 5h buckets, got: %+v", bkts)
	}
}

// TestParseCodexQuotaGroupsDetectsMonthlyWindow verifies that secondary windows
// with limit_window_seconds >= 2419200 (28 days) are identified as monthly limits.
func TestParseCodexQuotaGroupsDetectsMonthlyWindow(t *testing.T) {
	raw := []byte(`{
      "plan_type": "team",
      "rate_limit": {
        "allowed": true,
        "limit_reached": false,
        "primary_window":   {"used_percent": 10, "limit_window_seconds": 18000, "reset_after_seconds": 12805},
        "secondary_window": {"used_percent": 5, "limit_window_seconds": 2419200, "reset_after_seconds": 2400000}
      }
    }`)
	groups, planType, _ := parseCodexQuotaGroups(raw)
	if planType != "team" {
		t.Errorf("planType = %q, want team", planType)
	}
	if len(groups) != 1 || len(groups[0].Buckets) != 2 {
		t.Fatalf("got %d groups / %d buckets, want 1 / 2", len(groups), len(groups[0].Buckets))
	}
	b5h := groups[0].Buckets[0]
	if b5h.Window != "5h" || b5h.BucketID != "codex-5h" {
		t.Errorf("primary window = %q (%s), want 5h (codex-5h)", b5h.Window, b5h.BucketID)
	}
	bMonth := groups[0].Buckets[1]
	if bMonth.Window != "monthly" || bMonth.BucketID != "codex-monthly" {
		t.Errorf("secondary window = %q (%s), want monthly (codex-monthly)", bMonth.Window, bMonth.BucketID)
	}
	if bMonth.Name != "Monthly Limit Remaining" {
		t.Errorf("secondary name = %q, want Monthly Limit Remaining", bMonth.Name)
	}
}
