package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
)

// tosScanTimeout bounds how long the console may spend scanning rotated logs.
const tosScanTimeout = 20 * time.Second

// tosScanCacheTTL keeps the 15s console poll from re-reading every rotated log
// on each tick. The underlying enforcement records change on the order of
// hours, so a short cache loses nothing and protects the serving path.
const tosScanCacheTTL = 45 * time.Second

var (
	tosScanCacheMu  sync.Mutex
	tosScanCachedAt time.Time
	tosScanCached   TOSLogScanResult
)

// scanTOSBanLogsCached returns a recently scanned result when one exists.
func scanTOSBanLogsCached() TOSLogScanResult {
	tosScanCacheMu.Lock()
	defer tosScanCacheMu.Unlock()
	if !tosScanCachedAt.IsZero() && time.Since(tosScanCachedAt) < tosScanCacheTTL {
		return tosScanCached
	}
	result := ScanTOSBanLogs("")
	tosScanCachedAt = time.Now()
	tosScanCached = result
	return result
}

// invalidateTOSScanCache forces the next request to re-read the logs.
func invalidateTOSScanCache() {
	tosScanCacheMu.Lock()
	defer tosScanCacheMu.Unlock()
	tosScanCachedAt = time.Time{}
}

// TOSAccountStats summarises how an account behaved around its enforcement so an
// operator can tell a freshly banned credential from one that died days ago.
type TOSAccountStats struct {
	Account        string         `json:"account"`
	AuthFile       string         `json:"auth_file,omitempty"`
	Provider       string         `json:"provider,omitempty"`
	FirstBannedAt  int64          `json:"first_banned_at"`
	LastBannedAt   int64          `json:"last_banned_at"`
	BanCount       int            `json:"ban_count"`
	LastSuccessAt  int64          `json:"last_success_at"`
	Success24h     int64          `json:"success_24h"`
	Forbidden24h   int64          `json:"forbidden_24h"`
	Success7d      int64          `json:"success_7d"`
	Forbidden7d    int64          `json:"forbidden_7d"`
	TotalSuccess   int64          `json:"total_success"`
	TotalForbidden int64          `json:"total_forbidden"`
	AuthIndex      string         `json:"auth_index,omitempty"`
	AuthStatus     string         `json:"auth_status,omitempty"`
	AuthDisabled   bool           `json:"auth_disabled"`
	Models         []string       `json:"models"`
	Records        []TOSBanRecord `json:"records"`
}

// TOSBanSummary is the page-level roll-up.
type TOSBanSummary struct {
	BannedAccounts int `json:"banned_accounts"`
	// Bans24h/Bans7d count the enforcement records recovered from the log. They
	// are the authoritative figure: gate.db routinely reports zero 403s for a
	// banned account because the conductor retries onto another credential, so
	// the client-facing status never becomes 403.
	Bans24h        int    `json:"bans_24h"`
	Bans7d         int    `json:"bans_7d"`
	Forbidden24h   int64  `json:"forbidden_24h"`
	Forbidden7d    int64  `json:"forbidden_7d"`
	ForbiddenTotal int64  `json:"forbidden_total"`
	RecordCount    int    `json:"record_count"`
	TruncatedCount int    `json:"truncated_records"`
	LogDir         string `json:"log_dir"`
	FilesScanned   int    `json:"files_scanned"`
	ScanError      string `json:"scan_error,omitempty"`
	ScannedAt      int64  `json:"scanned_at"`
}

