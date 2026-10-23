package executor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	zcodes "github.com/router-for-me/CLIProxyAPI/v8/internal/auth/zcode"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/runtime/executor/helps"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/thinking"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// ZCodeExecutor executes Messages requests against ZCode's GLM Coding Plans
// (api.z.ai/api/anthropic and open.bigmodel.cn/api/anthropic) and ZCode's free
// Start Plan (zcode.z.ai/api/v1/zcode-plan/anthropic).
//
// The upstream speaks AI's Messages protocol, so the executor follows the
// Claude path for translation and response handling. What differs is the wire:
// a coding plan request carries the minted key both as x-api-key and as a
// bearer, and a Start Plan request is dressed into exactly the shape the ZCode
// app sends, because zcode.z.ai refuses anything else.
type ZCodeExecutor struct {
	cfg *config.Config
	// startBase overrides the Start Plan's base URL. It stays empty in
	// production, where the route names zcode.z.ai itself; a test points it at
	// a local server.
	startBase string
}

// NewZCodeExecutor creates a new ZCode executor.
func NewZCodeExecutor(cfg *config.Config) *ZCodeExecutor {
	return &ZCodeExecutor{cfg: cfg}
}

// Identifier returns the canonical executor identifier.
func (e *ZCodeExecutor) Identifier() string { return "zcode" }

// RequestToFormat reports the upstream request format: Messages.
func (e *ZCodeExecutor) RequestToFormat(_ cliproxyexecutor.Request, _ cliproxyexecutor.Options) sdktranslator.Format {
	return sdktranslator.FormatClaude
}

// PrepareRequest injects ZCode credentials into an outgoing request.
func (e *ZCodeExecutor) PrepareRequest(req *http.Request, auth *cliproxyauth.Auth) error {
	if req == nil {
		return nil
	}
	if key := zcodes.AccessKey(auth); key != "" {
		zcodes.ApplyCodingPlanHeaders(req, key)
	}
	var attrs map[string]string
	if auth != nil {
		attrs = auth.Attributes
	}
	util.ApplyCustomHeadersFromAttrs(req, attrs)
	return nil
}

// HttpRequest injects ZCode credentials into the request and executes it.
func (e *ZCodeExecutor) HttpRequest(ctx context.Context, auth *cliproxyauth.Auth, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("zcode executor: request is nil")
	}
	if ctx == nil {
		ctx = req.Context()
	}
	httpReq := req.WithContext(ctx)
	if errPrep := e.PrepareRequest(httpReq, auth); errPrep != nil {
		return nil, errPrep
	}
	return helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 0).Do(httpReq)
}

// Execute performs a non-streaming Messages request.
func (e *ZCodeExecutor) Execute(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (resp cliproxyexecutor.Response, err error) {
	ctx = helps.EnsureSessionContext(ctx, opts, req.Payload)
	baseModel := thinking.ParseSuffix(req.Model).ModelName

	route, errRoute := zcodes.ResolveRoute(auth, baseModel)
	if errRoute != nil {
		return resp, statusErr{code: http.StatusUnauthorized, msg: errRoute.Error()}
	}

	reporter := helps.NewExecutorUsageReporter(ctx, e, baseModel, auth)
	defer reporter.TrackFailure(ctx, &err)
	if route.Model != baseModel {
		reporter.SetUpstreamModel(route.Model)
	}

	body, originalTranslated, errBuild := e.buildRequest(ctx, auth, req, opts, route, false)
	if errBuild != nil {
		return resp, errBuild
	}

	httpResp, errDo := e.do(ctx, auth, route, body, false)
	if errDo != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, errDo)
		return resp, errDo
	}
	defer func() {
		if errClose := httpResp.Body.Close(); errClose != nil {
			log.Errorf("zcode executor: close response body error: %v", errClose)
		}
	}()

	helps.RecordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	data, errRead := io.ReadAll(httpResp.Body)
	if errRead != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, errRead)
		return resp, errRead
	}
	helps.AppendAPIResponseChunk(ctx, e.cfg, data)

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		helps.LogWithRequestID(ctx).Debugf("zcode request error, status: %d, message: %s", httpResp.StatusCode, helps.SummarizeErrorBody(httpResp.Header.Get("Content-Type"), data))
		return resp, e.classifyFailure(route, httpResp.StatusCode, data)
	}
	if errRefusal := e.refusalError(route, httpResp.StatusCode, data); errRefusal != nil {
		return resp, errRefusal
	}

	reporter.ObserveResponseModel(data)
	to := sdktranslator.FromString("claude")
	responseFormat := cliproxyexecutor.ResponseFormatOrSource(opts)
	out := data
	if responseFormat != to {
		var param any
		out = sdktranslator.TranslateNonStream(ctx, to, responseFormat, req.Model, helps.ApplyPatchOriginalRequest(req, opts), originalTranslated, data, &param)
		if helps.ApplyPatchTranslationError(param) != nil || len(out) == 0 {
			return cliproxyexecutor.Response{}, statusErr{code: http.StatusBadGateway, msg: helps.ApplyPatchUpstreamErrorMessage}
		}
	}
	reporter.Publish(ctx, helps.ParseClaudeUsage(data))
	if responseFormat == sdktranslator.FormatOpenAIResponse {
		out = helps.EnsureResponsesUsageDetails(out)
	}
	return cliproxyexecutor.Response{Payload: out, Headers: httpResp.Header.Clone()}, nil
}

