package registry

import (
	"strings"
	"testing"
	"time"
)

func TestQoderModelsBaseline(t *testing.T) {
	ClearQoderCatalogModels("")

	intlModels := GetQoderModels("intl")
	if len(intlModels) == 0 {
		t.Fatalf("expected non-empty baseline models for intl, got %d", len(intlModels))
	}

	cnModels := GetQoderModels("cn")
	if len(cnModels) == 0 {
		t.Fatalf("expected non-empty baseline models for cn, got %d", len(cnModels))
	}

	// Dynamic models should be empty when catalog is cold
	dynamicIntl := GetQoderDynamicModels("intl")
	if len(dynamicIntl) != 0 {
		t.Fatalf("expected 0 dynamic models for cold intl catalog, got %d", len(dynamicIntl))
	}
}

func TestParseQoderCatalogModels(t *testing.T) {
	rawJSON := []byte(`{
		"data": [
			{
				"id": "qwen3.8-flash",
				"display_name": "Qwen 3.8 Flash",
				"is_vl": true,
				"price_factor": 0.0,
				"efforts": ["low", "medium", "high"],
				"max_input_tokens": 1000000,
				"max_completion_tokens": 64000
			},
			{
				"id": "deepseek-v4-pro",
				"display_name": "DeepSeek V4 Pro",
				"is_vl": false,
				"price_factor": 1.5,
				"efforts": ["high", "max"],
				"max_input_tokens": 1048576,
				"max_completion_tokens": 128000
			}
		]
	}`)

	models := ParseQoderCatalogModels(rawJSON, "intl")
	if len(models) != 2 {
		t.Fatalf("expected 2 parsed models, got %d", len(models))
	}

	m1 := models[0]
	if m1.ID != "qwen3.8-flash" {
		t.Errorf("expected ID qwen3.8-flash, got %s", m1.ID)
	}
	if m1.DisplayName != "Qwen 3.8 Flash" {
		t.Errorf("expected DisplayName 'Qwen 3.8 Flash', got %s", m1.DisplayName)
	}
	if m1.ContextLength != 1000000 {
		t.Errorf("expected ContextLength 1000000, got %d", m1.ContextLength)
	}
	if m1.MaxCompletionTokens != 64000 {
		t.Errorf("expected MaxCompletionTokens 64000, got %d", m1.MaxCompletionTokens)
	}
	if m1.Credits == nil || m1.Credits.Multiplier != 0.0 {
		t.Errorf("expected Credits Multiplier 0.0, got %v", m1.Credits)
	}
	if len(m1.SupportedInputModalities) != 2 || m1.SupportedInputModalities[1] != "IMAGE" {
		t.Errorf("expected vision input modalities, got %v", m1.SupportedInputModalities)
	}
	if m1.Thinking == nil || len(m1.Thinking.Levels) != 3 {
		t.Errorf("expected 3 thinking levels, got %v", m1.Thinking)
	}

	m2 := models[1]
	if m2.ID != "deepseek-v4-pro" {
		t.Errorf("expected ID deepseek-v4-pro, got %s", m2.ID)
	}
	if m2.Credits == nil || m2.Credits.Multiplier != 1.5 {
		t.Errorf("expected Credits Multiplier 1.5, got %v", m2.Credits)
	}
	if len(m2.SupportedInputModalities) != 0 {
		t.Errorf("expected no vision modalities for m2, got %v", m2.SupportedInputModalities)
	}
}

func TestSetAndGetQoderModels(t *testing.T) {
	ClearQoderCatalogModels("")
	defer ClearQoderCatalogModels("")

	rawJSON := []byte(`{
		"data": [
			{"id": "qwen3.8-flash", "display_name": "Qwen 3.8 Flash"}
		]
	}`)

	models := ParseQoderCatalogModels(rawJSON, "intl")
	changed := SetQoderCatalogModels("intl", models)
	if !changed {
		t.Fatal("expected SetQoderCatalogModels to report change on first write")
	}

	dynamicGot := GetQoderDynamicModels("intl")
	if len(dynamicGot) != 1 || dynamicGot[0].ID != "qwen3.8-flash" {
		t.Fatalf("expected 1 dynamic model 'qwen3.8-flash', got %v", dynamicGot)
	}

	// CN realm dynamic models should still be empty
	cnGot := GetQoderDynamicModels("cn")
	if len(cnGot) != 0 {
		t.Fatalf("expected 0 dynamic models for cn, got %d", len(cnGot))
	}

	// Merged models should contain the dynamic model overlay
	got := GetQoderModels("intl")
	found := false
	for _, m := range got {
		if m != nil && m.ID == "qwen3.8-flash" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'qwen3.8-flash' to be present in merged models")
	}

	// Repeated identical set should report no change
	changedAgain := SetQoderCatalogModels("intl", models)
	if changedAgain {
		t.Fatal("expected SetQoderCatalogModels to report no change for identical list")
	}
}

