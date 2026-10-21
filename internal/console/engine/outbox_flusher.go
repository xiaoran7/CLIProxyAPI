package engine

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/sirupsen/logrus"
)

// OutboxFlusher handles durable request recording, 5s transactional batching, and crash recovery.
type OutboxFlusher struct {
	db     *db.DB
	mem    *MemoryState
	logger *logrus.Entry

	flushInterval time.Duration
	flushLock     sync.Mutex
	stopCh        chan struct{}
	stopped       chan struct{}
	running       atomic.Bool

	lastFlushTime atomic.Int64
	flushFailures atomic.Int64
}

// NewOutboxFlusher creates a flusher instance.
func NewOutboxFlusher(database *db.DB, mem *MemoryState, interval time.Duration) *OutboxFlusher {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &OutboxFlusher{
		db:            database,
		mem:           mem,
		logger:        logrus.WithField("subsystem", "console_outbox"),
		flushInterval: interval,
		stopCh:        make(chan struct{}),
		stopped:       make(chan struct{}),
	}
}

// Start begins the background flush ticker.
func (f *OutboxFlusher) Start() {
	if f.running.Swap(true) {
		return
	}
	go f.flusherLoop()
}

// Stop stops the background flusher and performs a final synchronous flush.
func (f *OutboxFlusher) Stop() {
	if !f.running.Swap(false) {
		return
	}
	close(f.stopCh)
	<-f.stopped

	// Final flush on graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := f.FlushUsage(ctx); err != nil {
		f.logger.WithError(err).Warn("console: error during final outbox flush")
	}
}

func (f *OutboxFlusher) flusherLoop() {
	defer close(f.stopped)
	ticker := time.NewTicker(f.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-f.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			if err := f.FlushUsage(ctx); err != nil {
				f.logger.WithError(err).Warn("console: outbox batch flush failed")
			}
			cancel()
		}
	}
}

// NormalizeRequestEvent sanitizes and completes telemetry fields into a db.RequestEventRow.
func NormalizeRequestEvent(info *RequestInfo) *db.RequestEventRow {
	prompt := info.PromptTokens
	if prompt < 0 {
		prompt = 0
	}
	completion := info.CompletionTokens
	if completion < 0 {
		completion = 0
	}
	cached := info.CachedTokens
	if cached < 0 {
		cached = 0
	}

	total := info.TotalTokens
	if total <= 0 {
		total = prompt + completion
	}

	latency := info.LatencyMs
	if latency < 0 {
		latency = 0
	}

	stream := 0
	if info.Stream {
		stream = 1
	}

	estimated := 0
	if info.Estimated {
		estimated = 1
	}

	tps := info.TokensPerSec
	if tps <= 0 {
		var ttftVal int64
		if info.TtftMs != nil {
			ttftVal = *info.TtftMs
		}
		tps = CalculateTokensPerSec(completion, latency, ttftVal, info.Stream)
	} else {
		tps = math.Round(tps*10.0) / 10.0
	}

	model := strings.ToLower(strings.TrimSpace(info.Model))
	if model == "" {
		model = "unknown"
	}
	if len(model) > 120 {
		model = model[:120]
	}

	reasoningEffort := strings.ToLower(strings.TrimSpace(info.ReasoningEffort))
	if len(reasoningEffort) > 32 {
		reasoningEffort = reasoningEffort[:32]
	}

	authIndex := strings.TrimSpace(info.AuthIndex)
	if len(authIndex) > 64 {
		authIndex = authIndex[:64]
	}

	var errType *string
	if info.ErrorType != "" {
		s := info.ErrorType
		if len(s) > 64 {
			s = s[:64]
		}
		errType = &s
	}

	now := time.Now().Unix()

	return &db.RequestEventRow{
		TS:               now,
		KeyID:            info.KeyID,
		Model:            model,
		Status:           info.Status,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		CachedTokens:     cached,
		TotalTokens:      total,
		LatencyMs:        latency,
		TtftMs:           info.TtftMs,
		Stream:           stream,
		Estimated:        estimated,
		ErrorType:        errType,
		AuthIndex:        authIndex,
		ReasoningEffort:  reasoningEffort,
		TokensPerSec:     tps,
	}
}

// RecordRequest synchronously writes the event to usage_outbox in WAL mode
// and updates in-memory live token counters.
func (f *OutboxFlusher) RecordRequest(ctx context.Context, info *RequestInfo) error {
	row := NormalizeRequestEvent(info)

	// Synchronously write to SQLite usage_outbox
	_, err := f.db.InsertUsageOutbox(ctx, row)
	if err != nil {
		// Retain exact event in memory for batch flush retry
		f.mem.AddPendingEvent(row)
		f.logger.WithError(err).Warn("console: usage outbox insert failed, retained in memory")
		return err
	}

	// Update in-memory live usage maps
	f.mem.AddLiveUsage(row)
	return nil
}

// ReplayOutboxIntoMemory is called during startup to accumulate uncommitted outbox rows into memory.
func (f *OutboxFlusher) ReplayOutboxIntoMemory(ctx context.Context) error {
	rows, err := f.db.SelectAllOutbox(ctx)
	if err != nil {
		return fmt.Errorf("replay outbox failed: %w", err)
	}

	for i := range rows {
		f.mem.AddLiveUsage(&rows[i])
	}

	f.logger.Infof("console: replayed %d outbox records into in-memory counters", len(rows))
	return nil
}

