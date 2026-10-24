package registry

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WorkBuddy credit multiplier constants.
//
// WorkBuddy and CodeBuddy bill by "credits". Every catalogue model carries a
// credit multiplier such as "x0.00" (the daily free tier), "x0.79" or "x3.31".
// The upstream config catalogue is authoritative for those rates, so the values
// below mirror the ones the client itself renders.
const (
	// WorkBuddyCreditsBaseURLFormat yields the catalogue URL for a realm.
	WorkBuddyCreditsBaseURLFormat = "%s/v3/config"
	// WorkBuddyCreditsSource is the provenance marker for live catalogue rates.
	WorkBuddyCreditsSource = "workbuddy"
	// workbuddyCreditsMaxBody caps how much of the catalogue response is read.
	workbuddyCreditsMaxBody = 4 << 20
	// workbuddyCreditsTTL is how long a fetched catalogue stays fresh.
	workbuddyCreditsTTL = 30 * time.Minute
	// workbuddyCreditsFailureTTL is how long a failed fetch is remembered before
	// another attempt, so a broken credential cannot cause a fetch storm.
	workbuddyCreditsFailureTTL = 2 * time.Minute
)

// workbuddyCreditsEntry holds one resolved model rate.
type workbuddyCreditsEntry struct {
	// Raw is the upstream string exactly as published, e.g. "x0.00", "x3.31".
	Raw string
	// Base is the list (pre-promotion) multiplier in credits per unit.
	Base float64
	// Effective is the multiplier after applying an active promotion.
	Effective float64
	// Source is either "workbuddy" (live) or "promotion" (fallback when the base
	// rate is unknown but a promotion still proves the model is free right now).
	Source string
	// PromotionID and PromotionNote describe the applied promotion, if any.
	PromotionID   string
	PromotionNote string
}

// Free reports whether the effective multiplier is zero, meaning requests
// consume no credits.
func (e *workbuddyCreditsEntry) Free() bool {
	if e == nil {
		return false
	}
	return e.Effective < workbuddyCreditsEpsilon
}

const workbuddyCreditsEpsilon = 1e-9

// staticWorkBuddyCNDefaultCredits provides authoritative fallback credit multipliers for
// domestic CodeBuddy (CN) models when live /v3/config catalogue data is unavailable.
// Domestic CodeBuddy has NO free tier for DeepSeek models.
var staticWorkBuddyCNDefaultCredits = map[string]float64{
	"hy3":                 0.0,
	"hy4-preview-f":       1.62,
	"deepseek-v4.1-flash": 0.11,
	"deepseek-v4-flash":   0.17,
	"deepseek-v4-pro":     0.51,
	"glm-5.3-flash":       0.06,
	"glm-5.3":             0.15,
	"kimi-k3":             0.15,
	"minimax-m3":          0.15,
}

// staticWorkBuddyIntlDefaultCredits provides authoritative fallback credit multipliers for
// international WorkBuddy models when live /v3/config catalogue data is unavailable.
var staticWorkBuddyIntlDefaultCredits = map[string]float64{
	"deepseek-v4.1-flash":    0.0,
	"deepseek-v4.1-flash-sg": 0.03,
	"glm-5.3-flash":          0.06,
	"deepseek-v4-pro":        0.15,
	"glm-5.3":                0.15,
	"kimi-k3":                0.15,
	"minimax-m3":             0.15,
	"hy4-preview-f":          1.62,
	"gpt-6-astra":            3.33,
	"gpt-5.6-sol":            2.0,
	"claude-fable-5-1":       3.0,
	"claude-opus-5-5":        3.33,
	"claude-sonnet-5-5":      1.0,
}

// staticWorkBuddyDefaultCredits provides authoritative fallback credit multipliers for
// WorkBuddy / CodeBuddy models when live /v3/config catalogue data is unavailable.
var staticWorkBuddyDefaultCredits = map[string]float64{
	"hy3":                 0.0,
	"deepseek-v4.1-flash": 0.0,
	"glm-5.3-flash":       0.06,
	"deepseek-v4-pro":     0.15,
	"glm-5.3":             0.15,
	"kimi-k3":             0.15,
	"minimax-m3":          0.15,
	"hy4-preview-f":       1.62,
	"gpt-6-astra":         3.33,
	"gpt-5.6-sol":         2.0,
	"claude-fable-5-1":    3.0,
	"claude-opus-5-5":     3.33,
	"claude-sonnet-5-5":   1.0,
}

// workbuddyCreditsZone is the fixed Beijing offset (UTC+8) that WorkBuddy
// promotions are scheduled in.
var workbuddyCreditsZone = time.FixedZone("CST", 8*60*60)

type workbuddyCreditsBucket struct {
	entries map[string]*workbuddyCreditsEntry
	fetched time.Time
	failed  bool
}

// workbuddyCreditsCatalog caches resolved credit multipliers per realm.
//
// The catalogue is purely a display/costing aid: a fetch failure must never
// remove models from the API surface, it only means rates are unknown.
type workbuddyCreditsCatalog struct {
	mu      sync.RWMutex
	buckets map[string]*workbuddyCreditsBucket
	httpGet func(url string) ([]byte, bool)
	nowFunc func() time.Time
}

