package registry

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// workbuddyTestConfig mirrors the shape of a real GET /v3/config payload.
const workbuddyTestConfig = `{
  "data": {
    "models": [
      {"id": "deepseek-v4.1-flash", "credits": "x0.00"},
      {"id": "glm-5.3", "credits": "x0.79"},
      {"id": "gpt-5.5", "credits": "x3.31"},
      {"id": "deep-model", "credits": "x3.33 credits"},
      {"id": "mystery-model", "credits": "not-a-number"},
      {"id": "no-credits-model"}
    ],
    "modelPromotions": [
      {
        "id": "hy3-free-trial",
        "kind": "discount",
        "enabled": true,
        "modelIds": ["hy3"],
        "discount": {"discountedCredits": "0x", "displayMode": "replace", "factor": 0},
        "schedule": {"timezone": "Asia/Shanghai", "validFrom": "2020-01-01T00:00:00+08:00", "validUntil": "2099-01-01T00:00:00+08:00"},
        "hover": {"textZh": "free for now"}
      },
      {
        "id": "hy3-expired",
        "kind": "discount",
        "enabled": true,
        "modelIds": ["kimi-k3"],
        "discount": {"discountedCredits": "0x", "displayMode": "replace", "factor": 0},
        "schedule": {"timezone": "Asia/Shanghai", "validFrom": "2020-01-01T00:00:00+08:00", "validUntil": "2020-02-01T00:00:00+08:00"}
      },
      {
        "id": "hy3-disabled",
        "kind": "discount",
        "enabled": false,
        "modelIds": ["glm-5.3-flash"],
        "discount": {"discountedCredits": "0x", "displayMode": "replace", "factor": 0}
      },
      {
        "id": "night-half-price",
        "kind": "discount",
        "enabled": true,
        "modelIds": ["glm-5.3"],
        "discount": {"discountedCredits": "0.50x", "displayMode": "strikethrough", "factor": 0.5},
        "schedule": {"timezone": "Asia/Shanghai", "daily": [{"start": "23:00", "end": "23:59"}]}
      },
      {
        "id": "badge-only",
        "kind": "discount",
        "enabled": true,
        "modelIds": ["gpt-5.5"],
        "schedule": {"timezone": "Asia/Shanghai", "daily": [{"start": "00:00", "end": "23:59"}]}
      }
    ]
  }
}`

