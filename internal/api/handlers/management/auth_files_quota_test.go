package management

import (
	"testing"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestModelQuotaObservationPayloadOmitsUnsupportedProviders(t *testing.T) {
	states := map[string]*coreauth.ModelState{
		"grok-4": {
			Quota: coreauth.QuotaState{
				ObservedAt: time.Unix(10, 0),
				Signals:    map[string]string{"X-Ratelimit-Remaining-Requests": "1"},
			},
		},
	}
	if got := modelQuotaObservationPayload("grok", states); len(got) != 0 {
		t.Fatalf("unsupported provider returned model observations: %#v", got)
	}
	for _, provider := range []string{"gemini", "gemini-interactions", "openai", "openai-compatibility", "plugin-provider"} {
		if got := modelQuotaObservationPayload(provider, states); len(got) != 0 {
			t.Fatalf("provider %q returned model observations: %#v", provider, got)
		}
	}
}

func TestModelQuotaObservationPayloadSkipsNilAndEmptyStates(t *testing.T) {
	states := map[string]*coreauth.ModelState{
		"nil":   nil,
		"empty": &coreauth.ModelState{},
		"observed": &coreauth.ModelState{Quota: coreauth.QuotaState{
			ObservedAt: time.Unix(10, 0),
			Signals:    map[string]string{"X-Codex-Plan-Type": "pro"},
		}},
	}
	got := modelQuotaObservationPayload("codex", states)
	if len(got) != 1 {
		t.Fatalf("model observations = %#v, want only observed state", got)
	}
	if _, ok := got["observed"]; !ok {
		t.Fatalf("observed model quota missing: %#v", got)
	}
}

func TestQuotaObservationPayloadExcludesCooldownState(t *testing.T) {
	payload := quotaObservationPayload(coreauth.QuotaState{
		Exceeded:      true,
		Reason:        "credential_quota",
		NextRecoverAt: time.Unix(20, 0),
		BackoffLevel:  3,
		ObservedAt:    time.Unix(10, 0),
		Signals:       map[string]string{"X-Codex-Plan-Type": "pro"},
	})
	if _, ok := payload["exceeded"]; ok {
		t.Fatalf("cooldown exceeded leaked: %#v", payload)
	}
	if _, ok := payload["reason"]; ok {
		t.Fatalf("cooldown reason leaked: %#v", payload)
	}
	if _, ok := payload["next_recover_at"]; ok {
		t.Fatalf("cooldown recovery leaked: %#v", payload)
	}
	if _, ok := payload["backoff_level"]; ok {
		t.Fatalf("cooldown backoff leaked: %#v", payload)
	}
}

func TestBuildAuthFileEntryLockedSupportsQuotaForWorkbuddyAndQoder(t *testing.T) {
	h := &Handler{}
	quotaCtx := authFileQuotaContext{
		v8: true,
	}

	testCases := []struct {
		provider     string
		wantProvider string
		wantPluginID string
	}{
		{provider: "workbuddy", wantProvider: "workbuddy", wantPluginID: "workbuddy"},
		{provider: "workbuddy-cn", wantProvider: "workbuddy", wantPluginID: "workbuddy"},
		{provider: "copilot-tencent", wantProvider: "workbuddy", wantPluginID: "workbuddy"},
		{provider: "qoder", wantProvider: "qoder", wantPluginID: "qoder"},
		{provider: "qoder-cn", wantProvider: "qoder", wantPluginID: "qoder"},
	}

	for _, tc := range testCases {
		auth := &coreauth.Auth{
			ID:       tc.provider + "-test",
			Provider: tc.provider,
			Attributes: map[string]string{
				"path": "test/" + tc.provider + ".json",
			},
		}
		entry := h.buildAuthFileEntryLocked(auth, quotaCtx)
		if entry == nil {
			t.Fatalf("entry is nil for provider %s", tc.provider)
		}
		if entry["supports_quota"] != true {
			t.Fatalf("supports_quota is not true for provider %s: %#v", tc.provider, entry)
		}
		if entry["quota_provider"] != tc.wantProvider {
			t.Fatalf("quota_provider = %v, want %v for %s", entry["quota_provider"], tc.wantProvider, tc.provider)
		}
		if entry["quota_plugin_id"] != tc.wantPluginID {
			t.Fatalf("quota_plugin_id = %v, want %v for %s", entry["quota_plugin_id"], tc.wantPluginID, tc.provider)
		}
	}
}
