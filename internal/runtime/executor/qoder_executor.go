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

	qoderauth "github.com/router-for-me/CLIProxyAPI/v8/internal/auth/qoder"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/runtime/executor/helps"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/thinking"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
)

// QoderExecutor executes chat completions against Qoder Global (api3.qoder.sh)
// and Qoder Mainland China (gateway.qoder.com.cn) backends with COSY request signing.
type QoderExecutor struct {
	cfg *config.Config
}

// NewQoderExecutor creates a new Qoder executor.
func NewQoderExecutor(cfg *config.Config) *QoderExecutor {
	return &QoderExecutor{cfg: cfg}
}

// Identifier returns the canonical executor identifier.
func (e *QoderExecutor) Identifier() string {
	return "qoder"
}

// RequestToFormat returns the intermediate format to translate inbound requests to.
func (e *QoderExecutor) RequestToFormat(_ cliproxyexecutor.Request, _ cliproxyexecutor.Options) sdktranslator.Format {
	return sdktranslator.FormatOpenAI
}

// PrepareRequest injects COSY authentication headers, machine tokens, and custom headers.
func (e *QoderExecutor) PrepareRequest(req *http.Request, auth *cliproxyauth.Auth, jobToken, requestBody string) error {
	if req == nil {
		return nil
	}
	if errSign := qoderauth.SignRequest(req, auth, jobToken, requestBody); errSign != nil {
		return fmt.Errorf("qoder executor: sign request failed: %w", errSign)
	}
	helps.StripQoderForbiddenHeaders(req)
	var attrs map[string]string
	if auth != nil {
		attrs = auth.Attributes
	}
	util.ApplyCustomHeadersFromAttrs(req, attrs)
	return nil
}

// HttpRequest injects Qoder credentials and COSY signature into the request and executes it.
func (e *QoderExecutor) HttpRequest(ctx context.Context, auth *cliproxyauth.Auth, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("qoder executor: request is nil")
	}
	if ctx == nil {
		ctx = req.Context()
	}
	httpReq := req.WithContext(ctx)

	client := helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	jobToken, errToken := qoderauth.EnsureValidJobToken(ctx, client, auth)
	if errToken != nil {
		jobToken = qoderauth.AccessToken(auth)
	}
	if jobToken == "" {
		return nil, fmt.Errorf("qoder executor: missing qoder credentials")
	}

	var bodyStr string
	if req.Body != nil {
		bodyBytes, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			return nil, fmt.Errorf("qoder executor: read request body: %w", errRead)
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		httpReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		bodyStr = string(bodyBytes)
	}

	if errPrep := e.PrepareRequest(httpReq, auth, jobToken, bodyStr); errPrep != nil {
		return nil, errPrep
	}
	return client.Do(httpReq)
}

