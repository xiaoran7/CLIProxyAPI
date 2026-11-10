// 模型定价规则查询 —— 拆自 queries_upstream.go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SelectAllModelPrices returns all model pricing rules sorted by pattern length descending.
func (d *DB) SelectAllModelPrices(ctx context.Context) ([]ModelPriceRow, error) {
	query := `
SELECT id, pattern, prompt_price, completion_price, cache_read_price, multiplier, created_at, updated_at
FROM model_prices ORDER BY LENGTH(pattern) DESC, id DESC
`
	rows, err := d.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select all model prices failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []ModelPriceRow
	for rows.Next() {
		var r ModelPriceRow
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.Pattern, &r.PromptPrice, &r.CompletionPrice, &r.CacheReadPrice, &r.Multiplier, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan model price failed: %w", err)
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

// UpsertModelPrice inserts or updates a model pricing rule.
func (d *DB) UpsertModelPrice(ctx context.Context, pattern string, promptPrice, completionPrice, cacheReadPrice, multiplier float64) error {
	query := `
INSERT INTO model_prices (pattern, prompt_price, completion_price, cache_read_price, multiplier, updated_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(pattern) DO UPDATE SET
  prompt_price = excluded.prompt_price,
  completion_price = excluded.completion_price,
  cache_read_price = excluded.cache_read_price,
  multiplier = excluded.multiplier,
  updated_at = CURRENT_TIMESTAMP
`
	_, err := d.ExecContext(ctx, query, pattern, promptPrice, completionPrice, cacheReadPrice, multiplier)
	if err != nil {
		return fmt.Errorf("upsert model price failed: %w", err)
	}
	return nil
}

// DeleteModelPrice removes a model pricing rule by ID.
func (d *DB) DeleteModelPrice(ctx context.Context, id int64) error {
	query := `DELETE FROM model_prices WHERE id = ?`
	_, err := d.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete model price failed: %w", err)
	}
	return nil
}
