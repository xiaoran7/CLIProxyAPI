package auth

import (
	"context"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

const (
	tierThresholdLow   = 0.80 // 80% usage marks the "low" tier
	tierThresholdSpent = 0.98 // 98% usage marks the "spent" tier
)

// SmartSelector prioritizes accounts whose quota resets soonest, because quota
// unspent before a reset is wasted, while accounts resetting later retain their allowance.
// Accounts are tiered into fine (<80%), low (80-98%), and spent (>=98%). Within the
// same tier and reset hour, stable ordering preserves Prompt Cache affinity.
type SmartSelector struct{}

// Pick implements the Selector interface for SmartSelector.
func (s *SmartSelector) Pick(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, auths []*Auth) (*Auth, error) {
	_ = opts
	now := time.Now()
	available, err := getSelectorAvailableAuths(ctx, auths, provider, model, now)
	if err != nil {
		return nil, err
	}
	if len(available) <= 1 {
		return available[0], nil
	}

	available = preferCodexWebsocketAuths(ctx, provider, available)

	// Collect quota info for candidates
	infoMap := make(map[string]authQuotaInfo, len(available))
	for _, a := range available {
		if a != nil {
			infoMap[a.ID] = extractAuthQuotaInfo(a, model, now)
		}
	}

	// Segment into tiers: fine, low, spent
	var fine, low, spent []*Auth
	for _, a := range available {
		info := infoMap[a.ID]
		switch {
		case info.usedRatio >= tierThresholdSpent:
			spent = append(spent, a)
		case info.usedRatio >= tierThresholdLow:
			low = append(low, a)
		default:
			fine = append(fine, a)
		}
	}

	// In fine tier: sort by soonest reset deadline (truncated to the hour to avoid thrashing),
	// preserving stable order within the same renewal hour for Prompt Cache affinity.
	sort.SliceStable(fine, func(i, j int) bool {
		infoI := infoMap[fine[i].ID]
		infoJ := infoMap[fine[j].ID]

		// Accounts with known reset deadlines come before accounts with unknown deadlines
		if infoI.hasResetAt != infoJ.hasResetAt {
			return infoI.hasResetAt
		}
		if !infoI.hasResetAt {
			return false // retain stable order
		}

		hourI := infoI.resetAt.Truncate(time.Hour)
		hourJ := infoJ.resetAt.Truncate(time.Hour)
		if !hourI.Equal(hourJ) {
			return hourI.Before(hourJ)
		}
		// In the same renewal hour, lower usage goes first
		return infoI.usedRatio < infoJ.usedRatio
	})

	// Low and spent tiers are sorted by lowest used ratio
	for _, tier := range [][]*Auth{low, spent} {
		sort.SliceStable(tier, func(i, j int) bool {
			return infoMap[tier[i].ID].usedRatio < infoMap[tier[j].ID].usedRatio
		})
	}

	ordered := append(append(fine, low...), spent...)
	ordered = ApplySinkOrdering(ordered)
	return ordered[0], nil
}

// PaceSelector ranks accounts by hourly remaining quota until reset (Remaining / HoursUntilReset).
// Accounts with the highest pace have the most headroom to absorb requests before resetting.
// Accounts within a 10% pace tolerance preserve stable order to maximize Prompt Cache affinity.
type PaceSelector struct{}

// Pick implements the Selector interface for PaceSelector.
func (s *PaceSelector) Pick(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, auths []*Auth) (*Auth, error) {
	_ = opts
	now := time.Now()
	available, err := getSelectorAvailableAuths(ctx, auths, provider, model, now)
	if err != nil {
		return nil, err
	}
	if len(available) <= 1 {
		return available[0], nil
	}

	available = preferCodexWebsocketAuths(ctx, provider, available)

	infoMap := make(map[string]authQuotaInfo, len(available))
	for _, a := range available {
		if a != nil {
			infoMap[a.ID] = extractAuthQuotaInfo(a, model, now)
		}
	}

	// Segment into tiers
	var fine, low, spent []*Auth
	for _, a := range available {
		info := infoMap[a.ID]
		switch {
		case info.usedRatio >= tierThresholdSpent:
			spent = append(spent, a)
		case info.usedRatio >= tierThresholdLow:
			low = append(low, a)
		default:
			fine = append(fine, a)
		}
	}

	// Sort fine tier by hourly pace (descending)
	sort.SliceStable(fine, func(i, j int) bool {
		infoI := infoMap[fine[i].ID]
		infoJ := infoMap[fine[j].ID]

		paceI := calculateHourlyPace(infoI, now)
		paceJ := calculateHourlyPace(infoJ, now)

		// If pace is within 10% tolerance, maintain stable relative order for prompt caching
		maxPace := math.Max(paceI, paceJ)
		if maxPace > 0 && math.Abs(paceI-paceJ)/maxPace <= 0.10 {
			return false // keep stable order
		}
		return paceI > paceJ
	})

	for _, tier := range [][]*Auth{low, spent} {
		sort.SliceStable(tier, func(i, j int) bool {
			return infoMap[tier[i].ID].usedRatio < infoMap[tier[j].ID].usedRatio
		})
	}

	ordered := append(append(fine, low...), spent...)
	ordered = ApplySinkOrdering(ordered)
	return ordered[0], nil
}

type authQuotaInfo struct {
	usedRatio  float64
	resetAt    time.Time
	hasResetAt bool
}

func calculateHourlyPace(info authQuotaInfo, now time.Time) float64 {
	rem := math.Max(0.01, 1.0-info.usedRatio)
	hours := 24.0 // default window of 24h if resetAt not explicitly set
	if info.hasResetAt {
		until := info.resetAt.Sub(now).Hours()
		if until > 0.05 {
			hours = until
		} else {
			hours = 0.05 // avoid divide by zero when reset is imminent
		}
	}
	return rem / hours
}

// extractAuthQuotaInfo parses watermark signals, recovery timestamps, and metadata.
func extractAuthQuotaInfo(auth *Auth, model string, now time.Time) authQuotaInfo {
	info := authQuotaInfo{
		usedRatio: 0.0,
	}

	if auth == nil {
		return info
	}

	// Check recovery time on Quota or ModelState
	if !auth.Quota.NextRecoverAt.IsZero() && auth.Quota.NextRecoverAt.After(now) {
		info.resetAt = auth.Quota.NextRecoverAt
		info.hasResetAt = true
	}
	if auth.ModelStates != nil {
		if ms, ok := auth.ModelStates[model]; ok && ms != nil {
			if !ms.Quota.NextRecoverAt.IsZero() && ms.Quota.NextRecoverAt.After(now) {
				if !info.hasResetAt || ms.Quota.NextRecoverAt.Before(info.resetAt) {
					info.resetAt = ms.Quota.NextRecoverAt
					info.hasResetAt = true
				}
			}
		}
	}

	// Inspect QuotaState.Signals (populated by passive response header observation)
	signals := auth.Quota.Signals
	if len(signals) == 0 && auth.ModelStates != nil {
		if ms, ok := auth.ModelStates[model]; ok && ms != nil {
			signals = ms.Quota.Signals
		}
	}

	if len(signals) > 0 {
		for k, v := range signals {
			lowerK := strings.ToLower(k)
			// Check for used percentage
			if strings.Contains(lowerK, "used-percent") || strings.Contains(lowerK, "utilization") {
				if parsed, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64); err == nil {
					ratio := parsed
					if ratio > 1.0 {
						ratio = ratio / 100.0
					}
					if ratio > info.usedRatio {
						info.usedRatio = ratio
					}
				}
			}
			// Check for reset headers
			if strings.Contains(lowerK, "reset-after-seconds") || lowerK == "retry-after" {
				if secs, err := strconv.ParseFloat(v, 64); err == nil && secs > 0 {
					candidateReset := now.Add(time.Duration(secs * float64(time.Second)))
					if !info.hasResetAt || candidateReset.Before(info.resetAt) {
						info.resetAt = candidateReset
						info.hasResetAt = true
					}
				}
			}
		}
	}

	// Check metadata for quota / points balance
	if auth.Metadata != nil {
		if u, ok := auth.Metadata["used_points"].(float64); ok {
			if tot, okTot := auth.Metadata["total_points"].(float64); okTot && tot > 0 {
				ratio := u / tot
				if ratio > info.usedRatio {
					info.usedRatio = ratio
				}
			}
		} else if rem, ok := auth.Metadata["remaining_points"].(float64); ok {
			if tot, okTot := auth.Metadata["total_points"].(float64); okTot && tot > 0 {
				ratio := 1.0 - (rem / tot)
				if ratio > info.usedRatio {
					info.usedRatio = ratio
				}
			}
		}

		if resetRaw, ok := auth.Metadata["quota_reset_at"].(string); ok && resetRaw != "" {
			if parsedTime, err := time.Parse(time.RFC3339, resetRaw); err == nil && parsedTime.After(now) {
				if !info.hasResetAt || parsedTime.Before(info.resetAt) {
					info.resetAt = parsedTime
					info.hasResetAt = true
				}
			}
		}
	}

	return info
}
