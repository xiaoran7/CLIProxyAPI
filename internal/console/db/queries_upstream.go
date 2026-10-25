package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// UpsertUpstreamDailyStatTx records daily upstream aggregate in upstream_daily_stats inside a transaction.
func (d *DB) UpsertUpstreamDailyStatTx(ctx context.Context, tx *sql.Tx, r *UpstreamDailyStatRow) error {
	query := `
INSERT INTO upstream_daily_stats (
  date, auth_index, provider, requests, success_count, failure_count,
  prompt_tokens, completion_tokens, cached_tokens, total_tokens
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(date, auth_index) DO UPDATE SET
  provider = CASE WHEN excluded.provider != '' THEN excluded.provider ELSE upstream_daily_stats.provider END,
  requests = requests + excluded.requests,
  success_count = success_count + excluded.success_count,
  failure_count = failure_count + excluded.failure_count,
  prompt_tokens = prompt_tokens + excluded.prompt_tokens,
  completion_tokens = completion_tokens + excluded.completion_tokens,
  cached_tokens = cached_tokens + excluded.cached_tokens,
  total_tokens = total_tokens + excluded.total_tokens
`
	_, err := tx.ExecContext(ctx, query,
		r.Date, r.AuthIndex, r.Provider, r.Requests, r.SuccessCount, r.FailureCount,
		r.PromptTokens, r.CompletionTokens, r.CachedTokens, r.TotalTokens,
	)
	if err != nil {
		return fmt.Errorf("upsert upstream daily stat tx failed: %w", err)
	}
	return nil
}

