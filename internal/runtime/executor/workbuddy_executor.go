package executor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	workbuddyauth "github.com/router-for-me/CLIProxyAPI/v8/internal/auth/workbuddy"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/runtime/executor/helps"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/thinking"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	log "github.com/sirupsen/logrus"
)

// WorkBuddyExecutor executes chat completions against Tencent WorkBuddy / CodeBuddy backends.
type WorkBuddyExecutor struct {
	cfg *config.Config
}

// NewWorkBuddyExecutor creates a new WorkBuddy executor.
func NewWorkBuddyExecutor(cfg *config.Config) *WorkBuddyExecutor {
	return &WorkBuddyExecutor{cfg: cfg}
}

// Identifier returns the executor identifier.
func (e *WorkBuddyExecutor) Identifier() string {
	return "workbuddy"
}

// RequestToFormat returns the intermediate format to translate inbound requests to.
func (e *WorkBuddyExecutor) RequestToFormat(_ cliproxyexecutor.Request, _ cliproxyexecutor.Options) sdktranslator.Format {
	return sdktranslator.FormatOpenAI
}

// workbuddyCreds extracts the bearer token from the given auth object.
func workbuddyCreds(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	if strings.TrimSpace(auth.Token) != "" {
		return auth.Token
	}
	if auth.Metadata != nil {
		if t, ok := auth.Metadata["access_token"].(string); ok && strings.TrimSpace(t) != "" {
			return t
		}
	}
	return ""
}

// PrepareRequest injects WorkBuddy device fingerprint and client headers into the outgoing HTTP request.
func (e *WorkBuddyExecutor) PrepareRequest(req *http.Request, auth *cliproxyauth.Auth) error {
	if req == nil {
		return nil
	}
	token := workbuddyCreds(auth)
	workbuddyauth.ApplyHeaders(req, auth, token)
	helps.StripWorkBuddyForbiddenHeaders(req)
	var attrs map[string]string
	if auth != nil {
		attrs = auth.Attributes
	}
	util.ApplyCustomHeadersFromAttrs(req, attrs)
	return nil
}

// HttpRequest injects WorkBuddy credentials into the request and executes it.
func (e *WorkBuddyExecutor) HttpRequest(ctx context.Context, auth *cliproxyauth.Auth, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("workbuddy executor: request is nil")
	}
	if ctx == nil {
		ctx = req.Context()
	}
	httpReq := req.WithContext(ctx)
	if err := e.PrepareRequest(httpReq, auth); err != nil {
		return nil, err
	}
	httpClient := helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	return httpClient.Do(httpReq)
}

