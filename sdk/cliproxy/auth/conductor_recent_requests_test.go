package auth

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestManagerMarkResultRecordsRecentRequests(t *testing.T) {
	mgr := NewManager(nil, nil, nil)
	auth := &Auth{
		ID:       "auth-1",
		Provider: "antigravity",
		Attributes: map[string]string{
			"runtime_only": "true",
		},
		Metadata: map[string]any{
			"type": "antigravity",
		},
	}

	if _, err := mgr.Register(WithSkipPersist(context.Background()), auth); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	mgr.MarkResult(context.Background(), Result{AuthID: "auth-1", Provider: "antigravity", Model: "gpt-5", Success: true})
	mgr.MarkResult(context.Background(), Result{AuthID: "auth-1", Provider: "antigravity", Model: "gpt-5", Success: false})

	gotAuth, ok := mgr.GetByID("auth-1")
	if !ok || gotAuth == nil {
		t.Fatalf("GetByID returned ok=%v auth=%v", ok, gotAuth)
	}

	if gotAuth.Success != 1 || gotAuth.Failed != 1 {
		t.Fatalf("auth totals = success=%d failed=%d, want 1/1", gotAuth.Success, gotAuth.Failed)
	}

	snapshot := gotAuth.RecentRequestsSnapshot(time.Now())
	var successTotal int64
	var failedTotal int64
	for _, bucket := range snapshot {
		successTotal += bucket.Success
		failedTotal += bucket.Failed
	}
	if successTotal != 1 || failedTotal != 1 {
		t.Fatalf("totals = success=%d failed=%d, want 1/1", successTotal, failedTotal)
	}
}

func TestManagerUpdatePreservesRecentRequestsAndTotals(t *testing.T) {
	mgr := NewManager(nil, nil, nil)
	auth := &Auth{
		ID:       "auth-1",
		Provider: "antigravity",
		Metadata: map[string]any{
			"type": "antigravity",
		},
	}
	if _, err := mgr.Register(WithSkipPersist(context.Background()), auth); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	mgr.MarkResult(context.Background(), Result{AuthID: "auth-1", Provider: "antigravity", Model: "gpt-5", Success: true})

	updated := &Auth{
		ID:       "auth-1",
		Provider: "antigravity",
		Metadata: map[string]any{
			"type": "antigravity",
			"note": "updated",
		},
	}
	if _, err := mgr.Update(WithSkipPersist(context.Background()), updated); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	gotAuth, ok := mgr.GetByID("auth-1")
	if !ok || gotAuth == nil {
		t.Fatalf("GetByID returned ok=%v auth=%v", ok, gotAuth)
	}
	if gotAuth.Success != 1 || gotAuth.Failed != 0 {
		t.Fatalf("auth totals = success=%d failed=%d, want 1/0", gotAuth.Success, gotAuth.Failed)
	}

	snapshot := gotAuth.RecentRequestsSnapshot(time.Now())
	var successTotal int64
	var failedTotal int64
	for _, bucket := range snapshot {
		successTotal += bucket.Success
		failedTotal += bucket.Failed
	}
	if successTotal != 1 || failedTotal != 0 {
		t.Fatalf("bucket totals = success=%d failed=%d, want 1/0", successTotal, failedTotal)
	}
}

