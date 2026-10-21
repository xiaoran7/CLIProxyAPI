package db

import (
	"context"
	"fmt"
)

// ddlStatements defines the complete schema for Console subsystem, 100% compatible with Gate.
const ddlStatements = `
CREATE TABLE IF NOT EXISTS keys (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  api_key TEXT NOT NULL UNIQUE,
  alias TEXT DEFAULT '',
  active INTEGER NOT NULL DEFAULT 1,
  created_at TEXT DEFAULT (datetime('now')),
  updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS quota_rules (
  key_id INTEGER PRIMARY KEY,
  enabled INTEGER NOT NULL DEFAULT 0,
  daily_limit INTEGER,
  hard_ratio REAL NOT NULL DEFAULT 1.2,
  rpm_limit INTEGER,
  model_whitelist TEXT DEFAULT '',
  model_group TEXT DEFAULT '',
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS usage_daily (
  date TEXT NOT NULL,
  key_id INTEGER NOT NULL,
  model TEXT NOT NULL,
  requests INTEGER NOT NULL DEFAULT 0,
  prompt_tokens INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  cached_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  estimated_tokens INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  failure_count INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (date, key_id, model)
);

CREATE TABLE IF NOT EXISTS request_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  key_id INTEGER,
  model TEXT,
  status INTEGER NOT NULL,
  prompt_tokens INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  cached_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  latency_ms INTEGER NOT NULL DEFAULT 0,
  ttft_ms INTEGER,
  stream INTEGER NOT NULL DEFAULT 0,
  estimated INTEGER NOT NULL DEFAULT 0,
  error_type TEXT,
  auth_index TEXT DEFAULT '',
  reasoning_effort TEXT DEFAULT '',
  tokens_per_sec REAL NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS usage_outbox (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  key_id INTEGER,
  model TEXT,
  status INTEGER NOT NULL,
  prompt_tokens INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  cached_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  latency_ms INTEGER NOT NULL DEFAULT 0,
  ttft_ms INTEGER,
  stream INTEGER NOT NULL DEFAULT 0,
  estimated INTEGER NOT NULL DEFAULT 0,
  error_type TEXT,
  auth_index TEXT DEFAULT '',
  reasoning_effort TEXT DEFAULT '',
  tokens_per_sec REAL NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS meta (
  k TEXT PRIMARY KEY,
  v TEXT
);

CREATE TABLE IF NOT EXISTS error_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  key_id INTEGER,
  auth_index TEXT,
  provider TEXT,
  model TEXT,
  status INTEGER NOT NULL,
  error_type TEXT,
  error_body TEXT,
  retry_after_ms INTEGER DEFAULT 0,
  recover_at INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS model_prices (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  pattern TEXT UNIQUE NOT NULL,
  prompt_price REAL NOT NULL,
  completion_price REAL NOT NULL,
  cache_read_price REAL NOT NULL,
  multiplier REAL NOT NULL DEFAULT 1.0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS quota_snapshots (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  auth_index TEXT NOT NULL,
  provider TEXT NOT NULL,
  account_email TEXT,
  window_name TEXT,
  remaining_percent REAL NOT NULL,
  reset_at INTEGER
);

CREATE TABLE IF NOT EXISTS upstream_auth_baselines (
  auth_index TEXT PRIMARY KEY,
  name TEXT,
  provider TEXT,
  base_requests INTEGER NOT NULL DEFAULT 0,
  base_success_count INTEGER NOT NULL DEFAULT 0,
  base_failure_count INTEGER NOT NULL DEFAULT 0,
  base_prompt_tokens INTEGER NOT NULL DEFAULT 0,
  base_completion_tokens INTEGER NOT NULL DEFAULT 0,
  base_cached_tokens INTEGER NOT NULL DEFAULT 0,
  base_total_tokens INTEGER NOT NULL DEFAULT 0,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS upstream_daily_stats (
  date TEXT NOT NULL,
  auth_index TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT '',
  requests INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  failure_count INTEGER NOT NULL DEFAULT 0,
  prompt_tokens INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  cached_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (date, auth_index)
);

CREATE TABLE IF NOT EXISTS upstream_validations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  auth_index TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT 'antigravity',
  account_name TEXT,
  validation_url TEXT NOT NULL,
  error_message TEXT,
  raw_error TEXT,
  status TEXT NOT NULL DEFAULT 'pending',
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS provider_registry (
  provider_key TEXT PRIMARY KEY,
  label TEXT NOT NULL,
  model_patterns TEXT NOT NULL DEFAULT '[]',
  auth_patterns TEXT NOT NULL DEFAULT '[]',
  source TEXT NOT NULL DEFAULT 'builtin',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 100,
  created_at TEXT DEFAULT (datetime('now')),
  updated_at TEXT DEFAULT (datetime('now'))
);
`