// Execute performs a non-streaming chat completion request to Qoder.
// Upstream Qoder gateway inference is SSE-based; non-streaming requests are executed with
// stream=true and aggregated into a standard OpenAI non-streaming response.
func (e *QoderExecutor) Execute(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (resp cliproxyexecutor.Response, err error) {
	ctx = helps.EnsureSessionContext(ctx, opts, req.Payload)
	baseModel := thinking.ParseSuffix(req.Model).ModelName

	reporter := helps.NewExecutorUsageReporter(ctx, e, baseModel, auth)
	defer reporter.TrackFailure(ctx, &err)

	client := helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	jobToken, errToken := qoderauth.EnsureValidJobToken(ctx, client, auth)
	if errToken != nil {
		jobToken = qoderauth.AccessToken(auth)
	}
	if jobToken == "" {
		err = statusErr{code: http.StatusUnauthorized, msg: "missing qoder credentials"}
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

	originalTranslated, translated, updatesChanged, err := helps.TranslateRequestPairWithAPIKeyModelCompatibilityAndUpdateIntent(ctx, opts.Headers, e.cfg, from, to, baseModel, originalPayload, req.Payload, true, isCompat)
	if err != nil {
		return resp, err
	}

	translated, err = helps.ApplyRequestThinking(translated, req, opts, from.String(), to.String(), e.Identifier(), updatesChanged)
	if err != nil {
		return resp, err
	}

	translated = helps.SanitizeQoderPayload(translated)

	originalTranslated = helps.SetBoolIfDifferent(originalTranslated, "stream", true)
	translated = helps.SetBoolIfDifferent(translated, "stream", true)
	translated = helps.SetBoolIfDifferent(translated, "stream_options.include_usage", true)
	reporter.SetTranslatedReasoningEffort(translated, to.String())

	requestedModel := helps.PayloadRequestedModel(opts, req.Model)
	requestPath := helps.PayloadRequestPath(opts)
	translated = helps.ApplyPayloadConfigWithRequest(e.cfg, baseModel, to.String(), from.String(), "", translated, originalTranslated, requestedModel, requestPath, opts.Headers)
	// Transport framing strictly requires stream=true
	translated = helps.SetBoolIfDifferent(translated, "stream", true)

	realm := qoderauth.ResolveRealm(auth)
	baseURL := qoderauth.ResolveBaseURL(realm)
	if auth != nil && auth.Attributes != nil {
		if bu := strings.TrimSpace(auth.Attributes["base_url"]); bu != "" {
			baseURL = strings.TrimRight(bu, "/")
		}
	}
	url := baseURL + qoderauth.ChatCompletionsPath

	bodyStr := string(translated)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(translated))
	if err != nil {
		return resp, err
	}

	if errPrep := e.PrepareRequest(httpReq, auth, jobToken, bodyStr); errPrep != nil {
		return resp, errPrep
	}
	httpReq.Header.Set("Content-Type", "application/json")
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

	httpClient := reporter.TrackHTTPClient(client)
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
		helps.LogWithRequestID(ctx).Debugf("qoder request error, status: %d, message: %s", httpResp.StatusCode, string(b))
		return resp, statusErr{code: httpResp.StatusCode, msg: string(b)}
	}

	data, err := io.ReadAll(httpResp.Body)
	if err != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, err)
		return resp, err
	}
	helps.AppendAPIResponseChunk(ctx, e.cfg, data)

	aggregated := data
	if bytes.Contains(data, []byte("data:")) {
		agg, errAgg := helps.AggregateQoderStream(data, baseModel)
		if errAgg != nil {
			helps.RecordAPIResponseError(ctx, e.cfg, errAgg)
			return resp, fmt.Errorf("qoder executor: aggregate stream failed: %w", errAgg)
		}
		aggregated = agg
	}
	reporter.ObserveResponseModel(aggregated)

	out := aggregated
	if responseFormat != sdktranslator.FormatOpenAI {
		var param any
		out = sdktranslator.TranslateNonStream(ctx, sdktranslator.FormatOpenAI, responseFormat, req.Model, originalPayload, translated, aggregated, &param)
	}

	if usage := helps.ParseOpenAIUsage(aggregated); usage.TotalTokens > 0 || usage.InputTokens > 0 {
		reporter.Publish(ctx, usage)
	}

	resp = cliproxyexecutor.Response{Payload: out, Headers: httpResp.Header.Clone()}
	return resp, nil
}

