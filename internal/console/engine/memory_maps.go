package engine

import (
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
)

var shanghaiLocation = time.FixedZone("CST", 8*3600)

// ShanghaiDate returns the YYYY-MM-DD date formatted in Asia/Shanghai timezone.
func ShanghaiDate(t time.Time) string {
	return t.In(shanghaiLocation).Format("2006-01-02")
}

// MemoryState coordinates in-memory caches, rate limits, and live counters.
type MemoryState struct {
	mu sync.RWMutex

	rawKeyToID    map[string]int64
	keyMetaMap    map[int64]*KeyMeta
	quotaRules    map[int64]*QuotaRule
	keyModelRules map[int64][]string
	rpmWindows    map[int64][]int64

	usageDate                string
	preloadedTodayTokens     map[int64]int64
	preloadedTodayRequests   map[int64]int64
	liveTodayTokensByKeyID   map[int64]int64
	liveTodayRequestsByKeyID map[int64]int64
	pendingEvents            []*db.RequestEventRow

	pendingMu sync.Mutex
}

// NewMemoryState instantiates a clean in-memory state tracker.
func NewMemoryState() *MemoryState {
	today := ShanghaiDate(time.Now())
	return &MemoryState{
		rawKeyToID:               make(map[string]int64),
		keyMetaMap:               make(map[int64]*KeyMeta),
		quotaRules:               make(map[int64]*QuotaRule),
		keyModelRules:            make(map[int64][]string),
		rpmWindows:               make(map[int64][]int64),
		usageDate:                today,
		preloadedTodayTokens:     make(map[int64]int64),
		preloadedTodayRequests:   make(map[int64]int64),
		liveTodayTokensByKeyID:   make(map[int64]int64),
		liveTodayRequestsByKeyID: make(map[int64]int64),
		pendingEvents:            make([]*db.RequestEventRow, 0),
	}
}

// checkDateRolloverLocked checks if the Shanghai date has changed and resets daily maps if so.
// Must be called with m.mu held.
func (m *MemoryState) checkDateRolloverLocked(now time.Time) {
	today := ShanghaiDate(now)
	if today != m.usageDate {
		m.usageDate = today
		m.preloadedTodayTokens = make(map[int64]int64)
		m.preloadedTodayRequests = make(map[int64]int64)
		m.liveTodayTokensByKeyID = make(map[int64]int64)
		m.liveTodayRequestsByKeyID = make(map[int64]int64)
	}
}

// AddLiveUsage accumulates a request event into live memory maps.
func (m *MemoryState) AddLiveUsage(event *db.RequestEventRow) {
	date := ShanghaiDate(time.Unix(event.TS, 0))
	var keyID int64
	if event.KeyID != nil {
		keyID = *event.KeyID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.checkDateRolloverLocked(time.Now())

	if date == m.usageDate {
		m.liveTodayTokensByKeyID[keyID] += event.TotalTokens
		m.liveTodayRequestsByKeyID[keyID] += 1
	}
}

// AddPendingEvent caches an event in memory when synchronous outbox insert fails.
func (m *MemoryState) AddPendingEvent(event *db.RequestEventRow) {
	m.pendingMu.Lock()
	m.pendingEvents = append(m.pendingEvents, event)
	m.pendingMu.Unlock()

	m.AddLiveUsage(event)
}

// DrainPendingEvents atomically extracts all queued pending events.
func (m *MemoryState) DrainPendingEvents() []*db.RequestEventRow {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	if len(m.pendingEvents) == 0 {
		return nil
	}
	events := m.pendingEvents
	m.pendingEvents = make([]*db.RequestEventRow, 0)
	return events
}

// GetTodayUsageForKey returns total tokens consumed today (preloaded + live).
func (m *MemoryState) GetTodayUsageForKey(keyID int64) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkDateRolloverLocked(time.Now())
	return m.preloadedTodayTokens[keyID] + m.liveTodayTokensByKeyID[keyID]
}

// GetTodayRequestsForKey returns total requests consumed today (preloaded + live).
func (m *MemoryState) GetTodayRequestsForKey(keyID int64) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkDateRolloverLocked(time.Now())
	return m.preloadedTodayRequests[keyID] + m.liveTodayRequestsByKeyID[keyID]
}

// CommitFlushedUsage updates preloaded baseline usage and decrements live usage for events committed to SQLite.
// This preserves live usage from concurrent in-flight requests and avoids quota drops.
func (m *MemoryState) CommitFlushedUsage(flushedTokens map[int64]int64, flushedRequests map[int64]int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.checkDateRolloverLocked(time.Now())

	for keyID, tokens := range flushedTokens {
		m.preloadedTodayTokens[keyID] += tokens
		m.liveTodayTokensByKeyID[keyID] -= tokens
		if m.liveTodayTokensByKeyID[keyID] < 0 {
			m.liveTodayTokensByKeyID[keyID] = 0
		}
	}
	for keyID, reqs := range flushedRequests {
		m.preloadedTodayRequests[keyID] += reqs
		m.liveTodayRequestsByKeyID[keyID] -= reqs
		if m.liveTodayRequestsByKeyID[keyID] < 0 {
			m.liveTodayRequestsByKeyID[keyID] = 0
		}
	}
}

// ResetLiveCounters resets transient live counters and sliding RPM windows (useful for testing and rollover).
func (m *MemoryState) ResetLiveCounters() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rpmWindows = make(map[int64][]int64)
	m.liveTodayTokensByKeyID = make(map[int64]int64)
	m.liveTodayRequestsByKeyID = make(map[int64]int64)
}
