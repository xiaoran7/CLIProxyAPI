# CLI Proxy API (Enhanced Fork)

[English](README.md) | 中文 | [日本語](README_JA.md)

[![Go Version](https://img.shields.io/badge/go-1.26+-00ADD8.svg?style=flat-square&logo=go)](https://golang.org)
[![Tests](https://img.shields.io/badge/tests-8444%20passed-10b981.svg?style=flat-square)](https://github.com/router-for-me/CLIProxyAPI)
[![Protocol](https://img.shields.io/badge/protocol-OpenAI%20%7C%20Claude%20%7C%20Codex-emerald.svg?style=flat-square)](llms-full.txt)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](LICENSE)
[![LLMs Index](https://img.shields.io/badge/LLMs-llms.txt-orange.svg?style=flat-square)](llms.txt)

> 基于上游 [router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 深度定制的生产级 AI 代理网关。提供 OpenAI / Claude / Gemini / Codex / Grok / Tencent WorkBuddy / OpenCode 协议互转、多策略配额调度与长连接流式防断流能力。

```text
Coding Agents (Claude Code / Codex CLI / Cursor / OpenAI & Anthropic SDKs)
  -> Gateway Layer (Auth, Body Decompression, SelectiveGzip, 15s SSE Keepalive, LoopGuard)
  -> Routing & Affinity (SessionCache [session_affinity.json], 429 Sink Pool, 5 Selectors)
  -> Canonical Pipeline (ApplyThinking -> ThinkingConfig -> Payload Semantic Barrier)
  -> Provider Executors (Claude, Codex, Gemini/Antigravity, WorkBuddy, OpenCode, Kimi, Grok)
```

## 核心特性

- **腾讯 WorkBuddy / CodeBuddy 原生双域套件**：国内版（`workbuddy-cn/`、`copilot.tencent.com`，支持 `WB_CN_CHAT_UPSTREAM` 海外出口）与国际版（`workbuddy/`）；5.8.4 会话头族与 System Prompt 伪装（防 `11128`）；强制 `stream: true` 并聚合请求（`AggregateWorkBuddyStream`，解 `11101`）；实时模型倍率（`:3.33x`）与账单计量；内置 `CheckinEngine`（`workbuddy_checkins.json`）与补偿领取。
- **OpenCode Zen 1M 免费层直通**：自动重写 `/zen/v1` 至 `/inference/v1`，自愈 `oc_sk_...` 密钥，对齐 `1.19.0` 指纹，注入合规 `bash`/`read` 工具 Schema 并剥离虚拟调用，内置 `glm-5.3-flash-free`、`deepseek-v4.1-flash-free` 等 1M 免费目录。
- **多策略配额调度与会话亲和**：支持 `round-robin`、`weighted-round-robin`、`fill-first`、`smart`（临期优先）与 `pace`（步调均衡）五种策略；跨重启会话绑定（`session_affinity.json`）与 HTTP 429 冷沉淀池。
- **流式防断流、零知识脱敏与内置控制台**：15s SSE `: keep-alive` 心跳、`LoopGuard` 熔断、`SelectiveGzipMiddleware`、确定性 HMAC-SHA256 零知识脱敏（`{{API_KEY_...}}`）与内置 `/management.html` 控制台。

## 支持的模型提供商

| 提供商 | 代表模型 | 支持通道与协议 |
| :--- | :--- | :--- |
| **Anthropic Claude** | Claude Fable 5.1, Opus 5.5, Sonnet 5.5, Haiku 5.5 | OAuth, Setup-Token, API Key, Interactions, Thinking 块绑定 |
| **OpenAI & Codex** | GPT-6 Astra, GPT-6.1 Sol, GPT-6 Luna, o3 | Codex OAuth (SIWC), API Key, Responses HTTP/WS 中继 |
| **Google Gemini** | Gemini 4 Argon, Gemini 3.8 Flash, 3.1 Pro | AI Studio, Vertex AI, Gemini CLI, Antigravity Hub 2.23.0 |
| **Tencent WorkBuddy** | DeepSeek V4.1 Flash, GLM 5.3, Kimi K3, Hy4 Preview | 国内/国际双域逆向（`workbuddy-cn/` & `workbuddy/`）、OAuth、自动打卡 |
| **OpenCode** | `glm-5.3-flash-free`, `deepseek-v4.1-flash-free` (1M) | `/inference/v1` 指纹伪装、虚拟工具过滤、免费层免配直通 |
| **Kimi / Grok / Muse / Devin** | Kimi K3, Grok 4.7, Muse Wonder/Spark 1.3, SWE-2 | OAuth、CLI 登录及通用 OpenAI 兼容端点 |

## 快速上手

```bash
gofmt -w . && go build -o cli-proxy-api ./cmd/server
./cli-proxy-api --config ./config.yaml --no-browser
```

| 配置项 / 环境变量 | 默认值 | 说明 |
| :--- | :--- | :--- |
| `routing.strategy` | `round-robin` | 调度策略：`round-robin`、`weighted-round-robin`、`fill-first`、`smart`、`pace` |
| `session-affinity` | `true` | 开启跨重启会话亲和（`session_affinity.json`），默认 `session-affinity-ttl: "1h"` |
| `models.variant-mode` | `compact` | `/v1/models` 输出模式：`compact`（仅带倍率后缀项）、`raw`、`all` |
| `global-excluded-models` | `[]` | 全局出口屏蔽模型通配符列表（从 `/v1/models` 与 `/v1beta/models` 剔除） |
| `WB_CN_CHAT_UPSTREAM` | `copilot.tencent.com` | 海外机器访问国内版 WorkBuddy 对话接口域名覆盖（如 `https://www.workbuddy.cn`） |

## 文档导航（Documentation）

- **Management V8 API 完整参考**：[docs/management-api-v8.md](docs/management-api-v8.md)（`/v8/management/config`、`/credentials`、`/workbuddy/checkin`、`/workbuddy/compensation`、`/plugins/workbuddy/quota`、`/models/global-excluded-models`、`/routing/cooldown/reset`）
- **Go SDK 接入文档**：[SDK 基础用法](docs/sdk-usage_CN.md) | [高级执行器与翻译器](docs/sdk-advanced_CN.md) | [鉴权与访问控制](docs/sdk-access_CN.md)
- **LLM 规范索引**：[llms.txt](llms.txt)（精简导航） | [llms-full.txt](llms-full.txt)（架构与协议全量参考）

## 鸣谢与许可证

基于 [router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 构建，会话亲和与零知识脱敏启发自 [Magpie](https://github.com/magpie-project)，腾讯 WorkBuddy 双域协议逆向参考自 [WorkBuddy2API-Hub](https://github.com/ardeyouxipianyi/workbuddy2api-hub)。基于 [MIT License](LICENSE) 开源发布。
