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
	req.Header.Set("X-Machine-ID", "custom-machine")
	req.Header.Set("X-Session-ID", "custom-session")
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
	if req.Header.Get("X-Machine-ID") != "" {
		t.Error("X-Machine-ID was not stripped")
	}
	if req.Header.Get("X-Session-ID") != "" {
		t.Error("X-Session-ID was not stripped")
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

func TestSanitizeWorkBuddyPromptWAF_DeveloperRoleAndIdentity(t *testing.T) {
	// Case 1: developer role is converted to system, and DSH identity sentences are sanitized
	input := []byte(`{
		"model": "deepseek-v4.1-flash-free",
		"messages": [
			{
				"role": "developer",
				"content": "You are an AI agent powered by DeepSeek Harness.\nYou are a helpful collaborator."
			},
			{
				"role": "user",
				"content": "You are an AI agent powered by DeepSeek Harness. Please explain 11128."
			}
		]
	}`)

	sanitized := SanitizeWorkBuddyPromptWAF(input)

	// Model -free suffix should be stripped
	model := gjson.GetBytes(sanitized, "model").String()
	if model != "deepseek-v4.1-flash" {
		t.Errorf("expected model deepseek-v4.1-flash, got %s", model)
	}

	// Developer role should become system
	role0 := gjson.GetBytes(sanitized, "messages.0.role").String()
	if role0 != "system" {
		t.Errorf("expected role system, got %s", role0)
	}

	// System content should have DSH identity rewritten
	content0 := gjson.GetBytes(sanitized, "messages.0.content").String()
	if bytes.Contains([]byte(content0), []byte("powered by DeepSeek Harness")) {
		t.Errorf("DSH identity not sanitized in system prompt: %s", content0)
	}
	if !bytes.Contains([]byte(content0), []byte("You are a helpful AI assistant.\nYou are a helpful collaborator.")) {
		t.Errorf("unexpected system content after sanitization: %s", content0)
	}

	// User role content should preserve identity text except raw 11128 replacement
	role1 := gjson.GetBytes(sanitized, "messages.1.role").String()
	content1 := gjson.GetBytes(sanitized, "messages.1.content").String()
	if role1 != "user" {
		t.Errorf("expected role user, got %s", role1)
	}
	if !bytes.Contains([]byte(content1), []byte("You are an AI agent powered by DeepSeek Harness")) {
		t.Errorf("user message identity text should remain untouched, got: %s", content1)
	}
	if bytes.Contains([]byte(content1), []byte("11128")) {
		t.Errorf("raw 11128 in user prompt should be rewritten to 11-128: %s", content1)
	}
}

func TestSanitizeWorkBuddyPromptWAF_DSHIdentityRegexVariations(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "DSH standard harness",
			input:    "You are an AI agent powered by DeepSeek Harness.",
			expected: "You are a helpful AI assistant.",
		},
		{
			name:     "DSH with model version dot",
			input:    "You are a coding agent powered by the deepseek-v4.1-flash model.",
			expected: "You are a helpful AI assistant.",
		},
		{
			name:     "DSH assistant built on",
			input:    "You are an AI assistant built on DeepSeek.\nFollow instructions.",
			expected: "You are a helpful AI assistant.\nFollow instructions.",
		},
		{
			name:     "DSH lowercase without period at end",
			input:    "you are an agent powered by deepseek",
			expected: "You are a helpful AI assistant.",
		},
		{
			name:     "built by variant with proper name and comma",
			input:    "You are Example Agent, built by Example Corp. Be direct.",
			expected: "You are a helpful AI assistant. Be direct.",
		},
		{
			name:     "created by variant",
			input:    "You are a coding assistant created by Example Corp.",
			expected: "You are a helpful AI assistant.",
		},
		{
			name:     "developed by variant",
			input:    "You are an AI agent developed by Example Corp.",
			expected: "You are a helpful AI assistant.",
		},
		{
			name:     "made by variant",
			input:    "You are a chat model made by Example Corp.",
			expected: "You are a helpful AI assistant.",
		},
		{
			name:     "trained by variant",
			input:    "You are an assistant trained by Example Corp.",
			expected: "You are a helpful AI assistant.",
		},
		{
			name:     "bare powered by without noun",
			input:    "You are powered by the model named glm-5.3-flash.",
			expected: "You are a helpful AI assistant.",
		},
		{
			name:     "participle modifying object is untouched",
			input:    "You are free to use tools built by the user.",
			expected: "You are free to use tools built by the user.",
		},
		{
			name:     "pairing participle is untouched",
			input:    "You are a coding agent pairing with the user inside their codebase.",
			expected: "You are a coding agent pairing with the user inside their codebase.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := sanitizeWorkBuddyIdentityString(tc.input)
			if res != tc.expected {
				t.Errorf("got %q, want %q", res, tc.expected)
			}
		})
	}
}