// Execute performs a non-streaming chat completion request to WorkBuddy.
func (e *WorkBuddyExecutor) Execute(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (resp cliproxyexecutor.Response, err error) {
	ctx = helps.EnsureSessionContext(ctx, opts, req.Payload)
	baseModel := thinking.ParseSuffix(req.Model).ModelName

	reporter := helps.NewExecutorUsageReporter(ctx, e, baseModel, auth)
	defer reporter.TrackFailure(ctx, &err)

	token := workbuddyCreds(auth)
	if token == "" {
		err = statusErr{code: http.StatusUnauthorized, msg: "missing workbuddy token"}
		return
	}

	from := opts.SourceFormat
	responseFormat := cliproxyexecutor.ResponseFormatOrSource(opts)
	to := sdktranslator.FromString("openai")

	originalPayloadSource := req.Payload
	if len(opts.OriginalRequest) > 0 {
		originalPayloadSource = opts.OriginalRequest
	}
	originalPayload := originalPayloadSource
	isCompat := helps.APIKeyModelIsCompat(req)
	originalTranslated, translated, updatesChanged, err := helps.TranslateRequestPairWithAPIKeyModelCompatibilityAndUpdateIntent(ctx, opts.Headers, e.cfg, from, to, baseModel, originalPayload, req.Payload, opts.Stream, isCompat)
	if err != nil {
		return resp, err
	}

	translated, err = helps.ApplyRequestThinking(translated, req, opts, from.String(), to.String(), e.Identifier(), updatesChanged)
	if err != nil {
		return resp, err
	}

	// Apply WorkBuddy WAF prompt sanitization and DeepSeek reasoning backfill
	translated = helps.SanitizeWorkBuddyPromptWAF(translated)
	translated, err = helps.BackfillWorkBuddyReasoningContent(translated)
	if err != nil {
		return resp, fmt.Errorf("workbuddy executor: backfill reasoning content error: %w", err)
	}

	requestedModel := helps.PayloadRequestedModel(opts, req.Model)
	requestPath := helps.PayloadRequestPath(opts)
	translated = helps.ApplyPayloadConfigWithRequest(e.cfg, baseModel, to.String(), from.String(), "", translated, originalTranslated, requestedModel, requestPath, opts.Headers)

	realm := workbuddyauth.ResolveRealm(auth)
	baseURL := workbuddyauth.ResolveBaseURL(realm)
	url := baseURL + workbuddyauth.ChatCompletionsPath

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPOST, url, bytes.NewReader(translated))
	if err != nil {
		return resp, err
	}

	if errPrep := e.PrepareRequest(httpReq, auth); errPrep != nil {
		return resp, errPrep
	}

	var authID, authLabel, authType, authValue string
	if auth != nil {
		authID = auth.ID
		authLabel = auth.Label
		authType, authValue = auth.AccountInfo()
	}
	helps.RecordAPIRequest(ctx, e.cfg, helps.UpstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      translated,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpClient = reporter.TrackHTTPClient(httpClient)
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, err)
		return resp, err
	}
	defer func() {
		_ = httpResp.Body.Close()
	}()

	helps.RecordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		b, _ := io.ReadAll(httpResp.Body)
		helps.AppendAPIResponseChunk(ctx, e.cfg, b)
		helps.LogWithRequestID(ctx).Debugf("workbuddy request error, status: %d, message: %s", httpResp.StatusCode, string(b))
		return resp, statusErr{code: httpResp.StatusCode, msg: string(b)}
	}

	data, err := io.ReadAll(httpResp.Body)
	if err != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, err)
		return resp, err
	}
	helps.AppendAPIResponseChunk(ctx, e.cfg, data)
	reporter.ObserveResponseModel(data)

	out := data
	if responseFormat != sdktranslator.FormatOpenAI {
		var param any
		out = sdktranslator.TranslateNonStream(ctx, sdktranslator.FormatOpenAI, responseFormat, req.Model, originalPayload, translated, data, &param)
	}

	if usage := helps.ParseOpenAIUsage(data); usage.TotalTokens > 0 || usage.InputTokens > 0 {
		reporter.Publish(ctx, usage)
	}

	resp = cliproxyexecutor.Response{Payload: out, Headers: httpResp.Header.Clone()}
	return resp, nil
}

