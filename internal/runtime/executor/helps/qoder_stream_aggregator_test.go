package helps

import (
	"net/http"
	"testing"

	"github.com/tidwall/gjson"
)

func TestStripQoderForbiddenHeaders(t *testing.T) {
	req, err := http.NewRequest("POST", "https://api3.qoder.sh/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-anthropic-billing-header", "secret")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-client", "cli")
	req.Header.Set("User-Agent", "my-agent")

	StripQoderForbiddenHeaders(req)

	if req.Header.Get("x-anthropic-billing-header") != "" {
		t.Error("expected x-anthropic-billing-header to be stripped")
	}
	if req.Header.Get("anthropic-version") != "" {
		t.Error("expected anthropic-version to be stripped")
	}
	if req.Header.Get("x-client") != "" {
		t.Error("expected x-client to be stripped")
	}
	if req.Header.Get("User-Agent") != "my-agent" {
		t.Errorf("expected User-Agent preserved, got %s", req.Header.Get("User-Agent"))
	}
}

func TestSanitizeQoderPayload(t *testing.T) {
	raw := []byte(`{"model":"qwen3.8-flash-free","messages":[{"role":"user","content":"hi"}]}`)
	sanitized := SanitizeQoderPayload(raw)
	model := gjson.GetBytes(sanitized, "model").String()
	if model != "qwen3.8-flash" {
		t.Errorf("expected model 'qwen3.8-flash', got '%s'", model)
	}
}

func TestAggregateQoderStream(t *testing.T) {
	stream := []byte("data: {\"id\":\"chatcmpl-test-123\",\"created\":1700000000,\"model\":\"qwen3.8-flash\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello \",\"reasoning_content\":\"Thinking step 1... \"}}]}\n\n" +
		"data: {\"id\":\"chatcmpl-test-123\",\"created\":1700000000,\"model\":\"qwen3.8-flash\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"world!\",\"reasoning_content\":\"step 2 done.\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\n" +
		"data: [DONE]\n\n")

	agg, err := AggregateQoderStream(stream, "fallback-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed := gjson.ParseBytes(agg)
	if parsed.Get("id").String() != "chatcmpl-test-123" {
		t.Errorf("expected id 'chatcmpl-test-123', got '%s'", parsed.Get("id").String())
	}
	if parsed.Get("choices.0.message.content").String() != "Hello world!" {
		t.Errorf("expected content 'Hello world!', got '%s'", parsed.Get("choices.0.message.content").String())
	}
	if parsed.Get("choices.0.message.reasoning_content").String() != "Thinking step 1... step 2 done." {
		t.Errorf("expected reasoning 'Thinking step 1... step 2 done.', got '%s'", parsed.Get("choices.0.message.reasoning_content").String())
	}
	if parsed.Get("choices.0.finish_reason").String() != "stop" {
		t.Errorf("expected finish_reason 'stop', got '%s'", parsed.Get("choices.0.finish_reason").String())
	}
	if parsed.Get("usage.total_tokens").Int() != 15 {
		t.Errorf("expected total_tokens 15, got %d", parsed.Get("usage.total_tokens").Int())
	}
}

func TestAggregateQoderStreamToolCalls(t *testing.T) {
	stream := []byte("data: " + `{"id":"chatcmpl-tc","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_001","type":"function","function":{"name":"calc","arguments":"{\"x\":"}}]}}]}` + "\n\n" +
		"data: " + `{"id":"chatcmpl-tc","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]}}]}` + "\n\n" +
		"data: [DONE]\n\n")

	agg, err := AggregateQoderStream(stream, "qwen3.8-flash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed := gjson.ParseBytes(agg)
	if parsed.Get("choices.0.finish_reason").String() != "tool_calls" {
		t.Errorf("expected finish_reason 'tool_calls', got '%s'", parsed.Get("choices.0.finish_reason").String())
	}
	if parsed.Get("choices.0.message.tool_calls.0.function.name").String() != "calc" {
		t.Errorf("expected function name 'calc', got '%s'", parsed.Get("choices.0.message.tool_calls.0.function.name").String())
	}
	if parsed.Get("choices.0.message.tool_calls.0.function.arguments").String() != `{"x":1}` {
		t.Errorf("expected arguments '{\"x\":1}', got '%s'", parsed.Get("choices.0.message.tool_calls.0.function.arguments").String())
	}
}