func TestQoderCatalogFreshnessAndTTL(t *testing.T) {
	ClearQoderCatalogModels("")
	defer ClearQoderCatalogModels("")

	currentTime := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	SetQoderCatalogNowFunc(func() time.Time {
		return currentTime
	})
	defer SetQoderCatalogNowFunc(nil)

	if IsQoderCatalogFresh("intl") {
		t.Fatal("expected cold catalog to be not fresh")
	}

	rawJSON := []byte(`{"data":[{"id":"qwen3.8-flash"}]}`)
	models := ParseQoderCatalogModels(rawJSON, "intl")
	SetQoderCatalogModels("intl", models)

	if !IsQoderCatalogFresh("intl") {
		t.Fatal("expected catalog to be fresh immediately after set")
	}

	// Advance time by 25 minutes (within 30m TTL)
	currentTime = currentTime.Add(25 * time.Minute)
	if !IsQoderCatalogFresh("intl") {
		t.Fatal("expected catalog to be fresh at 25 minutes")
	}

	// Advance time past 30m TTL
	currentTime = currentTime.Add(6 * time.Minute)
	if IsQoderCatalogFresh("intl") {
		t.Fatal("expected catalog to expire after 31 minutes")
	}

	// Mark failed: failure cooldown is 2m
	MarkQoderCatalogFetched("intl", true)
	if !IsQoderCatalogFresh("intl") {
		t.Fatal("expected failed fetch to be in cooldown right after mark")
	}
	currentTime = currentTime.Add(3 * time.Minute)
	if IsQoderCatalogFresh("intl") {
		t.Fatal("expected failure cooldown to expire after 3m")
	}
}

func TestParseQoderCatalogModels_GatewayChatFormat(t *testing.T) {
	gatewayJSON := []byte(`{
		"chat": [
			{
				"key": "auto",
				"display_name": "Auto",
				"is_vl": true,
				"is_reasoning": true,
				"price_factor": 0.5,
				"context_config": {
					"200K": {"token_count": 200000, "is_default": true}
				}
			},
			{
				"key": "qmodel_38max",
				"display_name": "Qwen3.8-Max",
				"is_vl": true,
				"is_reasoning": true,
				"price_factor": 0.5,
				"context_config": {
					"1M": {"token_count": 1000000},
					"200K": {"token_count": 200000, "is_default": true},
					"400K": {"token_count": 400000}
				}
			},
			{
				"key": "dfmodel",
				"display_name": "DeepSeek-Flash",
				"is_vl": true,
				"is_reasoning": false,
				"price_factor": 0.0,
				"context_config": {
					"1M": {"token_count": 1000000}
				}
			}
		]
	}`)

	models := ParseQoderCatalogModels(gatewayJSON, "cn")
	if len(models) < 3 {
		t.Fatalf("expected at least 3 parsed models from gateway chat format, got %d", len(models))
	}

	byID := make(map[string]*ModelInfo)
	for _, m := range models {
		if m != nil {
			byID[strings.ToLower(m.ID)] = m
		}
	}

	m0, ok0 := byID["auto"]
	if !ok0 {
		t.Fatalf("expected model 'auto' to be present")
	}
	if m0.ContextLength != 200000 {
		t.Errorf("expected ContextLength 200000, got %d", m0.ContextLength)
	}
	if m0.Thinking == nil {
		t.Errorf("expected Thinking support for auto")
	}

	m1, ok1 := byID["qmodel_38max"]
	if !ok1 {
		t.Fatalf("expected model 'qmodel_38max' to be present")
	}
	if m1.DisplayName != "Qwen3.8-Max" {
		t.Errorf("expected DisplayName 'Qwen3.8-Max', got %s", m1.DisplayName)
	}
	if m1.ContextLength != 1000000 {
		t.Errorf("expected ContextLength 1000000 from context_config max, got %d", m1.ContextLength)
	}
	if m1.Credits == nil || m1.Credits.Multiplier != 0.5 {
		t.Errorf("expected Credits Multiplier 0.5, got %v", m1.Credits)
	}

	m2, ok2 := byID["dfmodel"]
	if !ok2 {
		t.Fatalf("expected model 'dfmodel' to be present")
	}
	if m2.Thinking != nil {
		t.Errorf("expected no Thinking support for non-reasoning dfmodel")
	}
	if m2.OwnedBy != "deepseek" {
		t.Errorf("expected dfmodel OwnedBy 'deepseek', got %s", m2.OwnedBy)
	}

	// Verify enriched canonical models and aliases
	mDSFlash, okDSFlash := byID["deepseek-flash"]
	if !okDSFlash {
		t.Fatalf("expected canonical model 'deepseek-flash' to be present")
	}
	if mDSFlash.OwnedBy != "deepseek" {
		t.Errorf("expected deepseek-flash OwnedBy 'deepseek', got %s", mDSFlash.OwnedBy)
	}
	if mDSFlash.ContextLength != 1000000 {
		t.Errorf("expected deepseek-flash ContextLength 1000000, got %d", mDSFlash.ContextLength)
	}

	if _, okDSChat := byID["deepseek-chat"]; !okDSChat {
		t.Errorf("expected alias 'deepseek-chat' to be present")
	}
	if _, okDSV3 := byID["deepseek-v3"]; !okDSV3 {
		t.Errorf("expected alias 'deepseek-v3' to be present")
	}

	mQwen38Max, okQwen := byID["qwen3.8-max"]
	if !okQwen {
		t.Fatalf("expected canonical model 'qwen3.8-max' to be present")
	}
	if mQwen38Max.OwnedBy != "alibaba" {
		t.Errorf("expected qwen3.8-max OwnedBy 'alibaba', got %s", mQwen38Max.OwnedBy)
	}
	if _, okQwenMax := byID["qwen-max"]; !okQwenMax {
		t.Errorf("expected alias 'qwen-max' to be present")
	}
}