// FlushUsage atomically writes outbox events into usage_daily and request_events.
func (f *OutboxFlusher) FlushUsage(ctx context.Context) error {
	f.flushLock.Lock()
	defer f.flushLock.Unlock()

	today := ShanghaiDate(time.Now())

	// Shanghai date rollover check
	f.mem.mu.Lock()
	if today != f.mem.usageDate {
		f.mem.usageDate = today
		f.mem.liveTodayTokensByKeyID = make(map[int64]int64)
		f.mem.liveTodayRequestsByKeyID = make(map[int64]int64)

		// Reload preloaded today from DB
		preloadedTokens := make(map[int64]int64)
		preloadedRequests := make(map[int64]int64)
		if dailyRows, err := f.db.SelectDailyForDate(ctx, today); err == nil {
			for _, r := range dailyRows {
				preloadedTokens[r.KeyID] += r.TotalTokens
				preloadedRequests[r.KeyID] += r.Requests
			}
		}
		f.mem.preloadedTodayTokens = preloadedTokens
		f.mem.preloadedTodayRequests = preloadedRequests
	}
	f.mem.mu.Unlock()

	outboxRows, err := f.db.SelectAllOutbox(ctx)
	if err != nil {
		f.flushFailures.Add(1)
		return fmt.Errorf("read outbox failed: %w", err)
	}

	pending := f.mem.DrainPendingEvents()

	if len(outboxRows) == 0 && len(pending) == 0 {
		f.lastFlushTime.Store(time.Now().Unix())
		return nil
	}

	// Atomic SQLite transaction
	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		f.flushFailures.Add(1)
		// Put pending events back
		for _, ev := range pending {
			f.mem.AddPendingEvent(ev)
		}
		return fmt.Errorf("begin transaction failed: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	processEvent := func(ev *db.RequestEventRow) error {
		date := ShanghaiDate(time.Unix(ev.TS, 0))
		isSuccess := int64(0)
		isFailure := int64(1)
		if ev.Status >= 200 && ev.Status < 400 {
			isSuccess = 1
			isFailure = 0
		}

		var keyID int64
		if ev.KeyID != nil {
			keyID = *ev.KeyID
		}

		estimatedTokens := int64(0)
		if ev.Estimated == 1 {
			estimatedTokens = ev.TotalTokens
		}

		// 1. Upsert usage_daily
		dailyRow := &db.UsageDailyRow{
			Date:             date,
			KeyID:            keyID,
			Model:            ev.Model,
			Requests:         1,
			PromptTokens:     ev.PromptTokens,
			CompletionTokens: ev.CompletionTokens,
			CachedTokens:     ev.CachedTokens,
			TotalTokens:      ev.TotalTokens,
			EstimatedTokens:  estimatedTokens,
			SuccessCount:     isSuccess,
			FailureCount:     isFailure,
		}
		if err := f.db.UpsertUsageDailyTx(ctx, tx, dailyRow); err != nil {
			return err
		}

		// 2. Upsert upstream_daily_stats if auth_index present
		if ev.AuthIndex != "" {
			upstreamRow := &db.UpstreamDailyStatRow{
				Date:             date,
				AuthIndex:        ev.AuthIndex,
				Provider:         "",
				Requests:         1,
				SuccessCount:     isSuccess,
				FailureCount:     isFailure,
				PromptTokens:     ev.PromptTokens,
				CompletionTokens: ev.CompletionTokens,
				CachedTokens:     ev.CachedTokens,
				TotalTokens:      ev.TotalTokens,
			}
			if err := f.db.UpsertUpstreamDailyStatTx(ctx, tx, upstreamRow); err != nil {
				return err
			}
		}

		// 3. Insert permanent request_events
		if _, err := f.db.InsertRequestEventTx(ctx, tx, ev); err != nil {
			return err
		}

		return nil
	}

	flushedTodayTokens := make(map[int64]int64)
	flushedTodayRequests := make(map[int64]int64)

	recordFlushedUsage := func(ev *db.RequestEventRow) {
		date := ShanghaiDate(time.Unix(ev.TS, 0))
		var keyID int64
		if ev.KeyID != nil {
			keyID = *ev.KeyID
		}
		if keyID > 0 && date == ShanghaiDate(time.Now()) {
			flushedTodayTokens[keyID] += ev.TotalTokens
			flushedTodayRequests[keyID] += 1
		}
	}

	// Process outbox rows
	for i := range outboxRows {
		ev := &outboxRows[i]
		if err := processEvent(ev); err != nil {
			f.flushFailures.Add(1)
			return fmt.Errorf("process outbox event %d failed: %w", ev.ID, err)
		}
		recordFlushedUsage(ev)
		if err := f.db.DeleteOutboxItemTx(ctx, tx, ev.ID); err != nil {
			f.flushFailures.Add(1)
			return fmt.Errorf("delete outbox item %d failed: %w", ev.ID, err)
		}
	}

	// Process pending in-memory events
	for _, ev := range pending {
		if err := processEvent(ev); err != nil {
			f.flushFailures.Add(1)
			return fmt.Errorf("process pending event failed: %w", err)
		}
		recordFlushedUsage(ev)
	}

	if err := tx.Commit(); err != nil {
		f.flushFailures.Add(1)
		// Put pending back
		for _, ev := range pending {
			f.mem.AddPendingEvent(ev)
		}
		return fmt.Errorf("commit flush tx failed: %w", err)
	}

	// Commit flushed counters into in-memory preloaded baseline and decrement live delta
	f.mem.CommitFlushedUsage(flushedTodayTokens, flushedTodayRequests)

	f.lastFlushTime.Store(time.Now().Unix())
	f.flushFailures.Store(0)
	return nil
}

// LastFlushTime returns the unix timestamp of the last successful flush.
func (f *OutboxFlusher) LastFlushTime() int64 {
	return f.lastFlushTime.Load()
}

// FlushFailures returns the count of consecutive flush failures.
func (f *OutboxFlusher) FlushFailures() int64 {
	return f.flushFailures.Load()
}
