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

func TestManagerHydrateStats(t *testing.T) {
	mgr := NewManager(nil, nil, nil)
	auth := &Auth{
		ID:       "auth-hydrate-1",
		Provider: "antigravity",
		Index:    "idx-hydrate-1",
		Metadata: map[string]any{
			"type": "antigravity",
		},
	}
	if _, err := mgr.Register(WithSkipPersist(context.Background()), auth); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	now := time.Now()
	currentBucket := recentRequestBucketID(now)

	mgr.HydrateStats([]AuthStatsUpdate{
		{
			AuthIndex: "idx-hydrate-1",
			Success:   150,
			Failed:    5,
			Buckets: []AuthBucketStat{
				{
					BucketID: currentBucket,
					Success:  10,
					Failed:   1,
				},
				{
					BucketID: currentBucket - 1,
					Success:  20,
					Failed:   0,
				},
			},
		},
	})

	gotAuth, ok := mgr.GetByID("auth-hydrate-1")
	if !ok || gotAuth == nil {
		t.Fatalf("GetByID failed: ok=%v, auth=%v", ok, gotAuth)
	}

	if gotAuth.Success != 150 || gotAuth.Failed != 5 {
		t.Fatalf("auth totals = success=%d failed=%d, want 150/5", gotAuth.Success, gotAuth.Failed)
	}

	snapshot := gotAuth.RecentRequestsSnapshot(now)
	if len(snapshot) != recentRequestBucketCount {
		t.Fatalf("snapshot length = %d, want %d", len(snapshot), recentRequestBucketCount)
	}

	lastEntry := snapshot[len(snapshot)-1]
	if lastEntry.Success != 10 || lastEntry.Failed != 1 {
		t.Fatalf("current bucket = success=%d failed=%d, want 10/1", lastEntry.Success, lastEntry.Failed)
	}

	prevEntry := snapshot[len(snapshot)-2]
	if prevEntry.Success != 20 || prevEntry.Failed != 0 {
		t.Fatalf("previous bucket = success=%d failed=%d, want 20/0", prevEntry.Success, prevEntry.Failed)
	}
}

func TestManagerEnsureStatsHydrated_RateLimit(t *testing.T) {
	mgr := NewManager(nil, nil, nil)
	calls := 0
	mgr.SetStatsHydrator(func(ctx context.Context, m *Manager) error {
		calls++
		return nil
	})

	ctx := context.Background()
	if err := mgr.EnsureStatsHydrated(ctx); err != nil {
		t.Fatalf("first EnsureStatsHydrated failed: %v", err)
	}
	if err := mgr.EnsureStatsHydrated(ctx); err != nil {
		t.Fatalf("second EnsureStatsHydrated failed: %v", err)
	}

	if calls != 1 {
		t.Fatalf("hydrator calls = %d, want 1 (throttled)", calls)
	}
}

func TestAuthHydrateRecentBucketAndMerge(t *testing.T) {
	auth := &Auth{
		ID: "test-auth",
	}
	auth.HydrateRecentBucket(100, 5, 1)
	auth.HydrateRecentBucket(100, 3, 0)
	auth.HydrateRecentBucket(100, 10, 2)
	auth.HydrateLifetimeStats(50, 5)
	auth.HydrateLifetimeStats(30, 2)
	auth.HydrateLifetimeStats(80, 10)

	if auth.Success != 80 || auth.Failed != 10 {
		t.Fatalf("lifetime stats = %d/%d, want 80/10", auth.Success, auth.Failed)
	}

	idx := recentRequestBucketIndex(100)
	bucket := auth.recentRequests.buckets[idx]
	if bucket.bucketID != 100 || bucket.success != 10 || bucket.failed != 2 {
		t.Fatalf("bucket = ID:%d S:%d F:%d, want 100/10/2", bucket.bucketID, bucket.success, bucket.failed)
	}
}
