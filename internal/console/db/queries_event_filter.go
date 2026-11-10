// 请求事件过滤分页与统计查询 —— 拆自 queries_events.go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

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
		where = append(where, "(model LIKE ? OR error_type LIKE ? OR auth_index LIKE ? OR reasoning_effort LIKE ? OR client_ip LIKE ? OR CAST(key_id AS TEXT) LIKE ? OR EXISTS (SELECT 1 FROM keys WHERE keys.id = request_events.key_id AND (keys.alias LIKE ? OR keys.api_key LIKE ?)) OR EXISTS (SELECT 1 FROM upstream_auth_baselines WHERE upstream_auth_baselines.auth_index = request_events.auth_index AND upstream_auth_baselines.name LIKE ?))")
		args = append(args, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern)
	}

	if params.ClientIP != "" {
		where = append(where, "client_ip = ?")
		args = append(args, params.ClientIP)
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
SELECT r.id, r.ts, r.key_id, r.model, r.status, r.prompt_tokens, r.completion_tokens, r.cached_tokens, r.total_tokens, r.latency_ms, r.ttft_ms, r.stream, r.estimated, r.error_type, r.auth_index, r.reasoning_effort, r.tokens_per_sec, r.client_ip
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
		var clientIP sql.NullString
		if err := rows.Scan(
			&r.ID, &r.TS, &kid, &r.Model, &r.Status, &r.PromptTokens, &r.CompletionTokens,
			&r.CachedTokens, &r.TotalTokens, &r.LatencyMs, &ttft, &r.Stream, &r.Estimated,
			&errType, &r.AuthIndex, &r.ReasoningEffort, &r.TokensPerSec, &clientIP,
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
