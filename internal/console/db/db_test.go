package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestDB_OpenAndMigrate(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_gate.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Test Key CRUD
	keyID, err := database.InsertKey(ctx, "sk-test-key-1234567890123456", "Unit Test Key")
	if err != nil {
		t.Fatalf("failed to insert key: %v", err)
	}
	if keyID <= 0 {
		t.Fatalf("expected positive keyID, got %d", keyID)
	}

	key, err := database.SelectKeyByID(ctx, keyID)
	if err != nil || key == nil {
		t.Fatalf("failed to select key: %v", err)
	}
	if key.APIKey != "sk-test-key-1234567890123456" || key.Alias != "Unit Test Key" {
		t.Fatalf("unexpected key content: %+v", key)
	}

	// 2. Test Quota Rule
	dailyLimit := int64(1000000)
	rule := &QuotaRuleRow{
		KeyID:          keyID,
		Enabled:        1,
		DailyLimit:     &dailyLimit,
		HardRatio:      1.25,
		RPMLimit:       60,
		ModelWhitelist: `["gpt-4o","claude-3-7-sonnet"]`,
	}
	if err := database.UpsertQuotaRule(ctx, rule); err != nil {
		t.Fatalf("failed to upsert quota rule: %v", err)
	}

	fetchedRule, err := database.SelectQuotaRuleByKeyID(ctx, keyID)
	if err != nil || fetchedRule == nil {
		t.Fatalf("failed to fetch quota rule: %v", err)
	}
	if fetchedRule.DailyLimit == nil || *fetchedRule.DailyLimit != 1000000 || fetchedRule.RPMLimit != 60 {
		t.Fatalf("unexpected quota rule content: %+v", fetchedRule)
	}

	// 3. Test Outbox & Events
	ttft := int64(350)
	eventRow := &RequestEventRow{
		TS:               time.Now().Unix(),
		KeyID:            &keyID,
		Model:            "gpt-4o",
		Status:           200,
		PromptTokens:     100,
		CompletionTokens: 250,
		CachedTokens:     50,
		TotalTokens:      350,
		LatencyMs:        1200,
		TtftMs:           &ttft,
		Stream:           1,
		TokensPerSec:     294.1,
	}

	outboxID, err := database.InsertUsageOutbox(ctx, eventRow)
	if err != nil || outboxID <= 0 {
		t.Fatalf("failed to insert outbox: %v", err)
	}

	outboxItems, err := database.SelectAllOutbox(ctx)
	if err != nil || len(outboxItems) != 1 {
		t.Fatalf("expected 1 outbox item, got %d (err: %v)", len(outboxItems), err)
	}

	// 4. Test Transactional Flush Simulation
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}

	dailyRow := &UsageDailyRow{
		Date:             "2026-10-21",
		KeyID:            keyID,
		Model:            "gpt-4o",
		Requests:         1,
		PromptTokens:     100,
		CompletionTokens: 250,
		CachedTokens:     50,
		TotalTokens:      350,
		SuccessCount:     1,
		FailureCount:     0,
	}
	if err := database.UpsertUsageDailyTx(ctx, tx, dailyRow); err != nil {
		_ = tx.Rollback()
		t.Fatalf("failed to upsert usage daily tx: %v", err)
	}

	if _, err := database.InsertRequestEventTx(ctx, tx, eventRow); err != nil {
		_ = tx.Rollback()
		t.Fatalf("failed to insert request event tx: %v", err)
	}

	if err := database.DeleteOutboxItemTx(ctx, tx, outboxID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("failed to delete outbox item tx: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit tx: %v", err)
	}

	// Verify outbox is drained
	cnt, err := database.CountOutbox(ctx)
	if err != nil || cnt != 0 {
		t.Fatalf("expected outbox count 0, got %d (err: %v)", cnt, err)
	}

	// Verify daily summary
	summary, err := database.SelectDailyForDateKey(ctx, "2026-10-21", keyID)
	if err != nil || summary == nil {
		t.Fatalf("failed to select daily for date key: %v", err)
	}
	if summary.TotalTokens != 350 || summary.Requests != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}
