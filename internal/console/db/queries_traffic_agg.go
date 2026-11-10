// 流量大盘与时序聚合分析查询 —— 拆自 queries_events.go
package db

import (
	"context"
	"fmt"
	"strings"
)

// SelectHourlyRange returns 24h event stats grouped by hour and model.
func (d *DB) SelectHourlyRange(ctx context.Context, startTS, endTS int64, keyID *int64) ([]HourlyEventAggRow, error) {
	var where []string
	var args []interface{}

	where = append(where, "ts >= ? AND ts < ?")
	args = append(args, startTS, endTS)

	if keyID != nil {
		where = append(where, "key_id = ?")
		args = append(args, *keyID)
	}

	whereClause := "WHERE " + strings.Join(where, " AND ")

	query := fmt.Sprintf(`
SELECT strftime('%%H', datetime(ts, 'unixepoch', '+8 hours')) AS hr,
       model,
       COUNT(*) AS requests,
       SUM(prompt_tokens) AS prompt_tokens,
       SUM(completion_tokens) AS completion_tokens,
       SUM(cached_tokens) AS cached_tokens,
       SUM(total_tokens) AS total_tokens,
       SUM(CASE WHEN status >= 200 AND status < 400 THEN 1 ELSE 0 END) AS success_count,
       SUM(CASE WHEN status >= 400 OR status = 0 THEN 1 ELSE 0 END) AS failure_count
FROM request_events %s
GROUP BY hr, model
ORDER BY hr ASC
`, whereClause)

	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select hourly range failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []HourlyEventAggRow
	for rows.Next() {
		var r HourlyEventAggRow
		if err := rows.Scan(
			&r.Hour, &r.Model, &r.Requests, &r.PromptTokens, &r.CompletionTokens,
			&r.CachedTokens, &r.TotalTokens, &r.SuccessCount, &r.FailureCount,
		); err != nil {
			return nil, fmt.Errorf("scan hourly event row failed: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectAuthModelAggRange aggregates request_events by auth_index and model.
func (d *DB) SelectAuthModelAggRange(ctx context.Context, sinceTS int64, keyID *int64) ([]AuthModelAggRow, error) {
	var where []string
	var args []interface{}

	if sinceTS > 0 {
		where = append(where, "ts >= ?")
		args = append(args, sinceTS)
	}
	if keyID != nil {
		where = append(where, "key_id = ?")
		args = append(args, *keyID)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	query := fmt.Sprintf(`
SELECT auth_index,
       model,
       COUNT(*) AS requests,
       SUM(prompt_tokens) AS prompt_tokens,
       SUM(completion_tokens) AS completion_tokens,
       SUM(cached_tokens) AS cached_tokens,
       SUM(total_tokens) AS total_tokens
FROM request_events %s
GROUP BY auth_index, model
`, whereClause)

	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select auth model agg range failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []AuthModelAggRow
	for rows.Next() {
		var r AuthModelAggRow
		if err := rows.Scan(
			&r.AuthIndex, &r.Model, &r.Requests, &r.PromptTokens, &r.CompletionTokens,
			&r.CachedTokens, &r.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("scan auth model agg row failed: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// CleanPhantomEvents removes ghost request events mistakenly recorded for non-inference requests
// (model = 'unknown' or empty, status 200..299, 0 tokens, and empty upstream auth).
func (d *DB) CleanPhantomEvents(ctx context.Context) (int64, error) {
	query := `
DELETE FROM request_events
WHERE (model IS NULL OR model = '' OR model = 'unknown')
  AND status >= 200 AND status < 300
  AND prompt_tokens = 0 AND completion_tokens = 0 AND total_tokens = 0
  AND (auth_index IS NULL OR auth_index = '')
`
	res, err := d.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("clean phantom events failed: %w", err)
	}

	// Also clean orphaned daily usage records with model = 'unknown' and 0 tokens
	queryDaily := `
DELETE FROM usage_daily
WHERE (model IS NULL OR model = '' OR model = 'unknown')
  AND prompt_tokens = 0 AND completion_tokens = 0 AND total_tokens = 0
`
	_, _ = d.ExecContext(ctx, queryDaily)

	// Clean phantom error events from unauthenticated external network scans
	queryErrors := `
DELETE FROM error_events
WHERE (key_id IS NULL OR key_id <= 0)
  AND (auth_index IS NULL OR auth_index = '')
  AND (model IS NULL OR model = '' OR model = 'unknown')
`
	_, _ = d.ExecContext(ctx, queryErrors)

	return res.RowsAffected()
}

// SelectTrafficAggregates returns grouped request metrics across client IPs, models, channels, and statuses.
// It queries both request_events and usage_outbox via UNION ALL to avoid flush lock contention while reading in-flight events.
func (d *DB) SelectTrafficAggregates(ctx context.Context, sinceTS int64) ([]TrafficAggregateRow, error) {
	whereEvents := ""
	whereOutbox := ""
	var args []interface{}
	if sinceTS > 0 {
		whereEvents = "WHERE ts >= ?"
		whereOutbox = "WHERE ts >= ?"
		args = append(args, sinceTS, sinceTS)
	}

	query := fmt.Sprintf(`
SELECT
    COALESCE(NULLIF(r.client_ip, ''), '127.0.0.1') AS client_ip,
    COALESCE(NULLIF(r.model, ''), 'unknown') AS model,
    COALESCE(r.auth_index, '') AS auth_index,
    COALESCE(NULLIF(b.provider, ''), '') AS channel,
    r.status,
    COALESCE(NULLIF(k.alias, ''), NULLIF(k.api_key, ''), '-') AS key_label,
    COUNT(*) AS req_count,
    SUM(r.latency_ms) AS total_latency,
    SUM(r.total_tokens) AS total_tokens,
    MAX(r.ts) AS last_seen
FROM (
    SELECT ts, client_ip, model, auth_index, status, key_id, latency_ms, total_tokens
    FROM request_events
    %s
    UNION ALL
    SELECT ts, client_ip, model, auth_index, status, key_id, latency_ms, total_tokens
    FROM usage_outbox
    %s
) r
LEFT JOIN keys k ON r.key_id = k.id
LEFT JOIN upstream_auth_baselines b ON r.auth_index = b.auth_index
GROUP BY client_ip, model, r.auth_index, channel, r.status, key_label
`, whereEvents, whereOutbox)

	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select traffic aggregates failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []TrafficAggregateRow
	for rows.Next() {
		var r TrafficAggregateRow
		if err := rows.Scan(
			&r.ClientIP, &r.Model, &r.AuthIndex, &r.Channel, &r.Status, &r.KeyLabel,
			&r.ReqCount, &r.TotalLatency, &r.TotalTokens, &r.LastSeen,
		); err != nil {
			return nil, fmt.Errorf("scan traffic aggregate row failed: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectTrafficRecent returns the most recent requests up to limit within the specified time cutoff.
func (d *DB) SelectTrafficRecent(ctx context.Context, sinceTS int64, limit int) ([]TrafficRecentRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	whereEvents := ""
	whereOutbox := ""
	var args []interface{}
	if sinceTS > 0 {
		whereEvents = "WHERE ts >= ?"
		whereOutbox = "WHERE ts >= ?"
		args = append(args, sinceTS, sinceTS)
	}

	query := fmt.Sprintf(`
SELECT
    r.id,
    r.ts,
    COALESCE(NULLIF(r.client_ip, ''), '127.0.0.1') AS client_ip,
    COALESCE(NULLIF(k.alias, ''), NULLIF(k.api_key, ''), '-') AS key_label,
    COALESCE(NULLIF(r.model, ''), 'unknown') AS model,
    COALESCE(NULLIF(b.provider, ''), '') AS channel,
    r.status,
    r.latency_ms,
    r.total_tokens,
    COALESCE(r.auth_index, '') AS auth_index
FROM (
    SELECT id, ts, client_ip, model, auth_index, status, key_id, latency_ms, total_tokens
    FROM request_events
    %s
    UNION ALL
    SELECT id, ts, client_ip, model, auth_index, status, key_id, latency_ms, total_tokens
    FROM usage_outbox
    %s
) r
LEFT JOIN keys k ON r.key_id = k.id
LEFT JOIN upstream_auth_baselines b ON r.auth_index = b.auth_index
ORDER BY r.ts DESC, r.id DESC
LIMIT ?
`, whereEvents, whereOutbox)
	args = append(args, limit)

	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select traffic recent failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []TrafficRecentRow
	for rows.Next() {
		var r TrafficRecentRow
		if err := rows.Scan(
			&r.ID, &r.TS, &r.ClientIP, &r.KeyLabel, &r.Model, &r.Channel,
			&r.Status, &r.LatencyMs, &r.TotalTokens, &r.AuthIndex,
		); err != nil {
			return nil, fmt.Errorf("scan traffic recent row failed: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}