func TestQoderModelAliasAndKeyResolution(t *testing.T) {
	// DeepSeek aliases -> dfmodel
	dsCases := []string{
		"dfmodel",
		"deepseek-flash",
		"deepseek-chat",
		"deepseek-v3",
		"deepseek-v4-flash",
		"deepseek",
		"qoder/deepseek-flash",
		"qoder-cn/deepseek-chat-free",
	}
	for _, tc := range dsCases {
		if got := ResolveQoderUpstreamModelKey(tc); got != "dfmodel" {
			t.Errorf("ResolveQoderUpstreamModelKey(%q) = %q, want 'dfmodel'", tc, got)
		}
	}

	// Qwen max aliases -> qmodel_38max
	qwenMaxCases := []string{
		"qmodel_38max",
		"qwen3.8-max",
		"qwen-3.8-max",
		"qwen-max",
		"qwen-max-latest",
		"qoder/qwen3.8-max-free",
	}
	for _, tc := range qwenMaxCases {
		if got := ResolveQoderUpstreamModelKey(tc); got != "qmodel_38max" {
			t.Errorf("ResolveQoderUpstreamModelKey(%q) = %q, want 'qmodel_38max'", tc, got)
		}
	}

	// Direct passthrough
	if got := ResolveQoderUpstreamModelKey("auto"); got != "auto" {
		t.Errorf("ResolveQoderUpstreamModelKey('auto') = %q, want 'auto'", got)
	}
	if got := ResolveQoderUpstreamModelKey("custom-unmapped-model"); got != "custom-unmapped-model" {
		t.Errorf("ResolveQoderUpstreamModelKey('custom-unmapped-model') = %q, want 'custom-unmapped-model'", got)
	}
}

func TestQoderBaselineIncludesDeepSeekAndQwen(t *testing.T) {
	ClearQoderCatalogModels("")

	models := GetQoderModels("intl")
	byID := make(map[string]*ModelInfo)
	for _, m := range models {
		if m != nil {
			byID[strings.ToLower(m.ID)] = m
		}
	}

	required := []string{
		"deepseek-flash", "deepseek-chat", "deepseek-v3", "dfmodel",
		"deepseek-pro", "deepseek-r1", "dpmodel",
		"qwen3.8-max", "qmodel_38max", "qwen3.8-plus", "qmodel_38plus",
		"qwen3.8-flash", "qmodel_38flash",
		"auto", "ultimate",
	}

	for _, reqID := range required {
		if _, ok := byID[reqID]; !ok {
			t.Errorf("expected baseline model %q to be present in GetQoderModels(intl)", reqID)
		}
	}

	// Verify LookupStaticModelProviders finds qoder for deepseek-flash
	providers := LookupStaticModelProviders("deepseek-flash")
	foundQoder := false
	for _, p := range providers {
		if p == "qoder" {
			foundQoder = true
			break
		}
	}
	if !foundQoder {
		t.Errorf("expected LookupStaticModelProviders('deepseek-flash') to include 'qoder', got %v", providers)
	}

	// Test TrimQoderPrefixes
	if got := TrimQoderPrefixes("qoder/deepseek-flash"); got != "deepseek-flash" {
		t.Errorf("TrimQoderPrefixes('qoder/deepseek-flash') = %q, want 'deepseek-flash'", got)
	}
	if got := TrimQoderPrefixes("qoder-cn/deepseek-chat"); got != "deepseek-chat" {
		t.Errorf("TrimQoderPrefixes('qoder-cn/deepseek-chat') = %q, want 'deepseek-chat'", got)
	}
	if got := TrimQoderPrefixes("qoder-intl/dfmodel"); got != "dfmodel" {
		t.Errorf("TrimQoderPrefixes('qoder-intl/dfmodel') = %q, want 'dfmodel'", got)
	}

	// Test ResolveQoderUpstreamModelKey
	if got := ResolveQoderUpstreamModelKey("qoder/deepseek-flash"); got != "dfmodel" {
		t.Errorf("ResolveQoderUpstreamModelKey('qoder/deepseek-flash') = %q, want 'dfmodel'", got)
	}
	if got := ResolveQoderUpstreamModelKey("deepseek-flash"); got != "dfmodel" {
		t.Errorf("ResolveQoderUpstreamModelKey('deepseek-flash') = %q, want 'dfmodel'", got)
	}
	if got := ResolveQoderUpstreamModelKey("qoder/qwen3.8-max"); got != "qmodel_38max" {
		t.Errorf("ResolveQoderUpstreamModelKey('qoder/qwen3.8-max') = %q, want 'qmodel_38max'", got)
	}
}
