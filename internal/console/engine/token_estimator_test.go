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

func TestCalculateTokensPerSec(t *testing.T) {
	// 100 tokens, 1000ms latency -> 100 tok/s
	tps := CalculateTokensPerSec(100, 1000, 0, false)
	if tps != 100.0 {
		t.Errorf("expected 100.0, got %f", tps)
	}

	// Stream with TTFT: 100 tokens, 1500ms latency, 500ms TTFT -> generation window = 1000ms -> 100 tok/s
	tpsStream := CalculateTokensPerSec(100, 1500, 500, true)
	if tpsStream != 100.0 {
		t.Errorf("expected 100.0, got %f", tpsStream)
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
