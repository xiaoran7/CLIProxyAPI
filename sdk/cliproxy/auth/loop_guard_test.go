package auth

import (
	"fmt"
	"testing"
)

func TestLoopGuardNormalStream(t *testing.T) {
	guard := NewLoopGuard()
	normalChunks := []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"world. Today we are testing \"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"a brand new feature in CLIProxyAPI.\\n\"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"Everything looks great.\\n\"}}]}\n\n",
	}

	for _, chunk := range normalChunks {
		if trip, ok := guard.Feed([]byte(chunk)); ok {
			t.Fatalf("normal stream should not trip loop guard: %v", trip.Message())
		}
	}
}

func TestLoopGuardReasoningLoopTrip(t *testing.T) {
	guard := NewLoopGuard()
	patterns := []string{
		"Now analyzing the next step.\n",
		"Writing the implementation plan.\n",
		"Reviewing the code structure.\n",
	}

	tripped := false
	for i := 0; i < 300; i++ {
		text := patterns[i%len(patterns)]
		chunk := fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"reasoning_content\":%q}}]}\n\n", text)
		if trip, ok := guard.Feed([]byte(chunk)); ok {
			tripped = true
			if !trip.Reasoning {
				t.Fatalf("expected trip to be identified as reasoning, got: %v", trip)
			}
			if len(trip.Units) > loopDistinct {
				t.Fatalf("expected distinct units <= %d, got %d", loopDistinct, len(trip.Units))
			}
			break
		}
	}

	if !tripped {
		t.Fatal("expected reasoning loop to trip loop guard after 256 repetitive units")
	}
}

func TestLoopGuardAnthropicStreamLoopTrip(t *testing.T) {
	guard := NewLoopGuard()
	tripped := false
	for i := 0; i < 300; i++ {
		chunk := "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"Repeating thought step.\\n\"}}\n\n"
		if trip, ok := guard.Feed([]byte(chunk)); ok {
			tripped = true
			if !trip.Reasoning {
				t.Fatalf("expected Anthropic trip to be reasoning, got: %v", trip)
			}
			break
		}
	}

	if !tripped {
		t.Fatal("expected Anthropic stream loop to trip")
	}
}
