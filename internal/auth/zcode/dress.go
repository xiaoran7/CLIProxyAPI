package zcode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ApplyStartPlanHeaders replaces every caller header with the set the ZCode app
// sends to the Start Plan.
//
// Nothing of the caller's survives: Coding Assistant's anthropic-beta, x-app,
// x-stainless-*, and session headers reach this gateway but never ZCode, and
// the Start Plan's tool-search beta belongs to coding plans only.
func ApplyStartPlanHeaders(req *http.Request, jwt, device string) {
	if req == nil {
		return
	}
	for key := range req.Header {
		req.Header.Del(key)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("HTTP-Referer", OriginZCode)
	req.Header.Set("User-Agent", MessagesUserAgent)
	req.Header.Set("X-ZCode-App-Version", AppVersion)
	req.Header.Set("X-Title", "Z Code@cli")
	req.Header.Set("X-Release-Channel", "production")
	req.Header.Set("X-Client-Language", clientLocale())
	req.Header.Set("X-Client-Timezone", clientTimezone())
	req.Header.Set("X-ZCode-Agent", "glm")
	req.Header.Set("X-Platform", platformName())
	req.Header.Set("X-Os-Category", osCategory())
	req.Header.Set("X-Os-Version", osVersion())
	req.Header.Set("x-request-id", uuid.NewString())
	req.Header.Set("x-zcode-session-type", "main")
	req.Header.Set("x-zcode-trace-id", uuid.NewString())
	req.Header.Set("x-zcode-query-source", "main_turn")
	_ = device
}

// ApplyCodingPlanHeaders sets the credential headers a GLM Coding Plan request
// carries. ZCode sends the minted `<id>.<secret>` key both ways.
func ApplyCodingPlanHeaders(req *http.Request, key string) {
	if req == nil || strings.TrimSpace(key) == "" {
		return
	}
	req.Header.Del("Authorization")
	req.Header.Set("x-api-key", key)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("anthropic-version", "2023-06-01")
}

// StartPlanURL strips the query an agent adds to the Messages path. ZCode asks
// /v1/messages alone; the Start Plan refuses the beta query the SDK appends.
func StartPlanURL(url string) string {
	if idx := strings.IndexByte(url, '?'); idx >= 0 {
		return url[:idx]
	}
	return url
}

// WorkPlace is where the agent asking works, as the dressing's environment
// section names it. Both fields are optional and omitted when unknown.
type WorkPlace struct {
	// Dir is the agent's working directory.
	Dir string
	// Git reports whether that directory is a git repository ("yes"/"no").
	Git string
}

// DressContext carries what a Start Plan body's dressing needs.
type DressContext struct {
	// Site is the realm the account signed in to.
	Site string
	// Device is the device id the metadata names.
	Device string
	// Place is where the agent works, when known.
	Place WorkPlace
	// Now is the local date the context prefix names.
	Now time.Time
}

// Dress rewrites a Messages body into the shape the Start Plan accepts.
//
// ZCode sends exactly three cached system blocks, a <system-reminder> date turn
// before the agent's first turn, one cache breakpoint on the last content block
// of the last turn, none on the tools, and metadata.user_id naming the device.
// zcode.z.ai turns a fourth system block away with 405 / 3012, and the first two
// blocks must stay byte for byte ZCode's own texts.
//
// A body that is not a Messages request is returned unchanged.
func Dress(body []byte, dc DressContext) []byte {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body
	}
	root := gjson.ParseBytes(body)
	if !root.IsObject() || !root.Get("messages").IsArray() {
		return body
	}
	if dc.Now.IsZero() {
		dc.Now = nowFunc()
	}

	provider := "zai-api"
	if normalizeSite(dc.Site) == SiteBigModel {
		provider = "bigmodel-api"
	}
	model := strings.TrimSpace(root.Get("model").String())

	// The agent's own system text joins the third block, as ZCode's context
	// builder joins every dynamic section into that one block.
	own := callerSystemTexts(root.Get("system"))
	dynamic := []string{
		promptBeforeEnvironment,
		environmentSection(provider, model, dc.Place),
		promptAfterEnvironment,
	}
	dynamic = append(dynamic, own...)

	system := []any{
		cachedText(promptCLIPrefix),
		cachedText(promptStableSection0 + "\n\n" + promptStableSection1),
		cachedText("\n\n" + strings.Join(dynamic, "\n\n")),
	}

	messages := []any{contextPrefix(dc.Now)}
	for _, message := range root.Get("messages").Array() {
		if !message.IsObject() {
			continue
		}
		messages = append(messages, json.RawMessage(message.Raw))
	}

	out := body
	var err error
	out, err = sjson.SetBytes(out, "system", system)
	if err != nil {
		return body
	}
	out, err = sjson.SetBytes(out, "messages", messages)
	if err != nil {
		return body
	}

	// Only the last turn's last block keeps a cache breakpoint.
	out = stripTurnCacheControl(out)
	out = markLastTurnCacheable(out)
	out = stripToolCacheControl(out)

	out, err = sjson.SetBytes(out, "metadata.user_id", userIDEnvelope(dc.Device))
	if err != nil {
		return body
	}
	return out
}

