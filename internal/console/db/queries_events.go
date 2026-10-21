package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// InsertUsageOutbox inserts a raw request event synchronously into usage_outbox.
func (d *DB) InsertUsageOutbox(ctx context.Context, r *RequestEventRow) (int64, error) {
	query := `
INSERT INTO usage_outbox (ts, key_id, model, status, prompt_tokens, completion_tokens, cached_tokens, total_tokens, latency_ms, ttft_ms, stream, estimated, error_type, auth_index, reasoning_effort, tokens_per_sec)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	res, err := d.ExecContext(ctx, query,
		r.TS, r.KeyID, r.Model, r.Status, r.PromptTokens, r.CompletionTokens,
		r.CachedTokens, r.TotalTokens, r.LatencyMs, r.TtftMs, r.Stream, r.Estimated,
		r.ErrorType, r.AuthIndex, r.ReasoningEffort, r.TokensPerSec,
	)
	if err != nil {
		return 0, fmt.Errorf("insert usage outbox failed: %w", err)
	}
	return res.LastInsertId()
}

// SelectAllOutbox retrieves all un-flushed items from usage_outbox in order.
func (d *DB) SelectAllOutbox(ctx context.Context) ([]RequestEventRow, error) {
	query := `
SELECT id, ts, key_id, model, status, prompt_tokens, completion_tokens, cached_tokens, total_tokens, latency_ms, ttft_ms, stream, estimated, error_type, auth_index, reasoning_effort, tokens_per_sec
FROM usage_outbox ORDER BY id ASC
`
	rows, err := d.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select all outbox failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []RequestEventRow
	for rows.Next() {
		var r RequestEventRow
		var keyID sql.NullInt64
		var ttft sql.NullInt64
		var errType sql.NullString
		if err := rows.Scan(
			&r.ID, &r.TS, &keyID, &r.Model, &r.Status, &r.PromptTokens, &r.CompletionTokens,
			&r.CachedTokens, &r.TotalTokens, &r.LatencyMs, &ttft, &r.Stream, &r.Estimated,
			&errType, &r.AuthIndex, &r.ReasoningEffort, &r.TokensPerSec,
		); err != nil {
			return nil, fmt.Errorf("scan outbox row failed: %w", err)
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
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// DeleteOutboxItemTx removes a single processed item from usage_outbox inside a transaction.
func (d *DB) DeleteOutboxItemTx(ctx context.Context, tx *sql.Tx, id int64) error {
	query := `DELETE FROM usage_outbox WHERE id = ?`
	_, err := tx.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete outbox item tx failed: %w", err)
	}
	return nil
}

// DeleteOutboxItem removes an item from usage_outbox outside a transaction.
func (d *DB) DeleteOutboxItem(ctx context.Context, id int64) error {
	query := `DELETE FROM usage_outbox WHERE id = ?`
	_, err := d.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete outbox item failed: %w", err)
	}
	return nil
}

// CountOutbox returns the total number of un-flushed items in usage_outbox.
func (d *DB) CountOutbox(ctx context.Context) (int64, error) {
	query := `SELECT COUNT(*) FROM usage_outbox`
	var count int64
	err := d.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count outbox failed: %w", err)
	}
	return count, nil
}

// InsertRequestEventTx inserts a permanent record into request_events inside a transaction.
func (d *DB) InsertRequestEventTx(ctx context.Context, tx *sql.Tx, r *RequestEventRow) (int64, error) {
	query := `
INSERT INTO request_events (ts, key_id, model, status, prompt_tokens, completion_tokens, cached_tokens, total_tokens, latency_ms, ttft_ms, stream, estimated, error_type, auth_index, reasoning_effort, tokens_per_sec)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	res, err := tx.ExecContext(ctx, query,
		r.TS, r.KeyID, r.Model, r.Status, r.PromptTokens, r.CompletionTokens,
		r.CachedTokens, r.TotalTokens, r.LatencyMs, r.TtftMs, r.Stream, r.Estimated,
		r.ErrorType, r.AuthIndex, r.ReasoningEffort, r.TokensPerSec,
	)
	if err != nil {
		return 0, fmt.Errorf("insert request event tx failed: %w", err)
	}
	return res.LastInsertId()
}

