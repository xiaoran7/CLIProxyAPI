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

var staticQoderIntlModels = []*ModelInfo{
	{
		ID:                       "ultimate",
		Object:                   "model",
		OwnedBy:                  "qoder",
		Type:                     "qoder",
		DisplayName:              "Ultimate",
		ContextLength:            1000000,
		MaxCompletionTokens:      64000,
		InputTokenLimit:          1000000,
		OutputTokenLimit:         64000,
		SupportedInputModalities: []string{"TEXT", "IMAGE"},
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "medium", "high", "xhigh", "max"},
		},
		Credits: &ModelCredits{
			Multiplier: 1.6,
			Source:     "qoder",
		},
	},
	{
		ID:                       "auto",
		Object:                   "model",
		OwnedBy:                  "qoder",
		Type:                     "qoder",
		DisplayName:              "Auto",
		ContextLength:            1000000,
		MaxCompletionTokens:      64000,
		InputTokenLimit:          1000000,
		OutputTokenLimit:         64000,
		SupportedInputModalities: []string{"TEXT", "IMAGE"},
		Credits: &ModelCredits{
			Multiplier: 1.0,
			Source:     "qoder",
		},
	},
	{
		ID:                       "qwen3.8-flash",
		Object:                   "model",
		OwnedBy:                  "alibaba",
		Type:                     "qoder",
		DisplayName:              "Qwen 3.8 Flash",
		ContextLength:            1000000,
		MaxCompletionTokens:      64000,
		InputTokenLimit:          1000000,
		OutputTokenLimit:         64000,
		SupportedInputModalities: []string{"TEXT", "IMAGE"},
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "medium", "high"},
		},
		Credits: &ModelCredits{
			Multiplier: 0.0,
			Source:     "qoder",
		},
	},
	{
		ID:                  "deepseek-v4-pro",
		Object:              "model",
		OwnedBy:             "deepseek",
		Type:                "qoder",
		DisplayName:         "DeepSeek V4 Pro",
		ContextLength:       1048576,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    64000,
		Thinking: &ThinkingSupport{
			Levels: []string{"high", "max"},
		},
		Credits: &ModelCredits{
			Multiplier: 1.5,
			Source:     "qoder",
		},
	},
	{
		ID:                  "deepseek-v4-flash",
		Object:              "model",
		OwnedBy:             "deepseek",
		Type:                "qoder",
		DisplayName:         "DeepSeek V4 Flash",
		ContextLength:       1048576,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    64000,
		Credits: &ModelCredits{
			Multiplier: 0.5,
			Source:     "qoder",
		},
	},
	{
		ID:                       "qwen-max-latest",
		Object:                   "model",
		OwnedBy:                  "alibaba",
		Type:                     "qoder",
		DisplayName:              "Qwen Max (Latest)",
		ContextLength:            1000000,
		MaxCompletionTokens:      64000,
		InputTokenLimit:          1000000,
		OutputTokenLimit:         64000,
		SupportedInputModalities: []string{"TEXT", "IMAGE"},
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "medium", "high"},
		},
		Credits: &ModelCredits{
			Multiplier: 1.2,
			Source:     "qoder",
		},
	},
	{
		ID:                       "qwen-plus",
		Object:                   "model",
		OwnedBy:                  "alibaba",
		Type:                     "qoder",
		DisplayName:              "Qwen Plus",
		ContextLength:            1000000,
		MaxCompletionTokens:      64000,
		InputTokenLimit:          1000000,
		OutputTokenLimit:         64000,
		SupportedInputModalities: []string{"TEXT", "IMAGE"},
		Credits: &ModelCredits{
			Multiplier: 0.8,
			Source:     "qoder",
		},
	},
	{
		ID:                  "kimi-code",
		Object:              "model",
		OwnedBy:             "moonshot",
		Type:                "qoder",
		DisplayName:         "Kimi Code",
		ContextLength:       256000,
		MaxCompletionTokens: 32000,
		InputTokenLimit:     256000,
		OutputTokenLimit:    32000,
		Credits: &ModelCredits{
			Multiplier: 1.0,
			Source:     "qoder",
		},
	},
	{
		ID:                  "glm-5",
		Object:              "model",
		OwnedBy:             "zhipu",
		Type:                "qoder",
		DisplayName:         "GLM-5",
		ContextLength:       256000,
		MaxCompletionTokens: 32000,
		InputTokenLimit:     256000,
		OutputTokenLimit:    32000,
		Credits: &ModelCredits{
			Multiplier: 1.0,
			Source:     "qoder",
		},
	},
}

