package executor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	zcodes "github.com/router-for-me/CLIProxyAPI/v8/internal/auth/zcode"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestZCodeExecutorIdentity(t *testing.T) {
	exec := NewZCodeExecutor(&config.Config{})
	if got := exec.Identifier(); got != "zcode" {
		t.Errorf("Identifier() = %q, want zcode", got)
	}
	if got := exec.RequestToFormat(cliproxyexecutor.Request{}, cliproxyexecutor.Options{}); got != sdktranslator.FormatClaude {
		t.Errorf("RequestToFormat() = %v, want FormatClaude", got)
	}
}

func TestZCodeExecutorPrepareRequest(t *testing.T) {
	exec := NewZCodeExecutor(&config.Config{})
	req, _ := http.NewRequest(http.MethodPost, zcodes.BaseURLZAI+"/v1/messages", nil)
	auth := &cliproxyauth.Auth{
		Provider:   "zcode",
		Attributes: map[string]string{"api_key": "id.secret"},
	}
	if errPrep := exec.PrepareRequest(req, auth); errPrep != nil {
		t.Fatalf("PrepareRequest: %v", errPrep)
	}
	if got := req.Header.Get("x-api-key"); got != "id.secret" {
		t.Errorf("x-api-key = %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer id.secret" {
		t.Errorf("Authorization = %q", got)
	}
}

// codingPlanRequest is a Messages body a caller of the gateway would send.
const codingPlanRequest = `{
	"model": "GLM-5.3",
	"max_tokens": 128,
	"system": "be brief",
	"messages": [{"role":"user","content":"hello"}]
}`

func TestZCodeExecutorExecuteCodingPlan(t *testing.T) {
	var seenKey, seenPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenKey = r.Header.Get("x-api-key")
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"GLM-5.3","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2}}`))
	}))
	defer server.Close()

	exec := NewZCodeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		Provider: "zcode",
		Attributes: map[string]string{
			"api_key":  "id.secret",
			"base_url": server.URL + "/api/anthropic",
		},
	}
	resp, errExec := exec.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "GLM-5.3",
		Payload: []byte(codingPlanRequest),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude})
	if errExec != nil {
		t.Fatalf("Execute: %v", errExec)
	}
	if seenKey != "id.secret" {
		t.Errorf("upstream key = %q", seenKey)
	}
	if seenPath != "/api/anthropic/v1/messages" {
		t.Errorf("upstream path = %q", seenPath)
	}
	if got := gjson.GetBytes(resp.Payload, "content.0.text").String(); got != "hi" {
		t.Errorf("response text = %q", got)
	}
}

func TestZCodeExecutorStartPlanDressesAndReplacesHeaders(t *testing.T) {
	type captured struct {
		path    string
		query   string
		headers http.Header
		body    []byte
	}
	var got captured
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.query = r.URL.RawQuery
		got.headers = r.Header.Clone()
		got.body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":5,"output_tokens":2}}`))
	}))
	defer server.Close()

	exec := NewZCodeExecutor(&config.Config{})
	exec.startBase = server.URL + "/api/v1/zcode-plan/anthropic"
	auth := &cliproxyauth.Auth{
		Provider: "zcode",
		Attributes: map[string]string{
			"zcode_token": testJWTForExecutor(t, 3600),
			"device_id":   "dev-1",
		},
	}
	headers := http.Header{}
	headers.Set("anthropic-beta", "must-not-reach-zcode")
	headers.Set("x-magpie-work-dir", "C%3A%5Cwork")
	headers.Set("x-magpie-work-git", "yes")

	_, errExec := exec.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "GLM-5.2",
		Payload: []byte(codingPlanRequest),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude, Headers: headers})
	if errExec != nil {
		t.Fatalf("Execute: %v", errExec)
	}

	if got.query != "" {
		t.Errorf("Start Plan URL kept a query: %q", got.query)
	}
	if v := got.headers.Get("anthropic-beta"); v != "" {
		t.Errorf("the caller's anthropic-beta reached ZCode: %q", v)
	}
	if v := got.headers.Get("x-magpie-work-dir"); v != "" {
		t.Errorf("the work-dir header reached ZCode: %q", v)
	}
	if v := got.headers.Get("User-Agent"); v != zcodes.MessagesUserAgent {
		t.Errorf("User-Agent = %q, want %q", v, zcodes.MessagesUserAgent)
	}
	if v := got.headers.Get("x-zcode-session-type"); v != "main" {
		t.Errorf("x-zcode-session-type = %q", v)
	}
	if v := got.headers.Get("Content-Type"); v != "application/json" {
		t.Errorf("Content-Type = %q", v)
	}
	// ZCode leaves Accept to its client, which sends */*.
	if v := got.headers.Get("Accept"); v != "*/*" {
		t.Errorf("Accept = %q, want */*", v)
	}

	system := gjson.GetBytes(got.body, "system")
	if len(system.Array()) != 3 {
		t.Fatalf("system blocks = %d, want 3", len(system.Array()))
	}
	third := system.Array()[2].Get("text").String()
	if !strings.Contains(third, "be brief") {
		t.Errorf("the caller's system text was dropped")
	}
	if !strings.Contains(third, "C:\\work") {
		t.Errorf("the work place was not named: %s", third)
	}
	if !strings.Contains(gjson.GetBytes(got.body, "messages.0.content.0.text").String(), "<system-reminder>") {
		t.Errorf("the context turn was not prepended")
	}
}

