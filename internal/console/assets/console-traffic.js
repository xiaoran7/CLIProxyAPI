/**
 * Gate Admin Console - Visual Gateway (Magpie Architecture)
 * 纯轻量交互式 SVG 拓扑流向图、品牌矢量微标、SWR 极速缓存、IP/属地/Key 三维关联与时间滑动条流水
 */

let cachedTrafficData = null;
let trafficSelectedIP = '';
let trafficSelectedKey = '';
let trafficSelectedModel = '';
let trafficSelectedStatus = 'all';
let trafficSearchQuery = '';
let trafficTimer = null;
let trafficPollingInterval = 3;
let trafficHighlightedNode = null;

// 时间范围滑动条状态
let trafficTimeMinPercent = 0;
let trafficTimeMaxPercent = 100;
let trafficBufferMinTs = 0;
let trafficBufferMaxTs = 0;
let trafficActiveTimePreset = 'all';

const TRAFFIC_SWR_CACHE_KEY = 'cpa_traffic_snapshot_cache';

// 1. 矢量微标库 (严格 Lucide 规范，1.8px 细线条，零原生 Emoji)
const TRAFFIC_ICONS = {
  globe: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/></svg>',
  activity: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg>',
  zap: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>',
  shield: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>',
  refresh: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M3 21v-5h5"/></svg>',
  layers: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/></svg>',
  search: '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>',
  mapPin: '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 0 1 16 0Z"/><circle cx="12" cy="10" r="3"/></svg>',
  server: '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect width="20" height="8" x="2" y="2" rx="2" ry="2"/><rect width="20" height="8" x="2" y="14" rx="2" ry="2"/><line x1="6" x2="6.01" y1="6" y2="6"/><line x1="6" x2="6.01" y1="18" y2="18"/></svg>',
  arrowRight: '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12 5 19 12 12 19"/></svg>'
};

