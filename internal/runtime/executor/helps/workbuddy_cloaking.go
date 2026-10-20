// Package helps provides shared utilities and helpers for runtime executors.
package helps

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const workbuddyFallbackReasoning = "[reasoning unavailable]"

var (
	workbuddyIdentityRegex               = regexp.MustCompile(`(?i)(You are an?(?:\s+AI)?(?:\s+coding)?\s+(?:agent\s+powered by|assistant\s+built on)[^\n]*?)(?:\.(?:[\s\r\n]|$)|$)`)
	workbuddyAnthropicBillingHeaderRegex = regexp.MustCompile(`(?i)x-anthropic-billing-header:[^;\r\n]*;?\s*`)
	workbuddyCCKeyValRegex               = regexp.MustCompile(`(?i)\bcc_[a-z0-9_]+=[^;\r\n]*;?\s*`)
)

func sanitizeWorkBuddyIdentityString(text string) string {
	return workbuddyIdentityRegex.ReplaceAllStringFunc(text, func(m string) string {
		trailing := ""
		for len(m) > 0 && (m[len(m)-1] == '\n' || m[len(m)-1] == '\r' || m[len(m)-1] == ' ' || m[len(m)-1] == '\t') {
			trailing = string(m[len(m)-1]) + trailing
			m = m[:len(m)-1]
		}
		return "You are a helpful AI assistant." + trailing
	})
}

// StripWorkBuddyForbiddenHeaders removes client billing and tracking headers
// that would trigger Tencent WAF interception.
func StripWorkBuddyForbiddenHeaders(req *http.Request) {
	if req == nil {
		return
	}
	req.Header.Del("x-anthropic-billing-header")
	req.Header.Del("anthropic-version")
	req.Header.Del("x-client")
	req.Header.Del("x-app-name")
	req.Header.Del("x-app-version")
	req.Header.Del("x-zcode-app-version")
	req.Header.Del("x-zcode-agent")
	req.Header.Del("x-zcode-session-type")
	req.Header.Del("x-zcode-trace-id")
	req.Header.Del("x-machine-id")
	req.Header.Del("x-session-id")
}

// EnsureWorkBuddySystemPrompt ensures that the first message in the payload is a system prompt,
// injecting a default one if absent, to satisfy Tencent WAF rules.
func EnsureWorkBuddySystemPrompt(payload []byte) []byte {
	if len(payload) == 0 {
		return payload
	}
	msgs := gjson.GetBytes(payload, "messages")
	if !msgs.Exists() || !msgs.IsArray() {
		return payload
	}
	arr := msgs.Array()
	if len(arr) == 0 {
		newRaw := `[{"role":"system","content":"You are an expert coding assistant."}]`
		if updated, err := sjson.SetRawBytes(payload, "messages", []byte(newRaw)); err == nil {
			return updated
		}
		return payload
	}

	firstMsg := arr[0]
	firstRole := strings.TrimSpace(firstMsg.Get("role").String())
	if firstRole == "system" {
		content := strings.TrimSpace(firstMsg.Get("content").String())
		if content != "" {
			return payload
		}
		if updated, err := sjson.SetBytes(payload, "messages.0.content", "You are an expert coding assistant."); err == nil {
			return updated
		}
		return payload
	}

	raw := strings.TrimSpace(msgs.Raw)
	if strings.HasPrefix(raw, "[") {
		newRaw := `[{"role":"system","content":"You are an expert coding assistant."},` + strings.TrimPrefix(raw, "[")
		if updated, err := sjson.SetRawBytes(payload, "messages", []byte(newRaw)); err == nil {
			return updated
		}
	}
	return payload
}

