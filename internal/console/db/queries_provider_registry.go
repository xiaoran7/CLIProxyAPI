// 服务商注册表管理查询 —— 拆自 queries_upstream.go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SelectProviderRegistry returns all AI provider registry entries.
func (d *DB) SelectProviderRegistry(ctx context.Context) ([]ProviderRegistryRow, error) {
	query := `
SELECT provider_key, label, model_patterns, auth_patterns, source, enabled, sort_order, created_at, updated_at
FROM provider_registry ORDER BY sort_order ASC, provider_key ASC
`
	rows, err := d.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select provider registry failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []ProviderRegistryRow
	for rows.Next() {
		var r ProviderRegistryRow
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&r.ProviderKey, &r.Label, &r.ModelPatterns, &r.AuthPatterns, &r.Source, &r.Enabled, &r.SortOrder, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan provider registry failed: %w", err)
		}
		if createdAt.Valid {
			if t, err := time.Parse("2006-01-02 15:04:05", createdAt.String); err == nil {
				r.CreatedAt = t
			}
		}
		if updatedAt.Valid {
			if t, err := time.Parse("2006-01-02 15:04:05", updatedAt.String); err == nil {
				r.UpdatedAt = t
			}
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// UpsertProviderRegistry creates or updates an AI provider registry entry.
func (d *DB) UpsertProviderRegistry(ctx context.Context, r *ProviderRegistryRow) error {
	query := `
INSERT INTO provider_registry (provider_key, label, model_patterns, auth_patterns, source, enabled, sort_order, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT(provider_key) DO UPDATE SET
  label = excluded.label,
  model_patterns = excluded.model_patterns,
  auth_patterns = excluded.auth_patterns,
  source = excluded.source,
  enabled = excluded.enabled,
  sort_order = excluded.sort_order,
  updated_at = datetime('now')
`
	_, err := d.ExecContext(ctx, query, r.ProviderKey, r.Label, r.ModelPatterns, r.AuthPatterns, r.Source, r.Enabled, r.SortOrder)
	if err != nil {
		return fmt.Errorf("upsert provider registry failed: %w", err)
	}
	return nil
}

// DeleteProviderRegistry removes an AI provider registry entry by key.
func (d *DB) DeleteProviderRegistry(ctx context.Context, providerKey string) error {
	query := `DELETE FROM provider_registry WHERE provider_key = ?`
	_, err := d.ExecContext(ctx, query, providerKey)
	if err != nil {
		return fmt.Errorf("delete provider registry failed: %w", err)
	}
	return nil
}
