package auth

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

const (
	poolCooldownProvider = "pool-cd"
	poolCooldownModel    = "pool-cd-model"
)

// newPoolCooldownManager builds a manager holding one credential per id, all
// bound to a provider that has a registered executor and a model that the
// global registry knows about.
func newPoolCooldownManager(t *testing.T, authIDs ...string) *Manager {
	t.Helper()
	manager := NewManager(nil, nil, nil)
	manager.RegisterExecutor(&aliasRoutingExecutor{id: poolCooldownProvider})
	for _, id := range authIDs {
		if _, errRegister := manager.Register(WithSkipPersist(context.Background()), &Auth{
			ID:       id,
			Provider: poolCooldownProvider,
			Metadata: map[string]any{"type": poolCooldownProvider},
		}); errRegister != nil {
			t.Fatalf("Register(%s) error = %v", id, errRegister)
		}
	}
	registerSchedulerModels(t, poolCooldownProvider, poolCooldownModel, authIDs...)
	return manager
}

// coolPoolCooldownModel puts the credential into a quota cooldown for the
// pooled model, the way an upstream 429 does.
func coolPoolCooldownModel(t *testing.T, manager *Manager, authID string) {
	t.Helper()
	manager.MarkResult(context.Background(), Result{
		AuthID:   authID,
		Provider: poolCooldownProvider,
		Model:    poolCooldownModel,
		Success:  false,
		Error: &Error{
			Code:       "rate_limit",
			Message:    "quota",
			Retryable:  true,
			HTTPStatus: http.StatusTooManyRequests,
		},
	})
}

func TestPoolCooldownTerminalErrorPreservesUpstreamSignal(t *testing.T) {
	upstream := &Error{Code: "rate_limit", Message: "quota", HTTPStatus: http.StatusTooManyRequests}
	err := &poolCooldownTerminalError{cause: upstream, deadline: time.Now().Add(90 * time.Second)}

	if !err.IsModelCooldown() {
		t.Fatal("IsModelCooldown() = false, want true")
	}
	if got := err.StatusCode(); got != http.StatusTooManyRequests {
		t.Fatalf("StatusCode() = %d, want %d", got, http.StatusTooManyRequests)
	}
	remaining := err.RetryAfter()
	if remaining == nil || *remaining <= 0 || *remaining > 90*time.Second {
		t.Fatalf("RetryAfter() = %v, want a positive remaining time", remaining)
	}
	if header := err.SafeResponseHeaders().Get("Retry-After"); header == "" {
		t.Fatal("SafeResponseHeaders() carries no Retry-After")
	}
	var base *Error
	if !errors.As(err, &base) || base != upstream {
		t.Fatalf("errors.As did not expose the wrapped error, got %v", base)
	}
	if err.Error() != upstream.Error() {
		t.Fatalf("Error() = %q, want the upstream text %q", err.Error(), upstream.Error())
	}
	if !HasTrustedRetryDeadline(err) {
		t.Fatal("HasTrustedRetryDeadline() = false, want true")
	}
}

func TestPoolCooldownTerminalErrorWithoutDeadlineIsNotAModelCooldown(t *testing.T) {
	err := &poolCooldownTerminalError{cause: &Error{HTTPStatus: http.StatusTooManyRequests}}
	if err.IsModelCooldown() {
		t.Fatal("IsModelCooldown() = true, want false without a deadline")
	}
	if err.RetryAfter() != nil {
		t.Fatalf("RetryAfter() = %v, want nil", err.RetryAfter())
	}
}

func TestAnnotatePoolCooldownTerminalErrorWrapsTerminal429WhenPoolIsCooling(t *testing.T) {
	withQuotaCooldownEnabled(t)
	manager := newPoolCooldownManager(t, "pool-cd-auth-1", "pool-cd-auth-2")
	coolPoolCooldownModel(t, manager, "pool-cd-auth-1")
	coolPoolCooldownModel(t, manager, "pool-cd-auth-2")

	upstream := &Error{Code: "rate_limit", Message: "quota", HTTPStatus: http.StatusTooManyRequests}
	got := manager.annotatePoolCooldownTerminalErrorAt(
		context.Background(),
		[]string{poolCooldownProvider},
		poolCooldownModel,
		cliproxyexecutor.Options{},
		upstream,
		time.Time{},
		time.Now(),
	)

	var wrapped *poolCooldownTerminalError
	if !errors.As(got, &wrapped) {
		t.Fatalf("the terminal 429 was not wrapped, got %T: %v", got, got)
	}
	if !wrapped.IsModelCooldown() {
		t.Fatal("the wrapped error is not marked as a model cooldown")
	}
	if wrapped.StatusCode() != http.StatusTooManyRequests {
		t.Fatalf("StatusCode() = %d, want %d", wrapped.StatusCode(), http.StatusTooManyRequests)
	}
	if remaining := wrapped.RetryAfter(); remaining == nil || *remaining <= 0 {
		t.Fatalf("RetryAfter() = %v, want a positive remaining time", remaining)
	}
}

