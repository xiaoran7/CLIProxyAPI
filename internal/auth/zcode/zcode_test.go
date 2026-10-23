package zcode

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/tidwall/gjson"
)

func TestDress_ThreeSystemBlocks(t *testing.T) {
	body := []byte(`{
		"model": "GLM-5.3-Flash",
		"system": [{"type":"text","text":"You are a coding agent.","cache_control":{"type":"ephemeral"}}],
		"messages": [{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral"}}]}],
		"tools": [{"name":"read","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}]
	}`)
	out := Dress(body, DressContext{Site: SiteZAI, Device: "dev-1", Now: time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)})

	system := gjson.GetBytes(out, "system")
	if !system.IsArray() {
		t.Fatalf("system is not an array: %s", system.Raw)
	}
	if got := len(system.Array()); got != 3 {
		t.Fatalf("system blocks = %d, want 3", got)
	}
	for i, block := range system.Array() {
		if block.Get("type").String() != "text" {
			t.Errorf("block %d type = %q, want text", i, block.Get("type").String())
		}
		if block.Get("cache_control.type").String() != "ephemeral" {
			t.Errorf("block %d has no cache breakpoint", i)
		}
	}
	if got := system.Array()[0].Get("text").String(); got != promptCLIPrefix {
		t.Errorf("first block is not ZCode's own prefix text")
	}
	wantSecond := promptStableSection0 + "\n\n" + promptStableSection1
	if got := system.Array()[1].Get("text").String(); got != wantSecond {
		t.Errorf("second block is not ZCode's stable sections")
	}
	third := system.Array()[2].Get("text").String()
	if !strings.Contains(third, "You are a coding agent.") {
		t.Errorf("third block dropped the caller's system text")
	}
	if !strings.HasPrefix(third, "\n\n") {
		t.Errorf("third block does not start with the separator")
	}
}

func TestDress_PrependsContextTurn(t *testing.T) {
	body := []byte(`{"model":"GLM-5.3","messages":[{"role":"user","content":"hi"}]}`)
	out := Dress(body, DressContext{Device: "dev-1", Now: time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC)})

	messages := gjson.GetBytes(out, "messages").Array()
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(messages))
	}
	first := messages[0]
	if first.Get("role").String() != "user" {
		t.Fatalf("first turn role = %q, want user", first.Get("role").String())
	}
	text := first.Get("content.0.text").String()
	if !strings.HasPrefix(text, "<system-reminder>") || !strings.HasSuffix(text, "</system-reminder>") {
		t.Fatalf("context turn is not wrapped in a system-reminder: %q", text)
	}
	if !strings.Contains(text, "2026-03-07") {
		t.Errorf("context turn does not name the day: %q", text)
	}
	// The last turn carries the breakpoint, so its text becomes a block.
	last := messages[1].Get("content.0")
	if last.Get("text").String() != "hi" {
		t.Errorf("caller's text was changed: %s", messages[1].Get("content").Raw)
	}
	if last.Get("cache_control.type").String() != "ephemeral" {
		t.Errorf("last turn has no cache breakpoint")
	}
}

func TestDress_OneCacheBreakpointOnLastTurn(t *testing.T) {
	body := []byte(`{
		"model": "GLM-5.3",
		"messages": [
			{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral"}}]},
			{"role":"assistant","content":[{"type":"text","text":"b","cache_control":{"type":"ephemeral"}}]},
			{"role":"user","content":[{"type":"text","text":"c"},{"type":"text","text":"d"}]}
		],
		"tools": [{"name":"read","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}]
	}`)
	out := Dress(body, DressContext{Device: "dev-1", Now: time.Unix(0, 0)})

	markers := 0
	for i, message := range gjson.GetBytes(out, "messages").Array() {
		for j, block := range message.Get("content").Array() {
			if block.Get("cache_control").Exists() {
				markers++
				if i != 3 || j != 1 {
					t.Errorf("cache breakpoint on messages.%d.content.%d, want the last block of the last turn", i, j)
				}
			}
		}
	}
	if markers != 1 {
		t.Errorf("turn cache breakpoints = %d, want 1", markers)
	}
	if gjson.GetBytes(out, "tools.0.cache_control").Exists() {
		t.Errorf("tool definition kept a cache breakpoint")
	}
}

