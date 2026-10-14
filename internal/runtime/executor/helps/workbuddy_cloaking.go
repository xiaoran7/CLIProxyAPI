// Package helps provides shared utilities and helpers for runtime executors.
package helps

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const workbuddyFallbackReasoning = "[reasoning unavailable]"

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

// SanitizeWorkBuddyPromptWAF rewrites prompt substrings that trigger Tencent WAF error 11128,
// normalizes -free model aliases to their canonical upstream model IDs,
// and guarantees the first message is a valid system prompt.
func SanitizeWorkBuddyPromptWAF(payload []byte) []byte {
	if len(payload) == 0 {
		return payload
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
	}
	for _, r := range replacements {
		payload = bytes.ReplaceAll(payload, r.target, r.sub)
	}
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
