# AGENTS.md

## 工作约定

- **Git 与协作**：修改前先跑 `git status --short` 保留其他会话改动；改动必同步更新 `CHANGELOG.md`；提交、推送、部署按用户授权分步执行。
- **代码与文档语言**：遵循 KISS；Go 注释仅用英文（编辑时顺手将非英文注释译为英文），用户可见文案保持原语种；文档用 UTF-8 无 BOM 纯 LF。
- **Payload 最终语义屏障**：所有执行器（流式/WS/续写/重试/图像/计费）必须先完成内置转换、归一化、注入与清理，最后仅执行一次用户 Payload 规则；其后仅允许传输帧封装（如 WorkBuddy `stream: true`）、序列化与签名，改动构建路径必补单测。
- **错误处理与日志**：遵循 `gofmt`/goimports；HTTP Handler 禁 `panic` 或 `log.Fatal`；用 `logrus` 脱敏记录，遮蔽变量加方法后缀（`errStart`），包装 `defer Close()` 错误。
- **网络超时与确定性测试**：仅获取凭据阶段可设超时，上游建连后禁设超时（例外：`codex_websockets_executor.go`、`wsrelay/session.go`、`api_tools.go`、`cmd/fetch_antigravity_models`）；TTL/缓存单测禁用 `time.Sleep`，统一用可控时钟（`nowFunc`）。

## 项目边界

- **上游与逆向权威源**：Go 1.26+ 多协议 AI 代理网关（上游 `router-for-me/CLIProxyAPI`）；WorkBuddy 双域协议、会话头族、WAF 规则与伪装以 `ardeyouxipianyi/workbuddy2api-hub` 为第一参考源。
- **编译与测试门禁**：改动后必跑 `gofmt -w .` 与 `go build -o test-output ./cmd/server && rm test-output`；测试用 `go test ./...` 或 `go test -v -run TestName ./path/to/pkg`。
- **核心架构约束**：`internal/thinking/` 走标准管道（`suffix.go` → `ThinkingConfig` → `validate/convert` → `ProviderApplier`）；`internal/runtime/executor/` 仅放执行器与单测，辅助函数放 `helps/`；禁单独改 `internal/translator/`；`/v0/management` 已废弃；改 `internal/home/` 需同步 `CLIProxyAPIHome`。
- **跨仓静态同步红线**：`static/management.html`（及 `internal/managementasset/`）修改后必须立即同步至 `Project/gate/static/management.html` 并校验 MD5。
- **配置与深入文档**：默认加载 `config.yaml` 与 `.env`，凭据存 `auths/`。深入契约见 [docs/management-api-v8.md](docs/management-api-v8.md)、[docs/sdk-usage.md](docs/sdk-usage.md) 与 [llms-full.txt](llms-full.txt)。
