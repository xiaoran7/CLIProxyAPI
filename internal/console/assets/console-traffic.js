/**
 * Console 工作台 - Visual Gateway (可视化网关) 核心交互模块
 * 负责 Client IP 识别、4 阶段流向拓扑渲染、活跃 IP 台账与实时调用明细流水。
 */

let cachedTrafficData = null;
let trafficSelectedIP = '';
let trafficSelectedModel = '';
let trafficSelectedStatus = 'all';
let trafficSearchQuery = '';
let trafficTimer = null;
let trafficPollingInterval = 3; // 默认 3s 轮询

// 1. 获取并渲染可视化网关数据
async function fetchTrafficData(force = false) {
  try {
    const res = await fetch('/console/api/traffic', {
      headers: getAuthHeaders()
    });
    if (!res.ok) return;
    const data = await res.json();
    cachedTrafficData = data;

    // 更新侧边栏导航徽章
    const badge = document.getElementById('nav-badge-traffic');
    if (badge && data.summary) {
      badge.textContent = String(data.summary.active_ips || 0);
    }

    renderTrafficUI(data);
  } catch (err) {
    console.error('Failed to fetch visual gateway traffic data:', err);
  }
}

// 2. 渲染完整可视化网关面板
function renderTrafficUI(data) {
  if (!data) return;
  const summary = data.summary || { active_ips: 0, requests_5m: 0, avg_latency_ms: 0, success_rate: 0, total_recorded: 0 };

  // 更新指标卡
  const elIps = document.getElementById('traffic-val-ips');
  const elReqs = document.getElementById('traffic-val-reqs');
  const elLat = document.getElementById('traffic-val-latency');
  const elSucc = document.getElementById('traffic-val-success');
  const elTotal = document.getElementById('traffic-val-total');

  if (elIps) elIps.innerHTML = `${summary.active_ips || 0}<small>个活跃 IP</small>`;
  if (elReqs) elReqs.innerHTML = `${summary.requests_5m || 0}<small>次 / 5min</small>`;
  if (elLat) elLat.innerHTML = `${Math.round(summary.avg_latency_ms || 0)}<small>ms</small>`;
  if (elSucc) elSucc.innerHTML = `${((summary.success_rate || 0) * 100).toFixed(1)}<small>%</small>`;
  if (elTotal) elTotal.textContent = `环形缓冲累计记录: ${summary.total_recorded || 0} 条`;

  // 渲染拓扑与台账
  renderTrafficTopology(data);
  renderTrafficIPLedger(data.ip_stats || []);
  renderTrafficTable(data.records || data.recent_requests || []);
  updateTrafficFilterDropdowns(data.records || data.recent_requests || []);
}