func TestZCodeExecutorStartPlanRefusal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"code":3012,"msg":"unusual request"}`))
	}))
	defer server.Close()

	exec := NewZCodeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		Provider: "zcode",
		Attributes: map[string]string{
			"zcode_token": testJWTForExecutor(t, 3600),
			"device_id":   "dev-1",
		},
	}
	route, errRoute := zcodes.ResolveRoute(auth, "GLM-5.2")
	if errRoute != nil {
		t.Fatalf("ResolveRoute: %v", errRoute)
	}
	exec.startBase = server.URL + "/api/v1/zcode-plan/anthropic"

	body, _, errBuild := exec.buildRequest(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "GLM-5.2",
		Payload: []byte(codingPlanRequest),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude}, route, false)
	if errBuild != nil {
		t.Fatalf("buildRequest: %v", errBuild)
	}
	httpResp, errDo := exec.do(context.Background(), auth, route, body, false)
	if errDo != nil {
		t.Fatalf("do: %v", errDo)
	}
	defer func() { _ = httpResp.Body.Close() }()
	data, _ := io.ReadAll(httpResp.Body)

	errRefusal := exec.classifyFailure(route, httpResp.StatusCode, data)
	if errRefusal == nil {
		t.Fatal("a 405 answer was not classified as a refusal")
	}
	status, okStatus := errRefusal.(statusErr)
	if !okStatus {
		t.Fatalf("error type = %T, want statusErr", errRefusal)
	}
	if status.code != http.StatusMethodNotAllowed {
		t.Errorf("code = %d, want the vendor's own %d", status.code, http.StatusMethodNotAllowed)
	}
	if !strings.Contains(status.msg, "Start Plan") {
		t.Errorf("message does not explain the refusal: %q", status.msg)
	}
}

func TestZCodeExecutorTrialExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"code":1113,"msg":"quota exhausted"}`))
	}))
	defer server.Close()

	exec := NewZCodeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		Provider: "zcode",
		Attributes: map[string]string{
			"api_key":     "id.secret",
			"zcode_token": testJWTForExecutor(t, 3600),
			"device_id":   "dev-1",
		},
	}
	route, errRoute := zcodes.ResolveRoute(auth, "GLM-5.2-Trial")
	if errRoute != nil {
		t.Fatalf("ResolveRoute: %v", errRoute)
	}
	if !route.Trial || !route.Start {
		t.Fatalf("a trial entry did not route to the gift: %+v", route)
	}
	exec.startBase = server.URL + "/api/v1/zcode-plan/anthropic"

	body, _, errBuild := exec.buildRequest(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "GLM-5.2-Trial",
		Payload: []byte(codingPlanRequest),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude}, route, false)
	if errBuild != nil {
		t.Fatalf("buildRequest: %v", errBuild)
	}
	if model := gjson.GetBytes(body, "model").String(); model != "GLM-5.2" {
		t.Errorf("the trial suffix reached the upstream body: %q", model)
	}

	httpResp, errDo := exec.do(context.Background(), auth, route, body, false)
	if errDo != nil {
		t.Fatalf("do: %v", errDo)
	}
	defer func() { _ = httpResp.Body.Close() }()
	data, _ := io.ReadAll(httpResp.Body)

	errFailure := exec.classifyFailure(route, httpResp.StatusCode, data)
	status, okStatus := errFailure.(statusErr)
	if !okStatus {
		t.Fatalf("error type = %T, want statusErr", errFailure)
	}
	if status.code != http.StatusTooManyRequests {
		t.Errorf("code = %d, want 429", status.code)
	}
	if !strings.Contains(status.msg, "GLM-5.2") || !strings.Contains(status.msg, "exhausted") {
		t.Errorf("message = %q", status.msg)
	}
}

