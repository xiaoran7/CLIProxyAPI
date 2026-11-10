// 请求明细事件写入与查询 —— 拆自 queries_events.go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// InsertRequestEventTx inserts a permanent record into request_events inside a transaction.
func (d *DB) InsertRequestEventTx(ctx context.Context, tx *sql.Tx, r *RequestEventRow) (int64, error) {
	query := `
INSERT INTO request_events (ts, key_id, model, status, prompt_tokens, completion_tokens, cached_tokens, total_tokens, latency_ms, ttft_ms, stream, estimated, error_type, auth_index, reasoning_effort, tokens_per_sec, client_ip)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	res, err := tx.ExecContext(ctx, query,
		r.TS, r.KeyID, r.Model, r.Status, r.PromptTokens, r.CompletionTokens,
		r.CachedTokens, r.TotalTokens, r.LatencyMs, r.TtftMs, r.Stream, r.Estimated,
		r.ErrorType, r.AuthIndex, r.ReasoningEffort, r.TokensPerSec, r.ClientIP,
	)
	if err != nil {
		return 0, fmt.Errorf("insert request event tx failed: %w", err)
	}
	return res.LastInsertId()
}

// InsertRequestEvent inserts a record into request_events directly.
func (d *DB) InsertRequestEvent(ctx context.Context, r *RequestEventRow) (int64, error) {
	query := `
INSERT INTO request_events (ts, key_id, model, status, prompt_tokens, completion_tokens, cached_tokens, total_tokens, latency_ms, ttft_ms, stream, estimated, error_type, auth_index, reasoning_effort, tokens_per_sec, client_ip)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	res, err := d.ExecContext(ctx, query,
		r.TS, r.KeyID, r.Model, r.Status, r.PromptTokens, r.CompletionTokens,
		r.CachedTokens, r.TotalTokens, r.LatencyMs, r.TtftMs, r.Stream, r.Estimated,
		r.ErrorType, r.AuthIndex, r.ReasoningEffort, r.TokensPerSec, r.ClientIP,
	)
	if err != nil {
		return 0, fmt.Errorf("insert request event failed: %w", err)
	}
	return res.LastInsertId()
}

