package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/sirupsen/logrus"
)

// Engine is the central coordinator for Console's authentication, rate-limiting,
// quota enforcement, and asynchronous transactional outbox recording.
type Engine struct {
	db      *db.DB
	mem     *MemoryState
	flusher *OutboxFlusher
	logger  *logrus.Entry
}

// NewEngine creates and initializes the Console Engine.
func NewEngine(database *db.DB, flushInterval time.Duration) (*Engine, error) {
	mem := NewMemoryState()
	flusher := NewOutboxFlusher(database, mem, flushInterval)

	e := &Engine{
		db:      database,
		mem:     mem,
		flusher: flusher,
		logger:  logrus.WithField("subsystem", "console_engine"),
	}

	// Initial synchronous reload of keys, quota rules, and preloaded daily usage
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.ReloadAll(ctx); err != nil {
		return nil, fmt.Errorf("engine: initial reload failed: %w", err)
	}

	// Replay uncommitted outbox into live memory counters
	if err := flusher.ReplayOutboxIntoMemory(ctx); err != nil {
		e.logger.WithError(err).Warn("engine: replay outbox failed")
	}

	return e, nil
}

// DB returns the underlying SQLite connection wrapper.
func (e *Engine) DB() *db.DB {
	return e.db
}

// Flusher returns the OutboxFlusher instance.
func (e *Engine) Flusher() *OutboxFlusher {
	return e.flusher
}

// Memory returns the in-memory state tracker.
func (e *Engine) Memory() *MemoryState {
	return e.mem
}

// Start launches the engine background flusher.
func (e *Engine) Start() {
	e.flusher.Start()
	e.logger.Info("console engine started successfully")
}

// Stop halts the flusher and commits all pending usage.
func (e *Engine) Stop() {
	e.flusher.Stop()
	e.logger.Info("console engine stopped")
}

// ReloadAll reloads keys, quota rules, and preloaded today totals from SQLite.
func (e *Engine) ReloadAll(ctx context.Context) error {
	if err := e.ReloadKeys(ctx); err != nil {
		return err
	}
	if err := e.ReloadQuotaRules(ctx); err != nil {
		return err
	}
	if err := e.ReloadPreloadedToday(ctx); err != nil {
		return err
	}
	return nil
}

// ReloadState reloads all keys, quota rules, and preloaded usage from SQLite.
func (e *Engine) ReloadState(ctx context.Context) error {
	return e.ReloadAll(ctx)
}

// ReloadKeys reloads the rawKeyToID and keyMetaMap from the keys table.
func (e *Engine) ReloadKeys(ctx context.Context) error {
	rows, err := e.db.SelectAllKeys(ctx)
	if err != nil {
		return fmt.Errorf("reload keys failed: %w", err)
	}

	rawMap := make(map[string]int64, len(rows))
	metaMap := make(map[int64]*KeyMeta, len(rows))

	for _, k := range rows {
		active := k.Active == 1
		displayKey := k.APIKey
		if len(displayKey) > 14 {
			displayKey = displayKey[:10] + "…" + displayKey[len(displayKey)-4:]
		}

		metaMap[k.ID] = &KeyMeta{
			ID:         k.ID,
			APIKey:     k.APIKey,
			DisplayKey: displayKey,
			Alias:      k.Alias,
			Active:     active,
		}

		if active {
			rawMap[k.APIKey] = k.ID
		}
	}

	e.mem.mu.Lock()
	e.mem.rawKeyToID = rawMap
	e.mem.keyMetaMap = metaMap
	e.mem.mu.Unlock()

	return nil
}

// ReloadQuotaRules reloads all quota rules from the quota_rules table.
func (e *Engine) ReloadQuotaRules(ctx context.Context) error {
	rows, err := e.db.SelectAllQuotaRules(ctx)
	if err != nil {
		return fmt.Errorf("reload quota rules failed: %w", err)
	}

	rulesMap := make(map[int64]*QuotaRule, len(rows))
	modelRulesMap := make(map[int64][]string, len(rows))

	for _, r := range rows {
		dailyLimit := int64(0)
		if r.DailyLimit != nil {
			dailyLimit = *r.DailyLimit
		}
		hardRatio := r.HardRatio
		if hardRatio <= 0 {
			hardRatio = 1.2
		}

		parsedModels := ParseModelWhitelist(r.ModelWhitelist)

		rule := &QuotaRule{
			KeyID:          r.KeyID,
			Enabled:        r.Enabled == 1,
			DailyLimit:     dailyLimit,
			HardRatio:      hardRatio,
			RPMLimit:       r.RPMLimit,
			ModelWhitelist: r.ModelWhitelist,
			ModelGroup:     r.ModelGroup,
			AllowedModels:  parsedModels,
		}

		rulesMap[r.KeyID] = rule
		if len(parsedModels) > 0 {
			modelRulesMap[r.KeyID] = parsedModels
		}
	}

	e.mem.mu.Lock()
	e.mem.quotaRules = rulesMap
	e.mem.keyModelRules = modelRulesMap
	e.mem.mu.Unlock()

	return nil
}

// ReloadPreloadedToday loads today's Shanghai-date usage from usage_daily.
func (e *Engine) ReloadPreloadedToday(ctx context.Context) error {
	today := ShanghaiDate(time.Now())
	rows, err := e.db.SelectDailyForDate(ctx, today)
	if err != nil {
		return fmt.Errorf("reload preloaded today failed: %w", err)
	}

	tokens := make(map[int64]int64)
	requests := make(map[int64]int64)

	for _, r := range rows {
		tokens[r.KeyID] += r.TotalTokens
		requests[r.KeyID] += r.Requests
	}

	e.mem.mu.Lock()
	e.mem.usageDate = today
	e.mem.preloadedTodayTokens = tokens
	e.mem.preloadedTodayRequests = requests
	e.mem.mu.Unlock()

	return nil
}

