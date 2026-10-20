// Package registry provides centralized model management for all AI service providers.
package registry

import (
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
)

const (
	// QoderCatalogTTL is how long dynamic models remain fresh.
	QoderCatalogTTL = 30 * time.Minute
	// QoderCatalogFailureTTL is how long a failed fetch is remembered before re-attempting.
	QoderCatalogFailureTTL = 2 * time.Minute
)

type qoderModelsStore struct {
	mu          sync.RWMutex
	intlModels  []*ModelInfo
	cnModels    []*ModelInfo
	intlFetched time.Time
	cnFetched   time.Time
	intlFailed  bool
	cnFailed    bool
	nowFunc     func() time.Time
}

var qoderCatalogStore = &qoderModelsStore{
	nowFunc: time.Now,
}

// SetQoderCatalogNowFunc overrides the clock for deterministic testing.
func SetQoderCatalogNowFunc(fn func() time.Time) {
	qoderCatalogStore.mu.Lock()
	defer qoderCatalogStore.mu.Unlock()
	if fn == nil {
		fn = time.Now
	}
	qoderCatalogStore.nowFunc = fn
}

// IsQoderCatalogFresh reports whether dynamic models for the realm are fresh.
func IsQoderCatalogFresh(realm string) bool {
	cleanRealm := strings.ToLower(strings.TrimSpace(realm))
	qoderCatalogStore.mu.RLock()
	defer qoderCatalogStore.mu.RUnlock()

	now := qoderCatalogStore.nowFunc()
	var fetched time.Time
	var failed bool
	var hasModels bool

	if cleanRealm == "cn" {
		fetched = qoderCatalogStore.cnFetched
		failed = qoderCatalogStore.cnFailed
		hasModels = len(qoderCatalogStore.cnModels) > 0
	} else {
		fetched = qoderCatalogStore.intlFetched
		failed = qoderCatalogStore.intlFailed
		hasModels = len(qoderCatalogStore.intlModels) > 0
	}

	if fetched.IsZero() {
		return false
	}
	if failed {
		return now.Sub(fetched) < QoderCatalogFailureTTL
	}
	return hasModels && now.Sub(fetched) < QoderCatalogTTL
}

// MarkQoderCatalogFetched records the timestamp and result of an upstream catalog fetch.
func MarkQoderCatalogFetched(realm string, failed bool) {
	cleanRealm := strings.ToLower(strings.TrimSpace(realm))
	qoderCatalogStore.mu.Lock()
	defer qoderCatalogStore.mu.Unlock()

	now := qoderCatalogStore.nowFunc()
	if cleanRealm == "cn" {
		qoderCatalogStore.cnFetched = now
		qoderCatalogStore.cnFailed = failed
	} else {
		qoderCatalogStore.intlFetched = now
		qoderCatalogStore.intlFailed = failed
	}
}

// ClearQoderCatalogModels resets stored dynamic models for a realm, or all realms if realm is empty.
func ClearQoderCatalogModels(realm string) {
	qoderCatalogStore.mu.Lock()
	defer qoderCatalogStore.mu.Unlock()
	switch strings.ToLower(strings.TrimSpace(realm)) {
	case "cn":
		qoderCatalogStore.cnModels = nil
		qoderCatalogStore.cnFetched = time.Time{}
		qoderCatalogStore.cnFailed = false
	case "intl":
		qoderCatalogStore.intlModels = nil
		qoderCatalogStore.intlFetched = time.Time{}
		qoderCatalogStore.intlFailed = false
	default:
		qoderCatalogStore.intlModels = nil
		qoderCatalogStore.cnModels = nil
		qoderCatalogStore.intlFetched = time.Time{}
		qoderCatalogStore.cnFetched = time.Time{}
		qoderCatalogStore.intlFailed = false
		qoderCatalogStore.cnFailed = false
	}
}

// SetQoderCatalogModels stores dynamic models discovered from upstream for a realm.
// It returns true if the dynamic model definitions have changed.
// NOTE: Models are purely dynamic and NEVER hardcoded.
func SetQoderCatalogModels(realm string, models []*ModelInfo) bool {
	cleanRealm := strings.ToLower(strings.TrimSpace(realm))
	qoderCatalogStore.mu.Lock()
	defer qoderCatalogStore.mu.Unlock()

	now := qoderCatalogStore.nowFunc()
	if cleanRealm == "cn" {
		qoderCatalogStore.cnFetched = now
		qoderCatalogStore.cnFailed = false
	} else {
		qoderCatalogStore.intlFetched = now
		qoderCatalogStore.intlFailed = false
	}

	var existing []*ModelInfo
	if cleanRealm == "cn" {
		existing = qoderCatalogStore.cnModels
	} else {
		existing = qoderCatalogStore.intlModels
	}

	if !qoderModelsSliceChanged(existing, models) {
		return false
	}

	cloned := cloneModelInfos(models)
	if cleanRealm == "cn" {
		qoderCatalogStore.cnModels = cloned
	} else {
		qoderCatalogStore.intlModels = cloned
	}
	return true
}

// GetQoderModels returns dynamic models discovered from upstream for a realm ("intl" or "cn").
// If no models have been fetched yet from upstream, it returns an empty slice.
// Hardcoded model lists are strictly prohibited.
func GetQoderModels(realm string) []*ModelInfo {
	cleanRealm := strings.ToLower(strings.TrimSpace(realm))
	qoderCatalogStore.mu.RLock()
	defer qoderCatalogStore.mu.RUnlock()
	if cleanRealm == "cn" {
		if len(qoderCatalogStore.cnModels) == 0 {
			return []*ModelInfo{}
		}
		return cloneModelInfos(qoderCatalogStore.cnModels)
	}
	if len(qoderCatalogStore.intlModels) == 0 {
		return []*ModelInfo{}
	}
	return cloneModelInfos(qoderCatalogStore.intlModels)
}

