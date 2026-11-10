// 上游人机验证挑战记录查询 —— 拆自 queries_upstream.go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SelectValidations returns upstream validation challenges by status (e.g. "pending").
func (d *DB) SelectValidations(ctx context.Context, status string) ([]UpstreamValidationRow, error) {
	var query string
	var rows *sql.Rows
	var err error

	if status != "" {
		query = `SELECT id, ts, auth_index, provider, account_name, validation_url, error_message, raw_error, status, updated_at FROM upstream_validations WHERE status = ? ORDER BY id DESC`
		rows, err = d.QueryContext(ctx, query, status)
	} else {
		query = `SELECT id, ts, auth_index, provider, account_name, validation_url, error_message, raw_error, status, updated_at FROM upstream_validations ORDER BY id DESC LIMIT 100`
		rows, err = d.QueryContext(ctx, query)
	}
	if err != nil {
		return nil, fmt.Errorf("select validations failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []UpstreamValidationRow
	for rows.Next() {
		var r UpstreamValidationRow
		var acct, errMsg, rawErr sql.NullString
		if err := rows.Scan(&r.ID, &r.TS, &r.AuthIndex, &r.Provider, &acct, &r.ValidationURL, &errMsg, &rawErr, &r.Status, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan validation failed: %w", err)
		}
		if acct.Valid {
			v := acct.String
			r.AccountName = &v
		}
		if errMsg.Valid {
			v := errMsg.String
			r.ErrorMessage = &v
		}
		if rawErr.Valid {
			v := rawErr.String
			r.RawError = &v
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// InsertValidation records a new 403 challenge URL validation request.
func (d *DB) InsertValidation(ctx context.Context, r *UpstreamValidationRow) (int64, error) {
	query := `
INSERT INTO upstream_validations (ts, auth_index, provider, account_name, validation_url, error_message, raw_error, status, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?)
`
	res, err := d.ExecContext(ctx, query, r.TS, r.AuthIndex, r.Provider, r.AccountName, r.ValidationURL, r.ErrorMessage, r.RawError, r.UpdatedAt)
	if err != nil {
		return 0, fmt.Errorf("insert validation failed: %w", err)
	}
	return res.LastInsertId()
}

// ResolveValidation marks a validation item as resolved.
func (d *DB) ResolveValidation(ctx context.Context, id int64) error {
	query := `UPDATE upstream_validations SET status = 'resolved', updated_at = ? WHERE id = ?`
	now := time.Now().Unix()
	_, err := d.ExecContext(ctx, query, now, id)
	if err != nil {
		return fmt.Errorf("resolve validation failed: %w", err)
	}
	return nil
}

// ResolveValidationByAuth marks pending validations for an auth_index as resolved.
func (d *DB) ResolveValidationByAuth(ctx context.Context, authIndex string) error {
	query := `UPDATE upstream_validations SET status = 'resolved', updated_at = ? WHERE auth_index = ? AND status = 'pending'`
	now := time.Now().Unix()
	_, err := d.ExecContext(ctx, query, now, authIndex)
	if err != nil {
		return fmt.Errorf("resolve validation by auth failed: %w", err)
	}
	return nil
}

// DismissValidation marks a validation item as dismissed.
func (d *DB) DismissValidation(ctx context.Context, id int64) error {
	query := `UPDATE upstream_validations SET status = 'dismissed', updated_at = ? WHERE id = ?`
	now := time.Now().Unix()
	_, err := d.ExecContext(ctx, query, now, id)
	if err != nil {
		return fmt.Errorf("dismiss validation failed: %w", err)
	}
	return nil
}

// DeleteValidation permanently removes a validation entry.
func (d *DB) DeleteValidation(ctx context.Context, id int64) error {
	query := `DELETE FROM upstream_validations WHERE id = ?`
	_, err := d.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete validation failed: %w", err)
	}
	return nil
}
