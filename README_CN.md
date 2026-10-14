# CLI Proxy API (Enhanced Fork)

[English](README.md) | 中文 | [日本語](README_JA.md)

> 本仓库是基于上游官方 [router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 深度定制、架构加固与生产级增强的高性能 AI 代理中继服务器。
> 致力于为企业与开发者提供 OpenAI / Anthropic Claude / Google Gemini / OpenAI Codex / Grok / WorkBuddy / OpenCode 等全系大模型与代码智能体（Coding Agents）的协议统一转换、多账号高并发调度与生产级运维能力。

---

## 核心特性与 Fork 增强概览

我们在保留上游原生高性能代理与跨协议转换优势的基础上，围绕**智能体防断流**、**多策略配额均衡调度**、**逆向协议深度纳管（WorkBuddy & OpenCode）**以及**开箱即用可视化运维（内置定制版 CPAMC）**完成了一系列重大架构升级：

### 1. 深度逆向接入与国产代码智能体支持

- **腾讯 WorkBuddy / CodeBuddy 原生双区域全流程纳管**：
  - **双区域自动识别**：原生支持国内版（`copilot.tencent.com`）与国际版（`workbuddy.ai`）的独立域名解析、端点路由与凭据转换；
  - **WAF 指令清洗与特征伪装**：内建确定性设备指纹算法（`X-Machine-ID`、`X-Session-ID`、`X-IDE-Name`、`X-Domain`），实现真实 IDE 指纹级的 WAF 指令清洗与请求伪装；
  - **自动定时打卡引擎（CheckinEngine）**：内建北京时间自动签到与领点数后台守护协程，支持连签状态记录、自动防重、停电重启持久化存储（`workbuddy_checkins.json`）；
  - **运维级一键启停开关**：支持通过管理面板或 REST API（`PUT /v8/management/workbuddy/checkin`）动态热切换自动签到引擎运行状态；
  - **点数配额智能回退**：上游常规点数接口 404 时自动回退至活动账单计量接口，精准获取当季可用积分（如 200 点）与当期主题活动状态；
  - **原生单飞 Token 保活**：采用 `singleflight` 机制并发抑制 WorkBuddy 刷新令牌请求，避免高并发下多线程击穿刷新失效；
  - **一键式交互 OAuth 授权登录**：支持在终端直接调用交互式微信/企业微信二维码扫码登录，自动生成结构化持久凭据。

- **OpenCode 原生协议与 1M 免费模型零配置接入**：
  - **协议指纹深度伪装**：逆向并集成 OpenCode 专用反爬拦截规则，自动补全调用方请求头与虚拟工具提示词；
  - **免费模型零配置就绪**：原生直通 OpenCode 1M 上下文旗舰与免费层模型（`step-5-preview-free`、`longcat-2.5-preview-free`、`mimo-v2.6-flash-free`、`ling-3.1-flash-free` 等）；
  - **顶级提供商呈现**：在 CPAMC 管理控制台将其作为一等公民提供商（Provider）直观管理。

### 2. 企业级生产负载均衡与会话亲和

- **五大负载均衡与调度算法**：
  - **轮询（Round-Robin）**：标准无状态平权分配；
  - **加权轮询（Weighted Round-Robin）**：依据账号配额与权重自适应调度；
  - **首满优先（Fill-First）**：充分压榨单个账号直至耗尽，保护其余备用账号；
  - **临期优先（Smart Quota Routing）**：基于账号额度重置倒计时调度，优先消耗当前刷新周期即将截止的账号额度，实现资源最大化利用；
  - **步调均衡（Pace Quota Routing）**：结合窗口期剩余时间与已消耗速率进行动态削峰填谷，平滑分配请求，杜绝月初/周初暴饮暴食。
- **跨重启持久化会话亲和（Persistent Session Affinity）**：
  - 会话亲和映射记录持久化于 `session_affinity.json`，服务热重启或故障恢复后无缝继承长会话绑定；
  - 采用滑动窗口 TTL（Sliding-window TTL），持续活跃会话自动延寿，最大程度保障 Claude Code / Codex 长上下文 Prompt 缓存命中率；
  - 配合 **429 Rate-Limit 动态冷沉淀池**，当亲和账号遭遇上游限流时自动将其打入降温冷却队列，请求平滑重试至健康候选账号。

### 3. 长连接智能体高可用流式护航

- **流式循环守护与熔断熔解（Loop Guard Circuit Breaker）**：
  - 针对大模型长思考（Thinking/Reasoning）或重复死循环吐词场景，实现自适应流式循环熔断保护，防止卡死连接耗尽上游 Token 与客户端带宽。
- **SSE 长连接心跳保活（Keepalive Pumper）**：
  - 针对长时间生成但暂无内容输出的流式空档期，定时注入合规 SSE 注释帧心跳（`: keep-alive`），杜绝 Cloudflare、Nginx、Gate 或边缘 CDN 在 60s/100s 触发超时断开（HTTP 524 / 504）。
- **零知识审计与日志脱敏（Zero-Knowledge Redaction）**：
  - 日志与审计追踪全面脱敏，敏感 Authorization、Cookie、第三方 Token 与 Prompt 隐私正文绝不泄露到普通日志中。

### 4. 开箱即用内置管理面板（CPAMC v1.26.2 深度定制版）

- **完全嵌入式静态托管**：二进制与静态目录内嵌开箱即用的 Web 管理面板，零额外依赖直出；
- **WorkBuddy 专属管控套件**：
  - 独立双区域展示（国内版 / 国际版）；
  - 一键拉取各账号当前点数余额与免费层状态；
  - 今日签到状态与连签天数看板；
  - 自动打卡后台引擎运行状态实时徽标与动态启停按钮；
- **凭据管理安全性加固**：
  - 严格隔离底层持久化文件（`workbuddy_checkins.json`、`session_affinity.json` 等），防止内部数据文件被误识别为第三方未知账号或遭意外下载/删除；
  - 全局退出模型（Exit Models）可视化排除与 Master Key 多租户标签过滤。

---

## 支持的模型提供商（Providers）

| 提供商 | 说明 | 支持通道 / 协议 |
| :--- | :--- | :--- |
| **Anthropic Claude** | Claude Fable 5.1, Claude Opus 5.5, Claude Sonnet 5.5, Claude Haiku 5.5 | OAuth, Setup-Token, API Key, Interactions, Codex 协议映射 |
| **OpenAI & Codex** | GPT-6 Astra, GPT-6.1 Sol, GPT-6 Luna, o3, o1-preview | Codex OAuth, API Key, Responses, Stream/WS Relay |
| **Google Gemini** | Gemini 3.8 Flash, Gemini 3.1 Pro Preview, 2.5 Flash / Pro | AI Studio, Vertex AI, Gemini CLI, Antigravity 账号 |
| **Tencent WorkBuddy** | DeepSeek V4.1 Flash (1M), GLM 5.3, Kimi K3, Hy4 Preview | 国内版 Copilot 与国际版 WorkBuddy 双域逆向、交互 OAuth、自动打卡 |
| **OpenCode** | `step-5-preview-free`, `longcat-2.5-preview-free` 等 1M 免费模型 | 原生 OpenCode 逆向协议、防 WAF 指纹清洗、免配直通 |
| **Kimi (Moonshot)** | Kimi K3 (2.8T 3T-class 开源旗舰), K2.8 Preview | OAuth, Kimi Code 订阅, 开放平台 API Key |
| **xAI Grok** | Grok 4.7, Grok 4.7 Build Fast | Grok Build OAuth, xAI API Key |
| **Meta Muse** | Muse Spark 1.3, Muse Spark 1.2 | Meta 登录认证, Meta Model API Key |
| **Devin** | SWE-2, GPT-6 Astra, Claude Fable 5.1 | `--devin-login` 账户逆向路由 |
| **第三方 OpenAI 兼容** | 任意标准 OpenAI / Claude 兼容上游（DeepSeek、OpenRouter、SiliconFlow 等） | 配置文件自由声明与映射 |

---

## 快速上手

### 1. 编译构建

本项目使用 Go 1.26+ 开发，支持纯静态跨平台编译：

```bash
# 代码格式化
gofmt -w .

# 本地快速构建
go build -o cli-proxy-api ./cmd/server

# 跨平台构建 Linux amd64 静态无依赖二进制
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o cli-proxy-api-linux ./cmd/server
```

### 2. 运行与启动

默认载入当前目录下的 `config.yaml`（模板参考 `config.example.yaml`）和 `.env`：

```bash
# 直接启动
./cli-proxy-api

# 启动带控制台 TUI 界面
./cli-proxy-api --tui

# 禁用浏览器自动打开，指定配置文件
./cli-proxy-api --config ./config.yaml --no-browser
```

### 3. Docker 部署

```yaml
services:
  cliproxyapi:
    image: cliproxyapi:v8.0.28-unified
    container_name: cliproxyapi
    restart: unless-stopped
    ports:
      - "8315:8317"
    volumes:
      - ./config.yaml:/CLIProxyAPI/config.yaml
      - ./auths:/root/.cli-proxy-api
      - ./logs:/CLIProxyAPI/logs
      - ./plugins:/CLIProxyAPI/plugins
      - ./data:/CLIProxyAPI/data
      - ./static:/CLIProxyAPI/static
```

---

## 常用管理接口（Management V8 API）

| 方法 | 路径 | 说明 |
| :--- | :--- | :--- |
| `GET` | `/v8/management/credentials` | 列出已纳管凭据（自动排除系统存储文件） |
| `GET` | `/v8/management/workbuddy/checkin` | 查询今日打卡看板、各账号打卡记录与引擎当前开关状态 |
| `PUT` | `/v8/management/workbuddy/checkin` | 动态热切换自动签到引擎开关（`{"enabled": false/true}`） |
| `POST` | `/v8/management/workbuddy/checkin` | 触发指定账号或全量账号立即打卡 / 切换状态 |
| `GET` | `/v8/management/plugins/workbuddy/quota` | 批量拉取 WorkBuddy 真实可用点数与活动 VIP 方案名 |
| `GET` | `/v8/management/config` | 查看或动态热更新 CPA 核心配置 |
| `POST` | `/v8/management/routing/cooldown/reset` | 紧急重置上游冷却熔断池与配额限制 |

---

## 鸣谢（Acknowledgements）

特别鸣谢以下优秀开源项目为本 Fork 提供的设计思路、协议探索与架构启发：

- **[router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)**：上游核心代理框架与跨模型协议转换基石；
- **[Magpie](https://github.com/magpie-project)**：持久化会话亲和、滑动窗口 TTL、429 冷沉淀池及零知识审计脱敏机制的设计启发；
- **[WorkBuddy](https://github.com/workbuddy)**：腾讯 Copilot/CodeBuddy 双域逆向分析、计量协议、WAF 指令特征及签到工作流的深度探索。

---

## 许可证（License）

本项目延续开源协议，基于 [MIT License](LICENSE) 授权发布。
