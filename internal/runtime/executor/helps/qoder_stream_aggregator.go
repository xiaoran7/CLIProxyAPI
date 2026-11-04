// Package helps provides shared utilities and helpers for runtime executors.
package helps

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// UnwrapQoderSSELine unescapes Spring Boot ResponseEntity SSE chunks from Qoder
// (e.g. `data: {"headers":{...},"body":"{\"choices\":[...]}","statusCodeValue":200}`)
// into standard OpenAI SSE lines (`data: {"choices":[...]}`).
// It returns (unwrappedLine, true) for valid OpenAI stream events or [DONE],
// and (nil, false) for ignorable telemetry / metadata events.
func UnwrapQoderSSELine(line []byte) ([]byte, bool) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return nil, false
	}
	if !bytes.HasPrefix(trimmed, []byte("data:")) {
		return nil, false
	}
	data := bytes.TrimSpace(trimmed[5:])
	if len(data) == 0 {
		return nil, false
	}
	if bytes.Equal(data, []byte("[DONE]")) {
		return []byte("data: [DONE]"), true
	}

	// Non-JSON events like "data: event:finish"
	if !gjson.ValidBytes(data) {
		return nil, false
	}

	parsed := gjson.ParseBytes(data)

	// Check if wrapped in Spring ResponseEntity: {"headers": ..., "body": "...", "statusCodeValue": ...}
	if bodyRes := parsed.Get("body"); bodyRes.Exists() {
		rawBody := strings.TrimSpace(bodyRes.String())
		if rawBody == "[DONE]" {
			return []byte("data: [DONE]"), true
		}
		if len(rawBody) > 0 && (rawBody[0] == '{' || rawBody[0] == '[') && gjson.Valid(rawBody) {
			return []byte("data: " + rawBody), true
		}
		return nil, false
	}

	// Telemetry lines without choices/id (e.g. {"firstTokenDuration":...})
	if !parsed.Get("choices").Exists() && !parsed.Get("id").Exists() && !parsed.Get("error").Exists() && !parsed.Get("code").Exists() {
		return nil, false
	}

	// Already standard OpenAI SSE line
	return trimmed, true
}

// ParseQoderStreamError checks if an SSE line or data chunk contains an embedded Qoder error response.
func ParseQoderStreamError(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		trimmed = bytes.TrimSpace(trimmed[5:])
	}
	if len(trimmed) == 0 || !gjson.ValidBytes(trimmed) {
		return nil
	}
	parsed := gjson.ParseBytes(trimmed)
	if codeVal := parsed.Get("code"); codeVal.Exists() && codeVal.Int() != 0 && codeVal.Int() != 200 {
		msg := parsed.Get("msg").String()
		if msg == "" {
			msg = parsed.Get("message").String()
		}
		return fmt.Errorf("qoder upstream error (code %d): %s", codeVal.Int(), msg)
	}
	if errVal := parsed.Get("error"); errVal.Exists() {
		msg := errVal.Get("message").String()
		if msg == "" {
			msg = errVal.String()
		}
		return fmt.Errorf("qoder upstream error: %s", msg)
	}
	return nil
}

