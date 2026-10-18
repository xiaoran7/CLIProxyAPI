package cliproxy

import (
	"context"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/pluginhost"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const (
	pluginStaticGlobalModel     = "plugin-static-global"
	pluginStaticCredentialModel = "plugin-static-credential"
)

type pluginStaticModelRegistrar struct {
	provider string
}

func (r pluginStaticModelRegistrar) RegisterModels(context.Context, pluginapi.ModelRegistrationRequest) (pluginapi.ModelRegistrationResponse, error) {
	return pluginapi.ModelRegistrationResponse{
		Provider: r.provider,
		Models:   []pluginapi.ModelInfo{{ID: pluginStaticGlobalModel}, {ID: pluginStaticCredentialModel}},
	}, nil
}

// newPluginStaticModelHost loads a static-only plugin so the provider's models
// are appended to native registrations instead of handled per auth.
func newPluginStaticModelHost(t *testing.T, provider string) *pluginhost.Host {
	t.Helper()
	host := pluginhost.New()
	pluginID := t.Name()
	host.RegisterPluginForTest(pluginID, pluginapi.Plugin{Capabilities: pluginapi.Capabilities{ModelRegistrar: pluginStaticModelRegistrar{provider: provider}}})
	host.RegisterModels(t.Context(), registry.GetGlobalRegistry())
	host.RegisterExecutors(coreauth.NewManager(nil, nil, nil), registry.GetGlobalRegistry())
	t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient("plugin:" + pluginID + ":" + provider) })
	if got := len(host.ModelsForProvider(provider)); got != 2 {
		t.Fatalf("plugin static models for %s = %d, want 2", provider, got)
	}
	return host
}

func TestRegisterModelsForAuth_PluginStaticModelsFollowExclusions(t *testing.T) {
	const provider = "kimi"
	cfg := &config.Config{OAuthExcludedModels: map[string][]string{provider: {pluginStaticGlobalModel}}}
	service := &Service{cfg: cfg, pluginHost: newPluginStaticModelHost(t, provider)}
	reg := GlobalModelRegistry()

	for _, tc := range []struct {
		name          string
		ownRules      string
		wantGlobal    bool
		wantCredValue bool
	}{
		{name: "global rules", wantGlobal: false, wantCredValue: true},
		{name: "credential rules override global", ownRules: pluginStaticCredentialModel, wantGlobal: true, wantCredValue: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := &coreauth.Auth{ID: t.Name(), Provider: provider, Status: coreauth.StatusActive, Attributes: map[string]string{"auth_kind": "oauth"}}
			if tc.ownRules != "" {
				auth.Attributes["excluded_models"] = tc.ownRules
			}
			reg.UnregisterClient(auth.ID)
			t.Cleanup(func() { reg.UnregisterClient(auth.ID) })

			service.registerModelsForAuth(t.Context(), auth)

			present := make(map[string]bool)
			for _, model := range reg.GetModelsForClient(auth.ID) {
				present[model.ID] = true
			}
			if present[pluginStaticGlobalModel] != tc.wantGlobal || present[pluginStaticCredentialModel] != tc.wantCredValue {
				t.Fatalf("plugin static models = (%t,%t), want (%t,%t); models = %v", present[pluginStaticGlobalModel], present[pluginStaticCredentialModel], tc.wantGlobal, tc.wantCredValue, present)
			}
		})
	}
}

func TestRegisterModelsForAuth_PluginStaticExclusionsDoNotMatchNativeAliases(t *testing.T) {
	const provider = "kimi"
	const nativeAlias = "native-alias"
	catalog := registry.GetKimiModels()
	if len(catalog) == 0 {
		t.Fatal("kimi catalog is empty")
	}
	cfg := &config.Config{
		OAuthExcludedModels: map[string][]string{provider: {pluginStaticGlobalModel, nativeAlias}},
		OAuthModelAlias:     map[string][]config.OAuthModelAlias{provider: {{Name: catalog[0].ID, Alias: nativeAlias}}},
	}
	service := &Service{cfg: cfg, pluginHost: newPluginStaticModelHost(t, provider)}
	auth := &coreauth.Auth{ID: t.Name(), Provider: provider, Status: coreauth.StatusActive, Attributes: map[string]string{"auth_kind": "oauth"}}
	reg := GlobalModelRegistry()
	reg.UnregisterClient(auth.ID)
	t.Cleanup(func() { reg.UnregisterClient(auth.ID) })

	service.registerModelsForAuth(t.Context(), auth)

	// Exclusions match original model IDs, so an alias named like a rule stays visible.
	if !reg.ClientSupportsModel(auth.ID, nativeAlias) || reg.ClientSupportsModel(auth.ID, pluginStaticGlobalModel) {
		t.Fatalf("models = (%s:%t, %s:%t), want native alias kept and plugin model excluded", nativeAlias, reg.ClientSupportsModel(auth.ID, nativeAlias), pluginStaticGlobalModel, reg.ClientSupportsModel(auth.ID, pluginStaticGlobalModel))
	}
}

func TestAntigravityModelsForHints_PluginStaticModelsFollowExclusions(t *testing.T) {
	const provider = "antigravity"
	cfg := &config.Config{OAuthExcludedModels: map[string][]string{provider: {pluginStaticGlobalModel}}}
	service := &Service{cfg: cfg, pluginHost: newPluginStaticModelHost(t, provider)}
	hints := antigravityModelCapabilityHints{ModelIDs: map[string]struct{}{"gemini-3.1-flash-lite": {}}}

	for _, tc := range []struct {
		name          string
		ownRules      string
		wantGlobal    bool
		wantCredValue bool
	}{
		{name: "global rules", wantGlobal: false, wantCredValue: true},
		{name: "credential rules override global", ownRules: pluginStaticCredentialModel, wantGlobal: true, wantCredValue: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := antigravityTestAuth(t.Name(), "http://127.0.0.1:1")
			if tc.ownRules != "" {
				auth.Attributes["excluded_models"] = tc.ownRules
			}

			present := make(map[string]bool)
			for _, model := range service.antigravityModelsForHints(auth, hints) {
				present[model.ID] = true
			}

			if present[pluginStaticGlobalModel] != tc.wantGlobal || present[pluginStaticCredentialModel] != tc.wantCredValue {
				t.Fatalf("plugin static models = (%t,%t), want (%t,%t); models = %v", present[pluginStaticGlobalModel], present[pluginStaticCredentialModel], tc.wantGlobal, tc.wantCredValue, present)
			}
		})
	}
}
