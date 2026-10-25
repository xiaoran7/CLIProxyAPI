package api

import "testing"

// TestSelectCatalogAuthFilesKeepsEveryProvider guards the fleet-order starvation
// bug: with more credentials than the catalog budget and one provider owning the
// first N of them, providers listed later must still reach the catalog.
func TestSelectCatalogAuthFilesKeepsEveryProvider(t *testing.T) {
	t.Parallel()

	files := make([]catalogAuthFile, 0, 28)
	for i := 0; i < 25; i++ {
		files = append(files, catalogAuthFile{
			Name:     "antigravity-" + string(rune('a'+i)) + ".json",
			Provider: "antigravity",
		})
	}
	files = append(files,
		catalogAuthFile{Name: "workbuddy-1.json", Provider: "workbuddy"},
		catalogAuthFile{Name: "qoder-1.json", Provider: "qoder"},
		catalogAuthFile{Name: "zcode-cn-1.json", Provider: "zcode-cn"},
	)

	selected := selectCatalogAuthFiles(files, maxCatalogAuthFiles)
	if len(selected) != maxCatalogAuthFiles {
		t.Fatalf("expected %d selected files, got %d", maxCatalogAuthFiles, len(selected))
	}

	providers := make(map[string]bool, len(selected))
	for _, f := range selected {
		providers[f.Provider] = true
	}
	for _, want := range []string{"antigravity", "workbuddy", "qoder", "zcode-cn"} {
		if !providers[want] {
			t.Errorf("provider %q was starved out of the catalog selection", want)
		}
	}

	seen := make(map[string]bool, len(selected))
	for _, f := range selected {
		if seen[f.Name] {
			t.Errorf("file %q selected twice", f.Name)
		}
		seen[f.Name] = true
	}
}

func TestSelectCatalogAuthFilesUnderBudgetReturnsAll(t *testing.T) {
	t.Parallel()

	files := []catalogAuthFile{
		{Name: "a.json", Provider: "antigravity"},
		{Name: "b.json", Provider: "zcode"},
	}
	selected := selectCatalogAuthFiles(files, maxCatalogAuthFiles)
	if len(selected) != len(files) {
		t.Fatalf("expected all %d files under the budget, got %d", len(files), len(selected))
	}
}

func TestSelectCatalogAuthFilesPrefersEnabledCredential(t *testing.T) {
	t.Parallel()

	files := []catalogAuthFile{
		{Name: "zcode-cn-disabled.json", Provider: "zcode-cn", Disabled: true},
		{Name: "zcode-cn-enabled.json", Provider: "zcode-cn"},
	}
	for i := 0; i < 20; i++ {
		files = append(files, catalogAuthFile{
			Name:     "antigravity-" + string(rune('a'+i)) + ".json",
			Provider: "antigravity",
		})
	}

	selected := selectCatalogAuthFiles(files, maxCatalogAuthFiles)
	var enabledZCode bool
	for _, f := range selected {
		if f.Name == "zcode-cn-enabled.json" {
			enabledZCode = true
		}
	}
	if !enabledZCode {
		t.Error("expected the enabled zcode-cn credential to represent its provider")
	}
}

// TestCollectQuotaPoolsPreservesModelWindows guards the stats-card regression
// where ZCode's per-model buckets were dropped because they are neither 5-hour
// nor weekly windows.
func TestCollectQuotaPoolsPreservesModelWindows(t *testing.T) {
	t.Parallel()

	remaining := 100
	groups := []CpaQuotaGroup{{
		DisplayName: "ZCode 1024 Dev Day",
		Name:        "ZCode 1024 Dev Day",
		Buckets: []CpaQuotaBucket{
			{Window: "GLM-5.3-Flash", Name: "GLM-5.3-Flash · 0 / 300000000 (glm-5.3-flash)", RemainingPercent: &remaining, ResetTime: "2026-10-26T01:00:00Z"},
			{Window: "GLM-5.3", Name: "GLM-5.3 · 0 / 200000000 (glm-5.3)", RemainingPercent: &remaining},
			{Window: "no-level"}, // no remaining percent: must be skipped
		},
	}}

	pools := collectQuotaPools(groups)
	if len(pools) != 2 {
		t.Fatalf("expected 2 pools with a level, got %d", len(pools))
	}
	if pools[0].Name != "GLM-5.3-Flash · 0 / 300000000 (glm-5.3-flash)" {
		t.Errorf("unexpected first pool name %q", pools[0].Name)
	}
	if pools[0].GroupName != "ZCode 1024 Dev Day" {
		t.Errorf("expected group name carried through, got %q", pools[0].GroupName)
	}
	if pools[0].ResetTime != "2026-10-26T01:00:00Z" {
		t.Errorf("expected reset time carried through, got %q", pools[0].ResetTime)
	}
	if pools[1].RemainingPercent != 100 {
		t.Errorf("expected remaining percent 100, got %v", pools[1].RemainingPercent)
	}
}

func TestCollectQuotaPoolsFallsBackToWindowThenGroup(t *testing.T) {
	t.Parallel()

	remaining := 40
	groups := []CpaQuotaGroup{{
		DisplayName: "Qoder Mainland (cn)",
		Buckets: []CpaQuotaBucket{
			{Window: "points", RemainingPercent: &remaining},
			{RemainingPercent: &remaining},
		},
	}}

	pools := collectQuotaPools(groups)
	if len(pools) != 2 {
		t.Fatalf("expected 2 pools, got %d", len(pools))
	}
	if pools[0].Name != "points" {
		t.Errorf("expected window fallback %q, got %q", "points", pools[0].Name)
	}
	if pools[1].Name != "Qoder Mainland (cn)" {
		t.Errorf("expected group fallback %q, got %q", "Qoder Mainland (cn)", pools[1].Name)
	}
}
