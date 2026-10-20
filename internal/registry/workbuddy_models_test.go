package registry

import (
	"testing"
)

func TestGetWorkBuddyModels(t *testing.T) {
	intlModels := GetWorkBuddyModels("intl")
	if len(intlModels) == 0 {
		t.Fatal("GetWorkBuddyModels(intl) returned empty list")
	}

	foundDeepSeek := false
	foundGPT6 := false
	for _, m := range intlModels {
		if m.ID == "deepseek-v4.1-flash" {
			foundDeepSeek = true
		}
		if m.ID == "gpt-6-astra" {
			foundGPT6 = true
		}
	}
	if !foundDeepSeek {
		t.Error("deepseek-v4.1-flash not found in intl models")
	}
	if !foundGPT6 {
		t.Error("gpt-6-astra not found in intl models")
	}

	cnModels := GetWorkBuddyModels("cn")
	if len(cnModels) == 0 {
		t.Fatal("GetWorkBuddyModels(cn) returned empty list")
	}

	foundHy4 := false
	for _, m := range cnModels {
		if m.ID == "hy4-preview-f" {
			foundHy4 = true
		}
	}
	if !foundHy4 {
		t.Error("hy4-preview-f not found in cn models")
	}
}

func TestLookupWorkBuddyModel(t *testing.T) {
	m := LookupWorkBuddyModel("deepseek-v4.1-flash", "intl")
	if m == nil {
		t.Fatal("LookupWorkBuddyModel failed for deepseek-v4.1-flash")
	}
	if m.OwnedBy != "deepseek" {
		t.Errorf("OwnedBy = %q, want deepseek", m.OwnedBy)
	}

	mCN := LookupWorkBuddyModel("hy4-preview-f", "cn")
	if mCN == nil {
		t.Fatal("LookupWorkBuddyModel failed for hy4-preview-f")
	}
	if mCN.OwnedBy != "tencent" {
		t.Errorf("OwnedBy = %q, want tencent", mCN.OwnedBy)
	}

	mNonExistent := LookupWorkBuddyModel("non-existent-model", "intl")
	if mNonExistent != nil {
		t.Errorf("LookupWorkBuddyModel returned non-nil for non-existent model: %v", mNonExistent)
	}

	// Verify base models are labeled with realm names, and -free aliases with (Free / 1M)
	mFlashIntl := LookupWorkBuddyModel("deepseek-v4.1-flash", "intl")
	if mFlashIntl == nil || mFlashIntl.DisplayName != "DeepSeek V4.1 Flash (WorkBuddy)" {
		t.Errorf("deepseek-v4.1-flash (intl) DisplayName = %q, want %q", mFlashIntl.DisplayName, "DeepSeek V4.1 Flash (WorkBuddy)")
	}
	mFlashIntlFree := LookupWorkBuddyModel("deepseek-v4.1-flash-free", "intl")
	if mFlashIntlFree == nil || mFlashIntlFree.DisplayName != "DeepSeek V4.1 Flash (Free / 1M)" {
		t.Errorf("deepseek-v4.1-flash-free (intl) DisplayName = %q, want %q", mFlashIntlFree.DisplayName, "DeepSeek V4.1 Flash (Free / 1M)")
	}

	mFlashCN := LookupWorkBuddyModel("deepseek-v4.1-flash", "cn")
	if mFlashCN == nil || mFlashCN.DisplayName != "DeepSeek V4.1 Flash (CodeBuddy)" {
		t.Errorf("deepseek-v4.1-flash (cn) DisplayName = %q, want %q", mFlashCN.DisplayName, "DeepSeek V4.1 Flash (CodeBuddy)")
	}
	// CodeBuddy CN has no free models / free tier; -free lookups must return nil
	if m := LookupWorkBuddyModel("deepseek-v4.1-flash-free", "cn"); m != nil {
		t.Errorf("deepseek-v4.1-flash-free (cn) unexpectedly returned non-nil: %v", m)
	}

	mHy4CN := LookupWorkBuddyModel("hy4-preview-f", "cn")
	if mHy4CN == nil || mHy4CN.DisplayName != "Hunyuan 4 Preview F (CodeBuddy)" {
		t.Errorf("hy4-preview-f (cn) DisplayName = %q, want %q", mHy4CN.DisplayName, "Hunyuan 4 Preview F (CodeBuddy)")
	}
	if m := LookupWorkBuddyModel("hy4-preview-f-free", "cn"); m != nil {
		t.Errorf("hy4-preview-f-free (cn) unexpectedly returned non-nil: %v", m)
	}

	// Verify GetWorkBuddyModels("cn") does not contain any -free models
	for _, m := range GetWorkBuddyModels("cn") {
		if m != nil && (m.ID == "deepseek-v4.1-flash-free" || m.ID == "hy4-preview-f-free") {
			t.Errorf("GetWorkBuddyModels(cn) unexpectedly contains %s", m.ID)
		}
	}
}

