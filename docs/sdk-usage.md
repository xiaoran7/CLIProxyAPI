# CLI Proxy SDK Guide (`sdk/cliproxy`)

Embed routing, authentication, hot-reload, and protocol translation directly into Go 1.26+ applications via `github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy`.

## Minimal Embed & Server Options

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
_ = svc.Run(ctx) // Watches config.yaml & auths/, refreshes tokens, and shuts down on context cancel
```

## Core Auth Manager & Custom Transports

```go
core := coreauth.NewManager(coreauth.NewFileStore(cfg.AuthDir), nil, nil)
core.SetRoundTripperProvider(myRTProvider) // Per-auth *http.Transport (e.g. ProxyURL)

// Programmatic execution:
resp, err := core.Execute(ctx, []string{"gemini"}, req, opts)
chunks, err := core.ExecuteStream(ctx, []string{"gemini"}, req, opts)
```

## Custom Credential Loaders & Management V8

- Supply custom credential sources via `WithTokenClientProvider(...)` and `WithAPIKeyClientProvider(...)`.
- Management endpoints mount under `/v8/management` when `remote-management.secret-key` is configured (`remote-management.allow-remote: true` for non-localhost). See [docs/management-api-v8.md](management-api-v8.md).