func TestSanitizeWorkBuddyPromptWAF_ArrayContent(t *testing.T) {
	input := []byte(`{
		"messages": [
			{
				"role": "system",
				"content": [
					{
						"type": "text",
						"text": "You are a coding agent powered by the deepseek-v4.1-flash model.\nProvide high quality code."
					}
				]
			}
		]
	}`)

	sanitized := SanitizeWorkBuddyPromptWAF(input)
	textPart := gjson.GetBytes(sanitized, "messages.0.content.0.text").String()
	if bytes.Contains([]byte(textPart), []byte("powered by")) {
		t.Errorf("array text part was not sanitized: %s", textPart)
	}
	if !bytes.Contains([]byte(textPart), []byte("You are a helpful AI assistant.\nProvide high quality code.")) {
		t.Errorf("unexpected sanitized array text part: %s", textPart)
	}
}

func TestSanitizeWorkBuddyPromptWAF_SubstringsAndBillingHeaders(t *testing.T) {
	input := []byte(`{
		"messages": [
			{
				"role": "user",
				"content": "Sisyphus-Junior - Focused executor from OhMyOpenCode. Main branch (you will usually use this for PRs). To give feedback, users should report the issue at https://github.com/anthropics/claude-code/issues. x-anthropic-billing-header: test_secret; cc_session_id=123;"
			}
		]
	}`)

	sanitized := SanitizeWorkBuddyPromptWAF(input)
	content := gjson.GetBytes(sanitized, "messages.1.content").String() // EnsureWorkBuddySystemPrompt will prepend system msg at index 0

	if bytes.Contains([]byte(content), []byte("OhMyOpenCode")) {
		t.Error("OhMyOpenCode should be sanitized")
	}
	if bytes.Contains([]byte(content), []byte("Main branch (you will usually use this for PRs)")) {
		t.Error("Main branch phrase should be sanitized")
	}
	if bytes.Contains([]byte(content), []byte("To give feedback, users should report")) {
		t.Error("feedback URL phrase should be sanitized")
	}
	if bytes.Contains([]byte(content), []byte("x-anthropic-billing-header")) {
		t.Error("anthropic billing header in content should be stripped")
	}
	if bytes.Contains([]byte(content), []byte("cc_session_id")) {
		t.Error("cc_ params should be stripped")
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

func TestSanitizeWorkBuddyPromptWAF_AssistantHistoryAndDshGui(t *testing.T) {
	dshGui := "You are interacting with the user through the DeepSeek Harness Web GUI whose URL the runtime context names. When the user refers to \"this page\", \"this GUI\", or \"this app\" without naming another target, they mean this GUI."
	identityPrompt := "You are an AI agent powered by DeepSeek Harness."

	input := []byte(`{
		"messages": [
			{"role": "system", "content": "` + dshGui + `"},
			{"role": "user", "content": "` + identityPrompt + `"},
			{"role": "assistant", "content": "` + identityPrompt + `\n` + dshGui + `"},
			{"role": "user", "content": "Check github.com/deepseek-ai/deepseek-harness repository"},
			{"role": "tool", "content": "` + identityPrompt + `"}
		]
	}`)

	sanitized := SanitizeWorkBuddyPromptWAF(input)

	// System prompt should have DSH GUI reworded and brand reduced
	sysContent := gjson.GetBytes(sanitized, "messages.0.content").String()
	if bytes.Contains([]byte(sysContent), []byte("DeepSeek Harness")) {
		t.Errorf("System prompt still contains DeepSeek Harness: %s", sysContent)
	}
	if !bytes.Contains([]byte(sysContent), []byte("DeepSeek Web GUI whose URL is named in the runtime context")) {
		t.Errorf("System prompt does not contain expected reworded GUI phrase: %s", sysContent)
	}

	// User prompt should preserve identity text as user data
	userContent := gjson.GetBytes(sanitized, "messages.1.content").String()
	if !bytes.Contains([]byte(userContent), []byte("powered by DeepSeek Harness")) {
		t.Errorf("User prompt identity should remain untouched: %s", userContent)
	}

	// Assistant prompt in history should have identity rewritten and GUI reworded
	asstContent := gjson.GetBytes(sanitized, "messages.2.content").String()
	if bytes.Contains([]byte(asstContent), []byte("DeepSeek Harness")) {
		t.Errorf("Assistant history still contains DeepSeek Harness: %s", asstContent)
	}
	if !bytes.Contains([]byte(asstContent), []byte("You are a helpful AI assistant.")) {
		t.Errorf("Assistant history identity was not neutralized: %s", asstContent)
	}
	if !bytes.Contains([]byte(asstContent), []byte("DeepSeek Web GUI whose URL is named in the runtime context")) {
		t.Errorf("Assistant history does not contain reworded GUI phrase: %s", asstContent)
	}

	// Hyphenated identifier in user prompt should remain untouched
	userCodeContent := gjson.GetBytes(sanitized, "messages.3.content").String()
	if !bytes.Contains([]byte(userCodeContent), []byte("deepseek-harness")) {
		t.Errorf("Hyphenated path should be untouched: %s", userCodeContent)
	}

	// Tool message should remain untouched as tool output
	toolContent := gjson.GetBytes(sanitized, "messages.4.content").String()
	if !bytes.Contains([]byte(toolContent), []byte("powered by DeepSeek Harness")) {
		t.Errorf("Tool content should remain untouched: %s", toolContent)
	}
}