func TestDynamicWorkBuddyModels(t *testing.T) {
	ClearWorkBuddyCatalogModels("")
	defer ClearWorkBuddyCatalogModels("")

	dynamicIntl := []*ModelInfo{
		{
			ID:          "gpt-5.5",
			Object:      "model",
			OwnedBy:     "openai",
			Type:        "workbuddy",
			DisplayName: "GPT-5.5 (WorkBuddy)",
		},
		{
			// Model that duplicates a static model ID: static definition should take precedence
			ID:          "deepseek-v4.1-flash",
			Object:      "model",
			OwnedBy:     "deepseek",
			Type:        "workbuddy",
			DisplayName: "Overridden Display Name",
		},
	}

	changed := SetWorkBuddyCatalogModels("intl", dynamicIntl)
	if !changed {
		t.Fatal("SetWorkBuddyCatalogModels(intl) should have returned true on initial insert")
	}

	// Repeated set with identical models should report no change
	if SetWorkBuddyCatalogModels("intl", dynamicIntl) {
		t.Error("SetWorkBuddyCatalogModels(intl) should have returned false for identical models")
	}

	stored := GetWorkBuddyDynamicModels("intl")
	if len(stored) != 2 {
		t.Fatalf("GetWorkBuddyDynamicModels(intl) len = %d, want 2", len(stored))
	}

	models := GetWorkBuddyModels("intl")
	var foundGPT55, foundDeepSeek bool
	for _, m := range models {
		if m.ID == "gpt-5.5" {
			foundGPT55 = true
			if m.DisplayName != "GPT-5.5 (WorkBuddy)" {
				t.Errorf("gpt-5.5 DisplayName = %q, want %q", m.DisplayName, "GPT-5.5 (WorkBuddy)")
			}
		}
		if m.ID == "deepseek-v4.1-flash" {
			foundDeepSeek = true
			if m.DisplayName != "DeepSeek V4.1 Flash (WorkBuddy)" {
				t.Errorf("static deepseek-v4.1-flash was overwritten by dynamic model: %q", m.DisplayName)
			}
		}
	}
	if !foundGPT55 {
		t.Error("dynamic model gpt-5.5 not found in GetWorkBuddyModels(intl)")
	}
	if !foundDeepSeek {
		t.Error("static model deepseek-v4.1-flash not found in GetWorkBuddyModels(intl)")
	}

	// LookupWorkBuddyModel finds dynamic model
	mLookup := LookupWorkBuddyModel("gpt-5.5", "intl")
	if mLookup == nil {
		t.Fatal("LookupWorkBuddyModel failed for dynamic model gpt-5.5")
	}
	if mLookup.OwnedBy != "openai" {
		t.Errorf("gpt-5.5 OwnedBy = %q, want openai", mLookup.OwnedBy)
	}

	// GetAllWorkBuddyModels includes dynamic models
	all := GetAllWorkBuddyModels()
	foundInAll := false
	for _, m := range all {
		if m.ID == "gpt-5.5" {
			foundInAll = true
			break
		}
	}
	if !foundInAll {
		t.Error("dynamic model gpt-5.5 not found in GetAllWorkBuddyModels()")
	}

	// Clear dynamic models
	ClearWorkBuddyCatalogModels("intl")
	if len(GetWorkBuddyDynamicModels("intl")) != 0 {
		t.Error("ClearWorkBuddyCatalogModels(intl) failed to clear models")
	}
	if m := LookupWorkBuddyModel("gpt-5.5", "intl"); m != nil {
		t.Errorf("gpt-5.5 still found after clearing dynamic models: %v", m)
	}
}
