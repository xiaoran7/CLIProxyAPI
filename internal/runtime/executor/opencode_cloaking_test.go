package executor

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
	"github.com/tidwall/gjson"
)

func TestOpenCodeIDGeneration(t *testing.T) {
	// Test session ID (descending = true)
	sesID := genOpenCodeID("ses", true, time.Now())
	if !strings.HasPrefix(sesID, "ses_") {
		t.Fatalf("expected ses_ prefix, got %s", sesID)
	}
	if len(sesID) != 30 {
		t.Fatalf("expected len 30, got %d for %s", len(sesID), sesID)
	}
	if !openCodeRealSession(sesID) {
		t.Fatalf("expected openCodeRealSession to validate genuine session ID %s", sesID)
	}

	// Test request ID (descending = false)
	msgID := genOpenCodeID("msg", false, time.Now())
	if !strings.HasPrefix(msgID, "msg_") {
		t.Fatalf("expected msg_ prefix, got %s", msgID)
	}
	if len(msgID) != 30 {
		t.Fatalf("expected len 30, got %d for %s", len(msgID), msgID)
	}

	// Verify monotonic clock ordering
	t1 := time.Now()
	id1 := genOpenCodeID("ses", true, t1)
	id2 := genOpenCodeID("ses", true, t1)
	if id1 == id2 {
		t.Fatalf("expected distinct IDs, got duplicate %s", id1)
	}
}

func TestOpenCodeSessionAffinity(t *testing.T) {
	seed := "conv-session-12345"
	idFirst := resolveOpenCodeSessionID(seed)
	if !strings.HasPrefix(idFirst, "ses_") {
		t.Fatalf("expected ses_ prefix, got %s", idFirst)
	}

	// Calling with the same seed should return identical session ID
	idSecond := resolveOpenCodeSessionID(seed)
	if idFirst != idSecond {
		t.Fatalf("expected affinity to return same id %s, got %s", idFirst, idSecond)
	}

	// Calling with a different seed should return a different session ID
	otherSeed := "conv-session-67890"
	idOther := resolveOpenCodeSessionID(otherSeed)
	if idOther == idFirst {
		t.Fatalf("expected different session ID for different seed, got %s", idOther)
	}

	// If already a valid real OpenCode session ID, it should be preserved verbatim
	directID := genOpenCodeID("ses", true, time.Now())
	resolvedDirect := resolveOpenCodeSessionID(directID)
	if resolvedDirect != directID {
		t.Fatalf("expected direct real session ID to be preserved, got %s vs %s", resolvedDirect, directID)
	}
}

func TestNormalizeOpenCodeEndpoint(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"https://opencode.ai/zen/v1", "https://opencode.ai/inference/v1"},
		{"https://opencode.ai/zen/v1/chat/completions", "https://opencode.ai/inference/v1/chat/completions"},
		{"https://opencode.ai/inference/v1", "https://opencode.ai/inference/v1"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1"},
	}
	for _, tc := range cases {
		actual := normalizeOpenCodeEndpoint(tc.input)
		if actual != tc.expected {
			t.Errorf("normalizeOpenCodeEndpoint(%q) = %q, expected %q", tc.input, actual, tc.expected)
		}
	}
}