// InsertRequestEvent inserts a record into request_events directly.
func (d *DB) InsertRequestEvent(ctx context.Context, r *RequestEventRow) (int64, error) {
	query := `
INSERT INTO request_events (ts, key_id, model, status, prompt_tokens, completion_tokens, cached_tokens, total_tokens, latency_ms, ttft_ms, stream, estimated, error_type, auth_index, reasoning_effort, tokens_per_sec)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	res, err := d.ExecContext(ctx, query,
		r.TS, r.KeyID, r.Model, r.Status, r.PromptTokens, r.CompletionTokens,
		r.CachedTokens, r.TotalTokens, r.LatencyMs, r.TtftMs, r.Stream, r.Estimated,
		r.ErrorType, r.AuthIndex, r.ReasoningEffort, r.TokensPerSec,
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
SELECT id, ts, key_id, model, status, prompt_tokens, completion_tokens, cached_tokens, total_tokens, latency_ms, ttft_ms, stream, estimated, error_type, auth_index, reasoning_effort, tokens_per_sec
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
		if err := rows.Scan(
			&r.ID, &r.TS, &keyID, &r.Model, &r.Status, &r.PromptTokens, &r.CompletionTokens,
			&r.CachedTokens, &r.TotalTokens, &r.LatencyMs, &ttft, &r.Stream, &r.Estimated,
			&errType, &r.AuthIndex, &r.ReasoningEffort, &r.TokensPerSec,
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
SELECT id, ts, key_id, model, status, prompt_tokens, completion_tokens, cached_tokens, total_tokens, latency_ms, ttft_ms, stream, estimated, error_type, auth_index, reasoning_effort, tokens_per_sec
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

// InsertErrorEvent logs an error event into error_events.
func (d *DB) InsertErrorEvent(ctx context.Context, r *ErrorEventRow) (int64, error) {
	query := `
INSERT INTO error_events (ts, key_id, auth_index, provider, model, status, error_type, error_body, retry_after_ms, recover_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	res, err := d.ExecContext(ctx, query,
		r.TS, r.KeyID, r.AuthIndex, r.Provider, r.Model, r.Status,
		r.ErrorType, r.ErrorBody, r.RetryAfterMs, r.RecoverAt,
	)
	if err != nil {
		return 0, fmt.Errorf("insert error event failed: %w", err)
	}
	return res.LastInsertId()
}

// SelectRecentErrorEvents returns recent error events.
func (d *DB) SelectRecentErrorEvents(ctx context.Context, limit int) ([]ErrorEventRow, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
SELECT id, ts, key_id, auth_index, provider, model, status, error_type, error_body, retry_after_ms, recover_at
FROM error_events ORDER BY ts DESC, id DESC LIMIT ?
`
	rows, err := d.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("select recent error events failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []ErrorEventRow
	for rows.Next() {
		var r ErrorEventRow
		var keyID sql.NullInt64
		var authIndex, provider, model, errType, errBody sql.NullString
		if err := rows.Scan(
			&r.ID, &r.TS, &keyID, &authIndex, &provider, &model, &r.Status,
			&errType, &errBody, &r.RetryAfterMs, &r.RecoverAt,
		); err != nil {
			return nil, fmt.Errorf("scan error event failed: %w", err)
		}
		if keyID.Valid {
			v := keyID.Int64
			r.KeyID = &v
		}
		if authIndex.Valid {
			v := authIndex.String
			r.AuthIndex = &v
		}
		if provider.Valid {
			v := provider.String
			r.Provider = &v
		}
		if model.Valid {
			v := model.String
			r.Model = &v
		}
		if errType.Valid {
			v := errType.String
			r.ErrorType = &v
		}
		if errBody.Valid {
			v := errBody.String
			r.ErrorBody = &v
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectActiveErrors returns error events where recover_at is in the future.
func (d *DB) SelectActiveErrors(ctx context.Context, recoverAt int64) ([]ErrorEventRow, error) {
	query := `
SELECT id, ts, key_id, auth_index, provider, model, status, error_type, error_body, retry_after_ms, recover_at
FROM error_events WHERE recover_at > ? ORDER BY recover_at DESC
`
	rows, err := d.QueryContext(ctx, query, recoverAt)
	if err != nil {
		return nil, fmt.Errorf("select active errors failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []ErrorEventRow
	for rows.Next() {
		var r ErrorEventRow
		var keyID sql.NullInt64
		var authIndex, provider, model, errType, errBody sql.NullString
		if err := rows.Scan(
			&r.ID, &r.TS, &keyID, &authIndex, &provider, &model, &r.Status,
			&errType, &errBody, &r.RetryAfterMs, &r.RecoverAt,
		); err != nil {
			return nil, fmt.Errorf("scan error event failed: %w", err)
		}
		if keyID.Valid {
			v := keyID.Int64
			r.KeyID = &v
		}
		if authIndex.Valid {
			v := authIndex.String
			r.AuthIndex = &v
		}
		if provider.Valid {
			v := provider.String
			r.Provider = &v
		}
		if model.Valid {
			v := model.String
			r.Model = &v
		}
		if errType.Valid {
			v := errType.String
			r.ErrorType = &v
		}
		if errBody.Valid {
			v := errBody.String
			r.ErrorBody = &v
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// MetaGet retrieves a key-value pair from meta table.
func (d *DB) MetaGet(ctx context.Context, key string) (string, error) {
	query := `SELECT v FROM meta WHERE k = ?`
	var val string
	err := d.QueryRowContext(ctx, query, key).Scan(&val)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("meta get failed: %w", err)
	}
	return val, nil
}

// MetaSet upserts a key-value pair into meta table.
func (d *DB) MetaSet(ctx context.Context, key, val string) error {
	query := `INSERT INTO meta (k, v) VALUES (?, ?) ON CONFLICT(k) DO UPDATE SET v = excluded.v`
	_, err := d.ExecContext(ctx, query, key, val)
	if err != nil {
		return fmt.Errorf("meta set failed: %w", err)
	}
	return nil
}
