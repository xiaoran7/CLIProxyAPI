package registry

import (
	"strings"
	"testing"
)

func TestGetZCodeModels(t *testing.T) {
	models := GetZCodeModels()
	if len(models) == 0 {
		t.Fatal("the ZCode catalog is empty")
	}

	byID := make(map[string]*ModelInfo, len(models))
	for _, model := range models {
		if model == nil {
			t.Fatal("the catalog holds a nil model")
		}
		if model.Type != "zcode" {
			t.Errorf("%s has type %q, want zcode", model.ID, model.Type)
		}
		if model.ContextLength <= 0 {
			t.Errorf("%s has no context length", model.ID)
		}
		if model.Thinking == nil || len(model.Thinking.Levels) == 0 {
			t.Errorf("%s declares no reasoning levels", model.ID)
		}
		if _, dup := byID[model.ID]; dup {
			t.Errorf("%s appears twice", model.ID)
		}
		byID[model.ID] = model
	}

	for _, required := range []string{"GLM-5.3", "GLM-5.3-Flash", "GLM-5.2", "GLM-5-Turbo"} {
		if byID[required] == nil {
			t.Errorf("%s is missing from the catalog", required)
		}
	}
	// A trial entry spends the gift; each one must name a model the plans serve.
	trial := 0
	for id := range byID {
		if !strings.HasSuffix(id, "-Trial") {
			continue
		}
		trial++
		base := strings.TrimSuffix(id, "-Trial")
		if byID[base] == nil {
			t.Errorf("%s has no plain counterpart %s", id, base)
		}
	}
	if trial == 0 {
		t.Error("the catalog lists no trial entries, so the Start Plan cannot be picked")
	}
}

func TestGetZCodeCodingPlanModels_ExcludesTrial(t *testing.T) {
	models := GetZCodeCodingPlanModels()
	if len(models) == 0 {
		t.Fatal("the coding plan catalog is empty")
	}
	for _, model := range models {
		if strings.HasSuffix(model.ID, "-Trial") {
			t.Errorf("%s spends the gift but is listed for a coding plan", model.ID)
		}
	}
	if len(GetZCodeModels()) <= len(models) {
		t.Error("the full catalog should add the trial entries")
	}
}

func TestGetZCodeModels_ReturnsCopies(t *testing.T) {
	first := GetZCodeModels()
	first[0].ID = "mutated"
	if got := GetZCodeModels()[0].ID; got == "mutated" {
		t.Error("the catalog handed out its own model pointers")
	}
}

func TestStaticModelProviders_IncludesZCode(t *testing.T) {
	providers := LookupStaticModelProviders("GLM-5.3")
	found := false
	for _, provider := range providers {
		if provider == "zcode" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("GLM-5.3 is not attributed to zcode: %v", providers)
	}
	if models := GetStaticModelDefinitionsByChannel("zcode"); len(models) == 0 {
		t.Error("the zcode channel has no static definitions")
	}
}
