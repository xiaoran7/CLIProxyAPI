package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps standard database/sql DB instance with Console helper methods.
type DB struct {
	*sql.DB
	path string
	mu   sync.Mutex
}

// Open creates or connects to a pure-Go modernc SQLite database file.
// It initializes WAL journal mode, busy timeouts, and in-memory temporary tables.
func Open(dbPath string) (*DB, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("console db: path cannot be empty")
	}

	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("console db: failed to create directory %s: %w", dir, err)
		}
	}

	// DSN with query parameters for connection tuning
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=temp_store(MEMORY)&_pragma=cache_size(-32000)", dbPath)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("console db: failed to open sqlite at %s: %w", dbPath, err)
	}

	// Single-writer model guarantees thread-safe serialization without lock contention
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	// Ping database with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("console db: failed to ping sqlite database: %w", err)
	}

	// Explicitly apply PRAGMAs to guarantee WAL and concurrency settings
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA cache_size = -32000;",
		"PRAGMA temp_store = MEMORY;",
	}
	for _, pragma := range pragmas {
		if _, err := sqlDB.ExecContext(ctx, pragma); err != nil {
			// Non-fatal, log or continue
			continue
		}
	}

	db := &DB{
		DB:   sqlDB,
		path: dbPath,
	}

	// Run table migrations
	if err := db.Migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("console db: migration failed: %w", err)
	}

	return db, nil
}

// Path returns the SQLite file path.
func (d *DB) Path() string {
	return d.path
}

// Close closes the underlying SQLite database connection.
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.DB != nil {
		return d.DB.Close()
	}
	return nil
}