// callerSystemTexts collects the non-empty text blocks of the caller's system
// prompt, in order.
func callerSystemTexts(system gjson.Result) []string {
	var texts []string
	if system.Type == gjson.String {
		if text := system.String(); strings.TrimSpace(text) != "" {
			texts = append(texts, text)
		}
		return texts
	}
	for _, block := range system.Array() {
		if block.Get("type").String() != "text" {
			continue
		}
		if text := block.Get("text").String(); strings.TrimSpace(text) != "" {
			texts = append(texts, text)
		}
	}
	return texts
}

func cachedText(text string) map[string]any {
	return map[string]any{
		"type":          "text",
		"text":          text,
		"cache_control": map[string]any{"type": "ephemeral"},
	}
}

// environmentSection renders the prompt's environment block.
func environmentSection(provider, model string, place WorkPlace) string {
	lines := []string{
		promptEnvironmentHeading,
		promptEnvironmentInvokedLine,
	}
	if dir := strings.TrimSpace(place.Dir); dir != "" {
		lines = append(lines, fmt.Sprintf("- %s: %s", promptEnvironmentCwdLabel, dir))
		if place.Git == "yes" || place.Git == "no" {
			git := promptEnvironmentGitNo
			if place.Git == "yes" {
				git = "yes"
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", promptEnvironmentGitLabel, git))
		}
	}
	lines = append(lines,
		fmt.Sprintf("- %s: %s", promptEnvironmentPlatformLabel, platformOS()),
		fmt.Sprintf("- %s: %s", promptEnvironmentOSVersionLabel, promptOSVersion()),
	)
	if model != "" {
		line := strings.ReplaceAll(promptEnvironmentPoweredByLine, "{provider}", provider)
		line = strings.ReplaceAll(line, "{model}", model)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// contextPrefix renders the user turn ZCode puts before the agent's own.
func contextPrefix(now time.Time) map[string]any {
	body := strings.Join([]string{
		promptContextIntro,
		promptContextCurrentDateHeading + "\n" + strings.ReplaceAll(promptContextCurrentDateLine, "{date}", now.Format("2006-01-02")),
		"",
		promptContextOutro,
	}, "\n")
	return map[string]any{
		"role": "user",
		"content": []any{
			map[string]any{
				"type": "text",
				"text": "<system-reminder>" + body + "</system-reminder>",
			},
		},
	}
}

// userIDEnvelope renders the metadata.user_id the Start Plan expects.
func userIDEnvelope(device string) string {
	payload := map[string]string{
		"device_id":    device,
		"account_uuid": "",
		"session_id":   "",
	}
	encoded, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		return device
	}
	return string(encoded)
}

// stripTurnCacheControl removes every cache breakpoint from the conversation
// turns, leaving the tools and system blocks alone.
func stripTurnCacheControl(body []byte) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}
	out := body
	for i, message := range messages.Array() {
		if message.Get("role").String() == "system" {
			continue
		}
		for j, block := range message.Get("content").Array() {
			if !block.IsObject() || !block.Get("cache_control").Exists() {
				continue
			}
			path := fmt.Sprintf("messages.%d.content.%d.cache_control", i, j)
			if updated, errDelete := sjson.DeleteBytes(out, path); errDelete == nil {
				out = updated
			}
		}
	}
	return out
}

// markLastTurnCacheable puts one cache breakpoint on the last content block of
// the last turn.
func markLastTurnCacheable(body []byte) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}
	list := messages.Array()
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].Get("role").String() == "system" {
			continue
		}
		content := list[i].Get("content")
		if content.Type == gjson.String {
			block := cachedText(content.String())
			if updated, errSet := sjson.SetBytes(body, fmt.Sprintf("messages.%d.content", i), []any{block}); errSet == nil {
				return updated
			}
			return body
		}
		blocks := content.Array()
		if len(blocks) == 0 {
			return body
		}
		last := len(blocks) - 1
		if blocks[last].Get("cache_control").Exists() {
			return body
		}
		path := fmt.Sprintf("messages.%d.content.%d.cache_control", i, last)
		if updated, errSet := sjson.SetBytes(body, path, map[string]any{"type": "ephemeral"}); errSet == nil {
			return updated
		}
		return body
	}
	return body
}

// stripToolCacheControl removes cache breakpoints from the tool definitions.
func stripToolCacheControl(body []byte) []byte {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return body
	}
	out := body
	for i, tool := range tools.Array() {
		if !tool.Get("cache_control").Exists() {
			continue
		}
		if updated, errDelete := sjson.DeleteBytes(out, fmt.Sprintf("tools.%d.cache_control", i)); errDelete == nil {
			out = updated
		}
	}
	return out
}
