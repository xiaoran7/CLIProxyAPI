package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
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

func buildEventFilterWhere(params EventFilterParams) (string, []interface{}) {
	var where []string
	var args []interface{}

	if params.KeyID != nil {
		where = append(where, "key_id = ?")
		args = append(args, *params.KeyID)
	}

	if params.AuthIndex != "" && params.AuthIndex != "all" {
		where = append(where, "auth_index = ?")
		args = append(args, params.AuthIndex)
	}

	if params.Provider != "" && params.Provider != "all" {
		var provOr []string
		if len(params.ProviderAuths) > 0 {
			placeholders := make([]string, len(params.ProviderAuths))
			for i, a := range params.ProviderAuths {
				placeholders[i] = "?"
				args = append(args, a)
			}
			provOr = append(provOr, fmt.Sprintf("auth_index IN (%s)", strings.Join(placeholders, ",")))
		}
		for _, mp := range params.ModelPatterns {
			if len(params.ExcludeAuths) > 0 {
				exPlaceholders := make([]string, len(params.ExcludeAuths))
				for i, ea := range params.ExcludeAuths {
					exPlaceholders[i] = "?"
					args = append(args, ea)
				}
				provOr = append(provOr, fmt.Sprintf("(model LIKE ? AND (auth_index IS NULL OR auth_index = '' OR auth_index NOT IN (%s)))", strings.Join(exPlaceholders, ",")))
				args = append(args, "%"+mp+"%")
			} else {
				provOr = append(provOr, "model LIKE ?")
				args = append(args, "%"+mp+"%")
			}
		}
		for _, ap := range params.AuthPatterns {
			provOr = append(provOr, "auth_index LIKE ?")
			args = append(args, "%"+ap+"%")
		}
		if len(provOr) > 0 {
			where = append(where, "("+strings.Join(provOr, " OR ")+")")
		}
	}

	switch params.Filter {
	case "cached":
		where = append(where, "cached_tokens > 0")
	case "2xx":
		where = append(where, "status BETWEEN 200 AND 299")
	case "4xx":
		where = append(where, "status BETWEEN 400 AND 499")
	case "5xx":
		where = append(where, "status BETWEEN 500 AND 599")
	case "blocked":
		where = append(where, "error_type IN ('quota_exceeded', 'rate_limited', 'model_forbidden')")
	case "errors":
		where = append(where, "(status >= 400 OR error_type IS NOT NULL)")
	}

	if term := strings.TrimSpace(params.Search); term != "" {
		pattern := "%" + term + "%"
		where = append(where, "(model LIKE ? OR error_type LIKE ? OR auth_index LIKE ? OR reasoning_effort LIKE ? OR CAST(key_id AS TEXT) LIKE ? OR EXISTS (SELECT 1 FROM keys WHERE keys.id = request_events.key_id AND (keys.alias LIKE ? OR keys.api_key LIKE ?)) OR EXISTS (SELECT 1 FROM upstream_auth_baselines WHERE upstream_auth_baselines.auth_index = request_events.auth_index AND upstream_auth_baselines.name LIKE ?))")
		args = append(args, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern)
	}

	// Time filters
	if params.Since != nil {
		where = append(where, "ts >= ?")
		args = append(args, *params.Since)
	} else if params.TimeRange != "" && params.TimeRange != "all" {
		nowSec := time.Now().Unix()
		var resolvedSince *int64
		switch params.TimeRange {
		case "1h":
			s := nowSec - 3600
			resolvedSince = &s
		case "today":
			cstZone := time.FixedZone("CST", 8*3600)
			nowInCST := time.Now().In(cstZone)
			todayStart := time.Date(nowInCST.Year(), nowInCST.Month(), nowInCST.Day(), 0, 0, 0, 0, cstZone).Unix()
			resolvedSince = &todayStart
		case "24h":
			s := nowSec - 86400
			resolvedSince = &s
		case "3d":
			s := nowSec - 3*86400
			resolvedSince = &s
		case "7d":
			s := nowSec - 7*86400
			resolvedSince = &s
		case "30d":
			s := nowSec - 30*86400
			resolvedSince = &s
		}
		if resolvedSince != nil {
			where = append(where, "ts >= ?")
			args = append(args, *resolvedSince)
		}
	}

	if params.Until != nil {
		where = append(where, "ts <= ?")
		args = append(args, *params.Until)
	}

	if params.BeforeID > 0 {
		where = append(where, "id < ?")
		args = append(args, params.BeforeID)
	}

	if len(where) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(where, " AND "), args
}

// SelectEventsFilteredPaged returns paginated request events matching filter params using delayed join.
func (d *DB) SelectEventsFilteredPaged(ctx context.Context, params EventFilterParams) ([]RequestEventRow, error) {
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 25
	} else if pageSize > 100 {
		pageSize = 100
	}
	page := params.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize

	whereClause, args := buildEventFilterWhere(params)

	query := fmt.Sprintf(`
SELECT r.id, r.ts, r.key_id, r.model, r.status, r.prompt_tokens, r.completion_tokens, r.cached_tokens, r.total_tokens, r.latency_ms, r.ttft_ms, r.stream, r.estimated, r.error_type, r.auth_index, r.reasoning_effort, r.tokens_per_sec
FROM request_events r
JOIN (
    SELECT id FROM request_events %s
    ORDER BY id DESC
    LIMIT ? OFFSET ?
) p ON r.id = p.id
ORDER BY r.id DESC
`, whereClause)

	queryArgs := append(args, pageSize, offset)
	rows, err := d.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("select events filtered paged failed: %w", err)
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
			return nil, fmt.Errorf("scan request event row failed: %w", err)
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

