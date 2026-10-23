package api

import (
	"sync"
	"time"
)

var (
	upstreamCacheMu          sync.RWMutex
	cachedUpstreamQuotasResp *UpstreamQuotasResponse
	cachedUpstreamAt         time.Time
)

// InvalidateUpstreamQuotaCache forces the next fetch to query fresh data
func InvalidateUpstreamQuotaCache() {
	upstreamCacheMu.Lock()
	defer upstreamCacheMu.Unlock()
	cachedUpstreamQuotasResp = nil
	cachedUpstreamAt = time.Time{}
}

// CachedAccountQuota stores parsed on-demand quota snapshots in memory
type CachedAccountQuota struct {
	Groups           []CpaQuotaGroup
	PlanType         string
	PlanName         string
	RawError         string
	UpdatedAt        time.Time
	TotalCredits     *float64
	RemainingCredits *float64
	UsedCredits      *float64
	ExpiresAt        string
}

var (
	accountQuotaCacheMu sync.RWMutex
	accountQuotaCache   = make(map[string]CachedAccountQuota)

	probeThrottleMu sync.Mutex
	probeLastTime   = make(map[string]time.Time)
)

func shouldProbeAccount(key string, minInterval time.Duration) bool {
	if key == "" {
		return false
	}
	probeThrottleMu.Lock()
	defer probeThrottleMu.Unlock()
	last, ok := probeLastTime[key]
	if ok && time.Since(last) < minInterval {
		return false
	}
	probeLastTime[key] = time.Now()
	return true
}
