package helps

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
)

func TestAggregateWorkBuddyStream_Basic(t *testing.T) {
	streamInput := []byte(`
data: {"id":"chatcmpl-1","model":"deepseek-v4.1-flash","choices":[{"index":0,"delta":{"content":"Hello"}}]}

data: {"id":"chatcmpl-1","model":"deepseek-v4.1-flash","choices":[{"index":0,"delta":{"content":" world!"}}]}

data: {"id":"chatcmpl-1","model":"deepseek-v4.1-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]
`)

	aggregated, err := AggregateWorkBuddyStream(streamInput, "deepseek-v4.1-flash")
	if err != nil {
		t.Fatalf("AggregateWorkBuddyStream failed: %v", err)
	}

	if !gjson.ValidBytes(aggregated) {
		t.Fatalf("Aggregated output is not valid JSON: %s", string(aggregated))
	}

	parsed := gjson.ParseBytes(aggregated)
	if parsed.Get("id").String() != "chatcmpl-1" {
		t.Errorf("id = %q, want chatcmpl-1", parsed.Get("id").String())
	}
	if parsed.Get("model").String() != "deepseek-v4.1-flash" {
		t.Errorf("model = %q, want deepseek-v4.1-flash", parsed.Get("model").String())
	}
	if parsed.Get("choices.0.message.content").String() != "Hello world!" {
		t.Errorf("content = %q, want Hello world!", parsed.Get("choices.0.message.content").String())
	}
	if parsed.Get("choices.0.finish_reason").String() != "stop" {
		t.Errorf("finish_reason = %q, want stop", parsed.Get("choices.0.finish_reason").String())
	}
	if parsed.Get("usage.total_tokens").Int() != 15 {
		t.Errorf("total_tokens = %d, want 15", parsed.Get("usage.total_tokens").Int())
	}
}

func TestAggregateWorkBuddyStream_ReasoningAndToolCalls(t *testing.T) {
	streamInput := []byte(`
data: {"id":"chatcmpl-2","choices":[{"index":0,"delta":{"reasoning_content":"Thinking step 1. "}}]}
data: {"id":"chatcmpl-2","choices":[{"index":0,"delta":{"reasoning_content":"Step 2."}}]}
data: {"id":"chatcmpl-2","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_123","type":"function","function":{"name":"search","arguments":"{\"q\":"}}]}}]}
data: {"id":"chatcmpl-2","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"golang\"}"}}]}}]}
data: {"id":"chatcmpl-2","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}
data: [DONE]
`)

	aggregated, err := AggregateWorkBuddyStream(streamInput, "fallback-model")
	if err != nil {
		t.Fatalf("AggregateWorkBuddyStream failed: %v", err)
	}

	parsed := gjson.ParseBytes(aggregated)
	if parsed.Get("choices.0.message.reasoning_content").String() != "Thinking step 1. Step 2." {
		t.Errorf("reasoning_content = %q, want 'Thinking step 1. Step 2.'", parsed.Get("choices.0.message.reasoning_content").String())
	}
	tc := parsed.Get("choices.0.message.tool_calls.0")
	if tc.Get("id").String() != "call_123" {
		t.Errorf("tool_call.id = %q, want call_123", tc.Get("id").String())
	}
	if tc.Get("function.name").String() != "search" {
		t.Errorf("tool_call.function.name = %q, want search", tc.Get("function.name").String())
	}
	if tc.Get("function.arguments").String() != `{"q":"golang"}` {
		t.Errorf("tool_call.function.arguments = %q, want {\"q\":\"golang\"}", tc.Get("function.arguments").String())
	}
	if parsed.Get("choices.0.finish_reason").String() != "tool_calls" {
		t.Errorf("finish_reason = %q, want tool_calls", parsed.Get("choices.0.finish_reason").String())
	}
}

func TestAggregateWorkBuddyStream_EmptyOrFallback(t *testing.T) {
	aggregated, err := AggregateWorkBuddyStream([]byte(""), "test-model")
	if err != nil {
		t.Fatalf("AggregateWorkBuddyStream on empty failed: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(aggregated, &out); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if out["model"] != "test-model" {
		t.Errorf("model = %v, want test-model", out["model"])
	}
}
