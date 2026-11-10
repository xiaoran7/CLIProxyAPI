// 上游窗口统计与聚合查询 —— 拆自 queries_upstream.go
package db

import (
	"context"
	"fmt"
)

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