// ResolveKeyID returns the key ID for a plaintext API key if active.
func (e *Engine) ResolveKeyID(rawKey string) (int64, bool) {
	if rawKey == "" {
		return 0, false
	}
	e.mem.mu.RLock()
	defer e.mem.mu.RUnlock()
	id, ok := e.mem.rawKeyToID[rawKey]
	return id, ok
}

// GetKeyMeta returns cached metadata for a key ID.
func (e *Engine) GetKeyMeta(keyID int64) *KeyMeta {
	e.mem.mu.RLock()
	defer e.mem.mu.RUnlock()
	return e.mem.keyMetaMap[keyID]
}

// HasActiveKeys returns whether there is at least one active key configured in Console.
func (e *Engine) HasActiveKeys() bool {
	e.mem.mu.RLock()
	defer e.mem.mu.RUnlock()
	return len(e.mem.rawKeyToID) > 0
}

// CheckQuota enforces daily token quota limits with hard_ratio burst allowance.
func (e *Engine) CheckQuota(keyID int64) CheckResult {
	e.mem.mu.RLock()
	rule := e.mem.quotaRules[keyID]
	e.mem.mu.RUnlock()

	if rule == nil || !rule.Enabled || rule.DailyLimit <= 0 {
		return CheckResult{Allowed: true}
	}

	used := e.mem.GetTodayUsageForKey(keyID)
	hardRatio := rule.HardRatio
	if hardRatio <= 0 {
		hardRatio = 1.2
	}
	hardLimit := int64(float64(rule.DailyLimit) * hardRatio)

	if used >= hardLimit {
		meta := e.GetKeyMeta(keyID)
		aliasOrID := fmt.Sprintf("Key #%d", keyID)
		if meta != nil && meta.Alias != "" {
			aliasOrID = meta.Alias
		}

		reason := fmt.Sprintf("Daily token limit exceeded for %s. Used: %d, Limit: %d, Hard limit: %d",
			aliasOrID, used, rule.DailyLimit, hardLimit)

		return CheckResult{
			Allowed:   false,
			Reason:    reason,
			Usage:     used,
			Limit:     rule.DailyLimit,
			HardLimit: hardLimit,
		}
	}

	return CheckResult{
		Allowed:   true,
		Usage:     used,
		Limit:     rule.DailyLimit,
		HardLimit: hardLimit,
	}
}

// CheckRateLimit evaluates a 60-second sliding window of requests for the key.
func (e *Engine) CheckRateLimit(keyID int64) CheckResult {
	e.mem.mu.RLock()
	rule := e.mem.quotaRules[keyID]
	e.mem.mu.RUnlock()

	if rule == nil || rule.RPMLimit <= 0 {
		return CheckResult{Allowed: true}
	}

	now := time.Now().UnixMilli()
	windowStart := now - 60000

	e.mem.mu.Lock()
	defer e.mem.mu.Unlock()

	stamps := e.mem.rpmWindows[keyID]
	valid := 0
	for _, ts := range stamps {
		if ts > windowStart {
			stamps[valid] = ts
			valid++
		}
	}
	stamps = stamps[:valid]

	if len(stamps) >= rule.RPMLimit {
		e.mem.rpmWindows[keyID] = stamps
		return CheckResult{
			Allowed:  false,
			Reason:   fmt.Sprintf("Rate limit exceeded for this key. %d requests in the last 60s, limit: %d RPM.", len(stamps), rule.RPMLimit),
			RPMLimit: rule.RPMLimit,
		}
	}

	stamps = append(stamps, now)
	e.mem.rpmWindows[keyID] = stamps

	return CheckResult{
		Allowed:  true,
		RPMLimit: rule.RPMLimit,
	}
}

// CheckModelAllowed verifies whether the requested model satisfies the key's whitelist.
func (e *Engine) CheckModelAllowed(keyID int64, requestedModel string) CheckResult {
	if requestedModel == "" {
		return CheckResult{Allowed: true}
	}

	e.mem.mu.RLock()
	allowedList := e.mem.keyModelRules[keyID]
	e.mem.mu.RUnlock()

	if len(allowedList) == 0 {
		return CheckResult{Allowed: true}
	}

	if IsModelPermitted(requestedModel, allowedList) {
		return CheckResult{Allowed: true}
	}

	return CheckResult{
		Allowed: false,
		Reason:  fmt.Sprintf("Model '%s' is not permitted for this API key. Permitted models: %s", requestedModel, strings.Join(allowedList, ", ")),
	}
}

// FilterModelsForKey filters a slice of model names against the key's whitelist.
func (e *Engine) FilterModelsForKey(keyID int64, models []string) []string {
	if len(models) == 0 {
		return models
	}

	e.mem.mu.RLock()
	allowedList := e.mem.keyModelRules[keyID]
	e.mem.mu.RUnlock()

	if len(allowedList) == 0 {
		return models
	}

	var filtered []string
	for _, m := range models {
		if IsModelPermitted(m, allowedList) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// RecordRequest delegates asynchronous telemetry logging to OutboxFlusher.
func (e *Engine) RecordRequest(ctx context.Context, info *RequestInfo) error {
	return e.flusher.RecordRequest(ctx, info)
}
