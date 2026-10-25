package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestSelectLatestQuotaSnapshotsFoldsLegacyWindowNames guards the duplicate-meter
// regression: the same window written under an older name shape ("Gemini Models
// - Five Hour Limit Remaining") and a newer one ("Gemini Models:5h") used to be
// returned as two rows, rendering a stale second meter beside the real one.
func TestSelectLatestQuotaSnapshotsFoldsLegacyWindowNames(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	database, err := Open(filepath.Join(tempDir, "test_quota_dedup.db"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now().Unix()
	email := "user@example.com"
	legacy5h := "Gemini Models - Five Hour Limit Remaining"
	legacyWeekly := "Gemini Models - Weekly Limit Remaining"
	new5h := "Gemini Models:5h"
	newWeekly := "Gemini Models:weekly"

	// The legacy rows are older; the new rows are the truth to keep.
	rows := []QuotaSnapshotRow{
		{TS: now - 600, AuthIndex: "auth-1", Provider: "antigravity", AccountEmail: &email, WindowName: &legacy5h, RemainingPercent: 100},
		{TS: now - 600, AuthIndex: "auth-1", Provider: "antigravity", AccountEmail: &email, WindowName: &legacyWeekly, RemainingPercent: 100},
		{TS: now - 60, AuthIndex: "auth-1", Provider: "antigravity", AccountEmail: &email, WindowName: &new5h, RemainingPercent: 44},
		{TS: now - 60, AuthIndex: "auth-1", Provider: "antigravity", AccountEmail: &email, WindowName: &newWeekly, RemainingPercent: 77},
	}
	for _, row := range rows {
		r := row
		if _, errInsert := database.InsertQuotaSnapshot(ctx, &r); errInsert != nil {
			t.Fatalf("insert quota snapshot: %v", errInsert)
		}
	}

	got, errSelect := database.SelectLatestQuotaSnapshots(ctx, now-3600)
	if errSelect != nil {
		t.Fatalf("SelectLatestQuotaSnapshots: %v", errSelect)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 folded windows, got %d: %+v", len(got), windowNames(got))
	}
	for _, row := range got {
		if row.WindowName == nil {
			t.Fatal("row has no window name")
		}
		switch *row.WindowName {
		case new5h:
			if row.RemainingPercent != 44 {
				t.Errorf("5h window kept stale row: percent = %v, want 44", row.RemainingPercent)
			}
		case newWeekly:
			if row.RemainingPercent != 77 {
				t.Errorf("weekly window kept stale row: percent = %v, want 77", row.RemainingPercent)
			}
		default:
			t.Errorf("unexpected window survived dedup: %q", *row.WindowName)
		}
	}
}

func TestNormalizeQuotaWindowKey(t *testing.T) {
	t.Parallel()

	pairs := []struct {
		a, b string
	}{
		{"Gemini Models - Five Hour Limit Remaining", "Gemini Models:5h"},
		{"Gemini Models - Weekly Limit Remaining", "Gemini Models:weekly"},
		{"Claude and GPT models - Five Hour Limit Remaining", "Claude and GPT models:5h"},
		{"Claude and GPT models - Weekly Limit Remaining", "Claude and GPT models:weekly"},
	}
	for _, p := range pairs {
		if normalizeQuotaWindowKey(p.a) != normalizeQuotaWindowKey(p.b) {
			t.Errorf("keys differ: %q -> %q, %q -> %q",
				p.a, normalizeQuotaWindowKey(p.a), p.b, normalizeQuotaWindowKey(p.b))
		}
	}

	// Distinct windows must stay distinct.
	if normalizeQuotaWindowKey("Gemini 4 Argon:weekly") == normalizeQuotaWindowKey("Gemini Models:weekly") {
		t.Error("distinct groups collapsed into one key")
	}
	if normalizeQuotaWindowKey("Gemini Models:5h") == normalizeQuotaWindowKey("Gemini Models:weekly") {
		t.Error("distinct windows collapsed into one key")
	}
}

func windowNames(rows []QuotaSnapshotRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.WindowName != nil {
			out = append(out, *r.WindowName)
		}
	}
	return out
}
