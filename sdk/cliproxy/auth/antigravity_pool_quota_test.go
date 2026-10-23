package auth

import (
	"testing"
	"time"
)

func TestClassifyAntigravityModelPool(t *testing.T) {
	tests := []struct {
		model    string
		expected antigravityQuotaPool
	}{
		{"gemini-2.5-flash", antigravityPoolGeminiStandard},
		{"gemini-2.5-pro", antigravityPoolGeminiStandard},
		{"gemini-3.8-flash", antigravityPoolGeminiStandard},
		{"gemini-3.8-pro", antigravityPoolGeminiStandard},
		{"gemini-4-argon", antigravityPoolGemini4Argon},
		{"gemini-4-argon-low", antigravityPoolGemini4Argon},
		{"gemini-4-argon-medium", antigravityPoolGemini4Argon},
		{"gemini-4-argon-high", antigravityPoolGemini4Argon},
		{"gemini-4-high", antigravityPoolGemini4Argon},
		{"claude-3-5-sonnet", antigravityPoolClaudeGPT},
		{"claude-3-7-sonnet", antigravityPoolClaudeGPT},
		{"claude-3-opus", antigravityPoolClaudeGPT},
		{"gpt-4o", antigravityPoolClaudeGPT},
		{"gpt-4-turbo", antigravityPoolClaudeGPT},
		{"unknown-model-xyz", antigravityPoolUnknown},
	}

	for _, tc := range tests {
		actual := classifyAntigravityModelPool(tc.model)
		if actual != tc.expected {
			t.Errorf("classifyAntigravityModelPool(%q) = %d, want %d", tc.model, actual, tc.expected)
		}
	}
}

func TestAntigravityQuotaPoolRouting(t *testing.T) {
	now := time.Now()
	retryAfter := now.Add(18 * time.Hour)

	// Create an Antigravity auth where gemini-2.5-flash has hit quota exhaustion
	agAuth := &Auth{
		ID:       "ag-test-1",
		Provider: "antigravity",
		Status:   StatusActive,
		ModelStates: map[string]*ModelState{
			"gemini-2.5-flash": {
				Status:         StatusError,
				Unavailable:    true,
				NextRetryAfter: retryAfter,
				Quota: QuotaState{
					Exceeded:      true,
					Reason:        "quota",
					NextRecoverAt: retryAfter,
				},
			},
		},
	}

	// 1. Calling standard Gemini models should be blocked
	blocked, reason, next := isAuthBlockedForModel(agAuth, "gemini-2.5-flash", now)
	if !blocked || reason != blockReasonCooldown {
		t.Fatalf("expected gemini-2.5-flash to be blocked by cooldown, got blocked=%v, reason=%v", blocked, reason)
	}
	if !next.Equal(retryAfter) {
		t.Fatalf("expected next retry %v, got %v", retryAfter, next)
	}

	// Calling another standard Gemini model not explicitly in ModelStates should also be blocked
	blockedPro, reasonPro, _ := isAuthBlockedForModel(agAuth, "gemini-2.5-pro", now)
	if !blockedPro || reasonPro != blockReasonCooldown {
		t.Fatalf("expected gemini-2.5-pro (same pool) to be blocked by cooldown, got blocked=%v, reason=%v", blockedPro, reasonPro)
	}

	// 2. Calling Gemini 4 Argon should NOT be blocked
	blockedArgon, _, _ := isAuthBlockedForModel(agAuth, "gemini-4-argon-high", now)
	if blockedArgon {
		t.Fatalf("expected gemini-4-argon-high to NOT be blocked, but was blocked")
	}

	// 3. Calling Claude or GPT should NOT be blocked
	blockedClaude, _, _ := isAuthBlockedForModel(agAuth, "claude-3-7-sonnet", now)
	if blockedClaude {
		t.Fatalf("expected claude-3-7-sonnet to NOT be blocked, but was blocked")
	}

	blockedGPT, _, _ := isAuthBlockedForModel(agAuth, "gpt-4o", now)
	if blockedGPT {
		t.Fatalf("expected gpt-4o to NOT be blocked, but was blocked")
	}

	// 4. Test isAuthBlockedByQuotaOrAuthForModel consistency
	if !isAuthBlockedByQuotaOrAuthForModel(agAuth, "gemini-2.5-flash", now) {
		t.Fatalf("expected gemini-2.5-flash to be blocked in isAuthBlockedByQuotaOrAuthForModel")
	}
	if !isAuthBlockedByQuotaOrAuthForModel(agAuth, "gemini-2.5-pro", now) {
		t.Fatalf("expected gemini-2.5-pro to be blocked in isAuthBlockedByQuotaOrAuthForModel")
	}
	if isAuthBlockedByQuotaOrAuthForModel(agAuth, "gemini-4-argon-high", now) {
		t.Fatalf("expected gemini-4-argon-high to NOT be blocked in isAuthBlockedByQuotaOrAuthForModel")
	}
	if isAuthBlockedByQuotaOrAuthForModel(agAuth, "claude-3-7-sonnet", now) {
		t.Fatalf("expected claude-3-7-sonnet to NOT be blocked in isAuthBlockedByQuotaOrAuthForModel")
	}
}

func TestAntigravityUpdateAggregatedAvailability(t *testing.T) {
	now := time.Now()
	retryAfter := now.Add(18 * time.Hour)

	// Auth with only standard Gemini in cooldown
	agAuth := &Auth{
		ID:       "ag-test-2",
		Provider: "antigravity",
		Status:   StatusActive,
		ModelStates: map[string]*ModelState{
			"gemini-2.5-flash": {
				Status:         StatusError,
				Unavailable:    true,
				NextRetryAfter: retryAfter,
				Quota: QuotaState{
					Exceeded:      true,
					Reason:        "quota",
					NextRecoverAt: retryAfter,
				},
			},
		},
	}

	updateAggregatedAvailability(agAuth, now)

	// Credential itself should NOT be allUnavailable
	if agAuth.Unavailable {
		t.Fatalf("expected agAuth.Unavailable to be false when only standard Gemini pool is exhausted, got true")
	}
	if agAuth.Quota.Reason == "credential_quota" {
		t.Fatalf("expected agAuth.Quota.Reason not to be credential_quota")
	}

	// Now add cooldown for all 3 pools
	agAuth.ModelStates["gemini-4-argon"] = &ModelState{
		Status:         StatusError,
		Unavailable:    true,
		NextRetryAfter: retryAfter,
		Quota: QuotaState{
			Exceeded:      true,
			Reason:        "quota",
			NextRecoverAt: retryAfter,
		},
	}
	agAuth.ModelStates["claude-3-7-sonnet"] = &ModelState{
		Status:         StatusError,
		Unavailable:    true,
		NextRetryAfter: retryAfter,
		Quota: QuotaState{
			Exceeded:      true,
			Reason:        "quota",
			NextRecoverAt: retryAfter,
		},
	}

	updateAggregatedAvailability(agAuth, now)

	// When all 3 pools have failed, credential should be allUnavailable
	if !agAuth.Unavailable {
		t.Fatalf("expected agAuth.Unavailable to be true when all 3 pools are exhausted, got false")
	}
}
