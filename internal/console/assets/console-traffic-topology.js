/**
 * Gate Admin Console - Visual Gateway · 拓扑渲染
 * 从 console-traffic.js 拆出：增量 DOM Diff 拓扑图渲染与节点高亮追踪。
 */

// 6. 增量 DOM Diff 渲染拓扑图 (零销毁、零闪烁、平滑位移)
function renderTrafficTopology(data, animate = false) {
  const container = document.getElementById('traffic-topology-canvas-container');
  if (!container) return;

  const records = data.records || data.recent_requests || [];
  const topology = data.topology || { nodes: [], links: [] };
  const hasNodes = Array.isArray(topology.nodes) && topology.nodes.length > 0;

  if (records.length === 0 && !hasNodes) {
    const emptyMsg = (data.range && data.range !== '5m') ? '所选时间区间内暂无流量记录' : '暂无实时流量记录';
    container.innerHTML = `
      <div style="padding:48px 16px;text-align:center;color:var(--text-muted);">
        <div style="display:inline-flex;padding:12px;border-radius:12px;background:rgba(59,130,246,0.06);color:#3b82f6;margin-bottom:12px;">
          ${TRAFFIC_ICONS.layers}
        </div>
        <div style="font-size:14px;font-weight:600;color:var(--text-main);margin-bottom:4px;">${emptyMsg}</div>
        <div style="font-size:12px;color:var(--text-muted);max-width:420px;margin:0 auto;">网关正在侦听多用户调用，产生真实请求后全链路流向图将在此呈现。</div>
      </div>
    `;
    return;
  }

  const colIPs = (topology.nodes || []).filter(n => n.type === 'client_ip').slice(0, 10);
  const colModels = (topology.nodes || []).filter(n => n.type === 'model').slice(0, 10);
  const colChannels = (topology.nodes || []).filter(n => n.type === 'channel').slice(0, 10);
  const colStatuses = (topology.nodes || []).filter(n => n.type === 'status').slice(0, 10);

  const maxNodes = Math.max(colIPs.length, colModels.length, colChannels.length, colStatuses.length, 1);
  const svgHeight = Math.max(260, maxNodes * 56 + 48);
  const nodeW = 168;
  const nodeH = 38;
  const nodePositions = {};

  // 各品牌 path 的有效图形尺寸差异极大（gateway 波形线实测仅 14x6，deepseek 10.5x6.6，
  // stepfun 铺满 16x16）。统一 viewBox 盒子只能让"容器"等大，里面的图形仍然大小不一。
  // 这里在插入 DOM 前，用一次性离屏 svg 量出原始 bbox，再把每个子元素包一层 transform，
  // 使有效图形占满画布的 84% 并居中。用离屏测量是为了避开 getBBox 的时序陷阱：
  // 已带 transform 的子元素再次调用时会返回变换后的结果，导致二次缩放越缩越小。
  const ICON_NORM_TARGET = 16 * 0.84;
  const measureIconBBox = (iconMarkup) => {
    try {
      const probe = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
      probe.setAttribute('viewBox', '0 0 16 16');
      probe.setAttribute('width', '0');
      probe.setAttribute('height', '0');
      probe.style.cssText = 'position:absolute;left:-9999px;top:-9999px;';
      probe.innerHTML = iconMarkup;
      document.body.appendChild(probe);
      const b = probe.getBBox();
      probe.remove();
      if (!b.width || !b.height) return null;
      return { x: b.x, y: b.y, width: b.width, height: b.height };
    } catch (e) { return null; }
  };

  const normalizeNodeIcon = (iconSvg, rawIconMarkup) => {
    if (!iconSvg || iconSvg.tagName !== 'svg') return;
    if (iconSvg.getAttribute('data-icon-normalized') === '1') return;
    const b = measureIconBBox(rawIconMarkup || '');
    if (!b) return;
    // 归一化策略：让图形的短边铺满目标，视觉尺寸就齐平了。
    // 但不能只按短边算——gateway 波形线是 14x6 的宽图形，按短边（6）放大 2.24 倍后
    // 宽会变成 31.4，溢出图标盒并顶到旁边的节点文字。所以长边设 1.55 倍目标的上限：
    // 超限就退回到按长边算，牺牲一点高度换取不入侵文字区。
    const maxLongEdge = ICON_NORM_TARGET * 1.55;
    const scaleByShort = ICON_NORM_TARGET / Math.min(b.width, b.height);
    const scaleByLong = maxLongEdge / Math.max(b.width, b.height);
    const scale = Math.min(scaleByShort, scaleByLong);
    const cx = b.x + b.width / 2;
    const cy = b.y + b.height / 2;
    const dx = 8 - cx * scale;
    const dy = 8 - cy * scale;
    iconSvg.setAttribute('data-icon-normalized', '1');
    iconSvg.querySelectorAll('path, rect, circle, ellipse, line, polyline, polygon').forEach((el) => {
      el.setAttribute('transform', `translate(${dx.toFixed(3)}, ${dy.toFixed(3)}) scale(${scale.toFixed(3)})`);
    });
  };



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
        ? `<text class="node-title" x="36" y="18" fill="var(--text-main, #0f172a)" font-size="10.5" font-weight="700" font-family="var(--mono, monospace)">${escapeHtml(shortLabel)}</text>
           <text class="node-sub" x="36" y="30" fill="var(--text-muted, #64748b)" font-size="8.5" font-weight="500">${escapeHtml(subLocation)}</text>`
        : `<text class="node-title" x="36" y="24" fill="var(--text-main, #0f172a)" font-size="11" font-weight="600" font-family="var(--mono, monospace)">${escapeHtml(shortLabel)}</text>`;

      nodeGroup.innerHTML = `
        <rect class="node-box" width="${nodeW}" height="${nodeH}" rx="8" fill="${visual.bgColor}" stroke="${visual.borderColor}" stroke-width="1.2" />
        <svg class="node-icon" x="7" y="8" width="22" height="22" viewBox="0 0 16 16" overflow="visible" aria-hidden="true" focusable="false">
          ${visual.icon}
        </svg>
        ${textSvg}
        <rect x="${nodeW - 32}" y="10" width="24" height="18" rx="9" fill="rgba(15, 23, 42, 0.08)" />
        <text class="node-count-text" x="${nodeW - 20}" y="23" fill="var(--text-main, #0f172a)" font-size="9.5" font-weight="700" text-anchor="middle" font-family="var(--mono, monospace)">${item.count}</text>
      `;
      nodesLayer.appendChild(nodeGroup);
      const iconSvg = nodeGroup.querySelector(':scope > svg.node-icon');
      if (iconSvg) normalizeNodeIcon(iconSvg, visual.icon);
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

