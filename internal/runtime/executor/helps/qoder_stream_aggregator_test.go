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

	// DeepSeek aliases -> dfmodel
	rawDS := []byte(`{"model":"deepseek-flash","messages":[{"role":"user","content":"hi"}]}`)
	sanitizedDS := SanitizeQoderPayload(rawDS)
	if m := gjson.GetBytes(sanitizedDS, "model").String(); m != "dfmodel" {
		t.Errorf("expected model 'dfmodel' for deepseek-flash, got '%s'", m)
	}

	rawDSChat := []byte(`{"model":"qoder/deepseek-chat-free","messages":[{"role":"user","content":"hi"}]}`)
	sanitizedDSChat := SanitizeQoderPayload(rawDSChat)
	if m := gjson.GetBytes(sanitizedDSChat, "model").String(); m != "dfmodel" {
		t.Errorf("expected model 'dfmodel' for qoder/deepseek-chat-free, got '%s'", m)
	}

	// Qwen max aliases -> qmodel_38max
	rawQwen := []byte(`{"model":"qwen3.8-max","messages":[{"role":"user","content":"hi"}]}`)
	sanitizedQwen := SanitizeQoderPayload(rawQwen)
	if m := gjson.GetBytes(sanitizedQwen, "model").String(); m != "qmodel_38max" {
		t.Errorf("expected model 'qmodel_38max' for qwen3.8-max, got '%s'", m)
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

func TestUnwrapQoderSSELine(t *testing.T) {
	// 1. Wrapped Spring ResponseEntity chunk
	wrappedChunk := []byte(`data: {"headers":{"Content-Type":["application/json"]},"body":"{\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"index\":0}]}","statusCodeValue":200,"statusCode":"OK"}`)
	unwrapped, ok := UnwrapQoderSSELine(wrappedChunk)
	if !ok {
		t.Fatalf("expected unwrapped chunk to be ok")
	}
	want := `data: {"choices":[{"delta":{"content":"Hello"},"index":0}]}`
	if string(unwrapped) != want {
		t.Errorf("got %q, want %q", string(unwrapped), want)
	}

	// 2. Wrapped [DONE]
	wrappedDone := []byte(`data: {"headers":{"Content-Type":["application/json"]},"body":"[DONE]","statusCodeValue":200,"statusCode":"OK"}`)
	unwrappedDone, okDone := UnwrapQoderSSELine(wrappedDone)
	if !okDone {
		t.Fatalf("expected unwrapped done to be ok")
	}
	if string(unwrappedDone) != "data: [DONE]" {
		t.Errorf("got %q, want 'data: [DONE]'", string(unwrappedDone))
	}

	// 3. Raw standard [DONE]
	rawDone := []byte("data: [DONE]")
	unwrappedRawDone, okRawDone := UnwrapQoderSSELine(rawDone)
	if !okRawDone || string(unwrappedRawDone) != "data: [DONE]" {
		t.Errorf("got %q, ok=%v, want 'data: [DONE]', ok=true", string(unwrappedRawDone), okRawDone)
	}

	// 4. Raw standard OpenAI chunk
	rawChunk := []byte(`data: {"id":"c1","choices":[{"delta":{"content":"hi"}}]}`)
	unwrappedRaw, okRaw := UnwrapQoderSSELine(rawChunk)
	if !okRaw || string(unwrappedRaw) != string(rawChunk) {
		t.Errorf("got %q, ok=%v, want %q", string(unwrappedRaw), okRaw, string(rawChunk))
	}

	// 5. Telemetry & non-JSON events should be skipped
	telemetry := []byte(`data: {"firstTokenDuration":106,"totalDuration":634,"serverDuration":168}`)
	if _, okTel := UnwrapQoderSSELine(telemetry); okTel {
		t.Errorf("expected telemetry event to be skipped")
	}

	eventFinish := []byte("data: event:finish")
	if _, okFinish := UnwrapQoderSSELine(eventFinish); okFinish {
		t.Errorf("expected event:finish to be skipped")
	}

	emptyLine := []byte("   \n")
	if _, okEmpty := UnwrapQoderSSELine(emptyLine); okEmpty {
		t.Errorf("expected empty line to be skipped")
	}
}

func TestAggregateQoderStreamSpringWrapped(t *testing.T) {
	stream := []byte("data: {\"headers\":{\"Content-Type\":[\"application/json\"]},\"body\":\"{\\\"choices\\\":[{\\\"delta\\\":{\\\"reasoning_content\\\":\\\"Thinking... \\\"},\\\"index\\\":0}]}\",\"statusCodeValue\":200,\"statusCode\":\"OK\"}\n\n" +
		"data: {\"headers\":{\"Content-Type\":[\"application/json\"]},\"body\":\"{\\\"choices\\\":[{\\\"delta\\\":{\\\"content\\\":\\\"The answer is 42.\\\"},\\\"finish_reason\\\":\\\"stop\\\",\\\"index\\\":0}]}\",\"statusCodeValue\":200,\"statusCode\":\"OK\"}\n\n" +
		"data: {\"headers\":{\"Content-Type\":[\"application/json\"]},\"body\":\"[DONE]\",\"statusCodeValue\":200,\"statusCode\":\"OK\"}\n\n" +
		"data: event:finish\n\n" +
		"data: {\"firstTokenDuration\":77,\"totalDuration\":812,\"serverDuration\":102}\n\n")

	agg, err := AggregateQoderStream(stream, "dfmodel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed := gjson.ParseBytes(agg)
	if got := parsed.Get("choices.0.message.content").String(); got != "The answer is 42." {
		t.Errorf("expected content 'The answer is 42.', got %q", got)
	}
	if got := parsed.Get("choices.0.message.reasoning_content").String(); got != "Thinking... " {
		t.Errorf("expected reasoning 'Thinking... ', got %q", got)
	}
	if got := parsed.Get("choices.0.finish_reason").String(); got != "stop" {
		t.Errorf("expected finish_reason 'stop', got %q", got)
	}
}
