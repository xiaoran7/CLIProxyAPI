package redact

import (
	"strings"
	"testing"
)

func TestMask_Secrets(t *testing.T) {
	opts := Options{
		Enabled: true,
		Secrets: true,
	}
	redactor := NewSessionRedactor(opts)

	input := `My OpenAI key is sk-ant-api03-abcdef1234567890abcdef1234567890 and DB is postgres://admin:supersecretpassword123@localhost:5432/mydb`
	masked, count := redactor.Mask(input)

	if count < 2 {
		t.Fatalf("expected at least 2 secrets masked, got %d. Masked: %s", count, masked)
	}
	if strings.Contains(masked, "sk-ant-api03") {
		t.Fatalf("API key leaked in masked output: %s", masked)
	}
	if strings.Contains(masked, "supersecretpassword123") {
		t.Fatalf("Password leaked in masked output: %s", masked)
	}
	if !strings.Contains(masked, "{{API_KEY_") {
		t.Fatalf("expected {{API_KEY_ placeholder in output: %s", masked)
	}
	if !strings.Contains(masked, "{{PASSWORD_") {
		t.Fatalf("expected {{PASSWORD_ placeholder in output: %s", masked)
	}

	// Restore
	restored := redactor.Restore(masked)
	if restored != input {
		t.Fatalf("restored output does not match original.\nGot:  %s\nWant: %s", restored, input)
	}
}

func TestMask_DeterministicHMACHits(t *testing.T) {
	opts := Options{
		Enabled: true,
		Secrets: true,
	}
	redactor := NewSessionRedactor(opts)

	secret := "sk-proj-9876543210abcdef1234567890"
	msg1 := "First turn with " + secret + " in context"
	msg2 := "Second turn asking about " + secret + " again"

	masked1, _ := redactor.Mask(msg1)
	masked2, _ := redactor.Mask(msg2)

	// Extract placeholder from masked1
	idx1 := strings.Index(masked1, "{{API_KEY_")
	end1 := strings.Index(masked1, "}}") + 2
	ph1 := masked1[idx1:end1]

	// Extract placeholder from masked2
	idx2 := strings.Index(masked2, "{{API_KEY_")
	end2 := strings.Index(masked2, "}}") + 2
	ph2 := masked2[idx2:end2]

	if ph1 != ph2 {
		t.Fatalf("deterministic placeholder failed: turn 1 got %q, turn 2 got %q", ph1, ph2)
	}
}

func TestMask_PersonalData(t *testing.T) {
	opts := Options{
		Enabled:  true,
		Secrets:  true,
		Personal: true,
	}
	redactor := NewSessionRedactor(opts)

	input := "Please call 13812345678 or email dev@tannerlab.xyz"
	masked, count := redactor.Mask(input)

	if count != 2 {
		t.Fatalf("expected 2 personal items masked, got %d: %s", count, masked)
	}
	if strings.Contains(masked, "13812345678") || strings.Contains(masked, "dev@tannerlab.xyz") {
		t.Fatalf("personal data leaked in masked output: %s", masked)
	}

	restored := redactor.Restore(masked)
	if restored != input {
		t.Fatalf("restore mismatch. Got: %s, Want: %s", restored, input)
	}
}

func TestRestore_FuzzyWhitespaceRewrites(t *testing.T) {
	opts := Options{
		Enabled: true,
		Secrets: true,
	}
	redactor := NewSessionRedactor(opts)

	secret := "sk-ant-testkey1234567890123456"
	masked, _ := redactor.Mask("Key: " + secret)
	placeholder := strings.TrimPrefix(masked, "Key: ")
	inner := placeholder[2 : len(placeholder)-2]

	// Model returns variations:
	variations := []string{
		"Here is your key: " + placeholder,
		"Here is your key: {{ " + inner + " }}",
		"Here is your key: {" + inner + "}",
		"Here is your key: {{" + strings.ToLower(inner) + "}}",
	}

	for _, v := range variations {
		restored := redactor.Restore(v)
		if !strings.Contains(restored, secret) {
			t.Fatalf("failed to restore variation %q, got: %s", v, restored)
		}
	}
}

func TestStreamRestorer_ChunkSplitting(t *testing.T) {
	opts := Options{
		Enabled: true,
		Secrets: true,
	}
	redactor := NewSessionRedactor(opts)

	secret := "sk-ant-streamingkey1234567890"
	masked, _ := redactor.Mask("Use key " + secret + " for auth")
	placeholder := strings.TrimSuffix(strings.TrimPrefix(masked, "Use key "), " for auth")

	streamer := NewStreamRestorer(redactor)

	// Simulate streaming: split placeholder across 2 chunks
	half := len(placeholder) / 2
	part1 := "Your key is: " + placeholder[:half]
	part2 := placeholder[half:] + ".\nDone!"

	out1 := streamer.RestoreChunk([]byte(part1))
	out2 := streamer.RestoreChunk([]byte(part2))
	tail := streamer.Finish()

	full := string(out1) + string(out2) + string(tail)

	if !strings.Contains(full, secret) {
		t.Fatalf("stream restorer failed to assemble and restore split placeholder.\nGot: %s\nExpected secret: %s", full, secret)
	}
	if strings.Contains(full, placeholder) {
		t.Fatalf("unrestored placeholder leaked in stream output: %s", full)
	}
}
