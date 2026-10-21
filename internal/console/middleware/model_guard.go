package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/console/engine"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var geminiPathModelRegex = regexp.MustCompile(`/models/([^/:]+)`)

// ModelGuardMiddleware inspects requested models against the key's whitelist,
// extracts reasoning effort, and filters /v1/models listings.
func ModelGuardMiddleware(eng *engine.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if ShouldSkipGateway(path) {
			c.Next()
			return
		}

		// 1. Handle GET /v1/models or GET /v1beta/models filtering
		if c.Request.Method == http.MethodGet && (strings.HasSuffix(path, "/models") || strings.HasSuffix(path, "/models/")) {
			handleModelsListing(c, eng)
			return
		}

		// 2. Extract model name and reasoning effort for execution endpoints
		var model string
		var reasoningEffort string

		if c.Request.Body != nil && c.Request.ContentLength > 0 && c.Request.ContentLength < 16*1024*1024 {
			bodyBytes, err := io.ReadAll(c.Request.Body)
			if err == nil {
				c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				res := gjson.GetBytes(bodyBytes, "model")
				if res.Exists() && res.String() != "" {
					model = res.String()
				}
				reasoningEffort = engine.ExtractReasoningEffort(bodyBytes)
			}
		}

		// Fallback model extraction from URL path (e.g. Gemini /v1beta/models/gemini-2.5-flash:generateContent)
		if model == "" {
			match := geminiPathModelRegex.FindStringSubmatch(path)
			if len(match) > 1 {
				model = match[1]
			}
		}

		if model != "" {
			c.Set(ContextModel, model)
		}
		if reasoningEffort != "" {
			c.Set(ContextReasoning, reasoningEffort)
		}

		// 3. Enforce model allowlist if key is authenticated
		if val, exists := c.Get(ContextKeyID); exists {
			if keyID, ok := val.(int64); ok && keyID > 0 && model != "" {
				res := eng.CheckModelAllowed(keyID, model)
				if !res.Allowed {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
						"error": gin.H{
							"message": res.Reason,
							"type":    "permission_error",
							"code":    "model_not_allowed",
						},
					})
					return
				}
			}
		}

		c.Next()
	}
}

// modelsFilterWriter captures and filters model lists for /v1/models responses.
type modelsFilterWriter struct {
	gin.ResponseWriter
	body   *bytes.Buffer
	keyID  int64
	engine *engine.Engine
}

func (w *modelsFilterWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *modelsFilterWriter) WriteString(s string) (int, error) {
	return w.body.WriteString(s)
}

func handleModelsListing(c *gin.Context, eng *engine.Engine) {
	val, exists := c.Get(ContextKeyID)
	if !exists {
		c.Next()
		return
	}
	keyID, ok := val.(int64)
	if !ok || keyID <= 0 {
		c.Next()
		return
	}

	filterWriter := &modelsFilterWriter{
		ResponseWriter: c.Writer,
		body:           &bytes.Buffer{},
		keyID:          keyID,
		engine:         eng,
	}
	c.Writer = filterWriter

	c.Next()

	respBytes := filterWriter.body.Bytes()
	if c.Writer.Status() != http.StatusOK || len(respBytes) == 0 {
		_, _ = filterWriter.ResponseWriter.Write(respBytes)
		return
	}

	filteredBytes := filterModelsJSON(respBytes, keyID, eng)
	filterWriter.ResponseWriter.Header().Set("Content-Length", strconv.Itoa(len(filteredBytes)))
	_, _ = filterWriter.ResponseWriter.Write(filteredBytes)
}

func filterModelsJSON(raw []byte, keyID int64, eng *engine.Engine) []byte {
	res := gjson.ParseBytes(raw)

	// 1. OpenAI format: {"object":"list","data":[...]}
	if data := res.Get("data"); data.Exists() && data.IsArray() {
		var filtered []interface{}
		for _, item := range data.Array() {
			id := item.Get("id").String()
			if id != "" && eng.CheckModelAllowed(keyID, id).Allowed {
				var val interface{}
				if err := json.Unmarshal([]byte(item.Raw), &val); err == nil {
					filtered = append(filtered, val)
				}
			}
		}
		updated, err := sjson.SetBytes(raw, "data", filtered)
		if err == nil {
			return updated
		}
	}

	// 2. Gemini format: {"models":[...]}
	if models := res.Get("models"); models.Exists() && models.IsArray() {
		var filtered []interface{}
		for _, item := range models.Array() {
			name := item.Get("name").String()
			if name != "" && eng.CheckModelAllowed(keyID, name).Allowed {
				var val interface{}
				if err := json.Unmarshal([]byte(item.Raw), &val); err == nil {
					filtered = append(filtered, val)
				}
			}
		}
		updated, err := sjson.SetBytes(raw, "models", filtered)
		if err == nil {
			return updated
		}
	}

	return raw
}
