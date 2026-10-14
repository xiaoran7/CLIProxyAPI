package helps

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/tidwall/gjson"
)

func TestStripWorkBuddyForbiddenHeaders(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://www.workbuddy.ai/v2/chat/completions", nil)
	req.Header.Set("x-anthropic-billing-header", "secret-billing")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-client", "codex")
	req.Header.Set("x-app-name", "zcode")
	req.Header.Set("Authorization", "Bearer valid")

	StripWorkBuddyForbiddenHeaders(req)

	if req.Header.Get("x-anthropic-billing-header") != "" {
		t.Error("x-anthropic-billing-header was not stripped")
	}
	if req.Header.Get("anthropic-version") != "" {
		t.Error("anthropic-version was not stripped")
	}
	if req.Header.Get("x-client") != "" {
		t.Error("x-client was not stripped")
	}
	if req.Header.Get("x-app-name") != "" {
		t.Error("x-app-name was not stripped")
	}
	if req.Header.Get("Authorization") != "Bearer valid" {
		t.Error("Authorization header was unexpectedly stripped")
	}
}

func TestEnsureWorkBuddySystemPrompt(t *testing.T) {
	// Case 1: First message is user turn -> should prepend system message
	userFirst := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	ensured := EnsureWorkBuddySystemPrompt(userFirst)
	role0 := gjson.GetBytes(ensured, "messages.0.role").String()
	content0 := gjson.GetBytes(ensured, "messages.0.content").String()
	role1 := gjson.GetBytes(ensured, "messages.1.role").String()
	if role0 != "system" || content0 != "You are an expert coding assistant." || role1 != "user" {
		t.Errorf("Unexpected result for userFirst: %s", string(ensured))
	}

	// Case 2: First message is already system with non-empty content -> should keep unchanged
	sysFirst := []byte(`{"messages":[{"role":"system","content":"custom prompt"},{"role":"user","content":"hi"}]}`)
	ensuredSys := EnsureWorkBuddySystemPrompt(sysFirst)
	if gjson.GetBytes(ensuredSys, "messages.0.content").String() != "custom prompt" {
		t.Errorf("Custom system prompt was overwritten: %s", string(ensuredSys))
	}

	// Case 3: First message is system but empty content -> should populate default
	emptySys := []byte(`{"messages":[{"role":"system","content":""},{"role":"user","content":"hi"}]}`)
	ensuredEmpty := EnsureWorkBuddySystemPrompt(emptySys)
	if gjson.GetBytes(ensuredEmpty, "messages.0.content").String() != "You are an expert coding assistant." {
		t.Errorf("Empty system prompt was not populated: %s", string(ensuredEmpty))
	}

	// Case 4: Empty messages array
	emptyArr := []byte(`{"messages":[]}`)
	ensuredArr := EnsureWorkBuddySystemPrompt(emptyArr)
	if gjson.GetBytes(ensuredArr, "messages.0.role").String() != "system" {
		t.Errorf("Empty messages array was not initialized: %s", string(ensuredArr))
	}
}

func TestSanitizeWorkBuddyPromptWAF(t *testing.T) {
	input := []byte(`{"messages":[{"role":"system","content":"You are Claude Code, Anthropic's official CLI for Claude. Anthropic builds AI."}]}`)
	sanitized := SanitizeWorkBuddyPromptWAF(input)

	if bytes.Contains(sanitized, []byte("You are Claude Code, Anthropic's official CLI for Claude")) {
		t.Error("Sensitive Claude Code prompt substring was not sanitized")
	}
	if !bytes.Contains(sanitized, []byte("You are an expert coding assistant")) {
		t.Error("Replacement text was not found in sanitized output")
	}
}

func TestBackfillWorkBuddyReasoningContent(t *testing.T) {
	// Case 1: Assistant with tool_calls and missing reasoning_content
	input := []byte(`{
		"messages": [
			{"role": "user", "content": "read file"},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "read"}}]
			},
			{"role": "tool", "content": "file contents"}
		]
	}`)

	backfilled, err := BackfillWorkBuddyReasoningContent(input)
	if err != nil {
		t.Fatalf("BackfillWorkBuddyReasoningContent failed: %v", err)
	}

	rc := gjson.GetBytes(backfilled, "messages.1.reasoning_content")
	if !rc.Exists() || rc.String() != workbuddyFallbackReasoning {
		t.Errorf("messages.1.reasoning_content = %v, want %q", rc.Value(), workbuddyFallbackReasoning)
	}

	// Case 2: Assistant with existing reasoning_content should not be overwritten
	inputWithRC := []byte(`{
		"messages": [
			{
				"role": "assistant",
				"content": null,
				"reasoning_content": "I should read the file first",
				"tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "read"}}]
			}
		]
	}`)

	backfilledWithRC, err := BackfillWorkBuddyReasoningContent(inputWithRC)
	if err != nil {
		t.Fatalf("BackfillWorkBuddyReasoningContent failed: %v", err)
	}

	rcWith := gjson.GetBytes(backfilledWithRC, "messages.0.reasoning_content").String()
	if rcWith != "I should read the file first" {
		t.Errorf("Existing reasoning_content was overwritten: got %q", rcWith)
	}
}
