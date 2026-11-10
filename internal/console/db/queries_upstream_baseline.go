// 上游基线与历史回填查询 —— 拆自 queries_upstream.go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SelectBaselinesAll retrieves all baseline rows.
func (d *DB) SelectBaselinesAll(ctx context.Context) ([]UpstreamAuthBaselineRow, error) {
	query := `SELECT auth_index, name, provider, base_requests, base_success_count, base_failure_count, base_prompt_tokens, base_completion_tokens, base_cached_tokens, base_total_tokens, updated_at FROM upstream_auth_baselines`
	rows, err := d.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select baselines failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []UpstreamAuthBaselineRow
	for rows.Next() {
		var r UpstreamAuthBaselineRow
		var name, provider sql.NullString
		var updatedAt sql.NullString
		if err := rows.Scan(
			&r.AuthIndex, &name, &provider,
			&r.BaseRequests, &r.BaseSuccessCount, &r.BaseFailureCount,
			&r.BasePromptTokens, &r.BaseCompletionTokens, &r.BaseCachedTokens, &r.BaseTotalTokens,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan baseline failed: %w", err)
		}
		if name.Valid {
			v := name.String
			r.Name = &v
		}
		if provider.Valid {
			v := provider.String
			r.Provider = &v
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

// UpsertBaseline inserts or updates an account baseline row in upstream_auth_baselines.
func (d *DB) UpsertBaseline(ctx context.Context, r *UpstreamAuthBaselineRow) error {
	query := `
INSERT INTO upstream_auth_baselines (auth_index, name, provider, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(auth_index) DO UPDATE SET
  name = CASE WHEN excluded.name IS NOT NULL AND excluded.name != '' THEN excluded.name ELSE upstream_auth_baselines.name END,
  provider = CASE WHEN excluded.provider IS NOT NULL AND excluded.provider != '' THEN excluded.provider ELSE upstream_auth_baselines.provider END,
  updated_at = CURRENT_TIMESTAMP
`
	_, err := d.ExecContext(ctx, query, r.AuthIndex, r.Name, r.Provider)
	if err != nil {
		return fmt.Errorf("upsert baseline failed: %w", err)
	}
	return nil
}

// UpsertBaselines inserts or updates multiple account baseline rows atomically.
func (d *DB) UpsertBaselines(ctx context.Context, baselines []UpstreamAuthBaselineRow) error {
	if len(baselines) == 0 {
		return nil
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for upsert baselines failed: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO upstream_auth_baselines (auth_index, name, provider, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(auth_index) DO UPDATE SET
  name = CASE WHEN excluded.name IS NOT NULL AND excluded.name != '' THEN excluded.name ELSE upstream_auth_baselines.name END,
  provider = CASE WHEN excluded.provider IS NOT NULL AND excluded.provider != '' THEN excluded.provider ELSE upstream_auth_baselines.provider END,
  updated_at = CURRENT_TIMESTAMP
`)
	if err != nil {
		return fmt.Errorf("prepare upsert baselines stmt failed: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, b := range baselines {
		if b.AuthIndex == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, b.AuthIndex, b.Name, b.Provider); err != nil {
			return fmt.Errorf("exec upsert baseline for %s failed: %w", b.AuthIndex, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert baselines failed: %w", err)
	}
	return nil
}

// BackfillUpstreamDailyStatsProviders synchronizes known providers from upstream_auth_baselines into upstream_daily_stats.
func (d *DB) BackfillUpstreamDailyStatsProviders(ctx context.Context) error {
	query := `
UPDATE upstream_daily_stats
SET provider = (
  SELECT provider FROM upstream_auth_baselines
  WHERE upstream_auth_baselines.auth_index = upstream_daily_stats.auth_index
    AND upstream_auth_baselines.provider IS NOT NULL
    AND upstream_auth_baselines.provider != ''
)
WHERE (provider = '' OR provider IS NULL)
  AND EXISTS (
    SELECT 1 FROM upstream_auth_baselines
    WHERE upstream_auth_baselines.auth_index = upstream_daily_stats.auth_index
      AND upstream_auth_baselines.provider IS NOT NULL
      AND upstream_auth_baselines.provider != ''
  )
`
	_, err := d.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("backfill upstream daily stats providers failed: %w", err)
	}
	return nil
}
