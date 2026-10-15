package executor

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/cache"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openCodeBase62Chars      = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	openCodeVersion          = "1.19.0"
	openCodeUserAgent        = "opencode/" + openCodeVersion + " ai-sdk/provider-utils/4.0.23 runtime/bun/1.3.14"
	openCodeClientHeader     = "cli"
	openCodeProjectHeader    = "global"
	openCodeDefaultMaxTokens = 32000
	openCodeStubDescription  = "Not available in this session. Never call this tool."
	openCodeStubNote         = "(The model asked for a tool this session doesn't have.)"
)

var (
	// OpenCode mandatory tools required by Zen Free tier.
	openCodeRequiredTools = []string{"bash", "read"}

	openCodeIDRe = regexp.MustCompile(`^[a-z]{3}_[0-9a-f]{12}[0-9A-Za-z]{14}$`)

	openCodeClock = struct {
		sync.Mutex
		last    int64
		counter int64
	}{}

	// openCodeSessionCache keeps session IDs across conversation turns (up to 1000 items).
	openCodeSessionCache = cache.NewBoundedLRU[string, string](1000, nil)
)

// isOpenCodeTarget checks whether an upstream endpoint/provider is OpenCode.
func isOpenCodeTarget(baseURL, provider, compatName string) bool {
	u := strings.ToLower(strings.TrimSpace(baseURL))
	p := strings.ToLower(strings.TrimSpace(provider))
	c := strings.ToLower(strings.TrimSpace(compatName))
	return strings.Contains(u, "opencode.ai") ||
		p == "opencode" ||
		c == "opencode" ||
		strings.HasPrefix(p, "openai-compatible-opencode")
}

// genOpenCodeID generates a timestamp-bound Base62 ID according to OpenCode's reverse-engineered schema.
// descending = true for session ID (^current & 0xFFFFFFFFFFFF)
// descending = false for request ID (current & 0xFFFFFFFFFFFF)
func genOpenCodeID(prefix string, descending bool, now time.Time) string {
	ms := now.UnixMilli()
	openCodeClock.Lock()
	if ms != openCodeClock.last {
		openCodeClock.last, openCodeClock.counter = ms, 0
	}
	openCodeClock.counter++
	current := uint64(ms)*0x1000 + uint64(openCodeClock.counter)
	openCodeClock.Unlock()

	if descending {
		current = ^current
	}

	var randBytes [14]byte
	if _, err := rand.Read(randBytes[:]); err != nil {
		for i := range randBytes {
			randBytes[i] = byte(i * 17)
		}
	}

	id := []byte(fmt.Sprintf("%s_%012x", prefix, current&0xffffffffffff))
	for _, b := range randBytes {
		id = append(id, openCodeBase62Chars[int(b)%len(openCodeBase62Chars)])
	}
	return string(id)
}

// openCodeRealSession validates if id is a structurally genuine OpenCode session ID.
func openCodeRealSession(id string) bool {
	if !strings.HasPrefix(id, "ses_") || !openCodeIDRe.MatchString(id) {
		return false
	}
	low, err := strconv.ParseUint(id[13:16], 16, 16)
	return err == nil && 0xfff-low < 16
}

// resolveOpenCodeSessionID finds or creates a persistent session ID for the given conversation seed.
func resolveOpenCodeSessionID(seed string) string {
	if openCodeRealSession(seed) {
		return seed
	}
	if seed == "" {
		return genOpenCodeID("ses", true, time.Now())
	}
	return openCodeSessionCache.GetOrAdd(seed, func() string {
		return genOpenCodeID("ses", true, time.Now())
	})
}