// ExecuteStream performs a streaming Messages request.
func (e *ZCodeExecutor) ExecuteStream(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (_ *cliproxyexecutor.StreamResult, err error) {
	ctx = helps.EnsureSessionContext(ctx, opts, req.Payload)
	baseModel := thinking.ParseSuffix(req.Model).ModelName

	route, errRoute := zcodes.ResolveRoute(auth, baseModel)
	if errRoute != nil {
		return nil, statusErr{code: http.StatusUnauthorized, msg: errRoute.Error()}
	}

	reporter := helps.NewExecutorUsageReporter(ctx, e, baseModel, auth)
	defer reporter.TrackFailure(ctx, &err)
	if route.Model != baseModel {
		reporter.SetUpstreamModel(route.Model)
	}

	body, originalTranslated, errBuild := e.buildRequest(ctx, auth, req, opts, route, true)
	if errBuild != nil {
		return nil, errBuild
	}

	httpResp, errDo := e.do(ctx, auth, route, body, true)
	if errDo != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, errDo)
		return nil, errDo
	}

	helps.RecordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		defer func() {
			if errClose := httpResp.Body.Close(); errClose != nil {
				log.Errorf("zcode executor: close response body error: %v", errClose)
			}
		}()
		data, _ := io.ReadAll(httpResp.Body)
		helps.AppendAPIResponseChunk(ctx, e.cfg, data)
		helps.LogWithRequestID(ctx).Debugf("zcode stream error, status: %d, message: %s", httpResp.StatusCode, helps.SummarizeErrorBody(httpResp.Header.Get("Content-Type"), data))
		return nil, e.classifyFailure(route, httpResp.StatusCode, data)
	}

	to := sdktranslator.FromString("claude")
	responseFormat := cliproxyexecutor.ResponseFormatOrSource(opts)
	originalRequest := helps.ApplyPatchOriginalRequest(req, opts)
	out := make(chan cliproxyexecutor.StreamChunk)
	go func() {
		defer close(out)
		defer func() {
			if errClose := httpResp.Body.Close(); errClose != nil {
				log.Errorf("zcode executor: close response body error: %v", errClose)
			}
		}()

		var streamUsage helps.StreamUsageBuffer
		defer streamUsage.Publish(ctx, reporter)

		scanner := bufio.NewScanner(httpResp.Body)
		scanner.Buffer(nil, 52_428_800)

		// A Start Plan refusal can arrive as a 200 whose body is an error frame.
		var refusal error
		var param any
		for scanner.Scan() {
			line := scanner.Bytes()
			helps.AppendAPIResponseChunk(ctx, e.cfg, line)
			reporter.ObserveResponseModel(line)
			streamUsage.ObserveClaudeStream(line)

			if refusal == nil {
				if errRefusal := e.refusalError(route, http.StatusOK, line); errRefusal != nil {
					refusal = errRefusal
				}
			}
			chunks := [][]byte{line}
			if responseFormat != to {
				chunks = sdktranslator.TranslateStream(ctx, to, responseFormat, req.Model, originalRequest, originalTranslated, bytes.Clone(line), &param)
				if responseFormat == sdktranslator.FormatOpenAIResponse && helps.ApplyPatchTranslationError(param) == nil {
					for i := range chunks {
						chunks[i] = helps.EnsureResponsesUsageDetails(chunks[i])
					}
				}
			}
			for _, chunk := range chunks {
				select {
				case out <- cliproxyexecutor.StreamChunk{Payload: chunk}:
				case <-ctx.Done():
					return
				}
			}
		}
		if refusal != nil {
			helps.RecordAPIResponseError(ctx, e.cfg, refusal)
			streamUsage.PublishFailure(ctx, reporter, refusal)
			select {
			case out <- cliproxyexecutor.StreamChunk{Err: refusal}:
			case <-ctx.Done():
			}
			return
		}
		if errScan := scanner.Err(); errScan != nil {
			helps.RecordAPIResponseError(ctx, e.cfg, errScan)
			reporter.PublishFailure(ctx, errScan)
			select {
			case out <- cliproxyexecutor.StreamChunk{Err: errScan}:
			case <-ctx.Done():
			}
		}
	}()

	return &cliproxyexecutor.StreamResult{Headers: httpResp.Header.Clone(), Chunks: out}, nil
}

