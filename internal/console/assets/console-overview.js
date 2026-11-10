/**
 * Gate Admin Console - 总览页主逻辑
 * 从 console-overview.js 拆出保留：主控台卡片渲染、路由策略保存与启动装配。
 * 共享基础件与图表簇已拆入 console-overview-charts.js，符号经 window.GATE_OVW 传入。
 */
(() => {
  'use strict';

  const {
    PAGE_ID, $, escapeHtml, DEFAULT_WINDOW, getSavedWindow, state, authHeaders,
    setText, fmtInt, fmtPct, fmtWindow, fmtBucketSpan, verdictOf, sparkline,
    getOvwTooltip, renderThroughputChart, aggregateBuckets,
  } = window.GATE_OVW;

  function renderHero(facts) {
    const totals = facts?.totals;
    const verdict = verdictOf(totals);
    const node = $('ovw-verdict');
    if (node) {
      node.className = 'ovw-verdict' + (verdict.key === 'good' ? '' : ` is-${verdict.key}`);
      node.innerHTML = `<span class="dot"></span><span>${escapeHtml(verdict.label)}</span>`;
    }
    const bucketMinutes = facts?.bucket_minutes || totals?.bucket_minutes || 10;
    const bucketSpanText = fmtBucketSpan(bucketMinutes);

    setText('ovw-hero-value', fmtInt(totals?.total_requests));
    setText('ovw-hero-label', t('overview.runtimeHeroLabel'));
    setText('ovw-hero-meta', totals
      ? (totals.scope === 'cumulative'
        ? t('overview.runtimeCumulativeScopeNoTraffic', { value: fmtWindow(totals.window_minutes) })
        : `${t('overview.runtimeRollingWindow', { value: fmtWindow(totals.window_minutes) })} · ${bucketSpanText}`)
      : t('overview.runtimeAwaitingData'));
    setText('ovw-split-success', fmtInt(totals?.success_count));
    setText('ovw-split-failed', fmtInt(totals?.failure_count));

    const spark = $('ovw-hero-spark');
    if (spark) {
      const buckets = aggregateBuckets(facts);
      spark.innerHTML = buckets.length
        ? sparkline(buckets, { width: 260, height: 48 })
        : `<div class="ovw-empty" style="padding:12px 0">${t('overview.runtimeNoRequestVolume')}</div>`;
    }
  }

  function renderStats(facts) {
    const holder = $('ovw-stats');
    if (!holder) return;
    const totals = facts?.totals;
    const providers = facts?.providers || [];

    const successRate = totals?.success_rate ?? null;
    const rateClass = successRate == null ? '' : (successRate >= 99 ? ' is-good' : successRate >= 95 ? '' : ' is-bad');
    const providerKeys = providers.reduce((a, p) => a + (p.key_count || 0), 0);
    // 桶全空时服务端退回累计口径（scope=cumulative），提示语必须跟着换，
    // 否则会拿「窗口内共 N 次」去描述一个累计值。
    const scopeCumulative = totals?.scope === 'cumulative';

    const tiles = [
      {
        label: t('stats.successRate'),
        value: fmtPct(successRate),
        cls: rateClass,
        hint: totals
          ? (scopeCumulative
            ? t('overview.runtimeCumulativeNoTraffic', { value: fmtInt(totals.total_requests) })
            : `${t('stats.rangeLastPrefix')} ${fmtWindow(totals.window_minutes).replace(`${t('stats.rangeLastPrefix')} `, '')}${t('overview.runtimeRequestCountSuffix', { value: fmtInt(totals.total_requests) })}`)
          : t('stats.statusNoData'),
        meter: successRate == null ? null : Math.max(0, Math.min(100, successRate))
      },
      {
        label: t('overview.runtimeCredentialsLabel'),
        value: fmtInt(totals?.credentials_total),
        hint: totals ? t('overview.runtimeCredentialsStatusHint', { credentials_active: totals.credentials_active, credentials_disabled: totals.credentials_disabled }) : t('stats.statusNoData'),
        meter: null
      },
      {
        label: t('overview.runtimeProviderKeysLabel'),
        value: fmtInt(providerKeys),
        hint: providers.length ? t('overview.runtimeProviderCount', { count: providers.length }) : t('stats.statusNoData'),
        meter: null
      },
      {
        label: t('overview.runtimeCooldownLabel'),
        value: fmtInt(totals?.credentials_cooldown),
        cls: totals?.credentials_cooldown > 0 ? ' is-bad' : '',
        hint: totals?.credentials_blocked > 0 ? t('overview.runtimeNotSchedulableHint', { credentials_blocked: totals.credentials_blocked }) : t('overview.runtimeAllSchedulableHint'),
        meter: null
      }
    ];

    holder.innerHTML = tiles.map(t => `<div class="ovw-stat">
      <div class="ovw-stat-label">${escapeHtml(t.label)}</div>
      <div class="ovw-stat-value${t.cls || ''}">${escapeHtml(t.value)}</div>
      ${t.hint ? `<div class="ovw-stat-hint">${escapeHtml(t.hint)}</div>` : ''}
      ${t.meter != null ? `<div class="ovw-meter"><div class="ovw-meter-fill" style="width:${t.meter}%"></div></div>` : ''}
    </div>`).join('');
  }

  function renderFleet(facts) {
    const holder = $('ovw-fleet');
    if (!holder) return;
    const providers = facts?.providers || [];
    if (providers.length === 0) {
      holder.innerHTML = `<div class="ovw-empty">${t('overview.runtimeProviderNoTraffic')}</div>`;
      return;
    }
    const max = Math.max(1, ...providers.map(p => p.total || 0));
    holder.innerHTML = providers.map(p => {
      const rate = p.success_rate;
      const rateCls = rate == null ? '' : (rate >= 99 ? ' is-good' : rate >= 95 ? '' : ' is-bad');
      return `<div class="ovw-fleet-row">
        <div class="ovw-fleet-name" title="${escapeHtml(p.provider)}">${escapeHtml(p.provider)}</div>
        ${sparkline(p.buckets, { width: 300, height: 30, stroke: p.total >= max ? '#38bdf8' : '#a5c8e6' })}
        <div class="ovw-fleet-numbers">
          ${t('overview.runtimeProviderRequestSuffix', { value: fmtInt(p.total) })} <span class="ovw-fleet-rate${rateCls}">${fmtPct(rate)}</span>
        </div>
      </div>`;
    }).join('');
  }

  function readConfigValue(cfg, ...keys) {
    for (const k of keys) {
      const v = cfg?.[k];
      if (v !== undefined && v !== null && v !== '') return v;
    }
    return null;
  }

  // 选项文案取 CPA 面板的短标签：这一行只有四列栅格，长标签会把下拉框挤到裁切
  const ROUTING_STRATEGIES = [
    { value: 'round-robin', label: t('overview.runtimeStrategyRoundRobin') },
    { value: 'weighted-round-robin', label: t('overview.runtimeStrategyWeightedRoundRobin') },
    { value: 'fill-first', label: t('overview.runtimeStrategyFillFirst') },
    { value: 'smart', label: t('overview.runtimeStrategySmart') },
    { value: 'pace', label: t('overview.runtimeStrategyPace') }
  ];

  // 路由配置可编辑的前提与只读区块一致：管理员已配密钥，且不是成员视角
  function canEditRouting() {
    if (typeof isMemberAuth === 'function' && isMemberAuth()) return false;
    if (typeof getAdminSecret === 'function' && !getAdminSecret()) return false;
    return true;
  }

  function notify(message, type) {
    if (typeof showToast === 'function') showToast(message, type);
  }

  async function saveRouting(patch, control) {
    // 保存中禁用控件，避免重复提交把策略连写几次
    if (control) control.disabled = true;
    try {
      const res = await fetch('/console/api/upstream-routing', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', ...authHeaders() },
        body: JSON.stringify(patch)
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(body.error || `HTTP ${res.status}`);
      state.config = { ...(state.config || {}), routing: { ok: true, value: body.routing } };
      notify(t('route.runtimeConfigSaved'), 'success');
      return true;
    } catch (err) {
      // 回滚到服务端真值：界面不能停在「以为改了其实没改」的状态
      notify(t('overview.runtimeRoutingSaveFailed', { msg: err.message }), 'error');
      await fetchRuntimeConfig();
      return false;
    } finally {
      if (control) control.disabled = false;
    }
  }

  function renderRuntime() {
    const holder = $('ovw-runtime-grid');
    if (!holder) return;
    const cfg = state.config?.config;
    if (!cfg?.ok) {
      holder.innerHTML = `<div class="ovw-empty">${t('overview.runtimeConfigUnavailable', { value: state.config?.config?.error ? '：' + escapeHtml(state.config.config.error) : '' })}</div>`;
      return;
    }
    const v = cfg.value || {};
    const routing = state.config?.routing;
    const r = routing?.ok ? (routing.value || {}) : {};
    const version = state.config?.version?.ok ? (state.config.version.value?.['latest-version'] || t('overview.runtimeUnknownValue')) : t('overview.runtimeUnknownValue');
    const proxyUrl = readConfigValue(v, 'proxy-url');
    const editable = canEditRouting();

    const onOff = x => x === true
      ? `<span class="ovw-runtime-value is-on">${t('overview.runtimeSwitchOn')}</span>`
      : x === false
        ? `<span class="ovw-runtime-value is-off">${t('cpa.closeLabel')}</span>`
        : '<span class="ovw-runtime-value">--</span>';

    // 路由三项从纯文本升级为可编辑控件。routing 不可用时退回只读占位，
    // 不给一个改了也保存不上去的假输入框。
    let routingCells;
    if (!routing?.ok) {
      routingCells = [t('route.runtimeStrategyLabel'), '<span class="ovw-runtime-value is-off">--</span>'];
    } else if (!editable) {
      const label = ROUTING_STRATEGIES.find(s => s.value === r.strategy)?.label || r.strategy || t('overview.runtimeUnknownValue');
      routingCells = [t('route.runtimeStrategyLabel'), `<span class="ovw-runtime-value">${escapeHtml(label)}</span>`];
    } else {
      const options = ROUTING_STRATEGIES
        .map(s => `<option value="${escapeHtml(s.value)}"${s.value === r.strategy ? ' selected' : ''}>${escapeHtml(s.label)}</option>`)
        .join('');
      routingCells = [t('route.runtimeStrategyLabel'), `
        <div class="ovw-dropdown-wrap">
          <div class="dropdown ovw-routing-dropdown" id="ovw-routing-dropdown" aria-label="${t('route.runtimeStrategyLabel')}"></div>
          <select id="ovw-routing-strategy" style="display:none;" aria-hidden="true">${options}</select>
        </div>`];
    }

    const rows = [
      routingCells,
      [t('overview.runtimeRequestRetryLabel'), escapeHtml(String(readConfigValue(v, 'request-retry') ?? '--'))],
      [t('overview.runtimeLatestServerVersionLabel'), escapeHtml(String(version))],
      [t('overview.runtimeUpstreamProxyLabel'), proxyUrl ? escapeHtml(String(proxyUrl)) : `<span class="ovw-runtime-value is-off">${t('quotas.notConfigured')}</span>`],
      [t('overview.runtimeDebugModeLabel'), onOff(v.debug)],
      [t('overview.runtimeFileLogLabel'), onOff(v['logging-to-file'])],
      [t('cpa.settingRequestLog'), onOff(v['request-log'])],
      [t('cpa.settingWsAuth'), onOff(v['ws-auth'])],
      [t('overview.runtimeModelPrefixLabel'), onOff(v['force-model-prefix'])],
      [t('cpa.settingUsageStatistics'), onOff(v['usage-statistics-enabled'])]
    ];

    // 会话粘性两项只在可编辑时出现；只读视角下并入路由策略的说明文案，
    // 免得给成员看一排没用的空格子。
    if (editable && routing?.ok) {
      rows.splice(1, 0,
        [t('overview.runtimeSessionAffinityLabel'), `<label class="ovw-switch"><input type="checkbox" id="ovw-routing-affinity" aria-label="${t('overview.runtimeAffinityAria')}"${r.session_affinity ? ' checked' : ''}/><span>${r.session_affinity ? t('overview.runtimeSwitchOn') : t('cpa.closeLabel')}</span></label>`],
        [t('overview.runtimeSessionAffinityTtlLabel'), `<input class="ovw-control" id="ovw-routing-ttl" type="number" min="0" step="1" inputmode="numeric" placeholder="${t('overview.runtimeLeaveBlankToDisable')}" aria-label="${t('overview.runtimeAffinityTtlAria')}" value="${escapeHtml(String(r.session_affinity_ttl ?? ''))}"/>`]
      );
    }

    holder.innerHTML = rows.map(([label, value]) =>
      `<div class="ovw-runtime-row"><span class="ovw-runtime-label">${escapeHtml(label)}</span>${value}</div>`).join('');

    // 初始化/同步 Aozora UI 统一 CustomDropdown 组件
    if (window.CustomDropdown && $('ovw-routing-dropdown')) {
      const inst = new window.CustomDropdown({
        containerId: 'ovw-routing-dropdown',
        options: ROUTING_STRATEGIES,
        value: r.strategy || 'round-robin',
        menuClass: 'menu-right',
        onChange: async val => {
          const hiddenSelect = $('ovw-routing-strategy');
          if (hiddenSelect) hiddenSelect.value = val;
          const ok = await saveRouting({ strategy: val });
          if (ok) renderRuntime();
        }
      });
      if (window.dropdownInstances) {
        window.dropdownInstances['ovw-routing-dropdown'] = inst;
      }
    }
  }

  // 事件委托挂在 workspace 上：控件随 renderRuntime 重建，绑在控件本身会丢
  function bindRoutingControls() {
    const root = $('ovw-runtime-grid');
    if (!root) return;
    root.addEventListener('change', async event => {
      const target = event.target;
      if (!(target instanceof HTMLElement)) return;
      if (target.id === 'ovw-routing-strategy') {
        await saveRouting({ strategy: target.value }, target);
        renderRuntime();
      } else if (target.id === 'ovw-routing-affinity') {
        const on = target.checked;
        if (await saveRouting({ session_affinity: on }, target)) {
          const label = target.querySelector('span');
          if (label) label.textContent = on ? t('overview.runtimeSwitchOn') : t('cpa.closeLabel');
        }
      } else if (target.id === 'ovw-routing-ttl') {
        await saveRouting({ session_affinity_ttl: target.value.trim() }, target);
      }
    });
  }

  function renderBanner() {
    const holder = $('ovw-banner');
    if (!holder) return;
    const facts = state.facts;
    if (facts?.degraded) {
      holder.className = 'ovw-banner is-error';
      holder.innerHTML = `<strong>${t('overview.runtimeCpaUnreachable')}</strong><span>${escapeHtml(facts.error || t('overview.runtimeUpstreamApiUnreachable'))}${t('overview.runtimeDegradedDataNotice')}</span>`;
      holder.hidden = false;
      return;
    }
    if (facts?.providers_derived) {
      holder.className = 'ovw-banner';
      holder.innerHTML = `<strong>${t('overview.runtimeProviderDerivedTitle')}</strong><span>${t('overview.runtimeProviderDerivedNote')}</span>`;
      holder.hidden = false;
      return;
    }
    holder.hidden = true;
  }

  function renderSourceTag() {
    const holder = $('ovw-source-tag');
    if (!holder) return;
    const facts = state.facts;
    if (!facts) { holder.innerHTML = ''; return; }
    if (facts.degraded) {
      holder.className = 'ovw-source-tag is-warn';
      holder.innerHTML = t('overview.runtimeCpaDegradedTag');
    } else {
      holder.className = 'ovw-source-tag';
      holder.innerHTML = t('overview.runtimeCpaFactsTag');
    }
  }

  function renderWindowSwitcher() {
    const holder = $('ovw-window-switcher');
    if (!holder) return;
    const available = state.facts?.available_windows || [
      { id: '1h', short_label: '1h', label: t('overview.runtimeWindow1hLabel') },
      { id: '6h', short_label: '6h', label: t('overview.runtimeWindow6hLabel') },
      { id: '1d', short_label: '1d', label: t('overview.runtimeWindow1dLabel') },
      { id: '3d', short_label: '3d', label: t('overview.runtimeWindow3dLabel') },
      { id: '7d', short_label: '7d', label: t('stats.range7Days') },
      { id: '30d', short_label: '30d', label: t('stats.range30Days') }
    ];

    holder.innerHTML = available.map(w => {
      const isActive = (state.window || DEFAULT_WINDOW) === w.id;
      const titleLabel = (w.window_minutes ? fmtWindow(w.window_minutes) : '') || w.label || w.id;
      return `<button type="button" class="ovw-window-btn ${isActive ? 'active' : ''}" data-window-id="${escapeHtml(w.id)}" title="${escapeHtml(titleLabel)}">${escapeHtml(w.short_label || w.id)}</button>`;
    }).join('');

    holder.querySelectorAll('.ovw-window-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        const targetId = btn.dataset.windowId;
        if (!targetId || targetId === state.window) return;
        state.window = targetId;
        try {
          localStorage.setItem('gate_ovw_window', targetId);
        } catch (_) {}
        renderWindowSwitcher();
        fetchRuntimeFacts(true);
      });
    });
  }

  function renderAll() {
    renderBanner();
    renderSourceTag();
    renderWindowSwitcher();
    renderHero(state.facts);
    renderStats(state.facts);
    // 动态更新实时流量描述
    const bucketMins = state.facts?.bucket_minutes || state.facts?.totals?.bucket_minutes || 10;
    setText('ovw-chart-desc', `${t('overview.runtimeThroughputPrefix')} ${fmtBucketSpan(bucketMins).replace(`${t('overview.runtimeBucketSpanPrefix')} `, '')}${t('overview.runtimeThroughputSuffix')}`);

    // 热更新悬停保护：若用户正在查看吞吐图柱子的 Tooltip，推迟吞吐图 SVG 重建，避免悬停被打断闪烁
    const tip = getOvwTooltip();
    const isTipActive = tip && tip.classList.contains('is-visible');
    if (!isTipActive) {
      renderThroughputChart(state.facts);
    }
    renderFleet(state.facts);
    renderRuntime();
    const at = state.facts?.fetched_at;
    setText('ovw-updated', at ? (typeof formatCstTime === 'function' ? formatCstTime(at) : new Date(at).toLocaleTimeString(LOCALE, { hour12: false, timeZone: 'Asia/Shanghai' })) : '--');
  }

  async function fetchRuntimeFacts(force = false) {
    if (state.loading) return;
    if (typeof isMemberAuth === 'function' && isMemberAuth()) return;
    if (typeof getAdminSecret === 'function' && !getAdminSecret()) return;
    state.loading = true;
    try {
      if (!state.config) fetchRuntimeConfig();
      const currentWin = encodeURIComponent(state.window || DEFAULT_WINDOW);
      const url = `/console/api/runtime-facts?window=${currentWin}` + (force ? '&refresh=1' : '');
      const res = await fetch(url, { headers: authHeaders() });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      state.facts = await res.json();
      state.lastError = null;
    } catch (err) {
      state.lastError = err.message;
      console.warn('[Gate] runtime-facts error:', err.message);
    } finally {
      state.loading = false;
      renderAll();
    }
  }

  async function fetchRuntimeConfig() {
    if (typeof isMemberAuth === 'function' && isMemberAuth()) return;
    if (typeof getAdminSecret === 'function' && !getAdminSecret()) return;
    try {
      const res = await fetch('/console/api/runtime-config', { headers: authHeaders() });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      state.config = await res.json();
    } catch (err) {
      console.warn('[Gate] runtime-config error:', err.message);
      state.config = { config: { ok: false, error: err.message } };
    }
    renderRuntime();
  }

  function mount() {
    const nav = document.getElementById('main-nav-tabs');
    const workspace = document.getElementById('workspace');
    if (!nav || !workspace) return;

    // 切换语言后需要重建一次性注入的模板（侧栏按钮 + 工作区骨架），否则仍是旧语言文案。
    if (typeof onLocaleRebuild === 'function') {
      onLocaleRebuild(function () {
        const navBtn = document.getElementById(`tab-btn-${PAGE_ID}`);
        if (navBtn) {
          const label = navBtn.querySelector('span');
          if (label) label.textContent = t('overview.runtimeTitle');
        }
        if (typeof pageMeta === 'object' && pageMeta) {
          pageMeta[PAGE_ID] = {
            title: 'overview.runtimeTitle',
            eyebrow: 'RUNTIME OVERVIEW',
            description: 'overview.runtimeDescription'
          };
        }
        renderAll();
      });
    }

    // 侧栏首位；运行总览是进入工作台的第一屏（首屏切换见 promoteToFirstScreen）。
    nav.insertAdjacentHTML('afterbegin', `<button class="nav-tab-btn" id="tab-btn-${PAGE_ID}" onclick="switchNavTab('${PAGE_ID}')" type="button">
      <svg fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M3 13h8V3H3v10zm0 8h8v-6H3v6zm10 0h8V11h-8v10zm0-18v6h8V3h-8z" stroke-linecap="round" stroke-linejoin="round"></path></svg>
      <span data-i18n="overview.runtimeTitle">${t('overview.runtimeTitle')}</span>
    </button><div class="sidebar-section-divider"></div>`);

    workspace.insertAdjacentHTML('beforeend', `
<div class="tab-page ovw-workspace" id="tab-page-${PAGE_ID}">
  <div class="ovw-panel ovw-hero">
    <div class="ovw-hero-main">
      <div class="ovw-verdict" id="ovw-verdict"><span class="dot"></span><span data-i18n="stats.statusWaitingData">${t('stats.statusWaitingData')}</span></div>
      <div class="ovw-hero-figure">
        <span class="ovw-hero-value" id="ovw-hero-value">--</span>
        <span class="ovw-hero-label" id="ovw-hero-label" data-i18n="overview.runtimeHeroLabel">${t('overview.runtimeHeroLabel')}</span>
      </div>
      <div class="ovw-hero-meta" id="ovw-hero-meta" data-i18n="overview.runtimeAwaitingData">${t('overview.runtimeAwaitingData')}</div>
      <div class="ovw-hero-split">
        <span class="ovw-split-item"><i class="ovw-swatch is-success"></i><span data-i18n="overview.runtimeSuccessLabel">${t('overview.runtimeSuccessLabel')}</span> <span class="ovw-split-value" id="ovw-split-success">--</span></span>
        <span class="ovw-split-item"><i class="ovw-swatch is-failure"></i><span data-i18n="overview.runtimeFailureLabel">${t('overview.runtimeFailureLabel')}</span> <span class="ovw-split-value" id="ovw-split-failed">--</span></span>
      </div>
    </div>
    <div class="ovw-hero-spark" id="ovw-hero-spark"></div>
  </div>

  <div class="ovw-banner" id="ovw-banner" hidden></div>

  <div class="ovw-panel">
    <div class="ovw-panel-head">
      <div>
        <div class="ovw-eyebrow">Core metrics</div>
        <h3 class="ovw-title" data-i18n="overview.runtimeKeyMetricsTitle">${t('overview.runtimeKeyMetricsTitle')}</h3>
      </div>
      <div style="display:flex;align-items:center;gap:10px">
        <span class="ovw-source-tag" id="ovw-source-tag"></span>
        <span class="ovw-countdown" id="ovw-updated">--</span>
      </div>
    </div>
    <div class="ovw-stats" id="ovw-stats"></div>
  </div>

  <div class="ovw-panel">
    <div class="ovw-panel-head" style="align-items:center;">
      <div>
        <div class="ovw-eyebrow">Live traffic</div>
        <h3 class="ovw-title" data-i18n="overview.runtimeLiveTrafficTitle">${t('overview.runtimeLiveTrafficTitle')}</h3>
        <p class="ovw-desc" id="ovw-chart-desc" data-i18n="overview.runtimeLiveTrafficSubtitle">${t('overview.runtimeLiveTrafficSubtitle')}</p>
      </div>
      <div class="ovw-window-switcher" id="ovw-window-switcher" role="tablist" aria-label="${t('overview.runtimeWindowSwitcherAria')}"></div>
    </div>
    <div id="ovw-chart-holder"></div>
  </div>

  <div class="ovw-panel">
    <div class="ovw-panel-head">
      <div>
        <div class="ovw-eyebrow">Providers</div>
        <h3 class="ovw-title" data-i18n="overview.runtimeProviderCompareTitle">${t('overview.runtimeProviderCompareTitle')}</h3>
      </div>
    </div>
    <div class="ovw-fleet" id="ovw-fleet"></div>
  </div>

  <div class="ovw-panel">
    <div class="ovw-panel-head">
      <div>
        <div class="ovw-eyebrow">Runtime</div>
        <h3 class="ovw-title" data-i18n="overview.runtimeAtGlanceTitle">${t('overview.runtimeAtGlanceTitle')}</h3>
      </div>
    </div>
    <div class="ovw-runtime-grid" id="ovw-runtime-grid"></div>
  </div>
</div>`);

    // 供 admin.html 的导航与刷新流程调用。
    window.ovwRuntime = {
      fetch: fetchRuntimeFacts,
      fetchConfig: fetchRuntimeConfig,
      pageId: PAGE_ID
    };

    if (typeof pageMeta === 'object' && pageMeta) {
      pageMeta[PAGE_ID] = {
        title: 'overview.runtimeTitle',
        eyebrow: 'RUNTIME OVERVIEW',
        description: 'overview.runtimeDescription'
      };
    }

    promoteToFirstScreen();
    bindRoutingControls();
    fetchRuntimeConfig();
  }

  // 把运行总览提为默认首屏。
  //
  // 放在本文件而不是改 admin.html 的 activeNavTab 初值，是为了让「脚本没加载成功」
  // 这一情形仍然退回到原有 quotas 首屏，而不是停在一个半初始化的工作台上。
  // admin.html 结尾的 applyAuthModeUI() + switchNavTab() 先于本脚本执行，
  // 因此这里是在其后再次切页，两次切换都是幂等的 DOM 状态赋值。
  function promoteToFirstScreen() {
    // 成员看板与未配置管理密钥的用户看不到全站上游调度事实，保持 quotas 首屏
    const isMember = typeof isMemberAuth === 'function' && isMemberAuth();
    const hasSecret = typeof getAdminSecret === 'function' && !!getAdminSecret();
    if (isMember || !hasSecret) return;
    if (typeof switchNavTab !== 'function' || !pageMeta || !pageMeta[PAGE_ID]) return;
    // 只接管首屏；用户之后自己切走再刷新，仍尊重当前所在页
    if (document.body.dataset.activeTab && document.body.dataset.activeTab !== 'quotas') return;
    switchNavTab(PAGE_ID);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', mount);
  else mount();
})();