func TestManagerRegisterPreservesRecentRequestsAndTotals(t *testing.T) {
	mgr := NewManager(nil, nil, nil)
	auth := &Auth{
		ID:       "auth-1",
		Provider: "antigravity",
		Metadata: map[string]any{
			"type": "antigravity",
		},
	}
	if _, err := mgr.Register(WithSkipPersist(context.Background()), auth); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	mgr.MarkResult(context.Background(), Result{AuthID: "auth-1", Provider: "antigravity", Model: "gpt-5", Success: true})
	mgr.MarkResult(context.Background(), Result{AuthID: "auth-1", Provider: "antigravity", Model: "gpt-5", Success: false})

	// Re-register the credential (e.g. from a payload without metrics).
	reRegistered := &Auth{
		ID:       "auth-1",
		Provider: "antigravity",
		Metadata: map[string]any{
			"type": "antigravity",
			"tag":  "reloaded",
		},
	}
	if _, err := mgr.Register(WithSkipPersist(context.Background()), reRegistered); err != nil {
		t.Fatalf("second Register returned error: %v", err)
	}

	gotAuth, ok := mgr.GetByID("auth-1")
	if !ok || gotAuth == nil {
		t.Fatalf("GetByID returned ok=%v auth=%v", ok, gotAuth)
	}
	if gotAuth.Success != 1 || gotAuth.Failed != 1 {
		t.Fatalf("auth totals = success=%d failed=%d, want 1/1", gotAuth.Success, gotAuth.Failed)
	}

	snapshot := gotAuth.RecentRequestsSnapshot(time.Now())
	var successTotal int64
	var failedTotal int64
	for _, bucket := range snapshot {
		successTotal += bucket.Success
		failedTotal += bucket.Failed
	}
	if successTotal != 1 || failedTotal != 1 {
		t.Fatalf("bucket totals = success=%d failed=%d, want 1/1", successTotal, failedTotal)
	}
}

type inMemoryRecentStore struct {
	mu    sync.Mutex
	auths []*Auth
}

func (s *inMemoryRecentStore) List(context.Context) ([]*Auth, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	clones := make([]*Auth, len(s.auths))
	for i, a := range s.auths {
		clones[i] = a.Clone()
	}
	return clones, nil
}

func (s *inMemoryRecentStore) Save(_ context.Context, auth *Auth) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.auths {
		if a.ID == auth.ID {
			s.auths[i] = auth.Clone()
			return auth.FileName, nil
		}
	}
	s.auths = append(s.auths, auth.Clone())
	return auth.FileName, nil
}

func (s *inMemoryRecentStore) Delete(context.Context, string) error { return nil }

func TestManagerLoadPreservesRecentRequestsAndTotals(t *testing.T) {
	store := &inMemoryRecentStore{
		auths: []*Auth{
			{
				ID:       "auth-persist-1",
				Provider: "antigravity",
				FileName: "auth-persist-1.json",
				Metadata: map[string]any{
					"type": "antigravity",
				},
			},
		},
	}

	mgr := NewManager(store, nil, nil)
	if err := mgr.Load(context.Background()); err != nil {
		t.Fatalf("initial Load failed: %v", err)
	}

	mgr.MarkResult(context.Background(), Result{AuthID: "auth-persist-1", Provider: "antigravity", Model: "gpt-5", Success: true})
	mgr.MarkResult(context.Background(), Result{AuthID: "auth-persist-1", Provider: "antigravity", Model: "gpt-5", Success: true})
	mgr.MarkResult(context.Background(), Result{AuthID: "auth-persist-1", Provider: "antigravity", Model: "gpt-5", Success: false})

	// Simulate periodic full reload from disk where files on disk do not contain in-memory stats.
	if err := mgr.Load(context.Background()); err != nil {
		t.Fatalf("second Load failed: %v", err)
	}

	gotAuth, ok := mgr.GetByID("auth-persist-1")
	if !ok || gotAuth == nil {
		t.Fatalf("GetByID returned ok=%v auth=%v", ok, gotAuth)
	}
	if gotAuth.Success != 2 || gotAuth.Failed != 1 {
		t.Fatalf("auth totals after reload = success=%d failed=%d, want 2/1", gotAuth.Success, gotAuth.Failed)
	}

	snapshot := gotAuth.RecentRequestsSnapshot(time.Now())
	var successTotal int64
	var failedTotal int64
	for _, bucket := range snapshot {
		successTotal += bucket.Success
		failedTotal += bucket.Failed
	}
	if successTotal != 2 || failedTotal != 1 {
		t.Fatalf("bucket totals after reload = success=%d failed=%d, want 2/1", successTotal, failedTotal)
	}
}
