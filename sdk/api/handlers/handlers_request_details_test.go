package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
)

func TestGetRequestDetails_PreservesSuffix(t *testing.T) {
	modelRegistry := registry.GetGlobalRegistry()
	now := time.Now().Unix()

	modelRegistry.RegisterClient("test-request-details-gemini", "gemini", []*registry.ModelInfo{
		{ID: "gemini-2.5-pro", Created: now + 30},
		{ID: "gemini-2.5-flash", Created: now + 25},
	})
	modelRegistry.RegisterClient("test-request-details-openai", "openai", []*registry.ModelInfo{
		{ID: "gpt-5.2", Created: now + 20},
	})
	modelRegistry.RegisterClient("test-request-details-claude", "claude", []*registry.ModelInfo{
		{ID: "claude-sonnet-4-5", Created: now + 5},
	})

	// Ensure cleanup of all test registrations.
	clientIDs := []string{
		"test-request-details-gemini",
		"test-request-details-openai",
		"test-request-details-claude",
	}
	for _, clientID := range clientIDs {
		id := clientID
		t.Cleanup(func() {
			modelRegistry.UnregisterClient(id)
		})
	}

	handler := NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, coreauth.NewManager(nil, nil, nil))

	tests := []struct {
		name          string
		inputModel    string
		wantProviders []string
		wantModel     string
		wantErr       bool
	}{
		{
			name:          "numeric suffix preserved",
			inputModel:    "gemini-2.5-pro(8192)",
			wantProviders: []string{"gemini"},
			wantModel:     "gemini-2.5-pro(8192)",
			wantErr:       false,
		},
		{
			name:          "level suffix preserved",
			inputModel:    "gpt-5.2(high)",
			wantProviders: []string{"openai"},
			wantModel:     "gpt-5.2(high)",
			wantErr:       false,
		},
		{
			name:          "no suffix unchanged",
			inputModel:    "claude-sonnet-4-5",
			wantProviders: []string{"claude"},
			wantModel:     "claude-sonnet-4-5",
			wantErr:       false,
		},
		{
			name:          "unknown model with suffix",
			inputModel:    "unknown-model(8192)",
			wantProviders: nil,
			wantModel:     "",
			wantErr:       true,
		},
		{
			name:          "auto suffix resolved",
			inputModel:    "auto(high)",
			wantProviders: []string{"gemini"},
			wantModel:     "gemini-2.5-pro(high)",
			wantErr:       false,
		},
		{
			name:          "special suffix none preserved",
			inputModel:    "gemini-2.5-flash(none)",
			wantProviders: []string{"gemini"},
			wantModel:     "gemini-2.5-flash(none)",
			wantErr:       false,
		},
		{
			name:          "special suffix auto preserved",
			inputModel:    "claude-sonnet-4-5(auto)",
			wantProviders: []string{"claude"},
			wantModel:     "claude-sonnet-4-5(auto)",
			wantErr:       false,
		},
		{
			name:          "static opencode model resolved without dynamic registration",
			inputModel:    "step-5-preview-free",
			wantProviders: []string{"opencode"},
			wantModel:     "step-5-preview-free",
			wantErr:       false,
		},
		{
			name:          "static antigravity model resolved without dynamic registration",
			inputModel:    "gemini-3.8-flash-high",
			wantProviders: []string{"antigravity"},
			wantModel:     "gemini-3.8-flash-high",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providers, model, errMsg := handler.getRequestDetails(tt.inputModel)
			if (errMsg != nil) != tt.wantErr {
				t.Fatalf("getRequestDetails() error = %v, wantErr %v", errMsg, tt.wantErr)
			}
			if errMsg != nil {
				return
			}
			if !reflect.DeepEqual(providers, tt.wantProviders) {
				t.Fatalf("getRequestDetails() providers = %v, want %v", providers, tt.wantProviders)
			}
			if model != tt.wantModel {
				t.Fatalf("getRequestDetails() model = %v, want %v", model, tt.wantModel)
			}
		})
	}
}

