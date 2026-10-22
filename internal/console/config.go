package console

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds runtime configuration options for the Console subsystem.
type Config struct {
	Enabled       bool          `json:"enabled"`
	DBPath        string        `json:"db_path"`
	AdminSecret   string        `json:"admin_secret"`
	CpaHost       string        `json:"cpa_host"`
	CpaPort       int           `json:"cpa_port"`
	CpaSecret     string        `json:"cpa_secret"`
	FlushInterval time.Duration `json:"flush_interval"`
}

// SubsystemConfig is an alias for Config.
type SubsystemConfig = Config

// DefaultConfig returns default configuration populated from environment and file heuristics.
func DefaultConfig() Config {
	_ = godotenv.Load()
	if cwd, err := os.Getwd(); err == nil {
		_ = godotenv.Load(filepath.Join(cwd, ".env"))
	}

	enabled := true
	if val := os.Getenv("CONSOLE_ENABLED"); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			enabled = b
		}
	}
	if val := os.Getenv("DISABLE_CONSOLE"); val != "" {
		if b, err := strconv.ParseBool(val); err == nil && b {
			enabled = false
		}
	}

	dbPath := resolveDefaultDBPath()

	adminSecret := os.Getenv("CONSOLE_ADMIN_SECRET")
	if adminSecret == "" {
		adminSecret = os.Getenv("ADMIN_SECRET")
	}
	if adminSecret == "" {
		adminSecret = os.Getenv("GATE_ADMIN_SECRET")
	}

	cpaHost := os.Getenv("CPA_MGMT_HOST")
	if cpaHost == "" {
		cpaHost = "127.0.0.1"
	}

	cpaPort := 8000
	if pStr := os.Getenv("CPA_MGMT_PORT"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			cpaPort = p
		}
	}

	cpaSecret := os.Getenv("CPA_MGMT_SECRET")
	if cpaSecret == "" {
		cpaSecret = os.Getenv("MANAGEMENT_PASSWORD")
	}

	flushInterval := 100 * time.Millisecond
	if iv := os.Getenv("CONSOLE_FLUSH_INTERVAL"); iv != "" {
		if d, err := time.ParseDuration(iv); err == nil && d > 0 {
			flushInterval = d
		}
	}

	return Config{
		Enabled:       enabled,
		DBPath:        dbPath,
		AdminSecret:   adminSecret,
		CpaHost:       cpaHost,
		CpaPort:       cpaPort,
		CpaSecret:     cpaSecret,
		FlushInterval: flushInterval,
	}
}

// LoadConfig loads configuration from environment with defaults.
func LoadConfig() Config {
	return DefaultConfig()
}

// resolveDefaultDBPath locates the default SQLite database path.
func resolveDefaultDBPath() string {
	if p := os.Getenv("CONSOLE_DB_PATH"); p != "" {
		return p
	}
	if p := os.Getenv("GATE_DB_PATH"); p != "" {
		return p
	}
	if p := os.Getenv("GATE_DB"); p != "" {
		return p
	}
	if p := os.Getenv("CPA_GUARD_DB"); p != "" {
		return p
	}

	guardDir := os.Getenv("GUARD_DIR")
	if guardDir == "" {
		guardDir = os.Getenv("GATE_DIR")
	}
	if guardDir != "" {
		return filepath.Join(guardDir, "gate.db")
	}

	// Priority checks on standard file locations
	candidates := []string{
		"gate.db",
		filepath.Join("data", "gate.db"),
		filepath.Join("data", "quota.db"),
		"quota.db",
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}

	// Default fallback to gate.db in cwd
	return "gate.db"
}
