package util_test

import (
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
)

// TestMaskSensitiveQueryMasksValue covers the related half of issue #6666:
// deleting a client API key by value passes the secret as ?value=, so the
// request logger must mask it like other secret-bearing parameters.
func TestMaskSensitiveQueryMasksValue(t *testing.T) {
	secretKey := "sk-secret-client-key"

	t.Run("value parameter is masked", func(t *testing.T) {
		masked := util.MaskSensitiveQuery("value=" + secretKey)
		if strings.Contains(masked, secretKey) {
			t.Fatalf("value parameter leaked in full: %s", masked)
		}
		if masked != "value=sk-s...-key" {
			t.Fatalf("masked=%q want the HideAPIKey form", masked)
		}
	})

	t.Run("index parameter is untouched", func(t *testing.T) {
		masked := util.MaskSensitiveQuery("index=3")
		if masked != "index=3" {
			t.Fatalf("masked=%q want %q", masked, "index=3")
		}
	})

	t.Run("known secret parameters stay masked", func(t *testing.T) {
		masked := util.MaskSensitiveQuery("key=" + secretKey + "&access_token=" + secretKey)
		if strings.Contains(masked, secretKey) {
			t.Fatalf("secret parameter leaked in full: %s", masked)
		}
	})
}

func TestGetProviderName_StaticFallback(t *testing.T) {
	testCases := []struct {
		model        string
		wantProvider string
	}{
		{
			model:        "step-5-preview-free",
			wantProvider: "opencode",
		},
		{
			model:        "gemini-3.8-flash-high",
			wantProvider: "antigravity",
		},
		{
			model:        "cat-jam-free",
			wantProvider: "opencode",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.model, func(t *testing.T) {
			providers := util.GetProviderName(tc.model)
			if len(providers) == 0 {
				t.Fatalf("GetProviderName(%q) returned empty providers, want at least %q", tc.model, tc.wantProvider)
			}
			found := false
			for _, p := range providers {
				if p == tc.wantProvider {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("GetProviderName(%q) = %v, want to include %q", tc.model, providers, tc.wantProvider)
			}
		})
	}
}
