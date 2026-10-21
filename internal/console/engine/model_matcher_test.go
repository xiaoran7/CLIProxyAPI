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