var workbuddyCredits = &workbuddyCreditsCatalog{
	buckets: make(map[string]*workbuddyCreditsBucket),
	nowFunc: time.Now,
}

// WorkBuddyCreditsRate describes the credit multiplier of one WorkBuddy model.
type WorkBuddyCreditsRate struct {
	// Multiplier is the effective multiplier in credits per unit.
	Multiplier float64 `json:"multiplier"`
	// Effective is the multiplier rendered upstream-style, e.g. "0.79x".
	Effective string `json:"effective,omitempty"`
	// Base is the list-price multiplier rendered upstream-style.
	Base string `json:"base,omitempty"`
	// Free reports whether the model currently consumes no credits.
	Free bool `json:"free"`
	// Source is where the rate came from: "workbuddy" or "promotion".
	Source string `json:"source,omitempty"`
	// PromotionID and PromotionNote describe an applied discount, if any.
	PromotionID   string `json:"promotion_id,omitempty"`
	PromotionNote string `json:"promotion_note,omitempty"`
}

// WorkBuddyCreditsSummary aggregates the published rates for one realm.
type WorkBuddyCreditsSummary struct {
	// FreeCount and PaidCount split the known rates by free-ness.
	FreeCount int `json:"free_count"`
	PaidCount int `json:"paid_count"`
	// MinMultiplier and MaxMultiplier bound the paid (non-free) rates.
	MinMultiplier float64 `json:"min_multiplier"`
	MaxMultiplier float64 `json:"max_multiplier"`
	// HasRates reports whether any rate is known for this realm.
	HasRates bool `json:"has_rates"`
}

// SetWorkBuddyCreditsHTTPGetter injects the transport used to pull the upstream
// catalogue. It exists so tests can supply a deterministic stub; passing nil
// clears the override and disables the live fetch entirely.
func SetWorkBuddyCreditsHTTPGetter(fn func(url string) ([]byte, bool)) {
	if workbuddyCredits == nil {
		return
	}
	workbuddyCredits.mu.Lock()
	workbuddyCredits.httpGet = fn
	workbuddyCredits.mu.Unlock()
}

// InstallWorkBuddyCreditsHTTPGetterIfAbsent installs the production transport
// only when none is configured yet, so a pre-installed transport - such as a
// test stub - is never replaced. It reports whether the getter was installed.
func InstallWorkBuddyCreditsHTTPGetterIfAbsent(fn func(url string) ([]byte, bool)) bool {
	if workbuddyCredits == nil || fn == nil {
		return false
	}
	workbuddyCredits.mu.Lock()
	defer workbuddyCredits.mu.Unlock()
	if workbuddyCredits.httpGet != nil {
		return false
	}
	workbuddyCredits.httpGet = fn
	return true
}

// SetWorkBuddyCreditsFetcherForTest installs a stub fetcher and freezes time so
// catalogue behaviour can be exercised without network access.
func SetWorkBuddyCreditsFetcherForTest(fn func(url string) ([]byte, bool), now func() time.Time) func() {
	if workbuddyCredits == nil {
		return func() {}
	}
	workbuddyCredits.mu.Lock()
	prevGet := workbuddyCredits.httpGet
	prevNow := workbuddyCredits.nowFunc
	workbuddyCredits.httpGet = fn
	if now != nil {
		workbuddyCredits.nowFunc = now
	}
	workbuddyCredits.buckets = make(map[string]*workbuddyCreditsBucket)
	ClearWorkBuddyCatalogModels("")
	workbuddyCredits.mu.Unlock()
	return func() {
		workbuddyCredits.mu.Lock()
		workbuddyCredits.httpGet = prevGet
		workbuddyCredits.nowFunc = prevNow
		workbuddyCredits.buckets = make(map[string]*workbuddyCreditsBucket)
		ClearWorkBuddyCatalogModels("")
		workbuddyCredits.mu.Unlock()
	}
}

// ApplyWorkBuddyCredits overlays live credit multipliers onto a static model
// list.
//
// The overlay is strictly additive: rates are attached to models that exist in
// the static catalogue, and a model the upstream catalogue does not know about
// (for example a locally defined alias) keeps whatever it already carried.
// Because rates are stored in a realm-keyed cache rather than on the static
// entries, callers that never register a credential - such as configuration
// validation - continue to see the pristine registry.
func ApplyWorkBuddyCredits(models []*ModelInfo, realm string) []*ModelInfo {
	if len(models) == 0 {
		return models
	}
	catalog := resolveWorkBuddyCredits(workbuddyCreditsRealmKey(realm))
	for _, m := range models {
		if m == nil {
			continue
		}
		if strings.HasSuffix(strings.ToLower(m.ID), "-free") {
			if !strings.EqualFold(strings.TrimSpace(realm), "cn") {
				m.Credits = &ModelCredits{
					Multiplier: 0.0,
					Source:     "alias",
				}
				continue
			}
		}
		if len(catalog) > 0 {
			entry, ok := catalog[workbuddyCreditsLookupKey(m.ID)]
			if ok && entry != nil && strings.TrimSpace(entry.Source) != "" {
				m.Credits = &ModelCredits{
					Multiplier:    entry.Effective,
					Source:        entry.Source,
					PromotionID:   entry.PromotionID,
					PromotionNote: entry.PromotionNote,
				}
				continue
			}
		}
		if rate := LookupWorkBuddyModelCredits(m.ID, realm); rate != nil {
			m.Credits = &ModelCredits{
				Multiplier:    rate.Multiplier,
				Source:        rate.Source,
				PromotionID:   rate.PromotionID,
				PromotionNote: rate.PromotionNote,
			}
		}
	}
	return models
}

