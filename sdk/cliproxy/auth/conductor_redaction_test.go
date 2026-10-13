package auth

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/redact"
)

type redactionMockExecutor struct {
	lastReceivedPayload []byte
	responsePayload     []byte
	streamChunks        [][]byte
}

func (e *redactionMockExecutor) Identifier() string { return "test-provider" }

func (e *redactionMockExecutor) ShouldPrepareRequestAuth(auth *Auth) bool { return false }

func (e *redactionMockExecutor) PrepareRequestAuth(ctx context.Context, auth *Auth) (*Auth, error) {
	return auth, nil
}

func (e *redactionMockExecutor) Execute(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	e.lastReceivedPayload = append([]byte(nil), req.Payload...)
	return cliproxyexecutor.Response{
		Payload: append([]byte(nil), e.responsePayload...),
	}, nil
}

func (e *redactionMockExecutor) ExecuteStream(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	e.lastReceivedPayload = append([]byte(nil), req.Payload...)
	ch := make(chan cliproxyexecutor.StreamChunk, len(e.streamChunks))
	for _, chunkData := range e.streamChunks {
		ch <- cliproxyexecutor.StreamChunk{Payload: chunkData}
	}
	close(ch)
	return &cliproxyexecutor.StreamResult{
		Headers: http.Header{"Content-Type": []string{"text/event-stream"}},
		Chunks:  ch,
	}, nil
}

func (e *redactionMockExecutor) CountTokens(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	e.lastReceivedPayload = append([]byte(nil), req.Payload...)
	return cliproxyexecutor.Response{Payload: []byte(`{"count": 10}`)}, nil
}

func (e *redactionMockExecutor) HttpRequest(ctx context.Context, auth *Auth, req *http.Request) (*http.Response, error) {
	return nil, nil
}

func (e *redactionMockExecutor) Refresh(ctx context.Context, auth *Auth) (*Auth, error) {
	return auth, nil
}

type singleAuthSelector struct{}

func (singleAuthSelector) Pick(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, auths []*Auth) (*Auth, error) {
	if len(auths) > 0 {
		return auths[0], nil
	}
	return nil, &Error{Code: "auth_not_found", Message: "no auth"}
}

