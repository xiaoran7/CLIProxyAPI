package claude

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertGeminiResponseToClaude_SignatureOnlyPartDoesNotOpenEmptyTextBlock(t *testing.T) {
	requestJSON := []byte(`{"model":"gemini-test","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	thinkingChunk := []byte(`{
		"candidates": [{
			"content": {
				"parts": [{"text": "thinking text", "thought": true}]
			}
		}],
		"modelVersion": "gemini-test",
		"responseId": "resp-test"
	}`)
	signatureChunk := []byte(`{
		"candidates": [{
			"content": {
				"parts": [{"text": "", "thoughtSignature": "sig-test"}]
			},
			"finishReason": "STOP"
		}],
		"usageMetadata": {
			"promptTokenCount": 10,
			"thoughtsTokenCount": 2,
			"totalTokenCount": 12
		},
		"modelVersion": "gemini-test",
		"responseId": "resp-test"
	}`)

	var param any
	ctx := context.Background()
	output := bytes.Join(ConvertGeminiResponseToClaude(ctx, "gemini-test", requestJSON, requestJSON, thinkingChunk, &param), nil)
	output = append(output, bytes.Join(ConvertGeminiResponseToClaude(ctx, "gemini-test", requestJSON, requestJSON, signatureChunk, &param), nil)...)
	output = append(output, bytes.Join(ConvertGeminiResponseToClaude(ctx, "gemini-test", requestJSON, requestJSON, []byte("[DONE]"), &param), nil)...)
	outputText := string(output)

	if strings.Contains(outputText, `"content_block":{"type":"text"`) {
		t.Fatalf("signature-only part must not open an empty text block: %s", outputText)
	}
	if strings.Contains(outputText, `"type":"content_block_stop","index":1`) {
		t.Fatalf("signature-only part must not produce a stop for unopened index 1: %s", outputText)
	}
	if !strings.Contains(outputText, `"type":"signature_delta"`) || !strings.Contains(outputText, `"signature":"sig-test"`) {
		t.Fatalf("signature-only part must be emitted as a thinking signature delta: %s", outputText)
	}
	if got := strings.Count(outputText, `"type":"content_block_stop","index":0`); got != 1 {
		t.Fatalf("expected exactly one stop for thinking index 0, got %d: %s", got, outputText)
	}
	if !strings.Contains(outputText, `"type":"message_delta"`) || !strings.Contains(outputText, `"output_tokens":2`) {
		t.Fatalf("finish chunk without candidatesTokenCount must still emit final message_delta: %s", outputText)
	}
	if !strings.Contains(outputText, `"type":"message_stop"`) {
		t.Fatalf("DONE chunk must still emit message_stop after final events: %s", outputText)
	}
}

func TestConvertGeminiResponseToClaudeNonStream_PreservesThoughtSignature(t *testing.T) {
	requestJSON := []byte(`{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"hi"}]}`)
	geminiResponse := []byte(`{
		"candidates": [{
			"content": {
				"parts": [
					{"text": "thinking step 1\n", "thought": true},
					{"text": "thinking step 2", "thought": true, "thoughtSignature": "sig-xyz-123"},
					{"text": "visible answer"}
				]
			},
			"finishReason": "STOP"
		}],
		"usageMetadata": {
			"promptTokenCount": 10,
			"candidatesTokenCount": 5
		},
		"modelVersion": "gemini-2.5-pro",
		"responseId": "resp-non-stream"
	}`)

	ctx := context.Background()
	output := ConvertGeminiResponseToClaudeNonStream(ctx, "gemini-2.5-pro", requestJSON, requestJSON, geminiResponse, nil)
	outputJSON := gjson.ParseBytes(output)

	blocks := outputJSON.Get("content").Array()
	if len(blocks) != 2 {
		t.Fatalf("expected 2 content blocks (thinking + text), got %d: %s", len(blocks), string(output))
	}

	thinkingBlock := blocks[0]
	if thinkingBlock.Get("type").String() != "thinking" {
		t.Fatalf("expected first block to be thinking, got %s", thinkingBlock.Get("type").String())
	}
	if thinkingBlock.Get("thinking").String() != "thinking step 1\nthinking step 2" {
		t.Fatalf("unexpected thinking content: %s", thinkingBlock.Get("thinking").String())
	}
	if thinkingBlock.Get("signature").String() != "sig-xyz-123" {
		t.Fatalf("expected signature 'sig-xyz-123', got %q. Output: %s", thinkingBlock.Get("signature").String(), string(output))
	}

	textBlock := blocks[1]
	if textBlock.Get("type").String() != "text" || textBlock.Get("text").String() != "visible answer" {
		t.Fatalf("unexpected text block: %s", textBlock.Raw)
	}
}

func TestConvertGeminiResponseToClaudeNonStream_PartWithThoughtSignatureWithoutThoughtBool(t *testing.T) {
	requestJSON := []byte(`{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"hi"}]}`)
	geminiResponse := []byte(`{
		"candidates": [{
			"content": {
				"parts": [
					{"text": "inferred reasoning", "thought_signature": "sig-snake-case"},
					{"text": "final answer"}
				]
			},
			"finishReason": "STOP"
		}],
		"usageMetadata": {
			"promptTokenCount": 10,
			"candidatesTokenCount": 5
		},
		"modelVersion": "gemini-2.5-pro",
		"responseId": "resp-non-stream-2"
	}`)

	ctx := context.Background()
	output := ConvertGeminiResponseToClaudeNonStream(ctx, "gemini-2.5-pro", requestJSON, requestJSON, geminiResponse, nil)
	outputJSON := gjson.ParseBytes(output)

	blocks := outputJSON.Get("content").Array()
	if len(blocks) != 2 {
		t.Fatalf("expected 2 content blocks (thinking + text), got %d: %s", len(blocks), string(output))
	}

	thinkingBlock := blocks[0]
	if thinkingBlock.Get("type").String() != "thinking" {
		t.Fatalf("expected first block to be thinking, got %s", thinkingBlock.Get("type").String())
	}
	if thinkingBlock.Get("thinking").String() != "inferred reasoning" {
		t.Fatalf("unexpected thinking content: %s", thinkingBlock.Get("thinking").String())
	}
	if thinkingBlock.Get("signature").String() != "sig-snake-case" {
		t.Fatalf("expected signature 'sig-snake-case', got %q. Output: %s", thinkingBlock.Get("signature").String(), string(output))
	}

	textBlock := blocks[1]
	if textBlock.Get("type").String() != "text" || textBlock.Get("text").String() != "final answer" {
		t.Fatalf("unexpected text block: %s", textBlock.Raw)
	}
}

func TestConvertGeminiResponseToClaudeNonStream_TrailingSignatureOnlyPart(t *testing.T) {
	requestJSON := []byte(`{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"hi"}]}`)
	geminiResponse := []byte(`{
		"candidates": [{
			"content": {
				"parts": [
					{"text": "thinking step 1\n", "thought": true},
					{"text": "", "thoughtSignature": "sig-trailing"},
					{"text": "visible answer"}
				]
			},
			"finishReason": "STOP"
		}],
		"usageMetadata": {
			"promptTokenCount": 10,
			"candidatesTokenCount": 5
		},
		"modelVersion": "gemini-2.5-pro",
		"responseId": "resp-non-stream-trailing"
	}`)

	ctx := context.Background()
	output := ConvertGeminiResponseToClaudeNonStream(ctx, "gemini-2.5-pro", requestJSON, requestJSON, geminiResponse, nil)
	outputJSON := gjson.ParseBytes(output)

	blocks := outputJSON.Get("content").Array()
	if len(blocks) != 2 {
		t.Fatalf("expected 2 content blocks (thinking + text), got %d: %s", len(blocks), string(output))
	}

	thinkingBlock := blocks[0]
	if thinkingBlock.Get("type").String() != "thinking" {
		t.Fatalf("expected first block to be thinking, got %s", thinkingBlock.Get("type").String())
	}
	if thinkingBlock.Get("thinking").String() != "thinking step 1\n" {
		t.Fatalf("unexpected thinking content: %s", thinkingBlock.Get("thinking").String())
	}
	if thinkingBlock.Get("signature").String() != "sig-trailing" {
		t.Fatalf("expected signature 'sig-trailing', got %q. Output: %s", thinkingBlock.Get("signature").String(), string(output))
	}

	textBlock := blocks[1]
	if textBlock.Get("type").String() != "text" || textBlock.Get("text").String() != "visible answer" {
		t.Fatalf("unexpected text block: %s", textBlock.Raw)
	}
}

func TestConvertGeminiResponseToClaude_UsageWithCachedContentTokenCount(t *testing.T) {
	requestJSON := []byte(`{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"hi"}]}`)
	chunk := []byte(`{
		"candidates": [{
			"content": {
				"parts": [{"text": "Hello world"}]
			},
			"finishReason": "STOP"
		}],
		"usageMetadata": {
			"promptTokenCount": 100,
			"candidatesTokenCount": 7,
			"cachedContentTokenCount": 91
		},
		"modelVersion": "gemini-2.5-pro",
		"responseId": "resp-usage-cache"
	}`)

	var param any
	ctx := context.Background()
	output := bytes.Join(ConvertGeminiResponseToClaude(ctx, "gemini-2.5-pro", requestJSON, requestJSON, chunk, &param), nil)
	outputText := string(output)

	if !strings.Contains(outputText, `"type":"message_delta"`) {
		t.Fatalf("expected message_delta event in output, got: %s", outputText)
	}

	foundMessageDelta := false
	// Find the message_delta event data
	for _, line := range strings.Split(outputText, "\n") {
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"type":"message_delta"`) {
			foundMessageDelta = true
			deltaJSON := gjson.Parse(strings.TrimPrefix(line, "data: "))
			inputTokens := deltaJSON.Get("usage.input_tokens").Int()
			if inputTokens != 9 {
				t.Fatalf("expected usage.input_tokens = 9 (100 - 91), got %d. Payload: %s", inputTokens, line)
			}
			cacheReadTokens := deltaJSON.Get("usage.cache_read_input_tokens").Int()
			if cacheReadTokens != 91 {
				t.Fatalf("expected usage.cache_read_input_tokens = 91, got %d. Payload: %s", cacheReadTokens, line)
			}
			outputTokens := deltaJSON.Get("usage.output_tokens").Int()
			if outputTokens != 7 {
				t.Fatalf("expected usage.output_tokens = 7, got %d. Payload: %s", outputTokens, line)
			}
		}
	}
	if !foundMessageDelta {
		t.Fatalf("failed to locate parsed message_delta event in payload: %s", outputText)
	}
}

