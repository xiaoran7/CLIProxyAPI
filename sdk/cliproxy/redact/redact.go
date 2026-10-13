// Package redact provides zero-knowledge local sensitive data masking and restoration.
// It intercepts prompts before leaving the local machine, replacing sensitive secrets
// (API keys, private keys, passwords, connection strings, tokens) and personal data
// with deterministic HMAC placeholders (e.g. {{API_KEY_k3v9x2mq}}), and transparently
// restores them in upstream responses before delivery to the client.
package redact

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"regexp"
	"strings"
	"sync"
)

var defaultSalt = []byte("cliproxyapi-redact-salt-2026")

// Options configures which categories of sensitive data to mask.
type Options struct {
	Enabled  bool `json:"enabled" yaml:"enabled"`
	Secrets  bool `json:"secrets" yaml:"secrets"`
	Personal bool `json:"personal" yaml:"personal"`
}

type rule struct {
	kind       string
	re         *regexp.Regexp
	isPersonal bool
	groupIndex int // 0 for full match, >0 for specific capture group
}

var redactionRules = []rule{
	// 1. Private keys
	{
		kind:       "PRIVATE_KEY",
		re:         regexp.MustCompile(`-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----[\s\S]+?-----END (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----`),
		isPersonal: false,
	},
	// 2. Standard API keys
	{
		kind:       "API_KEY",
		re:         regexp.MustCompile(`sk-(?:ant-|proj-|or-|svcacct-|admin-)?[A-Za-z0-9_-]{20,}`),
		isPersonal: false,
	},
	{
		kind:       "API_KEY",
		re:         regexp.MustCompile(`(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,}|glpat-[A-Za-z0-9_-]{20,})`),
		isPersonal: false,
	},
	{
		kind:       "API_KEY",
		re:         regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`),
		isPersonal: false,
	},
	{
		kind:       "API_KEY",
		re:         regexp.MustCompile(`xox[abposr]-[0-9A-Za-z-]{10,}`),
		isPersonal: false,
	},
	{
		kind:       "API_KEY",
		re:         regexp.MustCompile(`(?:sk|rk)_live_[0-9A-Za-z]{20,}`),
		isPersonal: false,
	},
	{
		kind:       "API_KEY",
		re:         regexp.MustCompile(`(?:AKIA|ASIA)[0-9A-Z]{16}`),
		isPersonal: false,
	},
	{
		kind:       "API_KEY",
		re:         regexp.MustCompile(`(?:hf_[A-Za-z0-9]{30,}|gsk_[A-Za-z0-9]{40,}|xai-[A-Za-z0-9]{40,}|npm_[A-Za-z0-9]{36}|pypi-[A-Za-z0-9_-]{50,})`),
		isPersonal: false,
	},
	// 3. JWT and bearer tokens
	{
		kind:       "TOKEN",
		re:         regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),
		isPersonal: false,
	},
	// 4. Passwords in connection URLs (e.g. postgres://user:password@host:port)
	{
		kind:       "PASSWORD",
		re:         regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s:@/?#"'<>]+:([^\s@/?#"'<>]{4,})@`),
		isPersonal: false,
		groupIndex: 1,
	},
	// 5. Sensitive config assignments: KEY = "...", PASSWORD: "..."
	{
		kind:       "SECRET",
		re:         regexp.MustCompile(`(?i)(?:api_key|apikey|secret_key|secretkey|access_token|db_password|password|passwd|private_key)\s*[:=]\s*["']?([A-Za-z0-9_.-]{8,})["']?`),
		isPersonal: false,
		groupIndex: 1,
	},
	// 6. Personal data (when enabled)
	{
		kind:       "EMAIL",
		re:         regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`),
		isPersonal: true,
	},
	{
		kind:       "PHONE",
		re:         regexp.MustCompile(`(?:\+?86[- ]?)?1[3-9]\d{9}`),
		isPersonal: true,
	},
	{
		kind:       "ID_CARD",
		re:         regexp.MustCompile(`[1-9]\d{5}(?:18|19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]`),
		isPersonal: true,
	},
}

// SessionRedactor manages bidirectional secret <-> placeholder mapping for a request/session.
type SessionRedactor struct {
	mu                  sync.RWMutex
	salt                []byte
	secretToPlaceholder map[string]string
	placeholderToSecret map[string]string
	opts                Options
}

// NewSessionRedactor initializes a new redactor instance with optional custom salt.
func NewSessionRedactor(opts Options, salt ...[]byte) *SessionRedactor {
	s := defaultSalt
	if len(salt) > 0 && len(salt[0]) > 0 {
		s = salt[0]
	}
	return &SessionRedactor{
		salt:                s,
		secretToPlaceholder: make(map[string]string),
		placeholderToSecret: make(map[string]string),
		opts:                opts,
	}
}

// Mask scans text, replaces identified secrets with deterministic placeholders, and registers bindings.
func (r *SessionRedactor) Mask(text string) (string, int) {
	if !r.opts.Enabled || text == "" {
		return text, 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	result := text
	count := 0

	for _, rl := range redactionRules {
		if rl.isPersonal && !r.opts.Personal {
			continue
		}
		if !rl.isPersonal && !r.opts.Secrets {
			continue
		}

		if rl.groupIndex > 0 {
			matches := rl.re.FindAllStringSubmatch(result, -1)
			for _, m := range matches {
				if len(m) > rl.groupIndex {
					secret := m[rl.groupIndex]
					if len(secret) < 3 {
						continue
					}
					placeholder := r.getOrCreatePlaceholderLocked(rl.kind, secret)
					if strings.Contains(result, secret) {
						result = strings.ReplaceAll(result, secret, placeholder)
						count++
					}
				}
			}
		} else {
			matches := rl.re.FindAllString(result, -1)
			for _, secret := range matches {
				if len(secret) < 3 {
					continue
				}
				placeholder := r.getOrCreatePlaceholderLocked(rl.kind, secret)
				if strings.Contains(result, secret) {
					result = strings.ReplaceAll(result, secret, placeholder)
					count++
				}
			}
		}
	}

	return result, count
}

// Restore substitutes placeholders back to their original secret values.
// It flexibly matches standard placeholders as well as LLM whitespace rewrites (e.g. {{ KEY }} or {KEY}).
func (r *SessionRedactor) Restore(text string) string {
	if text == "" {
		return text
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.placeholderToSecret) == 0 {
		return text
	}

	result := text
	for placeholder, secret := range r.placeholderToSecret {
		// Exact placeholder match: {{API_KEY_k3v9x2mq}}
		if strings.Contains(result, placeholder) {
			result = strings.ReplaceAll(result, placeholder, secret)
		}

		// Handle inner token variations: "API_KEY_k3v9x2mq"
		if len(placeholder) > 4 && strings.HasPrefix(placeholder, "{{") && strings.HasSuffix(placeholder, "}}") {
			inner := placeholder[2 : len(placeholder)-2]

			// Case with spaces: {{ API_KEY_... }}
			spaced := "{{ " + inner + " }}"
			if strings.Contains(result, spaced) {
				result = strings.ReplaceAll(result, spaced, secret)
			}
			spacedLeft := "{{" + inner + " }}"
			if strings.Contains(result, spacedLeft) {
				result = strings.ReplaceAll(result, spacedLeft, secret)
			}
			spacedRight := "{{ " + inner + "}}"
			if strings.Contains(result, spacedRight) {
				result = strings.ReplaceAll(result, spacedRight, secret)
			}

			// Single braces: {API_KEY_...}
			singleBrace := "{" + inner + "}"
			if strings.Contains(result, singleBrace) {
				result = strings.ReplaceAll(result, singleBrace, secret)
			}

			// Lowercase variant if model lowered it
			innerLower := strings.ToLower(inner)
			if innerLower != inner {
				placeholderLower := "{{" + innerLower + "}}"
				if strings.Contains(result, placeholderLower) {
					result = strings.ReplaceAll(result, placeholderLower, secret)
				}
				spacedLower := "{{ " + innerLower + " }}"
				if strings.Contains(result, spacedLower) {
					result = strings.ReplaceAll(result, spacedLower, secret)
				}
			}
		}
	}

	return result
}

// HasBindings returns true if this redactor has any active placeholder bindings.
func (r *SessionRedactor) HasBindings() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.placeholderToSecret) > 0
}

// MatchPrefix checks if prefix matches the start of any active placeholder.
func (r *SessionRedactor) MatchPrefix(prefix string) (placeholder, secret string, ok bool) {
	if len(prefix) < 2 {
		return "", "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for p, s := range r.placeholderToSecret {
		if strings.HasPrefix(p, prefix) {
			return p, s, true
		}
	}
	return "", "", false
}

// ActiveBindings returns a snapshot copy of all active placeholder -> secret mappings.
func (r *SessionRedactor) ActiveBindings() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cp := make(map[string]string, len(r.placeholderToSecret))
	for k, v := range r.placeholderToSecret {
		cp[k] = v
	}
	return cp
}

func (r *SessionRedactor) getOrCreatePlaceholderLocked(kind, secret string) string {
	if p, ok := r.secretToPlaceholder[secret]; ok {
		return p
	}

	// Generate deterministic HMAC-SHA256 short hash (8 chars base32)
	mac := hmac.New(sha256.New, r.salt)
	mac.Write([]byte(secret))
	sum := mac.Sum(nil)
	hash := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:5]))

	placeholder := "{{" + kind + "_" + hash + "}}"
	r.secretToPlaceholder[secret] = placeholder
	r.placeholderToSecret[placeholder] = secret
	return placeholder
}