// SelectUpstreamDailyAgg aggregates lifetime statistics for all auth_index credentials.
func (d *DB) SelectUpstreamDailyAgg(ctx context.Context) ([]UpstreamDailyStatRow, error) {
	query := `
SELECT
  auth_index,
  SUM(requests) AS requests,
  SUM(success_count) AS success_count,
  SUM(failure_count) AS failure_count,
  SUM(prompt_tokens) AS prompt_tokens,
  SUM(completion_tokens) AS completion_tokens,
  SUM(cached_tokens) AS cached_tokens,
  SUM(total_tokens) AS total_tokens
FROM upstream_daily_stats
WHERE auth_index != ''
GROUP BY auth_index
`
	rows, err := d.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select upstream daily agg failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []UpstreamDailyStatRow
	for rows.Next() {
		var r UpstreamDailyStatRow
		if err := rows.Scan(
			&r.AuthIndex, &r.Requests, &r.SuccessCount, &r.FailureCount,
			&r.PromptTokens, &r.CompletionTokens, &r.CachedTokens, &r.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("scan upstream daily agg failed: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectUpstreamDailyAggSince aggregates statistics for auth_index on or after sinceDate.
func (d *DB) SelectUpstreamDailyAggSince(ctx context.Context, sinceDate string) ([]UpstreamDailyStatRow, error) {
	query := `
SELECT
  auth_index,
  SUM(requests) AS requests,
  SUM(success_count) AS success_count,
  SUM(failure_count) AS failure_count,
  SUM(prompt_tokens) AS prompt_tokens,
  SUM(completion_tokens) AS completion_tokens,
  SUM(cached_tokens) AS cached_tokens,
  SUM(total_tokens) AS total_tokens
FROM upstream_daily_stats
WHERE auth_index != '' AND date >= ?
GROUP BY auth_index
`
	rows, err := d.QueryContext(ctx, query, sinceDate)
	if err != nil {
		return nil, fmt.Errorf("select upstream daily agg since failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []UpstreamDailyStatRow
	for rows.Next() {
		var r UpstreamDailyStatRow
		if err := rows.Scan(
			&r.AuthIndex, &r.Requests, &r.SuccessCount, &r.FailureCount,
			&r.PromptTokens, &r.CompletionTokens, &r.CachedTokens, &r.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("scan upstream daily agg since failed: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectUpstreamDailyForDate aggregates statistics for all auth_index credentials on a specific date.
func (d *DB) SelectUpstreamDailyForDate(ctx context.Context, date string) ([]UpstreamDailyStatRow, error) {
	query := `
SELECT
  auth_index,
  SUM(requests) AS requests,
  SUM(success_count) AS success_count,
  SUM(failure_count) AS failure_count,
  SUM(prompt_tokens) AS prompt_tokens,
  SUM(completion_tokens) AS completion_tokens,
  SUM(cached_tokens) AS cached_tokens,
  SUM(total_tokens) AS total_tokens
FROM upstream_daily_stats
WHERE auth_index != '' AND date = ?
GROUP BY auth_index
`
	rows, err := d.QueryContext(ctx, query, date)
	if err != nil {
		return nil, fmt.Errorf("select upstream daily for date failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []UpstreamDailyStatRow
	for rows.Next() {
		var r UpstreamDailyStatRow
		if err := rows.Scan(
			&r.AuthIndex, &r.Requests, &r.SuccessCount, &r.FailureCount,
			&r.PromptTokens, &r.CompletionTokens, &r.CachedTokens, &r.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("scan upstream daily for date failed: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// UpstreamWindowStat holds aggregated traffic tokens and request counts in a time window.
type UpstreamWindowStat struct {
	Tokens   int64
	Requests int64
}

// SelectUpstreamWindowStats returns token and request aggregates from request_events since sinceTS.
func (d *DB) SelectUpstreamWindowStats(ctx context.Context, sinceTS int64) (map[string]UpstreamWindowStat, error) {
	query := `
SELECT auth_index, SUM(total_tokens), COUNT(1)
FROM request_events
WHERE ts >= ? AND auth_index != ''
GROUP BY auth_index
`
	rows, err := d.QueryContext(ctx, query, sinceTS)
	if err != nil {
		return nil, fmt.Errorf("select upstream window stats failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	res := make(map[string]UpstreamWindowStat)
	for rows.Next() {
		var authIndex string
		var tokens, reqs int64
		if err := rows.Scan(&authIndex, &tokens, &reqs); err != nil {
			return nil, fmt.Errorf("scan upstream window stats failed: %w", err)
		}
		if authIndex != "" {
			res[authIndex] = UpstreamWindowStat{
				Tokens:   tokens,
				Requests: reqs,
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return res, nil
}

// UpstreamEventAggRow represents aggregated traffic metrics for an auth_index from request_events.
type UpstreamEventAggRow struct {
	AuthIndex        string `json:"auth_index"`
	Requests         int64  `json:"requests"`
	SuccessCount     int64  `json:"success_count"`
	FailureCount     int64  `json:"failure_count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	CachedTokens     int64  `json:"cached_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

// SelectUpstreamEventAggSince aggregates request_events by auth_index on or after sinceTS.
func (d *DB) SelectUpstreamEventAggSince(ctx context.Context, sinceTS int64) ([]UpstreamEventAggRow, error) {
	query := `
SELECT
  auth_index,
  COUNT(*) AS requests,
  COALESCE(SUM(CASE WHEN status >= 200 AND status < 400 THEN 1 ELSE 0 END), 0) AS success_count,
  COALESCE(SUM(CASE WHEN status >= 400 OR status = 0 THEN 1 ELSE 0 END), 0) AS failure_count,
  COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
  COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
  COALESCE(SUM(cached_tokens), 0) AS cached_tokens,
  COALESCE(SUM(total_tokens), 0) AS total_tokens
FROM request_events
WHERE ts >= ? AND auth_index != ''
GROUP BY auth_index
`
	rows, err := d.QueryContext(ctx, query, sinceTS)
	if err != nil {
		return nil, fmt.Errorf("select upstream event agg since failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []UpstreamEventAggRow
	for rows.Next() {
		var r UpstreamEventAggRow
		if err := rows.Scan(
			&r.AuthIndex, &r.Requests, &r.SuccessCount, &r.FailureCount,
			&r.PromptTokens, &r.CompletionTokens, &r.CachedTokens, &r.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("scan upstream event agg since failed: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectLatestModelByAuth returns the most recent model requested by each auth_index.
func (d *DB) SelectLatestModelByAuth(ctx context.Context) (map[string]string, error) {
	query := `
SELECT auth_index, model
FROM request_events
WHERE id IN (
  SELECT MAX(id) FROM request_events WHERE auth_index != '' GROUP BY auth_index
)
`
	rows, err := d.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select latest model by auth failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	res := make(map[string]string)
	for rows.Next() {
		var authIndex, model string
		if err := rows.Scan(&authIndex, &model); err == nil && authIndex != "" {
			res[authIndex] = model
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan latest model by auth failed: %w", err)
	}
	return res, nil
}

// SelectBaselinesAll retrieves all baseline rows.
func (d *DB) SelectBaselinesAll(ctx context.Context) ([]UpstreamAuthBaselineRow, error) {
	query := `SELECT auth_index, name, provider, base_requests, base_success_count, base_failure_count, base_prompt_tokens, base_completion_tokens, base_cached_tokens, base_total_tokens, updated_at FROM upstream_auth_baselines`
	rows, err := d.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select baselines failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []UpstreamAuthBaselineRow
	for rows.Next() {
		var r UpstreamAuthBaselineRow
		var name, provider sql.NullString
		var updatedAt sql.NullString
		if err := rows.Scan(
			&r.AuthIndex, &name, &provider,
			&r.BaseRequests, &r.BaseSuccessCount, &r.BaseFailureCount,
			&r.BasePromptTokens, &r.BaseCompletionTokens, &r.BaseCachedTokens, &r.BaseTotalTokens,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan baseline failed: %w", err)
		}
		if name.Valid {
			v := name.String
			r.Name = &v
		}
		if provider.Valid {
			v := provider.String
			r.Provider = &v
		}
		if updatedAt.Valid {
			if t, err := time.Parse("2006-01-02 15:04:05", updatedAt.String); err == nil {
				r.UpdatedAt = t
			}
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// UpsertBaseline inserts or updates an account baseline row in upstream_auth_baselines.
func (d *DB) UpsertBaseline(ctx context.Context, r *UpstreamAuthBaselineRow) error {
	query := `
INSERT INTO upstream_auth_baselines (auth_index, name, provider, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(auth_index) DO UPDATE SET
  name = CASE WHEN excluded.name IS NOT NULL AND excluded.name != '' THEN excluded.name ELSE upstream_auth_baselines.name END,
  provider = CASE WHEN excluded.provider IS NOT NULL AND excluded.provider != '' THEN excluded.provider ELSE upstream_auth_baselines.provider END,
  updated_at = CURRENT_TIMESTAMP
`
	_, err := d.ExecContext(ctx, query, r.AuthIndex, r.Name, r.Provider)
	if err != nil {
		return fmt.Errorf("upsert baseline failed: %w", err)
	}
	return nil
}

// UpsertBaselines inserts or updates multiple account baseline rows atomically.
func (d *DB) UpsertBaselines(ctx context.Context, baselines []UpstreamAuthBaselineRow) error {
	if len(baselines) == 0 {
		return nil
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for upsert baselines failed: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO upstream_auth_baselines (auth_index, name, provider, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(auth_index) DO UPDATE SET
  name = CASE WHEN excluded.name IS NOT NULL AND excluded.name != '' THEN excluded.name ELSE upstream_auth_baselines.name END,
  provider = CASE WHEN excluded.provider IS NOT NULL AND excluded.provider != '' THEN excluded.provider ELSE upstream_auth_baselines.provider END,
  updated_at = CURRENT_TIMESTAMP
`)
	if err != nil {
		return fmt.Errorf("prepare upsert baselines stmt failed: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, b := range baselines {
		if b.AuthIndex == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, b.AuthIndex, b.Name, b.Provider); err != nil {
			return fmt.Errorf("exec upsert baseline for %s failed: %w", b.AuthIndex, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert baselines failed: %w", err)
	}
	return nil
}

// BackfillUpstreamDailyStatsProviders synchronizes known providers from upstream_auth_baselines into upstream_daily_stats.
func (d *DB) BackfillUpstreamDailyStatsProviders(ctx context.Context) error {
	query := `
UPDATE upstream_daily_stats
SET provider = (
  SELECT provider FROM upstream_auth_baselines
  WHERE upstream_auth_baselines.auth_index = upstream_daily_stats.auth_index
    AND upstream_auth_baselines.provider IS NOT NULL
    AND upstream_auth_baselines.provider != ''
)
WHERE (provider = '' OR provider IS NULL)
  AND EXISTS (
    SELECT 1 FROM upstream_auth_baselines
    WHERE upstream_auth_baselines.auth_index = upstream_daily_stats.auth_index
      AND upstream_auth_baselines.provider IS NOT NULL
      AND upstream_auth_baselines.provider != ''
  )
`
	_, err := d.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("backfill upstream daily stats providers failed: %w", err)
	}
	return nil
}

// SelectAllModelPrices returns all model pricing rules sorted by pattern length descending.
func (d *DB) SelectAllModelPrices(ctx context.Context) ([]ModelPriceRow, error) {
	query := `
SELECT id, pattern, prompt_price, completion_price, cache_read_price, multiplier, created_at, updated_at
FROM model_prices ORDER BY LENGTH(pattern) DESC, id DESC
`
	rows, err := d.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select all model prices failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []ModelPriceRow
	for rows.Next() {
		var r ModelPriceRow
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.Pattern, &r.PromptPrice, &r.CompletionPrice, &r.CacheReadPrice, &r.Multiplier, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan model price failed: %w", err)
		}
		if createdAt.Valid {
			if t, err := time.Parse("2006-01-02 15:04:05", createdAt.String); err == nil {
				r.CreatedAt = t
			}
		}
		if updatedAt.Valid {
			if t, err := time.Parse("2006-01-02 15:04:05", updatedAt.String); err == nil {
				r.UpdatedAt = t
			}
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// UpsertModelPrice inserts or updates a model pricing rule.
func (d *DB) UpsertModelPrice(ctx context.Context, pattern string, promptPrice, completionPrice, cacheReadPrice, multiplier float64) error {
	query := `
INSERT INTO model_prices (pattern, prompt_price, completion_price, cache_read_price, multiplier, updated_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(pattern) DO UPDATE SET
  prompt_price = excluded.prompt_price,
  completion_price = excluded.completion_price,
  cache_read_price = excluded.cache_read_price,
  multiplier = excluded.multiplier,
  updated_at = CURRENT_TIMESTAMP
`
	_, err := d.ExecContext(ctx, query, pattern, promptPrice, completionPrice, cacheReadPrice, multiplier)
	if err != nil {
		return fmt.Errorf("upsert model price failed: %w", err)
	}
	return nil
}

// DeleteModelPrice removes a model pricing rule by ID.
func (d *DB) DeleteModelPrice(ctx context.Context, id int64) error {
	query := `DELETE FROM model_prices WHERE id = ?`
	_, err := d.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete model price failed: %w", err)
	}
	return nil
}

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
	return group + "|" + canonicalQuotaWindow(window)
}

// canonicalQuotaWindow maps the window spellings both writers used onto one token.
func canonicalQuotaWindow(window string) string {
	w := strings.ToLower(strings.TrimSpace(window))
	switch {
	case w == "":
		return ""
	case w == "5h", strings.Contains(w, "five hour"), strings.Contains(w, "5 hour"), strings.Contains(w, "hour"):
		return "5h"
	case w == "weekly", strings.Contains(w, "week"):
		return "weekly"
	}
	return w
}

// SelectValidations returns upstream validation challenges by status (e.g. "pending").
func (d *DB) SelectValidations(ctx context.Context, status string) ([]UpstreamValidationRow, error) {
	var query string
	var rows *sql.Rows
	var err error

	if status != "" {
		query = `SELECT id, ts, auth_index, provider, account_name, validation_url, error_message, raw_error, status, updated_at FROM upstream_validations WHERE status = ? ORDER BY id DESC`
		rows, err = d.QueryContext(ctx, query, status)
	} else {
		query = `SELECT id, ts, auth_index, provider, account_name, validation_url, error_message, raw_error, status, updated_at FROM upstream_validations ORDER BY id DESC LIMIT 100`
		rows, err = d.QueryContext(ctx, query)
	}
	if err != nil {
		return nil, fmt.Errorf("select validations failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []UpstreamValidationRow
	for rows.Next() {
		var r UpstreamValidationRow
		var acct, errMsg, rawErr sql.NullString
		if err := rows.Scan(&r.ID, &r.TS, &r.AuthIndex, &r.Provider, &acct, &r.ValidationURL, &errMsg, &rawErr, &r.Status, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan validation failed: %w", err)
		}
		if acct.Valid {
			v := acct.String
			r.AccountName = &v
		}
		if errMsg.Valid {
			v := errMsg.String
			r.ErrorMessage = &v
		}
		if rawErr.Valid {
			v := rawErr.String
			r.RawError = &v
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// InsertValidation records a new 403 challenge URL validation request.
func (d *DB) InsertValidation(ctx context.Context, r *UpstreamValidationRow) (int64, error) {
	query := `
INSERT INTO upstream_validations (ts, auth_index, provider, account_name, validation_url, error_message, raw_error, status, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?)
`
	res, err := d.ExecContext(ctx, query, r.TS, r.AuthIndex, r.Provider, r.AccountName, r.ValidationURL, r.ErrorMessage, r.RawError, r.UpdatedAt)
	if err != nil {
		return 0, fmt.Errorf("insert validation failed: %w", err)
	}
	return res.LastInsertId()
}

// ResolveValidation marks a validation item as resolved.
func (d *DB) ResolveValidation(ctx context.Context, id int64) error {
	query := `UPDATE upstream_validations SET status = 'resolved', updated_at = ? WHERE id = ?`
	now := time.Now().Unix()
	_, err := d.ExecContext(ctx, query, now, id)
	if err != nil {
		return fmt.Errorf("resolve validation failed: %w", err)
	}
	return nil
}

// ResolveValidationByAuth marks pending validations for an auth_index as resolved.
func (d *DB) ResolveValidationByAuth(ctx context.Context, authIndex string) error {
	query := `UPDATE upstream_validations SET status = 'resolved', updated_at = ? WHERE auth_index = ? AND status = 'pending'`
	now := time.Now().Unix()
	_, err := d.ExecContext(ctx, query, now, authIndex)
	if err != nil {
		return fmt.Errorf("resolve validation by auth failed: %w", err)
	}
	return nil
}

// DismissValidation marks a validation item as dismissed.
func (d *DB) DismissValidation(ctx context.Context, id int64) error {
	query := `UPDATE upstream_validations SET status = 'dismissed', updated_at = ? WHERE id = ?`
	now := time.Now().Unix()
	_, err := d.ExecContext(ctx, query, now, id)
	if err != nil {
		return fmt.Errorf("dismiss validation failed: %w", err)
	}
	return nil
}

// DeleteValidation permanently removes a validation entry.
func (d *DB) DeleteValidation(ctx context.Context, id int64) error {
	query := `DELETE FROM upstream_validations WHERE id = ?`
	_, err := d.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete validation failed: %w", err)
	}
	return nil
}

// SelectProviderRegistry returns all AI provider registry entries.
func (d *DB) SelectProviderRegistry(ctx context.Context) ([]ProviderRegistryRow, error) {
	query := `
SELECT provider_key, label, model_patterns, auth_patterns, source, enabled, sort_order, created_at, updated_at
FROM provider_registry ORDER BY sort_order ASC, provider_key ASC
`
	rows, err := d.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select provider registry failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []ProviderRegistryRow
	for rows.Next() {
		var r ProviderRegistryRow
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&r.ProviderKey, &r.Label, &r.ModelPatterns, &r.AuthPatterns, &r.Source, &r.Enabled, &r.SortOrder, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan provider registry failed: %w", err)
		}
		if createdAt.Valid {
			if t, err := time.Parse("2006-01-02 15:04:05", createdAt.String); err == nil {
				r.CreatedAt = t
			}
		}
		if updatedAt.Valid {
			if t, err := time.Parse("2006-01-02 15:04:05", updatedAt.String); err == nil {
				r.UpdatedAt = t
			}
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// UpsertProviderRegistry creates or updates an AI provider registry entry.
func (d *DB) UpsertProviderRegistry(ctx context.Context, r *ProviderRegistryRow) error {
	query := `
INSERT INTO provider_registry (provider_key, label, model_patterns, auth_patterns, source, enabled, sort_order, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT(provider_key) DO UPDATE SET
  label = excluded.label,
  model_patterns = excluded.model_patterns,
  auth_patterns = excluded.auth_patterns,
  source = excluded.source,
  enabled = excluded.enabled,
  sort_order = excluded.sort_order,
  updated_at = datetime('now')
`
	_, err := d.ExecContext(ctx, query, r.ProviderKey, r.Label, r.ModelPatterns, r.AuthPatterns, r.Source, r.Enabled, r.SortOrder)
	if err != nil {
		return fmt.Errorf("upsert provider registry failed: %w", err)
	}
	return nil
}

// DeleteProviderRegistry removes an AI provider registry entry by key.
func (d *DB) DeleteProviderRegistry(ctx context.Context, providerKey string) error {
	query := `DELETE FROM provider_registry WHERE provider_key = ?`
	_, err := d.ExecContext(ctx, query, providerKey)
	if err != nil {
		return fmt.Errorf("delete provider registry failed: %w", err)
	}
	return nil
}

// AuthBucketRow captures aggregated throughput for an auth_index in a 10-minute bucket.
type AuthBucketRow struct {
	AuthIndex string
	BucketID  int64
	Success   int64
	Failed    int64
}

// SelectRecentAuthBuckets returns 10-minute bucket aggregates from request_events for ts >= sinceTS.
func (d *DB) SelectRecentAuthBuckets(ctx context.Context, sinceTS int64) ([]AuthBucketRow, error) {
	query := `
SELECT 
  auth_index,
  (ts / 600) AS bucket_id,
  COALESCE(SUM(CASE WHEN status >= 200 AND status < 400 THEN 1 ELSE 0 END), 0) AS success,
  COALESCE(SUM(CASE WHEN status >= 400 OR status = 0 THEN 1 ELSE 0 END), 0) AS failed
FROM request_events
WHERE ts >= ?
GROUP BY auth_index, bucket_id
HAVING auth_index != ''
`
	rows, err := d.QueryContext(ctx, query, sinceTS)
	if err != nil {
		return nil, fmt.Errorf("select recent auth buckets failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []AuthBucketRow
	for rows.Next() {
		var r AuthBucketRow
		if err := rows.Scan(&r.AuthIndex, &r.BucketID, &r.Success, &r.Failed); err != nil {
			return nil, fmt.Errorf("scan recent auth buckets failed: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}