// TestGetRequestDetails_UnknownModelErrorResistsJSONInjection pins the unroutable
// model error body against client-controlled model names. The name is echoed into
// the body, so formatting it into a JSON literal would let a caller corrupt the
// payload or overwrite the error code that clients branch on.
func TestGetRequestDetails_UnknownModelErrorResistsJSONInjection(t *testing.T) {
	handler := NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, coreauth.NewManager(nil, nil, nil))

	for _, model := range []string{
		"unroutable-model",
		`foo"bar`,
		`x","code":"insufficient_quota","x":"`,
		`x"}}`,
		`foo\bar`,
		"foo\nbar",
	} {
		t.Run(model, func(t *testing.T) {
			_, _, errMsg := handler.getRequestDetails(model)
			if errMsg == nil || errMsg.Error == nil {
				t.Fatal("expected an error for an unroutable model")
			}
			if errMsg.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", errMsg.StatusCode, http.StatusBadRequest)
			}
			body := errMsg.Error.Error()
			if !json.Valid([]byte(body)) {
				t.Fatalf("error body is not valid JSON: %s", body)
			}
			if got := gjson.Get(body, "error.code").String(); got != "model_not_found" {
				t.Fatalf("error code = %q, want model_not_found; the caller controlled the body: %s", got, body)
			}
			if got, want := gjson.Get(body, "error.message").String(), "unknown provider for model "+model; got != want {
				t.Fatalf("error message = %q, want %q", got, want)
			}
		})
	}
}

func TestGetRequestDetails_ImageModelReturns503(t *testing.T) {
	handler := NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, coreauth.NewManager(nil, nil, nil))

	imageOnlyModels := []string{
		"gpt-image-1.5",
		"gpt-image-2",
		"codex/gpt-image-2",
		"gpt-image-2.5-flare",
		"codex/gpt-image-2.5-flare",
		"gpt-image-2.5-sunburst",
		"codex/gpt-image-2.5-sunburst",
		"gpt-image-2.5",
		"codex/gpt-image-2.5",
		"grok-imagine-image",
		"xai/grok-imagine-image",
		"grok-imagine-image-quality",
		"xai/grok-imagine-image-quality",
		"grok-imagine-image-2.0",
		"xai/grok-imagine-image-2.0",
	}
	for _, model := range imageOnlyModels {
		t.Run(model, func(t *testing.T) {
			_, _, errMsg := handler.getRequestDetails(model)
			if errMsg == nil {
				t.Fatalf("expected error for %s, got nil", model)
			}
			if errMsg.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("unexpected status code: got %d want %d", errMsg.StatusCode, http.StatusServiceUnavailable)
			}
			if errMsg.Error == nil {
				t.Fatalf("expected error message, got nil")
			}
			msg := errMsg.Error.Error()
			if !strings.Contains(msg, "/v1/images/generations") || !strings.Contains(msg, "/v1/images/edits") {
				t.Fatalf("unexpected error message: %q", msg)
			}
		})
	}
}

func TestValidateImageOnlyModel_AllowsImageEndpoints(t *testing.T) {
	handler := NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, coreauth.NewManager(nil, nil, nil))

	imageOnlyModels := []string{
		"gpt-image-1.5",
		"gpt-image-2",
		"codex/gpt-image-2",
		"gpt-image-2.5-flare",
		"codex/gpt-image-2.5-flare",
		"gpt-image-2.5-sunburst",
		"codex/gpt-image-2.5-sunburst",
		"gpt-image-2.5",
		"codex/gpt-image-2.5",
		"grok-imagine-image",
		"xai/grok-imagine-image",
		"grok-imagine-image-quality",
		"xai/grok-imagine-image-quality",
		"grok-imagine-image-2.0",
		"xai/grok-imagine-image-2.0",
	}
	for _, model := range imageOnlyModels {
		t.Run(model, func(t *testing.T) {
			if errMsg := handler.validateImageOnlyModel(model, true); errMsg != nil {
				t.Fatalf("validateImageOnlyModel(%q, true) = %+v, want nil", model, errMsg)
			}
			if errMsg := handler.validateImageOnlyModel(model, false); errMsg == nil {
				t.Fatalf("validateImageOnlyModel(%q, false) = nil, want image-only error", model)
			} else if errMsg.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("unexpected status code: got %d want %d", errMsg.StatusCode, http.StatusServiceUnavailable)
			}
		})
	}
}

func TestIsOpenAIImageOnlyModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{model: "gpt-image-1.5", want: true},
		{model: "gpt-image-2", want: true},
		{model: "codex/gpt-image-1.5", want: true},
		{model: "gpt-image-2.5-flare", want: true},
		{model: "codex/gpt-image-2.5-flare", want: true},
		{model: "gpt-image-2.5-sunburst", want: true},
		{model: "codex/gpt-image-2.5-sunburst", want: true},
		{model: "gpt-image-2.5", want: true},
		{model: "codex/gpt-image-2.5", want: true},
		{model: "grok-imagine-image", want: true},
		{model: "xai/grok-imagine-image", want: true},
		{model: "XAI/Grok-Imagine-Image-Quality", want: true},
		{model: "grok-imagine-image-quality", want: true},
		{model: "grok-imagine-image-2.0", want: true},
		{model: "xai/grok-imagine-image-2.0", want: true},
		{model: "grok-3", want: false},
		{model: "gpt-5.2", want: false},
		{model: "grok-imagine-video", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := isOpenAIImageOnlyModel(tt.model); got != tt.want {
				t.Fatalf("isOpenAIImageOnlyModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestExecuteImageWithAuthManager_AllowsImageOnlyModels(t *testing.T) {
	handler := NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, coreauth.NewManager(nil, nil, nil))

	imageOnlyModels := []string{
		"gpt-image-1.5",
		"gpt-image-2",
		"gpt-image-2.5-flare",
		"gpt-image-2.5-sunburst",
		"gpt-image-2.5",
		"grok-imagine-image",
		"grok-imagine-image-quality",
		"xai/grok-imagine-image-quality",
		"grok-imagine-image-2.0",
		"xai/grok-imagine-image-2.0",
	}
	for _, model := range imageOnlyModels {
		t.Run(model, func(t *testing.T) {
			body := []byte(`{"model":"` + model + `","prompt":"draw"}`)
			_, _, errMsg := handler.ExecuteImageWithAuthManager(context.Background(), "openai-image", model, body, "")
			if errMsg == nil {
				t.Fatal("expected auth selection error, got nil")
			}
			if errMsg.Error == nil {
				t.Fatal("expected error message, got nil")
			}
			msg := errMsg.Error.Error()
			if strings.Contains(msg, "only supported on /v1/images/generations") {
				t.Fatalf("ExecuteImageWithAuthManager rejected image-only model: %q", msg)
			}

			_, _, errMsg = handler.ExecuteWithAuthManager(context.Background(), "openai-image", model, body, "")
			if errMsg == nil {
				t.Fatal("expected image-only rejection for non-image execution path, got nil")
			}
			if errMsg.Error == nil || !strings.Contains(errMsg.Error.Error(), "only supported on /v1/images/generations") {
				t.Fatalf("unexpected non-image execution error: %+v", errMsg)
			}
		})
	}
}

func TestGetRequestDetails_QoderSovereignAndBrandAliasRouting(t *testing.T) {
	modelRegistry := registry.GetGlobalRegistry()
	clientID := "test-qoder-cn-auth-client"

	modelRegistry.RegisterClient(clientID, "qoder-cn", []*registry.ModelInfo{
		{ID: "deepseek-flash", DisplayName: "DeepSeek Flash"},
	})
	t.Cleanup(func() {
		modelRegistry.UnregisterClient(clientID)
	})

	handler := NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, coreauth.NewManager(nil, nil, nil))

	// 1. Explicit qoder-cn prefix -> routes strictly to qoder-cn
	providers1, m1, err1 := handler.getRequestDetails("qoder-cn/deepseek-flash")
	if err1 != nil {
		t.Fatalf("expected no error for qoder-cn/deepseek-flash, got: %v", err1)
	}
	if !reflect.DeepEqual(providers1, []string{"qoder-cn"}) || m1 != "deepseek-flash" {
		t.Errorf("qoder-cn/deepseek-flash -> providers=%v, model=%q; want [qoder-cn], deepseek-flash", providers1, m1)
	}

	// 2. Generic brand qoder/ prefix -> converges to qoder-cn because only qoder-cn is registered
	providers2, m2, err2 := handler.getRequestDetails("qoder/deepseek-flash")
	if err2 != nil {
		t.Fatalf("expected no error for qoder/deepseek-flash brand convergence, got: %v", err2)
	}
	if !reflect.DeepEqual(providers2, []string{"qoder-cn"}) || m2 != "deepseek-flash" {
		t.Errorf("qoder/deepseek-flash -> providers=%v, model=%q; want [qoder-cn], deepseek-flash", providers2, m2)
	}

	// 3. Explicit qoder-intl prefix -> strictly routes to qoder (does NOT converge to qoder-cn)
	providers3, m3, err3 := handler.getRequestDetails("qoder-intl/deepseek-flash")
	if err3 != nil {
		t.Fatalf("expected no error for qoder-intl/deepseek-flash, got: %v", err3)
	}
	if !reflect.DeepEqual(providers3, []string{"qoder"}) || m3 != "deepseek-flash" {
		t.Errorf("qoder-intl/deepseek-flash -> providers=%v, model=%q; want [qoder], deepseek-flash", providers3, m3)
	}

	// 4. Bare deepseek-flash -> routes to qoder-cn
	providers4, m4, err4 := handler.getRequestDetails("deepseek-flash")
	if err4 != nil {
		t.Fatalf("expected no error for bare deepseek-flash, got: %v", err4)
	}
	if !reflect.DeepEqual(providers4, []string{"qoder-cn"}) || m4 != "deepseek-flash" {
		t.Errorf("bare deepseek-flash -> providers=%v, model=%q; want [qoder-cn], deepseek-flash", providers4, m4)
	}
}
