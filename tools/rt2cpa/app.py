#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
CPA Token Craft - Antigravity Refresh Token to CPA Auth JSON Web Service
Aozora UI (Blue-White-Sky Theme)
Author: WANGUI TANG
"""

import os
import sys
import json
import time
import datetime
import urllib.request
import urllib.parse
import urllib.error
from http.server import HTTPServer, BaseHTTPRequestHandler
from socketserver import ThreadingMixIn
import socket

class ThreadedHTTPServer(ThreadingMixIn, HTTPServer):
    daemon_threads = True

# Default configuration
DEFAULT_PORT = 51888
DEFAULT_PROXY = "http://127.0.0.1:7897"

# Antigravity official OAuth client credentials
ANTIGRAVITY_CLIENT_ID = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
ANTIGRAVITY_CLIENT_SECRET = "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"

TOKEN_ENDPOINT = "https://oauth2.googleapis.com/token"
USERINFO_ENDPOINT = "https://www.googleapis.com/oauth2/v2/userinfo?alt=json"
LOAD_CODE_ASSIST_ENDPOINT = "https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"
ONBOARD_USER_ENDPOINT = "https://daily-cloudcode-pa.googleapis.com/v1internal:onboardUser"

# Resolve project auths directory
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PROJECT_ROOT = os.path.abspath(os.path.join(SCRIPT_DIR, "..", ".."))
AUTHS_DIR = os.path.join(PROJECT_ROOT, "auths")
os.makedirs(AUTHS_DIR, exist_ok=True)


def get_opener(proxy_url=None):
    if proxy_url and proxy_url.strip():
        proxy_url = proxy_url.strip()
        handler = urllib.request.ProxyHandler({"http": proxy_url, "https": proxy_url})
        return urllib.request.build_opener(handler)
    return urllib.request.build_opener()


def make_request(url, data=None, headers=None, proxy_url=None, timeout=20):
    if headers is None:
        headers = {}
    if "User-Agent" not in headers:
        headers["User-Agent"] = "Go-http-client/2.0"

    encoded_data = None
    if data is not None:
        if isinstance(data, dict):
            if headers.get("Content-Type") == "application/json":
                encoded_data = json.dumps(data).encode("utf-8")
            else:
                encoded_data = urllib.parse.urlencode(data).encode("utf-8")
        elif isinstance(data, str):
            encoded_data = data.encode("utf-8")
        elif isinstance(data, bytes):
            encoded_data = data

    req = urllib.request.Request(url, data=encoded_data, headers=headers)
    opener = get_opener(proxy_url)
    try:
        with opener.open(req, timeout=timeout) as resp:
            content = resp.read().decode("utf-8")
            status = resp.status
            return status, content, None
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8", errors="replace")
        return e.code, err_body, f"HTTP {e.code}: {err_body}"
    except Exception as e:
        return 0, "", str(e)


def refresh_antigravity_token(refresh_token, client_id=None, client_secret=None, proxy_url=None):
    cid = (client_id or "").strip() or ANTIGRAVITY_CLIENT_ID
    csec = (client_secret or "").strip() or ANTIGRAVITY_CLIENT_SECRET
    rt = refresh_token.strip()

    data = {
        "client_id": cid,
        "client_secret": csec,
        "grant_type": "refresh_token",
        "refresh_token": rt,
    }
    headers = {
        "Content-Type": "application/x-www-form-urlencoded",
        "User-Agent": "Go-http-client/2.0",
    }

    status, body, err = make_request(TOKEN_ENDPOINT, data=data, headers=headers, proxy_url=proxy_url, timeout=20)
    if err or status != 200:
        return None, f"刷新Token失败: {err or body}"

    try:
        resp_json = json.loads(body)
        return resp_json, None
    except Exception as e:
        return None, f"解析Token响应异常: {e}"


def fetch_user_email(access_token, proxy_url=None):
    headers = {
        "Authorization": f"Bearer {access_token.strip()}",
        "User-Agent": "Go-http-client/2.0",
    }
    status, body, err = make_request(USERINFO_ENDPOINT, headers=headers, proxy_url=proxy_url, timeout=15)
    if err or status != 200:
        return "", f"获取用户信息失败: {err or body}"
    try:
        info = json.loads(body)
        return info.get("email", ""), None
    except Exception as e:
        return "", f"解析用户信息异常: {e}"


def extract_project_id_from_dict(data):
    if not isinstance(data, dict):
        return ""
    for k in ["cloudaicompanionProject", "projectId", "project"]:
        val = data.get(k)
        if isinstance(val, str) and val.strip():
            return val.strip()
        elif isinstance(val, dict):
            pid = val.get("id") or val.get("name")
            if isinstance(pid, str) and pid.strip():
                return pid.strip()
    return ""


def fetch_antigravity_project_id(access_token, proxy_url=None):
    headers = {
        "Authorization": f"Bearer {access_token.strip()}",
        "Content-Type": "application/json",
        "Accept": "*/*",
        "User-Agent": "google-cloud-code",
    }
    body = {
        "metadata": {
            "ideType": "IDE_UNSPECIFIED",
            "platform": "PLATFORM_UNSPECIFIED",
            "pluginType": "GEMINI"
        }
    }
    status, res_body, err = make_request(LOAD_CODE_ASSIST_ENDPOINT, data=body, headers=headers, proxy_url=proxy_url, timeout=20)
    if status == 200 and res_body:
        try:
            data = json.loads(res_body)
            project_id = extract_project_id_from_dict(data)
            if project_id:
                return project_id, None
        except Exception:
            pass

    # Try onboard user fallback
    onboard_body = {
        "tier_id": "free-tier",
        "metadata": {
            "ide_type": "ANTIGRAVITY"
        }
    }
    for attempt in range(2):
        st, ob_res, err2 = make_request(ONBOARD_USER_ENDPOINT, data=onboard_body, headers=headers, proxy_url=proxy_url, timeout=20)
        if st == 200 and ob_res:
            try:
                ob_data = json.loads(ob_res)
                if ob_data.get("done"):
                    resp_val = ob_data.get("response", {})
                    pid = extract_project_id_from_dict(resp_val)
                    if pid:
                        return pid, None
            except Exception:
                pass
        time.sleep(1)

    return "", None


def build_cpa_antigravity_json(refresh_token, token_resp, email="", project_id=""):
    access_token = token_resp.get("access_token", "")
    new_rt = token_resp.get("refresh_token") or refresh_token
    expires_in = int(token_resp.get("expires_in", 3599))
    now = time.time()
    now_ms = int(now * 1000)
    expire_dt = datetime.datetime.fromtimestamp(now + expires_in, datetime.timezone.utc)
    expired_str = expire_dt.strftime("%Y-%m-%dT%H:%M:%SZ")

    res = {
        "type": "antigravity",
        "access_token": access_token,
        "refresh_token": new_rt,
        "expires_in": expires_in,
        "timestamp": now_ms,
        "expired": expired_str,
    }
    if email:
        res["email"] = email
    if project_id:
        res["project_id"] = project_id

    filename = f"antigravity-{email}.json" if email else f"antigravity-{int(now)}.json"
    return res, filename


INDEX_HTML = """<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>CPA Token Craft // Google 反重力凭证工坊</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500&family=Noto+Sans+SC:wght@400;500;600;700&display=swap" rel="stylesheet">
  <style>
    :root {
      --bg-sky: #f4f8fc;
      --card-bg: rgba(255, 255, 255, 0.94);
      --card-border: rgba(220, 230, 242, 0.85);
      --primary: #0284c7;
      --primary-hover: #0369a1;
      --primary-soft: #e0f2fe;
      --accent: #38bdf8;
      --text-main: #0f172a;
      --text-muted: #64748b;
      --success: #10b981;
      --success-soft: #ecfdf5;
      --danger: #ef4444;
      --danger-soft: #fef2f2;
      --radius-sm: 8px;
      --radius-md: 14px;
      --radius-lg: 20px;
      --shadow-soft: 0 10px 30px -10px rgba(186, 215, 242, 0.45);
      --shadow-hover: 0 20px 40px -15px rgba(186, 215, 242, 0.65);
    }

    * {
      box-sizing: border-box;
      margin: 0;
      padding: 0;
    }

    body {
      font-family: 'Plus Jakarta Sans', 'Noto Sans SC', -apple-system, BlinkMacSystemFont, sans-serif;
      background: var(--bg-sky);
      min-height: 100vh;
      color: var(--text-main);
      line-height: 1.5;
      padding: 24px 16px 60px;
    }

    .bg-layer {
      position: fixed;
      top: 0;
      left: 0;
      width: 100vw;
      height: 100vh;
      pointer-events: none;
      z-index: -1;
      will-change: transform;
      transform: translateZ(0);
      background-image:
        radial-gradient(at 0% 0%, rgba(224, 242, 254, 0.8) 0px, transparent 50%),
        radial-gradient(at 100% 100%, rgba(240, 249, 255, 0.8) 0px, transparent 50%);
    }

    .container {
      max-width: 1080px;
      margin: 0 auto;
    }

    /* Top Header */
    header {
      display: flex;
      flex-wrap: wrap;
      gap: 12px;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 24px;
      padding: 12px 20px;
      background: var(--card-bg);
      border: 1px solid var(--card-border);
      border-radius: var(--radius-md);
      box-shadow: var(--shadow-soft);
    }

    .brand {
      display: flex;
      align-items: center;
      gap: 12px;
    }

    .logo-badge {
      width: 40px;
      height: 40px;
      background: linear-gradient(135deg, #0284c7, #38bdf8);
      border-radius: 10px;
      display: flex;
      align-items: center;
      justify-content: center;
      color: #fff;
      font-weight: 700;
      font-size: 18px;
      box-shadow: 0 4px 12px rgba(2, 132, 199, 0.3);
    }

    .brand-title {
      font-size: 17px;
      font-weight: 700;
      color: var(--text-main);
    }

    .brand-subtitle {
      font-size: 12px;
      color: var(--text-muted);
      letter-spacing: 0.5px;
    }

    .header-status {
      display: flex;
      align-items: center;
      gap: 16px;
      font-size: 13px;
    }

    .badge {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 4px 10px;
      border-radius: 20px;
      font-size: 12px;
      font-weight: 500;
    }

    .badge-success {
      background: var(--success-soft);
      color: var(--success);
      border: 1px solid rgba(16, 185, 129, 0.2);
    }

    .badge-primary {
      background: var(--primary-soft);
      color: var(--primary);
      border: 1px solid rgba(2, 132, 199, 0.2);
    }

    .dot {
      width: 7px;
      height: 7px;
      border-radius: 50%;
      background: currentColor;
    }

    /* Main Grid */
    .grid {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 24px;
    }

    @media (max-width: 860px) {
      .grid {
        grid-template-columns: 1fr;
      }
    }

    .card {
      background: var(--card-bg);
      border: 1px solid var(--card-border);
      border-radius: var(--radius-lg);
      padding: 24px;
      box-shadow: var(--shadow-soft);
      transition: transform 0.2s ease, box-shadow 0.2s ease;
    }

    .card:hover {
      box-shadow: var(--shadow-hover);
    }

    .card-title {
      font-size: 16px;
      font-weight: 700;
      color: var(--text-main);
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 18px;
    }

    .form-group {
      margin-bottom: 16px;
    }

    label {
      display: block;
      font-size: 13px;
      font-weight: 600;
      color: var(--text-main);
      margin-bottom: 6px;
    }

    .sub-label {
      font-size: 11px;
      color: var(--text-muted);
      font-weight: 400;
      margin-left: 6px;
    }

    textarea, input[type="text"] {
      width: 100%;
      padding: 10px 14px;
      background: #ffffff;
      border: 1px solid var(--card-border);
      border-radius: var(--radius-sm);
      font-family: 'JetBrains Mono', monospace;
      font-size: 13px;
      color: var(--text-main);
      outline: none;
      transition: border-color 0.15s ease, box-shadow 0.15s ease;
    }

    textarea:focus, input[type="text"]:focus {
      border-color: var(--primary);
      box-shadow: 0 0 0 3px rgba(2, 132, 199, 0.12);
    }

    textarea {
      resize: vertical;
      min-height: 120px;
    }

    .proxy-box {
      background: rgba(241, 245, 249, 0.6);
      border: 1px dashed var(--card-border);
      border-radius: var(--radius-sm);
      padding: 12px;
      margin-bottom: 18px;
    }

    .proxy-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 8px;
    }

    .toggle-label {
      font-size: 13px;
      font-weight: 600;
      display: flex;
      align-items: center;
      gap: 6px;
      cursor: pointer;
    }

    .btn {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
      padding: 10px 18px;
      font-size: 14px;
      font-weight: 600;
      border-radius: var(--radius-sm);
      border: none;
      cursor: pointer;
      transition: all 0.15s ease;
    }

    .btn-primary {
      background: linear-gradient(135deg, var(--primary), var(--accent));
      color: #fff;
      box-shadow: 0 4px 14px rgba(2, 132, 199, 0.35);
      width: 100%;
      padding: 12px;
      font-size: 15px;
    }

    .btn-primary:hover {
      transform: translateY(-1px);
      box-shadow: 0 6px 18px rgba(2, 132, 199, 0.45);
    }

    .btn-primary:active {
      transform: translateY(0);
    }

    .btn-primary:disabled {
      opacity: 0.6;
      cursor: not-allowed;
      transform: none;
    }

    .btn-outline {
      background: #ffffff;
      border: 1px solid var(--card-border);
      color: var(--text-main);
    }

    .btn-outline:hover {
      background: var(--primary-soft);
      color: var(--primary);
      border-color: rgba(2, 132, 199, 0.3);
    }

    .btn-sm {
      padding: 6px 12px;
      font-size: 12px;
    }

    /* Result Preview Area */
    .preview-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 12px;
    }

    .preview-actions {
      display: flex;
      gap: 8px;
    }

    .meta-cards {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 10px;
      margin-bottom: 12px;
    }

    .meta-item {
      background: #ffffff;
      border: 1px solid var(--card-border);
      border-radius: var(--radius-sm);
      padding: 10px 12px;
    }

    .meta-title {
      font-size: 11px;
      color: var(--text-muted);
      text-transform: uppercase;
      letter-spacing: 0.5px;
      margin-bottom: 2px;
    }

    .meta-val {
      font-size: 13px;
      font-weight: 600;
      color: var(--text-main);
      word-break: break-all;
    }

    pre.code-view {
      background: #0f172a;
      color: #e2e8f0;
      padding: 14px;
      border-radius: var(--radius-sm);
      font-family: 'JetBrains Mono', monospace;
      font-size: 12px;
      line-height: 1.5;
      overflow-x: auto;
      max-height: 340px;
    }

    .empty-state {
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      padding: 60px 20px;
      color: var(--text-muted);
      text-align: center;
      border: 1px dashed var(--card-border);
      border-radius: var(--radius-sm);
    }

    .empty-icon {
      font-size: 32px;
      margin-bottom: 10px;
      opacity: 0.6;
    }

    /* Auth files warehouse table */
    .warehouse-card {
      margin-top: 24px;
    }

    .auth-table {
      width: 100%;
      border-collapse: collapse;
      font-size: 13px;
    }

    .auth-table th {
      text-align: left;
      padding: 10px 12px;
      background: rgba(241, 245, 249, 0.7);
      color: var(--text-muted);
      font-weight: 600;
      border-bottom: 1px solid var(--card-border);
    }

    .auth-table td {
      padding: 10px 12px;
      border-bottom: 1px solid rgba(226, 232, 240, 0.6);
    }

    .auth-table tr:hover {
      background: rgba(248, 250, 252, 0.8);
    }

    /* Toast Notification */
    #toast {
      position: fixed;
      bottom: 24px;
      right: 24px;
      padding: 12px 20px;
      border-radius: var(--radius-sm);
      font-size: 13px;
      font-weight: 600;
      box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.15);
      z-index: 1000;
      opacity: 0;
      transform: translateY(20px);
      transition: all 0.25s ease;
      pointer-events: none;
    }

    #toast.show {
      opacity: 1;
      transform: translateY(0);
      pointer-events: auto;
    }

    .toast-success {
      background: #065f46;
      color: #ecfdf5;
    }

    .toast-error {
      background: #991b1b;
      color: #fef2f2;
    }
  </style>
