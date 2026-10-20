package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestQoderExecutorIdentifierAndFormat(t *testing.T) {
	exec := NewQoderExecutor(&config.Config{})
	if exec.Identifier() != "qoder" {
		t.Errorf("Identifier() = %q, want qoder", exec.Identifier())
	}
	format := exec.RequestToFormat(cliproxyexecutor.Request{}, cliproxyexecutor.Options{})
	if format != sdktranslator.FormatOpenAI {
		t.Errorf("RequestToFormat() = %v, want FormatOpenAI", format)
	}
}

func TestQoderExecutorPrepareRequest(t *testing.T) {
	exec := NewQoderExecutor(&config.Config{})
	req, _ := http.NewRequest(http.MethodPost, "https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation", nil)
	req.Header.Set("x-anthropic-billing-header", "must-strip")

	auth := &cliproxyauth.Auth{
		ID: "user-qoder-01",
		Metadata: map[string]any{
			"job_token": "jt-12345",
			"realm":     "intl",
			"user_id":   "uid-qoder-01",
		},
	}

	body := `{"model":"qwen3.8-flash","messages":[]}`
	err := exec.PrepareRequest(req, auth, "jt-12345", body)
	if err != nil {
		t.Fatalf("PrepareRequest failed: %v", err)
	}

	if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer COSY.") {
		t.Errorf("Authorization missing Bearer COSY: %s", req.Header.Get("Authorization"))
	}
	if req.Header.Get("Cosy-Key") == "" {
		t.Error("Cosy-Key header is empty")
	}
	if req.Header.Get("Cosy-User") != "uid-qoder-01" {
		t.Errorf("Cosy-User expected uid-qoder-01, got %s", req.Header.Get("Cosy-User"))
	}
	if req.Header.Get("Cosy-Machinetype") != "5" {
		t.Errorf("Cosy-Machinetype expected 5, got %s", req.Header.Get("Cosy-Machinetype"))
	}
	if req.Header.Get("x-anthropic-billing-header") != "" {
		t.Error("x-anthropic-billing-header was not stripped")
	}
}

func TestQoderExecutorExecuteNonStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/algo/api/v2/service/pro/sse/agent_chat_generation" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer COSY.") {
			t.Errorf("missing or invalid Authorization header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Accept = %q, want text/event-stream", r.Header.Get("Accept"))
		}

		bodyBytes, errRead := io.ReadAll(r.Body)
		if errRead != nil {
			t.Fatalf("failed to read upstream body: %v", errRead)
		}
		if !gjson.GetBytes(bodyBytes, "stream").Bool() {
			t.Errorf("upstream request body stream = false, want true")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-qoder-999\",\"model\":\"qwen3.8-flash\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello from \"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-qoder-999\",\"model\":\"qwen3.8-flash\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Qoder!\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-qoder-999\",\"model\":\"qwen3.8-flash\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":4,\"total_tokens\":14}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	exec := NewQoderExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		ID: "user-qoder-01",
		Attributes: map[string]string{
			"base_url": server.URL,
			"realm":    "intl",
		},
		Metadata: map[string]any{
			"job_token": "jt-stream-test",
			"realm":     "intl",
			"user_id":   "uid-qoder-01",
		},
	}

	reqPayload := []byte(`{
		"model": "qwen3.8-flash",
		"messages": [{"role": "user", "content": "hi"}],
		"stream": false
	}`)

	resp, err := exec.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "qwen3.8-flash",
		Payload: reqPayload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FormatOpenAI,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	parsed := gjson.ParseBytes(resp.Payload)
	if parsed.Get("choices.0.message.content").String() != "Hello from Qoder!" {
		t.Errorf("content = %q, want 'Hello from Qoder!'", parsed.Get("choices.0.message.content").String())
	}
	if parsed.Get("choices.0.finish_reason").String() != "stop" {
		t.Errorf("finish_reason = %q, want stop", parsed.Get("choices.0.finish_reason").String())
	}
	if parsed.Get("usage.total_tokens").Int() != 14 {
		t.Errorf("total_tokens = %d, want 14", parsed.Get("usage.total_tokens").Int())
	}
}

func TestQoderExecutorExecuteStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-qoder-stream\",\"model\":\"qwen3.8-flash\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Chunk 1 \"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-qoder-stream\",\"model\":\"qwen3.8-flash\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Chunk 2\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	exec := NewQoderExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		ID: "user-qoder-02",
		Attributes: map[string]string{
			"base_url": server.URL,
			"realm":    "intl",
		},
		Metadata: map[string]any{
			"job_token": "jt-stream-test",
			"realm":     "intl",
			"user_id":   "uid-qoder-02",
		},
	}

	reqPayload := []byte(`{
		"model": "qwen3.8-flash",
		"messages": [{"role": "user", "content": "hello"}],
		"stream": true
	}`)

	streamRes, err := exec.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "qwen3.8-flash",
		Payload: reqPayload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FormatOpenAI,
	})
	if err != nil {
		t.Fatalf("ExecuteStream failed: %v", err)
	}

	var chunks [][]byte
	for chunk := range streamRes.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error: %v", chunk.Err)
		}
		chunks = append(chunks, chunk.Payload)
	}

	if len(chunks) == 0 {
		t.Fatal("expected at least one stream chunk, got 0")
	}
}

func TestQoderExecutorCountTokens(t *testing.T) {
	exec := NewQoderExecutor(&config.Config{})
	reqPayload := []byte(`{
		"model": "qwen3.8-flash",
		"messages": [{"role": "user", "content": "calculate tokens for this message"}]
	}`)

	resp, err := exec.CountTokens(context.Background(), nil, cliproxyexecutor.Request{
		Model:   "qwen3.8-flash",
		Payload: reqPayload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FormatOpenAI,
	})
	if err != nil {
		t.Fatalf("CountTokens failed: %v", err)
	}
	if len(resp.Payload) == 0 {
		t.Fatal("expected non-empty count tokens response")
	}
}