// authFileInfo is the subset of the CPA /auth-files payload this page needs.
type authFileInfo struct {
	AuthIndex string `json:"auth_index"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	Disabled  bool   `json:"disabled"`
}

type authFilesPayload struct {
	Files []authFileInfo `json:"files"`
}

// accountBucket accumulates records for one account while scanning.
type accountBucket struct {
	stats TOSAccountStats
	seen  map[string]struct{}
}

// HandleGetTOSBans returns the terms-of-service enforcement ledger. Records come
// from the application log, the only place the upstream TOS marker survives; the
// 200/403 comparison comes from gate.db.
func HandleGetTOSBans(eng *engine.Engine, cpa *CpaClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")

		ctx, cancel := context.WithTimeout(c.Request.Context(), tosScanTimeout)
		defer cancel()

		// A forced refresh (?refresh=1) bypasses the cache so an operator can
		// re-read the logs on demand.
		if strings.TrimSpace(c.Query("refresh")) != "" {
			invalidateTOSScanCache()
		}
		scan := scanTOSBanLogsCached()

		// The log only carries the auth file name, so resolve it against CPA's
		// auth-file listing to recover the index and the console-facing status.
		authByFile := make(map[string]authFileInfo)
		if cpa != nil {
			var payload authFilesPayload
			if err := cpa.RequestJSON(ctx, http.MethodGet, "/auth-files", nil, "v0", &payload); err == nil {
				for _, f := range payload.Files {
					name := strings.TrimSpace(f.Name)
					if name == "" {
						name = strings.TrimSpace(f.ID)
					}
					if name != "" {
						authByFile[strings.ToLower(name)] = f
					}
				}
			}
		}

		buckets, order := groupTOSRecordsByAccount(scan.Records, authByFile)
		fillTOSDatabaseStats(ctx, eng, buckets, order)

		accounts := make([]TOSAccountStats, 0, len(order))
		for _, key := range order {
			bucket := buckets[key]
			if bucket.stats.Models == nil {
				bucket.stats.Models = []string{}
			}
			if bucket.stats.Records == nil {
				bucket.stats.Records = []TOSBanRecord{}
			}
			accounts = append(accounts, bucket.stats)
		}
		sort.SliceStable(accounts, func(i, j int) bool {
			return accounts[i].LastBannedAt > accounts[j].LastBannedAt
		})

		c.JSON(http.StatusOK, gin.H{
			"summary":  buildTOSSummary(accounts, scan),
			"accounts": accounts,
		})
	}
}

// groupTOSRecordsByAccount folds the flat log records into per-account stats.
func groupTOSRecordsByAccount(records []TOSBanRecord, authByFile map[string]authFileInfo) (map[string]*accountBucket, []string) {
	buckets := make(map[string]*accountBucket, len(records))
	order := make([]string, 0, len(records))
	for _, rec := range records {
		key := strings.ToLower(strings.TrimSpace(rec.Account))
		bucket, ok := buckets[key]
		if !ok {
			bucket = &accountBucket{
				stats: TOSAccountStats{
					Account:  rec.Account,
					AuthFile: rec.AuthFile,
					Provider: rec.Provider,
					Models:   []string{},
				},
				seen: make(map[string]struct{}),
			}
			buckets[key] = bucket
			order = append(order, key)
		}
		bucket.stats.Records = append(bucket.stats.Records, rec)
		bucket.stats.BanCount++
		if bucket.stats.FirstBannedAt == 0 || rec.TS < bucket.stats.FirstBannedAt {
			bucket.stats.FirstBannedAt = rec.TS
		}
		if rec.TS > bucket.stats.LastBannedAt {
			bucket.stats.LastBannedAt = rec.TS
		}
		if rec.Model != "" {
			if _, dup := bucket.seen[rec.Model]; !dup {
				bucket.seen[rec.Model] = struct{}{}
				bucket.stats.Models = append(bucket.stats.Models, rec.Model)
			}
		}
		if info, found := authByFile[strings.ToLower(rec.AuthFile)]; found {
			bucket.stats.AuthIndex = info.AuthIndex
			bucket.stats.AuthStatus = info.Status
			bucket.stats.AuthDisabled = info.Disabled
		}
	}
	return buckets, order
}

// buildTOSSummary produces the top-of-page roll-up.
func buildTOSSummary(accounts []TOSAccountStats, scan TOSLogScanResult) TOSBanSummary {
	summary := TOSBanSummary{
		BannedAccounts: len(accounts),
		RecordCount:    len(scan.Records),
		LogDir:         scan.LogDir,
		FilesScanned:   scan.FilesRead,
		ScannedAt:      time.Now().Unix(),
	}
	if scan.Err != nil {
		summary.ScanError = scan.Err.Error()
	}
	now := summary.ScannedAt
	for _, acct := range accounts {
		summary.Forbidden24h += acct.Forbidden24h
		summary.Forbidden7d += acct.Forbidden7d
		summary.ForbiddenTotal += acct.TotalForbidden
		for _, rec := range acct.Records {
			if rec.Truncated {
				summary.TruncatedCount++
			}
			if now-rec.TS <= 24*3600 {
				summary.Bans24h++
			}
			if now-rec.TS <= 7*86400 {
				summary.Bans7d++
			}
		}
	}
	return summary
}

// fillTOSDatabaseStats attaches gate.db counters to every banned account. The
// log is the only source of the ban itself, but the database is what tells us
// whether the account still produced successful calls afterwards.
func fillTOSDatabaseStats(ctx context.Context, eng *engine.Engine, buckets map[string]*accountBucket, order []string) {
	if eng == nil || eng.DB() == nil || len(order) == 0 {
		return
	}
	now := time.Now().Unix()
	since24h := now - 24*3600
	since7d := now - 7*86400

	for _, key := range order {
		bucket := buckets[key]
		authIndex := strings.TrimSpace(bucket.stats.AuthIndex)
		if authIndex == "" {
			continue
		}
		row := queryTOSAccountCounters(ctx, eng, authIndex, since24h, since7d)
		bucket.stats.LastSuccessAt = row.lastSuccess
		bucket.stats.Success24h = row.success24h
		bucket.stats.Forbidden24h = row.forbidden24h
		bucket.stats.Success7d = row.success7d
		bucket.stats.Forbidden7d = row.forbidden7d
		bucket.stats.TotalSuccess = row.totalSuccess
		bucket.stats.TotalForbidden = row.totalForbidden
	}
}

// tosAccountCounters carries the counts read from request_events.
type tosAccountCounters struct {
	lastSuccess    int64
	success24h     int64
	forbidden24h   int64
	success7d      int64
	forbidden7d    int64
	totalSuccess   int64
	totalForbidden int64
}

// queryTOSAccountCounters aggregates request_events for one upstream account.
func queryTOSAccountCounters(ctx context.Context, eng *engine.Engine, authIndex string, since24h, since7d int64) tosAccountCounters {
	var out tosAccountCounters
	database := eng.DB()

	// One pass produces every window and the last-success timestamp.
	query := `SELECT
		COALESCE(SUM(CASE WHEN status = 200 THEN 1 ELSE 0 END), 0) AS total_success,
		COALESCE(SUM(CASE WHEN status = 403 THEN 1 ELSE 0 END), 0) AS total_forbidden,
		COALESCE(SUM(CASE WHEN status = 200 AND ts >= ? THEN 1 ELSE 0 END), 0) AS success24h,
		COALESCE(SUM(CASE WHEN status = 403 AND ts >= ? THEN 1 ELSE 0 END), 0) AS forbidden24h,
		COALESCE(SUM(CASE WHEN status = 200 AND ts >= ? THEN 1 ELSE 0 END), 0) AS success7d,
		COALESCE(SUM(CASE WHEN status = 403 AND ts >= ? THEN 1 ELSE 0 END), 0) AS forbidden7d,
		COALESCE(MAX(CASE WHEN status = 200 THEN ts END), 0) AS last_success
		FROM request_events WHERE auth_index = ?`

	row := database.QueryRowContext(ctx, query, since24h, since24h, since7d, since7d, authIndex)
	if err := row.Scan(
		&out.totalSuccess,
		&out.totalForbidden,
		&out.success24h,
		&out.forbidden24h,
		&out.success7d,
		&out.forbidden7d,
		&out.lastSuccess,
	); err != nil {
		return tosAccountCounters{}
	}
	return out
}
