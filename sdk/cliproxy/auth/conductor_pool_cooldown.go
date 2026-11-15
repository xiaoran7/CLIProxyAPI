package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

// selectorExcludesZeroWeight reports whether the active selector drops
// credentials whose weight is zero. A zero-weight credential is never a
// selection candidate, so it must not count towards the pool cooldown either.
func selectorExcludesZeroWeight(selector Selector) bool {
	if _, weighted := selector.(*WeightedRoundRobinSelector); weighted {
		return true
	}
	if affinity, ok := selector.(*SessionAffinitySelector); ok && affinity != nil {
		_, weighted := affinity.fallback.(*WeightedRoundRobinSelector)
		return weighted
	}
	return false
}

// earliestPoolCooldownForModelAt scans the model's candidate credentials with
// the same filters selection uses. It reports:
//   - earliest: the earliest active timed cooldown recovery instant
//   - hasUnblocked: true when at least one eligible candidate is still available
//   - ok: true when no candidate is available and at least one is in a timed cooldown
//
// Ported from upstream commit 4da9f6852 (closes upstream issue #7521). The
// only adaptation is the blocked check: this fork has no request-aware
// variant of it yet, so the plain per-model check is used, which is also what
// selection uses on this code path.
func (m *Manager) earliestPoolCooldownForModelAt(ctx context.Context, providers []string, model string, opts cliproxyexecutor.Options, now time.Time) (earliest time.Time, hasUnblocked bool, ok bool) {
	if m == nil {
		return time.Time{}, false, false
	}
	excludeZeroWeight := selectorExcludesZeroWeight(m.Selector())
	m.mu.RLock()
	defer m.mu.RUnlock()
	eligibility := authSelectionEligibilityForRequest(ctx, opts)
	pinnedAuthID := pinnedAuthIDFromMetadata(opts.Metadata)
	providerSet := make(map[string]struct{}, len(providers))
	for _, p := range providers {
		if norm := canonicalSchedulingProvider(p); norm != "" && norm != "mixed" {
			providerSet[norm] = struct{}{}
		}
	}
	registryRef := registry.GetGlobalRegistry()
	trimmedModel := strings.TrimSpace(model)

	totalCandidates := 0
	cooling := 0
	for _, candidate := range m.auths {
		if candidate == nil || candidate.Disabled {
			continue
		}
		if excludeZeroWeight && authWeight(candidate) <= 0 {
			continue
		}
		rawProvider := executorKeyFromAuth(candidate)
		providerKey := canonicalSchedulingProvider(rawProvider)
		if providerKey == "" {
			continue
		}
		if len(providerSet) > 0 {
			if _, match := providerSet[providerKey]; !match {
				continue
			}
		}
		if _, hasExec := m.executorLocked(rawProvider); !hasExec {
			if _, hasNormExec := m.executorLocked(providerKey); !hasNormExec {
				continue
			}
		}
		if pinnedAuthID != "" && candidate.ID != pinnedAuthID {
			continue
		}
		if !eligibility.allows(candidate) {
			continue
		}
		if trimmedModel != "" && !m.authSupportsRouteModel(registryRef, candidate, model) {
			continue
		}
		totalCandidates++
		checkModel := m.selectionModelForAuth(candidate, model)
		blocked, reason, next := isAuthBlockedForModel(candidate, checkModel, now)
		if !blocked {
			return time.Time{}, true, false
		}
		if reason == blockReasonCooldown {
			cooling++
		}
		// Count every timed auto-recovery toward the pool's earliest reset,
		// including transient non-cooldown blocks (for example a short
		// capacity 429), matching how selection computes the earliest
		// availability.
		if reason != blockReasonDisabled && !next.IsZero() && next.After(now) {
			if earliest.IsZero() || next.Before(earliest) {
				earliest = next
			}
		}
	}
	if totalCandidates > 0 && cooling > 0 && !earliest.IsZero() {
		return earliest, false, true
	}
	return time.Time{}, false, false
}

// observeSelectionCooldownDeadline records the earliest model-cooldown reset
// seen while a request failed, so a terminal 429 can report the pool's real
// recovery time even when the failure never reached selection.
func observeSelectionCooldownDeadline(dst *time.Time, err error, now time.Time) {
	if dst == nil || err == nil {
		return
	}
	var cooldown *modelCooldownError
	if !errors.As(err, &cooldown) || cooldown == nil || cooldown.resetIn <= 0 {
		return
	}
	candidate := now.Add(cooldown.resetIn)
	if dst.IsZero() || candidate.Before(*dst) {
		*dst = candidate
	}
}

// annotatePoolCooldownTerminalError marks a terminal upstream 429 as a pool
// cooldown when every eligible candidate for the model is blocked and at
// least one is in a timed cooldown. The upstream body, status and headers are
// preserved; the client additionally learns when the pool frees up instead of
// seeing a bare 429 that hides the reset.
func (m *Manager) annotatePoolCooldownTerminalError(ctx context.Context, providers []string, model string, opts cliproxyexecutor.Options, lastErr error, selectionCooldownDeadline time.Time) error {
	return m.annotatePoolCooldownTerminalErrorAt(ctx, providers, model, opts, lastErr, selectionCooldownDeadline, time.Now())
}

// annotatePoolCooldownTerminalErrorAt is the deterministic variant of
// annotatePoolCooldownTerminalError, used by the tests.
func (m *Manager) annotatePoolCooldownTerminalErrorAt(ctx context.Context, providers []string, model string, opts cliproxyexecutor.Options, lastErr error, selectionCooldownDeadline time.Time, now time.Time) error {
	if m == nil || lastErr == nil {
		return lastErr
	}
	if statusCodeFromError(lastErr) != http.StatusTooManyRequests {
		return lastErr
	}
	var marker interface{ IsModelCooldown() bool }
	if errors.As(lastErr, &marker) && marker != nil && marker.IsModelCooldown() {
		return lastErr
	}
	var existing *poolCooldownTerminalError
	if errors.As(lastErr, &existing) && existing != nil {
		return lastErr
	}
	deadline, hasUnblocked, ok := m.earliestPoolCooldownForModelAt(ctx, providers, model, opts, now)
	if hasUnblocked {
		return lastErr
	}
	if !ok && !selectionCooldownDeadline.IsZero() && selectionCooldownDeadline.After(now) {
		deadline = selectionCooldownDeadline
		ok = true
	}
	if !ok {
		return lastErr
	}
	return &poolCooldownTerminalError{cause: lastErr, deadline: deadline}
}