var staticQoderCNModels = []*ModelInfo{
	{
		ID:                       "auto",
		Object:                   "model",
		OwnedBy:                  "alibaba",
		Type:                     "qoder",
		DisplayName:              "Auto (Qoder CN)",
		ContextLength:            1000000,
		MaxCompletionTokens:      64000,
		InputTokenLimit:          1000000,
		OutputTokenLimit:         64000,
		SupportedInputModalities: []string{"TEXT", "IMAGE"},
		Credits: &ModelCredits{
			Multiplier: 1.0,
			Source:     "qoder",
		},
	},
	{
		ID:                       "qwen3.8-flash",
		Object:                   "model",
		OwnedBy:                  "alibaba",
		Type:                     "qoder",
		DisplayName:              "Qwen 3.8 Flash (Qoder CN)",
		ContextLength:            1000000,
		MaxCompletionTokens:      64000,
		InputTokenLimit:          1000000,
		OutputTokenLimit:         64000,
		SupportedInputModalities: []string{"TEXT", "IMAGE"},
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "medium", "high"},
		},
		Credits: &ModelCredits{
			Multiplier: 0.0,
			Source:     "qoder",
		},
	},
	{
		ID:                  "deepseek-v4-pro",
		Object:              "model",
		OwnedBy:             "deepseek",
		Type:                "qoder",
		DisplayName:         "DeepSeek V4 Pro (Qoder CN)",
		ContextLength:       1048576,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    64000,
		Thinking: &ThinkingSupport{
			Levels: []string{"high", "max"},
		},
		Credits: &ModelCredits{
			Multiplier: 1.5,
			Source:     "qoder",
		},
	},
	{
		ID:                  "deepseek-v4-flash",
		Object:              "model",
		OwnedBy:             "deepseek",
		Type:                "qoder",
		DisplayName:         "DeepSeek V4 Flash (Qoder CN)",
		ContextLength:       1048576,
		MaxCompletionTokens: 64000,
		InputTokenLimit:     1048576,
		OutputTokenLimit:    64000,
		Credits: &ModelCredits{
			Multiplier: 0.5,
			Source:     "qoder",
		},
	},
	{
		ID:                       "qwen-max-latest",
		Object:                   "model",
		OwnedBy:                  "alibaba",
		Type:                     "qoder",
		DisplayName:              "Qwen Max (Qoder CN)",
		ContextLength:            1000000,
		MaxCompletionTokens:      64000,
		InputTokenLimit:          1000000,
		OutputTokenLimit:         64000,
		SupportedInputModalities: []string{"TEXT", "IMAGE"},
		Thinking: &ThinkingSupport{
			Levels: []string{"low", "medium", "high"},
		},
		Credits: &ModelCredits{
			Multiplier: 1.2,
			Source:     "qoder",
		},
	},
	{
		ID:                       "qwen-plus",
		Object:                   "model",
		OwnedBy:                  "alibaba",
		Type:                     "qoder",
		DisplayName:              "Qwen Plus (Qoder CN)",
		ContextLength:            1000000,
		MaxCompletionTokens:      64000,
		InputTokenLimit:          1000000,
		OutputTokenLimit:         64000,
		SupportedInputModalities: []string{"TEXT", "IMAGE"},
		Credits: &ModelCredits{
			Multiplier: 0.8,
			Source:     "qoder",
		},
	},
	{
		ID:                  "kimi",
		Object:              "model",
		OwnedBy:             "moonshot",
		Type:                "qoder",
		DisplayName:         "Kimi (Qoder CN)",
		ContextLength:       256000,
		MaxCompletionTokens: 32000,
		InputTokenLimit:     256000,
		OutputTokenLimit:    32000,
		Credits: &ModelCredits{
			Multiplier: 1.0,
			Source:     "qoder",
		},
	},
	{
		ID:                  "glm",
		Object:              "model",
		OwnedBy:             "zhipu",
		Type:                "qoder",
		DisplayName:         "GLM (Qoder CN)",
		ContextLength:       256000,
		MaxCompletionTokens: 32000,
		InputTokenLimit:     256000,
		OutputTokenLimit:    32000,
		Credits: &ModelCredits{
			Multiplier: 1.0,
			Source:     "qoder",
		},
	},
}