// SelectRecentEvents retrieves recent request_events ordered by id desc.
func (d *DB) SelectRecentEvents(ctx context.Context, limit int) ([]RequestEventRow, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
SELECT id, ts, key_id, model, status, prompt_tokens, completion_tokens, cached_tokens, total_tokens, latency_ms, ttft_ms, stream, estimated, error_type, auth_index, reasoning_effort, tokens_per_sec, client_ip
FROM request_events ORDER BY id DESC LIMIT ?
`
	rows, err := d.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("select recent events failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []RequestEventRow
	for rows.Next() {
		var r RequestEventRow
		var keyID sql.NullInt64
		var ttft sql.NullInt64
		var errType sql.NullString
		var clientIP sql.NullString
		if err := rows.Scan(
			&r.ID, &r.TS, &keyID, &r.Model, &r.Status, &r.PromptTokens, &r.CompletionTokens,
			&r.CachedTokens, &r.TotalTokens, &r.LatencyMs, &ttft, &r.Stream, &r.Estimated,
			&errType, &r.AuthIndex, &r.ReasoningEffort, &r.TokensPerSec, &clientIP,
		); err != nil {
			return nil, fmt.Errorf("scan request event failed: %w", err)
		}
		if keyID.Valid {
			v := keyID.Int64
			r.KeyID = &v
		}
		if ttft.Valid {
			v := ttft.Int64
			r.TtftMs = &v
		}
		if errType.Valid {
			v := errType.String
			r.ErrorType = &v
		}
		if clientIP.Valid {
			r.ClientIP = clientIP.String
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectRecentEventsByKey retrieves recent request_events for a specific key.
func (d *DB) SelectRecentEventsByKey(ctx context.Context, keyID int64, limit int) ([]RequestEventRow, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
SELECT id, ts, key_id, model, status, prompt_tokens, completion_tokens, cached_tokens, total_tokens, latency_ms, ttft_ms, stream, estimated, error_type, auth_index, reasoning_effort, tokens_per_sec, client_ip
FROM request_events WHERE key_id = ? ORDER BY id DESC LIMIT ?
`
	rows, err := d.QueryContext(ctx, query, keyID, limit)
	if err != nil {
		return nil, fmt.Errorf("select recent events by key failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []RequestEventRow
	for rows.Next() {
		var r RequestEventRow
		var kid sql.NullInt64
		var ttft sql.NullInt64
		var errType sql.NullString
		var clientIP sql.NullString
		if err := rows.Scan(
			&r.ID, &r.TS, &kid, &r.Model, &r.Status, &r.PromptTokens, &r.CompletionTokens,
			&r.CachedTokens, &r.TotalTokens, &r.LatencyMs, &ttft, &r.Stream, &r.Estimated,
			&errType, &r.AuthIndex, &r.ReasoningEffort, &r.TokensPerSec, &clientIP,
		); err != nil {
			return nil, fmt.Errorf("scan request event failed: %w", err)
		}
		if kid.Valid {
			v := kid.Int64
			r.KeyID = &v
		}
		if ttft.Valid {
			v := ttft.Int64
			r.TtftMs = &v
		}
		if errType.Valid {
			v := errType.String
			r.ErrorType = &v
		}
		if clientIP.Valid {
			r.ClientIP = clientIP.String
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectEventsPaged performs filtered, cursor-based pagination on request_events.
func (d *DB) SelectEventsPaged(ctx context.Context, beforeID int64, limit int, keyID *int64, model string, status *int, authIndex string) ([]RequestEventRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	var where []string
	var args []interface{}

	if beforeID > 0 {
		where = append(where, "id < ?")
		args = append(args, beforeID)
	}
	if keyID != nil {
		where = append(where, "key_id = ?")
		args = append(args, *keyID)
	}
	if model != "" {
		where = append(where, "model LIKE ?")
		args = append(args, "%"+model+"%")
	}
	if status != nil {
		where = append(where, "status = ?")
		args = append(args, *status)
	}
	if authIndex != "" {
		where = append(where, "auth_index = ?")
		args = append(args, authIndex)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	query := fmt.Sprintf(`
SELECT id, ts, key_id, model, status, prompt_tokens, completion_tokens, cached_tokens, total_tokens, latency_ms, ttft_ms, stream, estimated, error_type, auth_index, reasoning_effort, tokens_per_sec
FROM request_events %s ORDER BY id DESC LIMIT ?
`, whereClause)
	args = append(args, limit)

	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select events paged failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []RequestEventRow
	for rows.Next() {
		var r RequestEventRow
		var kid sql.NullInt64
		var ttft sql.NullInt64
		var errType sql.NullString
		if err := rows.Scan(
			&r.ID, &r.TS, &kid, &r.Model, &r.Status, &r.PromptTokens, &r.CompletionTokens,
			&r.CachedTokens, &r.TotalTokens, &r.LatencyMs, &ttft, &r.Stream, &r.Estimated,
			&errType, &r.AuthIndex, &r.ReasoningEffort, &r.TokensPerSec,
		); err != nil {
			return nil, fmt.Errorf("scan request event failed: %w", err)
		}
		if kid.Valid {
			v := kid.Int64
			r.KeyID = &v
		}
		if ttft.Valid {
			v := ttft.Int64
			r.TtftMs = &v
		}
		if errType.Valid {
			v := errType.String
			r.ErrorType = &v
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectEventsForStatus returns events since sinceTS for status analysis.
func (d *DB) SelectEventsForStatus(ctx context.Context, sinceTS int64, limit int) ([]RequestEventRow, error) {
	if limit <= 0 {
		limit = 3000
	}
	query := `
SELECT ts, model, status, latency_ms, ttft_ms, stream, auth_index
FROM request_events WHERE ts >= ? ORDER BY id DESC LIMIT ?
`
	rows, err := d.QueryContext(ctx, query, sinceTS, limit)
	if err != nil {
		return nil, fmt.Errorf("select events for status failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []RequestEventRow
	for rows.Next() {
		var r RequestEventRow
		var ttft sql.NullInt64
		if err := rows.Scan(&r.TS, &r.Model, &r.Status, &r.LatencyMs, &ttft, &r.Stream, &r.AuthIndex); err != nil {
			return nil, fmt.Errorf("scan event for status failed: %w", err)
		}
		if ttft.Valid {
			v := ttft.Int64
			r.TtftMs = &v
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// PruneEvents deletes old request_events beyond retainCount or older than olderThanTS.
func (d *DB) PruneEvents(ctx context.Context, retainCount int64, olderThanTS int64) (int64, error) {
	query := `
DELETE FROM request_events
WHERE id <= (SELECT MAX(id) FROM request_events) - ? OR ts < ?
`
	res, err := d.ExecContext(ctx, query, retainCount, olderThanTS)
	if err != nil {
		return 0, fmt.Errorf("prune events failed: %w", err)
	}
	return res.RowsAffected()
}

// EventWindowAggregateRow holds grouped request count for a window time slot.
type EventWindowAggregateRow struct {
	SlotStart int64 `json:"slot_start"`
	Success   int64 `json:"success"`
	Failed    int64 `json:"failed"`
}

// SelectEventsWindowAggregate groups request_events into time slots aligned to timezone offset.
func (d *DB) SelectEventsWindowAggregate(ctx context.Context, tzOffset int64, bucketSeconds int64, minTs int64, maxTs int64) ([]EventWindowAggregateRow, error) {
	query := `
SELECT
  ((ts + ?) - ((ts + ?) % ?)) - ? AS slot_start,
  COALESCE(SUM(CASE WHEN status >= 200 AND status < 400 THEN 1 ELSE 0 END), 0) AS success,
  COALESCE(SUM(CASE WHEN status >= 400 OR status = 0 THEN 1 ELSE 0 END), 0) AS failed
FROM request_events
WHERE ts >= ? AND ts < ?
GROUP BY slot_start
`
	rows, err := d.QueryContext(ctx, query, tzOffset, tzOffset, bucketSeconds, tzOffset, minTs, maxTs)
	if err != nil {
		return nil, fmt.Errorf("select events window aggregate failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []EventWindowAggregateRow
	for rows.Next() {
		var r EventWindowAggregateRow
		if err := rows.Scan(&r.SlotStart, &r.Success, &r.Failed); err != nil {
			return nil, fmt.Errorf("scan window aggregate row failed: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}
