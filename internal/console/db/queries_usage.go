package db

import (
	"context"
	"database/sql"
	"fmt"
)

// UpsertUsageDailyTx increments usage counters in usage_daily inside an existing transaction.
func (d *DB) UpsertUsageDailyTx(ctx context.Context, tx *sql.Tx, r *UsageDailyRow) error {
	query := `
INSERT INTO usage_daily (date, key_id, model, requests, prompt_tokens, completion_tokens, cached_tokens, total_tokens, estimated_tokens, success_count, failure_count)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(date, key_id, model) DO UPDATE SET
  requests = requests + excluded.requests,
  prompt_tokens = prompt_tokens + excluded.prompt_tokens,
  completion_tokens = completion_tokens + excluded.completion_tokens,
  cached_tokens = cached_tokens + excluded.cached_tokens,
  total_tokens = total_tokens + excluded.total_tokens,
  estimated_tokens = estimated_tokens + excluded.estimated_tokens,
  success_count = success_count + excluded.success_count,
  failure_count = failure_count + excluded.failure_count
`
	_, err := tx.ExecContext(ctx, query,
		r.Date, r.KeyID, r.Model, r.Requests, r.PromptTokens, r.CompletionTokens,
		r.CachedTokens, r.TotalTokens, r.EstimatedTokens, r.SuccessCount, r.FailureCount,
	)
	if err != nil {
		return fmt.Errorf("upsert usage daily tx failed: %w", err)
	}
	return nil
}