// ExecuteStream performs a streaming chat completion request to Qoder.
func (e *QoderExecutor) ExecuteStream(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (_ *cliproxyexecutor.StreamResult, err error) {
	ctx = helps.EnsureSessionContext(ctx, opts, req.Payload)
	baseModel := thinking.ParseSuffix(req.Model).ModelName

	reporter := helps.NewExecutorUsageReporter(ctx, e, baseModel, auth)
	defer reporter.TrackFailure(ctx, &err)

	client := helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	jobToken, errToken := qoderauth.EnsureValidJobToken(ctx, client, auth)
	if errToken != nil {
		jobToken = qoderauth.AccessToken(auth)
	}
	if jobToken == "" {
		return nil, statusErr{code: http.StatusUnauthorized, msg: "missing qoder credentials"}
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

	translated = helps.SanitizeQoderPayload(translated)

	originalTranslated = helps.SetBoolIfDifferent(originalTranslated, "stream", true)
	translated = helps.SetBoolIfDifferent(translated, "stream", true)
	translated = helps.SetBoolIfDifferent(translated, "stream_options.include_usage", true)
	reporter.SetTranslatedReasoningEffort(translated, to.String())

	requestedModel := helps.PayloadRequestedModel(opts, req.Model)
	requestPath := helps.PayloadRequestPath(opts)
	translated = helps.ApplyPayloadConfigWithRequest(e.cfg, baseModel, to.String(), from.String(), "", translated, originalTranslated, requestedModel, requestPath, opts.Headers)
	// Transport framing strictly requires stream=true
	translated = helps.SetBoolIfDifferent(translated, "stream", true)

	realm := qoderauth.ResolveRealm(auth)
	baseURL := qoderauth.ResolveBaseURL(realm)
	if auth != nil && auth.Attributes != nil {
		if bu := strings.TrimSpace(auth.Attributes["base_url"]); bu != "" {
			baseURL = strings.TrimRight(bu, "/")
		}
	}
	url := baseURL + qoderauth.ChatCompletionsPath

	bodyStr := string(translated)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(translated))
	if err != nil {
		return nil, err
	}

	if errPrep := e.PrepareRequest(httpReq, auth, jobToken, bodyStr); errPrep != nil {
		return nil, errPrep
	}
	httpReq.Header.Set("Content-Type", "application/json")
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

	httpClient := reporter.TrackHTTPClientRoundTripOnly(client)
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
		helps.LogWithRequestID(ctx).Debugf("qoder stream error, status: %d, message: %s", httpResp.StatusCode, string(b))
		return nil, statusErr{code: httpResp.StatusCode, msg: string(b)}
	}

	out := make(chan cliproxyexecutor.StreamChunk)
	go func() {
		defer close(out)
		defer func() { _ = httpResp.Body.Close() }()

		scanner := bufio.NewScanner(httpResp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

		var streamUsage helps.StreamUsageBuffer
		defer streamUsage.Publish(ctx, reporter)

		var param any
		for scanner.Scan() {
			line := scanner.Bytes()
			helps.AppendAPIResponseChunk(ctx, e.cfg, line)
			helps.AppendAPIResponseChunk(ctx, e.cfg, []byte("\n"))
			reporter.ObserveResponseModel(line)

			streamUsage.ObserveOpenAIStream(line)

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

// CountTokens estimates token count for Qoder requests.
func (e *QoderExecutor) CountTokens(ctx context.Context, _ *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
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
		return cliproxyexecutor.Response{}, fmt.Errorf("qoder executor: tokenizer init failed: %w", err)
	}

	count, err := helps.CountOpenAIChatTokens(enc, translated)
	if err != nil {
		return cliproxyexecutor.Response{}, fmt.Errorf("qoder executor: count tokens failed: %w", err)
	}
	usageJSON := helps.BuildOpenAIUsageJSON(count)
	responseFormat := cliproxyexecutor.ResponseFormatOrSource(opts)
	translatedUsage := sdktranslator.TranslateTokenCount(ctx, to, responseFormat, count, usageJSON)
	return cliproxyexecutor.Response{Payload: translatedUsage}, nil
}

// Refresh executes token refresh against Qoder endpoints (deviceToken refresh or jobToken exchange).
func (e *QoderExecutor) Refresh(ctx context.Context, auth *cliproxyauth.Auth) (*cliproxyauth.Auth, error) {
	if refreshed, handled, err := helps.RefreshAuthViaHome(ctx, e.cfg, auth); handled {
		return refreshed, err
	}
	if auth == nil {
		return nil, fmt.Errorf("qoder executor: auth is nil")
	}

	client := helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 30*time.Second)
	realm := qoderauth.ResolveRealm(auth)

	// 1. If PAT exists, exchange/refresh job token
	if pat := qoderauth.PersonalAccessToken(auth); pat != "" {
		resp, errExchange := qoderauth.ExchangeJobToken(ctx, client, realm, pat)
		if errExchange != nil {
			return nil, fmt.Errorf("qoder executor: refresh job token via PAT failed: %w", errExchange)
		}
		if auth.Metadata == nil {
			auth.Metadata = make(map[string]any)
		}
		auth.Metadata["job_token"] = resp.JobToken
		auth.Metadata["access_token"] = resp.JobToken
		if resp.RefreshToken != "" {
			auth.Metadata["job_refresh"] = resp.RefreshToken
		}
		auth.LastRefreshedAt = time.Now()
		return auth, nil
	}

	// 2. If OAuth job_refresh exists, rotate job token
	if jrt := qoderauth.JobRefreshToken(auth); jrt != "" {
		resp, errJob := qoderauth.RefreshJobToken(ctx, client, realm, jrt)
		if errJob == nil && resp != nil && strings.TrimSpace(resp.JobToken) != "" {
			if auth.Metadata == nil {
				auth.Metadata = make(map[string]any)
			}
			auth.Metadata["job_token"] = resp.JobToken
			auth.Metadata["access_token"] = resp.JobToken
			if resp.RefreshToken != "" {
				auth.Metadata["job_refresh"] = resp.RefreshToken
			}
			if resp.ExpiresIn > 0 {
				var expSec int64
				if resp.ExpiresIn > 1000000 {
					expSec = resp.ExpiresIn / 1000
				} else {
					expSec = resp.ExpiresIn
				}
				expTime := time.Now().Add(time.Duration(expSec) * time.Second)
				auth.Metadata["expired"] = expTime.UTC().Format(time.RFC3339)
				auth.Metadata["expires_at"] = expTime.Unix()
			}
			auth.LastRefreshedAt = time.Now()
			return auth, nil
		}
	}

	// 3. If OAuth device_refresh exists, rotate device token and attempt job token trade
	if drt := qoderauth.DeviceRefreshToken(auth); drt != "" {
		resp, errDev := qoderauth.RefreshDeviceToken(ctx, client, realm, drt)
		if errDev != nil {
			return nil, fmt.Errorf("qoder executor: refresh device token failed: %w", errDev)
		}
		if auth.Metadata == nil {
			auth.Metadata = make(map[string]any)
		}
		auth.Metadata["device_token"] = resp.Token
		if resp.RefreshToken != "" {
			auth.Metadata["device_refresh"] = resp.RefreshToken
			auth.Metadata["refresh_token"] = resp.RefreshToken
		}
		activeToken := resp.Token
		if jtResp, errTrade := qoderauth.TradeJobToken(ctx, client, realm, resp.Token); errTrade == nil && jtResp != nil && strings.TrimSpace(jtResp.JobToken) != "" {
			auth.Metadata["job_token"] = jtResp.JobToken
			activeToken = jtResp.JobToken
			if jtResp.RefreshToken != "" {
				auth.Metadata["job_refresh"] = jtResp.RefreshToken
			}
			if jtResp.ExpiresIn > 0 {
				var expSec int64
				if jtResp.ExpiresIn > 1000000 {
					expSec = jtResp.ExpiresIn / 1000
				} else {
					expSec = jtResp.ExpiresIn
				}
				expTime := time.Now().Add(time.Duration(expSec) * time.Second)
				auth.Metadata["expired"] = expTime.UTC().Format(time.RFC3339)
				auth.Metadata["expires_at"] = expTime.Unix()
			}
		}
		auth.Metadata["access_token"] = activeToken
		auth.LastRefreshedAt = time.Now()
		return auth, nil
	}

	return auth, nil
}