</head>
<body>
  <div class="bg-layer"></div>
  <div class="container">
    <!-- Top Header -->
    <header>
      <div class="brand">
        <div class="logo-badge">✦</div>
        <div>
          <div class="brand-title">CPA Token Craft // 反重力凭证工坊</div>
          <div class="brand-subtitle">Google Antigravity RT 转 CLIProxyAPI 标准 Auth JSON</div>
        </div>
      </div>
      <div class="header-status">
        <span class="badge badge-success"><span class="dot"></span>服务在线</span>
        <span class="badge badge-primary" id="authsStatusBadge">auths/ 目录已就绪</span>
      </div>
    </header>

    <!-- Main Workspace -->
    <div class="grid">
      <!-- Left Panel: Input & Settings -->
      <div class="card">
        <div class="card-title">
          <span>反重力凭证生成器</span>
          <span style="font-size: 12px; color: var(--primary); font-weight: 500;">Provider: Antigravity</span>
        </div>

        <div class="form-group">
          <label>Google Refresh Token (RT) <span class="sub-label">支持单条或多行批量</span></label>
          <textarea id="rtInput" placeholder="在此粘贴 1//04... 或多行 Refresh Token" rows="5"></textarea>
        </div>

        <!-- Proxy Config -->
        <div class="proxy-box">
          <div class="proxy-header">
            <label class="toggle-label">
              <input type="checkbox" id="useProxy">
              <span>使用代理服务器 (海外服务器可直接直连)</span>
            </label>
            <button class="btn btn-outline btn-sm" id="testProxyBtn" type="button">测试连通性</button>
          </div>
          <input type="text" id="proxyUrl" value="" placeholder="选填，如需要可填 http://127.0.0.1:7897">
        </div>

        <!-- Advanced Client ID toggle -->
        <details style="margin-bottom: 18px; font-size: 13px;">
          <summary style="cursor: pointer; color: var(--text-muted); font-weight: 500;">高级选项 (自定义 Client ID / Secret)</summary>
          <div style="margin-top: 10px; display: grid; gap: 10px;">
            <div>
              <label style="font-size: 12px;">Client ID (留空使用官方默认)</label>
              <input type="text" id="clientIdInput" placeholder="1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com">
            </div>
            <div>
              <label style="font-size: 12px;">Client Secret (留空使用官方默认)</label>
              <input type="text" id="clientSecretInput" placeholder="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">
            </div>
          </div>
        </details>

        <button class="btn btn-primary" id="convertBtn">
          <span>⚡ 换取并生成 CPA 凭证</span>
        </button>
      </div>

      <!-- Right Panel: Output & Actions -->
      <div class="card" id="resultCard">
        <div class="preview-header">
          <div class="card-title" style="margin-bottom: 0;">凭证生成结果</div>
          <div class="preview-actions" id="actionBtns" style="display: none;">
            <button class="btn btn-outline btn-sm" id="copyJsonBtn">复制 JSON</button>
            <button class="btn btn-outline btn-sm" id="downloadJsonBtn">下载文件</button>
            <button class="btn btn-primary btn-sm" id="saveAuthBtn" style="width: auto; padding: 6px 14px;">一键存入 auths/</button>
          </div>
        </div>

        <div id="emptyPreview" class="empty-state">
          <div class="empty-icon">📂</div>
          <p>在左侧输入 Refresh Token 并点击生成</p>
          <p style="font-size: 12px; margin-top: 4px;">将自动获取 Access Token、Email 与 Project ID 并组装 JSON</p>
        </div>

        <div id="resultContent" style="display: none;">
          <div class="meta-cards">
            <div class="meta-item">
              <div class="meta-title">账号邮箱</div>
              <div class="meta-val" id="metaEmail">-</div>
            </div>
            <div class="meta-item">
              <div class="meta-title">Project ID</div>
              <div class="meta-val" id="metaProject">-</div>
            </div>
            <div class="meta-item">
              <div class="meta-title">凭证有效期</div>
              <div class="meta-val" id="metaExpire">-</div>
            </div>
            <div class="meta-item">
              <div class="meta-title">推荐文件名</div>
              <div class="meta-val" id="metaFilename" style="color: var(--primary);">-</div>
            </div>
          </div>

          <pre class="code-view"><code id="jsonCodeView"></code></pre>
        </div>
      </div>
    </div>

    <!-- Auth Warehouse Card -->
    <div class="card warehouse-card">
      <div class="card-title">
        <span>当前项目凭证仓库 (`auths/`)</span>
        <button class="btn btn-outline btn-sm" id="refreshAuthsBtn">刷新列表</button>
      </div>
      <div id="authsTableContainer">
        <p style="color: var(--text-muted); font-size: 13px;">正在加载凭证文件列表...</p>
      </div>
    </div>
  </div>

  <div id="toast"></div>

  <script>
    let currentResult = null;

    function showToast(msg, isError = false) {
      const toast = document.getElementById('toast');
      toast.textContent = msg;
      toast.className = isError ? 'show toast-error' : 'show toast-success';
      setTimeout(() => {
        toast.className = '';
      }, 3000);
    }

    // Load auth files
    async function loadAuths() {
      try {
        const res = await fetch('/api/auths');
        const data = await res.json();
        const container = document.getElementById('authsTableContainer');
        if (!data.files || data.files.length === 0) {
          container.innerHTML = '<p style="color: var(--text-muted); font-size: 13px;">暂无凭证文件。可使用上方工具生成并保存。</p>';
          return;
        }
        let html = '<table class="auth-table"><thead><tr><th>文件名</th><th>Provider</th><th>邮箱 / 标识</th><th>修改时间</th><th>大小</th></tr></thead><tbody>';
        data.files.forEach(f => {
          html += `<tr>
            <td style="font-family: monospace; font-weight: 600;">${f.name}</td>
            <td><span class="badge badge-primary">${f.type || 'unknown'}</span></td>
            <td>${f.email || '-'}</td>
            <td style="color: var(--text-muted);">${f.modified}</td>
            <td style="color: var(--text-muted);">${f.size_str}</td>
          </tr>`;
        });
        html += '</tbody></table>';
        container.innerHTML = html;
      } catch (err) {
        document.getElementById('authsTableContainer').innerHTML = '<p style="color: var(--danger); font-size: 13px;">加载凭证列表失败: ' + err.message + '</p>';
      }
    }

    // Convert Action
    document.getElementById('convertBtn').addEventListener('click', async () => {
      const rtText = document.getElementById('rtInput').value.trim();
      if (!rtText) {
        showToast('请输入 Refresh Token', true);
        return;
      }
      const useProxy = document.getElementById('useProxy').checked;
      const proxyUrl = useProxy ? document.getElementById('proxyUrl').value.trim() : '';
      const clientId = document.getElementById('clientIdInput').value.trim();
      const clientSecret = document.getElementById('clientSecretInput').value.trim();

      const btn = document.getElementById('convertBtn');
      btn.disabled = true;
      btn.innerHTML = '<span>⏳ 正在换取并抓取账号信息...</span>';

      try {
        const res = await fetch('/api/convert/antigravity', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            refresh_token: rtText,
            proxy: proxyUrl,
            client_id: clientId,
            client_secret: clientSecret
          })
        });
        const data = await res.json();
        if (!data.success) {
          throw new Error(data.error || '转换失败');
        }

        currentResult = data;
        renderResult(data);
        showToast('凭证生成成功！');
      } catch (err) {
        showToast(err.message, true);
      } finally {
        btn.disabled = false;
        btn.innerHTML = '<span>⚡ 换取并生成 CPA 凭证</span>';
      }
    });

    function renderResult(data) {
      document.getElementById('emptyPreview').style.display = 'none';
      document.getElementById('resultContent').style.display = 'block';
      document.getElementById('actionBtns').style.display = 'flex';

      document.getElementById('metaEmail').textContent = data.email || '未获取 (将自动兜底)';
      document.getElementById('metaProject').textContent = data.project_id || '自动 onboard 或未关联';
      document.getElementById('metaExpire').textContent = data.json_data.expired || '-';
      document.getElementById('metaFilename').textContent = data.filename;

      document.getElementById('jsonCodeView').textContent = JSON.stringify(data.json_data, null, 2);
    }

    // Copy JSON
    document.getElementById('copyJsonBtn').addEventListener('click', () => {
      if (!currentResult) return;
      navigator.clipboard.writeText(JSON.stringify(currentResult.json_data, null, 2));
      showToast('已复制到剪贴板！');
    });

    // Download JSON
    document.getElementById('downloadJsonBtn').addEventListener('click', () => {
      if (!currentResult) return;
      const blob = new Blob([JSON.stringify(currentResult.json_data, null, 2)], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = currentResult.filename;
      a.click();
      URL.revokeObjectURL(url);
      showToast('已触发文件下载');
    });

    // Save to auths/
    document.getElementById('saveAuthBtn').addEventListener('click', async () => {
      if (!currentResult) return;
      try {
        const res = await fetch('/api/save', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            filename: currentResult.filename,
            data: currentResult.json_data
          })
        });
        const d = await res.json();
        if (!d.success) throw new Error(d.error || '保存失败');
        showToast('成功保存至 auths/' + currentResult.filename);
        loadAuths();
      } catch (err) {
        showToast(err.message, true);
      }
    });

    // Test Proxy
    document.getElementById('testProxyBtn').addEventListener('click', async () => {
      const proxyUrl = document.getElementById('proxyUrl').value.trim();
      const btn = document.getElementById('testProxyBtn');
      btn.textContent = '测试中...';
      try {
        const res = await fetch('/api/test-proxy', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ proxy: proxyUrl })
        });
        const d = await res.json();
        if (d.success) {
          showToast('代理正常，Google 连接顺畅！');
        } else {
          showToast('代理连接失败: ' + d.error, true);
        }
      } catch (err) {
        showToast('测试异常: ' + err.message, true);
      } finally {
        btn.textContent = '测试连通性';
      }
    });

    document.getElementById('refreshAuthsBtn').addEventListener('click', loadAuths);

    // Initial load
    loadAuths();
  </script>
