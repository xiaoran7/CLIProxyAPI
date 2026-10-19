# CLI Proxy SDK 使用指南（`sdk/cliproxy`）

通过 `github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy` 在 Go 1.26+ 服务中直接内嵌路由、鉴权、热更新与协议转换层。

## 最小嵌入与服务器可选项

```go
cfg, err := config.LoadConfig("config.yaml")
if err != nil { panic(err) }

svc, err := cliproxy.NewBuilder().
    WithConfig(cfg).
    WithConfigPath("config.yaml").
    WithServerOptions(
        cliproxy.WithMiddleware(func(c *gin.Context) { c.Header("X-Embed", "1"); c.Next() }),
        cliproxy.WithEngineConfigurator(func(e *gin.Engine) { e.ForwardedByClientIP = true }),
        cliproxy.WithRouterConfigurator(func(e *gin.Engine, _ *handlers.BaseAPIHandler, _ *config.Config) {
            e.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })
        }),
        cliproxy.WithRequestLoggerFactory(func(cfg *config.Config, cfgPath string) logging.RequestLogger {
            return logging.NewFileRequestLogger(true, "logs", filepath.Dir(cfgPath))
        }),
    ).
    WithHooks(cliproxy.Hooks{
        OnBeforeStart: func(cfg *config.Config) { log.Infof("starting on :%d", cfg.Port) },
        OnAfterStart:  func(s *cliproxy.Service) { log.Info("ready") },
    }).
    Build()
if err != nil { panic(err) }

ctx, cancel := context.WithCancel(context.Background())
defer cancel()
_ = svc.Run(ctx) // 自动监听 config.yaml 与 auths/、后台刷新令牌，取消 ctx 即优雅关闭
```

## 核心鉴权管理器与自定义传输

```go
core := coreauth.NewManager(coreauth.NewFileStore(cfg.AuthDir), nil, nil)
core.SetRoundTripperProvider(myRTProvider) // 按账户返回自定义 *http.Transport（如 ProxyURL）

// 编程式直接调用（非流式 / 流式）：
resp, err := core.Execute(ctx, []string{"gemini"}, req, opts)
chunks, err := core.ExecuteStream(ctx, []string{"gemini"}, req, opts)
```

## 自定义凭据源与管理接口

- 可通过 `WithTokenClientProvider(...)` 与 `WithAPIKeyClientProvider(...)` 接入内存或远端凭据源。
- 当 `config.yaml` 配置了 `remote-management.secret-key` 时自动挂载 `/v8/management`（跨机访问需 `remote-management.allow-remote: true`），详见 [docs/management-api-v8.md](management-api-v8.md)。
