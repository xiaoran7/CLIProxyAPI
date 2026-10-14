package config

import (
	"math"
	"testing"
	"time"
)

// TestClaudeConfigTLSHandshakeTimeout pins the Issue #6615 contract for the
// opt-in upstream TLS handshake bound: a positive value in seconds enables
// the bound, while zero or negative values disable it.
func TestClaudeConfigTLSHandshakeTimeout(t *testing.T) {
	tests := []struct {
		name string
		cfg  *ClaudeConfig
		want time.Duration
	}{
		{name: "nil receiver disables", cfg: nil, want: 0},
		{name: "zero disables", cfg: &ClaudeConfig{TLSHandshakeTimeoutSeconds: 0}, want: 0},
		{name: "negative disables", cfg: &ClaudeConfig{TLSHandshakeTimeoutSeconds: -5}, want: 0},
		{name: "positive seconds convert", cfg: &ClaudeConfig{TLSHandshakeTimeoutSeconds: 10}, want: 10 * time.Second},
		{name: "overflowing seconds saturate", cfg: &ClaudeConfig{TLSHandshakeTimeoutSeconds: math.MaxInt}, want: time.Duration(math.MaxInt64)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.TLSHandshakeTimeout(); got != tt.want {
				t.Fatalf("TLSHandshakeTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseConfigBytesClaudeTLSHandshakeTimeout ensures the canonical v8
// layout exposes the setting under upstream.claude and parses as integer
// seconds.
func TestParseConfigBytesClaudeTLSHandshakeTimeout(t *testing.T) {
	cfg, errParse := ParseConfigBytes([]byte(`
upstream:
  claude:
    tls-handshake-timeout-seconds: 12
`))
	if errParse != nil {
		t.Fatalf("ParseConfigBytes() error = %v", errParse)
	}
	if cfg.Claude.TLSHandshakeTimeoutSeconds != 12 {
		t.Fatalf("tls-handshake-timeout-seconds = %d, want 12", cfg.Claude.TLSHandshakeTimeoutSeconds)
	}
	if got := cfg.Claude.TLSHandshakeTimeout(); got != 12*time.Second {
		t.Fatalf("TLSHandshakeTimeout() = %v, want 12s", got)
	}
}

// TestParseConfigBytesClaudeTLSHandshakeTimeoutLegacyLayout ensures the
// pre-v8 top-level claude layout still maps onto the same field.
func TestParseConfigBytesClaudeTLSHandshakeTimeoutLegacyLayout(t *testing.T) {
	cfg, errParse := ParseConfigBytes([]byte(`
claude:
  tls-handshake-timeout-seconds: 7
`))
	if errParse != nil {
		t.Fatalf("ParseConfigBytes() error = %v", errParse)
	}
	if cfg.Claude.TLSHandshakeTimeoutSeconds != 7 {
		t.Fatalf("tls-handshake-timeout-seconds = %d, want 7", cfg.Claude.TLSHandshakeTimeoutSeconds)
	}
	if got := cfg.Claude.TLSHandshakeTimeout(); got != 7*time.Second {
		t.Fatalf("TLSHandshakeTimeout() = %v, want 7s", got)
	}
}