// 3. 动态渲染轻量交互式 SVG 拓扑图 (Client IP -> Model -> Channel -> Status)
function renderTrafficTopology(data) {
  const container = document.getElementById('traffic-topology-canvas-container');
  if (!container) return;

  const records = data.records || data.recent_requests || [];
  if (records.length === 0) {
    container.innerHTML = `
      <div style="padding:48px 16px;text-align:center;color:var(--text-muted);">
        <div style="font-size:36px;margin-bottom:8px;">🕸️</div>
        <div style="font-size:14px;font-weight:600;color:var(--text-main);margin-bottom:4px;">暂无实时流量记录</div>
        <div style="font-size:12px;">向网关发起请求后，流向感知拓扑图将在此动态呈现。</div>
      </div>
    `;
    return;
  }

  const topology = data.topology || { nodes: [], links: [] };
  const colIPs = (topology.nodes || []).filter(n => n.type === 'client_ip').slice(0, 8);
  const colModels = (topology.nodes || []).filter(n => n.type === 'model').slice(0, 8);
  const colChannels = (topology.nodes || []).filter(n => n.type === 'channel').slice(0, 8);
  const colStatuses = (topology.nodes || []).filter(n => n.type === 'status').slice(0, 8);

  const maxNodes = Math.max(colIPs.length, colModels.length, colChannels.length, colStatuses.length, 1);
  const svgHeight = Math.max(260, maxNodes * 46 + 40);
  const nodeW = 140;
  const nodeH = 28;
  const nodePositions = {};

  const calcPositions = (items, x) => {
    const count = items.length;
    const step = (svgHeight - 40) / Math.max(1, count);
    for (let i = 0; i < count; i++) {
      const item = items[i];
      const yPos = 30 + i * step + (step - nodeH) / 2;
      nodePositions[item.id] = { x, y: yPos, item };
    }
  };

  calcPositions(colIPs, 20);
  calcPositions(colModels, 260);
  calcPositions(colChannels, 500);
  calcPositions(colStatuses, 740);

  // 构建 Links SVG
  let linksSvg = '';
  for (const link of (topology.links || [])) {
    const p1 = nodePositions[link.source];
    const p2 = nodePositions[link.target];
    if (!p1 || !p2) continue;
    const x1 = p1.x + nodeW;
    const y1 = p1.y + nodeH / 2;
    const x2 = p2.x;
    const y2 = p2.y + nodeH / 2;
    const dx = (x2 - x1) * 0.5;
    const d = `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`;
    const isErr = link.target.includes('status:4') || link.target.includes('status:5');
    const strokeColor = isErr ? 'rgba(239, 68, 68, 0.45)' : 'rgba(99, 102, 241, 0.4)';
    const strokeW = Math.max(1.5, Math.min(8, Math.sqrt(link.value) * 1.5));
    linksSvg += `<path d="${d}" fill="none" stroke="${strokeColor}" stroke-width="${strokeW}" />`;
  }

  // 构建 Nodes SVG
  let nodesSvg = '';
  for (const [id, pos] of Object.entries(nodePositions)) {
    const item = pos.item;
    const isIP = item.type === 'client_ip';
    const isModel = item.type === 'model';
    const isChannel = item.type === 'channel';
    const isStatus = item.type === 'status';

    let borderColor = isIP ? 'rgba(6, 182, 212, 0.6)' : isModel ? 'rgba(139, 92, 246, 0.6)' : isChannel ? 'rgba(245, 158, 11, 0.6)' : 'rgba(16, 185, 129, 0.6)';
    let bgFill = isIP ? 'rgba(6, 182, 212, 0.12)' : isModel ? 'rgba(139, 92, 246, 0.12)' : isChannel ? 'rgba(245, 158, 11, 0.12)' : 'rgba(16, 185, 129, 0.12)';

    if (isStatus && (item.label.includes('4xx') || item.label.includes('5xx'))) {
      borderColor = 'rgba(239, 68, 68, 0.7)';
      bgFill = 'rgba(239, 68, 68, 0.15)';
    }

    const shortLabel = item.label.length > 15 ? item.label.slice(0, 14) + '…' : item.label;
    const clickHandler = isIP ? `onclick="filterTrafficByIP('${escapeHtml(item.label)}')" style="cursor:pointer;"` : '';

    nodesSvg += `
      <g transform="translate(${pos.x}, ${pos.y})" ${clickHandler}>
        <rect width="${nodeW}" height="${nodeH}" rx="6" fill="${bgFill}" stroke="${borderColor}" stroke-width="1.2" />
        <text x="8" y="18" fill="var(--text-main, #0f172a)" font-size="11" font-weight="600">${escapeHtml(shortLabel)}</text>
        <rect x="${nodeW - 32}" y="6" width="26" height="16" rx="8" fill="rgba(255, 255, 255, 0.15)" />
        <text x="${nodeW - 19}" y="18" fill="var(--text-main, #0f172a)" font-size="9.5" font-weight="700" text-anchor="middle">${item.count}</text>
      </g>
    `;
  }

  container.innerHTML = `
    <svg viewBox="0 0 920 ${svgHeight}" style="width:100%;min-width:860px;height:${svgHeight}px;display:block;">
      <text x="20" y="18" fill="#06b6d4" font-size="11" font-weight="700">① 客户端真实 IP (CLIENT IP)</text>
      <text x="260" y="18" fill="#8b5cf6" font-size="11" font-weight="700">② 请求模型 (MODEL)</text>
      <text x="500" y="18" fill="#f59e0b" font-size="11" font-weight="700">③ 映射上游渠道 (CHANNEL)</text>
      <text x="740" y="18" fill="#10b981" font-size="11" font-weight="700">④ 响应状态 (STATUS)</text>
      <g>${linksSvg}</g>
      <g>${nodesSvg}</g>
    </svg>
  `;
}

