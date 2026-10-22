package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SelectAllKeys returns all API keys from the keys table.
func (d *DB) SelectAllKeys(ctx context.Context) ([]KeyRow, error) {
	query := `SELECT id, api_key, alias, active, created_at, updated_at FROM keys ORDER BY id ASC`
	rows, err := d.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select all keys failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var keys []KeyRow
	for rows.Next() {
		var k KeyRow
		var alias sql.NullString
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&k.ID, &k.APIKey, &alias, &k.Active, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan key failed: %w", err)
		}
		if alias.Valid {
			k.Alias = alias.String
		}
		if createdAt.Valid {
			if t, err := time.Parse("2006-01-02 15:04:05", createdAt.String); err == nil {
				k.CreatedAt = t
			}
		}
		if updatedAt.Valid {
			if t, err := time.Parse("2006-01-02 15:04:05", updatedAt.String); err == nil {
				k.UpdatedAt = t
			}
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return keys, nil
}

// SelectKeyByID returns a key by its ID.
func (d *DB) SelectKeyByID(ctx context.Context, id int64) (*KeyRow, error) {
	query := `SELECT id, api_key, alias, active, created_at, updated_at FROM keys WHERE id = ?`
	row := d.QueryRowContext(ctx, query, id)

	var k KeyRow
	var alias sql.NullString
	var createdAt, updatedAt sql.NullString
	if err := row.Scan(&k.ID, &k.APIKey, &alias, &k.Active, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("select key by id failed: %w", err)
	}
	if alias.Valid {
		k.Alias = alias.String
	}
	if createdAt.Valid {
		if t, err := time.Parse("2006-01-02 15:04:05", createdAt.String); err == nil {
			k.CreatedAt = t
		}
	}
	if updatedAt.Valid {
		if t, err := time.Parse("2006-01-02 15:04:05", updatedAt.String); err == nil {
			k.UpdatedAt = t
		}
	}
	return &k, nil
}

// SelectKeyByRaw returns a key by its plaintext raw api_key.
func (d *DB) SelectKeyByRaw(ctx context.Context, apiKey string) (*KeyRow, error) {
	query := `SELECT id, api_key, alias, active, created_at, updated_at FROM keys WHERE api_key = ?`
	row := d.QueryRowContext(ctx, query, apiKey)

	var k KeyRow
	var alias sql.NullString
	var createdAt, updatedAt sql.NullString
	if err := row.Scan(&k.ID, &k.APIKey, &alias, &k.Active, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("select key by raw failed: %w", err)
	}
	if alias.Valid {
		k.Alias = alias.String
	}
	if createdAt.Valid {
		if t, err := time.Parse("2006-01-02 15:04:05", createdAt.String); err == nil {
			k.CreatedAt = t
		}
	}
	if updatedAt.Valid {
		if t, err := time.Parse("2006-01-02 15:04:05", updatedAt.String); err == nil {
			k.UpdatedAt = t
		}
	}
	return &k, nil
}

// InsertKey inserts a new API key and returns its last inserted ID.
func (d *DB) InsertKey(ctx context.Context, apiKey, alias string) (int64, error) {
	query := `INSERT INTO keys (api_key, alias, active) VALUES (?, ?, 1)`
	res, err := d.ExecContext(ctx, query, apiKey, alias)
	if err != nil {
		return 0, fmt.Errorf("insert key failed: %w", err)
	}
	return res.LastInsertId()
}

// UpdateKeyRaw updates the raw key string for key rotation.
func (d *DB) UpdateKeyRaw(ctx context.Context, id int64, apiKey string) error {
	query := `UPDATE keys SET api_key = ?, updated_at = datetime('now') WHERE id = ?`
	_, err := d.ExecContext(ctx, query, apiKey, id)
	if err != nil {
		return fmt.Errorf("update key raw failed: %w", err)
	}
	return nil
}

// SetActive toggles the active state of an API key.
func (d *DB) SetActive(ctx context.Context, id int64, active int) error {
	query := `UPDATE keys SET active = ?, updated_at = datetime('now') WHERE id = ?`
	_, err := d.ExecContext(ctx, query, active, id)
	if err != nil {
		return fmt.Errorf("set key active failed: %w", err)
	}
	return nil
}

// UpdateAlias updates the alias of an API key.
func (d *DB) UpdateAlias(ctx context.Context, id int64, alias string) error {
	query := `UPDATE keys SET alias = ?, updated_at = datetime('now') WHERE id = ?`
	_, err := d.ExecContext(ctx, query, alias, id)
	if err != nil {
		return fmt.Errorf("update key alias failed: %w", err)
	}
	return nil
}

// DeleteKey permanently deletes an API key and its associated quota rule.
func (d *DB) DeleteKey(ctx context.Context, id int64) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx failed: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM quota_rules WHERE key_id = ?`, id); err != nil {
		return fmt.Errorf("delete quota rule failed: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM keys WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete key failed: %w", err)
	}

	return tx.Commit()
}

// MarkKeyDeleted records an API key as explicitly deleted so reconciliation loops do not resurrect it.
func (d *DB) MarkKeyDeleted(ctx context.Context, apiKey string) error {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return nil
	}
	query := `INSERT OR REPLACE INTO deleted_keys (api_key, deleted_at) VALUES (?, datetime('now'))`
	_, err := d.ExecContext(ctx, query, trimmed)
	if err != nil {
		return fmt.Errorf("mark key deleted failed: %w", err)
	}
	return nil
}

// UnmarkKeyDeleted removes an API key from the tombstone list when explicitly created.
func (d *DB) UnmarkKeyDeleted(ctx context.Context, apiKey string) error {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return nil
	}
	query := `DELETE FROM deleted_keys WHERE api_key = ?`
	_, err := d.ExecContext(ctx, query, trimmed)
	if err != nil {
		return fmt.Errorf("unmark key deleted failed: %w", err)
	}
	return nil
}

// IsKeyDeleted returns whether an API key has been explicitly deleted.
func (d *DB) IsKeyDeleted(ctx context.Context, apiKey string) (bool, error) {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return false, nil
	}
	query := `SELECT 1 FROM deleted_keys WHERE api_key = ? LIMIT 1`
	var found int
	err := d.QueryRowContext(ctx, query, trimmed).Scan(&found)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("check is key deleted failed: %w", err)
	}
	return found == 1, nil
}

// SelectAllQuotaRules returns all quota rules from the quota_rules table.
func (d *DB) SelectAllQuotaRules(ctx context.Context) ([]QuotaRuleRow, error) {
	query := `SELECT key_id, enabled, daily_limit, hard_ratio, rpm_limit, model_whitelist, model_group, updated_at FROM quota_rules`
	rows, err := d.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select all quota rules failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var rules []QuotaRuleRow
	for rows.Next() {
		var r QuotaRuleRow
		var dailyLimit sql.NullInt64
		var hardRatio sql.NullFloat64
		var rpmLimit sql.NullInt64
		var modelWhitelist sql.NullString
		var modelGroup sql.NullString
		var updatedAt sql.NullString
		if err := rows.Scan(&r.KeyID, &r.Enabled, &dailyLimit, &hardRatio, &rpmLimit, &modelWhitelist, &modelGroup, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan quota rule failed: %w", err)
		}
		if dailyLimit.Valid {
			v := dailyLimit.Int64
			r.DailyLimit = &v
		}
		if hardRatio.Valid && hardRatio.Float64 > 0 {
			r.HardRatio = hardRatio.Float64
		} else {
			r.HardRatio = 1.2
		}
		if rpmLimit.Valid {
			r.RPMLimit = int(rpmLimit.Int64)
		}
		if modelWhitelist.Valid {
			r.ModelWhitelist = modelWhitelist.String
		}
		if modelGroup.Valid {
			r.ModelGroup = modelGroup.String
		}
		if updatedAt.Valid {
			if t, err := time.Parse("2006-01-02 15:04:05", updatedAt.String); err == nil {
				r.UpdatedAt = t
			}
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return rules, nil
}

// SelectQuotaRuleByKeyID retrieves a quota rule for a specific key.
func (d *DB) SelectQuotaRuleByKeyID(ctx context.Context, keyID int64) (*QuotaRuleRow, error) {
	query := `SELECT key_id, enabled, daily_limit, hard_ratio, rpm_limit, model_whitelist, model_group, updated_at FROM quota_rules WHERE key_id = ?`
	row := d.QueryRowContext(ctx, query, keyID)

	var r QuotaRuleRow
	var dailyLimit sql.NullInt64
	var hardRatio sql.NullFloat64
	var rpmLimit sql.NullInt64
	var modelWhitelist sql.NullString
	var modelGroup sql.NullString
	var updatedAt sql.NullString
	if err := row.Scan(&r.KeyID, &r.Enabled, &dailyLimit, &hardRatio, &rpmLimit, &modelWhitelist, &modelGroup, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("select quota rule by key_id failed: %w", err)
	}
	if dailyLimit.Valid {
		v := dailyLimit.Int64
		r.DailyLimit = &v
	}
	if hardRatio.Valid && hardRatio.Float64 > 0 {
		r.HardRatio = hardRatio.Float64
	} else {
		r.HardRatio = 1.2
	}
	if rpmLimit.Valid {
		r.RPMLimit = int(rpmLimit.Int64)
	}
	if modelWhitelist.Valid {
		r.ModelWhitelist = modelWhitelist.String
	}
	if modelGroup.Valid {
		r.ModelGroup = modelGroup.String
	}
	if updatedAt.Valid {
		if t, err := time.Parse("2006-01-02 15:04:05", updatedAt.String); err == nil {
			r.UpdatedAt = t
		}
	}
	return &r, nil
}

// UpsertQuotaRule creates or updates a quota rule for a key.
func (d *DB) UpsertQuotaRule(ctx context.Context, r *QuotaRuleRow) error {
	query := `
INSERT INTO quota_rules (key_id, enabled, daily_limit, hard_ratio, rpm_limit, model_whitelist, model_group, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(key_id) DO UPDATE SET
  enabled = excluded.enabled,
  daily_limit = excluded.daily_limit,
  hard_ratio = excluded.hard_ratio,
  rpm_limit = excluded.rpm_limit,
  model_whitelist = excluded.model_whitelist,
  model_group = excluded.model_group,
  updated_at = CURRENT_TIMESTAMP
`
	var dl interface{}
	if r.DailyLimit != nil {
		dl = *r.DailyLimit
	}
	hardRatio := r.HardRatio
	if hardRatio <= 0 {
		hardRatio = 1.2
	}

	_, err := d.ExecContext(ctx, query, r.KeyID, r.Enabled, dl, hardRatio, r.RPMLimit, r.ModelWhitelist, r.ModelGroup)
	if err != nil {
		return fmt.Errorf("upsert quota rule failed: %w", err)
	}
	return nil
}

// DeleteQuotaRule deletes a quota rule for a key.
func (d *DB) DeleteQuotaRule(ctx context.Context, keyID int64) error {
	query := `DELETE FROM quota_rules WHERE key_id = ?`
	_, err := d.ExecContext(ctx, query, keyID)
	if err != nil {
		return fmt.Errorf("delete quota rule failed: %w", err)
	}
	return nil
}