// WorkBuddyCreditsModelRates returns the published rate of every model in a
// realm's catalogue, keyed by model ID. It returns nil when the catalogue is
// unavailable.
func WorkBuddyCreditsModelRates(realm string) map[string]WorkBuddyCreditsRate {
	catalog := resolveWorkBuddyCredits(workbuddyCreditsRealmKey(realm))
	if len(catalog) == 0 {
		return nil
	}
	out := make(map[string]WorkBuddyCreditsRate, len(catalog))
	for id, entry := range catalog {
		if entry == nil || entry.Source == "" {
			continue
		}
		rate := WorkBuddyCreditsRate{
			Multiplier:    entry.Effective,
			Free:          entry.Free(),
			Source:        entry.Source,
			PromotionID:   entry.PromotionID,
			PromotionNote: entry.PromotionNote,
		}
		if entry.Source == WorkBuddyCreditsSource {
			rate.Effective = formatWorkBuddyMultiplier(entry.Effective)
			rate.Base = formatWorkBuddyMultiplier(entry.Base)
		}
		out[id] = rate
	}
	return out
}

// GetWorkBuddyModelCredits returns the resolved credit rate for one model in a
// realm, or nil when the rate is unknown.
func GetWorkBuddyModelCredits(modelID, realm string) *WorkBuddyCreditsRate {
	cleanID := strings.ToLower(TrimWorkBuddyPrefixes(modelID))
	if strings.EqualFold(strings.TrimSpace(realm), "cn") && strings.HasSuffix(cleanID, "-free") {
		return nil
	}
	catalog := resolveWorkBuddyCredits(workbuddyCreditsRealmKey(realm))
	if len(catalog) == 0 {
		return nil
	}
	entry, ok := catalog[workbuddyCreditsLookupKey(modelID)]
	if !ok || entry == nil || strings.TrimSpace(entry.Source) == "" {
		return nil
	}
	rate := &WorkBuddyCreditsRate{
		Multiplier:    entry.Effective,
		Free:          entry.Free(),
		Source:        entry.Source,
		PromotionID:   entry.PromotionID,
		PromotionNote: entry.PromotionNote,
	}
	if entry.Source == WorkBuddyCreditsSource {
		rate.Effective = formatWorkBuddyMultiplier(entry.Effective)
		rate.Base = formatWorkBuddyMultiplier(entry.Base)
	}
	return rate
}

// GetWorkBuddyCreditsSummary aggregates the published rates for one realm.
func GetWorkBuddyCreditsSummary(realm string) WorkBuddyCreditsSummary {
	var summary WorkBuddyCreditsSummary
	authoritative := false
	for _, entry := range resolveWorkBuddyCredits(workbuddyCreditsRealmKey(realm)) {
		if entry == nil {
			continue
		}
		if entry.Source == WorkBuddyCreditsSource {
			// Only live base rates describe the whole catalogue, so a summary
			// must not be derived from promotions alone.
			authoritative = true
		}
		if entry.Free() {
			summary.FreeCount++
			continue
		}
		summary.PaidCount++
		if summary.MinMultiplier == 0 || entry.Effective < summary.MinMultiplier {
			summary.MinMultiplier = entry.Effective
		}
		if entry.Effective > summary.MaxMultiplier {
			summary.MaxMultiplier = entry.Effective
		}
	}
	summary.HasRates = authoritative
	return summary
}

// TrimWorkBuddyPrefixes removes any known WorkBuddy or CodeBuddy provider prefixes from a model ID.
func TrimWorkBuddyPrefixes(modelID string) string {
	clean := strings.TrimSpace(modelID)
	lower := strings.ToLower(clean)
	prefixes := []string{
		"workbuddy-intl/",
		"workbuddy-cn/",
		"workbuddy/",
		"codebuddy/",
		"wb-cn/",
		"wb/",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			return clean[len(p):]
		}
	}
	return clean
}

// DetectWorkBuddyModelRealm identifies whether a model belongs to the domestic (CN) CodeBuddy realm or international (Intl) WorkBuddy realm.
func DetectWorkBuddyModelRealm(modelID string) (realm string, provider string) {
	rawLower := strings.ToLower(strings.TrimSpace(modelID))
	if strings.HasPrefix(rawLower, "workbuddy-cn/") || strings.HasPrefix(rawLower, "codebuddy/") || strings.HasPrefix(rawLower, "wb-cn/") {
		return "cn", "workbuddy-cn"
	}
	if strings.HasPrefix(rawLower, "workbuddy-intl/") || strings.HasPrefix(rawLower, "workbuddy/") || strings.HasPrefix(rawLower, "wb/") {
		return "intl", "workbuddy"
	}
	trimmed := TrimWorkBuddyPrefixes(modelID)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "hy") {
		return "cn", "workbuddy-cn"
	}
	return "intl", "workbuddy"
}

