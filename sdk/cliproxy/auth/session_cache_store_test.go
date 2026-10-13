package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionCacheSnapshotRestoreAndStore(t *testing.T) {
	cache := NewSessionCache(10 * time.Minute)
	defer cache.Stop()

	cache.Set("session-1", "auth-a")
	cache.SetAliases("auth-b", "session-2", "pck:cache-bucket-2")

	// Snapshot
	records := cache.Snapshot()
	if len(records) != 2 {
		t.Fatalf("expected 2 snapshot records, got %d", len(records))
	}

	// Persist to file
	tempDir, err := os.MkdirTemp("", "cpa-session-cache-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, ".session_cache.json")
	store := NewFileSessionCacheStore(filePath)

	ctx := context.Background()
	if err := store.Save(ctx, records); err != nil {
		t.Fatalf("store Save failed: %v", err)
	}

	// Load from file
	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("store Load failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 loaded records, got %d", len(loaded))
	}

	// Restore into a fresh cache
	freshCache := NewSessionCache(10 * time.Minute)
	defer freshCache.Stop()

	restoredCount := freshCache.Restore(loaded)
	if restoredCount != 2 {
		t.Fatalf("expected 2 restored records, got %d", restoredCount)
	}

	auth1, ok1 := freshCache.Get("session-1")
	if !ok1 || auth1 != "auth-a" {
		t.Fatalf("expected session-1 -> auth-a, got ok=%v auth=%s", ok1, auth1)
	}

	auth2, ok2 := freshCache.Get("session-2")
	if !ok2 || auth2 != "auth-b" {
		t.Fatalf("expected session-2 -> auth-b, got ok=%v auth=%s", ok2, auth2)
	}

	auth2Alias, ok2Alias := freshCache.Get("pck:cache-bucket-2")
	if !ok2Alias || auth2Alias != "auth-b" {
		t.Fatalf("expected pck:cache-bucket-2 -> auth-b, got ok=%v auth=%s", ok2Alias, auth2Alias)
	}
}

func TestSessionCacheStoreIgnoresExpired(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cpa-session-cache-expired-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, ".session_cache.json")
	store := NewFileSessionCacheStore(filePath)

	ctx := context.Background()
	records := []SessionCacheRecord{
		{
			PrimaryKey: "expired-session",
			AuthID:     "auth-old",
			ExpiresAt:  time.Now().Add(-1 * time.Minute),
			Aliases:    []string{"expired-session"},
		},
		{
			PrimaryKey: "valid-session",
			AuthID:     "auth-valid",
			ExpiresAt:  time.Now().Add(5 * time.Minute),
			Aliases:    []string{"valid-session"},
		},
	}

	if err := store.Save(ctx, records); err != nil {
		t.Fatalf("store Save failed: %v", err)
	}

	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("store Load failed: %v", err)
	}
	if len(loaded) != 1 || loaded[0].AuthID != "auth-valid" {
		t.Fatalf("expected 1 valid record, got %v", loaded)
	}
}
