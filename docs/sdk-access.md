# `@sdk/access` Reference

`github.com/router-for-me/CLIProxyAPI/v8/sdk/access` centralizes inbound request authentication by chaining credential providers in registration order.

## Registry & Lifecycle

```go
import sdkaccess "github.com/router-for-me/CLIProxyAPI/v8/sdk/access"

manager := sdkaccess.NewManager()
manager.SetProviders(sdkaccess.RegisteredProviders())
```

- `RegisterProvider(typ, provider)`: Registers an initialized provider instance globally (preserving first-seen order).
- `SetProviders` / `Providers`: Atomically updates or snapshots the active provider chain. Calling `Authenticate` on a `nil` or empty manager returns `nil, nil` (access control disabled).
- Built-in `config-api-key` provider validates top-level `api-keys` from `Authorization: Bearer`, `X-Goog-Api-Key`, `X-Api-Key`, `?key=`, and `?auth_token=`, recording the source in `Result.Metadata["source"]`.

## Authenticating Requests & Error Semantics

```go
result, authErr := manager.Authenticate(ctx, req)
switch {
case authErr == nil:
    // Authenticated: result.Provider, result.Principal, result.Metadata
case sdkaccess.IsAuthErrorCode(authErr, sdkaccess.AuthErrorCodeNoCredentials):
    // HTTP 401: No recognizable credentials supplied
case sdkaccess.IsAuthErrorCode(authErr, sdkaccess.AuthErrorCodeInvalidCredential):
    // HTTP 401: Credentials present but rejected
default:
    // HTTP 500: AuthErrorCodeInternal transport/system failure
}
```

## Custom Providers & Service Integration

```go
type customProvider struct{}

func (p *customProvider) Identifier() string { return "custom" }
func (p *customProvider) Authenticate(ctx context.Context, r *http.Request) (*sdkaccess.Result, *sdkaccess.AuthError) {
    token := r.Header.Get("X-Custom")
    if token == "" { return nil, sdkaccess.NewNotHandledError() }
    if token != "expected" { return nil, sdkaccess.NewInvalidCredentialError() }
    return &sdkaccess.Result{Provider: "custom", Principal: "svc", Metadata: map[string]string{"source": "x-custom"}}, nil
}

func init() { sdkaccess.RegisterProvider("custom", &customProvider{}) }
```

Wire into `sdk/cliproxy` and hot-reload on config changes:

```go
svc, _ := cliproxy.NewBuilder().
    WithConfig(coreCfg).
    WithConfigPath("config.yaml").
    WithRequestAccessManager(accessManager).
    Build()

// Hot-reload config-backed providers:
configaccess.Register(&newCfg.SDKConfig)
accessManager.SetProviders(sdkaccess.RegisteredProviders())
```