// 2. 品牌几何微标生成器 (SVG 内嵌嵌入)
function getBrandNodeVisual(type, rawLabel) {
  const l = (rawLabel || '').toLowerCase();

  // A. Client IP 节点
  if (type === 'client_ip') {
    if (l.startsWith('172.18.') || l.startsWith('172.17.') || l === '172.18.0.1') {
      return {
        icon: '<rect x="2" y="3.5" width="12" height="9" rx="2" fill="none" stroke="#0284c7" stroke-width="1.4"/><path d="M5 3.5V2M8 3.5V2M11 3.5V2" stroke="#0284c7" stroke-width="1.4"/>',
        borderColor: 'rgba(2, 132, 199, 0.55)',
        bgColor: 'rgba(2, 132, 199, 0.08)',
        badge: 'Bridge',
        color: '#0284c7'
      };
    }
    if (l.startsWith('100.')) {
      return {
        icon: '<circle cx="8" cy="8" r="6" fill="none" stroke="#8b5cf6" stroke-width="1.4"/><circle cx="8" cy="8" r="2.2" fill="#8b5cf6"/>',
        borderColor: 'rgba(139, 92, 246, 0.55)',
        bgColor: 'rgba(139, 92, 246, 0.08)',
        badge: 'Tailnet',
        color: '#8b5cf6'
      };
    }
    if (l === '127.0.0.1' || l === 'localhost' || l.startsWith('192.168.') || l.startsWith('10.')) {
      return {
        icon: '<path d="M2 13h12M4 13V6l4-3 4 3v7" fill="none" stroke="#10b981" stroke-width="1.3"/>',
        borderColor: 'rgba(16, 185, 129, 0.55)',
        bgColor: 'rgba(16, 185, 129, 0.08)',
        badge: 'LAN',
        color: '#10b981'
      };
    }
    return {
      icon: '<circle cx="8" cy="8" r="6" fill="none" stroke="#06b6d4" stroke-width="1.4"/><path d="M8 2a8.5 8.5 0 0 0 0 12 8.5 8.5 0 0 0 0-12M2 8h12" stroke="#06b6d4" stroke-width="1.2"/>',
      borderColor: 'rgba(6, 182, 212, 0.55)',
      bgColor: 'rgba(6, 182, 212, 0.08)',
      badge: 'WAN',
      color: '#06b6d4'
    };
  }

  // B. 请求模型节点
  if (type === 'model') {
    if (l.includes('claude')) {
      return {
        icon: '<path d="M8 2l1.6 4.4L14 8l-4.4 1.6L8 14l-1.6-4.4L2 8l4.4-1.6z" fill="#d97706" stroke="#b45309" stroke-width="1.2"/>',
        borderColor: 'rgba(217, 119, 6, 0.55)',
        bgColor: 'rgba(217, 119, 6, 0.09)',
        color: '#d97706'
      };
    }
    if (l.includes('gemini') || l.includes('argon')) {
      return {
        icon: '<path d="M8 2c0 3.3-2.7 6-6 6 3.3 0 6 2.7 6 6 0-3.3 2.7-6 6-6-3.3 0-6-2.7-6-6z" fill="#2563eb" stroke="#1d4ed8" stroke-width="1.2"/>',
        borderColor: 'rgba(37, 99, 235, 0.55)',
        bgColor: 'rgba(37, 99, 235, 0.09)',
        color: '#2563eb'
      };
    }
    if (l.includes('step') || l.includes('stepfun')) {
      return {
        icon: '<path fill-rule="evenodd" d="M8 0C3.58 0 0 3.58 0 8s3.58 8 8 8 8-3.58 8-8S12.42 0 8 0ZM5.84 13.07H2.92v-2.92h2.92v2.92ZM9.46 13.07H6.54v-2.92h2.92v2.92ZM9.46 9.46H6.54V6.54h2.92v2.92ZM9.46 5.85H6.54V2.93h2.92v2.92ZM13.07 5.85h-2.92V2.93h2.92v2.92Z" fill="#8b5cf6"/>',
        borderColor: 'rgba(139, 92, 246, 0.55)',
        bgColor: 'rgba(139, 92, 246, 0.09)',
        color: '#8b5cf6'
      };
    }
    if (l.includes('minimax') || l.includes('abab')) {
      return {
        icon: '<path d="M2 7v1a1 1 0 0 0 2 0V5.3a1 1 0 0 1 2 0v5.4a1 1 0 0 0 2 0V2.3a1 1 0 0 1 2 0V9M8 9.3v2.4a1 1 0 0 0 2 0V2.3a1 1 0 0 1 2 0v5.4a1 1 0 0 0 2 0v-2.7a1 1 0 0 0-2 0" fill="none" stroke="#0891b2" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/>',
        borderColor: 'rgba(8, 145, 178, 0.55)',
        bgColor: 'rgba(8, 145, 178, 0.09)',
        color: '#0891b2'
      };
    }
    if (l.includes('deepseek')) {
      return {
        icon: '<path d="M3 10.5c3-4 7-5 10.5-4-2 4-5 7-9.5 6-1 0-1-.5-1-2z" fill="#0284c7" stroke="#0369a1" stroke-width="1.2"/>',
        borderColor: 'rgba(2, 132, 199, 0.55)',
        bgColor: 'rgba(2, 132, 199, 0.09)',
        color: '#0284c7'
      };
    }
    if (l.includes('gpt') || l.includes('o1') || l.includes('o3') || l.includes('openai')) {
      return {
        icon: '<path d="M8 2a6 6 0 0 1 5.2 3A6 6 0 0 1 14 11a6 6 0 0 1-5.2 3A6 6 0 0 1 2 11a6 6 0 0 1 6-9z" fill="none" stroke="#10b981" stroke-width="1.6"/>',
        borderColor: 'rgba(16, 185, 129, 0.55)',
        bgColor: 'rgba(16, 185, 129, 0.09)',
        color: '#10b981'
      };
    }
    if (l.includes('qwen') || l.includes('qoder') || l.includes('dfmodel')) {
      return {
        icon: '<polygon points="8 2 14 8 8 14 2 8" fill="none" stroke="#6366f1" stroke-width="1.5"/><circle cx="8" cy="8" r="2" fill="#6366f1"/>',
        borderColor: 'rgba(99, 102, 241, 0.55)',
        bgColor: 'rgba(99, 102, 241, 0.09)',
        color: '#6366f1'
      };
    }
    if (l.includes('probe') || l.includes('unspecified') || l.includes('unknown')) {
      return {
        icon: '<path d="M8 2l5 2.5v4c0 3.5-2.5 5.5-5 6.5-2.5-1-5-3-5-6.5v-4z" fill="none" stroke="#f59e0b" stroke-width="1.4"/><line x1="8" y1="5.5" x2="8" y2="8.5" stroke="#f59e0b" stroke-width="1.5"/><circle cx="8" cy="11" r="0.8" fill="#f59e0b"/>',
        borderColor: 'rgba(245, 158, 11, 0.55)',
        bgColor: 'rgba(245, 158, 11, 0.09)',
        color: '#f59e0b'
      };
    }
    return {
      icon: '<circle cx="8" cy="8" r="6" fill="none" stroke="#8b5cf6" stroke-width="1.4"/><path d="M8 5v6M5 8h6" stroke="#8b5cf6" stroke-width="1.4"/>',
      borderColor: 'rgba(139, 92, 246, 0.55)',
      bgColor: 'rgba(139, 92, 246, 0.09)',
      color: '#8b5cf6'
    };
  }

  // C. 路由渠道节点
  if (type === 'channel') {
    if (l.includes('antigravity')) {
      return {
        icon: '<polygon points="8 2 14 5.5 14 11.5 8 14 2 11.5 2 5.5" fill="none" stroke="#06b6d4" stroke-width="1.4"/><circle cx="8" cy="8" r="2.2" fill="#06b6d4"/>',
        borderColor: 'rgba(6, 182, 212, 0.55)',
        bgColor: 'rgba(6, 182, 212, 0.09)',
        color: '#06b6d4'
      };
    }
    if (l.includes('workbuddy')) {
      return {
        icon: '<polyline points="4 11 8 8 4 5" stroke="#3b82f6" stroke-width="1.8" stroke-linecap="round"/><line x1="9" y1="11" x2="13" y2="11" stroke="#3b82f6" stroke-width="1.8"/>',
        borderColor: 'rgba(59, 130, 246, 0.55)',
        bgColor: 'rgba(59, 130, 246, 0.09)',
        color: '#3b82f6'
      };
    }
    if (l.includes('stepfun') || l.includes('step')) {
      return {
        icon: '<path fill-rule="evenodd" d="M8 0C3.58 0 0 3.58 0 8s3.58 8 8 8 8-3.58 8-8S12.42 0 8 0ZM5.84 13.07H2.92v-2.92h2.92v2.92ZM9.46 13.07H6.54v-2.92h2.92v2.92ZM9.46 9.46H6.54V6.54h2.92v2.92ZM9.46 5.85H6.54V2.93h2.92v2.92ZM13.07 5.85h-2.92V2.93h2.92v2.92Z" fill="#8b5cf6"/>',
        borderColor: 'rgba(139, 92, 246, 0.55)',
        bgColor: 'rgba(139, 92, 246, 0.09)',
        color: '#8b5cf6'
      };
    }
    if (l.includes('minimax')) {
      return {
        icon: '<path d="M2 7v1a1 1 0 0 0 2 0V5.3a1 1 0 0 1 2 0v5.4a1 1 0 0 0 2 0V2.3a1 1 0 0 1 2 0V9M8 9.3v2.4a1 1 0 0 0 2 0V2.3a1 1 0 0 1 2 0v5.4a1 1 0 0 0 2 0v-2.7a1 1 0 0 0-2 0" fill="none" stroke="#0891b2" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/>',
        borderColor: 'rgba(8, 145, 178, 0.55)',
        bgColor: 'rgba(8, 145, 178, 0.09)',
        color: '#0891b2'
      };
    }
    // Channel nodes are labelled by provider/credential name, which does not
    // always carry the upstream brand (workbuddy-cn serves deepseek models,
    // "gemini" and "qoder" are providers in their own right). Without these
    // branches every such node fell through to the neutral placeholder, which
    // is what made a topology full of distinct brands look duplicated.
    if (l.includes('deepseek')) {
      return {
        icon: '<path d="M3 10.5c3-4 7-5 10.5-4-2 4-5 7-9.5 6-1 0-1-.5-1-2z" fill="#0284c7" stroke="#0369a1" stroke-width="1.2"/>',
        borderColor: 'rgba(2, 132, 199, 0.55)',
        bgColor: 'rgba(2, 132, 199, 0.09)',
        color: '#0284c7'
      };
    }
    if (l.includes('gemini') || l.includes('google') || l.includes('vertex')) {
      return {
        icon: '<path d="M8 2c0 3.3-2.7 6-6 6 3.3 0 6 2.7 6 6 0-3.3 2.7-6 6-6-3.3 0-6-2.7-6-6z" fill="#2563eb" stroke="#1d4ed8" stroke-width="1.2"/>',
        borderColor: 'rgba(37, 99, 235, 0.55)',
        bgColor: 'rgba(37, 99, 235, 0.09)',
        color: '#2563eb'
      };
    }
    if (l.includes('claude') || l.includes('anthropic')) {
      return {
        icon: '<path d="M8 2l1.6 4.4L14 8l-4.4 1.6L8 14l-1.6-4.4L2 8l4.4-1.6z" fill="#d97706" stroke="#b45309" stroke-width="1.2"/>',
        borderColor: 'rgba(217, 119, 6, 0.55)',
        bgColor: 'rgba(217, 119, 6, 0.09)',
        color: '#d97706'
      };
    }
    if (l.includes('qoder')) {
      return {
        icon: '<circle cx="8" cy="8" r="5.5" fill="none" stroke="#ff6a00" stroke-width="1.5"/><path d="M8 5v6M5 8h6" stroke="#ff6a00" stroke-width="1.5" stroke-linecap="round"/>',
        borderColor: 'rgba(255, 106, 0, 0.55)',
        bgColor: 'rgba(255, 106, 0, 0.09)',
        color: '#ff6a00'
      };
    }
    if (l.includes('openrouter')) {
      return {
        icon: '<path d="M2 5h3l3 3h6M2 11h3l3-3h6" fill="none" stroke="#6366f1" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><circle cx="13" cy="4.5" r="1.6" fill="#6366f1"/><circle cx="13" cy="11.5" r="1.6" fill="#6366f1"/>',
        borderColor: 'rgba(99, 102, 241, 0.55)',
        bgColor: 'rgba(99, 102, 241, 0.09)',
        color: '#6366f1'
      };
    }
    if (l.includes('opencode')) {
      return {
        icon: '<rect x="2" y="3" width="12" height="10" rx="2" fill="none" stroke="#09090b" stroke-width="1.5"/><path d="M5 7l2 2-2 2M8.5 11h3" stroke="#09090b" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/>',
        borderColor: 'rgba(9, 9, 11, 0.55)',
        bgColor: 'rgba(9, 9, 11, 0.07)',
        color: '#09090b'
      };
    }
    if (l.includes('zcode') || l.includes('glm') || l.includes('bigmodel')) {
      return {
        icon: '<path d="M8 2l5 3v6l-5 3-5-3V5z" fill="none" stroke="#7c3aed" stroke-width="1.5" stroke-linejoin="round"/><circle cx="8" cy="8" r="2" fill="none" stroke="#7c3aed" stroke-width="1.4"/>',
        borderColor: 'rgba(124, 58, 237, 0.55)',
        bgColor: 'rgba(124, 58, 237, 0.09)',
        color: '#7c3aed'
      };
    }
    if (l.includes('kimi') || l.includes('moonshot')) {
      return {
        icon: '<circle cx="8" cy="8" r="5.5" fill="none" stroke="#0ea5e9" stroke-width="1.5"/><path d="M10.5 5.5a3.5 3.5 0 0 0 0 5" fill="none" stroke="#0ea5e9" stroke-width="1.4" stroke-linecap="round"/>',
        borderColor: 'rgba(14, 165, 233, 0.55)',
        bgColor: 'rgba(14, 165, 233, 0.09)',
        color: '#0ea5e9'
      };
    }
    if (l.includes('xai') || l.includes('grok')) {
      return {
        icon: '<path d="M3 3l10 10M13 3L3 13" stroke="#1f2937" stroke-width="1.6" stroke-linecap="round"/><circle cx="8" cy="8" r="6.2" fill="none" stroke="#1f2937" stroke-width="1.2"/>',
        borderColor: 'rgba(31, 41, 55, 0.55)',
        bgColor: 'rgba(31, 41, 55, 0.07)',
        color: '#1f2937'
      };
    }
    if (l.includes('gateway') || l.includes('probe') || l.includes('unspecified')) {
      return {
        icon: '<path d="M2 8h3l2-3 2 6 2-3h5" fill="none" stroke="#f59e0b" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>',
        borderColor: 'rgba(245, 158, 11, 0.55)',
        bgColor: 'rgba(245, 158, 11, 0.09)',
        color: '#f59e0b'
      };
    }
    return {
      icon: '<rect x="2" y="2.5" width="12" height="4.5" rx="1.5" fill="none" stroke="#64748b" stroke-width="1.3"/><rect x="2" y="9" width="12" height="4.5" rx="1.5" fill="none" stroke="#64748b" stroke-width="1.3"/><circle cx="4.5" cy="4.7" r="0.8" fill="#64748b"/><circle cx="4.5" cy="11.2" r="0.8" fill="#64748b"/>',
      borderColor: 'rgba(100, 116, 139, 0.55)',
      bgColor: 'rgba(100, 116, 139, 0.08)',
      color: '#64748b'
    };
  }

  // D. 状态节点
  if (type === 'status') {
    if (l.includes('2xx') || l.includes('success')) {
      return {
        icon: '<circle cx="8" cy="8" r="6" fill="none" stroke="#10b981" stroke-width="1.5"/><polyline points="5.5 8 7.5 10 11 6" stroke="#10b981" stroke-width="1.6" fill="none" stroke-linecap="round"/>',
        borderColor: 'rgba(16, 185, 129, 0.55)',
        bgColor: 'rgba(16, 185, 129, 0.1)',
        color: '#10b981'
      };
    }
    if (l.includes('4xx') || l.includes('499')) {
      return {
        icon: '<circle cx="8" cy="8" r="6" fill="none" stroke="#f59e0b" stroke-width="1.5"/><line x1="8" y1="5.5" x2="8" y2="8.5" stroke="#f59e0b" stroke-width="1.5"/><circle cx="8" cy="11" r="0.8" fill="#f59e0b"/>',
        borderColor: 'rgba(245, 158, 11, 0.55)',
        bgColor: 'rgba(245, 158, 11, 0.1)',
        color: '#f59e0b'
      };
    }
    return {
      icon: '<circle cx="8" cy="8" r="6" fill="none" stroke="#ef4444" stroke-width="1.5"/><line x1="5.5" y1="5.5" x2="10.5" y2="10.5" stroke="#ef4444" stroke-width="1.5"/><line x1="10.5" y1="5.5" x2="5.5" y2="10.5" stroke="#ef4444" stroke-width="1.5"/>',
      borderColor: 'rgba(239, 68, 68, 0.55)',
      bgColor: 'rgba(239, 68, 68, 0.1)',
      color: '#ef4444'
    };
  }

  return {
    icon: '',
    borderColor: 'rgba(148, 163, 184, 0.4)',
    bgColor: 'rgba(148, 163, 184, 0.08)',
    color: '#64748b'
  };
}