// extractOpenCodeSessionSeed extracts a candidate session identity seed from the HTTP request or context.
func extractOpenCodeSessionSeed(ctx context.Context, req *http.Request) string {
	if req != nil {
		for _, h := range []string{"x-opencode-session", "x-opencode-session-id", "x-session-affinity", "X-Session-Id", "x-session-id"} {
			if val := strings.TrimSpace(req.Header.Get(h)); val != "" {
				return val
			}
		}
	}
	if ctx != nil {
		if sessID := strings.TrimSpace(util.SessionIDFromContext(ctx)); sessID != "" {
			return sessID
		}
	}
	return ""
}

// applyOpenCodeHeaders injects required authentication & fingerprint headers to pass OpenCode cloud inspection.
// It also strips any headers that would expose non-OpenCode client identity.
func applyOpenCodeHeaders(req *http.Request) {
	if req == nil {
		return
	}
	// Strip foreign client headers that might trigger upstream inspection
	req.Header.Del("x-client")
	req.Header.Del("x-app-name")
	req.Header.Del("x-app-version")
	req.Header.Del("x-zcode-app-version")
	req.Header.Del("x-zcode-agent")
	req.Header.Del("x-zcode-session-type")
	req.Header.Del("x-zcode-trace-id")

	seed := extractOpenCodeSessionSeed(req.Context(), req)
	sesID := resolveOpenCodeSessionID(seed)
	msgID := genOpenCodeID("msg", false, time.Now())

	req.Header.Set("User-Agent", openCodeUserAgent)
	req.Header.Set("x-opencode-client", openCodeClientHeader)
	req.Header.Set("x-opencode-project", openCodeProjectHeader)
	req.Header.Set("x-opencode-session", sesID)
	req.Header.Set("x-opencode-session-id", sesID)
	req.Header.Set("x-session-affinity", sesID)
	req.Header.Set("X-Session-Id", sesID)
	req.Header.Set("x-opencode-request", msgID)
}

// openCodeToolsInfo holds mappings for tool normalization and injected stubs.
type openCodeToolsInfo struct {
	back  map[string]string // normalized name ("bash", "read") -> original client name ("Bash", "Read")
	stubs map[string]bool   // set of tool names injected as stubs
}

// makeOpenCodeStubTool creates a function tool definition matching OpenCode Zen Free's requirements.
func makeOpenCodeStubTool(name string) map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        name,
			"description": openCodeStubDescription,
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}
}

// applyOpenCodePayload adjusts request body to satisfy OpenCode's FreeTier requirements.
// Specifically:
// 1. Normalizes client tool names like "Bash" or "Read" to lowercase "bash" and "read".
// 2. Injects missing required tools ("bash", "read") as non-callable stubs.
// 3. Defaults max_tokens if missing.
// 4. Forces stream: true upstream if required.
func applyOpenCodePayload(payload []byte, forceStream bool) ([]byte, *openCodeToolsInfo, error) {
	if len(payload) == 0 {
		return payload, nil, nil
	}

	info := &openCodeToolsInfo{
		back:  make(map[string]string),
		stubs: make(map[string]bool),
	}

	toolsVal := gjson.GetBytes(payload, "tools")
	var existingTools []any
	if toolsVal.Exists() && toolsVal.IsArray() {
		for _, item := range toolsVal.Array() {
			existingTools = append(existingTools, item.Value())
		}
	}

	for _, reqName := range openCodeRequiredTools {
		foundIdx := -1
		for i, t := range existingTools {
			var name string
			switch tv := t.(type) {
			case map[string]any:
				if fn, ok := tv["function"].(map[string]any); ok {
					if n, ok := fn["name"].(string); ok {
						name = n
					}
				}
			}
			if strings.EqualFold(name, reqName) {
				foundIdx = i
				if name != reqName {
					// Rename in payload
					info.back[reqName] = name
					if tv, ok := existingTools[i].(map[string]any); ok {
						if fn, ok := tv["function"].(map[string]any); ok {
							fn["name"] = reqName
						}
					}
				}
				break
			}
		}

		if foundIdx < 0 {
			// Not found, inject as stub
			stub := makeOpenCodeStubTool(reqName)
			existingTools = append(existingTools, stub)
			info.stubs[reqName] = true
		}
	}

	var err error
	payload, err = sjson.SetBytes(payload, "tools", existingTools)
	if err != nil {
		return nil, nil, fmt.Errorf("opencode: set tools error: %w", err)
	}

	if !gjson.GetBytes(payload, "tool_choice").Exists() {
		payload, err = sjson.SetBytes(payload, "tool_choice", "auto")
		if err != nil {
			return nil, nil, fmt.Errorf("opencode: inject tool_choice error: %w", err)
		}
	}

	// 2. Default max_tokens if missing
	if !gjson.GetBytes(payload, "max_tokens").Exists() && !gjson.GetBytes(payload, "max_completion_tokens").Exists() {
		payload, err = sjson.SetBytes(payload, "max_tokens", openCodeDefaultMaxTokens)
		if err != nil {
			return nil, nil, fmt.Errorf("opencode: inject max_tokens error: %w", err)
		}
	}

	// 3. Force stream upstream if required
	if forceStream {
		payload, err = sjson.SetBytes(payload, "stream", true)
		if err != nil {
			return nil, nil, fmt.Errorf("opencode: force stream error: %w", err)
		}
	}

	return payload, info, nil
}

