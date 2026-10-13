package auth

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

func TestWrapStreamResultLoopGuardTrips(t *testing.T) {
	m := &Manager{}
	auth := &Auth{ID: "test-auth", Provider: "openai"}

	inCh := make(chan cliproxyexecutor.StreamChunk, 400)
	// Push 300 repeating reasoning chunks
	for i := 0; i < 300; i++ {
		payload := fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"Repetitive step %d.\\n\"}}]}\n\n", i%3)
		inCh <- cliproxyexecutor.StreamChunk{Payload: []byte(payload)}
	}
	close(inCh)

	res := m.wrapStreamResult(
		context.Background(),
		auth,
		"openai",
		"gpt-test",
		"gpt-test",
		http.Header{},
		nil,
		inCh,
		OAuthModelAliasResult{},
		true,
		cliproxyexecutor.Options{},
	)

	var lastChunk cliproxyexecutor.StreamChunk
	count := 0
	for chunk := range res.Chunks {
		count++
		lastChunk = chunk
	}

	if lastChunk.Err == nil {
		t.Fatal("expected wrapStreamResult to terminate stream with loop error, but got nil Err")
	}
	err, ok := lastChunk.Err.(*Error)
	if !ok || err.Code != "stream_loop_detected" {
		t.Fatalf("expected Error with Code 'stream_loop_detected', got: %v", lastChunk.Err)
	}
}
