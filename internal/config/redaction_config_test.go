package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRedactionConfig_Unmarshal(t *testing.T) {
	yamlData := []byte(`
routing:
  strategy: "smart"
  session-affinity: true
redaction:
  enabled: true
  mask-secrets: true
  mask-personal: true
`)

	var cfg Config
	if err := yaml.Unmarshal(yamlData, &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if cfg.Routing.Strategy != "smart" {
		t.Errorf("routing.strategy = %q, want 'smart'", cfg.Routing.Strategy)
	}
	if !cfg.Routing.SessionAffinity {
		t.Errorf("routing.session-affinity = false, want true")
	}

	if !cfg.Redaction.Enabled {
		t.Errorf("redaction.enabled = false, want true")
	}
	if !cfg.Redaction.SecretsEnabled() {
		t.Errorf("redaction.mask-secrets = false, want true")
	}
	if !cfg.Redaction.MaskPersonal {
		t.Errorf("redaction.mask-personal = false, want true")
	}
}

func TestRedactionConfig_DefaultSecretsEnabled(t *testing.T) {
	rc := RedactionConfig{
		Enabled: true,
	}
	if !rc.SecretsEnabled() {
		t.Errorf("SecretsEnabled() on unset MaskSecrets = false, want true")
	}

	falseVal := false
	rc.MaskSecrets = &falseVal
	if rc.SecretsEnabled() {
		t.Errorf("SecretsEnabled() on false MaskSecrets = true, want false")
	}
}