// LookupWorkBuddyModelCredits resolves the credit rate of a model that may be
// unknown to the static registry.
//
// It first checks the static catalogue, then the realm's rate table, and finally
// falls back to authoritative default rates if available.
func LookupWorkBuddyModelCredits(modelID, realm string) *WorkBuddyCreditsRate {
	cleanID := strings.ToLower(TrimWorkBuddyPrefixes(modelID))
	if strings.HasSuffix(cleanID, "-free") {
		// CN realm has no free models / free tier; do not report 0x alias for CN.
		if strings.EqualFold(strings.TrimSpace(realm), "cn") {
			return nil
		}
		return &WorkBuddyCreditsRate{
			Multiplier: 0.0,
			Free:       true,
			Source:     "alias",
			Effective:  "0x",
			Base:       "0x",
		}
	}

	key := workbuddyCreditsRealmKey(realm)
	if m := LookupWorkBuddyModel(cleanID, key); m != nil && m.Credits != nil {
		rate := &WorkBuddyCreditsRate{
			Multiplier:    m.Credits.Multiplier,
			Free:          m.Credits.Multiplier < workbuddyCreditsEpsilon,
			Source:        m.Credits.Source,
			PromotionID:   m.Credits.PromotionID,
			PromotionNote: m.Credits.PromotionNote,
		}
		if m.Credits.Source == WorkBuddyCreditsSource {
			rate.Effective = formatWorkBuddyMultiplier(m.Credits.Multiplier)
		}
		return rate
	}
	if rate := GetWorkBuddyModelCredits(cleanID, key); rate != nil {
		return rate
	}

	cleanKey := workbuddyCreditsLookupKey(cleanID)
	if strings.EqualFold(strings.TrimSpace(realm), "cn") {
		if defRate, ok := staticWorkBuddyCNDefaultCredits[cleanKey]; ok {
			rate := &WorkBuddyCreditsRate{
				Multiplier: defRate,
				Free:       defRate < workbuddyCreditsEpsilon,
				Source:     "default",
				Effective:  formatWorkBuddyMultiplier(defRate),
				Base:       formatWorkBuddyMultiplier(defRate),
			}
			return rate
		}
	} else {
		if defRate, ok := staticWorkBuddyIntlDefaultCredits[cleanKey]; ok {
			rate := &WorkBuddyCreditsRate{
				Multiplier: defRate,
				Free:       defRate < workbuddyCreditsEpsilon,
				Source:     "default",
				Effective:  formatWorkBuddyMultiplier(defRate),
				Base:       formatWorkBuddyMultiplier(defRate),
			}
			return rate
		}
	}
	if defRate, ok := staticWorkBuddyDefaultCredits[cleanKey]; ok {
		rate := &WorkBuddyCreditsRate{
			Multiplier: defRate,
			Free:       defRate < workbuddyCreditsEpsilon,
			Source:     "default",
			Effective:  formatWorkBuddyMultiplier(defRate),
			Base:       formatWorkBuddyMultiplier(defRate),
		}
		return rate
	}

	return nil
}

// ResolveWorkBuddyModelCreditRate finds the credit rate for modelID, checking CN first then Intl.
func ResolveWorkBuddyModelCreditRate(modelID string) *WorkBuddyCreditsRate {
	clean := TrimWorkBuddyPrefixes(modelID)
	realm, _ := DetectWorkBuddyModelRealm(modelID)
	if realm == "cn" {
		if rate := LookupWorkBuddyModelCredits(clean, "cn"); rate != nil {
			return rate
		}
		// Do not fall back to intl for -free aliases when model belongs to the cn realm.
		if strings.HasSuffix(strings.ToLower(clean), "-free") {
			return nil
		}
		if rate := LookupWorkBuddyModelCredits(clean, "intl"); rate != nil {
			return rate
		}
	} else {
		if rate := LookupWorkBuddyModelCredits(clean, "intl"); rate != nil {
			return rate
		}
		if rate := LookupWorkBuddyModelCredits(clean, "cn"); rate != nil {
			return rate
		}
	}
	return nil
}

