# SDK Advanced: Custom Executors & Translators

Extend the embedded proxy with custom upstream providers, schema translators, and model catalogs using Go 1.26+ and the `/v8` module path.

## 1. Implement & Register a `ProviderExecutor`

```go
package myprov

import (
    "context"
    "net/http"

    coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
    clipexec "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

type Executor struct{}

func (Executor) Identifier() string { return "myprov" }
func (Executor) PrepareRequest(req *http.Request, a *coreauth.Auth) error { return nil }
func (Executor) Execute(ctx context.Context, a *coreauth.Auth, req clipexec.Request, opts clipexec.Options) (clipexec.Response, error) {
    return clipexec.Response{Payload: []byte(`{"ok":true}`)}, nil
}
func (Executor) ExecuteStream(ctx context.Context, a *coreauth.Auth, req clipexec.Request, opts clipexec.Options) (<-chan clipexec.StreamChunk, error) {
    ch := make(chan clipexec.StreamChunk, 1)
    go func() { defer close(ch); ch <- clipexec.StreamChunk{Payload: []byte("data: {\"done\":true}\n\n")} }()
    return ch, nil
}
func (Executor) Refresh(ctx context.Context, a *coreauth.Auth) (*coreauth.Auth, error) { return a, nil }
```

Register with `core.RegisterExecutor(myprov.Executor{})` before passing `WithCoreAuthManager(core)` to `cliproxy.NewBuilder()`.

## 2. Register Schema Translators & Models

```go
import sdktr "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"

func init() {
    sdktr.Register(sdktr.Format("openai.chat"), sdktr.Format("myprov.chat"),
        func(model string, raw []byte, stream bool) []byte { return convertOpenAIToMyProv(model, raw, stream) },
        sdktr.ResponseTransform{
            Stream:    func(ctx context.Context, model string, orig, trans, raw []byte, param *any) []string { return nil },
            NonStream: func(ctx context.Context, model string, orig, trans, raw []byte, param *any) string { return "{}" },
        },
    )
}

// Publish models to /v1/models:
cliproxy.GlobalModelRegistry().RegisterClient(authID, "myprov", []*cliproxy.ModelInfo{
    {ID: "myprov-pro-1", Object: "model", Type: "myprov", DisplayName: "MyProv Pro 1"},
})
```

## 3. Debugging & Observability

- Toggle request logs: `GET/PUT /v8/management/config/observability/logs/request-log`
- Toggle debug logs: `GET/PUT /v8/management/config/observability/logs/debug`
