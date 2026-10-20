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
		line := bytes.TrimSpace(scanner.Bytes())
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

		if !gjson.ValidBytes(data) {
			continue
		}
		parsed := gjson.ParseBytes(data)

		// Check for error responses embedded in stream
		if codeVal := parsed.Get("code"); codeVal.Exists() && codeVal.Int() != 0 && codeVal.Int() != 200 {
			msg := parsed.Get("msg").String()
			if msg == "" {
				msg = parsed.Get("message").String()
			}
			return nil, fmt.Errorf("qoder upstream error (code %d): %s", codeVal.Int(), msg)
		}

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
