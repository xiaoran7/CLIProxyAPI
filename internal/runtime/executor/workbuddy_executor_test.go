package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
)

func TestWorkBuddyExecutorIdentifierAndFormat(t *testing.T) {
	exec := NewWorkBuddyExecutor(&config.Config{})
	if exec.Identifier() != "workbuddy" {
		t.Errorf("Identifier() = %q, want workbuddy", exec.Identifier())
	}
	format := exec.RequestToFormat(cliproxyexecutor.Request{}, cliproxyexecutor.Options{})
	if format != sdktranslator.FormatOpenAI {
		t.Errorf("RequestToFormat() = %v, want FormatOpenAI", format)
	}
}

func TestWorkBuddyExecutorPrepareRequest(t *testing.T) {
	exec := NewWorkBuddyExecutor(&config.Config{})
	req, _ := http.NewRequest(http.MethodPost, "https://www.workbuddy.ai/v2/chat/completions", nil)
	req.Header.Set("x-anthropic-billing-header", "must-strip")

	auth := &cliproxyauth.Auth{
		ID: "user-101",
		Metadata: map[string]any{
			"access_token": "tok-101",
			"realm":        "intl",
			"user_id":      "u-101",
		},
	}

	err := exec.PrepareRequest(req, auth)
	if err != nil {
		t.Fatalf("PrepareRequest failed: %v", err)
	}

	if req.Header.Get("Authorization") != "Bearer tok-101" {
		t.Errorf("Authorization = %q, want Bearer tok-101", req.Header.Get("Authorization"))
	}
	if req.Header.Get("x-anthropic-billing-header") != "" {
		t.Error("x-anthropic-billing-header was not stripped")
	}
	if req.Header.Get("X-Machine-ID") != "" {
		t.Errorf("X-Machine-ID must not be sent on chat completions, got %q", req.Header.Get("X-Machine-ID"))
	}
	if req.Header.Get("X-Session-ID") != "" {
		t.Errorf("X-Session-ID must not be sent on chat completions, got %q", req.Header.Get("X-Session-ID"))
	}
	if req.Header.Get("X-IDE-Name") != "WorkBuddy" {
		t.Errorf("X-IDE-Name = %q, want WorkBuddy", req.Header.Get("X-IDE-Name"))
	}
	if req.Header.Get("X-Product") != "SaaS" {
		t.Errorf("X-Product = %q, want SaaS", req.Header.Get("X-Product"))
	}
	if req.Header.Get("X-Conversation-ID") == "" {
		t.Error("X-Conversation-ID was not populated")
	}
}

func TestWorkBuddyExecutorExecuteNonStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("missing or invalid Authorization header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Machine-ID") != "" {
			t.Error("X-Machine-ID must not be sent to upstream chat endpoint")
		}

		// Verify that upstream request is always translated to stream: true
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Accept = %q, want text/event-stream", r.Header.Get("Accept"))
		}

		// Return SSE chunks as upstream WorkBuddy does
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-123\",\"model\":\"deepseek-v4.1-flash\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello from \"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-123\",\"model\":\"deepseek-v4.1-flash\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"WorkBuddy\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-123\",\"model\":\"deepseek-v4.1-flash\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	exec := NewWorkBuddyExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		ID: "user-101",
		Attributes: map[string]string{
			"base_url": server.URL,
			"realm":    "intl",
		},
		Metadata: map[string]any{
			"access_token": "tok-stream",
			"realm":        "intl",
		},
	}

	reqPayload := []byte(`{
		"model": "deepseek-v4.1-flash",
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	resp, err := exec.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "deepseek-v4.1-flash",
		Payload: reqPayload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FormatOpenAI,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(string(resp.Payload), "Hello from WorkBuddy") {
		t.Errorf("Unexpected response payload: %s", string(resp.Payload))
	}
}

func TestWorkBuddyExecutorExecuteStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"chunk1\"}}]}\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"chunk2\"}}]}\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	exec := NewWorkBuddyExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		ID: "user-101",
		Attributes: map[string]string{
			"base_url": server.URL,
		},
		Metadata: map[string]any{
			"access_token": "tok-stream",
			"realm":        "intl",
		},
	}

	reqPayload := []byte(`{
		"model": "deepseek-v4.1-flash",
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	streamResult, err := exec.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "deepseek-v4.1-flash",
		Payload: reqPayload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FormatOpenAI,
		Stream:       true,
	})

	if err != nil {
		t.Fatalf("ExecuteStream failed: %v", err)
	}

	var chunks []string
	for chunk := range streamResult.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error: %v", chunk.Err)
		}
		chunks = append(chunks, string(chunk.Payload))
	}

	if len(chunks) == 0 {
		t.Fatal("no chunks received from stream")
	}
}
