// 上游每日统计查询 —— 拆自 queries_upstream.go
package db

import (
	"context"
	"database/sql"
	"fmt"
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
