package auth

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

func TestWrapStreamResultSSEKeepAlivePumping(t *testing.T) {
	m := &Manager{}
	auth := &Auth{ID: "test-auth", Provider: "openai"}

	headers := http.Header{}
	headers.Set("Content-Type", "text/event-stream")

	inCh := make(chan cliproxyexecutor.StreamChunk)
	// We do NOT send immediate chunks, simulating slow upstream thinking
	go func() {
		// Wait longer than tick would take if we used a short interval,
		// or send a chunk then finish.
		time.Sleep(50 * time.Millisecond)
		inCh <- cliproxyexecutor.StreamChunk{Payload: []byte("data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n")}
		close(inCh)
	}()

	res := m.wrapStreamResult(
		context.Background(),
		auth,
		"openai",
		"gpt-test",
		"gpt-test",
		headers,
		nil,
		inCh,
		OAuthModelAliasResult{},
		true,
		cliproxyexecutor.Options{},
	)

	var chunks []cliproxyexecutor.StreamChunk
	for chunk := range res.Chunks {
		chunks = append(chunks, chunk)
	}

	if len(chunks) == 0 {
		t.Fatal("expected at least 1 chunk, got 0")
	}

	lastChunk := chunks[len(chunks)-1]
	if !bytes.Contains(lastChunk.Payload, []byte("done")) {
		t.Fatalf("expected last chunk to contain 'done', got: %s", string(lastChunk.Payload))
	}
}

func TestIsSSEStreamDetection(t *testing.T) {
	// 1. Detected via header
	h := http.Header{}
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	if !isSSEStream(h, nil) {
		t.Fatal("expected isSSEStream true from Content-Type")
	}

	// 2. Detected via buffered data: prefix
	h2 := http.Header{}
	buffered := []cliproxyexecutor.StreamChunk{
		{Payload: []byte("data: {\"text\":\"hi\"}\n\n")},
	}
	if !isSSEStream(h2, buffered) {
		t.Fatal("expected isSSEStream true from buffered chunks")
	}

	// 3. Plain json is not SSE
	h3 := http.Header{}
	h3.Set("Content-Type", "application/json")
	if isSSEStream(h3, nil) {
		t.Fatal("expected isSSEStream false for application/json")
	}
}
