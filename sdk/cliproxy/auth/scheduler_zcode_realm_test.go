package auth

import (
	"context"
	"testing"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

// TestSchedulerPick_ZCodeRealmProviderKeys guards the regression where a
// BigModel (`zcode-cn`) credential registered its scheduler shard under the
// executor key `zcode` while requests asked for provider `zcode-cn`, so every
// ZCode model answered auth_not_found even though the credential was active.
func TestSchedulerPick_ZCodeRealmProviderKeys(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		authProv  string
		reqProv   string
		model     string
		registryP string
	}{
		{name: "cn realm", authProv: "zcode-cn", reqProv: "zcode-cn", model: "GLM-5.3-Flash", registryP: "zcode-cn"},
		{name: "zai realm", authProv: "zcode-zai", reqProv: "zcode-zai", model: "GLM-5.3", registryP: "zcode-zai"},
		{name: "plain realm", authProv: "zcode", reqProv: "zcode", model: "GLM-5.3", registryP: "zcode"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			registerSchedulerModels(t, tc.registryP, tc.model, "zcode-auth-"+tc.authProv)
			scheduler := newSchedulerForTest(
				&RoundRobinSelector{},
				&Auth{ID: "zcode-auth-" + tc.authProv, Provider: tc.authProv},
			)

			got, errPick := scheduler.pickSingle(context.Background(), tc.reqProv, tc.model, cliproxyexecutor.Options{}, nil)
			if errPick != nil {
				t.Fatalf("pickSingle(%q, %q) error = %v", tc.reqProv, tc.model, errPick)
			}
			if got == nil {
				t.Fatalf("pickSingle(%q, %q) returned no auth", tc.reqProv, tc.model)
			}
			if got.Provider != tc.authProv {
				t.Fatalf("picked provider %q, want %q", got.Provider, tc.authProv)
			}
		})
	}
}