func TestConvertGeminiResponseToClaudeNonStream_UsageWithCachedContentTokenCount(t *testing.T) {
	requestJSON := []byte(`{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"hi"}]}`)
	geminiResponse := []byte(`{
		"candidates": [{
			"content": {
				"parts": [{"text": "Hello world"}]
			},
			"finishReason": "STOP"
		}],
		"usageMetadata": {
			"promptTokenCount": 100,
			"candidatesTokenCount": 7,
			"cachedContentTokenCount": 91
		},
		"modelVersion": "gemini-2.5-pro",
		"responseId": "resp-usage-cache-nonstream"
	}`)

	ctx := context.Background()
	output := ConvertGeminiResponseToClaudeNonStream(ctx, "gemini-2.5-pro", requestJSON, requestJSON, geminiResponse, nil)
	outputJSON := gjson.ParseBytes(output)

	inputTokens := outputJSON.Get("usage.input_tokens").Int()
	if inputTokens != 9 {
		t.Fatalf("expected usage.input_tokens = 9 (100 - 91), got %d. Output: %s", inputTokens, string(output))
	}
	cacheReadTokens := outputJSON.Get("usage.cache_read_input_tokens").Int()
	if cacheReadTokens != 91 {
		t.Fatalf("expected usage.cache_read_input_tokens = 91, got %d. Output: %s", cacheReadTokens, string(output))
	}
	outputTokens := outputJSON.Get("usage.output_tokens").Int()
	if outputTokens != 7 {
		t.Fatalf("expected usage.output_tokens = 7, got %d. Output: %s", outputTokens, string(output))
	}
}