// SelectDailyForDate returns all usage rows for a given date.
func (d *DB) SelectDailyForDate(ctx context.Context, date string) ([]UsageDailyRow, error) {
	query := `
SELECT key_id, model, requests, prompt_tokens, completion_tokens, cached_tokens, total_tokens, estimated_tokens, success_count, failure_count
FROM usage_daily WHERE date = ?
`
	rows, err := d.QueryContext(ctx, query, date)
	if err != nil {
		return nil, fmt.Errorf("select daily for date failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []UsageDailyRow
	for rows.Next() {
		var r UsageDailyRow
		r.Date = date
		if err := rows.Scan(&r.KeyID, &r.Model, &r.Requests, &r.PromptTokens, &r.CompletionTokens, &r.CachedTokens, &r.TotalTokens, &r.EstimatedTokens, &r.SuccessCount, &r.FailureCount); err != nil {
			return nil, fmt.Errorf("scan daily row failed: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectDailyForDateKey sums tokens and requests for a specific key on a specific date.
func (d *DB) SelectDailyForDateKey(ctx context.Context, date string, keyID int64) (*DailyStatsSummary, error) {
	query := `
SELECT
  COALESCE(SUM(requests), 0) AS requests,
  COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
  COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
  COALESCE(SUM(cached_tokens), 0) AS cached_tokens,
  COALESCE(SUM(total_tokens), 0) AS total_tokens,
  COALESCE(SUM(success_count), 0) AS success_count,
  COALESCE(SUM(failure_count), 0) AS failure_count
FROM usage_daily
WHERE date = ? AND key_id = ?
`
	row := d.QueryRowContext(ctx, query, date, keyID)
	var s DailyStatsSummary
	s.Date = date
	if err := row.Scan(&s.Requests, &s.PromptTokens, &s.CompletionTokens, &s.CachedTokens, &s.TotalTokens, &s.SuccessCount, &s.FailureCount); err != nil {
		return nil, fmt.Errorf("select daily for date key failed: %w", err)
	}
	return &s, nil
}

// SelectDailyRange returns daily aggregates on or after sinceDate.
func (d *DB) SelectDailyRange(ctx context.Context, sinceDate string) ([]DailyStatsSummary, error) {
	query := `
SELECT date,
       SUM(requests) AS requests,
       SUM(prompt_tokens) AS prompt_tokens,
       SUM(completion_tokens) AS completion_tokens,
       SUM(cached_tokens) AS cached_tokens,
       SUM(total_tokens) AS total_tokens,
       SUM(success_count) AS success_count,
       SUM(failure_count) AS failure_count
FROM usage_daily WHERE date >= ? GROUP BY date ORDER BY date ASC
`
	rows, err := d.QueryContext(ctx, query, sinceDate)
	if err != nil {
		return nil, fmt.Errorf("select daily range failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []DailyStatsSummary
	for rows.Next() {
		var s DailyStatsSummary
		if err := rows.Scan(&s.Date, &s.Requests, &s.PromptTokens, &s.CompletionTokens, &s.CachedTokens, &s.TotalTokens, &s.SuccessCount, &s.FailureCount); err != nil {
			return nil, fmt.Errorf("scan daily summary failed: %w", err)
		}
		list = append(list, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectDailyRangeByKey returns daily aggregates on or after sinceDate for a specific key.
func (d *DB) SelectDailyRangeByKey(ctx context.Context, sinceDate string, keyID int64) ([]DailyStatsSummary, error) {
	query := `
SELECT date,
       SUM(requests) AS requests,
       SUM(prompt_tokens) AS prompt_tokens,
       SUM(completion_tokens) AS completion_tokens,
       SUM(cached_tokens) AS cached_tokens,
       SUM(total_tokens) AS total_tokens,
       SUM(success_count) AS success_count,
       SUM(failure_count) AS failure_count
FROM usage_daily WHERE date >= ? AND key_id = ? GROUP BY date ORDER BY date ASC
`
	rows, err := d.QueryContext(ctx, query, sinceDate, keyID)
	if err != nil {
		return nil, fmt.Errorf("select daily range by key failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []DailyStatsSummary
	for rows.Next() {
		var s DailyStatsSummary
		if err := rows.Scan(&s.Date, &s.Requests, &s.PromptTokens, &s.CompletionTokens, &s.CachedTokens, &s.TotalTokens, &s.SuccessCount, &s.FailureCount); err != nil {
			return nil, fmt.Errorf("scan daily summary by key failed: %w", err)
		}
		list = append(list, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectModelRange returns aggregated model statistics on or after sinceDate.
func (d *DB) SelectModelRange(ctx context.Context, sinceDate string) ([]ModelStatsSummary, error) {
	query := `
SELECT model,
       SUM(requests) AS requests,
       SUM(prompt_tokens) AS prompt_tokens,
       SUM(completion_tokens) AS completion_tokens,
       SUM(cached_tokens) AS cached_tokens,
       SUM(total_tokens) AS total_tokens,
       SUM(success_count) AS success_count,
       SUM(failure_count) AS failure_count
FROM usage_daily WHERE date >= ? GROUP BY model ORDER BY total_tokens DESC
`
	rows, err := d.QueryContext(ctx, query, sinceDate)
	if err != nil {
		return nil, fmt.Errorf("select model range failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []ModelStatsSummary
	for rows.Next() {
		var s ModelStatsSummary
		if err := rows.Scan(&s.Model, &s.Requests, &s.PromptTokens, &s.CompletionTokens, &s.CachedTokens, &s.TotalTokens, &s.SuccessCount, &s.FailureCount); err != nil {
			return nil, fmt.Errorf("scan model summary failed: %w", err)
		}
		list = append(list, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectModelRangeByKey returns aggregated model statistics on or after sinceDate for a specific key.
func (d *DB) SelectModelRangeByKey(ctx context.Context, sinceDate string, keyID int64) ([]ModelStatsSummary, error) {
	query := `
SELECT model,
       SUM(requests) AS requests,
       SUM(prompt_tokens) AS prompt_tokens,
       SUM(completion_tokens) AS completion_tokens,
       SUM(cached_tokens) AS cached_tokens,
       SUM(total_tokens) AS total_tokens,
       SUM(success_count) AS success_count,
       SUM(failure_count) AS failure_count
FROM usage_daily WHERE date >= ? AND key_id = ? GROUP BY model ORDER BY total_tokens DESC
`
	rows, err := d.QueryContext(ctx, query, sinceDate, keyID)
	if err != nil {
		return nil, fmt.Errorf("select model range by key failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []ModelStatsSummary
	for rows.Next() {
		var s ModelStatsSummary
		if err := rows.Scan(&s.Model, &s.Requests, &s.PromptTokens, &s.CompletionTokens, &s.CachedTokens, &s.TotalTokens, &s.SuccessCount, &s.FailureCount); err != nil {
			return nil, fmt.Errorf("scan model summary by key failed: %w", err)
		}
		list = append(list, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectDailyAllTime returns the lifetime token and request sum across all usage.
func (d *DB) SelectDailyAllTime(ctx context.Context) (*DailyStatsSummary, error) {
	query := `
SELECT COALESCE(SUM(requests), 0) AS requests,
       COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
       COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
       COALESCE(SUM(cached_tokens), 0) AS cached_tokens,
       COALESCE(SUM(total_tokens), 0) AS total_tokens,
       COALESCE(SUM(success_count), 0) AS success_count,
       COALESCE(SUM(failure_count), 0) AS failure_count
FROM usage_daily
`
	row := d.QueryRowContext(ctx, query)
	var s DailyStatsSummary
	if err := row.Scan(&s.Requests, &s.PromptTokens, &s.CompletionTokens, &s.CachedTokens, &s.TotalTokens, &s.SuccessCount, &s.FailureCount); err != nil {
		return nil, fmt.Errorf("select daily all time failed: %w", err)
	}
	return &s, nil
}

// SelectDailyAllTimeByAllKeys returns lifetime totals indexed by key_id.
func (d *DB) SelectDailyAllTimeByAllKeys(ctx context.Context) (map[int64]*DailyStatsSummary, error) {
	query := `
SELECT key_id,
       COALESCE(SUM(requests), 0) AS requests,
       COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
       COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
       COALESCE(SUM(cached_tokens), 0) AS cached_tokens,
       COALESCE(SUM(total_tokens), 0) AS total_tokens,
       COALESCE(SUM(success_count), 0) AS success_count,
       COALESCE(SUM(failure_count), 0) AS failure_count
FROM usage_daily GROUP BY key_id
`
	rows, err := d.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select daily all time by all keys failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	res := make(map[int64]*DailyStatsSummary)
	for rows.Next() {
		var keyID int64
		var s DailyStatsSummary
		if err := rows.Scan(&keyID, &s.Requests, &s.PromptTokens, &s.CompletionTokens, &s.CachedTokens, &s.TotalTokens, &s.SuccessCount, &s.FailureCount); err != nil {
			return nil, fmt.Errorf("scan all keys daily failed: %w", err)
		}
		res[keyID] = &s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return res, nil
}

// PruneDaily removes usage_daily records older than beforeDate.
func (d *DB) PruneDaily(ctx context.Context, beforeDate string) (int64, error) {
	query := `DELETE FROM usage_daily WHERE date < ?`
	res, err := d.ExecContext(ctx, query, beforeDate)
	if err != nil {
		return 0, fmt.Errorf("prune daily failed: %w", err)
	}
	return res.RowsAffected()
}