// warmWorkBuddyCredits fetches the realm catalogue if the cache is cold or
// stale. When block is false a stale entry is left in place while a background
// refresh runs, so callers on a hot path never wait for the network.
func warmWorkBuddyCredits(realm string, block bool) {
	if workbuddyCredits == nil {
		return
	}
	key := workbuddyCreditsRealmKey(realm)

	workbuddyCredits.mu.RLock()
	getter := workbuddyCredits.httpGet
	bucket := workbuddyCredits.buckets[key]
	nowFn := workbuddyCredits.nowFunc
	workbuddyCredits.mu.RUnlock()

	if getter == nil {
		return
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	if bucket != nil {
		ttl := workbuddyCreditsTTL
		if bucket.failed {
			ttl = workbuddyCreditsFailureTTL
		}
		if nowFn().Sub(bucket.fetched) < ttl {
			return
		}
		if !block {
			go fetchWorkBuddyCredits(key, getter)
			return
		}
	}
	fetchWorkBuddyCredits(key, getter)
}

// WarmWorkBuddyCredits triggers a blocking refresh of a realm's rate catalogue.
func WarmWorkBuddyCredits(realm string) {
	warmWorkBuddyCredits(realm, true)
}

// InvalidateWorkBuddyCredits drops cached rates, optionally for a single realm.
func InvalidateWorkBuddyCredits(realm string) {
	if workbuddyCredits == nil {
		return
	}
	workbuddyCredits.mu.Lock()
	if strings.TrimSpace(realm) == "" {
		workbuddyCredits.buckets = make(map[string]*workbuddyCreditsBucket)
	} else {
		delete(workbuddyCredits.buckets, workbuddyCreditsRealmKey(realm))
	}
	workbuddyCredits.mu.Unlock()
	ClearWorkBuddyCatalogModels(realm)
}

func fetchWorkBuddyCredits(realmKey string, getter func(url string) ([]byte, bool)) {
	var url string
	switch realmKey {
	case "cn":
		url = strings.Replace(WorkBuddyCreditsBaseURLFormat, "%s", "https://copilot.tencent.com", 1)
	default:
		url = strings.Replace(WorkBuddyCreditsBaseURLFormat, "%s", "https://www.workbuddy.ai", 1)
	}

	body, ok := getter(url)
	nowFn := workbuddyCredits.nowFunc
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn()

	entries := map[string]*workbuddyCreditsEntry(nil)
	var dynamicModels []*ModelInfo
	if ok && len(body) > 0 {
		entries = parseWorkBuddyConfigCredits(body, now)
		dynamicModels = parseWorkBuddyConfigModels(body, realmKey, entries)
	}

	modelsChanged := false
	if len(dynamicModels) > 0 {
		modelsChanged = SetWorkBuddyCatalogModels(realmKey, dynamicModels)
	}

	creditsChanged := false
	workbuddyCredits.mu.Lock()
	if len(entries) == 0 {
		// An unusable credential answers 200 with an empty catalogue, so an
		// empty result is a failure: keep serving whatever was cached before
		// and retry sooner.
		if existing := workbuddyCredits.buckets[realmKey]; existing != nil {
			existing.fetched = now
			existing.failed = true
			workbuddyCredits.mu.Unlock()
			return
		}
		workbuddyCredits.buckets[realmKey] = &workbuddyCreditsBucket{
			entries: nil,
			fetched: now,
			failed:  true,
		}
		workbuddyCredits.mu.Unlock()
		return
	}

	prevBucket := workbuddyCredits.buckets[realmKey]
	if prevBucket != nil && prevBucket.entries != nil {
		creditsChanged = workbuddyCreditsEntriesChanged(prevBucket.entries, entries)
	} else {
		creditsChanged = true
	}

	workbuddyCredits.buckets[realmKey] = &workbuddyCreditsBucket{
		entries: entries,
		fetched: now,
		failed:  false,
	}
	workbuddyCredits.mu.Unlock()

	if modelsChanged || creditsChanged {
		notifyModelRefresh([]string{"workbuddy", "workbuddy-intl", "workbuddy-cn", "copilot-tencent"})
	}
}

func cachedWorkBuddyCredits(realmKey string) map[string]*workbuddyCreditsEntry {
	if workbuddyCredits == nil {
		return nil
	}
	workbuddyCredits.mu.RLock()
	defer workbuddyCredits.mu.RUnlock()
	bucket := workbuddyCredits.buckets[realmKey]
	if bucket == nil {
		return nil
	}
	return bucket.entries
}

func resolveWorkBuddyCredits(realmKey string) map[string]*workbuddyCreditsEntry {
	warmWorkBuddyCredits(realmKey, true)
	return cachedWorkBuddyCredits(realmKey)
}

func workbuddyCreditsRealmKey(realm string) string {
	if strings.EqualFold(strings.TrimSpace(realm), "cn") {
		return "cn"
	}
	return "intl"
}

// workbuddyCreditsLookupKey normalises a model ID for rate lookup. The "-free"
// variants are local aliases of a base model, so they inherit the base rate.
func workbuddyCreditsLookupKey(modelID string) string {
	key := strings.ToLower(strings.TrimSpace(TrimWorkBuddyPrefixes(modelID)))
	return strings.TrimSuffix(key, "-free")
}

// WorkBuddyConfigModelCredits extracts model ID -> credits rate from a raw
// /v3/config payload.
func WorkBuddyConfigModelCredits(body []byte) map[string]string {
	parsed := parseWorkBuddyConfigCredits(body, time.Now())
	if len(parsed) == 0 {
		return nil
	}
	out := make(map[string]string, len(parsed))
	for id, entry := range parsed {
		if entry == nil || entry.Source != WorkBuddyCreditsSource || strings.TrimSpace(entry.Raw) == "" {
			continue
		}
		out[id] = entry.Raw
	}
	return out
}

// ParseWorkBuddyCreditsRaw converts an upstream credits string such as "x0.79",
// "0.79x" or "3.31 credits" into a number. It reports false when the value is
// absent or not a number, so an unparsable rate is never treated as free.
func ParseWorkBuddyCreditsRaw(raw string) (float64, bool) {
	cleaned := strings.ToLower(strings.TrimSpace(raw))
	if cleaned == "" {
		return 0, false
	}
	cleaned = strings.TrimSuffix(cleaned, "credits")
	cleaned = strings.TrimSuffix(cleaned, "credit")
	cleaned = strings.TrimSuffix(cleaned, "x")
	cleaned = strings.TrimPrefix(cleaned, "x")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(cleaned, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, false
	}
	return value, true
}

// formatWorkBuddyMultiplier renders a multiplier the way the client does, e.g.
// "0.79x". Trailing zeros are trimmed but at least two decimals are kept.
func formatWorkBuddyMultiplier(value float64) string {
	text := strconv.FormatFloat(value, 'f', 2, 64)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(text, "0")
		text = strings.TrimRight(text, ".")
		if idx := strings.Index(text, "."); idx >= 0 && len(text)-idx-1 < 2 {
			text = strconv.FormatFloat(value, 'f', 2, 64)
		}
	}
	return text + "x"
}