// AggregateQoderStream folds an SSE stream from Qoder into a single standard OpenAI
// non-streaming chat.completion response.
func AggregateQoderStream(stream []byte, fallbackModel string) ([]byte, error) {
	var (
		respID       string
		model        string
		created      int64
		finishReason string
		contentBuf   strings.Builder
		reasoningBuf strings.Builder
		hasContent   bool
		hasReasoning bool
		toolCallsMap = make(map[int]*aggregatedToolCall)
		usageRaw     json.RawMessage
	)

	scanner := bufio.NewScanner(bytes.NewReader(stream))
	// Allocate buffer up to 2MB for long SSE lines
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		rawLine := scanner.Bytes()
		unwrapped, ok := UnwrapQoderSSELine(rawLine)
		if !ok {
			continue
		}
		line := bytes.TrimSpace(unwrapped)
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(line[5:])
		if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
			continue
		}

		if errStream := ParseQoderStreamError(data); errStream != nil {
			return nil, errStream
		}

		if !gjson.ValidBytes(data) {
			continue
		}
		parsed := gjson.ParseBytes(data)

		if idVal := parsed.Get("id").String(); idVal != "" {
			respID = idVal
		}
		if modelVal := parsed.Get("model").String(); modelVal != "" {
			model = modelVal
		}
		if crVal := parsed.Get("created").Int(); crVal > 0 {
			created = crVal
		}
		if uVal := parsed.Get("usage"); uVal.Exists() && uVal.Raw != "" {
			usageRaw = json.RawMessage(uVal.Raw)
		}

		choices := parsed.Get("choices").Array()
		for _, choice := range choices {
			delta := choice.Get("delta")
			if delta.Exists() {
				if cVal := delta.Get("content").String(); cVal != "" {
					contentBuf.WriteString(cVal)
					hasContent = true
				}
				if rVal := delta.Get("reasoning_content").String(); rVal != "" {
					reasoningBuf.WriteString(rVal)
					hasReasoning = true
				}
				if tcArray := delta.Get("tool_calls").Array(); len(tcArray) > 0 {
					for _, tc := range tcArray {
						idx := int(tc.Get("index").Int())
						entry, ok := toolCallsMap[idx]
						if !ok {
							entry = &aggregatedToolCall{
								Index: idx,
								Type:  "function",
							}
							toolCallsMap[idx] = entry
						}
						if callID := tc.Get("id").String(); callID != "" {
							entry.ID = callID
						}
						if callType := tc.Get("type").String(); callType != "" {
							entry.Type = callType
						}
						if fn := tc.Get("function"); fn.Exists() {
							if fnName := fn.Get("name").String(); fnName != "" {
								entry.Name += fnName
							}
							if fnArgs := fn.Get("arguments").String(); fnArgs != "" {
								entry.Arguments += fnArgs
							}
						}
					}
				}
			}
			if frVal := choice.Get("finish_reason").String(); frVal != "" {
				finishReason = frVal
			}
		}
	}

	if errScan := scanner.Err(); errScan != nil {
		return nil, fmt.Errorf("aggregate qoder stream error: %w", errScan)
	}

	if respID == "" {
		respID = fmt.Sprintf("chatcmpl-qoder-%d", time.Now().UnixNano())
	}
	if model == "" {
		model = fallbackModel
	}
	if created <= 0 {
		created = time.Now().Unix()
	}
	if finishReason == "" {
		if len(toolCallsMap) > 0 {
			finishReason = "tool_calls"
		} else {
			finishReason = "stop"
		}
	} else if finishReason == "stop" && len(toolCallsMap) > 0 {
		finishReason = "tool_calls"
	}

	// Build output message
	msgObj := map[string]any{
		"role": "assistant",
	}
	if hasContent {
		msgObj["content"] = contentBuf.String()
	} else {
		msgObj["content"] = ""
	}
	if hasReasoning {
		msgObj["reasoning_content"] = reasoningBuf.String()
	}

	if len(toolCallsMap) > 0 {
		keys := make([]int, 0, len(toolCallsMap))
		for k := range toolCallsMap {
			keys = append(keys, k)
		}
		sort.Ints(keys)

		var toolCallsList []map[string]any
		for _, k := range keys {
			tc := toolCallsMap[k]
			if tc.Name == "" && tc.Arguments == "" {
				continue
			}
			idVal := tc.ID
			if idVal == "" {
				idVal = fmt.Sprintf("call_%d_%d", k, time.Now().UnixNano())
			}
			toolCallsList = append(toolCallsList, map[string]any{
				"id":   idVal,
				"type": tc.Type,
				"function": map[string]any{
					"name":      tc.Name,
					"arguments": tc.Arguments,
				},
			})
		}
		if len(toolCallsList) > 0 {
			msgObj["tool_calls"] = toolCallsList
		}
	}

	choiceObj := map[string]any{
		"index":         0,
		"message":       msgObj,
		"finish_reason": finishReason,
	}

	outObj := map[string]any{
		"id":      respID,
		"object":  "chat.completion",
		"created": created,
		"model":   model,
		"choices": []any{choiceObj},
	}
	if len(usageRaw) > 0 {
		outObj["usage"] = usageRaw
	}

	return json.Marshal(outObj)
}
