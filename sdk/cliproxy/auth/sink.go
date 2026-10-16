package auth

import (
	"slices"
	"sync"
	"time"
)

type authSinkRecord struct {
	at  time.Time
	seq uint64
}

type authSinkRegistry struct {
	mu  sync.RWMutex
	m   map[string]authSinkRecord
	seq uint64
}

var globalAuthSink = authSinkRegistry{
	m: make(map[string]authSinkRecord),
}

// RecordRateLimitSink marks an auth credential as sunk due to HTTP 429 (rate limit).
// It assigns a monotonically increasing sequence number so that credentials that
// experienced rate limits are smoothly demoted to the back of the queue, preventing
// thrashing and immediate re-starvation when their cooldown expires.
func RecordRateLimitSink(authID string, now time.Time) {
	if authID == "" {
		return
	}
	globalAuthSink.mu.Lock()
	defer globalAuthSink.mu.Unlock()
	globalAuthSink.seq++
	globalAuthSink.m[authID] = authSinkRecord{at: now, seq: globalAuthSink.seq}
}

// GetRateLimitSink returns whether an auth was sunk, along with its sequence and timestamp.
func GetRateLimitSink(authID string) (uint64, time.Time, bool) {
	globalAuthSink.mu.RLock()
	defer globalAuthSink.mu.RUnlock()
	rec, ok := globalAuthSink.m[authID]
	if !ok {
		return 0, time.Time{}, false
	}
	return rec.seq, rec.at, true
}

// ClearRateLimitSink removes the sink record for an auth credential.
func ClearRateLimitSink(authID string) {
	globalAuthSink.mu.Lock()
	defer globalAuthSink.mu.Unlock()
	delete(globalAuthSink.m, authID)
}

// ResetRateLimitSinks clears all sink states across credentials.
func ResetRateLimitSinks() {
	globalAuthSink.mu.Lock()
	defer globalAuthSink.mu.Unlock()
	globalAuthSink.m = make(map[string]authSinkRecord)
	globalAuthSink.seq = 0
}

// ApplySinkOrdering sorts a slice of available auth candidates so that:
//  1. Un-sunk credentials (seq == 0) remain in front in their existing order.
//  2. Sunk credentials (seq > 0) are moved to the back, sorted by their sink sequence
//     (the one sunk earlier comes ahead of those sunk later, forming a round-robin cycle).
func ApplySinkOrdering(auths []*Auth) []*Auth {
	if len(auths) <= 1 {
		return auths
	}
	globalAuthSink.mu.RLock()
	defer globalAuthSink.mu.RUnlock()

	result := slices.Clone(auths)
	slices.SortStableFunc(result, func(a, b *Auth) int {
		if a == nil || b == nil {
			return 0
		}
		var seqA, seqB uint64
		if recA, ok := globalAuthSink.m[a.ID]; ok {
			seqA = recA.seq
		}
		if recB, ok := globalAuthSink.m[b.ID]; ok {
			seqB = recB.seq
		}
		switch {
		case seqA == seqB:
			return 0
		case seqA == 0:
			// a has not sunk, b has sunk -> a goes ahead of b
			return -1
		case seqB == 0:
			// b has not sunk, a has sunk -> b goes ahead of a
			return 1
		default:
			// both sunk: the one that sank earlier (smaller seq) goes ahead of the one that sank later
			if seqA < seqB {
				return -1
			}
			return 1
		}
	})
	return result
}