// GetAllQoderModels returns all unique dynamic Qoder models across both realms.
func GetAllQoderModels() []*ModelInfo {
	seen := make(map[string]bool)
	var all []*ModelInfo
	for _, m := range append(GetQoderModels("intl"), GetQoderModels("cn")...) {
		if m != nil && !seen[strings.ToLower(m.ID)] {
			seen[strings.ToLower(m.ID)] = true
			all = append(all, m)
		}
	}
	return all
}

// ParseQoderCatalogModels parses raw upstream Qoder model catalog JSON into ModelInfo structs.
func ParseQoderCatalogModels(body []byte, realm string) []*ModelInfo {
	if len(body) == 0 {
		return nil
	}

	parsed := gjson.ParseBytes(body)

	// Determine array of model definitions
	var items []gjson.Result
	if d := parsed.Get("data.models"); d.IsArray() {
		items = d.Array()
	} else if d := parsed.Get("data"); d.IsArray() {
		items = d.Array()
	} else if d := parsed.Get("result.models"); d.IsArray() {
		items = d.Array()
	} else if d := parsed.Get("result"); d.IsArray() {
		items = d.Array()
	} else if d := parsed.Get("models"); d.IsArray() {
		items = d.Array()
	} else if parsed.IsArray() {
		items = parsed.Array()
	}

	if len(items) == 0 {
		return nil
	}

	ownedBy := "qoder"
	if strings.EqualFold(strings.TrimSpace(realm), "cn") {
		ownedBy = "alibaba"
	}

	models := make([]*ModelInfo, 0, len(items))
	seen := make(map[string]bool, len(items))

	for _, item := range items {
		id := strings.TrimSpace(item.Get("id").String())
		if id == "" {
			id = strings.TrimSpace(item.Get("model").String())
		}
		if id == "" {
			id = strings.TrimSpace(item.Get("name").String())
		}
		if id == "" {
			id = strings.TrimSpace(item.Get("modelName").String())
		}
		if id == "" {
			id = strings.TrimSpace(item.Get("model_name").String())
		}
		if id == "" || seen[strings.ToLower(id)] {
			continue
		}
		seen[strings.ToLower(id)] = true

		displayName := strings.TrimSpace(item.Get("display_name").String())
		if displayName == "" {
			displayName = strings.TrimSpace(item.Get("displayName").String())
		}
		if displayName == "" {
			displayName = strings.TrimSpace(item.Get("label").String())
		}
		if displayName == "" {
			displayName = id
		}

		maxInputTokens := int(item.Get("max_input_tokens").Int())
		if maxInputTokens == 0 {
			maxInputTokens = int(item.Get("default_context_window").Int())
		}
		if maxInputTokens == 0 {
			maxInputTokens = 200000
		}

		maxCompletionTokens := int(item.Get("max_completion_tokens").Int())
		if maxCompletionTokens == 0 {
			maxCompletionTokens = int(item.Get("max_output_tokens").Int())
		}
		if maxCompletionTokens == 0 {
			maxCompletionTokens = 64000
		}

		m := &ModelInfo{
			ID:                  id,
			Object:              "model",
			OwnedBy:             ownedBy,
			Type:                "qoder",
			DisplayName:         displayName,
			ContextLength:       maxInputTokens,
			InputTokenLimit:     maxInputTokens,
			MaxCompletionTokens: maxCompletionTokens,
			OutputTokenLimit:    maxCompletionTokens,
		}

		// Reasoning / thinking efforts
		if effortsResult := item.Get("efforts"); effortsResult.IsArray() {
			var levels []string
			for _, e := range effortsResult.Array() {
				if s := strings.TrimSpace(e.String()); s != "" {
					levels = append(levels, s)
				}
			}
			if len(levels) > 0 {
				m.Thinking = &ThinkingSupport{
					Levels: levels,
				}
			}
		}

		// Multimodal vision capabilities
		if item.Get("is_vl").Bool() || item.Get("vision").Bool() {
			m.SupportedInputModalities = []string{"TEXT", "IMAGE"}
		}

		// Credits multiplier
		if priceFactor := item.Get("price_factor"); priceFactor.Exists() {
			m.Credits = &ModelCredits{
				Multiplier: priceFactor.Float(),
				Source:     "qoder",
			}
		}

		models = append(models, m)
	}

	return models
}

func qoderModelsSliceChanged(oldList, newList []*ModelInfo) bool {
	if len(oldList) != len(newList) {
		return true
	}
	for i := range oldList {
		if oldList[i] == nil || newList[i] == nil {
			if oldList[i] != newList[i] {
				return true
			}
			continue
		}
		if oldList[i].ID != newList[i].ID {
			return true
		}
		if oldList[i].ContextLength != newList[i].ContextLength {
			return true
		}
		if (oldList[i].Credits == nil) != (newList[i].Credits == nil) {
			return true
		}
		if oldList[i].Credits != nil && newList[i].Credits != nil {
			if oldList[i].Credits.Multiplier != newList[i].Credits.Multiplier {
				return true
			}
		}
	}
	return false
}