// SanitizeWorkBuddyPromptWAF normalizes developer roles to system, rewrites prompt substrings
// and framework identity sentences that trigger Tencent/WorkBuddy WAF error 11128,
// normalizes -free model aliases to their canonical upstream model IDs,
// and guarantees the first message is a valid system prompt.
func SanitizeWorkBuddyPromptWAF(payload []byte) []byte {
	if len(payload) == 0 {
		return payload
	}

	msgs := gjson.GetBytes(payload, "messages")
	if msgs.Exists() && msgs.IsArray() {
		arr := msgs.Array()
		for i, msg := range arr {
			role := strings.TrimSpace(msg.Get("role").String())
			pathPrefix := "messages." + strconv.Itoa(i)

			// Upstream WorkBuddy only accepts system/user/assistant/tool.
			// Map newer OpenAI "developer" role to "system" to prevent code 11128 rejection.
			if strings.EqualFold(role, "developer") {
				if updated, err := sjson.SetBytes(payload, pathPrefix+".role", "system"); err == nil {
					payload = updated
					role = "system"
				}
			}

			// Cleanse DSH/framework identity sentences in system (and developer) turns.
			// Upstream Tencent/WorkBuddy WAF inspects contiguous phrases like:
			// "You are an AI agent powered by DeepSeek Harness." or "You are a coding agent powered by ...".
			// Rewriting to neutral assistant identity in system prompts avoids WAF 11128 interception,
			// while leaving user/assistant/tool content untouched.
			if role == "system" {
				content := msg.Get("content")
				if content.Exists() {
					if content.Type == gjson.String {
						cStr := content.String()
						if workbuddyIdentityRegex.MatchString(cStr) {
							sanitizedContent := sanitizeWorkBuddyIdentityString(cStr)
							if updated, err := sjson.SetBytes(payload, pathPrefix+".content", sanitizedContent); err == nil {
								payload = updated
							}
						}
					} else if content.IsArray() {
						for j, part := range content.Array() {
							if part.Get("type").String() == "text" {
								tStr := part.Get("text").String()
								if workbuddyIdentityRegex.MatchString(tStr) {
									sanitizedPart := sanitizeWorkBuddyIdentityString(tStr)
									partPath := fmt.Sprintf("%s.content.%d.text", pathPrefix, j)
									if updated, err := sjson.SetBytes(payload, partPath, sanitizedPart); err == nil {
										payload = updated
									}
								}
							}
						}
					}
				}
			}
		}
	}

	payload = EnsureWorkBuddySystemPrompt(payload)
	if modelRes := gjson.GetBytes(payload, "model"); modelRes.Exists() {
		m := strings.TrimSpace(modelRes.String())
		if strings.HasSuffix(strings.ToLower(m), "-free") {
			cleanModel := m[:len(m)-len("-free")]
			if updated, err := sjson.SetBytes(payload, "model", cleanModel); err == nil {
				payload = updated
			}
		}
	}

	// Replacement map for known 11128 WAF triggers
	replacements := []struct {
		target []byte
		sub    []byte
	}{
		{
			target: []byte("You are Claude Code, Anthropic's official CLI for Claude"),
			sub:    []byte("You are an expert coding assistant"),
		},
		{
			target: []byte("Anthropic's official CLI"),
			sub:    []byte("AI developer assistant"),
		},
		{
			target: []byte("Claude Code"),
			sub:    []byte("Coding Assistant"),
		},
		{
			target: []byte("Anthropic"),
			sub:    []byte("AI"),
		},
		{
			target: []byte("Sisyphus-Junior - Focused executor from OhMyOpenCode"),
			sub:    []byte("Sisyphus-Junior - Focused executor"),
		},
		{
			target: []byte("Main branch (you will usually use this for PRs)"),
			sub:    []byte("Default branch (you will usually use this for PRs)"),
		},
		{
			target: []byte("You are a coding agent running in the Codex CLI, a terminal-based coding assistant."),
			sub:    []byte("You are a coding agent running in the Codex CLI tool, a terminal-based coding assistant."),
		},
		{
			target: []byte("To give feedback, users should report the issue at https://github.com/anthropics/claude-code/issues"),
			sub:    []byte("To provide feedback, users should report the issue at https://github.com/anthropics/claude-code/issues"),
		},
		{
			target: []byte("11128"),
			sub:    []byte("11-128"),
		},
	}
	for _, r := range replacements {
		payload = bytes.ReplaceAll(payload, r.target, r.sub)
	}

	payload = workbuddyAnthropicBillingHeaderRegex.ReplaceAll(payload, nil)
	payload = workbuddyCCKeyValRegex.ReplaceAll(payload, nil)
	return payload
}

// BackfillWorkBuddyReasoningContent ensures assistant turns with tool_calls have reasoning_content,
// as required by DeepSeek thinking models on the WorkBuddy platform.
func BackfillWorkBuddyReasoningContent(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return payload, nil
	}
	msgs := gjson.GetBytes(payload, "messages")
	if !msgs.Exists() || !msgs.IsArray() {
		return payload, nil
	}

	var latestReasoning string
	var err error

	for i, msg := range msgs.Array() {
		role := msg.Get("role").String()
		if role != "assistant" {
			continue
		}
		rc := msg.Get("reasoning_content")
		if rc.Exists() && strings.TrimSpace(rc.String()) != "" {
			latestReasoning = rc.String()
		}

		toolCalls := msg.Get("tool_calls")
		if toolCalls.Exists() && toolCalls.IsArray() && len(toolCalls.Array()) > 0 {
			if !rc.Exists() || strings.TrimSpace(rc.String()) == "" {
				val := latestReasoning
				if val == "" {
					val = workbuddyFallbackReasoning
				}
				path := "messages." + strconv.Itoa(i) + ".reasoning_content"
				payload, err = sjson.SetBytes(payload, path, val)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	return payload, nil
}
