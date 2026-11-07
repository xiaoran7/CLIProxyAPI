package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

func TestHandleGetRequests_TokenRateCalculation(t *testing.T) {
	r, eng, _, _, cleanup := setupTestRouter(t)
	defer cleanup()

	ctx := context.Background()
	keyID := int64(1)
	ttftBurst := int64(7100)
	ttftNormal := int64(500)

	// 1. Antigravity Gemini 3.8 Flash High single-chunk tool-call stream (TTFT == Latency)
	err := eng.RecordRequest(ctx, &engine.RequestInfo{
		KeyID:            &keyID,
		Model:            "gemini-3.8-flash-high",
		Status:           200,
		PromptTokens:     56455,
		CompletionTokens: 93,
		TotalTokens:      56548,
		LatencyMs:        7100,
		TtftMs:           &ttftBurst,
		Stream:           true,
	})
	if err != nil {
		t.Fatalf("failed to record gemini tool call: %v", err)
	}

	// 2. Normal sustained streaming
	err = eng.RecordRequest(ctx, &engine.RequestInfo{
		KeyID:            &keyID,
		Model:            "claude-3-7-sonnet",
		Status:           200,
		PromptTokens:     1000,
		CompletionTokens: 100,
		TotalTokens:      1100,
		LatencyMs:        1500,
		TtftMs:           &ttftNormal,
		Stream:           true,
	})
	if err != nil {
		t.Fatalf("failed to record sustained stream: %v", err)
	}

	// 3. Request with zero completion tokens
	err = eng.RecordRequest(ctx, &engine.RequestInfo{
		KeyID:            &keyID,
		Model:            "gpt-4o",
		Status:           500,
		PromptTokens:     500,
		CompletionTokens: 0,
		TotalTokens:      500,
		LatencyMs:        300,
		Stream:           false,
	})
	if err != nil {
		t.Fatalf("failed to record zero tokens: %v", err)
	}

	// Flush usage to SQLite
	_ = eng.Flusher().FlushUsage(ctx)

	req := httptest.NewRequest(http.MethodGet, "/console/api/requests", nil)
	req.Header.Set("Authorization", "Bearer test-secret-12345")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Events []struct {
			Model        string  `json:"model"`
			TokensPerSec float64 `json:"tokens_per_sec"`
		} `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	foundGemini := false
	foundClaude := false
	foundGPT := false

	for _, ev := range resp.Events {
		switch ev.Model {
		case "gemini-3.8-flash-high":
			foundGemini = true
			if ev.TokensPerSec != 13.1 {
				t.Errorf("expected 13.1 tok/s fallback for gemini tool call, got %f", ev.TokensPerSec)
			}
		case "claude-3-7-sonnet":
			foundClaude = true
			if ev.TokensPerSec != 100.0 {
				t.Errorf("expected 100.0 tok/s for sustained stream, got %f", ev.TokensPerSec)
			}
		case "gpt-4o":
			foundGPT = true
			if ev.TokensPerSec != 0 {
				t.Errorf("expected 0 tok/s for 0 completion tokens, got %f", ev.TokensPerSec)
			}
		}
	}

	if !foundGemini {
		t.Errorf("gemini-3.8-flash-high event not found in response")
	}
	if !foundClaude {
		t.Errorf("claude-3-7-sonnet event not found in response")
	}
	if !foundGPT {
		t.Errorf("gpt-4o event not found in response")
	}
}

func TestHandleGetRequests_ClientIPAndGeo(t *testing.T) {
	r, eng, _, _, cleanup := setupTestRouter(t)
	defer cleanup()

	ctx := context.Background()
	keyID := int64(1)

	err := eng.RecordRequest(ctx, &engine.RequestInfo{
		KeyID:            &keyID,
		Model:            "gemini-3.8-flash-high",
		Status:           200,
		PromptTokens:     100,
		CompletionTokens: 50,
		LatencyMs:        500,
		ClientIP:         "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("failed to record request: %v", err)
	}

	_ = eng.Flusher().FlushUsage(ctx)

	req := httptest.NewRequest(http.MethodGet, "/console/api/requests", nil)
	req.Header.Set("Authorization", "Bearer test-secret-12345")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Events []struct {
			ClientIP string `json:"client_ip"`
			Geo      *struct {
				Country string `json:"country"`
			} `json:"geo"`
		} `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	if len(resp.Events) == 0 {
		t.Fatalf("expected at least 1 event")
	}
	if resp.Events[0].ClientIP != "127.0.0.1" {
		t.Errorf("expected client_ip 127.0.0.1, got %s", resp.Events[0].ClientIP)
	}
	if resp.Events[0].Geo == nil || resp.Events[0].Geo.Country == "" {
		t.Errorf("expected non-nil Geo with Country, got %+v", resp.Events[0].Geo)
	}
}