// openCodeStreamFilter tracks and suppresses stub tool calls in real-time SSE streaming.
type openCodeStreamFilter struct {
	info           *openCodeToolsInfo
	droppingCallID map[string]bool
	droppingIdx    map[int64]bool
	stubCalled     bool
	realCallsCount int
	hasSaidContent bool
}

func newOpenCodeStreamFilter(info *openCodeToolsInfo) *openCodeStreamFilter {
	if info == nil {
		info = &openCodeToolsInfo{
			back:  make(map[string]string),
			stubs: make(map[string]bool),
		}
	}
	return &openCodeStreamFilter{
		info:           info,
		droppingCallID: make(map[string]bool),
		droppingIdx:    make(map[int64]bool),
	}
}

// FilterChunk inspects a single upstream SSE data JSON payload.
// Returns the filtered data payload and a boolean `drop` indicating whether to drop this frame completely.
func (f *openCodeStreamFilter) FilterChunk(dataPayload []byte) ([]byte, bool) {
	if len(dataPayload) == 0 || bytes.Equal(dataPayload, []byte("[DONE]")) {
		return dataPayload, false
	}

	parsed := gjson.ParseBytes(dataPayload)
	choices := parsed.Get("choices")
	if !choices.Exists() || !choices.IsArray() || len(choices.Array()) == 0 {
		return dataPayload, false
	}

	choice := choices.Array()[0]
	delta := choice.Get("delta")

	// Track if content was spoken
	if delta.Exists() {
		if content := delta.Get("content").String(); content != "" {
			f.hasSaidContent = true
		}
	}

	toolCalls := delta.Get("tool_calls")
	modified := false

	if delta.Exists() && toolCalls.Exists() && toolCalls.IsArray() && len(toolCalls.Array()) > 0 {
		var keptCalls []any
		for _, tc := range toolCalls.Array() {
			idx := tc.Get("index").Int()
			id := tc.Get("id").String()
			name := tc.Get("function.name").String()

			// Check if this tool call is a stub
			isStub := false
			if name != "" {
				if f.info.stubs[name] {
					isStub = true
					f.stubCalled = true
					f.droppingIdx[idx] = true
					if id != "" {
						f.droppingCallID[id] = true
					}
				} else {
					f.droppingIdx[idx] = false
					if id != "" {
						f.droppingCallID[id] = false
					}
					// Restore original casing if client used different casing (e.g. "Bash")
					if ownName, ok := f.info.back[name]; ok {
						modified = true
						name = ownName
					}
					f.realCallsCount++
				}
			} else {
				// Continuation chunk with arguments only
				if f.droppingIdx[idx] || (id != "" && f.droppingCallID[id]) {
					isStub = true
				}
			}

			if isStub {
				modified = true
				continue
			}

			// Keep this tool call
			tcVal := tc.Value()
			if ownName, ok := f.info.back[name]; ok && name != "" {
				if tcMap, ok := tcVal.(map[string]any); ok {
					if fnMap, ok := tcMap["function"].(map[string]any); ok {
						fnMap["name"] = ownName
					}
				}
			}
			keptCalls = append(keptCalls, tcVal)
		}

		if len(keptCalls) == 0 {
			// All tool calls in this chunk were stubs
			var err error
			dataPayload, err = sjson.DeleteBytes(dataPayload, "choices.0.delta.tool_calls")
			if err != nil {
				return dataPayload, false
			}
			modified = true
		} else if modified {
			var err error
			dataPayload, err = sjson.SetBytes(dataPayload, "choices.0.delta.tool_calls", keptCalls)
			if err != nil {
				return dataPayload, false
			}
		}
	}

	// Handle finish_reason conversion
	finishReason := choice.Get("finish_reason").String()
	if finishReason != "" {
		if finishReason == "tool_calls" && f.stubCalled && f.realCallsCount == 0 {
			// Convert to "stop" since only stubs were called
			var err error
			dataPayload, err = sjson.SetBytes(dataPayload, "choices.0.finish_reason", "stop")
			if err != nil {
				return dataPayload, false
			}
			if !f.hasSaidContent {
				// Inject fallback stub note so downstream client doesn't receive an empty stop
				dataPayload, _ = sjson.SetBytes(dataPayload, "choices.0.delta.content", openCodeStubNote)
				f.hasSaidContent = true
			}
			modified = true
		}
	}

	// If this chunk now has empty delta (no content, no role, no tool_calls, no finish_reason), drop it
	updatedChoice := gjson.GetBytes(dataPayload, "choices.0")
	updatedDelta := updatedChoice.Get("delta")
	updatedFinish := updatedChoice.Get("finish_reason").String()
	if updatedFinish == "" {
		if !updatedDelta.Exists() {
			return nil, true
		}
		hasAnyDelta := updatedDelta.Get("content").Exists() ||
			updatedDelta.Get("role").Exists() ||
			updatedDelta.Get("tool_calls").Exists() ||
			updatedDelta.Get("reasoning").Exists() ||
			updatedDelta.Get("reasoning_content").Exists()
		if !hasAnyDelta {
			return nil, true
		}
	}

	return dataPayload, false
}

