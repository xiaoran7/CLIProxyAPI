# `@sdk/access` 开发指引

`github.com/router-for-me/CLIProxyAPI/v8/sdk/access` 负责入站访问认证，按注册顺序串联多种凭证校验实现。

## 注册表与生命周期

```go
import sdkaccess "github.com/router-for-me/CLIProxyAPI/v8/sdk/access"

manager := sdkaccess.NewManager()
manager.SetProviders(sdkaccess.RegisteredProviders())
```

- `RegisterProvider(typ, provider)`：全局注册已初始化的 Provider 实例（保持首次注册顺序）。
- `SetProviders` / `Providers`：防御性拷贝更新或获取当前 Provider 快照；管理器为空或未配置 Provider 时返回 `nil, nil`（视为关闭访问控制）。
- 内置 `config-api-key`：校验 `config.yaml` 顶层 `api-keys`（支持 `Authorization: Bearer`、`X-Goog-Api-Key`、`X-Api-Key`、`?key=`、`?auth_token=`），并在 `Result.Metadata["source"]` 记录来源。

## 请求认证与错误语义

```go
result, authErr := manager.Authenticate(ctx, req)
switch {
case authErr == nil:
    // 认证通过：result.Provider, result.Principal, result.Metadata
case sdkaccess.IsAuthErrorCode(authErr, sdkaccess.AuthErrorCodeNoCredentials):
    // HTTP 401：未提供或未识别到凭证
case sdkaccess.IsAuthErrorCode(authErr, sdkaccess.AuthErrorCodeInvalidCredential):
    // HTTP 401：凭证存在但校验失败
default:
    // HTTP 500：AuthErrorCodeInternal 内部或传输错误
}
```

## 自定义 Provider 与服务集成

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

嵌入 `sdk/cliproxy` 及配置热更新：

```go
svc, _ := cliproxy.NewBuilder().
    WithConfig(coreCfg).
    WithConfigPath("config.yaml").
    WithRequestAccessManager(accessManager).
    Build()

// 配置变更时热刷新：
configaccess.Register(&newCfg.SDKConfig)
accessManager.SetProviders(sdkaccess.RegisteredProviders())
```
