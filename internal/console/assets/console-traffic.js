/**
 * Gate Admin Console - Visual Gateway (Magpie Architecture)
 * 纯轻量交互式 SVG 拓扑流向图、SWR 极速缓存、IP/属地/Key 三维关联
 * 微标库拆入 console-traffic-icons.js，拓扑渲染拆入 console-traffic-topology.js。
 */

let cachedTrafficData = null;
let trafficSelectedIP = '';
let trafficSelectedKey = '';
let trafficTimer = null;
let trafficPollingInterval = 3;
let trafficHighlightedNode = null;

// 时间范围滑动条状态
let trafficActiveRange = '5m';

const TRAFFIC_SWR_CACHE_KEY = 'cpa_traffic_snapshot_cache';

// 3. SWR 数据拉取调度

// 时间尺度切换 (5m / today / 7d / 30d / all)
function switchTrafficRange(range) {
  if (!range) range = '5m';
  if (trafficActiveRange === range) return;
  trafficActiveRange = range;

  // 1. 更新分段按钮高亮
  const tabs = document.querySelectorAll('#traffic-range-tabs button');
  tabs.forEach(btn => {
    btn.classList.toggle('active', btn.getAttribute('data-range') === range);
  });

  // 2. 在历史区间模式下，默认关闭高频 3s 轮询，改为主席点击刷新，避免频繁打 SQLite
  const pollingSelect = document.getElementById('traffic-polling-select');
  if (range !== '5m') {
    if (pollingSelect && pollingSelect.value !== '0') {
      pollingSelect.value = '0';
      setTrafficPollingRate(0);
    }
  } else {
    if (pollingSelect && pollingSelect.value === '0') {
      pollingSelect.value = '3';
      setTrafficPollingRate(3);
    }
  }

  // 3. 清理节点高亮选态
  trafficHighlightedNode = null;

  // 4. 立即渲染 1:1 骨架微光占位（杜绝切换停滞感，提供即时视觉反馈）
  renderTrafficSkeleton();

  // 5. 立即请求对应尺度数据
  fetchTrafficData(true);
}

async function fetchTrafficData(force = false) {
  const currentRange = trafficActiveRange || '5m';
  const url = currentRange === '5m' ? '/console/api/traffic' : `/console/api/traffic?range=${encodeURIComponent(currentRange)}`;
  const cacheKey = `${TRAFFIC_SWR_CACHE_KEY}_${currentRange}`;

  if (!cachedTrafficData && !force) {
    try {
      const cachedRaw = sessionStorage.getItem(cacheKey);
      if (cachedRaw) {
        const cached = JSON.parse(cachedRaw);
        if (cached && cached.summary) {
          cachedTrafficData = cached;
          renderTrafficUI(cached, false);
        }
      } else {
        renderTrafficSkeleton();
      }
    } catch (e) {
      renderTrafficSkeleton();
    }
  }

  try {
    const res = await fetch(url, {
      headers: getAuthHeaders(),
      cache: 'no-store'
    });
    if (!res.ok) return;
    const data = await res.json();
    cachedTrafficData = data;

    try {
      sessionStorage.setItem(cacheKey, JSON.stringify(data));
    } catch (e) {}

    const badge = document.getElementById('nav-badge-traffic');
    if (badge && data.summary) {
      const count = Number(data.summary.active_ips || 0);
      badge.textContent = String(count);
      badge.style.display = count > 0 ? 'inline-flex' : 'none';
    }

    renderTrafficUI(data, false);
  } catch (err) {
    console.error('Failed to fetch visual gateway traffic data:', err);
  }
}