func TestZCodeExecutorNoCredential(t *testing.T) {
	exec := NewZCodeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Provider: "zcode"}
	_, errExec := exec.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "GLM-5.3",
		Payload: []byte(codingPlanRequest),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude})
	if errExec == nil {
		t.Fatal("Execute accepted a credential with no key and no token")
	}
	status, okStatus := errExec.(statusErr)
	if !okStatus || status.code != http.StatusUnauthorized {
		t.Errorf("error = %v, want a 401 statusErr", errExec)
	}
}

func TestZCodeExecutorStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"usage\":{\"input_tokens\":5}}}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		}
		for _, event := range events {
			_, _ = w.Write([]byte(event))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer server.Close()

	exec := NewZCodeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		Provider: "zcode",
		Attributes: map[string]string{
			"api_key":  "id.secret",
			"base_url": server.URL + "/api/anthropic",
		},
	}
	result, errStream := exec.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "GLM-5.3",
		Payload: []byte(codingPlanRequest),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude})
	if errStream != nil {
		t.Fatalf("ExecuteStream: %v", errStream)
	}
	var sawDelta bool
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream error: %v", chunk.Err)
		}
		if strings.Contains(string(chunk.Payload), `"text":"hi"`) {
			sawDelta = true
		}
	}
	if !sawDelta {
		t.Error("the stream never carried the text delta")
	}
}

func TestZCodeExecutorStreamRefusalIn200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1113,"msg":"quota exhausted"}`))
	}))
	defer server.Close()

	exec := NewZCodeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		Provider: "zcode",
		Attributes: map[string]string{
			"zcode_token": testJWTForExecutor(t, 3600),
			"device_id":   "dev-1",
		},
	}
	route, errRoute := zcodes.ResolveRoute(auth, "GLM-5.2-Trial")
	if errRoute != nil {
		t.Fatalf("ResolveRoute: %v", errRoute)
	}
	exec.startBase = server.URL + "/api/v1/zcode-plan/anthropic"

	body, _, errBuild := exec.buildRequest(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "GLM-5.2-Trial",
		Payload: []byte(codingPlanRequest),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude}, route, true)
	if errBuild != nil {
		t.Fatalf("buildRequest: %v", errBuild)
	}
	httpResp, errDo := exec.do(context.Background(), auth, route, body, true)
	if errDo != nil {
		t.Fatalf("do: %v", errDo)
	}
	defer func() { _ = httpResp.Body.Close() }()

	data, _ := io.ReadAll(httpResp.Body)
	if got := exec.refusalError(route, httpResp.StatusCode, []byte(`{"code":1113,"msg":"quota exhausted"}`)); got == nil {
		t.Errorf("a spent gift inside a 200 was not read as a refusal (body %s)", data)
	}
}

func TestZCodeExecutorCountTokens(t *testing.T) {
	exec := NewZCodeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		Provider:   "zcode",
		Attributes: map[string]string{"api_key": "id.secret"},
	}
	resp, errCount := exec.CountTokens(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "GLM-5.3",
		Payload: []byte(codingPlanRequest),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude})
	if errCount != nil {
		t.Fatalf("CountTokens: %v", errCount)
	}
	if got := gjson.GetBytes(resp.Payload, "input_tokens").Int(); got <= 0 {
		t.Errorf("input_tokens = %d, want a positive estimate", got)
	}
}

// testJWTForExecutor mints an unsigned JWT whose expiry is offset seconds away.
func testJWTForExecutor(t *testing.T, offset int64) string {
	t.Helper()
	claims, errMarshal := json.Marshal(map[string]any{"exp": time.Now().Unix() + offset})
	if errMarshal != nil {
		t.Fatalf("marshal claims: %v", errMarshal)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	return header + "." + base64.RawURLEncoding.EncodeToString(claims) + ".sig"
}
