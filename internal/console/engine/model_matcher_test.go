package engine

import (
	"reflect"
	"testing"
)

func TestParseAndNormalizeModelWhitelist(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"*", nil},
		{`["*"]`, nil},
		{"gpt-4o, claude-3-7-sonnet", []string{"gpt-4o", "claude-3-7-sonnet"}},
		{`["gpt-4o", "claude-3-7-sonnet"]`, []string{"gpt-4o", "claude-3-7-sonnet"}},
		{"gpt-4o   gemini-2.5-flash\nclaude-3-7-sonnet", []string{"gpt-4o", "gemini-2.5-flash", "claude-3-7-sonnet"}},
	}

	for _, tc := range tests {
		actual := ParseModelWhitelist(tc.input)
		if !reflect.DeepEqual(actual, tc.expected) {
			t.Errorf("ParseModelWhitelist(%q) = %v, expected %v", tc.input, actual, tc.expected)
		}
	}
}

func TestIsModelPermitted(t *testing.T) {
	rules := []string{
		"claude-3-7-sonnet",
		"gpt-4o*",
		"models/gemini-2.5-flash",
		"deepseek-reasoner",
	}

	positiveCases := []string{
		"claude-3-7-sonnet",
		"CLAUDE-3-7-SONNET",                // case-insensitive
		"anthropic/claude-3-7-sonnet",      // provider prefix
		"claude-3-7-sonnet:3.33x",          // dynamic multiplier
		"anthropic/claude-3-7-sonnet:1.5x", // prefix + multiplier
		"claude-3-7-sonnet-20250219",       // date suffix
		"claude-3-7-sonnet@20250219",       // version suffix
		"gpt-4o",                           // wildcard gpt-4o*
		"gpt-4o-mini",                      // wildcard
		"openai/gpt-4o-2024-11-20",         // prefix + wildcard + date
		"gemini-2.5-flash",                 // stripped pattern match
		"google/gemini-2.5-flash",          // stripped cross match
		"deepseek-reasoner",
	}

	for _, model := range positiveCases {
		if !IsModelPermitted(model, rules) {
			t.Errorf("expected model %q to be permitted by %v", model, rules)
		}
	}

	negativeCases := []string{
		"claude-3-5-haiku",
		"gpt-3.5-turbo",
		"gemini-1.5-pro",
		"llama-3.3-70b",
		"",
	}

	for _, model := range negativeCases {
		if IsModelPermitted(model, rules) {
			t.Errorf("expected model %q to NOT be permitted by %v", model, rules)
		}
	}
}

func TestResolveModelTarget(t *testing.T) {
	tests := []struct {
		name           string
		requestedModel string
		allowedRules   []string
		wantAllowed    bool
		wantTarget     string
	}{
		{
			name:           "workbuddy specific namespaced model rewrites bare request",
			requestedModel: "claude-opus-5-5",
			allowedRules:   []string{"workbuddy/claude-opus-5-5"},
			wantAllowed:    true,
			wantTarget:     "workbuddy/claude-opus-5-5",
		},
		{
			name:           "workbuddy wildcard namespace rewrites bare request",
			requestedModel: "claude-opus-5-5",
			allowedRules:   []string{"workbuddy/*"},
			wantAllowed:    true,
			wantTarget:     "workbuddy/claude-opus-5-5",
		},
		{
			name:           "workbuddy-cn specific model rewrites bare request",
			requestedModel: "deepseek-v4.1-flash",
			allowedRules:   []string{"workbuddy-cn/deepseek-v4.1-flash"},
			wantAllowed:    true,
			wantTarget:     "workbuddy-cn/deepseek-v4.1-flash",
		},
		{
			name:           "bare model allowed does not rewrite",
			requestedModel: "claude-opus-5-5",
			allowedRules:   []string{"claude-opus-5-5"},
			wantAllowed:    true,
			wantTarget:     "claude-opus-5-5",
		},
		{
			name:           "universal wildcard does not rewrite",
			requestedModel: "claude-opus-5-5",
			allowedRules:   []string{"*"},
			wantAllowed:    true,
			wantTarget:     "claude-opus-5-5",
		},
		{
			name:           "both bare and namespaced allowed does not rewrite",
			requestedModel: "claude-opus-5-5",
			allowedRules:   []string{"claude-opus-5-5", "workbuddy/claude-opus-5-5"},
			wantAllowed:    true,
			wantTarget:     "claude-opus-5-5",
		},
		{
			name:           "prefixed request preserved",
			requestedModel: "workbuddy/claude-opus-5-5",
			allowedRules:   []string{"workbuddy/claude-opus-5-5"},
			wantAllowed:    true,
			wantTarget:     "workbuddy/claude-opus-5-5",
		},
		{
			name:           "disallowed model rejected",
			requestedModel: "gpt-4o",
			allowedRules:   []string{"workbuddy/claude-opus-5-5"},
			wantAllowed:    false,
			wantTarget:     "gpt-4o",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			allowed, target, _ := ResolveModelTarget(tc.requestedModel, tc.allowedRules)
			if allowed != tc.wantAllowed {
				t.Errorf("ResolveModelTarget(%q, %v) allowed = %v, want %v", tc.requestedModel, tc.allowedRules, allowed, tc.wantAllowed)
			}
			if target != tc.wantTarget {
				t.Errorf("ResolveModelTarget(%q, %v) target = %q, want %q", tc.requestedModel, tc.allowedRules, target, tc.wantTarget)
			}
		})
	}
}
