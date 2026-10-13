package auth

import (
	"context"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

func TestSmartSelector_SoonestResetFirst(t *testing.T) {
	authA := &Auth{
		ID:       "auth-reset-later",
		Provider: "claude",
		Status:   StatusActive,
		Quota: QuotaState{
			Signals: map[string]string{
				"anthropic-ratelimit-unified-5h-used-percent": "20.0",
				"retry-after": "21600", // resets in 6 hours
			},
		},
	}
	authB := &Auth{
		ID:       "auth-reset-soon",
		Provider: "claude",
		Status:   StatusActive,
		Quota: QuotaState{
			Signals: map[string]string{
				"anthropic-ratelimit-unified-5h-used-percent": "30.0",
				"retry-after": "7200", // resets in 2 hours
			},
		},
	}

	selector := &SmartSelector{}
	picked, err := selector.Pick(context.Background(), "claude", "claude-sonnet-4-6", cliproxyexecutor.Options{}, []*Auth{authA, authB})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if picked.ID != "auth-reset-soon" {
		t.Fatalf("expected auth-reset-soon to be picked, got %s", picked.ID)
	}
}

func TestSmartSelector_TieredFallback(t *testing.T) {
	authFine := &Auth{
		ID:       "auth-fine",
		Provider: "codex",
		Status:   StatusActive,
		Quota: QuotaState{
			Signals: map[string]string{
				"x-codex-primary-used-percent": "40.0",
			},
		},
	}
	authLow := &Auth{
		ID:       "auth-low",
		Provider: "codex",
		Status:   StatusActive,
		Quota: QuotaState{
			Signals: map[string]string{
				"x-codex-primary-used-percent": "85.0",
			},
		},
	}
	authSpent := &Auth{
		ID:       "auth-spent",
		Provider: "codex",
		Status:   StatusActive,
		Quota: QuotaState{
			Signals: map[string]string{
				"x-codex-primary-used-percent": "99.0",
			},
		},
	}

	selector := &SmartSelector{}
	// All three present -> picks fine
	picked, err := selector.Pick(context.Background(), "codex", "gpt-5.5", cliproxyexecutor.Options{}, []*Auth{authSpent, authLow, authFine})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if picked.ID != "auth-fine" {
		t.Fatalf("expected auth-fine, got %s", picked.ID)
	}

	// Only low and spent -> picks low
	picked2, err2 := selector.Pick(context.Background(), "codex", "gpt-5.5", cliproxyexecutor.Options{}, []*Auth{authSpent, authLow})
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if picked2.ID != "auth-low" {
		t.Fatalf("expected auth-low, got %s", picked2.ID)
	}
}

func TestPaceSelector_HighestHourlyPace(t *testing.T) {
	// Account A: 60% remaining, resets in 3 hours -> pace = 0.60 / 3 = 0.20
	authA := &Auth{
		ID:       "auth-fast-pace",
		Provider: "codex",
		Status:   StatusActive,
		Quota: QuotaState{
			Signals: map[string]string{
				"x-codex-primary-used-percent": "40.0",
				"retry-after":                  "10800", // 3h
			},
		},
	}
	// Account B: 80% remaining, resets in 10 hours -> pace = 0.80 / 10 = 0.08
	authB := &Auth{
		ID:       "auth-slow-pace",
		Provider: "codex",
		Status:   StatusActive,
		Quota: QuotaState{
			Signals: map[string]string{
				"x-codex-primary-used-percent": "20.0",
				"retry-after":                  "36000", // 10h
			},
		},
	}

	selector := &PaceSelector{}
	picked, err := selector.Pick(context.Background(), "codex", "gpt-5.5", cliproxyexecutor.Options{}, []*Auth{authB, authA})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if picked.ID != "auth-fast-pace" {
		t.Fatalf("expected auth-fast-pace (pace 0.20 > 0.08), got %s", picked.ID)
	}
}

func TestPaceSelector_PaceToleranceBand(t *testing.T) {
	// Account A: pace = 0.50 / 5 = 0.100
	authA := &Auth{
		ID:       "auth-first",
		Provider: "codex",
		Status:   StatusActive,
		Quota: QuotaState{
			Signals: map[string]string{
				"x-codex-primary-used-percent": "50.0",
				"retry-after":                  "18000", // 5h
			},
		},
	}
	// Account B: pace = 0.52 / 5 = 0.104 (within 4% difference, <= 10% band)
	authB := &Auth{
		ID:       "auth-second",
		Provider: "codex",
		Status:   StatusActive,
		Quota: QuotaState{
			Signals: map[string]string{
				"x-codex-primary-used-percent": "48.0",
				"retry-after":                  "18000", // 5h
			},
		},
	}

	selector := &PaceSelector{}
	// Original order [authA, authB] should be preserved due to <=10% pace tolerance band
	picked, err := selector.Pick(context.Background(), "codex", "gpt-5.5", cliproxyexecutor.Options{}, []*Auth{authA, authB})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if picked.ID != "auth-first" {
		t.Fatalf("expected auth-first to be kept due to cache stability band, got %s", picked.ID)
	}
}

func TestPaceSmartSelector_RateLimitSinkIntegration(t *testing.T) {
	authA := &Auth{
		ID:       "auth-sunk",
		Provider: "codex",
		Status:   StatusActive,
		Quota: QuotaState{
			Signals: map[string]string{
				"x-codex-primary-used-percent": "10.0", // lower usage
				"retry-after":                  "3600",
			},
		},
	}
	authB := &Auth{
		ID:       "auth-clean",
		Provider: "codex",
		Status:   StatusActive,
		Quota: QuotaState{
			Signals: map[string]string{
				"x-codex-primary-used-percent": "30.0",
				"retry-after":                  "7200",
			},
		},
	}

	// Sink authA
	RecordRateLimitSink(authA.ID, time.Now())

	smartSel := &SmartSelector{}
	pickedSmart, errSmart := smartSel.Pick(context.Background(), "codex", "gpt-5.5", cliproxyexecutor.Options{}, []*Auth{authA, authB})
	if errSmart != nil {
		t.Fatalf("unexpected error: %v", errSmart)
	}
	if pickedSmart.ID != "auth-clean" {
		t.Fatalf("expected auth-clean because auth-sunk is sunk, got %s", pickedSmart.ID)
	}

	paceSel := &PaceSelector{}
	pickedPace, errPace := paceSel.Pick(context.Background(), "codex", "gpt-5.5", cliproxyexecutor.Options{}, []*Auth{authA, authB})
	if errPace != nil {
		t.Fatalf("unexpected error: %v", errPace)
	}
	if pickedPace.ID != "auth-clean" {
		t.Fatalf("expected auth-clean because auth-sunk is sunk, got %s", pickedPace.ID)
	}
}