// CountTokens estimates the input token count for a request.
func (e *ZCodeExecutor) CountTokens(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	route, errRoute := zcodes.ResolveRoute(auth, baseModel)
	if errRoute != nil {
		return cliproxyexecutor.Response{}, statusErr{code: http.StatusUnauthorized, msg: errRoute.Error()}
	}

	from := opts.SourceFormat
	to := sdktranslator.FromString("claude")
	body, errTranslate := helps.TranslateRequestReturningError(ctx, opts.Headers, e.cfg, from, to, baseModel, req.Payload, false, helps.APIKeyModelIsCompat(req))
	if errTranslate != nil {
		return cliproxyexecutor.Response{}, errTranslate
	}
	originalTranslated := body
	if len(opts.OriginalRequest) > 0 {
		originalTranslated = helps.TranslateRequestWithAPIKeyModelCompatibility(ctx, opts.Headers, e.cfg, from, to, baseModel, opts.OriginalRequest, false, helps.APIKeyModelIsCompat(req))
	}
	body = helps.SetStringIfDifferent(body, "model", route.Model)
	body, errThinking := helps.ApplyRequestThinking(body, req, opts, from.String(), to.String(), e.Identifier())
	if errThinking != nil {
		return cliproxyexecutor.Response{}, errThinking
	}
	body = helps.NewPayloadFinalizer(e.cfg, e.Identifier(), baseModel, to.String(), "", originalTranslated, req, opts)(body)

	count, errCount := helps.CountClaudeInputTokens(body)
	if errCount != nil {
		return cliproxyexecutor.Response{}, fmt.Errorf("zcode executor: token counting failed: %w", errCount)
	}
	usageJSON := []byte(fmt.Sprintf(`{"input_tokens":%d}`, count))
	responseFormat := cliproxyexecutor.ResponseFormatOrSource(opts)
	return cliproxyexecutor.Response{Payload: sdktranslator.TranslateTokenCount(ctx, to, responseFormat, count, usageJSON)}, nil
}

// Refresh reports that a ZCode credential cannot be renewed in place. The
// coding plan key does not expire, and ZCode's session token cannot be
// refreshed; the account has to sign in again when it runs out.
func (e *ZCodeExecutor) Refresh(ctx context.Context, auth *cliproxyauth.Auth) (*cliproxyauth.Auth, error) {
	if refreshed, handled, err := helps.RefreshAuthViaHome(ctx, e.cfg, auth); handled {
		return refreshed, err
	}
	if auth == nil {
		return nil, fmt.Errorf("zcode executor: auth is nil")
	}
	if jwt := zcodes.StartToken(auth); jwt != "" && zcodes.JWTExpired(jwt, time.Now()) && zcodes.AccessKey(auth) == "" {
		return nil, statusErr{code: http.StatusUnauthorized, msg: "zcode: ZCode's sign-in has expired; sign in to ZCode again"}
	}
	return auth, nil
}

// buildRequest translates the inbound payload into a Messages body and applies
// the ZCode wire rules to it.
//
// The built-in transformation finishes here; the caller's payload rules run
// last, exactly once, before the transport-level dressing.
func (e *ZCodeExecutor) buildRequest(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options, route zcodes.Route, stream bool) ([]byte, []byte, error) {
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	from := opts.SourceFormat
	to := sdktranslator.FromString("claude")

	originalPayload := req.Payload
	if len(opts.OriginalRequest) > 0 {
		originalPayload = opts.OriginalRequest
	}

	originalTranslated, body, errTranslate := helps.TranslateRequestPairReturningError(ctx, opts.Headers, e.cfg, from, to, baseModel, originalPayload, req.Payload, stream, helps.APIKeyModelIsCompat(req))
	if errTranslate != nil {
		return nil, nil, errTranslate
	}
	body = helps.SetStringIfDifferent(body, "model", route.Model)

	body, errThinking := helps.ApplyRequestThinking(body, req, opts, from.String(), to.String(), e.Identifier())
	if errThinking != nil {
		return nil, nil, errThinking
	}

	// ZCode accepts one cache breakpoint shape only; the dressing below owns it
	// for the Start Plan, so the built-in cache rewriting is skipped there.
	if !route.Start {
		body = enforceCacheControlLimit(body, 4)
	}

	requestedModel := helps.PayloadRequestedModel(opts, req.Model)
	requestPath := helps.PayloadRequestPath(opts)
	body = helps.ApplyPayloadConfigWithRequest(e.cfg, baseModel, to.String(), from.String(), "", body, originalTranslated, requestedModel, requestPath, opts.Headers)
	reporter := helps.NewExecutorUsageReporter(ctx, e, baseModel, auth)
	reporter.SetTranslatedReasoningEffort(body, to.String())

	// The transport frame is the only thing allowed after the payload rules.
	if route.Start {
		body = zcodes.Dress(body, zcodes.DressContext{
			Site:   route.Site,
			Device: route.Device,
			Place:  workPlace(opts.Headers),
			Now:    time.Now(),
		})
	}
	return body, originalTranslated, nil
}