func TestConductor_Execute_Redaction_NonStreaming(t *testing.T) {
	secret := "sk-ant-api03-abcdef1234567890abcdef1234567890"
	inputPrompt := `{"prompt": "My OpenAI API key is ` + secret + `"}`

	trueVal := true
	cfg := &internalconfig.Config{
		Redaction: internalconfig.RedactionConfig{
			Enabled:     true,
			MaskSecrets: &trueVal,
		},
	}

	exec := &redactionMockExecutor{}
	manager := NewManager(nil, singleAuthSelector{}, nil)
	manager.runtimeConfig.Store(cfg)
	manager.RegisterExecutor(exec)

	if _, err := manager.Register(context.Background(), &Auth{
		ID:       "auth-test",
		Provider: "test-provider",
		Status:   StatusActive,
	}); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	registry.GetGlobalRegistry().RegisterClient("auth-test", "test-provider", []*registry.ModelInfo{{ID: "test-model"}})
	t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient("auth-test") })

	// Prepare mock executor to echo whatever placeholder it gets
	// We can pre-calculate the expected placeholder using SessionRedactor
	redactor := redact.NewSessionRedactor(redact.Options{Enabled: true, Secrets: true})
	maskedInput, _ := redactor.Mask(inputPrompt)
	idx := strings.Index(maskedInput, "{{API_KEY_")
	end := strings.Index(maskedInput, "}}") + 2
	placeholder := maskedInput[idx:end]

	exec.responsePayload = []byte(`{"answer": "Acknowledged key ` + placeholder + `"}`)

	req := cliproxyexecutor.Request{
		Model:   "test-model",
		Payload: []byte(inputPrompt),
	}
	resp, err := manager.Execute(context.Background(), []string{"test-provider"}, req, cliproxyexecutor.Options{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	// Check 1: upstream executor received masked payload, not raw secret
	if bytes.Contains(exec.lastReceivedPayload, []byte(secret)) {
		t.Fatalf("secret leaked in upstream payload: %s", string(exec.lastReceivedPayload))
	}
	if !bytes.Contains(exec.lastReceivedPayload, []byte("{{API_KEY_")) {
		t.Fatalf("expected placeholder in upstream payload: %s", string(exec.lastReceivedPayload))
	}

	// Check 2: downstream response was restored with secret
	respStr := string(resp.Payload)
	if !strings.Contains(respStr, secret) {
		t.Fatalf("secret not restored in response: %s", respStr)
	}
	if strings.Contains(respStr, placeholder) {
		t.Fatalf("unrestored placeholder leaked in response: %s", respStr)
	}
}

func TestConductor_ExecuteStream_Redaction_Streaming(t *testing.T) {
	secret := "sk-ant-streamingkey1234567890abcdef"
	inputPrompt := `{"prompt": "Use secret ` + secret + `"}`

	trueVal := true
	cfg := &internalconfig.Config{
		Redaction: internalconfig.RedactionConfig{
			Enabled:     true,
			MaskSecrets: &trueVal,
		},
	}

	redactor := redact.NewSessionRedactor(redact.Options{Enabled: true, Secrets: true})
	maskedInput, _ := redactor.Mask(inputPrompt)
	idx := strings.Index(maskedInput, "{{API_KEY_")
	end := strings.Index(maskedInput, "}}") + 2
	placeholder := maskedInput[idx:end]

	// Split placeholder across 2 stream chunks
	half := len(placeholder) / 2
	chunk1 := []byte("data: {\"text\": \"Key is " + placeholder[:half] + "\"}\n\n")
	chunk2 := []byte("data: {\"text\": \"" + placeholder[half:] + ".\"} \n\n")

	exec := &redactionMockExecutor{
		streamChunks: [][]byte{chunk1, chunk2},
	}
	manager := NewManager(nil, singleAuthSelector{}, nil)
	manager.runtimeConfig.Store(cfg)
	manager.RegisterExecutor(exec)

	if _, err := manager.Register(context.Background(), &Auth{
		ID:       "auth-test",
		Provider: "test-provider",
		Status:   StatusActive,
	}); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	registry.GetGlobalRegistry().RegisterClient("auth-test", "test-provider", []*registry.ModelInfo{{ID: "test-model"}})
	t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient("auth-test") })

	req := cliproxyexecutor.Request{
		Model:   "test-model",
		Payload: []byte(inputPrompt),
	}
	res, err := manager.ExecuteStream(context.Background(), []string{"test-provider"}, req, cliproxyexecutor.Options{Stream: true})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}

	var assembled bytes.Buffer
	for c := range res.Chunks {
		if c.Err != nil {
			t.Fatalf("chunk error: %v", c.Err)
		}
		assembled.Write(c.Payload)
	}

	fullOutput := assembled.String()
	if !strings.Contains(fullOutput, secret) {
		t.Fatalf("expected restored secret in assembled stream output.\nGot: %s\nExpected secret: %s", fullOutput, secret)
	}
	if strings.Contains(fullOutput, placeholder) {
		t.Fatalf("unrestored placeholder leaked in stream output: %s", fullOutput)
	}
}

func TestConductor_Redaction_DisabledByDefault(t *testing.T) {
	secret := "sk-ant-api03-abcdef1234567890abcdef1234567890"
	inputPrompt := `{"prompt": "Key ` + secret + `"}`

	cfg := &internalconfig.Config{
		Redaction: internalconfig.RedactionConfig{
			Enabled: false,
		},
	}

	exec := &redactionMockExecutor{
		responsePayload: []byte(`{"echo": "ok"}`),
	}
	manager := NewManager(nil, singleAuthSelector{}, nil)
	manager.runtimeConfig.Store(cfg)
	manager.RegisterExecutor(exec)

	if _, err := manager.Register(context.Background(), &Auth{
		ID:       "auth-test",
		Provider: "test-provider",
		Status:   StatusActive,
	}); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	registry.GetGlobalRegistry().RegisterClient("auth-test", "test-provider", []*registry.ModelInfo{{ID: "test-model"}})
	t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient("auth-test") })

	req := cliproxyexecutor.Request{
		Model:   "test-model",
		Payload: []byte(inputPrompt),
	}
	_, err := manager.Execute(context.Background(), []string{"test-provider"}, req, cliproxyexecutor.Options{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	// Secret passes through unmodified when redaction is disabled
	if !bytes.Contains(exec.lastReceivedPayload, []byte(secret)) {
		t.Fatalf("expected original secret to pass through when redaction is disabled")
	}
}