// 4. 骨架屏渲染 (1:1 结构几何微光)
function renderTrafficSkeleton() {
  const container = document.getElementById('traffic-topology-canvas-container');
  if (container) {
    container.innerHTML = `
      <div class="traffic-topology-skeleton">
        <div class="traffic-skeleton-col">
          <div class="traffic-skeleton-shimmer traffic-skeleton-head"></div>
          <div class="traffic-skeleton-shimmer traffic-skeleton-card"></div>
          <div class="traffic-skeleton-shimmer traffic-skeleton-card"></div>
        </div>
        <div class="traffic-skeleton-col">
          <div class="traffic-skeleton-shimmer traffic-skeleton-head"></div>
          <div class="traffic-skeleton-shimmer traffic-skeleton-card"></div>
          <div class="traffic-skeleton-shimmer traffic-skeleton-card"></div>
        </div>
        <div class="traffic-skeleton-col">
          <div class="traffic-skeleton-shimmer traffic-skeleton-head"></div>
          <div class="traffic-skeleton-shimmer traffic-skeleton-card"></div>
          <div class="traffic-skeleton-shimmer traffic-skeleton-card"></div>
        </div>
        <div class="traffic-skeleton-col">
          <div class="traffic-skeleton-shimmer traffic-skeleton-head"></div>
          <div class="traffic-skeleton-shimmer traffic-skeleton-card"></div>
        </div>
      </div>
    `;
  }

  // 顶部 4 张核心指标卡同步骨架微光占位（与真实卡片尺寸 1:1 锁死）
  const elIps = document.getElementById('traffic-val-ips');
  const elReqs = document.getElementById('traffic-val-reqs');
  const elLat = document.getElementById('traffic-val-latency');
  const elSucc = document.getElementById('traffic-val-success');

  const skeletonPill = (w) => `<span class="traffic-skeleton-shimmer" style="display:inline-block;width:${w}px;height:22px;border-radius:6px;vertical-align:middle;"></span>`;
  if (elIps) elIps.innerHTML = `${skeletonPill(52)}<small style="opacity:0.6;">个活跃 IP</small>`;
  if (elReqs) elReqs.innerHTML = `${skeletonPill(68)}<small style="opacity:0.6;">次调用</small>`;
  if (elLat) elLat.innerHTML = `${skeletonPill(48)}<small style="opacity:0.6;">ms</small>`;
  if (elSucc) elSucc.innerHTML = `${skeletonPill(50)}<small style="opacity:0.6;">%</small>`;

  // 底部两块账本同步骨架屏
  const ipLedger = document.getElementById('traffic-ip-ledger-grid');
  if (ipLedger) {
    ipLedger.innerHTML = `
      <div class="traffic-skeleton-shimmer" style="height:64px;border-radius:10px;margin-bottom:8px;"></div>
      <div class="traffic-skeleton-shimmer" style="height:64px;border-radius:10px;"></div>
    `;
  }
  const geoLedger = document.getElementById('traffic-geo-distribution-container');
  if (geoLedger) {
    geoLedger.innerHTML = `
      <div class="traffic-skeleton-shimmer" style="height:56px;border-radius:10px;margin-bottom:8px;"></div>
      <div class="traffic-skeleton-shimmer" style="height:56px;border-radius:10px;"></div>
    `;
  }
}