func TestConvertGeminiResponseToClaudeStream_PartlessSafetyClosesMessageWithRefusal(t *testing.T) {
	requestJSON := []byte(`{"model":"claude-opus-5-5","messages":[{"role":"user","content":"hi"}]}`)
	chunk := []byte(`{
		"candidates": [{
			"content": {"role": "model", "parts": []},
			"index": 0,
			"finishReason": "SAFETY"
		}],
		"modelVersion": "m",
		"usageMetadata": {
			"promptTokenCount": 120,
			"candidatesTokenCount": 37,
			"totalTokenCount": 157
		}
	}`)

	var param any
	ctx := context.Background()
	output := bytes.Join(ConvertGeminiResponseToClaude(ctx, "claude-opus-5-5", requestJSON, requestJSON, chunk, &param), nil)
	output = append(output, bytes.Join(ConvertGeminiResponseToClaude(ctx, "claude-opus-5-5", requestJSON, requestJSON, []byte("[DONE]"), &param), nil)...)
	outputText := string(output)

	lastIndex := -1
	for _, eventName := range []string{"message_start", "content_block_start", "content_block_stop", "message_delta", "message_stop"} {
		index := strings.Index(outputText, "event: "+eventName+"\n")
		if index < 0 {
			t.Fatalf("event %q not found in output:\n%s", eventName, outputText)
		}
		if index <= lastIndex {
			t.Fatalf("event %q is out of order in output:\n%s", eventName, outputText)
		}
		lastIndex = index
	}

	var foundDelta bool
	for _, line := range strings.Split(outputText, "\n") {
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"type":"message_delta"`) {
			foundDelta = true
			deltaJSON := gjson.Parse(strings.TrimPrefix(line, "data: "))
			if got := deltaJSON.Get("delta.stop_reason").String(); got != "refusal" {
				t.Fatalf("stop_reason = %q, want refusal. Payload: %s", got, line)
			}
			if got := deltaJSON.Get("usage.input_tokens").Int(); got != 120 {
				t.Fatalf("input_tokens = %d, want 120. Payload: %s", got, line)
			}
			if got := deltaJSON.Get("usage.output_tokens").Int(); got != 37 {
				t.Fatalf("output_tokens = %d, want 37. Payload: %s", got, line)
			}
		}
	}
	if !foundDelta {
		t.Fatalf("failed to find message_delta event in output:\n%s", outputText)
	}
}

func TestConvertGeminiResponseToClaudeStream_PartlessMalformedFunctionCallClosesMessage(t *testing.T) {
	requestJSON := []byte(`{"model":"claude-opus-5-5","messages":[{"role":"user","content":"hi"}]}`)
	chunk := []byte(`{
		"candidates": [{
			"content": {"role": "model", "parts": []},
			"index": 0,
			"finishReason": "MALFORMED_FUNCTION_CALL"
		}],
		"modelVersion": "m",
		"usageMetadata": {
			"promptTokenCount": 50,
			"candidatesTokenCount": 10,
			"totalTokenCount": 60
		}
	}`)

	var param any
	ctx := context.Background()
	output := bytes.Join(ConvertGeminiResponseToClaude(ctx, "claude-opus-5-5", requestJSON, requestJSON, chunk, &param), nil)
	output = append(output, bytes.Join(ConvertGeminiResponseToClaude(ctx, "claude-opus-5-5", requestJSON, requestJSON, []byte("[DONE]"), &param), nil)...)
	outputText := string(output)

	if !strings.Contains(outputText, `"type":"message_delta"`) {
		t.Fatalf("expected message_delta in output:\n%s", outputText)
	}
	if !strings.Contains(outputText, `"type":"message_stop"`) {
		t.Fatalf("expected message_stop in output:\n%s", outputText)
	}

	var foundDelta bool
	for _, line := range strings.Split(outputText, "\n") {
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"type":"message_delta"`) {
			foundDelta = true
			deltaJSON := gjson.Parse(strings.TrimPrefix(line, "data: "))
			if got := deltaJSON.Get("delta.stop_reason").String(); got != "refusal" {
				t.Fatalf("stop_reason = %q, want refusal. Payload: %s", got, line)
			}
		}
	}
	if !foundDelta {
		t.Fatalf("failed to find message_delta in output:\n%s", outputText)
	}
}

