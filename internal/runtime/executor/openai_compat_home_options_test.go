package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cpaauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cpaexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestHomeV8PromptCacheKey(t *testing.T) {
	auth := &cpaauth.Auth{Provider: "compat", Attributes: map[string]string{"compat_name": "compat", "provider_key": "compat", "api_key": "fixture", "base_url": "https://example.test", "support_prompt_cache_key": "true"}, Metadata: map[string]any{"credential_options": map[string]any{"support-prompt-cache-key": true}}}
	for _, home := range []bool{false, true} {
		cfg := &config.Config{}
		cfg.Home.Enabled = home
		if !home {
			cfg.OpenAICompatibility = []config.OpenAICompatibility{{Name: "compat", SupportPromptCacheKey: true}}
		}
		e := NewOpenAICompatExecutor("compat", cfg)
		out, err := e.applyPromptCacheKey(context.Background(), auth, translator.FromString("openai"), "review-model", cpaexecutor.Request{Payload: []byte(`{"model":"review-model","prompt_cache_key":"fixture-cache-key"}`)}, cpaexecutor.Options{}, []byte(`{"model":"review-model"}`))
		if err != nil {
			t.Fatal(err)
		}
		got := gjson.GetBytes(out, "prompt_cache_key").String()
		t.Logf("home=%v prompt_cache_key=%q", home, got)
		if got != "fixture-cache-key" {
			t.Errorf("home=%v: enabled prompt cache option is not consumed", home)
		}
	}
}

func TestHomeV8CompatOptionsReachUpstream(t *testing.T) {
	bodies := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		bodies <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"fixture","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()
	cfg := &config.Config{}
	cfg.Home.Enabled = true
	e := NewOpenAICompatExecutor("home-compat", cfg)
	for _, enabled := range []bool{true, false, true} {
		auth := &cpaauth.Auth{Provider: "home-compat", Attributes: map[string]string{"compat_name": "home-compat", "api_key": "fixture", "base_url": server.URL + "/v1"}, Metadata: map[string]any{"credential_options": map[string]any{
			"support-prompt-cache-key": enabled,
			"models":                   []any{map[string]any{"name": "upstream", "use-max-completion-tokens": enabled}},
		}}}
		payload := []byte(`{"model":"upstream","input":[{"role":"user","content":"hi"}],"max_output_tokens":512,"prompt_cache_key":"fixture-cache"}`)
		_, err := e.Execute(context.Background(), auth, cpaexecutor.Request{Model: "upstream", Payload: payload}, cpaexecutor.Options{SourceFormat: translator.FromString("openai-response")})
		if err != nil {
			t.Fatal(err)
		}
		body := <-bodies
		field := "max_tokens"
		if enabled {
			field = "max_completion_tokens"
		}
		if gjson.GetBytes(body, field).Int() != 512 {
			t.Fatalf("enabled=%v missing %s in %s", enabled, field, body)
		}
		if enabled && gjson.GetBytes(body, "prompt_cache_key").String() != "fixture-cache" {
			t.Fatalf("prompt cache missing: %s", body)
		}
	}
}

func TestHomeV8PromptCacheOverrideAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name         string
		home         bool
		attrs        map[string]string
		metadata     map[string]any
		global, want bool
	}{
		{name: "explicit false", home: true, attrs: map[string]string{"support_prompt_cache_key": "false"}, metadata: map[string]any{"credential_options": map[string]any{"support-prompt-cache-key": true}}, global: true, want: false},
		{name: "older Home fallback", home: true, global: true, want: true},
		{name: "standalone ignores Home options", home: false, metadata: map[string]any{"credential_options": map[string]any{"support-prompt-cache-key": true}}, want: false},
		{name: "metadata only", home: true, metadata: map[string]any{"credential_options": map[string]any{"support-prompt-cache-key": true}}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{OpenAICompatibility: []config.OpenAICompatibility{{Name: "compat", SupportPromptCacheKey: tc.global}}}
			cfg.Home.Enabled = tc.home
			if tc.attrs == nil {
				tc.attrs = map[string]string{}
			}
			tc.attrs["compat_name"] = "compat"
			e := NewOpenAICompatExecutor("compat", cfg)
			auth := &cpaauth.Auth{Provider: "compat", Attributes: tc.attrs, Metadata: tc.metadata}
			result, err := e.applyPromptCacheKey(context.Background(), auth, translator.FromString("openai"), "model", cpaexecutor.Request{Payload: []byte(`{"prompt_cache_key":"cache"}`)}, cpaexecutor.Options{}, []byte(`{"model":"model"}`))
			if err != nil {
				t.Fatal(err)
			}
			if got := gjson.GetBytes(result, "prompt_cache_key").Exists(); got != tc.want {
				t.Fatalf("cache key present=%v, want=%v", got, tc.want)
			}
		})
	}
}
