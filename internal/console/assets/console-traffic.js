/**
 * Gate Admin Console - Visual Gateway (Magpie Architecture)
 * 纯轻量交互式 SVG 拓扑流向图、多用户 Client IP 台账与全链路追踪
 */

let cachedTrafficData = null;
let trafficSelectedIP = '';
let trafficSelectedModel = '';
let trafficSelectedStatus = 'all';
let trafficSearchQuery = '';
let trafficTimer = null;
let trafficPollingInterval = 3;
let trafficHighlightedNode = null; // 选中高亮的拓扑节点 ID

// SVG 图标库 (Lucide 规范，1.5px 极细线条单色矢量，杜绝原生 Emoji)
const TRAFFIC_ICONS = {
  globe: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/></svg>',
  activity: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg>',
  zap: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>',
  shield: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>',
  refresh: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M3 21v-5h5"/></svg>',
  layers: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/></svg>',
  search: '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>',
  arrowRight: '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12 5 19 12 12 19"/></svg>'
};

// 1. 获取并渲染可视化网关数据
async function fetchTrafficData(force = false) {
  try {
    const res = await fetch('/console/api/traffic', {
      headers: getAuthHeaders()
    });
    if (!res.ok) return;
    const data = await res.json();
    cachedTrafficData = data;

    // 更新侧边栏导航徽章 (活跃客户端 IP 数)
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
  if (elReqs) elReqs.innerHTML = `${summary.requests_5m || 0}<small>次 / 5m</small>`;
  if (elLat) elLat.innerHTML = `${Math.round(summary.avg_latency_ms || 0)}<small>ms</small>`;
  if (elSucc) elSucc.innerHTML = `${((summary.success_rate || 0) * 100).toFixed(1)}<small>%</small>`;
  if (elTotal) elTotal.textContent = `环形缓冲累计: ${summary.total_recorded || 0} 条`;

  // 渲染拓扑、IP台账与流水明细
  renderTrafficTopology(data);
  renderTrafficIPLedger(data.ip_stats || []);
  renderTrafficTable(data.records || data.recent_requests || []);
  updateTrafficFilterDropdowns(data.records || data.recent_requests || []);
}

// 3. 动态渲染 Magpie 级 4 阶段交互式 SVG 拓扑图 (Client IP ➔ Model ➔ Channel ➔ Status)
function renderTrafficTopology(data) {
  const container = document.getElementById('traffic-topology-canvas-container');
  if (!container) return;

  const records = data.records || data.recent_requests || [];
  if (records.length === 0) {
    container.innerHTML = `
      <div style="padding:48px 16px;text-align:center;color:var(--text-muted);">
        <div style="display:inline-flex;padding:12px;border-radius:12px;background:rgba(59,130,246,0.06);color:#3b82f6;margin-bottom:12px;">
          ${TRAFFIC_ICONS.layers}
        </div>
        <div style="font-size:14px;font-weight:600;color:var(--text-main);margin-bottom:4px;">暂无实时流量记录</div>
        <div style="font-size:12px;color:var(--text-muted);max-width:420px;margin:0 auto;">网关正在实时侦听多用户调用，产生真实请求后全链路流向图将在此实时呈现。</div>
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
  const svgHeight = Math.max(260, maxNodes * 48 + 48);
  const nodeW = 144;
  const nodeH = 30;
  const nodePositions = {};

  const calcPositions = (items, x) => {
    const count = items.length;
    const step = (svgHeight - 48) / Math.max(1, count);
    for (let i = 0; i < count; i++) {
      const item = items[i];
      const yPos = 36 + i * step + (step - nodeH) / 2;
      nodePositions[item.id] = { x, y: yPos, item };
    }
  };

  calcPositions(colIPs, 24);
  calcPositions(colModels, 268);
  calcPositions(colChannels, 512);
  calcPositions(colStatuses, 756);

  // 判定是否激活链路高亮 (严格依阶段有向追踪，杜绝共享 Status:2xx 汇聚节点横向泄露)
  const isFiltered = Boolean(trafficHighlightedNode);
  const activeLinks = new Set();
  const activeNodes = new Set();

  if (isFiltered) {
    activeNodes.add(trafficHighlightedNode);
    const allLinks = topology.links || [];

    if (trafficHighlightedNode.startsWith('ip:')) {
      const activeModels = new Set();
      const activeChannels = new Set();
      for (const link of allLinks) {
        if (link.source === trafficHighlightedNode) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.target);
          activeModels.add(link.target);
        }
      }
      for (const link of allLinks) {
        if (activeModels.has(link.source)) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.target);
          activeChannels.add(link.target);
        }
      }
      for (const link of allLinks) {
        if (activeChannels.has(link.source)) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.target);
        }
      }
    } else if (trafficHighlightedNode.startsWith('model:')) {
      const activeIPs = new Set();
      const activeChannels = new Set();
      for (const link of allLinks) {
        if (link.target === trafficHighlightedNode) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.source);
          activeIPs.add(link.source);
        } else if (link.source === trafficHighlightedNode) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.target);
          activeChannels.add(link.target);
        }
      }
      for (const link of allLinks) {
        if (activeChannels.has(link.source)) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.target);
        }
      }
    } else if (trafficHighlightedNode.startsWith('channel:')) {
      const activeModels = new Set();
      for (const link of allLinks) {
        if (link.target === trafficHighlightedNode) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.source);
          activeModels.add(link.source);
        } else if (link.source === trafficHighlightedNode) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.target);
        }
      }
      for (const link of allLinks) {
        if (activeModels.has(link.target)) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.source);
        }
      }
    } else if (trafficHighlightedNode.startsWith('status:')) {
      const activeChannels = new Set();
      const activeModels = new Set();
      for (const link of allLinks) {
        if (link.target === trafficHighlightedNode) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.source);
          activeChannels.add(link.source);
        }
      }
      for (const link of allLinks) {
        if (activeChannels.has(link.target)) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.source);
          activeModels.add(link.source);
        }
      }
      for (const link of allLinks) {
        if (activeModels.has(link.target)) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.source);
        }
      }
    }
  }

  // 构建 Links SVG (Smooth Bezier Curves)
  let linksSvg = '';
  for (const link of (topology.links || [])) {
    const p1 = nodePositions[link.source];
    const p2 = nodePositions[link.target];
    if (!p1 || !p2) continue;

    const linkKey = `${link.source}|${link.target}`;
    const isLinkActive = !isFiltered || activeLinks.has(linkKey);
    const x1 = p1.x + nodeW;
    const y1 = p1.y + nodeH / 2;
    const x2 = p2.x;
    const y2 = p2.y + nodeH / 2;
    const dx = (x2 - x1) * 0.52;
    const d = `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`;

    const isErr = link.target.includes('status:4') || link.target.includes('status:5');
    let strokeColor = isErr ? 'rgba(239, 68, 68, 0.45)' : 'rgba(59, 130, 246, 0.38)';
    let strokeW = Math.max(1.5, Math.min(7, Math.sqrt(link.value) * 1.5));

    if (isFiltered && isLinkActive) {
      strokeColor = isErr ? '#ef4444' : '#06b6d4';
      strokeW = Math.max(2.5, strokeW + 1.2);
    }

    const pathClass = `traffic-link-path ${isLinkActive ? 'is-active' : 'is-dimmed'}`;
    linksSvg += `<path d="${d}" fill="none" stroke="${strokeColor}" stroke-width="${strokeW}" class="${pathClass}" />`;
  }

  // 构建 Nodes SVG
  let nodesSvg = '';
  for (const [id, pos] of Object.entries(nodePositions)) {
    const item = pos.item;
    const isIP = item.type === 'client_ip';
    const isModel = item.type === 'model';
    const isChannel = item.type === 'channel';
    const isStatus = item.type === 'status';

    const isNodeActive = !isFiltered || activeNodes.has(id);
    const isNodeSelected = trafficHighlightedNode === id;

    let borderColor = isIP ? 'rgba(6, 182, 212, 0.45)' : isModel ? 'rgba(139, 92, 246, 0.45)' : isChannel ? 'rgba(245, 158, 11, 0.45)' : 'rgba(16, 185, 129, 0.45)';
    let bgFill = isIP ? 'rgba(6, 182, 212, 0.08)' : isModel ? 'rgba(139, 92, 246, 0.08)' : isChannel ? 'rgba(245, 158, 11, 0.08)' : 'rgba(16, 185, 129, 0.08)';

    if (isStatus && (item.label.includes('4xx') || item.label.includes('5xx'))) {
      borderColor = 'rgba(239, 68, 68, 0.55)';
      bgFill = 'rgba(239, 68, 68, 0.12)';
    }

    const shortLabel = item.label.length > 15 ? item.label.slice(0, 14) + '…' : item.label;
    const groupClass = `traffic-node-group ${isNodeActive ? '' : 'is-dimmed'} ${isNodeSelected ? 'is-selected' : ''}`;

    nodesSvg += `
      <g transform="translate(${pos.x}, ${pos.y})" class="${groupClass}" onclick="toggleHighlightTrafficNode('${escapeHtml(id)}')">
        <rect class="node-box" width="${nodeW}" height="${nodeH}" rx="8" fill="${bgFill}" stroke="${borderColor}" stroke-width="1.2" />
        <text x="10" y="19" fill="var(--text-main, #0f172a)" font-size="11" font-weight="600" font-family="var(--mono, monospace)">${escapeHtml(shortLabel)}</text>
        <rect x="${nodeW - 32}" y="6" width="24" height="18" rx="9" fill="rgba(15, 23, 42, 0.08)" />
        <text x="${nodeW - 20}" y="19" fill="var(--text-main, #0f172a)" font-size="9.5" font-weight="700" text-anchor="middle" font-family="var(--mono, monospace)">${item.count}</text>
      </g>
    `;
  }

  container.innerHTML = `
    <svg viewBox="0 0 924 ${svgHeight}" style="width:100%;min-width:880px;height:${svgHeight}px;display:block;">
      <defs>
        <linearGradient id="grad-active-link" x1="0%" y1="0%" x2="100%" y2="0%">
          <stop offset="0%" stop-color="#06b6d4" />
          <stop offset="100%" stop-color="#3b82f6" />
        </linearGradient>
      </defs>
      <!-- Column Stage Headers -->
      <text x="24" y="20" fill="#0891b2" font-size="11" font-weight="700" letter-spacing="0.04em">STAGE 1 · CLIENT IP</text>
      <text x="268" y="20" fill="#7c3aed" font-size="11" font-weight="700" letter-spacing="0.04em">STAGE 2 · REQUEST MODEL</text>
      <text x="512" y="20" fill="#d97706" font-size="11" font-weight="700" letter-spacing="0.04em">STAGE 3 · ROUTED CHANNEL</text>
      <text x="756" y="20" fill="#059669" font-size="11" font-weight="700" letter-spacing="0.04em">STAGE 4 · OUTCOME STATUS</text>
      <g>${linksSvg}</g>
      <g>${nodesSvg}</g>
    </svg>
  `;
}

// 4. 拓扑节点交互高亮追踪
function toggleHighlightTrafficNode(nodeId) {
  if (trafficHighlightedNode === nodeId) {
    trafficHighlightedNode = null;
    if (nodeId.startsWith('ip:')) {
      trafficSelectedIP = '';
      const ipSelect = document.getElementById('traffic-select-ip');
      if (ipSelect) ipSelect.value = '';
    }
  } else {
    trafficHighlightedNode = nodeId;
    // 如果点击的是 client_ip 节点，同步筛选明细列表
    if (nodeId.startsWith('ip:')) {
      const ip = nodeId.slice(3);
      trafficSelectedIP = ip;
      const ipSelect = document.getElementById('traffic-select-ip');
      if (ipSelect) ipSelect.value = ip;
    }
  }
  if (cachedTrafficData) {
    renderTrafficUI(cachedTrafficData);
  }
}

// 5. 渲染多用户 IP 统计台账 (带活跃呼吸微点)
function renderTrafficIPLedger(ipStats) {
  const container = document.getElementById('traffic-ip-ledger-grid');
  if (!container) return;

  if (ipStats.length === 0) {
    container.innerHTML = `<div style="color:var(--text-muted);font-size:12px;padding:12px 0;">暂无活跃客户端 IP 记录</div>`;
    return;
  }

  container.innerHTML = ipStats.map(s => {
    const isSelected = trafficSelectedIP === s.ip;
    const timeStr = s.last_seen ? new Date(s.last_seen * 1000).toLocaleTimeString() : '-';
    const modelsBadge = (s.top_models || []).map(m => `<span class="traffic-pill traffic-pill-model">${escapeHtml(m)}</span>`).join(' ');

    return `
      <div class="traffic-ip-card ${isSelected ? 'is-active' : ''}" onclick="filterTrafficByIP('${escapeHtml(s.ip)}')">
        <div style="display:flex;justify-content:space-between;align-items:center;">
          <div style="display:flex;align-items:center;gap:7px;">
            <span class="traffic-pulse-dot"></span>
            <span style="font-family:var(--mono, monospace);font-weight:700;font-size:13px;color:${isSelected ? '#0891b2' : 'var(--text-main)'};">${escapeHtml(s.ip)}</span>
          </div>
          <span style="font-size:11px;color:var(--text-muted);">${timeStr}</span>
        </div>
        <div style="display:flex;gap:14px;font-size:11.5px;margin:4px 0;color:var(--text-muted);">
          <span>请求: <strong style="color:var(--text-main);font-family:var(--mono, monospace);">${s.request_count}</strong></span>
          <span>Tokens: <strong style="color:#7c3aed;font-family:var(--mono, monospace);">${(s.tokens || 0).toLocaleString()}</strong></span>
        </div>
        <div style="display:flex;gap:4px;flex-wrap:wrap;margin-top:2px;">${modelsBadge}</div>
      </div>
    `;
  }).join('');
}

// 6. 渲染实时调用流水明细表格
function renderTrafficTable(records) {
  const tbody = document.getElementById('traffic-tbody');
  const countEl = document.getElementById('traffic-records-count');
  if (!tbody) return;

  const filtered = records.filter(r => {
    if (trafficSelectedIP && r.client_ip !== trafficSelectedIP) return false;
    if (trafficSelectedModel && r.model !== trafficSelectedModel && r.request_model !== trafficSelectedModel) return false;
    if (trafficSelectedStatus !== 'all') {
      if (trafficSelectedStatus === '2xx' && (r.status < 200 || r.status >= 300)) return false;
      if (trafficSelectedStatus === '4xx' && (r.status < 400 || r.status >= 500)) return false;
      if (trafficSelectedStatus === '5xx' && r.status < 500) return false;
    }
    if (trafficSearchQuery) {
      const q = trafficSearchQuery.toLowerCase();
      const mIP = r.client_ip && r.client_ip.toLowerCase().includes(q);
      const mModel = r.model && r.model.toLowerCase().includes(q);
      const mReqModel = r.request_model && r.request_model.toLowerCase().includes(q);
      const mKey = r.key_label && r.key_label.toLowerCase().includes(q);
      const mChan = r.channel && r.channel.toLowerCase().includes(q);
      if (!mIP && !mModel && !mReqModel && !mKey && !mChan) return false;
    }
    return true;
  });

  if (countEl) {
    countEl.textContent = `展示 ${filtered.length} / ${records.length} 条`;
  }

  if (filtered.length === 0) {
    tbody.innerHTML = `<tr><td colspan="8" style="text-align:center;padding:36px;color:var(--text-muted);font-size:12.5px;">无匹配的调用记录</td></tr>`;
    return;
  }

  tbody.innerHTML = filtered.slice(0, 100).map(r => {
    const isSuccess = r.status >= 200 && r.status < 300;
    const is4xx = r.status >= 400 && r.status < 500;
    const statusClass = isSuccess ? 'traffic-pill-success' : is4xx ? 'traffic-pill-warning' : 'traffic-pill-danger';
    const timeStr = r.timestamp ? new Date(r.timestamp * 1000).toLocaleTimeString() : '-';

    // 如果客户端请求模型与实际路由执行模型不同，展示真实透传流向
    const modelHtml = (r.request_model && r.request_model !== r.model)
      ? `<span style="opacity:0.8;" title="客户端请求模型: ${escapeHtml(r.request_model)}">${escapeHtml(r.request_model)}</span> <span style="color:var(--text-muted);">${TRAFFIC_ICONS.arrowRight}</span> <strong style="color:#7c3aed;">${escapeHtml(r.model || '-')}</strong>`
      : `<span style="font-weight:600;">${escapeHtml(r.model || '-')}</span>`;

    return `
      <tr style="border-bottom:1px solid var(--border-subtle, rgba(0,0,0,0.05));">
        <td style="padding:10px 12px;font-family:var(--mono, monospace);color:var(--text-muted);">${timeStr}</td>
        <td style="padding:10px 12px;">
          <span class="traffic-pill traffic-pill-ip" onclick="filterTrafficByIP('${escapeHtml(r.client_ip)}')" title="点击筛选此 IP">${escapeHtml(r.client_ip || '-')}</span>
        </td>
        <td style="padding:10px 12px;color:var(--text-muted);font-size:11.5px;">${escapeHtml(r.key_label || '-')}</td>
        <td style="padding:10px 12px;"><span class="traffic-pill traffic-pill-model">${modelHtml}</span></td>
        <td style="padding:10px 12px;"><span class="traffic-pill traffic-pill-channel">${escapeHtml(r.channel || '-')}</span></td>
        <td style="padding:10px 12px;"><span class="traffic-pill ${statusClass}">${r.status || 200}</span></td>
        <td style="padding:10px 12px;font-family:var(--mono, monospace);font-variant-numeric:tabular-nums;color:${r.latency_ms > 3000 ? '#d97706' : 'inherit'};">${r.latency_ms || 0}ms</td>
        <td style="padding:10px 12px;font-family:var(--mono, monospace);font-variant-numeric:tabular-nums;color:#7c3aed;">${(r.tokens || 0).toLocaleString()}</td>
      </tr>
    `;
  }).join('');
}

// 7. 更新下拉筛选菜单
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
    if (r.request_model) modelSet.add(r.request_model);
  }

  const ips = Array.from(ipSet).sort();
  const models = Array.from(modelSet).sort();

  ipSelect.innerHTML = `<option value="">全部客户端 IP</option>` + ips.map(ip => `<option value="${escapeHtml(ip)}" ${ip === currentIP ? 'selected' : ''}>${escapeHtml(ip)}</option>`).join('');
  modelSelect.innerHTML = `<option value="">全部请求模型</option>` + models.map(m => `<option value="${escapeHtml(m)}" ${m === currentModel ? 'selected' : ''}>${escapeHtml(m)}</option>`).join('');
}

// 8. 交互过滤操作
function filterTrafficByIP(ip) {
  trafficSelectedIP = (trafficSelectedIP === ip) ? '' : ip;
  trafficHighlightedNode = trafficSelectedIP ? `ip:${trafficSelectedIP}` : null;
  const ipSelect = document.getElementById('traffic-select-ip');
  if (ipSelect) ipSelect.value = trafficSelectedIP;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData);
}

function onTrafficIPChange(val) {
  trafficSelectedIP = val;
  trafficHighlightedNode = val ? `ip:${val}` : null;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData);
}

function onTrafficModelChange(val) {
  trafficSelectedModel = val;
  trafficHighlightedNode = val ? `model:${val}` : null;
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
  trafficHighlightedNode = null;

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

// 页面加载绑定
document.addEventListener('DOMContentLoaded', () => {
  setTrafficPollingRate(3);
});