// 4. 渲染多用户 IP 统计台账
function renderTrafficIPLedger(ipStats) {
  const container = document.getElementById('traffic-ip-ledger-grid');
  if (!container) return;

  if (ipStats.length === 0) {
    container.innerHTML = `<div style="color:var(--text-muted);font-size:12px;padding:12px 0;">暂无多用户客户端统计</div>`;
    return;
  }

  container.innerHTML = ipStats.map(s => {
    const isSelected = trafficSelectedIP === s.ip;
    const timeStr = s.last_seen ? new Date(s.last_seen * 1000).toLocaleTimeString() : '-';
    const modelsBadge = (s.top_models || []).map(m => `<span style="font-size:10px;padding:1px 6px;border-radius:4px;background:rgba(139,92,246,0.12);color:#8b5cf6;border:1px solid rgba(139,92,246,0.25);">${escapeHtml(m)}</span>`).join(' ');

    return `
      <div class="traffic-ip-pill ${isSelected ? 'active' : ''}" onclick="filterTrafficByIP('${escapeHtml(s.ip)}')">
        <div style="display:flex;justify-content:space-between;align-items:center;">
          <span style="font-family:var(--mono, monospace);font-weight:700;font-size:13px;color:${isSelected ? '#06b6d4' : 'inherit'};">${escapeHtml(s.ip)}</span>
          <span style="font-size:11px;color:var(--text-muted);">${timeStr}</span>
        </div>
        <div style="display:flex;gap:12px;font-size:11.5px;margin:6px 0;color:var(--text-muted);">
          <span>请求: <strong style="color:#0284c7;">${s.request_count}</strong></span>
          <span>Tokens: <strong style="color:#7c3aed;">${(s.tokens || 0).toLocaleString()}</strong></span>
        </div>
        <div style="display:flex;gap:4px;flex-wrap:wrap;margin-top:4px;">${modelsBadge}</div>
      </div>
    `;
  }).join('');
}

// 5. 渲染实时请求明细流水表格
function renderTrafficTable(records) {
  const tbody = document.getElementById('traffic-tbody');
  const countEl = document.getElementById('traffic-records-count');
  if (!tbody) return;

  const filtered = records.filter(r => {
    if (trafficSelectedIP && r.client_ip !== trafficSelectedIP) return false;
    if (trafficSelectedModel && r.model !== trafficSelectedModel) return false;
    if (trafficSelectedStatus !== 'all') {
      if (trafficSelectedStatus === '2xx' && (r.status < 200 || r.status >= 300)) return false;
      if (trafficSelectedStatus === '4xx' && (r.status < 400 || r.status >= 500)) return false;
      if (trafficSelectedStatus === '5xx' && r.status < 500) return false;
    }
    if (trafficSearchQuery) {
      const q = trafficSearchQuery.toLowerCase();
      const mIP = r.client_ip && r.client_ip.toLowerCase().includes(q);
      const mModel = r.model && r.model.toLowerCase().includes(q);
      const mKey = r.key_label && r.key_label.toLowerCase().includes(q);
      const mChan = r.channel && r.channel.toLowerCase().includes(q);
      if (!mIP && !mModel && !mKey && !mChan) return false;
    }
    return true;
  });

  if (countEl) {
    countEl.textContent = `展示 ${filtered.length} / ${records.length} 条`;
  }

  if (filtered.length === 0) {
    tbody.innerHTML = `<tr><td colspan="8" style="text-align:center;padding:32px;color:var(--text-muted);">无匹配的流量记录</td></tr>`;
    return;
  }

  tbody.innerHTML = filtered.slice(0, 100).map(r => {
    const isSuccess = r.status >= 200 && r.status < 300;
    const is4xx = r.status >= 400 && r.status < 500;
    const statusBg = isSuccess ? 'rgba(16, 185, 129, 0.12)' : is4xx ? 'rgba(245, 158, 11, 0.12)' : 'rgba(239, 68, 68, 0.12)';
    const statusColor = isSuccess ? '#059669' : is4xx ? '#d97706' : '#dc2626';
    const timeStr = r.timestamp ? new Date(r.timestamp * 1000).toLocaleTimeString() : '-';
    const modelDisplay = (r.request_model && r.request_model !== r.model)
      ? `<span style="opacity:0.75;" title="客户端请求模型: ${escapeHtml(r.request_model)}">${escapeHtml(r.request_model)}</span> <span style="color:var(--text-muted);">➔</span> <strong>${escapeHtml(r.model || '-')}</strong>`
      : escapeHtml(r.model || '-');

    return `
      <tr style="border-bottom:1px solid var(--border-subtle, rgba(0,0,0,0.05));">
        <td style="padding:8px 10px;font-family:var(--mono, monospace);color:var(--text-muted);">${timeStr}</td>
        <td style="padding:8px 10px;">
          <span onclick="filterTrafficByIP('${escapeHtml(r.client_ip)}')" style="cursor:pointer;font-family:var(--mono, monospace);padding:2px 6px;border-radius:4px;background:rgba(6,182,212,0.1);color:#0891b2;border:1px solid rgba(6,182,212,0.25);" title="点击筛选此 IP">${escapeHtml(r.client_ip || '-')}</span>
        </td>
        <td style="padding:8px 10px;color:var(--text-muted);">${escapeHtml(r.key_label || '-')}</td>
        <td style="padding:8px 10px;"><span style="padding:2px 6px;border-radius:4px;background:rgba(139,92,246,0.1);color:#7c3aed;font-size:11px;border:1px solid rgba(139,92,246,0.2);">${modelDisplay}</span></td>
        <td style="padding:8px 10px;"><span style="padding:2px 6px;border-radius:4px;background:rgba(245,158,11,0.1);color:#d97706;font-size:11px;border:1px solid rgba(245,158,11,0.2);">${escapeHtml(r.channel || '-')}</span></td>
        <td style="padding:8px 10px;"><span style="padding:2px 6px;border-radius:4px;background:${statusBg};color:${statusColor};font-weight:600;font-size:11px;">${r.status || 200}</span></td>
        <td style="padding:8px 10px;font-family:var(--mono, monospace);">${r.latency_ms || 0}ms</td>
        <td style="padding:8px 10px;font-family:var(--mono, monospace);color:#7c3aed;">${(r.tokens || 0).toLocaleString()}</td>
      </tr>
    `;
  }).join('');
}

