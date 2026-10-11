package executor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	"github.com/tidwall/sjson"
)

// toolSearchExecutorGoldenPayload is a Codex Responses turn with a client
// tool_search session plus a named declaration, which predates tool_search
// support only as a generic tool.
const toolSearchExecutorGoldenPayload = `{"model":"claude-sonnet-4-5","input":[` +
	`{"type":"message","role":"user","content":[{"type":"input_text","text":"create a calendar event"}]},` +
	`{"type":"tool_search_call","call_id":"call_search_1","status":"completed","execution":"client","arguments":{"query":"calendar"}},` +
	`{"type":"tool_search_output","call_id":"call_search_1","status":"completed","execution":"client","tools":[{"type":"namespace","name":"mcp__codex_apps__","description":"Apps.","tools":[{"type":"function","name":"calendar_create_event","description":"Create a calendar event.","strict":false,"defer_loading":true,"parameters":{"type":"object","properties":{"title":{"type":"string"}},"required":["title"]}}]}]}` +
	`],"tools":[` +
	`{"type":"tool_search","execution":"client","name":"ClientSearch","description":"Named search."},` +
	`{"type":"function","name":"exec_command","description":"Run a command.","strict":false,"parameters":{"type":"object","properties":{"cmd":{"type":"string"}},"required":["cmd"]}},` +
	`{"type":"function","name":"lookup_order","description":"Look up an order.","strict":false,"defer_loading":true,"parameters":{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}},` +
	`{"type":"tool_search","execution":"client","description":"Search deferred tools.","parameters":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}}` +
	`],"stream":false}`

var toolSearchExecutorGoldenCCH = regexp.MustCompile(`cch=[0-9a-f]+;`)

// renderToolSearchExecutorGolden captures the upstream bodies the Claude
// executor sends for toolSearchExecutorGoldenPayload on every request path.
func renderToolSearchExecutorGolden(t *testing.T) map[string]string {
	t.Helper()
	payload := []byte(toolSearchExecutorGoldenPayload)
	auths := map[string]map[string]string{
		"official": {"api_key": "key-123", "cloak_mode": "never"},
		"gateway":  {"api_key": "key-123", "cloak_mode": "never", "base_url": "https://gateway.example.com", "native_tool_search": "true"},
		"oauth":    {"api_key": "sk-ant-oat-test"},
	}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatOpenAIResponse, OriginalRequest: payload}
	request := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
	golden := make(map[string]string)
	for authName, attrs := range auths {
		for _, path := range []string{"execute", "stream", "count_tokens"} {
			auth := &cliproxyauth.Auth{Attributes: attrs, Metadata: map[string]any{"account_uuid": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}}
			upstream := &midSystemUpstream{}
			ctx := upstream.context(t, nil)
			ex := NewClaudeExecutor(&config.Config{})
			var errSend error
			switch path {
			case "execute":
				_, errSend = ex.Execute(ctx, auth, request, opts)
			case "stream":
				result, errStream := ex.ExecuteStream(ctx, auth, request, opts)
				errSend = errStream
				if errStream == nil {
					for chunk := range result.Chunks {
						if chunk.Err != nil {
							errSend = chunk.Err
						}
					}
				}
			case "count_tokens":
				_, errSend = ex.CountTokens(ctx, auth, request, opts)
			}
			if errSend != nil {
				t.Fatalf("%s %s: %v", authName, path, errSend)
			}
			// OAuth cloaking derives metadata.user_id from a random device id,
			// and the billing header's cch hash covers it.
			body, _ := sjson.DeleteBytes(upstream.body, "metadata.user_id")
			golden[authName+"/"+path] = toolSearchExecutorGoldenCCH.ReplaceAllString(string(body), "cch=0;")
		}
	}
	return golden
}

// The dev golden was produced by running renderToolSearchExecutorGolden against
// the executor at origin/dev d739485df, before Codex tool_search support. With
// upstream.claude.tool-search off every request path must reproduce it, even
// for credentials that would otherwise use native tool search.
func TestClaudeExecutor_ToolSearchDisabledMatchesDevGolden(t *testing.T) {
	enableClaudeToolSearchForTest(t, false)
	if NewClaudeExecutor(&config.Config{}).SupportsNativeToolSearch(nativeToolSearchAuth(nil), "claude-sonnet-4-5") {
		t.Fatal("SupportsNativeToolSearch() = true with upstream.claude.tool-search off")
	}
	raw, errRead := os.ReadFile(filepath.Join("testdata", "tool_search_disabled", "dev_executor_golden.json"))
	if errRead != nil {
		t.Fatalf("read golden: %v", errRead)
	}
	var want map[string]string
	if errUnmarshal := json.Unmarshal(raw, &want); errUnmarshal != nil {
		t.Fatalf("decode golden: %v", errUnmarshal)
	}
	got := renderToolSearchExecutorGolden(t)
	if len(got) != len(want) {
		t.Fatalf("rendered %d bodies, golden has %d", len(got), len(want))
	}
	for name, body := range got {
		if body != want[name] {
			t.Errorf("%s differs from dev\n got: %s\nwant: %s", name, body, want[name])
		}
	}
}

// With the switch on, the same request must leave the dev translation, so the
// golden comparison above cannot pass by accident.
func TestClaudeExecutor_ToolSearchEnabledDiffersFromDevGolden(t *testing.T) {
	enableClaudeToolSearchForTest(t, true)
	raw, errRead := os.ReadFile(filepath.Join("testdata", "tool_search_disabled", "dev_executor_golden.json"))
	if errRead != nil {
		t.Fatalf("read golden: %v", errRead)
	}
	var want map[string]string
	if errUnmarshal := json.Unmarshal(raw, &want); errUnmarshal != nil {
		t.Fatalf("decode golden: %v", errUnmarshal)
	}
	if got := renderToolSearchExecutorGolden(t); got["official/execute"] == want["official/execute"] {
		t.Fatal("official/execute matches dev with upstream.claude.tool-search on")
	}
}