// workPlace reads where the agent that asked works, when the caller says so.
//
// The dressing's environment section names the folder and whether it is a git
// repository; an agent that tells the gateway neither gets neither, as ZCode
// leaves them out rather than naming a place that is not the agent's.
func workPlace(headers http.Header) zcodes.WorkPlace {
	if headers == nil {
		return zcodes.WorkPlace{}
	}
	var place zcodes.WorkPlace
	if raw := strings.TrimSpace(headers.Get("x-magpie-work-dir")); raw != "" {
		if dir, errUnescape := url.PathUnescape(raw); errUnescape == nil {
			place.Dir = strings.TrimSpace(dir)
		} else {
			place.Dir = raw
		}
	}
	if place.Dir != "" {
		switch strings.ToLower(strings.TrimSpace(headers.Get("x-magpie-work-git"))) {
		case "yes", "no":
			place.Git = strings.ToLower(strings.TrimSpace(headers.Get("x-magpie-work-git")))
		}
	}
	return place
}

// do sends the prepared body to the route's endpoint.
func (e *ZCodeExecutor) do(ctx context.Context, auth *cliproxyauth.Auth, route zcodes.Route, body []byte, stream bool) (*http.Response, error) {
	base := route.BaseURL
	if route.Start && e.startBase != "" {
		base = e.startBase
	}
	url := strings.TrimRight(base, "/") + "/v1/messages"
	if route.Start {
		// ZCode asks /v1/messages alone; the SDK's ?beta=true is refused.
		url = zcodes.StartPlanURL(url)
	}
	httpReq, errReq := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if errReq != nil {
		return nil, errReq
	}
	// The header set comes first: a Start Plan request carries ZCode's headers
	// and nothing of the caller's, so anything set before this is discarded.
	if route.Start {
		zcodes.ApplyStartPlanHeaders(httpReq, route.JWT, route.Device)
	} else {
		zcodes.ApplyCodingPlanHeaders(httpReq, route.Key)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if route.Start {
		// ZCode's own client leaves Accept to its HTTP client, which sends */*.
		// Naming text/event-stream here would be a header ZCode never sends.
		httpReq.Header.Set("Accept", "*/*")
	} else if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	} else {
		httpReq.Header.Set("Accept", "application/json")
	}
	var attrs map[string]string
	if auth != nil {
		attrs = auth.Attributes
	}
	util.ApplyCustomHeadersFromAttrs(httpReq, attrs)

	authID, authLabel, authType, authValue := claudeAuthLogIdentity(auth)
	helps.RecordAPIRequest(ctx, e.cfg, helps.UpstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      body,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	client := helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	return client.Do(httpReq)
}

// classifyFailure turns an upstream error answer into the error the caller sees.
func (e *ZCodeExecutor) classifyFailure(route zcodes.Route, status int, body []byte) error {
	if errRefusal := e.refusalError(route, status, body); errRefusal != nil {
		return errRefusal
	}
	return statusErr{code: status, msg: string(body)}
}

// refusalError reports a Start Plan answer that turned the request away, in the
// words that tell an operator what to do about it.
func (e *ZCodeExecutor) refusalError(route zcodes.Route, status int, body []byte) error {
	if !route.Start {
		return nil
	}
	kind := zcodes.ClassifyRefusal(status, body)
	if kind == zcodes.RefusalNone {
		return nil
	}
	if route.Trial && kind == zcodes.RefusalQuota {
		return statusErr{code: http.StatusTooManyRequests, msg: zcodes.TrialExhaustedMessage(route.Model, true)}
	}
	// A refusal keeps the vendor's own status when it gave one: a 405 from
	// zcode.z.ai's edge is what an operator needs to see. A refusal inside a
	// 2xx answer has no status of its own and gets the one that fits.
	code := status
	if code < 400 {
		code = http.StatusForbidden
	}
	if kind == zcodes.RefusalQuota {
		code = http.StatusTooManyRequests
	}
	return statusErr{code: code, msg: zcodes.RefusalMessage(kind, upstreamMessage(body))}
}

// upstreamMessage reads the message an upstream error body states.
func upstreamMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	for _, path := range []string{"error.message", "message", "msg"} {
		if text := strings.TrimSpace(gjson.GetBytes(body, path).String()); text != "" {
			return text
		}
	}
	return ""
}
