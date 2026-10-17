package config

import (
	"testing"
)

func TestModelVariantModeNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected ModelVariantMode
	}{
		{"", ModelVariantModeCompact},
		{"compact", ModelVariantModeCompact},
		{"prefixed", ModelVariantModeCompact},
		{"clean", ModelVariantModeCompact},
		{"short", ModelVariantModeCompact},
		{"true", ModelVariantModeCompact},
		{"raw", ModelVariantModeRaw},
		{"unprefixed", ModelVariantModeRaw},
		{"bare", ModelVariantModeRaw},
		{"all", ModelVariantModeAll},
		{"expanded", ModelVariantModeAll},
		{"full", ModelVariantModeAll},
		{"false", ModelVariantModeAll},
		{"  COMPACT  ", ModelVariantModeCompact},
		{"RAW", ModelVariantModeRaw},
		{"ALL", ModelVariantModeAll},
		{"unknown-xyz", ModelVariantModeCompact},
	}

	for _, tt := range tests {
		got := NormalizeModelVariantMode(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeModelVariantMode(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestModelVariantModeYAMLUnmarshal(t *testing.T) {
	t.Run("v8_string_compact", func(t *testing.T) {
		yamlData := []byte(`
config-version: 8
models:
  variant-mode: compact
`)
		cfg, err := ParseConfigBytes(yamlData)
		if err != nil {
			t.Fatalf("ParseConfigBytes failed: %v", err)
		}
		if cfg.ModelVariantMode != ModelVariantModeCompact {
			t.Errorf("ModelVariantMode = %q, want %q", cfg.ModelVariantMode, ModelVariantModeCompact)
		}
	})

	t.Run("v8_string_raw", func(t *testing.T) {
		yamlData := []byte(`
config-version: 8
models:
  variant-mode: raw
`)
		cfg, err := ParseConfigBytes(yamlData)
		if err != nil {
			t.Fatalf("ParseConfigBytes failed: %v", err)
		}
		if cfg.ModelVariantMode != ModelVariantModeRaw {
			t.Errorf("ModelVariantMode = %q, want %q", cfg.ModelVariantMode, ModelVariantModeRaw)
		}
	})

	t.Run("v8_string_all", func(t *testing.T) {
		yamlData := []byte(`
config-version: 8
models:
  variant-mode: all
`)
		cfg, err := ParseConfigBytes(yamlData)
		if err != nil {
			t.Fatalf("ParseConfigBytes failed: %v", err)
		}
		if cfg.ModelVariantMode != ModelVariantModeAll {
			t.Errorf("ModelVariantMode = %q, want %q", cfg.ModelVariantMode, ModelVariantModeAll)
		}
	})

	t.Run("v8_boolean_filter_true", func(t *testing.T) {
		yamlData := []byte(`
config-version: 8
models:
  filter-variants: true
`)
		cfg, err := ParseConfigBytes(yamlData)
		if err != nil {
			t.Fatalf("ParseConfigBytes failed: %v", err)
		}
		if cfg.ModelVariantMode != ModelVariantModeCompact {
			t.Errorf("ModelVariantMode = %q, want %q", cfg.ModelVariantMode, ModelVariantModeCompact)
		}
	})

	t.Run("v8_boolean_filter_false", func(t *testing.T) {
		yamlData := []byte(`
config-version: 8
models:
  filter-variants: false
`)
		cfg, err := ParseConfigBytes(yamlData)
		if err != nil {
			t.Fatalf("ParseConfigBytes failed: %v", err)
		}
		if cfg.ModelVariantMode != ModelVariantModeAll {
			t.Errorf("ModelVariantMode = %q, want %q", cfg.ModelVariantMode, ModelVariantModeAll)
		}
	})

	t.Run("flat_legacy_model_variant_mode", func(t *testing.T) {
		yamlData := []byte(`
model-variant-mode: raw
`)
		cfg, err := ParseConfigBytes(yamlData)
		if err != nil {
			t.Fatalf("ParseConfigBytes failed: %v", err)
		}
		if cfg.ModelVariantMode != ModelVariantModeRaw {
			t.Errorf("ModelVariantMode = %q, want %q", cfg.ModelVariantMode, ModelVariantModeRaw)
		}
	})
}