</body>
</html>
"""


class RequestHandler(BaseHTTPRequestHandler):
    def _send_json(self, data, status=200):
        body = json.dumps(data, ensure_ascii=False, indent=2).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Access-Control-Allow-Origin", "*")
        self.end_headers()
        self.wfile.write(body)

    def _send_html(self, html_content, status=200):
        body = html_content.encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_HEAD(self):
        parsed = urllib.parse.urlparse(self.path)
        path = parsed.path
        if path == "/" or path == "/index.html":
            body = INDEX_HTML.encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
        else:
            self.send_response(200)
            self.end_headers()

    def do_OPTIONS(self):
        self.send_response(200)
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Content-Type")
        self.end_headers()

    def do_GET(self):
        parsed = urllib.parse.urlparse(self.path)
        path = parsed.path

        if path == "/" or path == "/index.html":
            self._send_html(INDEX_HTML)
        elif path == "/api/auths":
            self.handle_list_auths()
        else:
            self.send_error(404, "Not Found")

    def do_POST(self):
        parsed = urllib.parse.urlparse(self.path)
        path = parsed.path

        length = int(self.headers.get("Content-Length", 0))
        body_bytes = self.rfile.read(length) if length > 0 else b"{}"
        try:
            body = json.loads(body_bytes.decode("utf-8"))
        except Exception:
            body = {}

        if path == "/api/convert/antigravity":
            self.handle_convert_antigravity(body)
        elif path == "/api/save":
            self.handle_save(body)
        elif path == "/api/test-proxy":
            self.handle_test_proxy(body)
        else:
            self.send_error(404, "Not Found")

    def handle_convert_antigravity(self, body):
        rt = body.get("refresh_token", "").strip()
        proxy = body.get("proxy", "").strip() or None
        client_id = body.get("client_id", "").strip() or None
        client_secret = body.get("client_secret", "").strip() or None

        if not rt:
            self._send_json({"success": False, "error": "refresh_token 不能为空"}, status=400)
            return

        # Handle potential multiple lines: take the first one or clean quotes
        lines = [line.strip().strip('"').strip("'") for line in rt.splitlines() if line.strip()]
        first_rt = lines[0] if lines else ""

        token_resp, err = refresh_antigravity_token(first_rt, client_id, client_secret, proxy)
        if err:
            self._send_json({"success": False, "error": err}, status=400)
            return

        access_token = token_resp.get("access_token", "")
        email, _ = fetch_user_email(access_token, proxy)
        project_id, _ = fetch_antigravity_project_id(access_token, proxy)

        json_data, filename = build_cpa_antigravity_json(first_rt, token_resp, email, project_id)

        self._send_json({
            "success": True,
            "email": email,
            "project_id": project_id,
            "filename": filename,
            "json_data": json_data
        })

    def handle_save(self, body):
        filename = body.get("filename", "").strip()
        data = body.get("data")
        if not filename or not data:
            self._send_json({"success": False, "error": "参数不完整"}, status=400)
            return

        # Basic path traversal protection
        filename = os.path.basename(filename)
        dest_path = os.path.join(AUTHS_DIR, filename)

        try:
            with open(dest_path, "w", encoding="utf-8") as f:
                json.dump(data, f, ensure_ascii=False, indent=2)
            self._send_json({
                "success": True,
                "saved_path": dest_path
            })
        except Exception as e:
            self._send_json({"success": False, "error": f"写入文件失败: {e}"}, status=500)

    def handle_test_proxy(self, body):
        proxy = body.get("proxy", "").strip() or None
        status, _, err = make_request("https://oauth2.googleapis.com/token", proxy_url=proxy, timeout=5)
        # Google returns 404 for empty GET /token, which proves connection succeeded
        if status in [200, 400, 404] or (err and "HTTP" in err):
            self._send_json({"success": True})
        else:
            self._send_json({"success": False, "error": err or f"状态码: {status}"})

    def handle_list_auths(self):
        files_info = []
        try:
            for fname in os.listdir(AUTHS_DIR):
                if not fname.endswith(".json"):
                    continue
                fpath = os.path.join(AUTHS_DIR, fname)
                st = os.stat(fpath)
                mod_time = datetime.datetime.fromtimestamp(st.st_mtime).strftime("%Y-%m-%d %H:%M")
                size_str = f"{st.st_size} B" if st.st_size < 1024 else f"{st.st_size / 1024:.1f} KB"

                type_val = ""
                email_val = ""
                try:
                    with open(fpath, "r", encoding="utf-8") as fp:
                        content = json.load(fp)
                        type_val = content.get("type", "")
                        email_val = content.get("email", "")
                except Exception:
                    pass

                files_info.append({
                    "name": fname,
                    "type": type_val,
                    "email": email_val,
                    "modified": mod_time,
                    "size_str": size_str
                })
        except Exception as e:
            self._send_json({"files": [], "error": str(e)})
            return

        # Sort by mtime descending
        files_info.sort(key=lambda x: x["name"])
        self._send_json({"files": files_info})


def find_free_port(start_port):
    port = start_port
    while port < start_port + 50:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
            if s.connect_ex(('127.0.0.1', port)) != 0:
                return port
        port += 1
    return start_port


def main():
    host = os.environ.get("HOST", "0.0.0.0")
    port = DEFAULT_PORT
    if len(sys.argv) > 1 and sys.argv[1].isdigit():
        port = int(sys.argv[1])
    else:
        port = find_free_port(port)

    server = ThreadedHTTPServer((host, port), RequestHandler)
    url = f"http://{host}:{port}"
    print("======================================================")
    print(f"[CPA Token Craft] Antigravity Web Helper Ready!")
    print(f"Address: {url}")
    print(f"Auths Dir: {AUTHS_DIR}")
    print("======================================================")
    sys.stdout.flush()

    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\n服务已停止。")
        server.server_close()


if __name__ == "__main__":
    main()
