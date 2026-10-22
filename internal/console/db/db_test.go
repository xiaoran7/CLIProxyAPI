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

func TestDB_NullableColumnsHandling(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_nulls.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Raw insert key with NULL alias
	_, err = database.ExecContext(ctx, "INSERT INTO keys (id, api_key, alias, active) VALUES (1, 'sk-test-null', NULL, 1)")
	if err != nil {
		t.Fatalf("failed to raw insert key: %v", err)
	}

	// Raw insert quota_rule with NULL rpm_limit, NULL daily_limit, NULL model_whitelist, NULL model_group
	_, err = database.ExecContext(ctx, "INSERT INTO quota_rules (key_id, enabled, daily_limit, hard_ratio, rpm_limit, model_whitelist, model_group) VALUES (1, 1, NULL, 1.2, NULL, NULL, NULL)")
	if err != nil {
		t.Fatalf("failed to raw insert quota rule: %v", err)
	}

	// Verify SelectAllKeys scans without error
	keys, err := database.SelectAllKeys(ctx)
	if err != nil {
		t.Fatalf("SelectAllKeys failed on NULL alias: %v", err)
	}
	if len(keys) != 1 || keys[0].Alias != "" {
		t.Fatalf("unexpected keys result: %+v", keys)
	}

	// Verify SelectKeyByID scans without error
	key, err := database.SelectKeyByID(ctx, 1)
	if err != nil || key == nil {
		t.Fatalf("SelectKeyByID failed on NULL alias: %v", err)
	}

	// Verify SelectAllQuotaRules scans without error
	rules, err := database.SelectAllQuotaRules(ctx)
	if err != nil {
		t.Fatalf("SelectAllQuotaRules failed on NULL fields: %v", err)
	}
	if len(rules) != 1 || rules[0].RPMLimit != 0 || rules[0].DailyLimit != nil {
		t.Fatalf("unexpected rules result: %+v", rules)
	}

	// Verify SelectQuotaRuleByKeyID scans without error
	rule, err := database.SelectQuotaRuleByKeyID(ctx, 1)
	if err != nil || rule == nil {
		t.Fatalf("SelectQuotaRuleByKeyID failed on NULL fields: %v", err)
	}
}

func TestDB_SelectRecentAuthBuckets(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_recent_buckets.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().Unix()
	bucketID1 := now / 600
	bucketID2 := bucketID1 - 1

	// Insert events for auth_1 (2 success, 1 failure in bucket 1)
	_, _ = database.InsertRequestEvent(ctx, &RequestEventRow{
		TS:        bucketID1*600 + 10,
		Status:    200,
		AuthIndex: "auth_1",
	})
	_, _ = database.InsertRequestEvent(ctx, &RequestEventRow{
		TS:        bucketID1*600 + 50,
		Status:    200,
		AuthIndex: "auth_1",
	})
	_, _ = database.InsertRequestEvent(ctx, &RequestEventRow{
		TS:        bucketID1*600 + 100,
		Status:    500,
		AuthIndex: "auth_1",
	})

	// Insert event for auth_1 in bucket 2 (1 success)
	_, _ = database.InsertRequestEvent(ctx, &RequestEventRow{
		TS:        bucketID2*600 + 20,
		Status:    200,
		AuthIndex: "auth_1",
	})

	// Insert event with empty auth_index (should be ignored)
	_, _ = database.InsertRequestEvent(ctx, &RequestEventRow{
		TS:        bucketID1*600 + 30,
		Status:    200,
		AuthIndex: "",
	})

	sinceTS := (bucketID2 - 1) * 600
	buckets, err := database.SelectRecentAuthBuckets(ctx, sinceTS)
	if err != nil {
		t.Fatalf("SelectRecentAuthBuckets failed: %v", err)
	}

	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d: %+v", len(buckets), buckets)
	}

	for _, b := range buckets {
		if b.AuthIndex != "auth_1" {
			t.Fatalf("unexpected auth_index: %s", b.AuthIndex)
		}
		if b.BucketID == bucketID1 {
			if b.Success != 2 || b.Failed != 1 {
				t.Fatalf("bucketID1 counts = S:%d F:%d, want 2/1", b.Success, b.Failed)
			}
		} else if b.BucketID == bucketID2 {
			if b.Success != 1 || b.Failed != 0 {
				t.Fatalf("bucketID2 counts = S:%d F:%d, want 1/0", b.Success, b.Failed)
			}
		} else {
			t.Fatalf("unexpected bucketID: %d", b.BucketID)
		}
	}
}