// 5. 渲染完整可视化网关面板
function renderTrafficUI(data, animate = false) {
  if (!data) return;
  const summary = data.summary || { active_ips: 0, requests_5m: 0, avg_latency_ms: 0, success_rate: 0, total_recorded: 0 };
  const currentRange = data.range || trafficActiveRange || '5m';

  const rangeMeta = {
    '5m': {
      titleText: '实时调用流向拓扑 (Traffic Topology)',
      reqsLabel: '近 5 分钟请求量 (Requests)',
      unit: '次 / 5m',
      descIps: '近 5 分钟请求发起唯一真实 IP 地址',
      descTotal: '环形缓冲累计记录',
      descLat: '近 5 分钟调用平均往返时延',
      descSucc: 'HTTP 2xx 成功调用比例'
    },
    'today': {
      titleText: '今日调用流向拓扑 (Traffic Topology)',
      reqsLabel: '今日请求量 (Requests)',
      unit: '次 / 今日',
      descIps: '今日请求发起唯一真实 IP 地址',
      descTotal: '今日已归档请求',
      descLat: '今日调用平均往返时延',
      descSucc: '今日 HTTP 2xx 成功调用比例'
    },
    '7d': {
      titleText: '近 7 天调用流向拓扑 (Traffic Topology)',
      reqsLabel: '近 7 天请求量 (Requests)',
      unit: '次 / 7天',
      descIps: '近 7 天请求发起唯一真实 IP 地址',
      descTotal: '近 7 天已归档请求',
      descLat: '近 7 天调用平均往返时延',
      descSucc: '近 7 天 HTTP 2xx 成功调用比例'
    },
    '30d': {
      titleText: '近 30 天调用流向拓扑 (Traffic Topology)',
      reqsLabel: '近 30 天请求量 (Requests)',
      unit: '次 / 30天',
      descIps: '近 30 天请求发起唯一真实 IP 地址',
      descTotal: '近 30 天已归档请求',
      descLat: '近 30 天调用平均往返时延',
      descSucc: '近 30 天 HTTP 2xx 成功调用比例'
    },
    'all': {
      titleText: '全时期调用流向拓扑 (Traffic Topology)',
      reqsLabel: '全时期请求量 (Requests)',
      unit: '次 / 全部',
      descIps: '全时期请求发起唯一真实 IP 地址',
      descTotal: '全时期已归档请求',
      descLat: '全时期调用平均往返时延',
      descSucc: '全时期 HTTP 2xx 成功调用比例'
    }
  };
  const meta = rangeMeta[currentRange] || rangeMeta['5m'];

  // 同步拓扑卡片标题与请求量指标名
  const titleEl = document.getElementById('traffic-topology-title-text');
  if (titleEl) titleEl.textContent = meta.titleText;

  const reqsLabelEl = document.getElementById('traffic-metric-reqs-label');
  if (reqsLabelEl) reqsLabelEl.textContent = meta.reqsLabel;

  const elIps = document.getElementById('traffic-val-ips');
  const elReqs = document.getElementById('traffic-val-reqs');
  const elLat = document.getElementById('traffic-val-latency');
  const elSucc = document.getElementById('traffic-val-success');
  const elTotal = document.getElementById('traffic-val-total');

  if (elIps) elIps.innerHTML = `${summary.active_ips || 0}<small>个活跃 IP</small>`;
  if (elReqs) elReqs.innerHTML = `${summary.requests_5m || summary.total_recorded || 0}<small>${meta.unit}</small>`;
  if (elLat) elLat.innerHTML = `${Math.round(summary.avg_latency_ms || 0)}<small>ms</small>`;
  if (elSucc) elSucc.innerHTML = `${((summary.success_rate || 0) * 100).toFixed(1)}<small>%</small>`;
  if (elTotal) elTotal.textContent = `${meta.descTotal}: ${summary.total_recorded || 0} 条`;

  // 动态更新卡片副文本
  const cardDescs = document.querySelectorAll('.traffic-metrics-grid .traffic-metric-card .traffic-metric-desc');
  if (cardDescs.length >= 4) {
    cardDescs[0].textContent = meta.descIps;
    cardDescs[2].textContent = meta.descLat;
    cardDescs[3].textContent = meta.descSucc;
  }

  // 增量 Diff 渲染拓扑 (零销毁、零闪烁)
  renderTrafficTopology(data, animate);
  renderTrafficIPLedger(data.ip_stats || []);
  renderTrafficGeoDistribution(data.geo_stats || []);
}