func TestApplyOpenCodeHeaders(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://opencode.ai/zen/v1/chat/completions", nil)
	// Inject headers that should be stripped
	req.Header.Set("x-client", "cursor")
	req.Header.Set("x-app-name", "cursor")
	req.Header.Set("x-zcode-app-version", "1.0.0")
	req.Header.Set("x-stainless-os", "Windows")
	req.Header.Set("x-cursor-client-version", "0.45.0")
	req.Header.Set("x-cline-version", "3.0.0")
	req.Header.Set("x-continue-session-id", "cont-123")

	// Set session context
	ctx := util.WithSessionID(context.Background(), "user-sess-abc")
	req = req.WithContext(ctx)

	applyOpenCodeHeaders(req)

	// Verify forbidden headers are stripped
	for _, h := range []string{
		"x-client",
		"x-app-name",
		"x-zcode-app-version",
		"x-stainless-os",
		"x-cursor-client-version",
		"x-cline-version",
		"x-continue-session-id",
	} {
		if req.Header.Get(h) != "" {
			t.Fatalf("expected header %s to be stripped, got %s", h, req.Header.Get(h))
		}
	}

	// Verify required headers
	expectedUA := openCodeUserAgent
	if req.Header.Get("User-Agent") != expectedUA {
		t.Fatalf("expected User-Agent %s, got %s", expectedUA, req.Header.Get("User-Agent"))
	}
	if req.Header.Get("x-opencode-client") != "cli" {
		t.Fatalf("expected x-opencode-client cli, got %s", req.Header.Get("x-opencode-client"))
	}
	if req.Header.Get("x-opencode-project") != "global" {
		t.Fatalf("expected x-opencode-project global, got %s", req.Header.Get("x-opencode-project"))
	}

	sesID := req.Header.Get("x-opencode-session")
	if !openCodeRealSession(sesID) {
		t.Fatalf("expected valid session ID, got %s", sesID)
	}
	if req.Header.Get("x-opencode-session-id") != sesID {
		t.Fatalf("expected x-opencode-session-id match %s, got %s", sesID, req.Header.Get("x-opencode-session-id"))
	}
	if req.Header.Get("x-session-affinity") != sesID {
		t.Fatalf("expected x-session-affinity match %s, got %s", sesID, req.Header.Get("x-session-affinity"))
	}
	if req.Header.Get("X-Session-Id") != sesID {
		t.Fatalf("expected X-Session-Id match %s, got %s", sesID, req.Header.Get("X-Session-Id"))
	}

	msgID := req.Header.Get("x-opencode-request")
	if !strings.HasPrefix(msgID, "msg_") {
		t.Fatalf("expected msg_ prefix for request ID, got %s", msgID)
	}

	// Subsequent request on same context should reuse session ID
	req2, _ := http.NewRequestWithContext(ctx, "POST", "https://opencode.ai/zen/v1/chat/completions", nil)
	applyOpenCodeHeaders(req2)
	if req2.Header.Get("x-opencode-session") != sesID {
		t.Fatalf("expected session affinity across requests with same context, got %s vs %s", req2.Header.Get("x-opencode-session"), sesID)
	}
}

func TestApplyOpenCodePayload(t *testing.T) {
	// Case 1: Caller provides no tools -> bash and read stubs injected
	raw := []byte(`{"model":"big-pickle","messages":[{"role":"user","content":"hello"}]}`)
	mutated, info, err := applyOpenCodePayload(raw, true)
	if err != nil {
		t.Fatalf("applyOpenCodePayload error: %v", err)
	}

	if !gjson.GetBytes(mutated, "stream").Bool() {
		t.Fatalf("expected stream: true")
	}
	if gjson.GetBytes(mutated, "max_tokens").Int() != 32000 {
		t.Fatalf("expected default max_tokens: 32000")
	}

	tools := gjson.GetBytes(mutated, "tools").Array()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if !info.stubs["bash"] || !info.stubs["read"] {
		t.Fatalf("expected both bash and read in stubs map")
	}

	// Verify stub tools declare non-empty properties to pass FreeTier tool validation
	for _, tool := range tools {
		name := tool.Get("function.name").String()
		switch name {
		case "bash":
			if tool.Get("function.parameters.properties.command.type").String() != "string" {
				t.Fatalf("expected bash tool to declare command parameter, got %s", tool.Raw)
			}
		case "read":
			if tool.Get("function.parameters.properties.filePath.type").String() != "string" {
				t.Fatalf("expected read tool to declare filePath parameter, got %s", tool.Raw)
			}
		}
		if tool.Get("function.parameters.additionalProperties").Bool() != false {
			t.Fatalf("expected additionalProperties false on stub tool %s", name)
		}
	}

	// Case 2: Caller provides "Bash" and "Read" with uppercase casing
	customTools := []byte(`{
		"model": "big-pickle",
		"messages": [{"role":"user","content":"run command"}],
		"tools": [
			{"type": "function", "function": {"name": "Bash", "description": "Client Bash"}},
			{"type": "function", "function": {"name": "Read", "description": "Client Read"}}
		]
	}`)
	mutatedCustom, infoCustom, errCustom := applyOpenCodePayload(customTools, false)
	if errCustom != nil {
		t.Fatalf("applyOpenCodePayload custom tools error: %v", errCustom)
	}

	// Tools should be normalized to lowercase
	tArr := gjson.GetBytes(mutatedCustom, "tools").Array()
	if len(tArr) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tArr))
	}
	if tArr[0].Get("function.name").String() != "bash" || tArr[1].Get("function.name").String() != "read" {
		t.Fatalf("expected lowercase tool names, got %s and %s", tArr[0].Get("function.name").String(), tArr[1].Get("function.name").String())
	}
	if infoCustom.stubs["bash"] || infoCustom.stubs["read"] {
		t.Fatalf("client provided bash and read, should not be marked as stubs")
	}
	if infoCustom.back["bash"] != "Bash" || infoCustom.back["read"] != "Read" {
		t.Fatalf("expected back mapping for Bash and Read, got %v", infoCustom.back)
	}

	// Case 3: Caller provides bash and read with empty properties -> patched defensively
	emptyPropsTools := []byte(`{
		"model": "big-pickle",
		"messages": [{"role":"user","content":"run"}],
		"tools": [
			{"type": "function", "function": {"name": "bash", "parameters": {"type": "object", "properties": {}}}},
			{"type": "function", "function": {"name": "read", "parameters": {"type": "object", "properties": {}}}}
		]
	}`)
	mutatedEmpty, _, errEmpty := applyOpenCodePayload(emptyPropsTools, false)
	if errEmpty != nil {
		t.Fatalf("applyOpenCodePayload empty properties error: %v", errEmpty)
	}
	emptyToolsArr := gjson.GetBytes(mutatedEmpty, "tools").Array()
	if len(emptyToolsArr) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(emptyToolsArr))
	}
	if emptyToolsArr[0].Get("function.parameters.properties.command.type").String() != "string" {
		t.Fatalf("expected bash tool properties to be defensively patched, got %s", emptyToolsArr[0].Raw)
	}
	if emptyToolsArr[1].Get("function.parameters.properties.filePath.type").String() != "string" {
		t.Fatalf("expected read tool properties to be defensively patched, got %s", emptyToolsArr[1].Raw)
	}
}

