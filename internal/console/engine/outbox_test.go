package engine

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
)

func TestEngine_OutboxAndQuotas(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_engine.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Create Key and Quota Rule in SQLite
	rawKey := "sk-cpa-test-key-for-engine-testing-987654321"
	keyID, err := database.InsertKey(ctx, rawKey, "Engine Test Key")
	if err != nil {
		t.Fatalf("insert key failed: %v", err)
	}

	dailyLimit := int64(1000)
	rule := &db.QuotaRuleRow{
		KeyID:          keyID,
		Enabled:        1,
		DailyLimit:     &dailyLimit,
		HardRatio:      1.2, // Hard limit = 1200
		RPMLimit:       3,   // Max 3 requests per 60s
		ModelWhitelist: `["gpt-4o","claude-3-7-sonnet"]`,
	}
	if err := database.UpsertQuotaRule(ctx, rule); err != nil {
		t.Fatalf("upsert rule failed: %v", err)
	}

	// 2. Instantiate Engine
	eng, err := NewEngine(database, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("new engine failed: %v", err)
	}

	// Verify key resolution
	resolvedID, ok := eng.ResolveKeyID(rawKey)
	if !ok || resolvedID != keyID {
		t.Fatalf("expected key resolution to %d, got %d (%v)", keyID, resolvedID, ok)
	}

	// 3. Test RPM Rate Limiting (Limit: 3)
	for i := 0; i < 3; i++ {
		res := eng.CheckRateLimit(keyID)
		if !res.Allowed {
			t.Fatalf("expected request %d to be allowed by RPM limiter, got blocked: %s", i+1, res.Reason)
		}
	}
	// 4th request must be blocked
	resRPM := eng.CheckRateLimit(keyID)
	if resRPM.Allowed {
		t.Fatalf("expected 4th request to be blocked by RPM limiter")
	}

	// 4. Test Model Allowlist
	resModel := eng.CheckModelAllowed(keyID, "claude-3-7-sonnet")
	if !resModel.Allowed {
		t.Fatalf("expected claude-3-7-sonnet to be allowed: %s", resModel.Reason)
	}
	resDisallowed := eng.CheckModelAllowed(keyID, "llama-3.3-70b")
	if resDisallowed.Allowed {
		t.Fatalf("expected llama-3.3-70b to be disallowed")
	}

	// 5. Test Quota Enforcement (Daily limit: 1000, Hard limit: 1200)
	// Initial check: 0 tokens used
	qRes := eng.CheckQuota(keyID)
	if !qRes.Allowed || qRes.Usage != 0 {
		t.Fatalf("unexpected quota check: %+v", qRes)
	}

	// Record request with 500 tokens
	ttft := int64(200)
	info1 := &RequestInfo{
		KeyID:            &keyID,
		Model:            "gpt-4o",
		Status:           200,
		PromptTokens:     200,
		CompletionTokens: 300,
		TotalTokens:      500,
		LatencyMs:        800,
		TtftMs:           &ttft,
		Stream:           true,
	}
	if err := eng.RecordRequest(ctx, info1); err != nil {
		t.Fatalf("record request 1 failed: %v", err)
	}

	// Live tokens should now be 500
	qRes2 := eng.CheckQuota(keyID)
	if !qRes2.Allowed || qRes2.Usage != 500 {
		t.Fatalf("expected 500 used tokens, got %+v", qRes2)
	}

	// Record another request with 600 tokens (Total: 1100, above 1000 soft limit but below 1200 hard limit)
	info2 := &RequestInfo{
		KeyID:            &keyID,
		Model:            "gpt-4o",
		Status:           200,
		PromptTokens:     300,
		CompletionTokens: 300,
		TotalTokens:      600,
		LatencyMs:        900,
	}
	if err := eng.RecordRequest(ctx, info2); err != nil {
		t.Fatalf("record request 2 failed: %v", err)
	}

	qRes3 := eng.CheckQuota(keyID)
	if !qRes3.Allowed || qRes3.Usage != 1100 {
		t.Fatalf("expected 1100 used tokens to be allowed under hard limit 1200, got: %+v", qRes3)
	}

	// Record another request with 200 tokens (Total: 1300, exceeding 1200 hard limit)
	info3 := &RequestInfo{
		KeyID:            &keyID,
		Model:            "gpt-4o",
		Status:           200,
		PromptTokens:     100,
		CompletionTokens: 100,
		TotalTokens:      200,
		LatencyMs:        400,
	}
	if err := eng.RecordRequest(ctx, info3); err != nil {
		t.Fatalf("record request 3 failed: %v", err)
	}

	qRes4 := eng.CheckQuota(keyID)
	if qRes4.Allowed {
		t.Fatalf("expected 1300 used tokens to be blocked by hard limit, got allowed: %+v", qRes4)
	}

	// 6. Test FlushUsage
	outboxCnt, err := database.CountOutbox(ctx)
	if err != nil || outboxCnt != 3 {
		t.Fatalf("expected 3 outbox records before flush, got %d", outboxCnt)
	}

	if err := eng.Flusher().FlushUsage(ctx); err != nil {
		t.Fatalf("flush usage failed: %v", err)
	}

	// Outbox should be drained
	outboxCntAfter, err := database.CountOutbox(ctx)
	if err != nil || outboxCntAfter != 0 {
		t.Fatalf("expected 0 outbox records after flush, got %d", outboxCntAfter)
	}

	// Permanent request_events should have 3 rows
	events, err := database.SelectRecentEvents(ctx, 10)
	if err != nil || len(events) != 3 {
		t.Fatalf("expected 3 request_events, got %d (err: %v)", len(events), err)
	}

	// 7. Test Crash Replay
	// Insert an outbox record directly simulating an un-flushed crash
	crashRow := &db.RequestEventRow{
		TS:               time.Now().Unix(),
		KeyID:            &keyID,
		Model:            "gpt-4o",
		Status:           200,
		PromptTokens:     100,
		CompletionTokens: 100,
		TotalTokens:      200,
		LatencyMs:        500,
	}
	if _, err := database.InsertUsageOutbox(ctx, crashRow); err != nil {
		t.Fatalf("insert crash outbox failed: %v", err)
	}

	// Create new engine instance representing server restart
	engRestarted, err := NewEngine(database, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("restart engine failed: %v", err)
	}

	// Replayed usage should include preloaded (1300) + replayed outbox (200) = 1500
	todayUsage := engRestarted.Memory().GetTodayUsageForKey(keyID)
	if todayUsage != 1500 {
		t.Fatalf("expected 1500 tokens after crash replay, got %d", todayUsage)
	}
}