// 8. 渲染多用户关联视图 (IP · 属地 · Key 三维关联台账)
function renderTrafficIPLedger(ipStats) {
  const container = document.getElementById('traffic-ip-ledger-grid');
  if (!container) return;

  if (ipStats.length === 0) {
    container.innerHTML = `<div style="color:var(--text-muted);font-size:12px;padding:12px 0;">暂无活跃客户端 IP 记录</div>`;
    return;
  }

  container.innerHTML = ipStats.map(s => {
    const isSelected = trafficSelectedIP === s.ip;
    let timeStr = '-';
    if (s.last_seen) {
      const d = new Date(s.last_seen * 1000);
      if (trafficActiveRange === '5m' || trafficActiveRange === 'today') {
        timeStr = d.toLocaleTimeString();
      } else {
        timeStr = `${d.getMonth() + 1}/${d.getDate()} ${d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`;
      }
    }
    const modelsBadge = (s.top_models || []).map(m => `<span class="traffic-pill traffic-pill-model">${escapeHtml(m)}</span>`).join(' ');

    let netBadge = '';
    if (s.ip === '172.18.0.1' || s.ip.startsWith('172.18.') || s.ip.startsWith('172.17.')) {
      netBadge = '<span style="font-size:10px;padding:1px 5px;border-radius:4px;background:rgba(2,132,199,0.12);color:#0284c7;font-weight:600;">Docker Bridge</span>';
    } else if (s.ip.startsWith('100.')) {
      netBadge = '<span style="font-size:10px;padding:1px 5px;border-radius:4px;background:rgba(139,92,246,0.12);color:#8b5cf6;font-weight:600;">Tailnet</span>';
    }

    // 属地与 ASN
    let locLine = '';
    if (s.geo && (s.geo.formatted || s.geo.country)) {
      const loc = s.geo.formatted || s.geo.country;
      const asn = s.geo.asn ? `AS${s.geo.asn} ${s.geo.asn_org || ''}`.trim() : (s.geo.asn_org || '');
      locLine = `
        <div style="display:flex;align-items:center;gap:6px;font-size:11px;color:var(--text-muted);margin:2px 0 4px 0;flex-wrap:wrap;">
          <span style="display:inline-flex;color:#0284c7;">${TRAFFIC_ICONS.mapPin}</span>
          <span style="font-weight:600;color:var(--text-main);">${escapeHtml(loc)}</span>
          ${asn ? `<span style="opacity:0.65;font-size:10px;">(${escapeHtml(asn)})</span>` : ''}
          ${s.geo.is_proxy ? '<span class="traffic-pill traffic-pill-danger" style="font-size:9px;padding:0 4px;">VPN/Proxy</span>' : ''}
        </div>
      `;
    }

    // 该 IP 名下所有使用过的 Keys 胶囊 (支持点击反查)
    let keysLine = '';
    if (Array.isArray(s.keys) && s.keys.length > 0) {
      const keyPills = s.keys.map(k => {
        const isKeyActive = trafficSelectedKey === k.key;
        return `
          <span class="traffic-pill traffic-pill-key ${isKeyActive ? 'is-active' : ''}" onclick="event.stopPropagation(); filterTrafficByKey('${escapeHtml(k.key)}')" title="点击反查使用此 Key 的所有关联 IP">
            Key: ${escapeHtml(k.key)} (${k.request_count}次 · ${(k.tokens || 0).toLocaleString()}tok)
          </span>
        `;
      }).join(' ');

      keysLine = `
        <div style="display:flex;gap:4px;flex-wrap:wrap;margin:4px 0;">
          ${keyPills}
        </div>
      `;
    }

    // 成功率与平均时延
    const succPct = ((s.success_rate || 0) * 100).toFixed(1);
    const avgLat = Math.round(s.avg_latency_ms || 0);

    return `
      <div class="traffic-ip-card ${isSelected ? 'is-active' : ''}" onclick="filterTrafficByIP('${escapeHtml(s.ip)}')">
        <div style="display:flex;justify-content:space-between;align-items:center;">
          <div style="display:flex;align-items:center;gap:7px;">
            <span class="traffic-pulse-dot"></span>
            <span style="font-family:var(--mono, monospace);font-weight:700;font-size:13px;color:${isSelected ? '#0891b2' : 'var(--text-main)'};">${escapeHtml(s.ip)}</span>
            ${netBadge}
          </div>
          <div style="display:flex;align-items:center;gap:8px;">
            <span style="font-size:11px;color:var(--text-muted);">${timeStr}</span>
            <span style="font-size:11px;color:#2563eb;cursor:pointer;font-weight:600;" onclick="event.stopPropagation(); jumpToLogsWithIP('${escapeHtml(s.ip)}')" title="在请求流水中查看此 IP 的所有调用明细">查看流水 ↗</span>
          </div>
        </div>
        ${locLine}
        ${keysLine}
        <div style="display:flex;gap:14px;font-size:11.5px;margin:4px 0;color:var(--text-muted);flex-wrap:wrap;">
          <span>请求: <strong style="color:var(--text-main);font-family:var(--mono, monospace);">${s.request_count}</strong></span>
          <span>Tokens: <strong style="color:#7c3aed;font-family:var(--mono, monospace);">${(s.tokens || 0).toLocaleString()}</strong></span>
          <span>成功率: <strong style="color:#059669;font-family:var(--mono, monospace);">${succPct}%</strong></span>
          <span>延迟: <strong style="color:var(--text-main);font-family:var(--mono, monospace);">${avgLat}ms</strong></span>
        </div>
        <div style="display:flex;gap:4px;flex-wrap:wrap;margin-top:2px;">${modelsBadge}</div>
      </div>
    `;
  }).join('');
}