// workbuddyConfigPromotion is one entry of data.modelPromotions.
type workbuddyConfigPromotion struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Enabled  *bool    `json:"enabled"`
	Priority int      `json:"priority"`
	ModelIDs []string `json:"modelIds"`
	Discount *struct {
		DisplayMode      string  `json:"displayMode"`
		Factor           float64 `json:"factor"`
		DiscountedCredit string  `json:"discountedCredits"`
	} `json:"discount"`
	Schedule *struct {
		Timezone   string `json:"timezone"`
		ValidFrom  string `json:"validFrom"`
		ValidUntil string `json:"validUntil"`
		Daily      []struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"daily"`
	} `json:"schedule"`
	Hover *struct {
		TextZh string `json:"textZh"`
		TextEn string `json:"textEn"`
	} `json:"hover"`
}

// parseWorkBuddyConfigCredits builds the rate table from a /v3/config payload.
//
// The payload is walked generically for objects carrying a "models" array so
// both the enveloped API response and the desktop cache file resolve the same
// way. Base rates come from data.models[].credits; promotions can further lower
// the effective rate and are the only signal that a model with no published
// base rate is free right now.
func parseWorkBuddyConfigCredits(body []byte, now time.Time) map[string]*workbuddyCreditsEntry {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil
	}
	rootMap, ok := root.(map[string]any)
	if !ok {
		return nil
	}
	payload := rootMap
	if data, okData := rootMap["data"].(map[string]any); okData {
		payload = data
	}

	entries := make(map[string]*workbuddyCreditsEntry)

	collectWorkBuddyModelCredits(root, entries)

	for _, promotion := range parseWorkBuddyPromotions(payload) {
		if !workbuddyPromotionActive(promotion, now) {
			continue
		}
		discounted, hasDiscounted := workbuddyPromotionRate(promotion)
		for _, id := range promotion.ModelIDs {
			key := workbuddyCreditsLookupKey(id)
			if key == "" {
				continue
			}
			note := workbuddyPromotionNote(promotion)
			if !hasDiscounted {
				// A pure scheduling entry (for example a daytime "badge" that
				// only describes an offer) must not overwrite a real rate.
				if entries[key] == nil {
					continue
				}
				if entries[key].PromotionNote == "" {
					entries[key].PromotionNote = note
					entries[key].PromotionID = promotion.ID
				}
				continue
			}
			entry := entries[key]
			if entry == nil {
				// The catalogue publishes no base rate for this model, so the
				// promotion rate is the only rate known and must be applied
				// outright rather than compared against the zero value.
				entries[key] = &workbuddyCreditsEntry{
					Effective:     discounted,
					Source:        "promotion",
					PromotionID:   promotion.ID,
					PromotionNote: note,
				}
				continue
			}
			if discounted < entry.Effective {
				entry.Effective = discounted
				entry.PromotionID = promotion.ID
				entry.PromotionNote = note
			}
		}
	}

	for _, entry := range entries {
		if entry.Source == "" {
			entry.Source = WorkBuddyCreditsSource
		}
	}
	return entries
}

// parseWorkBuddyPromotions extracts data.modelPromotions entries.
func parseWorkBuddyPromotions(payload map[string]any) []workbuddyConfigPromotion {
	raw, ok := payload["modelPromotions"]
	if !ok || raw == nil {
		return nil
	}
	encoded, errMarshal := json.Marshal(raw)
	if errMarshal != nil {
		return nil
	}
	var promotions []workbuddyConfigPromotion
	if errUnmarshal := json.Unmarshal(encoded, &promotions); errUnmarshal != nil {
		return nil
	}
	return promotions
}

// workbuddyPromotionActive reports whether the promotion applies at now.
func workbuddyPromotionActive(promotion workbuddyConfigPromotion, now time.Time) bool {
	if promotion.Enabled != nil && !*promotion.Enabled {
		return false
	}
	schedule := promotion.Schedule
	if schedule == nil {
		return true
	}
	if schedule.Timezone != "" && schedule.Timezone != "Asia/Shanghai" {
		return true
	}
	local := now.In(workbuddyCreditsZone)
	if from, ok := parseWorkBuddyScheduleTime(schedule.ValidFrom); ok && local.Before(from) {
		return false
	}
	if until, ok := parseWorkBuddyScheduleTime(schedule.ValidUntil); ok && !local.Before(until) {
		return false
	}
	if len(schedule.Daily) == 0 {
		return true
	}
	for _, window := range schedule.Daily {
		if workbuddyInDailyWindow(local, window.Start, window.End) {
			return true
		}
	}
	return false
}

// workbuddyInDailyWindow reports whether local time falls inside an HH:MM
// window, tolerating windows that cross midnight.
func workbuddyInDailyWindow(local time.Time, start, end string) bool {
	startMinutes, okStart := workbuddyParseClock(start)
	endMinutes, okEnd := workbuddyParseClock(end)
	if !okStart || !okEnd {
		return false
	}
	current := local.Hour()*60 + local.Minute()
	if startMinutes <= endMinutes {
		return current >= startMinutes && current < endMinutes
	}
	return current >= startMinutes || current < endMinutes
}

func workbuddyParseClock(value string) (int, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, false
	}
	parts := strings.SplitN(text, ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	hour, errHour := strconv.Atoi(strings.TrimSpace(parts[0]))
	minute, errMinute := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errHour != nil || errMinute != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

func parseWorkBuddyScheduleTime(value string) (time.Time, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.In(workbuddyCreditsZone), true
}

// workbuddyPromotionRate returns the discounted multiplier a promotion imposes.
func workbuddyPromotionRate(promotion workbuddyConfigPromotion) (float64, bool) {
	if promotion.Discount == nil {
		return 0, false
	}
	if value, ok := ParseWorkBuddyCreditsRaw(promotion.Discount.DiscountedCredit); ok {
		return value, true
	}
	if promotion.Discount.Factor > 0 {
		return promotion.Discount.Factor, true
	}
	// A discount block with a zero factor and no explicit rate means free.
	return 0, true
}

func workbuddyPromotionNote(promotion workbuddyConfigPromotion) string {
	if promotion.Hover != nil {
		if note := strings.TrimSpace(promotion.Hover.TextZh); note != "" {
			return note
		}
		if note := strings.TrimSpace(promotion.Hover.TextEn); note != "" {
			return note
		}
	}
	return promotion.ID
}

// collectWorkBuddyModelCredits walks a decoded JSON value looking for objects
// that carry a "models" array, recording each model's "credits" rate.
func collectWorkBuddyModelCredits(node any, out map[string]*workbuddyCreditsEntry) {
	switch typed := node.(type) {
	case map[string]any:
		if models, ok := typed["models"].([]any); ok {
			for _, item := range models {
				recordWorkBuddyModelCredit(item, out)
			}
		}
		for key, value := range typed {
			if key == "models" {
				continue
			}
			collectWorkBuddyModelCredits(value, out)
		}
	case []any:
		for _, item := range typed {
			collectWorkBuddyModelCredits(item, out)
		}
	}
}

func recordWorkBuddyModelCredit(item any, out map[string]*workbuddyCreditsEntry) {
	model, ok := item.(map[string]any)
	if !ok {
		return
	}
	id, okID := model["id"].(string)
	if !okID {
		return
	}
	key := workbuddyCreditsLookupKey(id)
	if key == "" {
		return
	}
	raw, okRaw := model["credits"].(string)
	if !okRaw {
		return
	}
	value, okParse := ParseWorkBuddyCreditsRaw(raw)
	if !okParse {
		// An unparsable rate is unknown, not free: fail closed.
		return
	}
	entry := out[key]
	if entry == nil {
		entry = &workbuddyCreditsEntry{Raw: raw, Effective: value}
		out[key] = entry
	}
	entry.Raw = raw
	entry.Base = value
	if entry.Source == "" || entry.Source == "promotion" {
		entry.Effective = value
	}
	entry.Source = WorkBuddyCreditsSource
}

// LocalWorkBuddyCreditsRate derives a rate from a static model definition when
// the realm catalogue is unavailable. It only ever reports the local "-free"
// alias convention, so callers must treat the result as a lower-confidence
// fallback.
func LocalWorkBuddyCreditsRate(modelID string) *WorkBuddyCreditsRate {
	clean := strings.ToLower(strings.TrimSpace(modelID))
	if !strings.HasSuffix(clean, "-free") {
		return nil
	}
	realm, _ := DetectWorkBuddyModelRealm(modelID)
	if realm == "cn" {
		return nil
	}
	return &WorkBuddyCreditsRate{
		Multiplier: 0,
		Effective:  formatWorkBuddyMultiplier(0),
		Free:       true,
		Source:     "local-alias",
	}
}

func workbuddyCreditsEntriesChanged(oldEntries, newEntries map[string]*workbuddyCreditsEntry) bool {
	if len(oldEntries) != len(newEntries) {
		return true
	}
	for k, newEntry := range newEntries {
		oldEntry, ok := oldEntries[k]
		if !ok || oldEntry == nil || newEntry == nil {
			return true
		}
		if math.Abs(oldEntry.Effective-newEntry.Effective) > workbuddyCreditsEpsilon ||
			oldEntry.PromotionID != newEntry.PromotionID ||
			oldEntry.Source != newEntry.Source {
			return true
		}
	}
	return false
}

type workbuddyConfigModelItem struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Credits     string `json:"credits,omitempty"`
}

// parseWorkBuddyConfigModels extracts published models from data.models in /v3/config.
func parseWorkBuddyConfigModels(body []byte, realmKey string, entries map[string]*workbuddyCreditsEntry) []*ModelInfo {
	var root struct {
		Data struct {
			Models []workbuddyConfigModelItem `json:"models"`
		} `json:"data"`
		Models []workbuddyConfigModelItem `json:"models"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil
	}

	items := root.Data.Models
	if len(items) == 0 {
		items = root.Models
	}
	if len(items) == 0 {
		return nil
	}

	var models []*ModelInfo
	seen := make(map[string]bool, len(items))

	for _, item := range items {
		cleanID := strings.TrimSpace(item.ID)
		if cleanID == "" {
			continue
		}
		lowerID := strings.ToLower(cleanID)
		if seen[lowerID] {
			continue
		}
		seen[lowerID] = true

		m := buildWorkBuddyDynamicModel(cleanID, item.DisplayName, item.Name, realmKey)
		lookupKey := workbuddyCreditsLookupKey(cleanID)
		if entry, ok := entries[lookupKey]; ok && entry != nil && entry.Source != "" {
			m.Credits = &ModelCredits{
				Multiplier:    entry.Effective,
				Source:        entry.Source,
				PromotionID:   entry.PromotionID,
				PromotionNote: entry.PromotionNote,
			}
		}
		models = append(models, m)
	}

	return models
}