func TestDress_MetadataUserIDEnvelope(t *testing.T) {
	body := []byte(`{"model":"GLM-5.3","metadata":{"user_id":"caller","client_trace_id":"trace-123","counter":42},"messages":[{"role":"user","content":"hi"}]}`)
	out := Dress(body, DressContext{Device: "dev-9", Now: time.Unix(0, 0)})

	if got := gjson.GetBytes(out, "metadata.client_trace_id").String(); got != "trace-123" {
		t.Errorf("metadata.client_trace_id = %q, want trace-123", got)
	}
	if got := gjson.GetBytes(out, "metadata.counter").Int(); got != 42 {
		t.Errorf("metadata.counter = %d, want 42", got)
	}

	raw := gjson.GetBytes(out, "metadata.user_id").String()
	var envelope map[string]string
	if errUnmarshal := json.Unmarshal([]byte(raw), &envelope); errUnmarshal != nil {
		t.Fatalf("metadata.user_id is not JSON: %v (%q)", errUnmarshal, raw)
	}
	if envelope["device_id"] != "dev-9" {
		t.Errorf("device_id = %q, want dev-9", envelope["device_id"])
	}
	if _, ok := envelope["account_uuid"]; !ok {
		t.Errorf("envelope is missing account_uuid")
	}
	if _, ok := envelope["session_id"]; !ok {
		t.Errorf("envelope is missing session_id")
	}
}

func TestDress_EnvironmentNamesPlace(t *testing.T) {
	body := []byte(`{"model":"GLM-5.3","messages":[{"role":"user","content":"hi"}]}`)
	out := Dress(body, DressContext{
		Site:   SiteBigModel,
		Device: "dev-1",
		Place:  WorkPlace{Dir: "C:\\work", Git: "yes"},
		Now:    time.Unix(0, 0),
	})
	third := gjson.GetBytes(out, "system.2.text").String()
	if !strings.Contains(third, "C:\\work") {
		t.Errorf("environment does not name the working directory")
	}
	if !strings.Contains(third, "bigmodel-api") {
		t.Errorf("powered-by line does not name the BigModel realm")
	}
}

func TestDress_LeavesNonMessagesBodiesAlone(t *testing.T) {
	body := []byte(`{"input":"hello"}`)
	if got := Dress(body, DressContext{}); string(got) != string(body) {
		t.Errorf("a non-Messages body was rewritten")
	}
	if got := Dress([]byte("not json"), DressContext{}); string(got) != "not json" {
		t.Errorf("an invalid body was rewritten")
	}
}

func TestApplyStartPlanHeaders_ReplacesEveryCallerHeader(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, OriginZCode+"/api/v1/zcode-plan/anthropic/v1/messages", nil)
	req.Header.Set("anthropic-beta", "tool-search-2025-01-01")
	req.Header.Set("x-app", "cli")
	req.Header.Set("x-stainless-lang", "js")
	req.Header.Set("x-api-key", "should-be-gone")

	ApplyStartPlanHeaders(req, "jwt-token", "dev-1")

	for _, gone := range []string{"anthropic-beta", "x-app", "x-stainless-lang", "x-api-key"} {
		if got := req.Header.Get(gone); got != "" {
			t.Errorf("%s survived: %q", gone, got)
		}
	}
	if got := req.Header.Get("Authorization"); got != "Bearer jwt-token" {
		t.Errorf("Authorization = %q", got)
	}
	if got := req.Header.Get("User-Agent"); got != MessagesUserAgent {
		t.Errorf("User-Agent = %q, want %q", got, MessagesUserAgent)
	}
	if got := req.Header.Get("X-ZCode-App-Version"); got != AppVersion {
		t.Errorf("X-ZCode-App-Version = %q", got)
	}
	for _, required := range []string{
		"X-Title", "X-Release-Channel", "X-Client-Language", "X-Client-Timezone",
		"X-ZCode-Agent", "X-Platform", "X-Os-Category", "X-Os-Version",
		"x-request-id", "x-zcode-session-type", "x-zcode-trace-id", "x-zcode-query-source",
		"anthropic-version", "HTTP-Referer", "Content-Type",
	} {
		if req.Header.Get(required) == "" {
			t.Errorf("%s is missing", required)
		}
	}
}