func TestConvertGeminiResponseToClaudeStream_PartlessStopClosesMessageWithEndTurn(t *testing.T) {
	requestJSON := []byte(`{"model":"claude-opus-5-5","messages":[{"role":"user","content":"hi"}]}`)
	chunk := []byte(`{
		"candidates": [{
			"content": {"role": "model", "parts": [{"text": ""}]},
			"index": 0,
			"finishReason": "STOP"
		}],
		"modelVersion": "m",
		"usageMetadata": {
			"promptTokenCount": 80,
			"candidatesTokenCount": 0,
			"totalTokenCount": 80
		}
	}`)

	var param any
	ctx := context.Background()
	output := bytes.Join(ConvertGeminiResponseToClaude(ctx, "claude-opus-5-5", requestJSON, requestJSON, chunk, &param), nil)
	output = append(output, bytes.Join(ConvertGeminiResponseToClaude(ctx, "claude-opus-5-5", requestJSON, requestJSON, []byte("[DONE]"), &param), nil)...)
	outputText := string(output)

	lastIndex := -1
	for _, eventName := range []string{"message_start", "content_block_start", "content_block_stop", "message_delta", "message_stop"} {
		index := strings.Index(outputText, "event: "+eventName+"\n")
		if index < 0 {
			t.Fatalf("event %q not found in output:\n%s", eventName, outputText)
		}
		if index <= lastIndex {
			t.Fatalf("event %q is out of order in output:\n%s", eventName, outputText)
		}
		lastIndex = index
	}

	var foundDelta bool
	for _, line := range strings.Split(outputText, "\n") {
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"type":"message_delta"`) {
			foundDelta = true
			deltaJSON := gjson.Parse(strings.TrimPrefix(line, "data: "))
			if got := deltaJSON.Get("delta.stop_reason").String(); got != "end_turn" {
				t.Fatalf("stop_reason = %q, want end_turn. Payload: %s", got, line)
			}
		}
	}
	if !foundDelta {
		t.Fatalf("failed to find message_delta in output:\n%s", outputText)
	}
}

func TestConvertGeminiResponseToClaudeNonStream_SafetyAndMalformedFunctionCallRefusal(t *testing.T) {
	requestJSON := []byte(`{"model":"claude-opus-5-5","messages":[{"role":"user","content":"hi"}]}`)
	testCases := []struct {
		finishReason string
		wantReason   string
	}{
		{finishReason: "SAFETY", wantReason: "refusal"},
		{finishReason: "MALFORMED_FUNCTION_CALL", wantReason: "refusal"},
		{finishReason: "RECITATION", wantReason: "refusal"},
		{finishReason: "PROHIBITED_CONTENT", wantReason: "refusal"},
		{finishReason: "SPII", wantReason: "refusal"},
		{finishReason: "BLOCKLIST", wantReason: "refusal"},
		{finishReason: "MAX_TOKENS", wantReason: "max_tokens"},
		{finishReason: "STOP", wantReason: "end_turn"},
	}

	ctx := context.Background()
	for _, tc := range testCases {
		t.Run(tc.finishReason, func(t *testing.T) {
			rawJSON := []byte(`{
				"candidates": [{
					"content": {"role": "model", "parts": []},
					"finishReason": "` + tc.finishReason + `"
				}],
				"usageMetadata": {
					"promptTokenCount": 120,
					"candidatesTokenCount": 37,
					"totalTokenCount": 157
				},
				"modelVersion": "m",
				"responseId": "resp-test"
			}`)
			output := ConvertGeminiResponseToClaudeNonStream(ctx, "claude-opus-5-5", requestJSON, requestJSON, rawJSON, nil)
			outputJSON := gjson.ParseBytes(output)
			if got := outputJSON.Get("stop_reason").String(); got != tc.wantReason {
				t.Fatalf("stop_reason = %q, want %q. Output: %s", got, tc.wantReason, string(output))
			}
		})
	}
}
