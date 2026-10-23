package registry

import (
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
