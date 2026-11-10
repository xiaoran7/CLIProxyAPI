// 上游配额快照与窗口规范化查询 —— 拆自 queries_upstream.go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// InsertQuotaSnapshot saves an upstream quota percentage snapshot.
func (d *DB) InsertQuotaSnapshot(ctx context.Context, r *QuotaSnapshotRow) (int64, error) {
	query := `
INSERT INTO quota_snapshots (ts, auth_index, provider, account_email, window_name, remaining_percent, reset_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`
	res, err := d.ExecContext(ctx, query, r.TS, r.AuthIndex, r.Provider, r.AccountEmail, r.WindowName, r.RemainingPercent, r.ResetAt)
	if err != nil {
		return 0, fmt.Errorf("insert quota snapshot failed: %w", err)
	}
	return res.LastInsertId()
}

// SelectQuotaSnapshotsByAuth retrieves quota snapshots for an auth_index since timestamp.
func (d *DB) SelectQuotaSnapshotsByAuth(ctx context.Context, authIndex string, sinceTS int64) ([]QuotaSnapshotRow, error) {
	query := `
SELECT id, ts, auth_index, provider, account_email, window_name, remaining_percent, reset_at
FROM quota_snapshots WHERE auth_index = ? AND ts >= ? ORDER BY ts ASC
`
	rows, err := d.QueryContext(ctx, query, authIndex, sinceTS)
	if err != nil {
		return nil, fmt.Errorf("select quota snapshots failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []QuotaSnapshotRow
	for rows.Next() {
		var r QuotaSnapshotRow
		var email, winName sql.NullString
		var resetAt sql.NullInt64
		if err := rows.Scan(&r.ID, &r.TS, &r.AuthIndex, &r.Provider, &email, &winName, &r.RemainingPercent, &resetAt); err != nil {
			return nil, fmt.Errorf("scan quota snapshot failed: %w", err)
		}
		if email.Valid {
			v := email.String
			r.AccountEmail = &v
		}
		if winName.Valid {
			v := winName.String
			r.WindowName = &v
		}
		if resetAt.Valid {
			v := resetAt.Int64
			r.ResetAt = &v
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectLatestQuotaSnapshots retrieves the most recent quota snapshot for each auth_index and window_name.
func (d *DB) SelectLatestQuotaSnapshots(ctx context.Context, sinceTS int64) ([]QuotaSnapshotRow, error) {
	query := `
SELECT id, ts, auth_index, provider, account_email, window_name, remaining_percent, reset_at
FROM quota_snapshots
WHERE ts >= ?
ORDER BY ts DESC
`
	rows, err := d.QueryContext(ctx, query, sinceTS)
	if err != nil {
		return nil, fmt.Errorf("select latest quota snapshots failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	seen := make(map[string]bool)
	var list []QuotaSnapshotRow
	for rows.Next() {
		var r QuotaSnapshotRow
		var email, winName sql.NullString
		var resetAt sql.NullInt64
		if err := rows.Scan(&r.ID, &r.TS, &r.AuthIndex, &r.Provider, &email, &winName, &r.RemainingPercent, &resetAt); err != nil {
			return nil, fmt.Errorf("scan latest quota snapshot failed: %w", err)
		}
		if email.Valid {
			v := email.String
			r.AccountEmail = &v
		}
		if winName.Valid {
			v := winName.String
			r.WindowName = &v
		}
		if resetAt.Valid {
			v := resetAt.Int64
			r.ResetAt = &v
		}

		key := r.AuthIndex
		if r.WindowName != nil {
			key += "|" + normalizeQuotaWindowKey(*r.WindowName)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// normalizeQuotaWindowKey folds the two window-name shapes this table has held
// into one semantic key, so the newest row wins instead of both being returned.
//
// Older writers stored "Gemini Models - Five Hour Limit Remaining"; newer ones
// store "Gemini Models:5h". Deduping on the raw name kept both, and the stale
// 100% row rendered as a second meter beside the real one.
func normalizeQuotaWindowKey(windowName string) string {
	name := strings.TrimSpace(windowName)
	if name == "" {
		return ""
	}
	group, window := name, ""
	if idx := strings.Index(name, ":"); idx >= 0 {
		group, window = strings.TrimSpace(name[:idx]), strings.TrimSpace(name[idx+1:])
	} else if idx := strings.Index(name, " - "); idx >= 0 {
		group, window = strings.TrimSpace(name[:idx]), strings.TrimSpace(name[idx+3:])
	}
	return canonicalQuotaGroup(group) + "|" + canonicalQuotaWindow(window)
}

// canonicalQuotaGroup maps legacy and modern group spellings onto canonical group identifiers.
func canonicalQuotaGroup(group string) string {
	gLower := strings.ToLower(strings.TrimSpace(group))
	switch {
	case strings.Contains(gLower, "argon") || strings.Contains(gLower, "gemini 4") ||
		strings.Contains(gLower, "gemini-4") || strings.Contains(gLower, "gemini4"):
		return "models/gemini4_argon"
	case strings.Contains(gLower, "gemini"):
		return "models/gemini"
	case strings.Contains(gLower, "claude") || strings.Contains(gLower, "gpt") || strings.Contains(gLower, "3p"):
		return "models/claude"
	case strings.Contains(gLower, "codex"):
		return "codex"
	default:
		return strings.TrimSpace(group)
	}
}

// canonicalQuotaWindow maps the window spellings both writers used onto one token.
func canonicalQuotaWindow(window string) string {
	w := strings.ToLower(strings.TrimSpace(window))
	switch {
	case w == "":
		return ""
	case w == "5h", strings.Contains(w, "five hour"), strings.Contains(w, "five-hour"),
		strings.Contains(w, "5 hour"), strings.Contains(w, "5-hour"), strings.Contains(w, "5h"):
		return "5h"
	case w == "weekly", strings.Contains(w, "week"):
		return "weekly"
	case w == "monthly", strings.Contains(w, "month"):
		return "monthly"
	}
	return w
}
