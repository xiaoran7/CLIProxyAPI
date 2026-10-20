package cliproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestQoderAuthIsUsable(t *testing.T) {
	cases := []struct {
		name     string
		auth     *coreauth.Auth
		expected bool
	}{
		{"nil auth", nil, false},
		{"qoder provider", &coreauth.Auth{Provider: "qoder"}, true},
		{"qoder-intl provider", &coreauth.Auth{Provider: "qoder-intl"}, true},
		{"qoder-en provider", &coreauth.Auth{Provider: "qoder-en"}, true},
		{"qoder-cn provider", &coreauth.Auth{Provider: "qoder-cn"}, true},
		{"uppercase provider", &coreauth.Auth{Provider: "QODER-CN"}, true},
		{"other provider", &coreauth.Auth{Provider: "openai"}, false},
		{"qoder auth_kind attr", &coreauth.Auth{Provider: "custom", Attributes: map[string]string{"auth_kind": "qoder"}}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := qoderAuthIsUsable(tc.auth)
			if got != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestWarmQoderCatalogForAuth(t *testing.T) {
	registry.ClearQoderCatalogModels("")
	defer registry.ClearQoderCatalogModels("")

	upstreamJSON := `{"code":0,"data":{"models":[{"modelName":"qoder-pro-v1","label":"Qoder Pro"}]}}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(upstreamJSON))
	}))
	defer ts.Close()

	prevFactory := qoderCatalogClientFactory
	qoderCatalogClientFactory = func() *http.Client {
		return ts.Client()
	}
	defer func() {
		qoderCatalogClientFactory = prevFactory
	}()

	svc := &Service{}
	auth := &coreauth.Auth{
		ID:       "qoder-test-warm",
		Provider: "qoder",
		Attributes: map[string]string{
			"base_url": ts.URL,
		},
		Metadata: map[string]any{
			"access_token": "pt-test-12345",
			"job_token":    "jt-test-12345",
			"user_id":      "usr-1",
		},
	}

	svc.warmQoderCatalogForAuth(context.Background(), auth, "intl")

	models := registry.GetQoderModels("intl")
	if len(models) == 0 {
		t.Fatalf("expected non-empty dynamic models after warming")
	}
	found := false
	for _, m := range models {
		if m.ID == "qoder-pro-v1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected to find qoder-pro-v1 in warmed models, got %+v", models)
	}
}
