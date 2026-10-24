package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
)

func TestStartOAuthV8_Validation(t *testing.T) {
	t.Parallel()

	h := &Handler{
		cfg: &config.Config{},
	}

	// 1. Missing provider returns 400
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v8/management/oauth/auth-url", nil)

	h.StartOAuthV8(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty provider, got %d", rec.Code)
	}

	// 2. Unknown provider returns 404
	recUnknown := httptest.NewRecorder()
	cUnknown, _ := gin.CreateTestContext(recUnknown)
	cUnknown.Request = httptest.NewRequest(http.MethodGet, "/v8/management/oauth/auth-url?provider=nonexistent_unknown_provider", nil)

	h.StartOAuthV8(cUnknown)

	if recUnknown.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown provider, got %d", recUnknown.Code)
	}
}

func TestListPlugins_DualZCodeAndBigModelLogos(t *testing.T) {
	t.Parallel()

	h := &Handler{
		cfg: &config.Config{
			Plugins: config.PluginsConfig{
				Enabled: false,
			},
		},
		configFilePath: writeTestConfigFile(t),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/plugins", nil)

	h.ListPlugins(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Plugins []struct {
			ID            string `json:"id"`
			SupportsOAuth bool   `json:"supports_oauth"`
			OAuthProvider string `json:"oauth_provider"`
			Logo          string `json:"logo"`
			Metadata      *struct {
				Name string `json:"name"`
				Logo string `json:"logo"`
			} `json:"metadata"`
		} `json:"plugins"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	var foundZCode, foundZCodeCN bool
	for _, p := range resp.Plugins {
		if p.ID == "zcode" {
			foundZCode = true
			if !p.SupportsOAuth || p.OAuthProvider != "zcode" {
				t.Errorf("zcode OAuthProvider mismatch: got %q", p.OAuthProvider)
			}
			if !strings.HasPrefix(p.Logo, "data:image/png;base64,") {
				t.Errorf("zcode logo must be base64 PNG data URI, got %q", p.Logo)
			}
			if p.Metadata == nil || !strings.Contains(p.Metadata.Name, "ZCode") {
				t.Errorf("zcode metadata name mismatch: %#v", p.Metadata)
			}
		}
		if p.ID == "zcode-cn" {
			foundZCodeCN = true
			if !p.SupportsOAuth || p.OAuthProvider != "zcode-cn" {
				t.Errorf("zcode-cn OAuthProvider mismatch: got %q", p.OAuthProvider)
			}
			if !strings.HasPrefix(p.Logo, "data:image/png;base64,") {
				t.Errorf("zcode-cn logo must be base64 PNG data URI, got %q", p.Logo)
			}
			if p.Metadata == nil || !strings.Contains(p.Metadata.Name, "BigModel") {
				t.Errorf("zcode-cn metadata name mismatch: %#v", p.Metadata)
			}
		}
	}

	if !foundZCode {
		t.Error("expected zcode plugin in response")
	}
	if !foundZCodeCN {
		t.Error("expected zcode-cn plugin in response")
	}
}
