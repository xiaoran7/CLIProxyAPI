package middleware

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/db"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
	"github.com/tidwall/gjson"
)

var (
	traceIDAuthIndexRegex  = regexp.MustCompile(`-([0-9a-fA-F]{16})-`)
	googleAccountsURLRegex = regexp.MustCompile(`https://accounts\.google\.com/[^\s"'<>]+`)
)

const (
	maxBufferedResponseSize   = 4 * 1024 * 1024 // 4 MB
	standardGeminiContinueURL = "https://developers.google.com/gemini-code-assist/auth/auth_success_gemini"
)

// AccountingMiddleware wraps the response writer to capture latency, TTFT, token metrics,
// and errors, synchronously persisting them to the transactional usage outbox.
func AccountingMiddleware(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Skip Console management routes, native CPA management, OAuth, and static health endpoints
		if ShouldSkipGateway(path) {
			c.Next()
			return
		}

		startTime := time.Now()

		writer := &accountingWriter{
			ResponseWriter: c.Writer,
			startTime:      startTime,
			body:           &bytes.Buffer{},
			sseParser:      newSSEParser(),
		}
		c.Writer = writer

		c.Next()

		writer.Finalize(c, eng)
	}
}

type accountingWriter struct {
	gin.ResponseWriter
	mu sync.Mutex

	startTime   time.Time
	ttftMs      int64
	isStreaming bool
	body        *bytes.Buffer
	sseParser   *sseParser

	promptTokens     int64
	completionTokens int64
	cachedTokens     int64
	totalTokens      int64
	isEstimated      bool
	outputChars      int64

	finalized bool
}

func (w *accountingWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.isStreaming {
		ct := w.ResponseWriter.Header().Get("Content-Type")
		if strings.Contains(ct, "text/event-stream") {
			w.isStreaming = true
		}
	}

	if w.isStreaming {
		if w.ttftMs == 0 && len(data) > 0 {
			w.ttftMs = time.Since(w.startTime).Milliseconds()
		}
		w.processSSEData(data)
	} else {
		if w.body.Len() < maxBufferedResponseSize {
			remain := maxBufferedResponseSize - w.body.Len()
			if len(data) <= remain {
				w.body.Write(data)
			} else {
				w.body.Write(data[:remain])
			}
		}
	}

	return w.ResponseWriter.Write(data)
}

func (w *accountingWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *accountingWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *accountingWriter) processSSEData(chunk []byte) {
	events := w.sseParser.Feed(chunk)
	for _, eventData := range events {
		if eventData == "[DONE]" {
			continue
		}

		rawBytes := []byte(eventData)

		// 1. Try extracting standard usage
		prompt, completion, cached, total, ok := engine.ExtractUsageFromJSON(rawBytes)
		if !ok {
			// Check nested message usage (Anthropic message_start/message_delta)
			if msg := gjson.GetBytes(rawBytes, "message"); msg.Exists() {
				prompt, completion, cached, total, ok = engine.ExtractUsageFromJSON([]byte(msg.Raw))
			}
		}

		if ok {
			// Merge cross-chunk usage (e.g. preserving promptTokens from message_start, adding completion from message_delta)
			if prompt > 0 || w.promptTokens == 0 {
				w.promptTokens = prompt
			}
			if cached > 0 || w.cachedTokens == 0 {
				w.cachedTokens = cached
			}
			if completion > 0 {
				w.completionTokens = completion
			}
			if total > 0 {
				w.totalTokens = total
			} else {
				w.totalTokens = w.promptTokens + w.completionTokens
			}
		}

		// 2. Count output characters for fallback token estimation
		if choices := gjson.GetBytes(rawBytes, "choices"); choices.IsArray() {
			for _, choice := range choices.Array() {
				delta := choice.Get("delta")
				if !delta.Exists() {
					delta = choice.Get("message")
				}
				if delta.Exists() {
					w.outputChars += int64(len(delta.Get("content").String()))
					w.outputChars += int64(len(delta.Get("reasoning_content").String()))
					w.outputChars += int64(len(delta.Get("reasoning").String()))
					if toolCalls := delta.Get("tool_calls"); toolCalls.IsArray() {
						for _, tc := range toolCalls.Array() {
							w.outputChars += int64(len(tc.Get("function.arguments").String()))
						}
					}
				}
			}
		}

		// Anthropic text_delta and thinking_delta
		if delta := gjson.GetBytes(rawBytes, "delta"); delta.Exists() {
			w.outputChars += int64(len(delta.Get("text").String()))
			w.outputChars += int64(len(delta.Get("thinking").String()))
		}
	}
}

