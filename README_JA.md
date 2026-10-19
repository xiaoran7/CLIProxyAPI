# CLI Proxy API (Enhanced Fork)

[English](README.md) | [中文](README_CN.md) | 日本語

[![Go Version](https://img.shields.io/badge/go-1.26+-00ADD8.svg?style=flat-square&logo=go)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](LICENSE)
[![LLMs Index](https://img.shields.io/badge/LLMs-llms.txt-orange.svg?style=flat-square)](llms.txt)

> [router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) をベースとしたエンタープライズ向け AI プロキシゲートウェイ。OpenAI、Anthropic Claude、Google Gemini、OpenAI Codex、Grok、Tencent WorkBuddy、OpenCode 間のプロトコル変換、マルチアカウントのクォータ負荷分散、およびストリーム保護を提供します。

---

## 主な特徴

- **マルチプロバイダー統合 & デュアルレルム対応**: Claude、OpenAI/Codex、Gemini (Antigravity Hub 2.23.0)、Tencent WorkBuddy (`workbuddy/` 国際版・`workbuddy-cn/` 中国本土版)、OpenCode Zen 無料枠 (`1M` コンテキスト) を単一のエンドポイントで統合。
- **インテリジェント負荷分散 & セッションアフィニティ**: `round-robin`、`weighted-round-robin`、`fill-first`、`smart`（リセット期限優先）、`pace`（時間あたり消費ペース調整）の 5 戦略をサポート。`session_affinity.json` による永続セッション固定と HTTP 429 クールダウンシンクを実装。
- **ストリーム耐障害性 & ゼロ知識リダクション**: 15 秒ごとの SSE `: keep-alive` ハートビート、`LoopGuard` による思考ループ検知、HMAC-SHA256 プレースホルダー（`{{API_KEY_...}}`）による機密情報の自動マスキングと復元。
- **組込管理コンソール (CPAMC)**: `/management.html` および `/v8/management/*` API によるリアルタイム設定・クォータ監視。

## クイックスタート

```bash
gofmt -w .
go build -o cli-proxy-api ./cmd/server
./cli-proxy-api --config ./config.yaml --no-browser
```

詳細なアーキテクチャと API リファレンスについては、[README.md](README.md)、[docs/management-api-v8.md](docs/management-api-v8.md)、および [llms-full.txt](llms-full.txt) を参照してください。

## ライセンス

[MIT License](LICENSE) の下で公開されています。