func TestParseWorkBuddyCreditsRaw(t *testing.T) {
	cases := []struct {
		in    string
		want  float64
		valid bool
	}{
		{"x0.00", 0, true},
		{"x0", 0, true},
		{"0", 0, true},
		{"0.00", 0, true},
		{"x0.79", 0.79, true},
		{"0.79x", 0.79, true},
		{"x3.31", 3.31, true},
		{"x3.33 credits", 3.33, true},
		{"", 0, false},
		{"   ", 0, false},
		{"x", 0, false},
		{"abc", 0, false},
		{"-1", 0, false},
		{"NaN", 0, false},
	}
	for _, tc := range cases {
		got, ok := ParseWorkBuddyCreditsRaw(tc.in)
		if ok != tc.valid {
			t.Errorf("ParseWorkBuddyCreditsRaw(%q) valid = %v, want %v", tc.in, ok, tc.valid)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("ParseWorkBuddyCreditsRaw(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestParseWorkBuddyConfigCreditsFailsClosedOnUnparsableRate ensures an
// unreadable rate is treated as unknown rather than as free.
func TestParseWorkBuddyConfigCreditsFailsClosedOnUnparsableRate(t *testing.T) {
	entries := parseWorkBuddyConfigCredits([]byte(workbuddyTestConfig), workbuddyTestPromotionTime(t))
	if entry := entries["mystery-model"]; entry != nil {
		t.Fatalf("unparsable rate produced an entry: %+v", entry)
	}
	if entry := entries["no-credits-model"]; entry != nil {
		t.Fatalf("missing rate produced an entry: %+v", entry)
	}
}

func TestParseWorkBuddyConfigCreditsBaseRates(t *testing.T) {
	entries := parseWorkBuddyConfigCredits([]byte(workbuddyTestConfig), workbuddyTestPromotionTime(t))

	assertCredits := func(id string, wantEffective float64) {
		t.Helper()
		entry := entries[id]
		if entry == nil {
			t.Fatalf("no entry for %s", id)
		}
		if entry.Effective != wantEffective {
			t.Errorf("%s effective = %v, want %v", id, entry.Effective, wantEffective)
		}
		if entry.Source != WorkBuddyCreditsSource {
			t.Errorf("%s source = %q, want %q", id, entry.Source, WorkBuddyCreditsSource)
		}
	}

	// "x3.33 credits" carries a trailing unit word and must still parse.
	assertCredits("deep-model", 3.33)

	if entry := entries["deepseek-v4.1-flash"]; entry == nil || !entry.Free() {
		t.Errorf("deepseek-v4.1-flash should be free: %+v", entry)
	}
	if entry := entries["glm-5.3"]; entry == nil || entry.Free() {
		t.Errorf("glm-5.3 should not be free: %+v", entry)
	}
}

func TestParseWorkBuddyConfigCreditsPromotions(t *testing.T) {
	entries := parseWorkBuddyConfigCredits([]byte(workbuddyTestConfig), workbuddyTestPromotionTime(t))

	// An active promotion supplies a rate for a model absent from models[].
	hy3 := entries["hy3"]
	if hy3 == nil {
		t.Fatal("active promotion did not register hy3")
	}
	if !hy3.Free() {
		t.Errorf("hy3 should be free under promotion, got %v", hy3.Effective)
	}
	if hy3.Source != "promotion" {
		t.Errorf("hy3 source = %q, want promotion", hy3.Source)
	}
	if hy3.PromotionID != "hy3-free-trial" {
		t.Errorf("hy3 promotion id = %q", hy3.PromotionID)
	}

	// An expired promotion must not apply.
	if entry := entries["kimi-k3"]; entry != nil {
		t.Errorf("expired promotion applied: %+v", entry)
	}

	// A disabled promotion must not apply.
	if entry := entries["glm-5.3-flash"]; entry != nil {
		t.Errorf("disabled promotion applied: %+v", entry)
	}

	// A daytime badge with no discount block must not erase a real rate.
	gpt55 := entries["gpt-5.5"]
	if gpt55 == nil {
		t.Fatal("badge-only promotion removed the base rate")
	}
	if gpt55.Source != WorkBuddyCreditsSource {
		t.Errorf("gpt-5.5 source = %q, want %q", gpt55.Source, WorkBuddyCreditsSource)
	}
	if gpt55.Base != 3.31 {
		t.Errorf("gpt-5.5 base = %v, want 3.31", gpt55.Base)
	}
}

func TestApplyWorkBuddyCreditsOverlaysStaticModels(t *testing.T) {
	restore := SetWorkBuddyCreditsFetcherForTest(
		func(string) ([]byte, bool) { return []byte(workbuddyTestConfig), true },
		func() time.Time { return workbuddyTestPromotionTime(t) },
	)
	defer restore()

	models := GetWorkBuddyModels("intl")
	if len(models) == 0 {
		t.Fatal("no intl models")
	}
	before := len(models)
	ApplyWorkBuddyCredits(models, "intl")
	if len(models) != before {
		t.Fatalf("overlay changed model count: %d -> %d", before, len(models))
	}

	byID := make(map[string]*ModelInfo, len(models))
	for _, m := range models {
		byID[m.ID] = m
	}

	// A paid model picks up its live rate.
	glm := byID["glm-5.3"]
	if glm == nil || glm.Credits == nil {
		t.Fatalf("glm-5.3 has no credits: %+v", glm)
	}
	if glm.Credits.Multiplier != 0.79 {
		t.Errorf("glm-5.3 multiplier = %v, want 0.79", glm.Credits.Multiplier)
	}

	// A free model reports zero.
	ds := byID["deepseek-v4.1-flash"]
	if ds == nil || ds.Credits == nil || ds.Credits.Multiplier != 0 {
		t.Fatalf("deepseek-v4.1-flash credits = %+v", ds)
	}

	// A "-free" alias inherits the base model's rate.
	free := byID["deepseek-v4.1-flash-free"]
	if free == nil || free.Credits == nil || free.Credits.Multiplier != 0 {
		t.Fatalf("deepseek-v4.1-flash-free credits = %+v", free)
	}

	// A model not in the online catalogue falls back to default rates.
	if m := byID["gpt-6-astra"]; m != nil {
		if m.Credits == nil || m.Credits.Source != "default" || m.Credits.Multiplier != 3.33 {
			t.Errorf("gpt-6-astra expected default credits, got: %+v", m.Credits)
		}
	}

	// A completely unknown model keeps its rate unknown.
	unknownModel := &ModelInfo{ID: "unknown-future-model"}
	ApplyWorkBuddyCredits([]*ModelInfo{unknownModel}, "intl")
	if unknownModel.Credits != nil {
		t.Errorf("unknown-future-model unexpectedly got credits: %+v", unknownModel.Credits)
	}
}

// TestApplyWorkBuddyCreditsEmptyCatalogKeepsModels guards the failure path: an
// unusable credential answers 200 with an empty catalogue, which must not prune
// the static model list.
func TestApplyWorkBuddyCreditsEmptyCatalogKeepsModels(t *testing.T) {
	restore := SetWorkBuddyCreditsFetcherForTest(
		func(string) ([]byte, bool) { return []byte(`{"data":{"models":[]}}`), true },
		nil,
	)
	defer restore()

	models := GetWorkBuddyModels("cn")
	before := len(models)
	ApplyWorkBuddyCredits(models, "cn")
	if len(models) != before {
		t.Fatalf("empty catalogue changed model count: %d -> %d", before, len(models))
	}
	for _, m := range models {
		if m.Credits != nil && m.Credits.Source != "default" && m.Credits.Source != "alias" {
			t.Errorf("%s got unexpected credits from an empty catalogue: %+v", m.ID, m.Credits)
		}
	}
	if summary := GetWorkBuddyCreditsSummary("cn"); summary.HasRates {
		t.Error("empty catalogue reported rates")
	}
}

func TestApplyWorkBuddyCreditsNilGetterLeavesModelsUntouched(t *testing.T) {
	restore := SetWorkBuddyCreditsFetcherForTest(nil, nil)
	defer restore()

	models := GetWorkBuddyModels("intl")
	ApplyWorkBuddyCredits(models, "intl")
	for _, m := range models {
		if m.Credits != nil && m.Credits.Source != "default" && m.Credits.Source != "alias" {
			t.Fatalf("%s got unexpected credits with no fetcher: %+v", m.ID, m.Credits)
		}
	}
}

func TestGetWorkBuddyCreditsSummary(t *testing.T) {
	restore := SetWorkBuddyCreditsFetcherForTest(
		func(string) ([]byte, bool) { return []byte(workbuddyTestConfig), true },
		func() time.Time { return workbuddyTestPromotionTime(t) },
	)
	defer restore()

	// Prime the cache through the public entry point.
	ApplyWorkBuddyCredits(GetWorkBuddyModels("intl"), "intl")

	summary := GetWorkBuddyCreditsSummary("intl")
	if !summary.HasRates {
		t.Fatal("summary reports no rates")
	}
	// deepseek-v4.1-flash (0) and hy3 (promotion, 0) are free.
	if summary.FreeCount != 2 {
		t.Errorf("FreeCount = %d, want 2", summary.FreeCount)
	}
	// glm-5.3, gpt-5.5, deep-model.
	if summary.PaidCount != 3 {
		t.Errorf("PaidCount = %d, want 3", summary.PaidCount)
	}
	if summary.MinMultiplier != 0.79 {
		t.Errorf("MinMultiplier = %v, want 0.79", summary.MinMultiplier)
	}
	if summary.MaxMultiplier != 3.33 {
		t.Errorf("MaxMultiplier = %v, want 3.33", summary.MaxMultiplier)
	}
}

// TestGetWorkBuddyCreditsSummaryIgnoresPromotionsOnly keeps a promotions-only
// catalogue from being advertised as a complete rate table.
func TestGetWorkBuddyCreditsSummaryIgnoresPromotionsOnly(t *testing.T) {
	const promotionsOnly = `{"data":{"models":[],"modelPromotions":[{"id":"p","enabled":true,"modelIds":["hy3"],"discount":{"discountedCredits":"0x","factor":0}}]}}`
	restore := SetWorkBuddyCreditsFetcherForTest(
		func(string) ([]byte, bool) { return []byte(promotionsOnly), true },
		nil,
	)
	defer restore()

	ApplyWorkBuddyCredits(GetWorkBuddyModels("intl"), "intl")
	if summary := GetWorkBuddyCreditsSummary("intl"); summary.HasRates {
		t.Errorf("promotions-only catalogue reported rates: %+v", summary)
	}
}

func TestGetWorkBuddyModelCredits(t *testing.T) {
	restore := SetWorkBuddyCreditsFetcherForTest(
		func(string) ([]byte, bool) { return []byte(workbuddyTestConfig), true },
		func() time.Time { return workbuddyTestPromotionTime(t) },
	)
	defer restore()

	rate := GetWorkBuddyModelCredits("glm-5.3", "intl")
	if rate == nil {
		t.Fatal("no rate for glm-5.3")
	}
	if rate.Effective != "0.79x" {
		t.Errorf("Effective = %q, want 0.79x", rate.Effective)
	}
	if rate.Base != "0.79x" {
		t.Errorf("Base = %q, want 0.79x", rate.Base)
	}
	if rate.Free {
		t.Error("glm-5.3 reported free")
	}

	free := GetWorkBuddyModelCredits("deepseek-v4.1-flash", "intl")
	if free == nil || !free.Free {
		t.Fatalf("deepseek-v4.1-flash rate = %+v", free)
	}
	if free.Effective != "0x" {
		t.Errorf("free Effective = %q, want 0x", free.Effective)
	}

	if unknown := GetWorkBuddyModelCredits("mystery-model", "intl"); unknown != nil {
		t.Errorf("unparsable rate returned %+v", unknown)
	}
}

// TestRealmsKeepSeparateRateTables ensures the same model ID can be free in one
// realm and paid in the other.
func TestRealmsKeepSeparateRateTables(t *testing.T) {
	const intlBody = `{"data":{"models":[{"id":"deepseek-v4.1-flash","credits":"x0.00"}]}}`
	const cnBody = `{"data":{"models":[{"id":"deepseek-v4.1-flash","credits":"x0.11"}]}}`

	restore := SetWorkBuddyCreditsFetcherForTest(func(url string) ([]byte, bool) {
		if contains(url, "tencent.com") {
			return []byte(cnBody), true
		}
		return []byte(intlBody), true
	}, nil)
	defer restore()

	intlRate := GetWorkBuddyModelCredits("deepseek-v4.1-flash", "intl")
	if intlRate == nil || !intlRate.Free {
		t.Fatalf("intl deepseek-v4.1-flash = %+v, want free", intlRate)
	}
	cnRate := GetWorkBuddyModelCredits("deepseek-v4.1-flash", "cn")
	if cnRate == nil {
		t.Fatal("no cn rate")
	}
	if cnRate.Free || cnRate.Multiplier != 0.11 {
		t.Errorf("cn deepseek-v4.1-flash = %+v, want 0.11 paid", cnRate)
	}
}

// TestFailedFetchKeepsPreviousRates ensures a transient failure does not wipe a
// good catalogue.
func TestFailedFetchKeepsPreviousRates(t *testing.T) {
	calls := 0
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	restore := SetWorkBuddyCreditsFetcherForTest(func(string) ([]byte, bool) {
		calls++
		if calls == 1 {
			return []byte(workbuddyTestConfig), true
		}
		return nil, false
	}, func() time.Time { return now })
	defer restore()

	ApplyWorkBuddyCredits(GetWorkBuddyModels("intl"), "intl")
	first := GetWorkBuddyModelCredits("glm-5.3", "intl")
	if first == nil {
		t.Fatal("initial fetch produced no rate")
	}

	// Move past the TTL and force a refresh that fails.
	now = now.Add(workbuddyCreditsTTL + time.Minute)
	WarmWorkBuddyCredits("intl")
	second := GetWorkBuddyModelCredits("glm-5.3", "intl")
	if second == nil {
		t.Fatal("failed refresh discarded the known rate")
	}
	if second.Multiplier != first.Multiplier {
		t.Errorf("rate changed across a failed refresh: %v -> %v", first.Multiplier, second.Multiplier)
	}
}

func TestWarmWorkBuddyCreditsHonoursTTL(t *testing.T) {
	calls := 0
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	restore := SetWorkBuddyCreditsFetcherForTest(func(string) ([]byte, bool) {
		calls++
		return []byte(workbuddyTestConfig), true
	}, func() time.Time { return now })
	defer restore()

	WarmWorkBuddyCredits("intl")
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	WarmWorkBuddyCredits("intl")
	if calls != 1 {
		t.Fatalf("fresh cache refetched: calls = %d", calls)
	}
	now = now.Add(workbuddyCreditsTTL + time.Minute)
	WarmWorkBuddyCredits("intl")
	if calls != 2 {
		t.Fatalf("stale cache did not refetch: calls = %d", calls)
	}
}

func TestFormatWorkBuddyMultiplier(t *testing.T) {
	cases := map[float64]string{
		0:    "0x",
		0.79: "0.79x",
		3.31: "3.31x",
		1:    "1x",
		1.5:  "1.50x",
		0.05: "0.05x",
	}
	for in, want := range cases {
		if got := formatWorkBuddyMultiplier(in); got != want {
			t.Errorf("formatWorkBuddyMultiplier(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestWorkBuddyCreditsLookupKeyNormalizesAliases(t *testing.T) {
	cases := map[string]string{
		"GLM-5.3":                "glm-5.3",
		"glm-5.3-free":           "glm-5.3",
		"workbuddy/glm-5.3":      "glm-5.3",
		"workbuddy/glm-5.3-free": "glm-5.3",
		"  glm-5.3  ":            "glm-5.3",
	}
	for in, want := range cases {
		if got := workbuddyCreditsLookupKey(in); got != want {
			t.Errorf("workbuddyCreditsLookupKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestLocalWorkBuddyCreditsRate covers the offline fallback that recognises the
// local "-free" alias convention.
func TestLocalWorkBuddyCreditsRate(t *testing.T) {
	rate := LocalWorkBuddyCreditsRate("glm-5.3-flash-free")
	if rate == nil || !rate.Free {
		t.Fatalf("local alias rate = %+v, want free", rate)
	}
	if rate.Source != "local-alias" {
		t.Errorf("Source = %q, want local-alias", rate.Source)
	}
	if paid := LocalWorkBuddyCreditsRate("glm-5.3"); paid != nil {
		t.Errorf("non-alias model produced a local rate: %+v", paid)
	}
	if cnFree := LocalWorkBuddyCreditsRate("hy4-preview-f-free"); cnFree != nil {
		t.Errorf("CN model hy4-preview-f-free produced a local rate: %+v", cnFree)
	}
	if cnPrefixFree := LocalWorkBuddyCreditsRate("workbuddy-cn/deepseek-v4.1-flash-free"); cnPrefixFree != nil {
		t.Errorf("workbuddy-cn/deepseek-v4.1-flash-free produced a local rate: %+v", cnPrefixFree)
	}
}

func TestWorkBuddyCNRealmRejectsFreeModelCredits(t *testing.T) {
	// CN realm has no free tier / free models; lookups with -free under CN must return nil.
	if r := LookupWorkBuddyModelCredits("deepseek-v4.1-flash-free", "cn"); r != nil {
		t.Errorf("LookupWorkBuddyModelCredits(deepseek-v4.1-flash-free, cn) = %+v, want nil", r)
	}
	if r := LookupWorkBuddyModelCredits("hy4-preview-f-free", "cn"); r != nil {
		t.Errorf("LookupWorkBuddyModelCredits(hy4-preview-f-free, cn) = %+v, want nil", r)
	}

	// Intl realm should still resolve -free alias as free.
	rIntl := LookupWorkBuddyModelCredits("deepseek-v4.1-flash-free", "intl")
	if rIntl == nil || !rIntl.Free || rIntl.Source != "alias" {
		t.Errorf("LookupWorkBuddyModelCredits(deepseek-v4.1-flash-free, intl) = %+v, want free alias", rIntl)
	}

	// ResolveWorkBuddyModelCreditRate for explicit CN prefixes must return nil for -free.
	if r := ResolveWorkBuddyModelCreditRate("workbuddy-cn/deepseek-v4.1-flash-free"); r != nil {
		t.Errorf("ResolveWorkBuddyModelCreditRate(workbuddy-cn/deepseek-v4.1-flash-free) = %+v, want nil", r)
	}
	if r := ResolveWorkBuddyModelCreditRate("hy4-preview-f-free"); r != nil {
		t.Errorf("ResolveWorkBuddyModelCreditRate(hy4-preview-f-free) = %+v, want nil", r)
	}

	// GetWorkBuddyModelCredits for -free in CN must return nil.
	if r := GetWorkBuddyModelCredits("deepseek-v4.1-flash-free", "cn"); r != nil {
		t.Errorf("GetWorkBuddyModelCredits(deepseek-v4.1-flash-free, cn) = %+v, want nil", r)
	}
}

func TestCollectWorkBuddyModelCreditsFromNestedPayload(t *testing.T) {
	const nested = `{"config":{"agents":[{"name":"cli","models":["a"]}],"blocks":[{"models":[{"id":"nested-model","credits":"x2.50"}]}]}}`
	out := make(map[string]*workbuddyCreditsEntry)
	collectWorkBuddyModelCredits(mustDecodeJSON(t, nested), out)
	entry := out["nested-model"]
	if entry == nil {
		t.Fatal("nested model not collected")
	}
	if entry.Effective != 2.50 {
		t.Errorf("nested rate = %v, want 2.50", entry.Effective)
	}
}

func TestWorkBuddyConfigModelCredits(t *testing.T) {
	rates := WorkBuddyConfigModelCredits([]byte(workbuddyTestConfig))
	if rates["glm-5.3"] != "x0.79" {
		t.Errorf("glm-5.3 raw = %q, want x0.79", rates["glm-5.3"])
	}
	if rates["deep-model"] != "x3.33 credits" {
		t.Errorf("deep-model raw = %q", rates["deep-model"])
	}
	if _, ok := rates["mystery-model"]; ok {
		t.Error("unparsable rate surfaced in the raw map")
	}
}

func TestParseWorkBuddyCreditsInvalidJSON(t *testing.T) {
	if entries := parseWorkBuddyConfigCredits([]byte("not json"), time.Now()); entries != nil {
		t.Errorf("invalid JSON produced entries: %+v", entries)
	}
	if entries := parseWorkBuddyConfigCredits(nil, time.Now()); entries != nil {
		t.Errorf("nil body produced entries: %+v", entries)
	}
}

// TestDailyPromotionWindow covers windows that cross midnight.
func TestDailyPromotionWindow(t *testing.T) {
	inWindow := func(hour, minute int, start, end string) bool {
		local := time.Date(2026, 10, 15, hour, minute, 0, 0, workbuddyCreditsZone)
		return workbuddyInDailyWindow(local, start, end)
	}
	if !inWindow(23, 30, "23:00", "23:59") {
		t.Error("23:30 should fall inside 23:00-23:59")
	}
	if inWindow(12, 0, "23:00", "23:59") {
		t.Error("12:00 should fall outside 23:00-23:59")
	}
	// Crossing midnight: 23:00 -> 08:00.
	if !inWindow(23, 30, "23:00", "8:00") {
		t.Error("23:30 should fall inside 23:00-8:00")
	}
	if !inWindow(7, 0, "23:00", "8:00") {
		t.Error("07:00 should fall inside 23:00-8:00")
	}
	if inWindow(12, 0, "23:00", "8:00") {
		t.Error("12:00 should fall outside 23:00-8:00")
	}
}

// workbuddyTestPromotionTime is midday Beijing time on the test day, chosen so
// the fixed date promotions in the fixture are active and the 23:00-23:59 daily
// window is not.
func workbuddyTestPromotionTime(t *testing.T) time.Time {
	t.Helper()
	return time.Date(2026, 10, 15, 12, 0, 0, 0, workbuddyCreditsZone)
}

func mustDecodeJSON(t *testing.T, body string) any {
	t.Helper()
	var out any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return out
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func TestParseWorkBuddyConfigModels(t *testing.T) {
	entries := parseWorkBuddyConfigCredits([]byte(workbuddyTestConfig), workbuddyTestPromotionTime(t))
	models := parseWorkBuddyConfigModels([]byte(workbuddyTestConfig), "intl", entries)
	if len(models) == 0 {
		t.Fatal("parseWorkBuddyConfigModels returned empty list")
	}

	byID := make(map[string]*ModelInfo)
	for _, m := range models {
		byID[m.ID] = m
	}

	// gpt-5.5 should have owner "openai", display name "GPT-5.5 (WorkBuddy)", credits 3.31
	gpt55 := byID["gpt-5.5"]
	if gpt55 == nil {
		t.Fatal("gpt-5.5 not parsed from data.models")
	}
	if gpt55.OwnedBy != "openai" {
		t.Errorf("gpt-5.5 OwnedBy = %q, want openai", gpt55.OwnedBy)
	}
	if gpt55.DisplayName != "GPT-5.5 (WorkBuddy)" {
		t.Errorf("gpt-5.5 DisplayName = %q, want %q", gpt55.DisplayName, "GPT-5.5 (WorkBuddy)")
	}
	if gpt55.Credits == nil || gpt55.Credits.Multiplier != 3.31 {
		t.Errorf("gpt-5.5 Credits = %+v, want multiplier 3.31", gpt55.Credits)
	}
	if gpt55.ContextLength != 1048576 {
		t.Errorf("gpt-5.5 ContextLength = %d, want 1048576", gpt55.ContextLength)
	}

	// deepseek-v4.1-flash should have owner "deepseek", thinking support, credits 0
	ds := byID["deepseek-v4.1-flash"]
	if ds == nil {
		t.Fatal("deepseek-v4.1-flash not parsed")
	}
	if ds.OwnedBy != "deepseek" {
		t.Errorf("deepseek OwnedBy = %q, want deepseek", ds.OwnedBy)
	}
	if ds.Thinking == nil {
		t.Error("deepseek should have ThinkingSupport")
	}
	if ds.Credits == nil || ds.Credits.Multiplier != 0 {
		t.Errorf("deepseek Credits = %+v, want multiplier 0", ds.Credits)
	}

	// Model Promotions alone (like hy3-free-trial) should NOT create a model entry
	if _, ok := byID["hy3"]; ok {
		t.Error("hy3 should not be created as a dynamic model because it is only in modelPromotions")
	}
}

func TestFetchWorkBuddyCreditsStoresDynamicModelsAndNotifies(t *testing.T) {
	var notified []string
	SetModelRefreshCallback(func(changed []string) {
		notified = append(notified, changed...)
	})
	defer SetModelRefreshCallback(nil)

	restore := SetWorkBuddyCreditsFetcherForTest(
		func(string) ([]byte, bool) { return []byte(workbuddyTestConfig), true },
		func() time.Time { return workbuddyTestPromotionTime(t) },
	)
	defer restore()

	// Initial warm should discover dynamic models and notify refresh
	WarmWorkBuddyCredits("intl")

	dynamic := GetWorkBuddyDynamicModels("intl")
	if len(dynamic) == 0 {
		t.Fatal("WarmWorkBuddyCredits did not populate dynamic models")
	}

	var foundGPT55 bool
	for _, m := range dynamic {
		if m.ID == "gpt-5.5" {
			foundGPT55 = true
			if m.Credits == nil || m.Credits.Multiplier != 3.31 {
				t.Errorf("dynamic gpt-5.5 credits = %+v, want 3.31", m.Credits)
			}
		}
	}
	if !foundGPT55 {
		t.Error("gpt-5.5 not found in dynamic models after WarmWorkBuddyCredits")
	}

	// Verify notification was sent
	foundWorkBuddyNotify := false
	for _, p := range notified {
		if p == "workbuddy" {
			foundWorkBuddyNotify = true
			break
		}
	}
	if !foundWorkBuddyNotify {
		t.Errorf("notifyModelRefresh was not triggered with workbuddy; notified: %v", notified)
	}
}

func TestDualRealmCreditResolution(t *testing.T) {
	// 1. Domestic CN realm default fallback rates: DeepSeek Flash is paid at 0.11x, no free tier.
	cnFlash := LookupWorkBuddyModelCredits("deepseek-v4.1-flash", "cn")
	if cnFlash == nil {
		t.Fatal("LookupWorkBuddyModelCredits(deepseek-v4.1-flash, cn) returned nil")
	}
	if cnFlash.Multiplier != 0.11 || cnFlash.Effective != "0.11x" || cnFlash.Free {
		t.Errorf("CN deepseek-v4.1-flash rate = %+v, want multiplier 0.11, effective 0.11x, free false", cnFlash)
	}

	// 2. International Intl realm default fallback rates: DeepSeek Flash is promotional free 0x.
	intlFlash := LookupWorkBuddyModelCredits("deepseek-v4.1-flash", "intl")
	if intlFlash == nil {
		t.Fatal("LookupWorkBuddyModelCredits(deepseek-v4.1-flash, intl) returned nil")
	}
	if intlFlash.Multiplier != 0.0 || intlFlash.Effective != "0x" || !intlFlash.Free {
		t.Errorf("Intl deepseek-v4.1-flash rate = %+v, want multiplier 0.0, effective 0x, free true", intlFlash)
	}

	// 3. Prefix detection respects realm namespaces
	if realm, prov := DetectWorkBuddyModelRealm("workbuddy-cn/deepseek-v4.1-flash"); realm != "cn" || prov != "workbuddy-cn" {
		t.Errorf("DetectWorkBuddyModelRealm(workbuddy-cn/...) = (%q, %q), want (cn, workbuddy-cn)", realm, prov)
	}
	if realm, prov := DetectWorkBuddyModelRealm("workbuddy/deepseek-v4.1-flash"); realm != "intl" || prov != "workbuddy" {
		t.Errorf("DetectWorkBuddyModelRealm(workbuddy/...) = (%q, %q), want (intl, workbuddy)", realm, prov)
	}
	if realm, prov := DetectWorkBuddyModelRealm("hy4-preview-f"); realm != "cn" || prov != "workbuddy-cn" {
		t.Errorf("DetectWorkBuddyModelRealm(hy4-preview-f) = (%q, %q), want (cn, workbuddy-cn)", realm, prov)
	}
}