func buildWorkBuddyDynamicModel(id, displayName, name, realmKey string) *ModelInfo {
	lowerID := strings.ToLower(id)
	ownedBy := inferWorkBuddyModelOwner(lowerID, realmKey)
	dn := strings.TrimSpace(displayName)
	if dn == "" {
		dn = strings.TrimSpace(name)
	}
	if dn == "" {
		dn = formatWorkBuddyDynamicDisplayName(id, realmKey)
	}

	m := &ModelInfo{
		ID:                  id,
		Object:              "model",
		OwnedBy:             ownedBy,
		Type:                "workbuddy",
		DisplayName:         dn,
		ContextLength:       1048576,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    64000,
	}

	if shouldSupportThinking(lowerID) {
		m.Thinking = &ThinkingSupport{
			Levels: []string{"low", "medium", "high", "max"},
		}
	}
	return m
}

func inferWorkBuddyModelOwner(lowerID, realmKey string) string {
	switch {
	case strings.HasPrefix(lowerID, "deepseek"):
		return "deepseek"
	case strings.HasPrefix(lowerID, "gpt"), strings.HasPrefix(lowerID, "o1"), strings.HasPrefix(lowerID, "o3"), strings.HasPrefix(lowerID, "o4"):
		return "openai"
	case strings.HasPrefix(lowerID, "claude"):
		return "anthropic"
	case strings.HasPrefix(lowerID, "glm"):
		return "zhipu"
	case strings.HasPrefix(lowerID, "kimi"):
		return "moonshotai"
	case strings.HasPrefix(lowerID, "minimax"):
		return "minimax"
	case strings.HasPrefix(lowerID, "hy"), strings.HasPrefix(lowerID, "hunyuan"):
		return "tencent"
	case strings.HasPrefix(lowerID, "qwen"):
		return "alibaba"
	case strings.HasPrefix(lowerID, "doubao"):
		return "bytedance"
	case strings.HasPrefix(lowerID, "gemini"):
		return "google"
	case strings.HasPrefix(lowerID, "grok"):
		return "xai"
	default:
		if strings.EqualFold(strings.TrimSpace(realmKey), "cn") {
			return "tencent"
		}
		return "workbuddy"
	}
}