// aggregateOpenCodeSSEToChatCompletion consumes an SSE stream from OpenCode Zen
// and rebuilds a standard non-streaming OpenAI chat completion JSON response.
func aggregateOpenCodeSSEToChatCompletion(sseReader io.Reader, fallbackModel string, info *openCodeToolsInfo) ([]byte, error) {
	if info == nil {
		info = &openCodeToolsInfo{
			back:  make(map[string]string),
			stubs: make(map[string]bool),
		}
	}

	scanner := bufio.NewScanner(sseReader)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

	var (
		resID         string
		resModel      = fallbackModel
		created       int64
		contentBuffer bytes.Buffer
		reasoningBuf  bytes.Buffer
		finishReason  = "stop"
		promptTokens  int64
		compTokens    int64
	)

	type toolCallState struct {
		id      string
		name    string
		argsBuf bytes.Buffer
		typeVal string
	}
	toolCallsByIdx := make(map[int64]*toolCallState)
	var toolIdxOrder []int64

	for scanner.Scan() {
		line := scanner.Bytes()
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if bytes.Equal(data, []byte("[DONE]")) {
			break
		}
		if len(data) == 0 {
			continue
		}

		parsed := gjson.ParseBytes(data)
		if id := parsed.Get("id").String(); id != "" && resID == "" {
			resID = id
		}
		if m := parsed.Get("model").String(); m != "" {
			resModel = m
		}
		if c := parsed.Get("created").Int(); c != 0 {
			created = c
		}

		choices := parsed.Get("choices").Array()
		if len(choices) > 0 {
			choice := choices[0]
			delta := choice.Get("delta")
			if delta.Exists() {
				if text := delta.Get("content").String(); text != "" {
					contentBuffer.WriteString(text)
				}
				if r := delta.Get("reasoning").String(); r != "" {
					reasoningBuf.WriteString(r)
				}
				if rc := delta.Get("reasoning_content").String(); rc != "" {
					reasoningBuf.WriteString(rc)
				}
				if tcs := delta.Get("tool_calls"); tcs.Exists() && tcs.IsArray() {
					for _, tc := range tcs.Array() {
						idx := tc.Get("index").Int()
						st, ok := toolCallsByIdx[idx]
						if !ok {
							st = &toolCallState{typeVal: "function"}
							toolCallsByIdx[idx] = st
							toolIdxOrder = append(toolIdxOrder, idx)
						}
						if id := tc.Get("id").String(); id != "" {
							st.id = id
						}
						if name := tc.Get("function.name").String(); name != "" {
							st.name = name
						}
						if args := tc.Get("function.arguments").String(); args != "" {
							st.argsBuf.WriteString(args)
						}
					}
				}
			}
			if fr := choice.Get("finish_reason").String(); fr != "" {
				finishReason = fr
			}
		}

		if usage := parsed.Get("usage"); usage.Exists() {
			if pt := usage.Get("prompt_tokens").Int(); pt > 0 {
				promptTokens = pt
			}
			if ct := usage.Get("completion_tokens").Int(); ct > 0 {
				compTokens = ct
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return nil, fmt.Errorf("opencode: read stream error: %w", err)
	}

	if resID == "" {
		resID = fmt.Sprintf("chatcmpl-opencode-%d", time.Now().UnixMilli())
	}
	if created == 0 {
		created = time.Now().Unix()
	}
	if promptTokens == 0 {
		promptTokens = 10
	}
	if compTokens == 0 {
		compTokens = int64(contentBuffer.Len() / 4)
		if compTokens == 0 && contentBuffer.Len() > 0 {
			compTokens = 1
		}
	}

	// Filter and normalize tool calls
	var finalToolCalls []map[string]any
	stubCalled := false
	for _, idx := range toolIdxOrder {
		st := toolCallsByIdx[idx]
		if st == nil {
			continue
		}
		if info.stubs[st.name] {
			stubCalled = true
			continue
		}
		displayName := st.name
		if own, ok := info.back[st.name]; ok {
			displayName = own
		}
		finalToolCalls = append(finalToolCalls, map[string]any{
			"id":   st.id,
			"type": st.typeVal,
			"function": map[string]any{
				"name":      displayName,
				"arguments": st.argsBuf.String(),
			},
		})
	}

	// If only stubs were called, convert finish_reason from tool_calls to stop
	if len(finalToolCalls) == 0 && stubCalled && finishReason == "tool_calls" {
		finishReason = "stop"
	}

	finalContent := contentBuffer.String()
	if finalContent == "" && reasoningBuf.Len() > 0 {
		finalContent = reasoningBuf.String()
	}
	if finalContent == "" && len(finalToolCalls) == 0 && stubCalled {
		finalContent = openCodeStubNote
	}

	message := map[string]any{
		"role":    "assistant",
		"content": finalContent,
	}
	if len(finalToolCalls) > 0 {
		message["tool_calls"] = finalToolCalls
	}

	finalResponse := map[string]any{
		"id":      resID,
		"object":  "chat.completion",
		"created": created,
		"model":   resModel,
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       message,
				"finish_reason": finishReason,
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     promptTokens,
			"completion_tokens": compTokens,
			"total_tokens":      promptTokens + compTokens,
		},
	}

	return json.Marshal(finalResponse)
}
