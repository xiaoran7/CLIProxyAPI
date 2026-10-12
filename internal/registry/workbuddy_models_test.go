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
}
