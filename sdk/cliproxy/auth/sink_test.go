package auth

import (
	"testing"
	"time"
)

func TestApplySinkOrdering(t *testing.T) {
	ResetRateLimitSinks()
	defer ResetRateLimitSinks()

	now := time.Now()
	a := &Auth{ID: "auth-1"}
	b := &Auth{ID: "auth-2"}
	c := &Auth{ID: "auth-3"}

	// 1. Initial state: none sunk, preserves original order
	initial := []*Auth{a, b, c}
	ordered := ApplySinkOrdering(initial)
	if ordered[0].ID != "auth-1" || ordered[1].ID != "auth-2" || ordered[2].ID != "auth-3" {
		t.Fatalf("unexpected order when none sunk: %v", ordered)
	}

	// 2. Auth-1 experiences 429 rate limit and sinks
	RecordRateLimitSink("auth-1", now)
	sunk1 := ApplySinkOrdering(initial)
	if sunk1[0].ID != "auth-2" || sunk1[1].ID != "auth-3" || sunk1[2].ID != "auth-1" {
		t.Fatalf("auth-1 should sink to bottom: got %v, want auth-2, auth-3, auth-1", sunk1)
	}

	// 3. Auth-2 also experiences 429 and sinks after auth-1
	RecordRateLimitSink("auth-2", now.Add(time.Second))
	sunk2 := ApplySinkOrdering(initial)
	// auth-3 was never sunk, so it is first; auth-1 was sunk before auth-2, so auth-1 is before auth-2
	if sunk2[0].ID != "auth-3" || sunk2[1].ID != "auth-1" || sunk2[2].ID != "auth-2" {
		t.Fatalf("auth-3 should be first, followed by auth-1 then auth-2: got %v", sunk2)
	}

	// 4. Auth-3 also sinks
	RecordRateLimitSink("auth-3", now.Add(2*time.Second))
	sunk3 := ApplySinkOrdering(initial)
	// All three sunk: ordered by sink sequence (auth-1, auth-2, auth-3)
	if sunk3[0].ID != "auth-1" || sunk3[1].ID != "auth-2" || sunk3[2].ID != "auth-3" {
		t.Fatalf("all sunk: should cycle back in sequence order: got %v", sunk3)
	}

	// 5. Clear sink for auth-2
	ClearRateLimitSink("auth-2")
	cleared := ApplySinkOrdering(initial)
	if cleared[0].ID != "auth-2" {
		t.Fatalf("cleared auth-2 should float to front: got %v", cleared)
	}
}
