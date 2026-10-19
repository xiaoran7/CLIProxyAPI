# SDK 高级指南：自定义执行器与翻译器

基于 Go 1.26+ 与 `/v8` 模块路径扩展自定义上游 Provider、协议转换器与模型目录。

## 1. 实现并注册 `ProviderExecutor`

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

在构建服务前通过 `core.RegisterExecutor(myprov.Executor{})` 注册，并传入 `WithCoreAuthManager(core)`。

## 2. 注册协议转换器与模型目录

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

// 发布模型至 /v1/models：
cliproxy.GlobalModelRegistry().RegisterClient(authID, "myprov", []*cliproxy.ModelInfo{
    {ID: "myprov-pro-1", Object: "model", Type: "myprov", DisplayName: "MyProv Pro 1"},
})
```

## 3. 调试与可观测性

- 切换请求日志：`GET/PUT /v8/management/config/observability/logs/request-log`
- 切换调试日志：`GET/PUT /v8/management/config/observability/logs/debug`