// CountEventsFiltered returns total matching request events count.
func (d *DB) CountEventsFiltered(ctx context.Context, params EventFilterParams) (int64, error) {
	whereClause, args := buildEventFilterWhere(params)
	query := fmt.Sprintf("SELECT COUNT(*) FROM request_events %s", whereClause)
	var count int64
	err := d.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count events filtered failed: %w", err)
	}
	return count, nil
}

// CountTodayEvents returns total requests recorded today.
func (d *DB) CountTodayEvents(ctx context.Context, sinceTS int64, keyID *int64) (int64, error) {
	if sinceTS <= 0 {
		cst := time.FixedZone("CST", 8*3600)
		nowCST := time.Now().In(cst)
		sinceTS = time.Date(nowCST.Year(), nowCST.Month(), nowCST.Day(), 0, 0, 0, 0, cst).Unix()
	}
	var query string
	var args []interface{}
	if keyID != nil {
		query = "SELECT COUNT(*) FROM request_events WHERE ts >= ? AND key_id = ?"
		args = []interface{}{sinceTS, *keyID}
	} else {
		query = "SELECT COUNT(*) FROM request_events WHERE ts >= ?"
		args = []interface{}{sinceTS}
	}
	var count int64
	err := d.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count today events failed: %w", err)
	}
	return count, nil
}

func buildErrorFilterWhere(params ErrorFilterParams) (string, []interface{}) {
	var where []string
	var args []interface{}

	if params.KeyID != nil {
		where = append(where, "key_id = ?")
		args = append(args, *params.KeyID)
	}

	if params.Since != nil {
		where = append(where, "ts >= ?")
		args = append(args, *params.Since)
	} else if params.TimeRange != "" && params.TimeRange != "all" {
		nowSec := time.Now().Unix()
		var resolvedSince *int64
		switch params.TimeRange {
		case "1h":
			s := nowSec - 3600
			resolvedSince = &s
		case "today":
			cstZone := time.FixedZone("CST", 8*3600)
			nowInCST := time.Now().In(cstZone)
			todayStart := time.Date(nowInCST.Year(), nowInCST.Month(), nowInCST.Day(), 0, 0, 0, 0, cstZone).Unix()
			resolvedSince = &todayStart
		case "24h":
			s := nowSec - 86400
			resolvedSince = &s
		case "3d":
			s := nowSec - 3*86400
			resolvedSince = &s
		case "7d":
			s := nowSec - 7*86400
			resolvedSince = &s
		case "30d":
			s := nowSec - 30*86400
			resolvedSince = &s
		}
		if resolvedSince != nil {
			where = append(where, "ts >= ?")
			args = append(args, *resolvedSince)
		}
	}

	if params.Until != nil {
		where = append(where, "ts <= ?")
		args = append(args, *params.Until)
	}

	if params.BeforeID > 0 {
		where = append(where, "id < ?")
		args = append(args, params.BeforeID)
	}

	if len(where) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(where, " AND "), args
}

// SelectErrorsFilteredPaged returns paginated error events matching filter params using delayed join.
func (d *DB) SelectErrorsFilteredPaged(ctx context.Context, params ErrorFilterParams) ([]ErrorEventRow, error) {
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 25
	} else if pageSize > 100 {
		pageSize = 100
	}
	page := params.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize

	whereClause, args := buildErrorFilterWhere(params)

	query := fmt.Sprintf(`
SELECT e.id, e.ts, e.key_id, e.auth_index, e.provider, e.model, e.status, e.error_type, e.error_body, e.retry_after_ms, e.recover_at
FROM error_events e
JOIN (
    SELECT id FROM error_events %s
    ORDER BY ts DESC, id DESC
    LIMIT ? OFFSET ?
) p ON e.id = p.id
ORDER BY e.ts DESC, e.id DESC
`, whereClause)

	queryArgs := append(args, pageSize, offset)
	rows, err := d.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("select errors filtered paged failed: %w", err)
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
			return nil, fmt.Errorf("scan error event row failed: %w", err)
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

// CountErrorsFiltered returns total matching error events count.
func (d *DB) CountErrorsFiltered(ctx context.Context, params ErrorFilterParams) (int64, error) {
	whereClause, args := buildErrorFilterWhere(params)
	query := fmt.Sprintf("SELECT COUNT(*) FROM error_events %s", whereClause)
	var count int64
	err := d.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count errors filtered failed: %w", err)
	}
	return count, nil
}

// CountTodayErrors returns total error events recorded today.
func (d *DB) CountTodayErrors(ctx context.Context, sinceTS int64, keyID *int64) (int64, error) {
	if sinceTS <= 0 {
		cst := time.FixedZone("CST", 8*3600)
		nowCST := time.Now().In(cst)
		sinceTS = time.Date(nowCST.Year(), nowCST.Month(), nowCST.Day(), 0, 0, 0, 0, cst).Unix()
	}
	var query string
	var args []interface{}
	if keyID != nil {
		query = "SELECT COUNT(*) FROM error_events WHERE ts >= ? AND key_id = ?"
		args = []interface{}{sinceTS, *keyID}
	} else {
		query = "SELECT COUNT(*) FROM error_events WHERE ts >= ?"
		args = []interface{}{sinceTS}
	}
	var count int64
	err := d.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count today errors failed: %w", err)
	}
	return count, nil
}

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

	return res.RowsAffected()
}
