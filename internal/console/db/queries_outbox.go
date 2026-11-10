// 异步用量出件箱 (outbox) 操作 —— 拆自 queries_events.go
package db

import (
	"context"
	"database/sql"
	"fmt"
)

// InsertUsageOutbox inserts a raw request event synchronously into usage_outbox.
func (d *DB) InsertUsageOutbox(ctx context.Context, r *RequestEventRow) (int64, error) {
	query := `
INSERT INTO usage_outbox (ts, key_id, model, status, prompt_tokens, completion_tokens, cached_tokens, total_tokens, latency_ms, ttft_ms, stream, estimated, error_type, auth_index, reasoning_effort, tokens_per_sec, client_ip)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	res, err := d.ExecContext(ctx, query,
		r.TS, r.KeyID, r.Model, r.Status, r.PromptTokens, r.CompletionTokens,
		r.CachedTokens, r.TotalTokens, r.LatencyMs, r.TtftMs, r.Stream, r.Estimated,
		r.ErrorType, r.AuthIndex, r.ReasoningEffort, r.TokensPerSec, r.ClientIP,
	)
	if err != nil {
		return 0, fmt.Errorf("insert usage outbox failed: %w", err)
	}
	return res.LastInsertId()
}

// SelectAllOutbox retrieves all un-flushed items from usage_outbox in order.
func (d *DB) SelectAllOutbox(ctx context.Context) ([]RequestEventRow, error) {
	query := `
SELECT id, ts, key_id, model, status, prompt_tokens, completion_tokens, cached_tokens, total_tokens, latency_ms, ttft_ms, stream, estimated, error_type, auth_index, reasoning_effort, tokens_per_sec, client_ip
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
		var clientIP sql.NullString
		if err := rows.Scan(
			&r.ID, &r.TS, &keyID, &r.Model, &r.Status, &r.PromptTokens, &r.CompletionTokens,
			&r.CachedTokens, &r.TotalTokens, &r.LatencyMs, &ttft, &r.Stream, &r.Estimated,
			&errType, &r.AuthIndex, &r.ReasoningEffort, &r.TokensPerSec, &clientIP,
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