// ExecuteStream performs a streaming chat completion request to WorkBuddy.
func (e *WorkBuddyExecutor) ExecuteStream(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (_ *cliproxyexecutor.StreamResult, err error) {
	ctx = helps.EnsureSessionContext(ctx, opts, req.Payload)
	baseModel := thinking.ParseSuffix(req.Model).ModelName

	reporter := helps.NewExecutorUsageReporter(ctx, e, baseModel, auth)
	defer reporter.TrackFailure(ctx, &err)

	token := workbuddyCreds(auth)
	if token == "" {
		return nil, statusErr{code: http.StatusUnauthorized, msg: "missing workbuddy token"}
	}

	from := opts.SourceFormat
	responseFormat := cliproxyexecutor.ResponseFormatOrSource(opts)
	to := sdktranslator.FromString("openai")

	originalPayloadSource := req.Payload
	if len(opts.OriginalRequest) > 0 {
		originalPayloadSource = opts.OriginalRequest
	}
	originalPayload := originalPayloadSource
	isCompat := helps.APIKeyModelIsCompat(req)
	originalTranslated, translated, updatesChanged, err := helps.TranslateRequestPairWithAPIKeyModelCompatibilityAndUpdateIntent(ctx, opts.Headers, e.cfg, from, to, baseModel, originalPayload, req.Payload, true, isCompat)
	if err != nil {
		return nil, err
	}

	translated, err = helps.ApplyRequestThinking(translated, req, opts, from.String(), to.String(), e.Identifier(), updatesChanged)
	if err != nil {
		return nil, err
	}

	translated = helps.SanitizeWorkBuddyPromptWAF(translated)
	translated, err = helps.BackfillWorkBuddyReasoningContent(translated)
	if err != nil {
		return nil, fmt.Errorf("workbuddy executor: backfill reasoning content error: %w", err)
	}

	translated = helps.SetBoolIfDifferent(translated, "stream_options.include_usage", true)
	reporter.SetTranslatedReasoningEffort(translated, to.String())

	requestedModel := helps.PayloadRequestedModel(opts, req.Model)
	requestPath := helps.PayloadRequestPath(opts)
	translated = helps.ApplyPayloadConfigWithRequest(e.cfg, baseModel, to.String(), from.String(), "", translated, originalTranslated, requestedModel, requestPath, opts.Headers)

	realm := workbuddyauth.ResolveRealm(auth)
	baseURL := workbuddyauth.ResolveBaseURL(realm)
	url := baseURL + workbuddyauth.ChatCompletionsPath

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPOST, url, bytes.NewReader(translated))
	if err != nil {
		return nil, err
	}

	if errPrep := e.PrepareRequest(httpReq, auth); errPrep != nil {
		return nil, errPrep
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")

	var authID, authLabel, authType, authValue string
	if auth != nil {
		authID = auth.ID
		authLabel = auth.Label
		authType, authValue = auth.AccountInfo()
	}
	helps.RecordAPIRequest(ctx, e.cfg, helps.UpstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      translated,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpClient = reporter.TrackHTTPClientRoundTripOnly(httpClient)
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, err)
		return nil, err
	}

	helps.RecordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		defer func() { _ = httpResp.Body.Close() }()
		b, _ := io.ReadAll(httpResp.Body)
		helps.AppendAPIResponseChunk(ctx, e.cfg, b)
		helps.LogWithRequestID(ctx).Debugf("workbuddy stream error, status: %d, message: %s", httpResp.StatusCode, string(b))
		return nil, statusErr{code: httpResp.StatusCode, msg: string(b)}
	}

	out := make(chan cliproxyexecutor.StreamChunk)
	go func() {
		defer close(out)
		defer func() { _ = httpResp.Body.Close() }()

		scanner := bufio.NewScanner(httpResp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

		streamUsage := helps.NewStreamUsageObserver(reporter)
		defer streamUsage.Flush(ctx)

		var param any
		for scanner.Scan() {
			line := scanner.Bytes()
			helps.AppendAPIResponseChunk(ctx, e.cfg, line)
			helps.AppendAPIResponseChunk(ctx, e.cfg, []byte("\n"))
			reporter.ObserveResponseModel(line)

			if bytes.HasPrefix(line, []byte("data:")) {
				dataBytes := bytes.TrimSpace(line[5:])
				if usage := helps.ParseOpenAIUsage(dataBytes); usage.TotalTokens > 0 || usage.InputTokens > 0 {
					streamUsage.Observe(usage, true)
				}
			}

			lines := sdktranslator.TranslateStream(ctx, sdktranslator.FormatOpenAI, responseFormat, req.Model, originalPayload, translated, line, &param)
			for _, converted := range lines {
				select {
				case out <- cliproxyexecutor.StreamChunk{Payload: converted}:
				case <-ctx.Done():
					return
				}
			}
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

// CountTokens estimates token count for WorkBuddy requests.
func (e *WorkBuddyExecutor) CountTokens(ctx context.Context, _ *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	baseModel := thinking.ParseSuffix(req.Model).ModelName

	from := opts.SourceFormat
	to := sdktranslator.FromString("openai")
	isCompat := helps.APIKeyModelIsCompat(req)
	translated, updatesChanged, err := helps.TranslateRequestWithAPIKeyModelCompatibilityAndUpdateIntent(ctx, opts.Headers, e.cfg, from, to, baseModel, req.Payload, false, isCompat)
	if err != nil {
		return cliproxyexecutor.Response{}, err
	}
	originalTranslatedForPayload := append([]byte(nil), translated...)
	if len(opts.OriginalRequest) > 0 {
		originalTranslatedForPayload, _, _ = helps.TranslateRequestWithAPIKeyModelCompatibilityAndUpdateIntent(ctx, opts.Headers, e.cfg, from, to, baseModel, opts.OriginalRequest, false, isCompat)
	}

	translated, err = helps.ApplyRequestThinking(translated, req, opts, from.String(), to.String(), e.Identifier(), updatesChanged)
	if err != nil {
		return cliproxyexecutor.Response{}, err
	}

	translated = helps.NewPayloadFinalizer(e.cfg, e.Identifier(), baseModel, to.String(), "", originalTranslatedForPayload, req, opts)(translated)
	enc, err := helps.TokenizerForModel(baseModel)
	if err != nil {
		return cliproxyexecutor.Response{}, fmt.Errorf("workbuddy executor: tokenizer init failed: %w", err)
	}

	count, err := helps.CountOpenAIChatTokens(enc, translated)
	if err != nil {
		return cliproxyexecutor.Response{}, fmt.Errorf("workbuddy executor: count tokens failed: %w", err)
	}
	return helps.TokenCountResponse(count)
}

// Refresh executes token refresh against WorkBuddy OAuth/refresh endpoints.
func (e *WorkBuddyExecutor) Refresh(ctx context.Context, auth *cliproxyauth.Auth) (*cliproxyauth.Auth, error) {
	log.Debugf("workbuddy executor: refresh called")
	if refreshed, handled, err := helps.RefreshAuthViaHome(ctx, e.cfg, auth); handled {
		return refreshed, err
	}
	if auth == nil {
		return nil, fmt.Errorf("workbuddy executor: auth is nil")
	}

	var refreshToken string
	if auth.Metadata != nil {
		if v, ok := auth.Metadata["refresh_token"].(string); ok && strings.TrimSpace(v) != "" {
			refreshToken = v
		}
	}
	if strings.TrimSpace(refreshToken) == "" {
		return auth, nil
	}

	realm := workbuddyauth.ResolveRealm(auth)
	uid := auth.ID
	if auth.Metadata != nil {
		if u, ok := auth.Metadata["user_id"].(string); ok && u != "" {
			uid = u
		}
	}

	httpClient := helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 30*time.Second)
	td, err := workbuddyauth.RefreshToken(ctx, httpClient, realm, refreshToken, uid)
	if err != nil {
		return nil, err
	}

	if auth.Metadata == nil {
		auth.Metadata = make(map[string]any)
	}
	auth.Token = td.AccessToken
	auth.Metadata["access_token"] = td.AccessToken
	if td.RefreshToken != "" {
		auth.Metadata["refresh_token"] = td.RefreshToken
	}
	if td.ExpiresIn > 0 {
		exp := time.Now().Add(time.Duration(td.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
		auth.Metadata["expired"] = exp
	}
	auth.Metadata["realm"] = realm
	auth.Metadata["domain"] = workbuddyauth.ResolveDomain(realm)
	auth.Metadata["base_url"] = workbuddyauth.ResolveBaseURL(realm)
	auth.LastRefreshedAt = time.Now()

	if storage, ok := auth.Storage.(*workbuddyauth.WorkBuddyTokenStorage); ok && storage != nil {
		newStorage := *storage
		newStorage.AccessToken = td.AccessToken
		if td.RefreshToken != "" {
			newStorage.RefreshToken = td.RefreshToken
		}
		if td.ExpiresIn > 0 {
			newStorage.Expired = time.Now().Add(time.Duration(td.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
		}
		newStorage.Realm = realm
		newStorage.Domain = workbuddyauth.ResolveDomain(realm)
		newStorage.BaseURL = workbuddyauth.ResolveBaseURL(realm)
		auth.Storage = &newStorage
	}

	auth.Metadata["last_refresh"] = time.Now().UTC().Format(time.RFC3339)
	return auth, nil
}
