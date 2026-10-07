package executor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	translatorcommon "github.com/router-for-me/CLIProxyAPI/v8/internal/translator/common"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
)

// A turn whose only content is a file the target cannot receive must be refused
// before any upstream call, instead of forwarding an empty turn.
func TestExecutorsRefuseAFileOnlyTurnBeforeCallingUpstream(t *testing.T) {
	var upstreamCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	const (
		claudeFile = `{"model":"m","max_tokens":8,"messages":[{"role":"user","content":[{"type":"container_upload","file_id":"file_not_stored"}]}]}`
		openAIFile = `{"model":"m","messages":[{"role":"user","content":[{"type":"file","file":{"file_id":"file-not-stored"}}]}]}`
	)
	keyAuth := func(provider string, extra map[string]string) *cliproxyauth.Auth {
		attrs := map[string]string{"api_key": "k", "base_url": server.URL}
		for k, v := range extra {
			attrs[k] = v
		}
		return &cliproxyauth.Auth{Provider: provider, Attributes: attrs}
	}
	targets := []struct {
		name     string
		exec     cliproxyauth.ProviderExecutor
		auth     *cliproxyauth.Auth
		source   string
		payload  string
		wantPart string
	}{
		{"claude to openai-compat", NewOpenAICompatExecutor("openai-compatibility", &config.Config{OpenAICompatibility: []config.OpenAICompatibility{{Name: "compat"}}}),
			keyAuth("openai-compatibility", map[string]string{"base_url": server.URL + "/v1", "compat_name": "compat", "provider_key": "compat"}), "claude", claudeFile, "container_upload"},
		{"claude to codex", NewCodexExecutor(&config.Config{}), keyAuth("codex", map[string]string{"plan_type": "pro"}), "claude", claudeFile, "container_upload"},
		{"openai to claude", NewClaudeExecutor(&config.Config{}), keyAuth("claude", nil), "openai", openAIFile, "file"},
		{"openai to gemini", NewGeminiExecutor(&config.Config{}), keyAuth("gemini", nil), "openai", openAIFile, "file"},
	}
	for _, target := range targets {
		req := cliproxyexecutor.Request{Model: "m", Payload: []byte(target.payload)}
		opts := func(stream bool) cliproxyexecutor.Options {
			return cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString(target.source), Stream: stream}
		}
		calls := map[string]func() error{
			"Execute": func() error {
				_, err := target.exec.Execute(context.Background(), target.auth, req, opts(false))
				return err
			},
			"ExecuteStream": func() error {
				_, err := target.exec.ExecuteStream(context.Background(), target.auth, req, opts(true))
				return err
			},
			"CountTokens": func() error {
				_, err := target.exec.CountTokens(context.Background(), target.auth, req, opts(false))
				return err
			},
		}
		for method, call := range calls {
			err := call()
			var part *translatorcommon.UnsupportedPartError
			if !errors.As(err, &part) || part.Type != target.wantPart {
				t.Errorf("%s %s: err = %v, want unsupported %s", target.name, method, err, target.wantPart)
			}
		}
	}
	if upstreamCalls != 0 {
		t.Fatalf("upstream was called %d times, want 0", upstreamCalls)
	}
}
