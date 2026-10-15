package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestOpenAICompatExecutorOpenCodeExecuteEndToEnd(t *testing.T) {
	var gotHeaders http.Header
	var gotBody []byte

	// Mock OpenCode Zen upstream server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		gotBody, _ = io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "text/event-stream")
		// Simulate OpenCode Zen SSE stream where the model attempts to invoke stub tool "bash"
		sseResponse := `data: {"id":"chatcmpl-test-1","created":1700000000,"model":"big-pickle","choices":[{"index":0,"delta":{"role":"assistant"}}]}

data: {"id":"chatcmpl-test-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_stub_1","type":"function","function":{"name":"bash","arguments":"uname -a"}}]}}]}

data: {"id":"chatcmpl-test-1","choices":[{"index":0,"finish_reason":"tool_calls"}]}

data: [DONE]

`
		_, _ = w.Write([]byte(sseResponse))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{
		Provider: "openai-compatibility",
		Attributes: map[string]string{
			"base_url":     server.URL + "/zen/v1",
			"api_key":      "public",
			"compat_name":  "opencode",
			"provider_key": "opencode",
		},
	}

	ctx := util.WithSessionID(context.Background(), "test-session-affinity-001")
	payload := []byte(`{"model":"big-pickle","messages":[{"role":"user","content":"who are you"}]}`)

	// 1. Test non-streaming Execute
	resp, err := executor.Execute(ctx, auth, cliproxyexecutor.Request{
		Model:   "big-pickle",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       false,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	// Verify upstream headers
	if gotHeaders.Get("User-Agent") != openCodeUserAgent {
		t.Fatalf("expected User-Agent %s, got %s", openCodeUserAgent, gotHeaders.Get("User-Agent"))
	}
	sesID := gotHeaders.Get("x-opencode-session")
	if !openCodeRealSession(sesID) {
		t.Fatalf("expected valid real session ID, got %s", sesID)
	}
	if gotHeaders.Get("x-session-affinity") != sesID {
		t.Fatalf("expected x-session-affinity %s, got %s", sesID, gotHeaders.Get("x-session-affinity"))
	}
	if gotHeaders.Get("X-Session-Id") != sesID {
		t.Fatalf("expected X-Session-Id %s, got %s", sesID, gotHeaders.Get("X-Session-Id"))
	}

	// Verify upstream body has tools injected (bash and read) and stream: true
	bodyParsed := gjson.ParseBytes(gotBody)
	if !bodyParsed.Get("stream").Bool() {
		t.Fatalf("expected stream: true in upstream body")
	}
	tools := bodyParsed.Get("tools").Array()
	if len(tools) != 2 {
		t.Fatalf("expected 2 required tools (bash, read), got %d", len(tools))
	}

	// Verify downstream aggregated response: stub tool call dropped and converted to stop
	respJSON := gjson.ParseBytes(resp.Payload)
	if respJSON.Get("choices.0.finish_reason").String() != "stop" {
		t.Fatalf("expected finish_reason stop, got %s", respJSON.Get("choices.0.finish_reason").String())
	}
	if respJSON.Get("choices.0.message.tool_calls").Exists() {
		t.Fatalf("stub tool calls should be omitted from downstream message")
	}
	if respJSON.Get("choices.0.message.content").String() != openCodeStubNote {
		t.Fatalf("expected fallback stub note, got %s", respJSON.Get("choices.0.message.content").String())
	}

	// 2. Test streaming ExecuteStream
	streamResp, errStream := executor.ExecuteStream(ctx, auth, cliproxyexecutor.Request{
		Model:   "big-pickle",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       true,
	})
	if errStream != nil {
		t.Fatalf("ExecuteStream error: %v", errStream)
	}

	var streamChunks []string
	for chunk := range streamResp.Chunks {
		if chunk.Err != nil {
			t.Fatalf("unexpected chunk error: %v", chunk.Err)
		}
		streamChunks = append(streamChunks, string(chunk.Payload))
	}

	allStream := strings.Join(streamChunks, "")
	// Ensure no tool_calls emitted to downstream and finish_reason is stop
	if strings.Contains(allStream, `"tool_calls"`) && strings.Contains(allStream, `call_stub_1`) {
		t.Fatalf("stream emitted stub tool call chunks: %s", allStream)
	}
	if !strings.Contains(allStream, `"finish_reason":"stop"`) {
		t.Fatalf("expected stream to contain finish_reason: stop, got: %s", allStream)
	}
	if !strings.Contains(allStream, openCodeStubNote) {
		t.Fatalf("expected stream to emit stub note, got: %s", allStream)
	}
}