// indexStatements defines indexes for high throughput querying.
var indexStatements = []string{
	"CREATE INDEX IF NOT EXISTS idx_events_ts ON request_events (ts);",
	"CREATE INDEX IF NOT EXISTS idx_events_key_id ON request_events (key_id, id DESC);",
	"CREATE INDEX IF NOT EXISTS idx_events_auth_ts ON request_events (auth_index, ts);",
	"CREATE INDEX IF NOT EXISTS idx_events_key_ts ON request_events (key_id, ts);",
	"CREATE INDEX IF NOT EXISTS idx_events_ts_id ON request_events (ts DESC, id DESC);",
	"CREATE INDEX IF NOT EXISTS idx_outbox_id ON usage_outbox (id);",
	"CREATE INDEX IF NOT EXISTS idx_daily_date ON usage_daily (date);",
	"CREATE INDEX IF NOT EXISTS idx_daily_date_key ON usage_daily (date, key_id);",
	"CREATE INDEX IF NOT EXISTS idx_daily_key_id ON usage_daily (key_id);",
	"CREATE INDEX IF NOT EXISTS idx_error_events_ts ON error_events (ts);",
	"CREATE INDEX IF NOT EXISTS idx_error_events_auth ON error_events (auth_index);",
	"CREATE INDEX IF NOT EXISTS idx_error_events_ts_id ON error_events (ts DESC, id DESC);",
	"CREATE INDEX IF NOT EXISTS idx_quota_snapshots_auth_ts ON quota_snapshots (auth_index, ts);",
	"CREATE INDEX IF NOT EXISTS idx_upstream_daily_date ON upstream_daily_stats (date);",
	"CREATE INDEX IF NOT EXISTS idx_upstream_daily_auth ON upstream_daily_stats (auth_index);",
	"CREATE INDEX IF NOT EXISTS idx_upstream_validations_auth ON upstream_validations(auth_index, status);",
	"CREATE INDEX IF NOT EXISTS idx_upstream_validations_status ON upstream_validations(status);",
}

// alterStatements provides defensive column additions for older gate databases.
var alterStatements = []string{
	"ALTER TABLE upstream_auth_baselines ADD COLUMN base_prompt_tokens INTEGER NOT NULL DEFAULT 0;",
	"ALTER TABLE upstream_auth_baselines ADD COLUMN base_completion_tokens INTEGER NOT NULL DEFAULT 0;",
	"ALTER TABLE quota_rules ADD COLUMN model_whitelist TEXT DEFAULT '';",
	"ALTER TABLE quota_rules ADD COLUMN model_group TEXT DEFAULT '';",
	"ALTER TABLE quota_rules ADD COLUMN rpm_limit INTEGER;",
	"ALTER TABLE usage_daily ADD COLUMN cached_tokens INTEGER NOT NULL DEFAULT 0;",
	"ALTER TABLE usage_daily ADD COLUMN success_count INTEGER NOT NULL DEFAULT 0;",
	"ALTER TABLE usage_daily ADD COLUMN failure_count INTEGER NOT NULL DEFAULT 0;",
	"ALTER TABLE request_events ADD COLUMN cached_tokens INTEGER NOT NULL DEFAULT 0;",
	"ALTER TABLE request_events ADD COLUMN ttft_ms INTEGER NOT NULL DEFAULT 0;",
	"ALTER TABLE request_events ADD COLUMN auth_index TEXT DEFAULT '';",
	"ALTER TABLE usage_outbox ADD COLUMN cached_tokens INTEGER NOT NULL DEFAULT 0;",
	"ALTER TABLE usage_outbox ADD COLUMN ttft_ms INTEGER NOT NULL DEFAULT 0;",
	"ALTER TABLE usage_outbox ADD COLUMN auth_index TEXT DEFAULT '';",
	"ALTER TABLE request_events ADD COLUMN reasoning_effort TEXT DEFAULT '';",
	"ALTER TABLE request_events ADD COLUMN tokens_per_sec REAL NOT NULL DEFAULT 0;",
	"ALTER TABLE usage_outbox ADD COLUMN reasoning_effort TEXT DEFAULT '';",
	"ALTER TABLE usage_outbox ADD COLUMN tokens_per_sec REAL NOT NULL DEFAULT 0;",
	"ALTER TABLE provider_registry ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 100;",
}

// dropStatements drops legacy redundant tables.
var dropStatements = []string{
	"DROP TABLE IF EXISTS custom_provider_keys;",
	"DROP TABLE IF EXISTS custom_providers;",
}

// Migrate executes all DDL, index, alter, and cleanup migrations on the SQLite database.
func (d *DB) Migrate(ctx context.Context) error {
	// 1. Create base tables
	if _, err := d.ExecContext(ctx, ddlStatements); err != nil {
		return fmt.Errorf("execute base DDL failed: %w", err)
	}

	// 2. Execute alter migrations defensively
	for _, sql := range alterStatements {
		_, _ = d.ExecContext(ctx, sql) // Ignore error if column already exists
	}

	// 3. Drop legacy tables
	for _, sql := range dropStatements {
		_, _ = d.ExecContext(ctx, sql)
	}

	// 4. Create indexes
	for _, sql := range indexStatements {
		if _, err := d.ExecContext(ctx, sql); err != nil {
			return fmt.Errorf("create index failed [%s]: %w", sql, err)
		}
	}

	return nil
}
