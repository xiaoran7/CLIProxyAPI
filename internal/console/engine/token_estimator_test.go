package engine

import (
	"testing"
)

func TestExtractUsageFromJSON_OpenAI(t *testing.T) {
	raw := []byte(`{
		"usage": {
			"prompt_tokens": 120,
			"completion_tokens": 80,
			"total_tokens": 200,
			"prompt_tokens_details": {
				"cached_tokens": 40
			}
		}
	}`)

	prompt, completion, cached, total, ok := ExtractUsageFromJSON(raw)
	if !ok {
		t.Fatalf("expected usage extracted")
	}
	if prompt != 120 || completion != 80 || cached != 40 || total != 200 {
		t.Errorf("unexpected tokens: p=%d, c=%d, cached=%d, t=%d", prompt, completion, cached, total)
	}
}

func TestExtractUsageFromJSON_Anthropic(t *testing.T) {
	raw := []byte(`{
		"usage": {
			"input_tokens": 50,
			"output_tokens": 150,
			"cache_read_input_tokens": 100
		}
	}`)

	prompt, completion, cached, total, ok := ExtractUsageFromJSON(raw)
	if !ok {
		t.Fatalf("expected usage extracted")
	}
	// input_tokens (50) + cache_read (100) = 150 total prompt
	if prompt != 150 || completion != 150 || cached != 100 || total != 300 {
		t.Errorf("unexpected anthropic tokens: p=%d, c=%d, cached=%d, t=%d", prompt, completion, cached, total)
	}
}

func TestExtractUsageFromJSON_Gemini(t *testing.T) {
	raw := []byte(`{
		"usageMetadata": {
			"promptTokenCount": 200,
			"candidatesTokenCount": 100,
			"totalTokenCount": 300,
			"cachedContentTokenCount": 50
		}
	}`)

	prompt, completion, cached, total, ok := ExtractUsageFromJSON(raw)
	if !ok {
		t.Fatalf("expected usage extracted")
	}
	if prompt != 200 || completion != 100 || cached != 50 || total != 300 {
		t.Errorf("unexpected gemini tokens: p=%d, c=%d, cached=%d, t=%d", prompt, completion, cached, total)
	}
}

func TestExtractUsageFromJSON_RootTokens(t *testing.T) {
	raw := []byte(`{"input_tokens": 128, "output_tokens": 64}`)
	prompt, completion, cached, total, ok := ExtractUsageFromJSON(raw)
	if !ok {
		t.Fatalf("expected usage extracted from root tokens")
	}
	if prompt != 128 || completion != 64 || total != 192 || cached != 0 {
		t.Errorf("unexpected root tokens: p=%d, c=%d, cached=%d, t=%d", prompt, completion, cached, total)
	}
}

func TestCalculateTokensPerSec(t *testing.T) {
	// 1. Non-stream: 100 tokens, 1000ms latency -> 100 tok/s
	tps := CalculateTokensPerSec(100, 1000, 0, false)
	if tps != 100.0 {
		t.Errorf("expected 100.0, got %f", tps)
	}

	// 2. Normal stream with TTFT: 100 tokens, 1500ms latency, 500ms TTFT -> generation window = 1000ms -> 100 tok/s
	tpsStream := CalculateTokensPerSec(100, 1500, 500, true)
	if tpsStream != 100.0 {
		t.Errorf("expected 100.0, got %f", tpsStream)
	}

	// 3. Realistic sustained streaming: 1234 tokens, 15800ms latency, 5100ms TTFT -> 10700ms -> 115.3 tok/s
	tpsRealStream := CalculateTokensPerSec(1234, 15800, 5100, true)
	if tpsRealStream != 115.3 {
		t.Errorf("expected 115.3, got %f", tpsRealStream)
	}

	// 4. Realistic fast streaming: 106 tokens, 4600ms latency, 4132ms TTFT -> 468ms -> 226.5 tok/s
	tpsFastStream := CalculateTokensPerSec(106, 4600, 4132, true)
	if tpsFastStream != 226.5 {
		t.Errorf("expected 226.5, got %f", tpsFastStream)
	}

	// 5. Short burst window (<100ms): 132 tokens, 9400ms latency, 9330ms TTFT -> genMs = 70ms -> falls back to end-to-end: 132 / 9.4s = 14.0 tok/s
	tpsBurstShort := CalculateTokensPerSec(132, 9400, 9330, true)
	if tpsBurstShort != 14.0 {
		t.Errorf("expected 14.0 for burst delivery < 100ms fallback, got %f", tpsBurstShort)
	}

	// 6. Implausible burst rate (>500 tok/s): 217 tokens, 22200ms latency, 22056ms TTFT -> genMs = 144ms -> rate > 500 -> falls back to end-to-end: 217 / 22.2s = 9.8 tok/s
	tpsBurstHighRate := CalculateTokensPerSec(217, 22200, 22056, true)
	if tpsBurstHighRate != 9.8 {
		t.Errorf("expected 9.8 for implausible burst rate fallback, got %f", tpsBurstHighRate)
	}

	// 7. Gemini single-chunk / tool-call scenario (TTFT == total latency): 93 tokens, 7100ms latency, 7100ms TTFT -> 93 / 7.1s = 13.1 tok/s
	tpsGeminiBurst := CalculateTokensPerSec(93, 7100, 7100, true)
	if tpsGeminiBurst != 13.1 {
		t.Errorf("expected 13.1 for single-chunk tool-call stream, got %f", tpsGeminiBurst)
	}

	// 8. Missing or invalid TTFT in stream: gracefully falls back to end-to-end throughput
	if got := CalculateTokensPerSec(100, 1000, 0, true); got != 100.0 {
		t.Errorf("expected 100.0 for stream without TTFT fallback, got %f", got)
	}
	if got := CalculateTokensPerSec(100, 1000, 1000, true); got != 100.0 {
		t.Errorf("expected 100.0 for stream with TTFT >= latency fallback, got %f", got)
	}

	// 9. Zero or negative completion tokens or latency
	if got := CalculateTokensPerSec(0, 1000, 500, true); got != 0 {
		t.Errorf("expected 0 for 0 completion tokens, got %f", got)
	}
	if got := CalculateTokensPerSec(100, 0, 0, false); got != 0 {
		t.Errorf("expected 0 for 0 latency, got %f", got)
	}
}

func TestExtractReasoningEffort(t *testing.T) {
	tests := []struct {
		json     string
		expected string
	}{
		{`{"reasoning_effort":"high"}`, "high"},
		{`{"reasoning":{"effort":"medium"}}`, "medium"},
		{`{"output_config":{"effort":"low"}}`, "low"},
		{`{"model":"gemini-2.5-flash-high"}`, "high"},
	}

	for _, tc := range tests {
		actual := ExtractReasoningEffort([]byte(tc.json))
		if actual != tc.expected {
			t.Errorf("ExtractReasoningEffort(%s) = %q, expected %q", tc.json, actual, tc.expected)
		}
	}
}