// 8.5 渲染地域分布 Top 来源排行
function renderTrafficGeoDistribution(geoStats) {
  const container = document.getElementById('traffic-geo-distribution-grid');
  if (!container) return;

  if (!geoStats || geoStats.length === 0) {
    container.innerHTML = `<div style="color:var(--text-muted);font-size:12px;padding:12px 0;">暂无地域解析数据</div>`;
    return;
  }

  container.innerHTML = geoStats.slice(0, 6).map((g, idx) => {
    return `
      <div class="traffic-geo-card">
        <div style="display:flex;align-items:center;gap:8px;">
          <span class="traffic-geo-rank ${idx < 3 ? 'is-top' : ''}">${idx + 1}</span>
          <div style="display:flex;flex-direction:column;gap:1px;">
            <div style="display:flex;align-items:center;gap:5px;">
              <span style="color:#0284c7;display:inline-flex;">${TRAFFIC_ICONS.mapPin}</span>
              <span style="font-size:12px;font-weight:600;color:var(--text-main);">${escapeHtml(g.location)}</span>
            </div>
            <span style="font-size:10.5px;color:var(--text-muted);">${(g.tokens || 0).toLocaleString()} tokens</span>
          </div>
        </div>
        <div style="display:flex;align-items:center;gap:6px;">
          ${g.is_proxy ? '<span class="traffic-pill traffic-pill-warning" style="font-size:9px;padding:1px 4px;">Proxy</span>' : ''}
          <span style="font-family:var(--mono, monospace);font-weight:700;font-size:12px;color:#0284c7;">${g.request_count} 次</span>
        </div>
      </div>
    `;
  }).join('');
}

function filterTrafficByIP(ip) {
  trafficSelectedIP = (trafficSelectedIP === ip) ? '' : ip;
  trafficHighlightedNode = trafficSelectedIP ? `ip:${trafficSelectedIP}` : null;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData, false);
}

function filterTrafficByKey(keyName) {
  trafficSelectedKey = (trafficSelectedKey === keyName) ? '' : keyName;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData, false);
}

window.jumpToLogsWithIP = function(ip) {
  if (typeof switchNavTab === 'function') {
    switchNavTab('logs');
  }
  if (typeof filterLogsByIP === 'function') {
    filterLogsByIP(ip);
  }
};

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

// 页面加载与 SWR 初次直出
document.addEventListener('DOMContentLoaded', () => {
  setTrafficPollingRate(3);
  try {
    const cachedRaw = sessionStorage.getItem(TRAFFIC_SWR_CACHE_KEY);
    if (cachedRaw) {
      const cached = JSON.parse(cachedRaw);
      if (cached && cached.summary) {
        cachedTrafficData = cached;
        renderTrafficUI(cached, false);
      }
    }
  } catch (e) {}
});