func TestApplyCodingPlanHeaders_SetsBothKeyHeaders(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, BaseURLZAI+"/v1/messages", nil)
	req.Header.Set("Authorization", "Bearer stale")
	ApplyCodingPlanHeaders(req, "id.secret")

	if got := req.Header.Get("x-api-key"); got != "id.secret" {
		t.Errorf("x-api-key = %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer id.secret" {
		t.Errorf("Authorization = %q", got)
	}
	if got := req.Header.Get("anthropic-version"); got != "2023-06-01" {
		t.Errorf("anthropic-version = %q", got)
	}
}

func TestStartPlanURL_StripsQuery(t *testing.T) {
	if got := StartPlanURL(BaseURLStartPlan + "/v1/messages?beta=true"); got != BaseURLStartPlan+"/v1/messages" {
		t.Errorf("StartPlanURL = %q", got)
	}
	plain := BaseURLStartPlan + "/v1/messages"
	if got := StartPlanURL(plain); got != plain {
		t.Errorf("StartPlanURL changed a URL without a query: %q", got)
	}
}

func TestResolveRoute(t *testing.T) {
	coding := &cliproxyauth.Auth{
		Provider:   "zcode",
		Attributes: map[string]string{"api_key": "id.secret", "zcode_token": "jwt"},
	}
	startOnly := &cliproxyauth.Auth{
		Provider:   "zcode",
		Attributes: map[string]string{"zcode_token": "jwt", "device_id": "dev-1"},
	}
	forced := &cliproxyauth.Auth{
		Provider:   "zcode",
		Attributes: map[string]string{"api_key": "id.secret", "zcode_token": "jwt", "zcode_route": "start"},
	}

	cases := []struct {
		name    string
		auth    *cliproxyauth.Auth
		model   string
		want    bool
		wantURL string
		wantMdl string
	}{
		{"coding plan model", coding, "GLM-5.3", false, BaseURLZAI, "GLM-5.3"},
		{"trial entry spends the gift", coding, "GLM-5.3-Flash-Trial", true, BaseURLStartPlan, "GLM-5.3-Flash"},
		{"start-only account", startOnly, "GLM-5.2", true, BaseURLStartPlan, "GLM-5.2"},
		{"forced start", forced, "GLM-5.3", true, BaseURLStartPlan, "GLM-5.3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route, errRoute := ResolveRoute(tc.auth, tc.model)
			if errRoute != nil {
				t.Fatalf("ResolveRoute error: %v", errRoute)
			}
			if route.Start != tc.want {
				t.Errorf("Start = %v, want %v", route.Start, tc.want)
			}
			if route.BaseURL != tc.wantURL {
				t.Errorf("BaseURL = %q, want %q", route.BaseURL, tc.wantURL)
			}
			if route.Model != tc.wantMdl {
				t.Errorf("Model = %q, want %q", route.Model, tc.wantMdl)
			}
		})
	}
}

func TestResolveRoute_NoCredential(t *testing.T) {
	empty := &cliproxyauth.Auth{Provider: "zcode"}
	if _, errRoute := ResolveRoute(empty, "GLM-5.3"); errRoute == nil {
		t.Fatal("ResolveRoute accepted a credential with no key and no token")
	}
	// A trial entry on an account with only a coding plan key has no gift to spend.
	coding := &cliproxyauth.Auth{Provider: "zcode", Attributes: map[string]string{"api_key": "id.secret"}}
	if _, errRoute := ResolveRoute(coding, "GLM-5.3-Flash-Trial"); errRoute == nil {
		t.Fatal("ResolveRoute accepted a trial entry without a Start Plan token")
	}
}

func TestResolveSite(t *testing.T) {
	cases := map[string]string{
		"zcode":          SiteZAI,
		"zcode-cn":       SiteBigModel,
		"zcode-bigmodel": SiteBigModel,
		"zcode-zai":      SiteZAI,
	}
	for provider, want := range cases {
		if got := ResolveSite(&cliproxyauth.Auth{Provider: provider}); got != want {
			t.Errorf("ResolveSite(%q) = %q, want %q", provider, got, want)
		}
	}
	byURL := &cliproxyauth.Auth{Provider: "zcode", Attributes: map[string]string{"base_url": BaseURLBigModel}}
	if got := ResolveSite(byURL); got != SiteBigModel {
		t.Errorf("ResolveSite by base_url = %q, want %q", got, SiteBigModel)
	}
}

func TestSplitTrialModel(t *testing.T) {
	if got, trial := SplitTrialModel("GLM-5.2-Trial"); got != "GLM-5.2" || !trial {
		t.Errorf("SplitTrialModel = %q, %v", got, trial)
	}
	if got, trial := SplitTrialModel("GLM-5.2"); got != "GLM-5.2" || trial {
		t.Errorf("SplitTrialModel = %q, %v", got, trial)
	}
}