func shouldSupportThinking(lowerID string) bool {
	return strings.Contains(lowerID, "deepseek") ||
		strings.Contains(lowerID, "reasoner") ||
		strings.Contains(lowerID, "thinking") ||
		strings.Contains(lowerID, "glm") ||
		strings.Contains(lowerID, "kimi") ||
		strings.Contains(lowerID, "claude") ||
		strings.HasPrefix(lowerID, "o1") ||
		strings.HasPrefix(lowerID, "o3") ||
		strings.HasPrefix(lowerID, "o4")
}

func formatWorkBuddyDynamicDisplayName(id, realmKey string) string {
	tag := " (WorkBuddy)"
	if strings.EqualFold(strings.TrimSpace(realmKey), "cn") {
		tag = " (CodeBuddy)"
	}
	parts := strings.Split(id, "-")
	for i, p := range parts {
		lp := strings.ToLower(p)
		switch {
		case lp == "gpt":
			parts[i] = "GPT"
		case lp == "glm":
			parts[i] = "GLM"
		case lp == "hy":
			parts[i] = "Hunyuan"
		case strings.HasPrefix(lp, "hy") && len(lp) > 2 && lp[2] >= '0' && lp[2] <= '9':
			parts[i] = "Hunyuan " + p[2:]
		case lp == "deepseek":
			parts[i] = "DeepSeek"
		case lp == "claude":
			parts[i] = "Claude"
		case lp == "kimi":
			parts[i] = "Kimi"
		case lp == "minimax":
			parts[i] = "MiniMax"
		case lp == "qwen":
			parts[i] = "Qwen"
		case lp == "flash":
			parts[i] = "Flash"
		case lp == "pro":
			parts[i] = "Pro"
		default:
			if len(p) > 0 {
				parts[i] = strings.ToUpper(p[:1]) + p[1:]
			}
		}
	}

	var formatted []string
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if (p == "GPT" || p == "GLM") && i+1 < len(parts) && len(parts[i+1]) > 0 && parts[i+1][0] >= '0' && parts[i+1][0] <= '9' {
			formatted = append(formatted, p+"-"+parts[i+1])
			i++
		} else {
			formatted = append(formatted, p)
		}
	}
	return strings.Join(formatted, " ") + tag
}
