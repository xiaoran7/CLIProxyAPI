package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// UpsertUpstreamDailyStatTx records daily upstream aggregate in upstream_daily_stats inside a transaction.
func (d *DB) UpsertUpstreamDailyStatTx(ctx context.Context, tx *sql.Tx, r *UpstreamDailyStatRow) error {
	query := `
INSERT INTO upstream_daily_stats (
  date, auth_index, provider, requests, success_count, failure_count,
  prompt_tokens, completion_tokens, cached_tokens, total_tokens
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(date, auth_index) DO UPDATE SET
  requests = requests + excluded.requests,
  success_count = success_count + excluded.success_count,
  failure_count = failure_count + excluded.failure_count,
  prompt_tokens = prompt_tokens + excluded.prompt_tokens,
  completion_tokens = completion_tokens + excluded.completion_tokens,
  cached_tokens = cached_tokens + excluded.cached_tokens,
  total_tokens = total_tokens + excluded.total_tokens
`
	_, err := tx.ExecContext(ctx, query,
		r.Date, r.AuthIndex, r.Provider, r.Requests, r.SuccessCount, r.FailureCount,
		r.PromptTokens, r.CompletionTokens, r.CachedTokens, r.TotalTokens,
	)
	if err != nil {
		return fmt.Errorf("upsert upstream daily stat tx failed: %w", err)
	}
	return nil
}

// SelectUpstreamDailyAgg aggregates lifetime statistics for all auth_index credentials.
func (d *DB) SelectUpstreamDailyAgg(ctx context.Context) ([]UpstreamDailyStatRow, error) {
	query := `
SELECT
  auth_index,
  SUM(requests) AS requests,
  SUM(success_count) AS success_count,
  SUM(failure_count) AS failure_count,
  SUM(prompt_tokens) AS prompt_tokens,
  SUM(completion_tokens) AS completion_tokens,
  SUM(cached_tokens) AS cached_tokens,
  SUM(total_tokens) AS total_tokens
FROM upstream_daily_stats
WHERE auth_index != ''
GROUP BY auth_index
`
	rows, err := d.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select upstream daily agg failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []UpstreamDailyStatRow
	for rows.Next() {
		var r UpstreamDailyStatRow
		if err := rows.Scan(
			&r.AuthIndex, &r.Requests, &r.SuccessCount, &r.FailureCount,
			&r.PromptTokens, &r.CompletionTokens, &r.CachedTokens, &r.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("scan upstream daily agg failed: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

// SelectUpstreamDailyAggSince aggregates statistics for auth_index on or after sinceDate.
func (d *DB) SelectUpstreamDailyAggSince(ctx context.Context, sinceDate string) ([]UpstreamDailyStatRow, error) {
	query := `
SELECT
  auth_index,
  SUM(requests) AS requests,
  SUM(success_count) AS success_count,
  SUM(failure_count) AS failure_count,
  SUM(prompt_tokens) AS prompt_tokens,
  SUM(completion_tokens) AS completion_tokens,
  SUM(cached_tokens) AS cached_tokens,
  SUM(total_tokens) AS total_tokens
FROM upstream_daily_stats
WHERE auth_index != '' AND date >= ?
GROUP BY auth_index
`
	rows, err := d.QueryContext(ctx, query, sinceDate)
	if err != nil {
		return nil, fmt.Errorf("select upstream daily agg since failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []UpstreamDailyStatRow
	for rows.Next() {
		var r UpstreamDailyStatRow
		if err := rows.Scan(
			&r.AuthIndex, &r.Requests, &r.SuccessCount, &r.FailureCount,
			&r.PromptTokens, &r.CompletionTokens, &r.CachedTokens, &r.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("scan upstream daily agg since failed: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

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

// InsertQuotaSnapshot saves an upstream quota percentage snapshot.
func (d *DB) InsertQuotaSnapshot(ctx context.Context, r *QuotaSnapshotRow) (int64, error) {
	query := `
INSERT INTO quota_snapshots (ts, auth_index, provider, account_email, window_name, remaining_percent, reset_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`
	res, err := d.ExecContext(ctx, query, r.TS, r.AuthIndex, r.Provider, r.AccountEmail, r.WindowName, r.RemainingPercent, r.ResetAt)
	if err != nil {
		return 0, fmt.Errorf("insert quota snapshot failed: %w", err)
	}
	return res.LastInsertId()
}

// SelectQuotaSnapshotsByAuth retrieves quota snapshots for an auth_index since timestamp.
func (d *DB) SelectQuotaSnapshotsByAuth(ctx context.Context, authIndex string, sinceTS int64) ([]QuotaSnapshotRow, error) {
	query := `
SELECT id, ts, auth_index, provider, account_email, window_name, remaining_percent, reset_at
FROM quota_snapshots WHERE auth_index = ? AND ts >= ? ORDER BY ts ASC
`
	rows, err := d.QueryContext(ctx, query, authIndex, sinceTS)
	if err != nil {
		return nil, fmt.Errorf("select quota snapshots failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []QuotaSnapshotRow
	for rows.Next() {
		var r QuotaSnapshotRow
		var email, winName sql.NullString
		var resetAt sql.NullInt64
		if err := rows.Scan(&r.ID, &r.TS, &r.AuthIndex, &r.Provider, &email, &winName, &r.RemainingPercent, &resetAt); err != nil {
			return nil, fmt.Errorf("scan quota snapshot failed: %w", err)
		}
		if email.Valid {
			v := email.String
			r.AccountEmail = &v
		}
		if winName.Valid {
			v := winName.String
			r.WindowName = &v
		}
		if resetAt.Valid {
			v := resetAt.Int64
			r.ResetAt = &v
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}
	return list, nil
}

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
