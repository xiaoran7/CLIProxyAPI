/**
 * Gate Admin Console - Request Logs · 数据拉取与筛选
 * 从 console-logs.js 拆出：流水拉取、Key/服务商/鉴权维度筛选与跳转。
 */

    // 4. 获取并渲染最近请求 (服务端真分页与检索，支持前端 SWR 缓存与非阻塞并发)
    async function fetchEventsData(force = false) {
      // 首次取数（尚无用例行）时先铺骨架行，取数完成后原地替换，避免布局跳动。
      const eventsTbody = document.getElementById('events-tbody');
      if (eventsTbody && !eventsTbody.querySelector('tr[data-id]') && typeof window.renderSkeletonTableBody === 'function') {
        window.renderSkeletonTableBody(eventsTbody, logsPageSize, 15);
      }
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
      if (logsSelectedIP) params.set('client_ip', logsSelectedIP);
      if (logsSelectedModel) {
        params.set('search', logsSelectedModel);
      } else if (logsSearchQuery) {
        params.set('search', logsSearchQuery);
      }
      if (logsTimeBufferMinTs > 0 && logsTimeBufferMaxTs > 0 && (logsTimeMinPercent > 0 || logsTimeMaxPercent < 100)) {
        const span = logsTimeBufferMaxTs - logsTimeBufferMinTs;
        const sinceTs = Math.round(logsTimeBufferMinTs + span * (logsTimeMinPercent / 100));
        const untilTs = Math.round(logsTimeBufferMinTs + span * (logsTimeMaxPercent / 100));
        params.set('since', String(sinceTs));
        params.set('until', String(untilTs));
      }

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

        // 维护滑动条两端极值时间戳
        if (incomingEvents.length > 0) {
          const timestamps = incomingEvents.map(e => e.ts > 1e11 ? Math.floor(e.ts / 1000) : e.ts).filter(t => t > 0);
          if (timestamps.length > 0) {
            const minTs = Math.min(...timestamps);
            const maxTs = Math.max(...timestamps);
            if (logsTimeBufferMinTs === 0 || minTs < logsTimeBufferMinTs) logsTimeBufferMinTs = minTs;
            if (logsTimeBufferMaxTs === 0 || maxTs > logsTimeBufferMaxTs) logsTimeBufferMaxTs = maxTs;

            const labelStart = document.getElementById('logs-time-label-start');
            const labelEnd = document.getElementById('logs-time-label-end');
            if (labelStart) labelStart.textContent = new Date(logsTimeBufferMinTs * 1000).toLocaleTimeString();
            if (labelEnd) labelEnd.textContent = new Date(logsTimeBufferMaxTs * 1000).toLocaleTimeString();
            updateLogsSliderProgressUI();
          }
        }

        if (logsPage === 1 && (!eventFilter || eventFilter === 'all') && (!logsKeyFilter || logsKeyFilter === 'all') && (!logsProviderFilter || logsProviderFilter === 'all') && (!logsTimeRange || logsTimeRange === 'all') && (!logsAuthFilter || logsAuthFilter === 'all') && !logsSearchQuery && !logsSelectedIP && !logsSelectedModel) {
          try {
            localStorage.setItem(KEY_STORAGE_LOGS, JSON.stringify({ events: cachedEvents.slice(0, 50), total: cachedEventsTotal, ts: Date.now() }));
          } catch {}
        }
        if (!isMember) {
          updateLogsKeySelect(cachedEvents);
        }
        updateLogsProviderSelect(cachedEvents);
        updateLogsIPSelect(cachedEvents);
        updateLogsModelSelect(cachedEvents);

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

    function updateLogsIPSelect(events = []) {
      if (!dropdownInstances['logs-ip-dropdown']) return;
      const ipSet = new Set();
      for (const ev of events) {
        if (ev.client_ip && ev.client_ip !== '-' && ev.client_ip !== '') {
          ipSet.add(ev.client_ip);
        }
      }
      const options = [{ value: '', label: '全部客户端 IP' }];
      for (const ip of Array.from(ipSet).sort()) {
        options.push({ value: ip, label: ip });
      }
      dropdownInstances['logs-ip-dropdown'].updateOptions(options, logsSelectedIP);
    }

    function updateLogsModelSelect(events = []) {
      if (!dropdownInstances['logs-model-dropdown']) return;
      const modelSet = new Set();
      for (const ev of events) {
        if (ev.model) modelSet.add(ev.model);
        if (ev.request_model) modelSet.add(ev.request_model);
      }
      const options = [{ value: '', label: '全部请求模型' }];
      for (const m of Array.from(modelSet).sort()) {
        options.push({ value: m, label: m });
      }
      dropdownInstances['logs-model-dropdown'].updateOptions(options, logsSelectedModel);
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