func (w *accountingWriter) Finalize(c *gin.Context, eng *engine.Engine) {
	w.mu.Lock()
	if w.finalized {
		w.mu.Unlock()
		return
	}
	w.finalized = true
	w.mu.Unlock()

	latencyMs := time.Since(w.startTime).Milliseconds()
	status := c.Writer.Status()
	if status == 0 {
		status = http.StatusOK
	}

	// Check if client aborted connection
	if c.Request.Context().Err() == context.Canceled || status == 499 {
		status = 499
	}

	// Extract AuthIndex
	authIndex := w.extractAuthIndex(c)

	// Extract Model and Reasoning Effort
	model := c.GetString(ContextModel)
	reasoningEffort := c.GetString(ContextReasoning)

	// Extract KeyID
	var keyIDPtr *int64
	if val, exists := c.Get(ContextKeyID); exists {
		if id, ok := val.(int64); ok && id > 0 {
			keyIDPtr = &id
		}
	}

	// Token Accounting
	if !w.isStreaming {
		bodyBytes := w.body.Bytes()
		prompt, completion, cached, total, ok := engine.ExtractUsageFromJSON(bodyBytes)
		if ok {
			w.promptTokens = prompt
			w.completionTokens = completion
			w.cachedTokens = cached
			w.totalTokens = total
		} else if status >= 200 && status < 300 {
			// Fallback token estimation for non-stream responses with text content
			outChars := int64(0)
			if choices := gjson.GetBytes(bodyBytes, "choices"); choices.IsArray() {
				for _, choice := range choices.Array() {
					msg := choice.Get("message")
					outChars += int64(len(msg.Get("content").String()))
					outChars += int64(len(msg.Get("reasoning_content").String()))
				}
			}
			if outChars > 0 {
				w.completionTokens = (outChars + 3) / 4
				w.totalTokens = w.promptTokens + w.completionTokens
				w.isEstimated = true
			}
		}
		if w.ttftMs == 0 {
			w.ttftMs = latencyMs
		}
	} else {
		// Streaming fallback estimation if upstream didn't send usage chunk
		if w.totalTokens == 0 && w.outputChars > 0 {
			w.completionTokens = (w.outputChars + 3) / 4
			w.totalTokens = w.promptTokens + w.completionTokens
			w.isEstimated = true
		}
	}

	// Error & Validation URL Interception
	var errorType string
	if status >= 400 {
		errorType = fmt.Sprintf("upstream_status_%d", status)
		if status == 499 {
			errorType = "client_closed"
		}

		errBody := w.body.String()
		validationURL := extractGoogleValidationURL(errBody)
		if validationURL != "" || (status == 403 && strings.Contains(errBody, "VALIDATION_REQUIRED")) {
			errorType = "upstream_validation_required"
			if validationURL != "" || authIndex != "" {
				provider := "antigravity"
				errMsg := "Verify your account to continue."
				if msg := gjson.Get(errBody, "error.metadata.validation_error_message").String(); msg != "" {
					errMsg = msg
				} else if msg := gjson.Get(errBody, "metadata.validation_error_message").String(); msg != "" {
					errMsg = msg
				}
				now := time.Now().Unix()
				_, _ = eng.DB().InsertValidation(c.Request.Context(), &db.UpstreamValidationRow{
					TS:            now,
					AuthIndex:     authIndex,
					Provider:      provider,
					ValidationURL: validationURL,
					ErrorMessage:  &errMsg,
					RawError:      &errBody,
					Status:        "pending",
					UpdatedAt:     now,
				})
			}
		}

		summary := errBody
		if len(summary) > 300 {
			summary = summary[:300]
		}
		if summary == "" {
			summary = fmt.Sprintf("HTTP %d error", status)
		}

		var authIndexPtr *string
		if authIndex != "" {
			authIndexPtr = &authIndex
		}
		var modelPtr *string
		if model != "" {
			modelPtr = &model
		}
		var errorTypePtr *string
		if errorType != "" {
			errorTypePtr = &errorType
		}
		summaryPtr := &summary

		_, _ = eng.DB().InsertErrorEvent(c.Request.Context(), &db.ErrorEventRow{
			TS:        time.Now().Unix(),
			KeyID:     keyIDPtr,
			AuthIndex: authIndexPtr,
			Model:     modelPtr,
			Status:    status,
			ErrorType: errorTypePtr,
			ErrorBody: summaryPtr,
		})
	}

	var ttftPtr *int64
	if w.ttftMs > 0 {
		ttft := w.ttftMs
		ttftPtr = &ttft
	}

	info := &engine.RequestInfo{
		KeyID:            keyIDPtr,
		Model:            model,
		Status:           status,
		PromptTokens:     w.promptTokens,
		CompletionTokens: w.completionTokens,
		CachedTokens:     w.cachedTokens,
		TotalTokens:      w.totalTokens,
		LatencyMs:        latencyMs,
		TtftMs:           ttftPtr,
		Stream:           w.isStreaming,
		Estimated:        w.isEstimated,
		ReasoningEffort:  reasoningEffort,
		AuthIndex:        authIndex,
		ErrorType:        errorType,
	}

	_ = eng.RecordRequest(context.Background(), info)
}