// GetQoderDynamicModels returns dynamic models discovered from upstream for a realm.
func GetQoderDynamicModels(realm string) []*ModelInfo {
	cleanRealm := strings.ToLower(strings.TrimSpace(realm))
	qoderCatalogStore.mu.RLock()
	defer qoderCatalogStore.mu.RUnlock()
	if cleanRealm == "cn" {
		return cloneModelInfos(qoderCatalogStore.cnModels)
	}
	return cloneModelInfos(qoderCatalogStore.intlModels)
}

// GetQoderModels returns dynamic models discovered from upstream merged with baseline static models for a realm ("intl" or "cn").
func GetQoderModels(realm string) []*ModelInfo {
	cleanRealm := strings.ToLower(strings.TrimSpace(realm))
	var staticModels []*ModelInfo
	var dynamicModels []*ModelInfo

	if cleanRealm == "cn" {
		staticModels = staticQoderCNModels
		qoderCatalogStore.mu.RLock()
		dynamicModels = qoderCatalogStore.cnModels
		qoderCatalogStore.mu.RUnlock()
	} else {
		staticModels = staticQoderIntlModels
		qoderCatalogStore.mu.RLock()
		dynamicModels = qoderCatalogStore.intlModels
		qoderCatalogStore.mu.RUnlock()
	}

	seen := make(map[string]bool, len(staticModels)+len(dynamicModels))
	result := make([]*ModelInfo, 0, len(staticModels)+len(dynamicModels))

	// 1. Add static baseline models
	for _, m := range staticModels {
		if m != nil && !seen[strings.ToLower(m.ID)] {
			seen[strings.ToLower(m.ID)] = true
			result = append(result, cloneModelInfo(m))
		}
	}

	// 2. Dynamic models discovered from upstream enrich or override static models
	for _, m := range dynamicModels {
		if m == nil {
			continue
		}
		idLower := strings.ToLower(m.ID)
		replaced := false
		for i, existing := range result {
			if existing != nil && strings.ToLower(existing.ID) == idLower {
				result[i] = cloneModelInfo(m)
				replaced = true
				break
			}
		}
		if !replaced && !seen[idLower] {
			seen[idLower] = true
			result = append(result, cloneModelInfo(m))
		}
	}

	return result
}

// GetAllQoderModels returns all unique dynamic and baseline Qoder models across both realms.
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
	if d := parsed.Get("chat"); d.IsArray() {
		items = d.Array()
	} else if d := parsed.Get("data.chat"); d.IsArray() {
		items = d.Array()
	} else if d := parsed.Get("data.models"); d.IsArray() {
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
		id := strings.TrimSpace(item.Get("key").String())
		if id == "" {
			id = strings.TrimSpace(item.Get("id").String())
		}
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
			if cc := item.Get("context_config"); cc.IsObject() {
				for _, conf := range cc.Map() {
					if tc := int(conf.Get("token_count").Int()); tc > maxInputTokens {
						maxInputTokens = tc
					}
				}
			}
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
		} else if item.Get("is_reasoning").Bool() {
			m.Thinking = &ThinkingSupport{
				Levels: []string{"low", "medium", "high"},
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
