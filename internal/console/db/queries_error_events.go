// 错误事件日志与过滤分页查询 —— 拆自 queries_events.go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

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