// 3. SWR 数据拉取调度
async function fetchTrafficData(force = false) {
  if (!cachedTrafficData && !force) {
    try {
      const cachedRaw = sessionStorage.getItem(TRAFFIC_SWR_CACHE_KEY);
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
    const res = await fetch('/console/api/traffic', {
      headers: getAuthHeaders(),
      cache: 'no-store'
    });
    if (!res.ok) return;
    const data = await res.json();
    cachedTrafficData = data;

    try {
      sessionStorage.setItem(TRAFFIC_SWR_CACHE_KEY, JSON.stringify(data));
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
  if (!container) return;

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

// 5. 渲染完整可视化网关面板
function renderTrafficUI(data, animate = false) {
  if (!data) return;
  const summary = data.summary || { active_ips: 0, requests_5m: 0, avg_latency_ms: 0, success_rate: 0, total_recorded: 0 };

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

  // 增量 Diff 渲染拓扑 (零销毁、零闪烁)
  renderTrafficTopology(data, animate);
  renderTrafficIPLedger(data.ip_stats || []);
  renderTrafficGeoDistribution(data.geo_stats || []);
}

// 6. 增量 DOM Diff 渲染拓扑图 (零销毁、零闪烁、平滑位移)
function renderTrafficTopology(data, animate = false) {
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
  const svgHeight = Math.max(260, maxNodes * 52 + 48);
  const nodeW = 168;
  const nodeH = 34;
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
  calcPositions(colModels, 280);
  calcPositions(colChannels, 536);
  calcPositions(colStatuses, 792);

  // 1. 初始化持久化外层 SVG (仅在初次或被清空时创建)
  let svgEl = document.getElementById('traffic-topology-svg');
  if (!svgEl || !container.contains(svgEl)) {
    container.innerHTML = `
      <svg id="traffic-topology-svg" viewBox="0 0 984 ${svgHeight}" style="width:100%;min-width:940px;height:${svgHeight}px;display:block;">
        <defs>
          <linearGradient id="grad-active-link" x1="0%" y1="0%" x2="100%" y2="0%">
            <stop offset="0%" stop-color="#06b6d4" />
            <stop offset="100%" stop-color="#3b82f6" />
          </linearGradient>
        </defs>
        <text x="24" y="20" fill="#0891b2" font-size="11" font-weight="700" letter-spacing="0.04em">STAGE 1 · CLIENT IP</text>
        <text x="280" y="20" fill="#7c3aed" font-size="11" font-weight="700" letter-spacing="0.04em">STAGE 2 · REQUEST MODEL</text>
        <text x="536" y="20" fill="#d97706" font-size="11" font-weight="700" letter-spacing="0.04em">STAGE 3 · ROUTED CHANNEL</text>
        <text x="792" y="20" fill="#059669" font-size="11" font-weight="700" letter-spacing="0.04em">STAGE 4 · OUTCOME STATUS</text>
        <g id="traffic-links-layer"></g>
        <g id="traffic-nodes-layer"></g>
      </svg>
    `;
    svgEl = document.getElementById('traffic-topology-svg');
  } else {
    svgEl.setAttribute('viewBox', `0 0 984 ${svgHeight}`);
    svgEl.style.height = `${svgHeight}px`;
  }

  const linksLayer = document.getElementById('traffic-links-layer');
  const nodesLayer = document.getElementById('traffic-nodes-layer');
  if (!linksLayer || !nodesLayer) return;

  // 2. 有向 DAG 高亮拓扑计算
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

  // 3. 增量 Diff 更新 Links (平滑过渡 d 属性与透明度，绝不销毁 DOM)
  const seenLinkKeys = new Set();
  for (const link of (topology.links || [])) {
    const p1 = nodePositions[link.source];
    const p2 = nodePositions[link.target];
    if (!p1 || !p2) continue;

    const linkKey = `${link.source}|${link.target}`;
    seenLinkKeys.add(linkKey);

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

    let pathEl = linksLayer.querySelector(`path[data-link-key="${CSS.escape(linkKey)}"]`);
    if (!pathEl) {
      pathEl = document.createElementNS('http://www.w3.org/2000/svg', 'path');
      pathEl.setAttribute('data-link-key', linkKey);
      pathEl.setAttribute('fill', 'none');
      linksLayer.appendChild(pathEl);
    }

    pathEl.setAttribute('d', d);
    pathEl.setAttribute('stroke', strokeColor);
    pathEl.setAttribute('stroke-width', String(strokeW));
    pathEl.setAttribute('class', `traffic-link-path ${isLinkActive ? 'is-active' : 'is-dimmed'}`);
  }

  // 移除过期 Links
  linksLayer.querySelectorAll('path[data-link-key]').forEach(el => {
    if (!seenLinkKeys.has(el.getAttribute('data-link-key'))) {
      el.remove();
    }
  });

  // 4. 增量 Diff 更新 Nodes (平滑插值 transform 坐标，更新计数，绝不销毁 DOM)
  const seenNodeIds = new Set();
  for (const [id, pos] of Object.entries(nodePositions)) {
    const item = pos.item;
    seenNodeIds.add(id);

    const isNodeActive = !isFiltered || activeNodes.has(id);
    const isNodeSelected = trafficHighlightedNode === id;
    const visual = getBrandNodeVisual(item.type, item.label);

    let shortLabel = item.label;
    if (shortLabel.length > 14) {
      shortLabel = shortLabel.slice(0, 13) + '…';
    }

    let subLocation = '';
    if (item.type === 'client_ip') {
      if (item.geo && (item.geo.formatted || item.geo.country)) {
        subLocation = item.geo.formatted || item.geo.country;
      } else if (item.label.startsWith('172.18.') || item.label.startsWith('172.17.')) {
        subLocation = 'Docker Gateway';
      } else if (item.label.startsWith('100.')) {
        subLocation = 'Tailnet Mesh';
      } else if (item.label === '127.0.0.1') {
        subLocation = '本机回环';
      }
      if (subLocation.length > 14) {
        subLocation = subLocation.slice(0, 13) + '…';
      }
    }

    let nodeGroup = nodesLayer.querySelector(`g[data-node-id="${CSS.escape(id)}"]`);
    if (!nodeGroup) {
      nodeGroup = document.createElementNS('http://www.w3.org/2000/svg', 'g');
      nodeGroup.setAttribute('data-node-id', id);
      nodeGroup.setAttribute('onclick', `toggleHighlightTrafficNode('${escapeHtml(id)}')`);

      const textSvg = subLocation
        ? `<text class="node-title" x="30" y="16.5" fill="var(--text-main, #0f172a)" font-size="10.5" font-weight="700" font-family="var(--mono, monospace)">${escapeHtml(shortLabel)}</text>
           <text class="node-sub" x="30" y="27" fill="var(--text-muted, #64748b)" font-size="8.5" font-weight="500">${escapeHtml(subLocation)}</text>`
        : `<text class="node-title" x="30" y="21" fill="var(--text-main, #0f172a)" font-size="11" font-weight="600" font-family="var(--mono, monospace)">${escapeHtml(shortLabel)}</text>`;

      nodeGroup.innerHTML = `
        <rect class="node-box" width="${nodeW}" height="${nodeH}" rx="8" fill="${visual.bgColor}" stroke="${visual.borderColor}" stroke-width="1.2" />
        <g transform="translate(8, 9)">
          <svg width="16" height="16" viewBox="0 0 16 16">
            ${visual.icon}
          </svg>
        </g>
        ${textSvg}
        <rect x="${nodeW - 32}" y="8" width="24" height="18" rx="9" fill="rgba(15, 23, 42, 0.08)" />
        <text class="node-count-text" x="${nodeW - 20}" y="21" fill="var(--text-main, #0f172a)" font-size="9.5" font-weight="700" text-anchor="middle" font-family="var(--mono, monospace)">${item.count}</text>
      `;
      nodesLayer.appendChild(nodeGroup);
    } else {
      // 节点已存在：更新位置坐标与计数
      const countEl = nodeGroup.querySelector('.node-count-text');
      if (countEl) countEl.textContent = String(item.count);

      const subEl = nodeGroup.querySelector('.node-sub');
      if (subEl && subLocation) subEl.textContent = subLocation;
    }

    // 采用 CSS transform 平滑位移
    nodeGroup.setAttribute('transform', `translate(${pos.x}, ${pos.y})`);
    nodeGroup.setAttribute('class', `traffic-node-group ${isNodeActive ? '' : 'is-dimmed'} ${isNodeSelected ? 'is-selected' : ''}`);
  }

  // 移除过期 Nodes
  nodesLayer.querySelectorAll('g[data-node-id]').forEach(el => {
    if (!seenNodeIds.has(el.getAttribute('data-node-id'))) {
      el.remove();
    }
  });
}

// 7. 拓扑节点交互高亮追踪
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
    if (nodeId.startsWith('ip:')) {
      const ip = nodeId.slice(3);
      trafficSelectedIP = ip;
      const ipSelect = document.getElementById('traffic-select-ip');
      if (ipSelect) ipSelect.value = ip;
    }
  }
  if (cachedTrafficData) {
    renderTrafficUI(cachedTrafficData, false);
  }
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
    const timeStr = s.last_seen ? new Date(s.last_seen * 1000).toLocaleTimeString() : '-';
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

// 9. 更新时间范围滑动条极值
function updateTrafficTimeSliderLimits(records) {
  if (!records || records.length === 0) {
    trafficBufferMinTs = 0;
    trafficBufferMaxTs = 0;
    return;
  }

  let minTs = records[0].timestamp;
  let maxTs = records[0].timestamp;
  for (const r of records) {
    if (r.timestamp < minTs) minTs = r.timestamp;
    if (r.timestamp > maxTs) maxTs = r.timestamp;
  }

  trafficBufferMinTs = minTs;
  trafficBufferMaxTs = maxTs;

  const startLabel = document.getElementById('traffic-time-label-start');
  const endLabel = document.getElementById('traffic-time-label-end');
  if (startLabel) startLabel.textContent = minTs ? new Date(minTs * 1000).toLocaleTimeString() : '-';
  if (endLabel) endLabel.textContent = maxTs ? new Date(maxTs * 1000).toLocaleTimeString() : '-';

  updateTrafficSliderProgressUI();
}

// 10. 时间滑动条输入与快捷预设处理
function onTrafficTimeSliderInput() {
  trafficActiveTimePreset = 'custom';
  document.querySelectorAll('.traffic-preset-btn').forEach(b => b.classList.remove('active'));

  const minInput = document.getElementById('traffic-time-min');
  const maxInput = document.getElementById('traffic-time-max');
  if (!minInput || !maxInput) return;

  let minVal = Number(minInput.value);
  let maxVal = Number(maxInput.value);

  if (minVal > maxVal) {
    const tmp = minVal;
    minVal = maxVal;
    maxVal = tmp;
  }

  trafficTimeMinPercent = minVal;
  trafficTimeMaxPercent = maxVal;

  updateTrafficSliderProgressUI();
  if (cachedTrafficData) {
    renderTrafficTable(cachedTrafficData.records || cachedTrafficData.recent_requests || []);
  }
}

function updateTrafficSliderProgressUI() {
  const progress = document.getElementById('traffic-slider-progress');
  const currentLabel = document.getElementById('traffic-time-label-current');
  if (!progress) return;

  progress.style.left = `${trafficTimeMinPercent}%`;
  progress.style.right = `${100 - trafficTimeMaxPercent}%`;

  if (currentLabel) {
    if (trafficTimeMinPercent === 0 && trafficTimeMaxPercent === 100) {
      currentLabel.textContent = '全量跨度';
    } else if (trafficBufferMinTs > 0 && trafficBufferMaxTs > 0) {
      const span = trafficBufferMaxTs - trafficBufferMinTs;
      const startT = Math.round(trafficBufferMinTs + span * (trafficTimeMinPercent / 100));
      const endT = Math.round(trafficBufferMinTs + span * (trafficTimeMaxPercent / 100));
      currentLabel.textContent = `${new Date(startT * 1000).toLocaleTimeString()} ➔ ${new Date(endT * 1000).toLocaleTimeString()}`;
    }
  }
}

function setTrafficTimePreset(preset) {
  trafficActiveTimePreset = preset;
  document.querySelectorAll('.traffic-preset-btn').forEach(b => {
    b.classList.toggle('active', b.id === `btn-preset-${preset}`);
  });

  const minInput = document.getElementById('traffic-time-min');
  const maxInput = document.getElementById('traffic-time-max');
  if (!minInput || !maxInput) return;

  if (preset === 'all' || trafficBufferMaxTs <= trafficBufferMinTs) {
    trafficTimeMinPercent = 0;
    trafficTimeMaxPercent = 100;
  } else {
    const nowTs = Math.floor(Date.now() / 1000);
    const span = Math.max(1, trafficBufferMaxTs - trafficBufferMinTs);
    let durationSec = 300; // 5m default
    if (preset === '1h') durationSec = 3600;
    else if (preset === '15m') durationSec = 900;
    else if (preset === '5m') durationSec = 300;

    const targetStartTs = Math.max(trafficBufferMinTs, trafficBufferMaxTs - durationSec);
    trafficTimeMinPercent = Math.max(0, Math.min(100, Math.round(((targetStartTs - trafficBufferMinTs) / span) * 100)));
    trafficTimeMaxPercent = 100;
  }

  minInput.value = String(trafficTimeMinPercent);
  maxInput.value = String(trafficTimeMaxPercent);

  updateTrafficSliderProgressUI();
  if (cachedTrafficData) {
    renderTrafficTable(cachedTrafficData.records || cachedTrafficData.recent_requests || []);
  }
}

// 11. 渲染实时调用流水明细表格 (10 列规范对齐)
function renderTrafficTable(records) {
  const tbody = document.getElementById('traffic-tbody');
  const countEl = document.getElementById('traffic-records-count');
  if (!tbody) return;

  // 计算滑动条时间窗口
  let filterStartTs = 0;
  let filterEndTs = 0;
  if (trafficBufferMinTs > 0 && trafficBufferMaxTs > 0) {
    const span = trafficBufferMaxTs - trafficBufferMinTs;
    filterStartTs = trafficBufferMinTs + span * (trafficTimeMinPercent / 100);
    filterEndTs = trafficBufferMinTs + span * (trafficTimeMaxPercent / 100);
  }

  const filtered = records.filter(r => {
    // 时间范围滑动条过滤
    if (filterStartTs > 0 && filterEndTs > 0) {
      if (r.timestamp < filterStartTs || r.timestamp > filterEndTs) return false;
    }

    if (trafficSelectedIP && r.client_ip !== trafficSelectedIP) return false;
    if (trafficSelectedKey && r.key_label !== trafficSelectedKey) return false;
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
      const mPath = r.path && r.path.toLowerCase().includes(q);
      const mGeo = r.geo && ((r.geo.formatted && r.geo.formatted.toLowerCase().includes(q)) || (r.geo.country && r.geo.country.toLowerCase().includes(q)) || (r.geo.region && r.geo.region.toLowerCase().includes(q)) || (r.geo.city && r.geo.city.toLowerCase().includes(q)) || (r.geo.asn_org && r.geo.asn_org.toLowerCase().includes(q)));
      if (!mIP && !mModel && !mReqModel && !mKey && !mChan && !mPath && !mGeo) return false;
    }
    return true;
  });

  if (countEl) {
    countEl.textContent = `展示 ${filtered.length} / ${records.length} 条`;
  }

  if (filtered.length === 0) {
    tbody.innerHTML = `<tr><td colspan="10" style="text-align:center;padding:36px;color:var(--text-muted);font-size:12.5px;">无匹配的调用记录</td></tr>`;
    return;
  }

  tbody.innerHTML = filtered.slice(0, 100).map(r => {
    const isSuccess = r.status >= 200 && r.status < 300;
    const is4xx = r.status >= 400 && r.status < 500;
    const statusClass = isSuccess ? 'traffic-pill-success' : is4xx ? 'traffic-pill-warning' : 'traffic-pill-danger';
    const timeStr = r.timestamp ? new Date(r.timestamp * 1000).toLocaleTimeString() : '-';

    // 属地与 ASN
    let locText = (r.geo && (r.geo.formatted || r.geo.country))
      ? (r.geo.formatted || r.geo.country)
      : (r.client_ip === '172.18.0.1' || r.client_ip.startsWith('172.18.') ? 'Docker Gateway' : (r.client_ip.startsWith('100.') ? 'Tailnet 专网' : (r.client_ip === '127.0.0.1' ? '本机回环' : '未知地域')));
    const isProxy = Boolean(r.geo && (r.geo.is_proxy || r.geo.is_vpn || r.geo.is_tor));
    const asnText = r.geo && r.geo.asn ? `AS${r.geo.asn} ${r.geo.asn_org || ''}`.trim() : (r.geo?.asn_org || '-');

    // 如果客户端请求模型与实际路由执行模型不同，展示真实透传流向
    let modelHtml = '';
    if (r.request_model && r.request_model !== r.model) {
      modelHtml = `<span style="opacity:0.8;" title="客户端请求: ${escapeHtml(r.request_model)}">${escapeHtml(r.request_model)}</span> <span style="color:var(--text-muted);">${TRAFFIC_ICONS.arrowRight}</span> <strong style="color:#7c3aed;">${escapeHtml(r.model || '-')}</strong>`;
    } else {
      modelHtml = `<span style="font-weight:600;">${escapeHtml(r.model || '-')}</span>`;
    }
    if (r.path && r.path !== '/v1/chat/completions' && r.path !== '/v1/messages') {
      modelHtml += ` <span style="font-size:10px;color:var(--text-muted);opacity:0.75;">(${escapeHtml(r.path)})</span>`;
    }

    return `
      <tr style="border-bottom:1px solid var(--border-subtle, rgba(0,0,0,0.05));">
        <td style="padding:10px 12px;font-family:var(--mono, monospace);color:var(--text-muted);">${timeStr}</td>
        <td style="padding:10px 12px;">
          <span class="traffic-pill traffic-pill-ip" onclick="filterTrafficByIP('${escapeHtml(r.client_ip)}')" title="点击筛选此 IP">${escapeHtml(r.client_ip || '-')}</span>
        </td>
        <td style="padding:10px 12px;">
          <span class="traffic-pill traffic-pill-geo">${escapeHtml(locText)}</span>
          ${isProxy ? ' <span class="traffic-pill traffic-pill-danger" style="font-size:9px;padding:0 4px;">Proxy</span>' : ''}
        </td>
        <td style="padding:10px 12px;color:var(--text-muted);font-size:11px;font-family:var(--mono, monospace);max-width:140px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${escapeHtml(asnText)}">${escapeHtml(asnText)}</td>
        <td style="padding:10px 12px;">
          <span class="traffic-pill traffic-pill-key ${trafficSelectedKey === r.key_label ? 'is-active' : ''}" onclick="filterTrafficByKey('${escapeHtml(r.key_label)}')" title="点击反查使用此 Key 的所有 IP">${escapeHtml(r.key_label || '-')}</span>
        </td>
        <td style="padding:10px 12px;"><span class="traffic-pill traffic-pill-model">${modelHtml}</span></td>
        <td style="padding:10px 12px;"><span class="traffic-pill traffic-pill-channel">${escapeHtml(r.channel || '-')}</span></td>
        <td style="padding:10px 12px;"><span class="traffic-pill ${statusClass}">${r.status || 200}</span></td>
        <td style="padding:10px 12px;font-family:var(--mono, monospace);font-variant-numeric:tabular-nums;color:${r.latency_ms > 3000 ? '#d97706' : 'inherit'};">${r.latency_ms || 0}ms</td>
        <td style="padding:10px 12px;font-family:var(--mono, monospace);font-variant-numeric:tabular-nums;color:#7c3aed;">${(r.tokens || 0).toLocaleString()}</td>
      </tr>
    `;
  }).join('');
}

// 12. 更新下拉筛选菜单
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

function onTrafficIPChange(val) {
  trafficSelectedIP = val;
  trafficHighlightedNode = val ? `ip:${val}` : null;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData, false);
}

function onTrafficModelChange(val) {
  trafficSelectedModel = val;
  trafficHighlightedNode = val ? `model:${val}` : null;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData, false);
}

function onTrafficStatusChange(val) {
  trafficSelectedStatus = val;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData, false);
}

function onTrafficSearchChange(val) {
  trafficSearchQuery = val.trim();
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData, false);
}

function resetTrafficFilters() {
  trafficSelectedIP = '';
  trafficSelectedKey = '';
  trafficSelectedModel = '';
  trafficSelectedStatus = 'all';
  trafficSearchQuery = '';
  trafficHighlightedNode = null;
  setTrafficTimePreset('all');

  const ipSel = document.getElementById('traffic-select-ip');
  const mSel = document.getElementById('traffic-select-model');
  const stSel = document.getElementById('traffic-select-status');
  const sInput = document.getElementById('traffic-search-input');

  if (ipSel) ipSel.value = '';
  if (mSel) mSel.value = '';
  if (stSel) stSel.value = 'all';
  if (sInput) sInput.value = '';

  if (cachedTrafficData) renderTrafficUI(cachedTrafficData, false);
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