func TestOpenCodeStreamFilter(t *testing.T) {
	// Stub tools injected: "bash" and "read"
	info := &openCodeToolsInfo{
		back:  map[string]string{"custom_bash": "Custom_Bash"},
		stubs: map[string]bool{"bash": true, "read": true},
	}
	filter := newOpenCodeStreamFilter(info)

	// Chunk 1: Role delta
	chunk1 := []byte(`{"id":"1","choices":[{"index":0,"delta":{"role":"assistant"}}]}`)
	f1, drop1 := filter.FilterChunk(chunk1)
	if drop1 {
		t.Fatalf("chunk1 should not be dropped")
	}
	if !bytes.Equal(chunk1, f1) {
		t.Fatalf("expected chunk1 unchanged")
	}

	// Chunk 2: Upstream calls stub "bash"
	chunk2 := []byte(`{"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"bash","arguments":""}}]}}]}`)
	_, drop2 := filter.FilterChunk(chunk2)
	if !drop2 {
		t.Fatalf("chunk2 calling stub tool should be dropped entirely")
	}

	// Chunk 3: Arguments for stub tool call
	chunk3 := []byte(`{"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"echo hello"}}]}}]}`)
	_, drop3 := filter.FilterChunk(chunk3)
	if !drop3 {
		t.Fatalf("chunk3 argument chunk for stub should be dropped entirely")
	}

	// Chunk 4: Finish reason tool_calls -> should be rewritten to stop with stub note
	chunk4 := []byte(`{"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
	f4, drop4 := filter.FilterChunk(chunk4)
	if drop4 {
		t.Fatalf("finish chunk should not be dropped")
	}
	finishReason := gjson.GetBytes(f4, "choices.0.finish_reason").String()
	if finishReason != "stop" {
		t.Fatalf("expected finish_reason to be rewritten to stop, got %s", finishReason)
	}
	content := gjson.GetBytes(f4, "choices.0.delta.content").String()
	if content != openCodeStubNote {
		t.Fatalf("expected stub note content, got %s", content)
	}
}

func TestAggregateOpenCodeSSEToChatCompletion(t *testing.T) {
	info := &openCodeToolsInfo{
		back:  map[string]string{},
		stubs: map[string]bool{"bash": true},
	}

	// SSE stream where model called stub "bash"
	sseData := `data: {"id":"chat-1","created":1700000000,"model":"big-pickle","choices":[{"index":0,"delta":{"role":"assistant"}}]}

data: {"id":"chat-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"bash","arguments":"ls -la"}}]}}]}

data: {"id":"chat-1","choices":[{"index":0,"finish_reason":"tool_calls"}]}

data: [DONE]
`
	resBytes, err := aggregateOpenCodeSSEToChatCompletion(strings.NewReader(sseData), "big-pickle", info)
	if err != nil {
		t.Fatalf("aggregate error: %v", err)
	}

	res := gjson.ParseBytes(resBytes)
	if res.Get("choices.0.finish_reason").String() != "stop" {
		t.Fatalf("expected finish_reason stop, got %s", res.Get("choices.0.finish_reason").String())
	}
	if res.Get("choices.0.message.tool_calls").Exists() {
		t.Fatalf("stub tool calls should be omitted from final message")
	}
	if res.Get("choices.0.message.content").String() != openCodeStubNote {
		t.Fatalf("expected fallback stub note, got %s", res.Get("choices.0.message.content").String())
	}
}