// 6. 更新下拉筛选菜单
function updateTrafficFilterDropdowns(records) {
  const ipSelect = document.getElementById('traffic-select-ip');
  const modelSelect = document.getElementById('traffic-select-model');
  if (!ipSelect || !modelSelect) return;

  const currentIP = trafficSelectedIP;
  const currentModel = trafficSelectedModel;

  const ipSet = new Set();
  const modelSet = new Set();
  for (const r of records) {
    if (r.client_ip) ipSet.add(r.client_ip);
    if (r.model) modelSet.add(r.model);
  }

  const ips = Array.from(ipSet).sort();
  const models = Array.from(modelSet).sort();

  ipSelect.innerHTML = `<option value="">全部客户端 IP</option>` + ips.map(ip => `<option value="${escapeHtml(ip)}" ${ip === currentIP ? 'selected' : ''}>${escapeHtml(ip)}</option>`).join('');
  modelSelect.innerHTML = `<option value="">全部请求模型</option>` + models.map(m => `<option value="${escapeHtml(m)}" ${m === currentModel ? 'selected' : ''}>${escapeHtml(m)}</option>`).join('');
}

// 7. 交互筛选回调
function filterTrafficByIP(ip) {
  trafficSelectedIP = (trafficSelectedIP === ip) ? '' : ip;
  const ipSelect = document.getElementById('traffic-select-ip');
  if (ipSelect) ipSelect.value = trafficSelectedIP;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData);
}

function onTrafficIPChange(val) {
  trafficSelectedIP = val;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData);
}

function onTrafficModelChange(val) {
  trafficSelectedModel = val;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData);
}

function onTrafficStatusChange(val) {
  trafficSelectedStatus = val;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData);
}

function onTrafficSearchChange(val) {
  trafficSearchQuery = val.trim();
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData);
}

function resetTrafficFilters() {
  trafficSelectedIP = '';
  trafficSelectedModel = '';
  trafficSelectedStatus = 'all';
  trafficSearchQuery = '';

  const ipSel = document.getElementById('traffic-select-ip');
  const mSel = document.getElementById('traffic-select-model');
  const stSel = document.getElementById('traffic-select-status');
  const sInput = document.getElementById('traffic-search-input');

  if (ipSel) ipSel.value = '';
  if (mSel) mSel.value = '';
  if (stSel) stSel.value = 'all';
  if (sInput) sInput.value = '';

  if (cachedTrafficData) renderTrafficUI(cachedTrafficData);
}

function setTrafficPollingRate(rate) {
  trafficPollingInterval = Number(rate);
  if (trafficTimer) {
    clearInterval(trafficTimer);
    trafficTimer = null;
  }
  if (trafficPollingInterval > 0) {
    trafficTimer = setInterval(() => {
      if (typeof activeNavTab !== 'undefined' && activeNavTab === 'traffic') {
        fetchTrafficData();
      }
    }, trafficPollingInterval * 1000);
  }
}

// 页面初始化时挂载定时轮询
document.addEventListener('DOMContentLoaded', () => {
  setTrafficPollingRate(3);
});