func TestAnnotatePoolCooldownTerminalErrorLeavesAvailablePoolAlone(t *testing.T) {
	withQuotaCooldownEnabled(t)
	manager := newPoolCooldownManager(t, "pool-cd-auth-1", "pool-cd-auth-2")
	coolPoolCooldownModel(t, manager, "pool-cd-auth-1")

	upstream := &Error{Code: "rate_limit", Message: "quota", HTTPStatus: http.StatusTooManyRequests}
	got := manager.annotatePoolCooldownTerminalErrorAt(
		context.Background(),
		[]string{poolCooldownProvider},
		poolCooldownModel,
		cliproxyexecutor.Options{},
		upstream,
		time.Time{},
		time.Now(),
	)

	var wrapped *poolCooldownTerminalError
	if errors.As(got, &wrapped) {
		t.Fatal("the terminal 429 was wrapped although a credential is still available")
	}
	if got != upstream {
		t.Fatalf("the error was replaced: got %v, want the upstream error", got)
	}
}

func TestAnnotatePoolCooldownTerminalErrorIgnoresNonRateLimit(t *testing.T) {
	manager := newPoolCooldownManager(t, "pool-cd-auth-1")
	coolPoolCooldownModel(t, manager, "pool-cd-auth-1")

	upstream := &Error{Code: "forbidden", Message: "denied", HTTPStatus: http.StatusForbidden}
	got := manager.annotatePoolCooldownTerminalErrorAt(
		context.Background(),
		[]string{poolCooldownProvider},
		poolCooldownModel,
		cliproxyexecutor.Options{},
		upstream,
		time.Time{},
		time.Now(),
	)
	if got != upstream {
		t.Fatalf("the error was replaced: got %v, want the upstream error", got)
	}
}

func TestAnnotatePoolCooldownTerminalErrorFallsBackToSelectionDeadline(t *testing.T) {
	// The fresh-pool variant: the credential scan finds no timed cooldown to
	// report, but selection already observed the pool reset, so that deadline
	// wins.
	manager := NewManager(nil, nil, nil)
	manager.RegisterExecutor(&aliasRoutingExecutor{id: poolCooldownProvider})
	if _, errRegister := manager.Register(WithSkipPersist(context.Background()), &Auth{
		ID:       "pool-cd-auth-disabled",
		Provider: poolCooldownProvider,
		Disabled: true,
		Metadata: map[string]any{"type": poolCooldownProvider},
	}); errRegister != nil {
		t.Fatalf("Register error = %v", errRegister)
	}
	registerSchedulerModels(t, poolCooldownProvider, poolCooldownModel, "pool-cd-auth-disabled")
	deadline := time.Now().Add(45 * time.Second)

	upstream := &Error{Code: "rate_limit", Message: "quota", HTTPStatus: http.StatusTooManyRequests}
	got := manager.annotatePoolCooldownTerminalErrorAt(
		context.Background(),
		[]string{poolCooldownProvider},
		poolCooldownModel,
		cliproxyexecutor.Options{},
		upstream,
		deadline,
		time.Now(),
	)

	var wrapped *poolCooldownTerminalError
	if !errors.As(got, &wrapped) {
		t.Fatalf("the terminal 429 was not wrapped, got %T: %v", got, got)
	}
	if !wrapped.deadline.Equal(deadline) {
		t.Fatalf("deadline = %v, want the selection deadline %v", wrapped.deadline, deadline)
	}
}

func TestAnnotatePoolCooldownTerminalErrorKeepsExistingModelCooldown(t *testing.T) {
	manager := newPoolCooldownManager(t, "pool-cd-auth-1")
	coolPoolCooldownModel(t, manager, "pool-cd-auth-1")

	existing := &modelCooldownError{model: poolCooldownModel, resetIn: 30 * time.Second}
	got := manager.annotatePoolCooldownTerminalErrorAt(
		context.Background(),
		[]string{poolCooldownProvider},
		poolCooldownModel,
		cliproxyexecutor.Options{},
		existing,
		time.Time{},
		time.Now(),
	)
	if got != error(existing) {
		t.Fatalf("the existing model cooldown error was replaced: got %v", got)
	}
}

func TestObserveSelectionCooldownDeadline(t *testing.T) {
	var deadline time.Time
	observeSelectionCooldownDeadline(&deadline, nil, time.Now())
	if !deadline.IsZero() {
		t.Fatalf("deadline = %v, want the zero value for a nil error", deadline)
	}

	observeSelectionCooldownDeadline(&deadline, &modelCooldownError{resetIn: 2 * time.Minute}, time.Now())
	if deadline.IsZero() {
		t.Fatal("deadline stayed zero, want the observed cooldown reset")
	}

	first := deadline
	observeSelectionCooldownDeadline(&deadline, &modelCooldownError{resetIn: 5 * time.Minute}, time.Now())
	if !deadline.Equal(first) {
		t.Fatalf("deadline = %v, want the earliest reset %v", deadline, first)
	}
}

func TestSelectorExcludesZeroWeight(t *testing.T) {
	if selectorExcludesZeroWeight(&RoundRobinSelector{}) {
		t.Fatal("a plain round-robin selector excludes zero-weight credentials")
	}
	if !selectorExcludesZeroWeight(&WeightedRoundRobinSelector{}) {
		t.Fatal("a weighted selector must exclude zero-weight credentials")
	}
	affinity := &SessionAffinitySelector{fallback: &WeightedRoundRobinSelector{}}
	if !selectorExcludesZeroWeight(affinity) {
		t.Fatal("an affinity selector with a weighted fallback must exclude zero-weight credentials")
	}
	plainAffinity := &SessionAffinitySelector{fallback: &RoundRobinSelector{}}
	if selectorExcludesZeroWeight(plainAffinity) {
		t.Fatal("an affinity selector with a plain fallback keeps zero-weight credentials")
	}
}