func (w *accountingWriter) extractAuthIndex(c *gin.Context) string {
	// 1. From response header X-Cpa-Trace-Id (format: ...-<16-hex-auth-index>-...)
	traceID := w.ResponseWriter.Header().Get("X-Cpa-Trace-Id")
	if traceID == "" {
		traceID = c.Writer.Header().Get("X-Cpa-Trace-Id")
	}
	if traceID != "" {
		matches := traceIDAuthIndexRegex.FindStringSubmatch(traceID)
		if len(matches) > 1 {
			return strings.ToLower(matches[1])
		}
	}

	// 2. From context
	if val, exists := c.Get(ContextAuthIndex); exists {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	if val, exists := c.Get("AUTH_INDEX"); exists {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}

	return ""
}

// sseParser parses chunks into complete SSE data strings.
type sseParser struct {
	buffer []byte
}

func newSSEParser() *sseParser {
	return &sseParser{
		buffer: make([]byte, 0, 4096),
	}
}

func (p *sseParser) Feed(chunk []byte) []string {
	p.buffer = append(p.buffer, chunk...)
	var events []string

	for {
		idx := bytes.IndexByte(p.buffer, '\n')
		if idx == -1 {
			break
		}
		line := bytes.TrimRight(p.buffer[:idx], "\r")
		p.buffer = p.buffer[idx+1:]

		if bytes.HasPrefix(line, []byte("data:")) {
			data := bytes.TrimSpace(line[5:])
			if len(data) > 0 {
				events = append(events, string(data))
			}
		}
	}

	return events
}

// ExtractGoogleValidationURL extracts and completes Google Antigravity verification URLs.
func ExtractGoogleValidationURL(body string) string {
	if body == "" || !strings.Contains(body, "accounts.google.com") {
		return ""
	}

	cleaned := strings.ReplaceAll(body, `\/`, `/`)
	cleaned = strings.ReplaceAll(cleaned, `&amp;`, `&`)

	match := googleAccountsURLRegex.FindString(cleaned)
	if match == "" {
		return ""
	}

	match = strings.TrimRight(match, "),.;]}>")
	parsed, err := url.Parse(match)
	if err != nil || parsed.Hostname() != "accounts.google.com" {
		return ""
	}

	// Autocomplete missing redirect destination if needed
	if parsed.Path == "/signin/continue" {
		q := parsed.Query()
		if !q.Has("sarp") {
			q.Set("sarp", "1")
		}
		if !q.Has("scc") {
			q.Set("scc", "1")
		}
		if !q.Has("continue") {
			q.Set("continue", standardGeminiContinueURL)
		}
		if !q.Has("flowName") {
			q.Set("flowName", "GlifWebSignIn")
		}
		parsed.RawQuery = q.Encode()
	}

	return parsed.String()
}

func extractGoogleValidationURL(body string) string {
	return ExtractGoogleValidationURL(body)
}
