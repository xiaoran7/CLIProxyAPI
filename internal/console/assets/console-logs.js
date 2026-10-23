/**
 * Gate Admin Console - Request Logs & Error Auditing Module
 * 包含请求流水真实分页、服务商反查与推断、错误与熔断中心审计
 */

    // 4. 获取并渲染最近请求 (服务端真分页与检索，支持前端 SWR 缓存与非阻塞并发)
    async function fetchEventsData(force = false) {
      const isMember = isMemberAuth();
      // 后台静默预加载 Key 列表，不阻塞当前流水请求发起
      if (!isMember && cachedKeys.length === 0) {
        ensureKeysLoaded().catch(() => {});
      }
      const params = new URLSearchParams({
        page: String(logsPage),
        page_size: String(logsPageSize)
      });
      if (eventFilter && eventFilter !== 'all') params.set('filter', eventFilter);
      if (logsKeyFilter && logsKeyFilter !== 'all') params.set('key_id', logsKeyFilter);
      if (logsProviderFilter && logsProviderFilter !== 'all') params.set('provider', logsProviderFilter);
      if (logsTimeRange && logsTimeRange !== 'all') params.set('time_range', logsTimeRange);
      if (!isMember && logsAuthFilter && logsAuthFilter !== 'all') params.set('auth_index', logsAuthFilter);
      if (logsSearchQuery) params.set('search', logsSearchQuery);

      const cacheKey = `${isMember ? 'member' : 'admin'}:${params.toString()}`;
      const cached = getLogsQueryCache(cacheKey);
      if (cached && !force) {
        cachedEvents = cached.data.events || [];
        cachedEventsTotal = Number.isFinite(cached.data.total) ? cached.data.total : cachedEvents.length;
        if (Number.isFinite(cached.data?.today_total) && typeof updateNavBadgeLogs === 'function') {
          updateNavBadgeLogs(cached.data.today_total);
        }
        renderEventsTable(cachedEvents, cachedEventsTotal);
        if (!isMember) updateLogsKeySelect(cachedEvents);
        updateLogsProviderSelect(cachedEvents);
        // 如果缓存极新鲜 (< 4s)，直接返回，实现瞬时翻页体验
        if (Date.now() - cached.ts < 4000) return;
      }

      const baseUrl = isMember ? '/api/member/requests' : '/console/api/requests';
      const url = `${baseUrl}?${params.toString()}`;
      try {
        const res = await fetch(url, { headers: getAuthHeaders() });
        if (!res.ok) return;
        const data = await res.json();
        setLogsQueryCache(cacheKey, data);
        const incomingEvents = data.events || [];
        const incomingTotal = Number.isFinite(data.total) ? data.total : incomingEvents.length;

        // 识别是否有新入流水，双重交叉采样现有 DOM 第一行与内存首条 ID
        const firstDomRow = document.getElementById('events-tbody')?.querySelector('tr[data-id]');
        const domFirstId = firstDomRow ? firstDomRow.dataset.id : null;
        const oldFirstId = domFirstId != null ? domFirstId : (cachedEvents[0]?.id != null ? String(cachedEvents[0].id) : null);
        const newlyAddedIds = new Set();
        if (oldFirstId != null && incomingEvents.length > 0 && String(incomingEvents[0]?.id) !== String(oldFirstId)) {
          for (const ev of incomingEvents) {
            if (String(ev.id) === String(oldFirstId)) break;
            newlyAddedIds.add(ev.id);
          }
        }

        const isExactSame = (cachedEventsTotal === incomingTotal) &&
          (cachedEvents.length === incomingEvents.length) &&
          (incomingEvents.length > 0 && cachedEvents[0]?.id === incomingEvents[0]?.id && cachedEvents[0]?.latency_ms === incomingEvents[0]?.latency_ms && cachedEvents[0]?.status === incomingEvents[0]?.status);

        cachedEvents = incomingEvents;
        cachedEventsTotal = incomingTotal;
        if (Number.isFinite(data?.today_total) && typeof updateNavBadgeLogs === 'function') {
          updateNavBadgeLogs(data.today_total);
        }
        if (logsPage === 1 && (!eventFilter || eventFilter === 'all') && (!logsKeyFilter || logsKeyFilter === 'all') && (!logsProviderFilter || logsProviderFilter === 'all') && (!logsTimeRange || logsTimeRange === 'all') && (!logsAuthFilter || logsAuthFilter === 'all') && !logsSearchQuery) {
          try {
            localStorage.setItem(KEY_STORAGE_LOGS, JSON.stringify({ events: cachedEvents.slice(0, 50), total: cachedEventsTotal, ts: Date.now() }));
          } catch {}
        }
        if (!isMember) {
          updateLogsKeySelect(cachedEvents);
        }
        updateLogsProviderSelect(cachedEvents);

        // 热加载：若数据一模一样，静默更新分页器文字，绝不销毁重建 DOM，杜绝抖动与选区丢失
        if (isExactSame && !force) {
          const pageInfo = document.getElementById('logs-page-info');
          const totalPages = Math.max(1, Math.ceil(cachedEventsTotal / logsPageSize));
          if (pageInfo) pageInfo.innerText = t('logs.cachedEventsPageCounter', { logsPage: logsPage, totalPages: totalPages, cachedEventsTotal: cachedEventsTotal });
        } else {
          renderEventsTable(cachedEvents, cachedEventsTotal, newlyAddedIds.size > 0 ? newlyAddedIds : null);
        }
      } catch (err) {
        console.warn('Fetch events error:', err.message);
      }
    }

    // ---- 实时流水色彩与筛选控制 ----
    const KEY_COLOR_PALETTES = [
      { bg: 'rgba(56, 189, 248, 0.1)', border: 'rgba(56, 189, 248, 0.3)', color: '#38bdf8' },
      { bg: 'rgba(168, 85, 247, 0.1)', border: 'rgba(168, 85, 247, 0.3)', color: '#c084fc' },
      { bg: 'rgba(52, 211, 153, 0.1)', border: 'rgba(52, 211, 153, 0.3)', color: '#34d399' },
      { bg: 'rgba(251, 191, 36, 0.1)', border: 'rgba(251, 191, 36, 0.3)', color: '#fbbf24' },
      { bg: 'rgba(244, 63, 94, 0.1)', border: 'rgba(244, 63, 94, 0.3)', color: '#fb7185' },
      { bg: 'rgba(99, 102, 241, 0.1)', border: 'rgba(99, 102, 241, 0.3)', color: '#818cf8' },
      { bg: 'rgba(20, 184, 166, 0.1)', border: 'rgba(20, 184, 166, 0.3)', color: '#2dd4bf' },
      { bg: 'rgba(249, 115, 22, 0.1)', border: 'rgba(249, 115, 22, 0.3)', color: '#fb923c' }
    ];

    function getKeyStyle(keyId, displayKey) {
      let hash = 0;
      const str = String(keyId != null ? keyId : (displayKey || 'unknown'));
      for (let i = 0; i < str.length; i++) {
        hash = (hash << 5) - hash + str.charCodeAt(i);
        hash |= 0;
      }
      const idx = Math.abs(hash) % KEY_COLOR_PALETTES.length;
      return KEY_COLOR_PALETTES[idx];
    }

    function updateLogsKeySelect(events) {
      const keyMap = new Map();
      // 先从 cachedKeys 中收集
      if (Array.isArray(cachedKeys)) {
        for (const k of cachedKeys) {
          keyMap.set(String(k.key_id), { id: String(k.key_id), alias: k.key_alias || '', display: k.display_key || '' });
        }
      }
      // 再从流水中收集未知 key
      for (const ev of events) {
        const id = ev.key_id != null ? String(ev.key_id) : (ev.display_key || '');
        if (id && !keyMap.has(id)) {
          keyMap.set(id, { id, alias: ev.key_alias || '', display: ev.display_key || '' });
        }
      }

      const options = [{ value: 'all', label: t('logs.allKeysOption') }];
      for (const [id, info] of keyMap.entries()) {
        const label = info.alias ? `${info.alias} (${info.display || id})` : (info.display || `Key #${id}`);
        options.push({ value: id, label });
      }

      if (dropdownInstances['logs-key-dropdown']) {
        dropdownInstances['logs-key-dropdown'].updateOptions(options, logsKeyFilter);
      }
    }

    function changeLogsKeyFilter(val) {
      logsKeyFilter = val;
      logsPage = 1;
      fetchEventsData();
    }

    function changeLogsProviderFilter(val) {
      logsProviderFilter = val || 'all';
      logsPage = 1;
      fetchEventsData();
    }

    function filterBySpecificProvider(provider) {
      if (!provider) return;
      logsProviderFilter = String(provider).toLowerCase();
      if (dropdownInstances['logs-provider-dropdown']) {
        dropdownInstances['logs-provider-dropdown'].setValue(logsProviderFilter, false);
      }
      logsPage = 1;
      fetchEventsData();
      const brand = getProviderBrand(logsProviderFilter);
      showToast(t('logs.filteredByProvider', { value: brand.name || logsProviderFilter }), 'success');
    }

    function updateLogsProviderSelect(events = []) {
      if (!dropdownInstances['logs-provider-dropdown']) return;
      const provMap = new Map();
      const presetProviders = [
        { value: 'antigravity', label: t('logs.providerAntigravity') },
        { value: 'codex', label: 'OpenAI Codex' },
        { value: 'gemini', label: 'Google Gemini' },
        { value: 'claude', label: 'Anthropic Claude' },
        { value: 'stepfun', label: t('logs.providerStepFun') },
        { value: 'opencode', label: 'OpenCode Zen' },
        { value: 'workbuddy', label: t('logs.providerWorkBuddy') },
        { value: 'workbuddy-cn', label: 'WorkBuddy (CodeBuddy)' },
        { value: 'deepseek', label: 'DeepSeek' },
        { value: 'qoder-cn', label: t('logs.providerQoderCn') },
        { value: 'qoder', label: t('logs.providerQoder') },
        { value: 'zcode', label: t('provider.zcode') },
        { value: 'zcode-cn', label: t('provider.zcodeCn') },
        { value: 'openrouter', label: 'OpenRouter' },
        { value: 'qwen', label: t('logs.providerQwen') },
        { value: 'moonshot', label: 'Moonshot Kimi' },
        { value: 'zhipu', label: 'Zhipu GLM' },
        { value: 'stealth', label: 'stealth' },
        { value: 'other', label: t('logs.providerOther') }
      ];
      for (const p of presetProviders) {
        provMap.set(p.value, p.label);
      }
      if (typeof cachedUpstream !== 'undefined' && Array.isArray(cachedUpstream?.accounts)) {
        for (const acc of cachedUpstream.accounts) {
          if (acc?.provider && acc.provider !== 'unknown' && !provMap.has(acc.provider)) {
            const brand = getProviderBrand(acc.provider, acc.display_name || acc.name);
            provMap.set(acc.provider, brand.name || acc.provider);
          }
        }
      }
      if (Array.isArray(events)) {
        for (const ev of events) {
          const p = resolveEventProvider(ev);
          if (p && !provMap.has(p)) {
            const brand = getProviderBrand(p);
            provMap.set(p, brand.name || p);
          }
        }
      }
      const options = [{ value: 'all', label: t('logs.allProvidersOption') }];
      for (const [val, label] of provMap.entries()) {
        options.push({ value: val, label });
      }
      dropdownInstances['logs-provider-dropdown'].updateOptions(options, logsProviderFilter);
    }

    function changeLogsTimeRange(val) {
      logsTimeRange = val || 'all';
      logsPage = 1;
      fetchEventsData();
    }

    function changeLogsPageSize(size) {
      logsPageSize = Number(size) || 25;
      logsPage = 1;
      fetchEventsData();
    }

    function filterBySpecificKey(keyIdOrDisplay) {
      logsKeyFilter = String(keyIdOrDisplay);
      if (dropdownInstances['logs-key-dropdown']) {
        dropdownInstances['logs-key-dropdown'].setValue(logsKeyFilter, false);
      }
      logsPage = 1;
      fetchEventsData();
      showToast(t('logs.filteredByKey'), 'success');
    }

    function onLogsSearch(query) {
      clearTimeout(logsSearchTimer);
      logsSearchTimer = setTimeout(() => {
        logsSearchQuery = String(query || '').trim();
        logsPage = 1;
        fetchEventsData();
      }, 300);
    }

    function changeLogsLimit(limit) {
      logsPage = 1;
      fetchEventsData();
    }

    function prevLogsPage() {
      if (logsPage > 1) {
        logsPage--;
        fetchEventsData();
      }
    }

    function nextLogsPage() {
      const totalPages = Math.max(1, Math.ceil(cachedEventsTotal / logsPageSize));
      if (logsPage < totalPages) {
        logsPage++;
        fetchEventsData();
      }
    }

    function resolveEventProvider(ev) {
      if (ev?.provider) return ev.provider;
      // 1. 尝试从已同步凭据列表中反查提供商
      if (ev?.auth_index) {
        const cleanIdx = String(ev.auth_index).trim();
        if (typeof cachedUpstream !== 'undefined' && Array.isArray(cachedUpstream?.accounts)) {
          const acc = cachedUpstream.accounts.find(a => a.auth_index === cleanIdx);
          if (acc?.provider && acc.provider !== 'unknown' && acc.provider !== 'other') return acc.provider;
        }
        if (typeof cachedUpstreamStats !== 'undefined' && Array.isArray(cachedUpstreamStats?.items)) {
          const it = cachedUpstreamStats.items.find(i => i.auth_index === cleanIdx);
          if (it?.provider && it.provider !== 'unknown' && it.provider !== 'other') return it.provider;
        }
      }
      // 2. 模型维度推断：与 lib/core/provider-registry.mjs 权威规则全面同口径
      const m = String(ev?.model || '').toLowerCase();
      if (m.includes('antigravity')) return 'antigravity';
      if (m.includes('opencode') || m.includes('-free')) return 'opencode';
      if (m.startsWith('step') || m.includes('stepfun')) return 'stepfun';
      if (m.startsWith('claude') || m.includes('anthropic')) return 'claude';
      if (m.startsWith('gemini')) return 'gemini';
      if (m.startsWith('gpt') || m.startsWith('chatgpt') || m.startsWith('o1') || m.startsWith('o3') || m.startsWith('o4') || m.startsWith('text-embedding')) return 'codex';
      if (m.includes('stealth')) return 'stealth';
      if (m.includes('deepseek')) return 'deepseek';
      if (m.includes('openrouter')) return 'openrouter';
      if (m.includes('qwen') || m.includes('qianwen')) return 'qwen';
      if (m.includes('kimi') || m.includes('moonshot')) return 'moonshot';
      if (m.includes('glm') || m.includes('zhipu')) return 'zhipu';
      if (m.includes('workbuddy') || m.includes('codebuddy') || m.includes('copilot.tencent')) return 'workbuddy';
      if (m.includes('qoder-cn') || m.includes('lingma-cn')) return 'qoder-cn';
      if (m.includes('qoder') || m.includes('lingma')) return 'qoder';
      return '';
    }

    function getAuthDisplayName(authIndex) {
      if (!authIndex) return '';
      const str = String(authIndex).trim();
      if (typeof cachedUpstream !== 'undefined' && cachedUpstream && Array.isArray(cachedUpstream.accounts)) {
        const acc = cachedUpstream.accounts.find(a => a.auth_index === str);
        if (acc) {
          const name = acc.display_name || acc.nickname || acc.email || acc.filename || str;
          return String(name).replace(/\.json$/i, '');
        }
      }
      if (typeof cachedUpstreamStats !== 'undefined' && cachedUpstreamStats && Array.isArray(cachedUpstreamStats.items)) {
        const it = cachedUpstreamStats.items.find(i => i.auth_index === str);
        if (it) {
          const name = it.display_name || it.nickname || it.name || str;
          return String(name).replace(/\.json$/i, '');
        }
      }
      return str;
    }

    function filterBySpecificAuth(authIndex) {
      if (!authIndex) return;
      logsAuthFilter = String(authIndex);
      logsPage = 1;
      updateAuthFilterNotice();
      fetchEventsData();
      const name = getAuthDisplayName(logsAuthFilter);
      showToast(t('logs.filteredByUpstreamAccount', { name: name }), 'success');
    }

    function clearAuthFilter() {
      logsAuthFilter = 'all';
      logsPage = 1;
      updateAuthFilterNotice();
      fetchEventsData();
    }

    function updateAuthFilterNotice() {
      const notice = document.getElementById('logs-auth-filter-notice');
      const nameEl = document.getElementById('logs-auth-filter-name');
      if (!notice) return;
      if (logsAuthFilter && logsAuthFilter !== 'all') {
        const name = getAuthDisplayName(logsAuthFilter);
        if (nameEl) nameEl.innerText = name;
        notice.style.display = 'inline-flex';
      } else {
        notice.style.display = 'none';
      }
    }

    function filterEvents(filter) {
      eventFilter = filter;
      logsPage = 1;
      const tabs = document.getElementById('events-filter-tabs').querySelectorAll('button');
      tabs.forEach(btn => btn.classList.toggle('active', btn.getAttribute('onclick')?.includes(`'${filter}'`)));
      fetchEventsData();
    }

    window.jumpToErrorLogs = function(timeRangeHint, failCount) {
      switchNavTab('logs');
      filterEvents('errors');
      if (typeof showToast === 'function') {
        const msg = failCount > 0
          ? `${t('logs.jumpedToErrorLogs')}${timeRangeHint ? ` ${t('logs.timeRangeFailHintSuffix', { timeRangeHint: timeRangeHint, failCount: failCount })}` : ''}`
          : `${t('logs.jumpedToErrorLogs')}${timeRangeHint ? ` ${t('logs.timeRangeHintSuffix', { timeRangeHint: timeRangeHint })}` : ''}`;
        showToast(msg, 'info');
      }
    };

    function renderEventRowHtml(ev, isNew = false) {
      const rowClassAttr = isNew ? ' class="live-row-enter live-row-highlight"' : '';
      const tsMs = ev.ts > 1e11 ? ev.ts : ev.ts * 1000;
      const d = new Date(tsMs);
      const timeStr = d.toLocaleTimeString(LOCALE, { hour12: false });
      const dateStr = `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
      const fullIso = Number.isNaN(d.getTime()) ? '' : d.toISOString();

      let statusClass = 'chip-green';
      if (ev.status === 429 || ev.status === 403) statusClass = 'chip-red';
      else if (ev.status >= 500) statusClass = 'chip-red';
      else if (ev.status >= 400) statusClass = 'chip-yellow';

      let cacheTag = '<span style="color:var(--text-dim);">—</span>';
      if (ev.cached_tokens > 0) {
        cacheTag = `<span class="chip chip-cache" title="${t('logs.promptCacheHitTitle', { value: ev.cached_tokens.toLocaleString(LOCALE) })} Tokens">${formatTokens(ev.cached_tokens)} (${ev.cache_hit_rate}%)</span>`;
      } else if (ev.status >= 200 && ev.status < 400 && (ev.prompt_tokens > 0 || ev.total_tokens > 0)) {
        cacheTag = `<span style="color:var(--text-dim); font-size:11.5px; font-family:var(--mono);" title="${t('logs.cacheMissTitle')}">0%</span>`;
      }

      const costTag = ev.cost_usd > 0 ? `<span class="num" style="color:#34d399;">${formatCost(ev.cost_usd)}</span>` : '<span style="color:var(--text-dim);">$0.00</span>';

      const flags = [];
      if (ev.stream) flags.push(`<span class="flag-pill">${t('logs.streamFlag')}</span>`);
      if (ev.estimated) flags.push(`<span class="flag-pill">${t('logs.estimatedFlag')}</span>`);
      if (ev.error_type) {
        flags.push(`<span class="flag-pill" style="background:#fef2f2;color:#ef4444;border-color:#fecaca;font-weight:600;" title="${t('logs.errorTypeTitle', { value: escapeHtml(errorTypeLabel(ev.error_type)) })}">${escapeHtml(errorTypeLabel(ev.error_type))}</span>`);
      }

      const totalLatStr = ev.latency_ms >= 1000 ? (ev.latency_ms / 1000).toFixed(1) + 's' : ev.latency_ms + 'ms';
      const ttftStr = ev.ttft_ms ? (ev.ttft_ms >= 1000 ? (ev.ttft_ms / 1000).toFixed(1) + 's' : ev.ttft_ms + 'ms') : '-';
      const latencyHtml = `<div class="num" style="font-weight:500;">${totalLatStr}</div>` +
        (ev.stream && ev.ttft_ms ? `<div style="font-size:10.5px; color:var(--text-dim);">${t('logs.ttftLabel', { ttftStr: ttftStr })}</div>` : '');

      const ks = getKeyStyle(ev.key_id, ev.display_key);
      const filterKeyTarget = ev.key_id != null ? ev.key_id : ev.display_key;
      const mainKeyText = ev.key_alias || ev.display_key;
      const subKeyText = ev.key_alias ? ev.display_key : '—';

      // 上游承接账号
      const shortAuth = ev.auth_index ? ev.auth_index.slice(0, 8) : '';
      let authName = ev.auth_name || (ev.auth_index ? getAuthDisplayName(ev.auth_index) : '') || '';
      if (authName && typeof authName === 'string') {
        authName = authName.replace(/\.json$/i, '');
      }
      const authBrand = getProviderBrand(resolveEventProvider(ev), authName);
      if (ev.auth_index && (!authName || authName === ev.auth_index)) {
        authName = (authBrand && authBrand.id !== 'other')
          ? t('logs.channelByShortAuth', { name: authBrand.name, shortAuth: shortAuth })
          : t('logs.deletedAccountByShortAuth', { shortAuth: shortAuth });
      }
      let authTdHtml = '<span style="color:var(--text-dim); font-size:11.5px;">—</span>';
      if (ev.auth_index) {
        authTdHtml = `
          <div style="cursor:pointer;" onclick="filterBySpecificAuth('${escapeHtml(ev.auth_index)}')" title="${t('logs.filterByUpstreamAccountHint')}">
            <div style="display:flex; align-items:center; gap:5px;">
              <span class="provider-pill ${authBrand.pillClass}" style="font-size:9.5px; padding:0 4px; cursor:pointer;" onclick="event.stopPropagation(); filterBySpecificProvider('${escapeHtml(authBrand.id)}');" title="${t('logs.filterByProviderHint')}">${escapeHtml(authBrand.name)}</span>
              <span style="font-weight:600; color:var(--text-main); font-size:11.5px; max-width:160px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${escapeHtml(authName || ev.auth_index)}">
                ${escapeHtml(authName || ev.auth_index)}
              </span>
            </div>
            <div style="font-size:10px; font-family:var(--mono); color:var(--text-dim); margin-top:1px;">ID: ${escapeHtml(shortAuth)}</div>
          </div>
        `;
      }

      const statusTitle = ev.status >= 400
        ? `HTTP ${ev.status}${ev.error_type ? ` · ${errorTypeLabel(ev.error_type)}` : ''}`
        : `HTTP ${t('logs.httpSuccessStatus', { status: ev.status })}`;

      // 思考强度标签 (Reasoning Effort / Thinking Budget)
      let reasoningHtml = '<span style="color:var(--text-dim); font-size:11.5px;">—</span>';
      if (ev.reasoning_effort) {
        reasoningHtml = `<span class="chip chip-purple" title="${t('logs.reasoningEffortTitle', { value: escapeHtml(ev.reasoning_effort) })}">${escapeHtml(ev.reasoning_effort)}</span>`;
      }

      // 速率 (Tokens/s 生成吞吐)
      let tpsHtml = '<span style="color:var(--text-dim); font-size:11.5px;">—</span>';
      const tpsNum = Number(ev.tokens_per_sec);
      if (Number.isFinite(tpsNum) && tpsNum > 0) {
        let speedColor = '#0284c7';
        if (tpsNum >= 100) speedColor = '#10b981';
        else if (tpsNum >= 50) speedColor = '#06b6d4';
        else if (tpsNum < 20) speedColor = '#f59e0b';
        tpsHtml = `<span class="speed-badge" style="color:${speedColor}; font-weight:600; font-size:11.5px;" title="${t('logs.generationRateTitle', { tpsNum: tpsNum })}">${tpsNum} <span style="font-size:10px; font-weight:400; opacity:0.8;">tok/s</span></span>`;
      }

      return `<tr${rowClassAttr} data-id="${escapeHtml(String(ev.id || ''))}">
        <td class="num" style="color:var(--text-dim); font-size:11.5px;" title="${escapeHtml(fullIso)}">
          <div class="live-cell-content"><span style="color:var(--text-muted); margin-right:3px;">${dateStr}</span>${timeStr}</div>
        </td>
        <td>
          <div class="live-cell-content">
            <div class="key-badge" style="--key-bg:${ks.bg}; --key-border:${ks.border}; --key-color:${ks.color};"
                 data-target="${escapeHtml(String(filterKeyTarget))}"
                 onclick="filterBySpecificKey(this.dataset.target)"
                 title="${t('logs.filterByKeyHint')}">
              <span>${escapeHtml(mainKeyText)}</span>
            </div>
            <div class="key-display" style="font-size:10px; color:var(--text-dim); margin-top:2px; height:14px; line-height:14px;">${escapeHtml(subKeyText)}</div>
          </div>
        </td>
        <td><div class="live-cell-content">${authTdHtml}</div></td>
        <td><div class="live-cell-content"><span class="tag-pill" title="${escapeHtml(ev.model)}">${escapeHtml(ev.model || '-')}</span></div></td>
        <td><div class="live-cell-content">${reasoningHtml}</div></td>
        <td><div class="live-cell-content"><span class="chip ${statusClass}" title="${escapeHtml(statusTitle)}">${ev.status}</span></div></td>
        <td><div class="live-cell-content">${tpsHtml}</div></td>
        <td class="num"><div class="live-cell-content">${(ev.prompt_tokens || 0).toLocaleString(LOCALE)} / <b>${(ev.completion_tokens || 0).toLocaleString(LOCALE)}</b></div></td>
        <td class="num"><div class="live-cell-content">${cacheTag}</div></td>
        <td><div class="live-cell-content">${costTag}</div></td>
        <td><div class="live-cell-content">${latencyHtml}</div></td>
        <td><div class="live-cell-content">${flags.join('') || '—'}</div></td>
      </tr>`;
    }

    function renderEventsTable(events, total, newlyAddedIds = null) {
      const pageEvents = Array.isArray(events) ? events : [];
      const totalFiltered = Number.isFinite(total) ? total : pageEvents.length;
      const totalPages = Math.max(1, Math.ceil(totalFiltered / logsPageSize));
      if (logsPage > totalPages) logsPage = totalPages;

      const pageInfo = document.getElementById('logs-page-info');
      const prevBtn = document.getElementById('logs-prev-btn');
      const nextBtn = document.getElementById('logs-next-btn');
      if (pageInfo) pageInfo.innerText = t('logs.pageCounter', { logsPage: logsPage, totalPages: totalPages, totalFiltered: totalFiltered });
      if (prevBtn) prevBtn.disabled = logsPage <= 1;
      if (nextBtn) nextBtn.disabled = logsPage >= totalPages;

      const tbody = document.getElementById('events-tbody');
      if (!tbody) return;
      if (totalFiltered === 0 || pageEvents.length === 0) {
        tbody.innerHTML = `<tr><td colspan="12" style="text-align: center; color: var(--text-dim); padding: 32px;">${t('logs.emptyFiltered')}</td></tr>`;
        return;
      }

      // 平滑无感增量插入：当在第1页且检测到有新增行时，仅向顶部平滑插入新行，未变动行保持不动
      const hasExistingRows = tbody.children.length > 0 && !tbody.querySelector('td[colspan]');
      if (logsPage === 1 && newlyAddedIds && newlyAddedIds.size > 0 && hasExistingRows) {
        const eventsToInsert = pageEvents.filter(ev => newlyAddedIds.has(ev.id) && !tbody.querySelector(`tr[data-id="${ev.id}"]`));
        if (eventsToInsert.length > 0) {
          const temp = document.createElement('tbody');
          let newRowsHtml = '';
          for (const ev of eventsToInsert) {
            newRowsHtml += renderEventRowHtml(ev, true);
          }
          temp.innerHTML = newRowsHtml;
          const rowsToInsert = Array.from(temp.children);
          for (let i = rowsToInsert.length - 1; i >= 0; i--) {
            tbody.insertBefore(rowsToInsert[i], tbody.firstElementChild);
          }
          while (tbody.children.length > logsPageSize) {
            tbody.lastElementChild.remove();
          }
          setTimeout(() => {
            rowsToInsert.forEach(row => {
              row.classList.remove('live-row-enter');
              row.classList.remove('live-row-highlight');
            });
          }, 2600);
          return;
        }
      }

      let html = '';
      for (const ev of pageEvents) {
        const isNewRow = newlyAddedIds && newlyAddedIds.has(ev.id);
        html += renderEventRowHtml(ev, isNewRow);
      }
      tbody.innerHTML = html;
      if (newlyAddedIds && newlyAddedIds.size > 0) {
        setTimeout(() => {
          tbody.querySelectorAll('.live-row-enter, .live-row-highlight').forEach(row => {
            row.classList.remove('live-row-enter');
            row.classList.remove('live-row-highlight');
          });
        }, 2600);
      }
    }

    function errorTypeLabel(type) {
      const map = {
        quota_exceeded: t('logs.errorQuotaExceeded'),
        rate_limited: t('logs.errorRateLimited'),
        model_forbidden: t('logs.errorModelForbidden'),
        empty_response: t('logs.errorEmptyResponse'),
        stream_interrupted: t('logs.errorStreamInterrupted'),
        connection_error: t('logs.errorConnectionLost'),
        stream_loop_detected: t('logs.errorThinkingLoopBreaker'),
        payload_too_large: t('logs.errorPayloadTooLarge'),
        upstream_validation_required: t('logs.errorUpstreamVerification')
      };
      return map[type] || type;
    }

    // ---- 错误与熔断中心 (Error Events & Circuit Breaker, 支持 SWR 缓存) ----
    async function fetchErrorsData(force = false) {
      const secret = getAdminSecret();
      if (!secret) return;
      const params = new URLSearchParams({
        page: String(errorsPage),
        page_size: String(errorsPageSize)
      });
      if (errorsTimeRange && errorsTimeRange !== 'all') params.set('time_range', errorsTimeRange);

      const cacheKey = params.toString();
      const cached = getErrorsQueryCache(cacheKey);
      if (cached && !force) {
        cachedErrors = cached.data.errors || [];
        cachedErrorsTotal = Number.isFinite(cached.data.total) ? cached.data.total : cachedErrors.length;
        if (Number.isFinite(cached.data?.today_total) && typeof updateNavBadgeErrors === 'function') {
          updateNavBadgeErrors(cached.data.today_total);
        }
        renderErrorsTable(cachedErrors, cachedErrorsTotal);
        if (Date.now() - cached.ts < 4000) return;
      }

      try {
        const res = await fetch(`/console/api/errors?${params.toString()}`, { headers: getAuthHeaders() });
        if (!res.ok) return;
        const data = await res.json();
        setErrorsQueryCache(cacheKey, data);
        cachedErrors = data.errors || [];
        cachedErrorsTotal = Number.isFinite(data.total) ? data.total : cachedErrors.length;
        if (Number.isFinite(data?.today_total) && typeof updateNavBadgeErrors === 'function') {
          updateNavBadgeErrors(data.today_total);
        }
        if (errorsPage === 1 && (!errorsTimeRange || errorsTimeRange === 'all')) {
          try {
            localStorage.setItem(KEY_STORAGE_ERRORS, JSON.stringify({ errors: cachedErrors.slice(0, 50), total: cachedErrorsTotal, ts: Date.now() }));
          } catch {}
        }
        renderErrorsTable(cachedErrors, cachedErrorsTotal);
      } catch (err) {
        console.warn('Errors fetch error:', err.message);
      }
    }

    function prevErrorsPage() {
      if (errorsPage > 1) {
        errorsPage--;
        fetchErrorsData();
      }
    }

    function nextErrorsPage() {
      const totalPages = Math.max(1, Math.ceil(cachedErrorsTotal / errorsPageSize));
      if (errorsPage < totalPages) {
        errorsPage++;
        fetchErrorsData();
      }
    }

    function changeErrorsPageSize(size) {
      errorsPageSize = Number(size) || 25;
      errorsPage = 1;
      fetchErrorsData();
    }

    function changeErrorsTimeRange(val) {
      errorsTimeRange = val || 'all';
      errorsPage = 1;
      fetchErrorsData();
    }

    function renderErrorsTable(errors, total) {
      const totalFiltered = Number.isFinite(total) ? total : (errors ? errors.length : 0);
      const totalPages = Math.max(1, Math.ceil(totalFiltered / errorsPageSize));
      if (errorsPage > totalPages) errorsPage = totalPages;

      const pageInfo = document.getElementById('errors-page-info');
      const prevBtn = document.getElementById('errors-prev-btn');
      const nextBtn = document.getElementById('errors-next-btn');
      if (pageInfo) pageInfo.innerText = t('logs.errorsPageCounter', { errorsPage: errorsPage, totalPages: totalPages, totalFiltered: totalFiltered });
      if (prevBtn) prevBtn.disabled = errorsPage <= 1;
      if (nextBtn) nextBtn.disabled = errorsPage >= totalPages;

      const tbody = document.getElementById('errors-tbody');
      if (!tbody) return;
      if (!errors || errors.length === 0 || totalFiltered === 0) {
        tbody.innerHTML = `<tr><td colspan="8" style="text-align: center; color: var(--text-dim); padding: 32px;">${t('logs.emptyIncidents')}</td></tr>`;
        return;
      }

      const now = Date.now();
      let html = '';
      for (const ev of errors) {
        // 兼容自适应秒级或毫秒级时间戳，杜绝重复乘 1000 导致时间错乱与溢出
        const tsMs = ev.ts > 1e11 ? ev.ts : ev.ts * 1000;
        const d = new Date(tsMs);
        const dateStr = `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
        const timeStr = d.toLocaleTimeString(LOCALE, { hour12: false });
        const fullTimeStr = `${dateStr} ${timeStr}`;
        const keyInfo = ev.key_alias ? `${escapeHtml(ev.key_alias)} (#${ev.key_id || '—'})` : (ev.key_id ? `#${ev.key_id}` : '—');

        let statusClass = 'chip-red';
        if (ev.status === 429) statusClass = 'chip-yellow';

        // 熔断与退避倒计时
        let circuitStatus = `<span style="color:var(--text-dim);">${t('logs.statusEnded')}</span>`;
        if (ev.recover_at && ev.recover_at > now) {
          const remainSec = Math.ceil((ev.recover_at - now) / 1000);
          circuitStatus = `<span class="chip chip-red" style="font-weight:600;">${t('logs.circuitBrokenChip', { remainSec: remainSec })}</span>`;
        } else if (ev.retry_after_ms > 0) {
          circuitStatus = `<span class="chip chip-gray">${t('logs.backoffChip', { value: Math.round(ev.retry_after_ms / 1000) })}s</span>`;
        }

        // 上游账号与凭据身份直观展示
        const shortAuth = ev.auth_index ? ev.auth_index.slice(0, 16) : '';
        const shortId = ev.auth_index ? ev.auth_index.slice(0, 8) : '';
        let authName = ev.auth_name || (ev.auth_index ? getAuthDisplayName(ev.auth_index) : '') || '';
        if (authName && typeof authName === 'string') {
          authName = authName.replace(/\.json$/i, '');
        }
        const brand = getProviderBrand(resolveEventProvider(ev), authName);
        if (ev.auth_index && (!authName || authName === ev.auth_index)) {
          authName = (brand && brand.id !== 'other')
            ? t('logs.channelByShortId', { name: brand.name, shortId: shortId })
            : t('logs.deletedAccountByShortId', { shortId: shortId });
        }
        let authDisplay = '<span style="color:var(--text-dim);">—</span>';
        if (ev.auth_index) {
          authDisplay = `
            <div style="display:flex; flex-direction:column; gap:2px;">
              <div style="display:flex; align-items:center; gap:5px;">
                <span class="provider-pill ${brand.pillClass}" style="font-size:9.5px; padding:0 4px;">${escapeHtml(brand.name)}</span>
                <span style="font-weight:600; color:var(--text-main); font-size:12px; max-width:210px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${escapeHtml(authName || ev.auth_index)}">
                  ${escapeHtml(authName || ev.auth_index)}
                </span>
              </div>
              <div style="font-size:10.5px; font-family:var(--mono); color:var(--text-dim);">ID: ${escapeHtml(shortAuth)}</div>
            </div>
          `;
        }

        let bodySnippet = ev.error_body ? escapeHtml(ev.error_body.slice(0, 200)) + (ev.error_body.length > 200 ? '…' : '') : '—';

        if (ev.error_type === 'upstream_validation_required') {
          statusClass = 'chip-red';
          const urlMatch = (ev.error_body || '').match(/https:\/\/accounts\.google\.com\/signin\/continue\?[^\s"'\\]+/);
          if (urlMatch) {
            const rawUrl = urlMatch[0];
            bodySnippet = `<span style="color:var(--danger); font-weight:700;">${t('logs.googleVerificationBlocked')}</span> · <a href="${escapeHtml(rawUrl)}" target="_blank" rel="noopener noreferrer" style="color:#2563eb; font-weight:700; text-decoration:underline;">${t('logs.openVerificationPageLink')}</a>`;
          }
        }

        html += `<tr>
          <td class="num" style="color:var(--text-dim); font-size:11.5px; white-space:nowrap;" title="${d.toISOString()}">${fullTimeStr}</td>
          <td>${keyInfo}</td>
          <td>${authDisplay}</td>
          <td><span class="tag-pill">${escapeHtml(ev.model || '—')}</span></td>
          <td><span class="chip ${statusClass}">${ev.status}</span></td>
          <td><span class="flag-pill" style="${ev.error_type === 'upstream_validation_required' ? 'background:rgba(239,68,68,0.15); color:var(--danger); border-color:rgba(239,68,68,0.4);' : ''}">${escapeHtml(ev.error_type || 'error')}</span></td>
          <td>${circuitStatus}</td>
          <td style="max-width:320px; font-size:11px; color:var(--text-muted); line-height:1.4; word-break:break-all;">${bodySnippet}</td>
        </tr>`;
      }
      tbody.innerHTML = html;
    }
