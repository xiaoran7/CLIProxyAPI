/**
 * Gate Admin Console - Status & Analytics Dashboard Module
 * 包含 Uptime Kuma 风格服务心跳卡、免密探针、核心大卡统计与趋势图渲染
 */

    // 0.5 服务状态汇总 (Uptime Kuma 风格心跳卡)
    async function fetchGateStatus() {
      try {
        const url = isMemberAuth() ? '/api/member/status-summary' : '/console/api/status-summary';
        const res = await fetch(url, { headers: getAuthHeaders() });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        cachedStatusSummary = await res.json();
        renderStatusCards();

        // 统一更新侧边栏今日流水与今日故障徽标（仅展示当天的指标，杜绝混入历史全量）
        if (Number.isFinite(cachedStatusSummary?.today_requests)) {
          if (typeof updateNavBadgeLogs === 'function') {
            updateNavBadgeLogs(cachedStatusSummary.today_requests);
          }
        }
        if (Number.isFinite(cachedStatusSummary?.today_errors)) {
          if (typeof updateNavBadgeErrors === 'function') {
            updateNavBadgeErrors(cachedStatusSummary.today_errors);
          }
        }
      } catch (err) {
        cachedStatusSummary = null;
        renderOperationsStrip();
        document.getElementById('status-cards-container').innerHTML = `<div class="status-empty">${t('stats.statusUnavailable')}</div>`;
        console.warn('Status summary fetch error:', err.message);
      }
    }

    // 每次调用现取，保证切换语言后 formatter 的 locale 同步，锁定 Asia/Shanghai
    function heartbeatTimeFormatter() {
      return new Intl.DateTimeFormat(LOCALE, { timeZone: 'Asia/Shanghai', hour12: false, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
    }
    function renderOperationsStrip() {
      try {
        const accounts = upstreamLoaded ? cachedUpstream?.accounts : null;
        const overall = cachedStatusSummary?.overall;
        const availability = overall?.availability_7d_daily ?? overall?.availability_7d;
        const elTotal = document.getElementById('ops-total-accounts');
        if (elTotal) elTotal.textContent = Array.isArray(accounts) ? String(accounts.length) : '--';
        const elEnabled = document.getElementById('ops-enabled-accounts');
        if (elEnabled) elEnabled.textContent = Array.isArray(accounts) ? String(accounts.filter(a => !a.disabled && a.status !== 'disabled' && !a.health?.blocked).length) : '--';
        const elReq = document.getElementById('ops-requests-24h');
        if (elReq) elReq.textContent = Number.isFinite(Number(overall?.requests_24h)) && overall?.requests_24h != null ? formatTokens(overall.requests_24h) : '--';
        const elAvail = document.getElementById('ops-availability-7d');
        if (elAvail) elAvail.textContent = Number.isFinite(Number(availability)) && availability != null ? `${Number(availability).toFixed(1)}%` : '--';
      } catch (err) {
        console.warn('renderOperationsStrip non-fatal error:', err);
      }
    }
    function statusHeartbeatBarsHtml(bars, totalCount, timeLabelCache = new Map()) {
      const slotCount = Math.max(totalCount, bars.length, 1);
      const pad = slotCount - bars.length;
      let html = '';
      for (let i = 0; i < pad; i++) {
        html += `<i title="${t('stats.statusWaitingSlot')}"></i>`;
      }
      for (const b of bars) {
        let cls = 'ok';
        if (b.requests === 0) {
          cls = 'live';
        } else if (b.status >= 500 || b.status === 0) {
          cls = 'err';
        } else if (b.status >= 400) {
          cls = 'warn';
        }
        let when = timeLabelCache.get(b.ts);
        if (when === undefined) {
          const date = new Date(b.ts);
          when = Number.isNaN(date.getTime()) ? t('stats.statusUnknownTime') : heartbeatTimeFormatter().format(date);
          timeLabelCache.set(b.ts, when);
        }
        const tip = `${when} · ${b.requests > 0 ? t('stats.statusCallsSummary', { requests: b.requests, status: b.status }) : t('stats.statusAliveIdle')}${b.latency_ms ? ' · ' + b.latency_ms + 'ms' : ''}${b.ttft_ms ? ` ${t('status.heartbeatTtft')} ` + b.ttft_ms + 'ms' : ''}${b.model ? ' · ' + b.model : ''}`;
        html += `<i class="${cls}" title="${escapeHtml(tip)}"></i>`;
      }
      return html;
    }

    // 组级健康点文案与配色（与后端 buildProviderGroups 的判定口径保持一致）
    function statusGroupDot(healthDot, healthTitle) {
      const dot = healthDot || '';
      const title = healthTitle || t('stats.statusNoData');
      return { dot, title };
    }

    function renderStatusCards() {
      renderOperationsStrip();
      const container = document.getElementById('status-cards-container');
      if (!container) return;
      const accounts = cachedUpstream.accounts || [];
      const summary = cachedStatusSummary;
      if (!summary && accounts.length === 0) {
        container.innerHTML = `<div class="status-empty">${t('stats.statusNoDataYet')}</div>`;
        return;
      }

      const entities = [];

      // 1. 网关总入口
      const gateBrand = getProviderBrand('gate');
      entities.push({
        key: '__overall__',
        name: t('stats.gatewayMainEntry'),
        brand: gateBrand,
        kind: 'overall',
        stat: summary ? summary.overall : null,
        availability: summary ? (summary.overall.availability_7d_daily ?? summary.overall.availability_7d) : null,
        req24h: summary ? summary.overall.requests_24h : null,
        bars: summary && summary.overall ? (summary.overall.bars || []) : []
      });

      // 2. 按提供商分组。后端已把账号归组并给出组级健康点，
      //    前端只做展示，不再在这里二次遍历账号列表。
      const groups = (summary && Array.isArray(summary.provider_groups)) ? summary.provider_groups : [];
      for (const g of groups) {
        const provKey = g.provider || g.group_id;
        const brand = getProviderBrand(provKey, g.name);
        const health = statusGroupDot(g.health_dot, g.health_title);
        const meta = [];
        if (g.accounts > 0) {
          meta.push(t('stats.accountCountSuffix', { accounts: g.accounts }));
          if (g.disabled_accounts > 0) meta.push(t('stats.disabledAccountsSuffix', { disabled_accounts: g.disabled_accounts }));
        }
        entities.push({
          key: `group:${g.group_id}`,
          name: g.name,
          brand,
          kind: 'cpa',
          stat: { bars: g.bars || [], latency_avg_ms: g.latency_avg_ms, ttft_avg_ms: g.ttft_avg_ms, last_request_ts: g.last_request_ts },
          availability: g.availability_7d != null ? g.availability_7d : null,
          req24h: g.requests_24h != null ? g.requests_24h : null,
          req7d: g.requests_7d,
          bars: g.bars || [],
          disabled: g.enabled === false || g.status === 'disabled',
          statusKey: g.status === 'major_outage' ? 'outage' : (g.status === 'degraded' ? 'degraded' : (g.status === 'disabled' ? 'disabled' : (g.status === 'no_data' ? 'no_data' : (g.status === 'idle' ? 'idle' : 'normal')))),
          statusLabel: g.status === 'major_outage' ? t('stats.statusOutage') : (g.status === 'degraded' ? t('stats.statusDegraded') : (g.status === 'disabled' ? t('quotas.disabledBadge') : (g.status === 'no_data' ? t('stats.statusNoData') : (g.status === 'idle' ? t('stats.statusIdle') : t('stats.statusNormal'))))),
          meta,
          healthDot: health.dot,
          healthTitle: health.title
        });
      }

      const heartbeatTimeLabels = new Map();
      container.innerHTML = entities.map(e => {
        const stat = e.stat;
        const bars = Array.isArray(e.bars) ? e.bars : (stat && stat.bars) || [];
        // 健康点：组级优先用后端判定；仅在完全拿不到后端判定时才本地兜底。
        // 本地兜底必须与后端 deriveGroupHealth 保持同一口径：只看心跳窗口内
        // 有没有失败请求，绝不拿 7 天可用率判红判黄（小样本组会被一次历史
        // 失败永久钉成故障，生产上的 OpenCode Zen 组就这么误报了一整天）。
        let dotCls = e.healthDot || '';
        let dotTitle = e.healthTitle || t('stats.statusNoData');
        if (!dotCls && e.status !== 'no_data' && e.status !== 'idle' && bars.length > 0) {
          const windowRequests = bars.reduce((a, b) => a + (b.requests || 0), 0);
          if (windowRequests === 0) {
            const hasHistory = e.availability != null && e.req7d > 0;
            dotCls = hasHistory ? 'idle' : '';
            dotTitle = hasHistory ? t('stats.statusRecentNoTraffic') : t('stats.statusNoData');
          }
          else {
            const activeBars = bars.filter(b => b.requests > 0);
            const evalBars = activeBars.length > 0 ? activeBars.slice(-3) : bars.slice(-3);
            if (evalBars.some(b => b.status >= 500 || b.status === 0)) { dotCls = 'err'; dotTitle = t('stats.statusHasFailures'); }
            else if (evalBars.some(b => b.status >= 400 && b.status < 500)) { dotCls = 'warn'; dotTitle = t('stats.statusHas4xx'); }
            else { dotCls = 'ok'; dotTitle = t('keys.statusRunningOk'); }
          }
        }
        const availVal = e.availability;
        let availCls = '';
        if (availVal != null) availCls = availVal >= 99 ? '' : (availVal >= 95 ? 'warn' : 'err');
        const heartbeatCount = 60;
        const lastReq = stat && stat.last_request_ts ? new Date(stat.last_request_ts).toLocaleString(LOCALE, { timeZone: 'Asia/Shanghai', hour12: false, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '';
        const b = e.brand;

        // 延迟/TTFT 组内无明细时留空，不显示误导性的 --
        const latencyHtml = stat && Number.isFinite(stat.latency_avg_ms) && stat.latency_avg_ms > 0
          ? formatLatencyMs(stat.latency_avg_ms) : '--';
        const ttftHtml = stat && Number.isFinite(stat.ttft_avg_ms) && stat.ttft_avg_ms > 0
          ? formatLatencyMs(stat.ttft_avg_ms) : '--';

        const reqBadge = e.req24h != null
          ? `24h ${t('stats.requestCountSuffix', { value: Number(e.req24h).toLocaleString(LOCALE) })}`
          : (e.req7d != null ? `7d ${t('stats.requestCountSuffix', { value: Number(e.req7d).toLocaleString(LOCALE) })}` : '--');

        return `
          <div class="status-card ${e.disabled ? 'is-disabled' : ''}">
            <div class="status-card-header">
              <div class="status-card-title-wrap" title="${escapeHtml(e.name)}">
                <span class="status-dot ${dotCls}" title="${escapeHtml(dotTitle)}"></span>
                <div class="provider-icon-badge ${b.iconClass}" title="${escapeHtml(b.name)}">${b.iconSvg}</div>
                <div style="display:flex; flex-direction:column; min-width:0; gap:2px;">
                  <div style="display:flex; align-items:center; gap:6px; flex-wrap:wrap;">
                    <span class="provider-pill ${b.pillClass}">${escapeHtml(b.name)}</span>
                    ${e.statusLabel ? `<span class="chip ${e.statusKey === 'normal' ? 'chip-green' : (e.statusKey === 'degraded' ? 'chip-yellow' : (e.statusKey === 'disabled' ? 'chip-gray' : (e.statusKey === 'no_data' || e.statusKey === 'idle' ? 'chip-gray' : 'chip-red')))}" style="font-size:10px; padding:1px 5px; line-height:1.2; white-space:nowrap; flex-shrink:0;">${escapeHtml(e.statusLabel)}</span>` : ''}
                    ${e.disabled ? `<span class="chip chip-red" style="font-size:10px; padding:1px 5px; line-height:1.2; white-space:nowrap; flex-shrink:0;">${t('quotas.disabledBadge')}</span>` : ''}
                  </div>
                  <span class="status-card-name" style="max-width:180px;">${escapeHtml(e.name)}</span>
                </div>
              </div>
              <span class="status-req-badge">${escapeHtml(reqBadge)}</span>
            </div>
            ${e.meta && e.meta.length ? `<div style="display:flex; gap:6px; flex-wrap:wrap; font-size:10.5px; color:var(--text-dim);">${e.meta.map(m => `<span style="background:rgba(186,215,242,0.25); border:1px solid var(--card-border-subtle); border-radius:8px; padding:1px 6px;">${escapeHtml(m)}</span>`).join('')}</div>` : ''}
            <div class="status-metrics">
              <div class="status-metric-box">
                <div class="status-metric-label">
                  <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
                  ${t('status.conversationLatency')}
                </div>
                <div class="status-metric-value num">${latencyHtml}</div>
              </div>
              <div class="status-metric-box">
                <div class="status-metric-label">
                  <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
                  ${t('status.firstTokenTtft')}
                </div>
                <div class="status-metric-value num">${ttftHtml}</div>
              </div>
            </div>
            <div class="status-availability">
              <span class="status-availability-label">${t('status.availability7d')}</span>
              <span class="status-availability-value num ${availCls}">${availVal != null ? availVal.toFixed(2) : '--'}<i>%</i></span>
            </div>
            <div class="status-heartbeat">
              <div class="status-heartbeat-header">
                <span>${t('stats.heartbeatMinutes', { heartbeatCount: heartbeatCount })}${lastReq ? ` ${t('stats.lastCallSuffix', { value: escapeHtml(lastReq) })}` : ''}</span>
                <span class="status-refresh-countdown">${pollingIntervalMs > 0 ? t('status.cardRefreshCountdown') : t('stats.pollingPausedLabel')}</span>
              </div>
              <div class="heartbeat-bars">${statusHeartbeatBarsHtml(bars, heartbeatCount, heartbeatTimeLabels)}</div>
              <div class="status-heartbeat-footer"><span>-60m</span><span>NOW</span></div>
            </div>
          </div>
        `;
      }).join('');
    }

    // 1. 获取免密配额探针状态 (Uptime Kuma 接口)
    let probeAutoRetryTimer = null;
    async function fetchQuotaProbeStatus(isRetry = false) {
      const badge = document.getElementById('quota-probe-badge');
      const text = document.getElementById('quota-probe-text');
      try {
        const fetchOpts = {};
        if (typeof AbortSignal !== 'undefined' && typeof AbortSignal.timeout === 'function') {
          fetchOpts.signal = AbortSignal.timeout(8000);
        }
        const res = await fetch('/api/status/quota', fetchOpts);
        const data = await res.json();

        if (probeAutoRetryTimer) {
          clearTimeout(probeAutoRetryTimer);
          probeAutoRetryTimer = null;
        }

        if (badge) {
          badge.className = 'status-badge ' + (data.status === 'ok' ? '' : data.status);
          badge.title = t('stats.probeDetailsHint');
        }
        if (text) {
          if (data.status === 'ok') {
            text.innerText = t('stats.statusGatewayOk');
          } else if (data.status === 'warning') {
            text.innerText = (data.summary?.keys_warning || 0) + ` ${t('status.keyWarningCount')}`;
          } else if (data.status === 'blocked') {
            text.innerText = (data.summary?.keys_blocked || 0) + ` ${t('status.keyBlockedCount')}`;
          } else {
            text.innerText = t('stats.statusGatewayDegraded');
          }
        }

        // 渲染弹窗中的健康清单
        renderQuotaHealthList(data);
      } catch (err) {
        console.warn('[Gate] Quota probe error:', err.message);
        const isOffline = typeof navigator !== 'undefined' && navigator.onLine === false;
        if (badge) {
          badge.className = 'status-badge error';
          badge.title = isOffline ? t('stats.offlineAutoRetryNotice') : t('stats.probeCommError', { msg: err.message });
        }
        if (text) {
          text.innerText = isOffline ? t('stats.offlineLabel') : t('stats.probeRetryingLabel');
        }

        // 若网络瞬时抖动，启动 5 秒后台探测自愈，探测成功后自动同步全页数据
        if (!probeAutoRetryTimer) {
          probeAutoRetryTimer = setTimeout(async () => {
            probeAutoRetryTimer = null;
            await fetchQuotaProbeStatus(true);
            const curBadge = document.getElementById('quota-probe-badge');
            if (curBadge && !curBadge.classList.contains('error')) {
              refreshAll();
            }
          }, 5000);
        }
      }
    }

    function renderQuotaHealthList(probeData) {
      const list = document.getElementById('quota-modal-list');
      if (!list) return;
      if (!probeData || !probeData.summary) {
        list.innerHTML = `<div style="color:var(--text-dim); text-align:center;">${t('stats.statusNoData')}</div>`;
        return;
      }
      let html = '';
      const blockedList = Array.isArray(probeData.blocked_details)
        ? probeData.blocked_details
        : (Array.isArray(probeData.blocked_keys) ? probeData.blocked_keys : []);
      const warningList = Array.isArray(probeData.warning_details)
        ? probeData.warning_details
        : (Array.isArray(probeData.warning_keys) ? probeData.warning_keys : []);

      if (blockedList.length > 0) {
        html += `<div style="color:var(--danger); font-weight:600; margin-bottom:6px;">${t('status.hardCapBlockedList')}</div>`;
        for (const bk of blockedList) {
          html += `<div style="background:var(--danger-bg); border:1px solid var(--danger-border); padding:8px 12px; border-radius:6px; margin-bottom:6px; display:flex; justify-content:space-between;">
            <span><b>${escapeHtml(bk.alias || bk.name || 'Key')}</b> (#${bk.id || '—'})</span>
            <span class="num">${formatTokens(bk.used || 0)} / ${formatTokens(bk.hard_limit || bk.limit || 0)}</span>
          </div>`;
        }
      }
      if (warningList.length > 0) {
        html += `<div style="color:var(--warning); font-weight:600; margin-top:8px; margin-bottom:6px;">${t('status.nearLimitWarningList')}</div>`;
        for (const wk of warningList) {
          html += `<div style="background:var(--warning-bg); border:1px solid var(--warning-border); padding:8px 12px; border-radius:6px; margin-bottom:6px; display:flex; justify-content:space-between;">
            <span><b>${escapeHtml(wk.alias || wk.name || 'Key')}</b> (#${wk.id || '—'})</span>
            <span class="num">${formatTokens(wk.used || 0)} (${wk.percent || 0}%)</span>
          </div>`;
        }
      }
      if (!html) {
        html = `<div style="color:var(--success); text-align:center; padding:18px;">${t('status.allQuotasSafeNote')}</div>`;
      }
      list.innerHTML = html;
    }

    function openQuotaModal() {
      document.getElementById('probe-url-text').innerText = window.location.origin + '/api/status/quota';
      openModal('quota-modal');
    }
    function copyProbeUrl() {
      const url = document.getElementById('probe-url-text').innerText;
      copyToClipboard(url, t('stats.monitorLinkCopied'));
    }

    // 3. 获取统计图表与大卡数据
    async function fetchStatsData() {
      if (!isMemberAuth() && cachedKeys.length === 0) {
        ensureKeysLoaded().catch(() => {});
      }
      let url = '/console/api/stats?days=' + statsRangeDays;
      if (statsKeyFilter && statsKeyFilter !== 'all') {
        url += '&key_id=' + encodeURIComponent(statsKeyFilter);
      }
      try {
        const res = await fetch(url, { headers: getAuthHeaders() });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        cachedStats = data;
        renderSummaryCards(data.summary || {});
        renderMainChart(data);
        renderModelsList(data.models || []);
        renderProvidersList(data.providers || []);
      } catch (err) {
        document.getElementById('trend-chart').innerHTML = `<div class="trend-empty">${t('stats.usageUnavailable')}</div>`;
        console.warn('Usage stats fetch error:', err);
      }
    }

    function changeStatsKeyFilter(keyId) {
      statsKeyFilter = keyId;
      fetchStatsData();
    }

    function updateStatsKeyDropdown(keys) {
      const options = [{ value: 'all', label: t('stats.allCredentialsOption') }];
      if (Array.isArray(keys)) {
        for (const k of keys) {
          const label = k.key_alias ? `${k.key_alias} (${k.display_key})` : (k.display_key || `Key #${k.key_id}`);
          options.push({ value: String(k.key_id), label });
        }
      }
      if (dropdownInstances['chart-key-dropdown']) {
        dropdownInstances['chart-key-dropdown'].updateOptions(options, statsKeyFilter);
      }
    }

    function renderSummaryCards(summary) {
      // 动态时间范围标签
      const days = statsRangeDays;
      const isAllRange = days === 'all' || days === 0 || days === '0';
      let rangeLabel = t('stats.range7Days');
      if (days === 1 || days === '1') rangeLabel = t('stats.todayRangeLabel');
      else if (days === 14 || days === '14') rangeLabel = t('stats.range14Days');
      else if (days === 30 || days === '30') rangeLabel = t('stats.range30Days');
      else if (isAllRange) rangeLabel = t('filter.allHistory');
      else if (days === 7 || days === '7') rangeLabel = t('stats.range7Days');
      else if (typeof days === 'number') rangeLabel = t('stats.rangeNDays', { days: days });

      // 动态更新卡片标题以对齐所选时间跨度
      const tokensTitleEl = document.getElementById('card-tokens-title');
      if (tokensTitleEl) {
        tokensTitleEl.innerText = isAllRange ? t('stats.allHistoryTokensTitle') : t('stats.cumulativeTokensTitle', { rangeLabel: rangeLabel });
      }
      const costTitleEl = document.getElementById('card-cost-title');
      if (costTitleEl) {
        costTitleEl.innerText = t('stats.estimatedCostTitle', { rangeLabel: rangeLabel });
      }
      const reqTitleEl = document.getElementById('card-req-title');
      if (reqTitleEl) {
        reqTitleEl.innerText = isAllRange ? t('stats.allHistoryRequestsTitle') : t('stats.cumulativeRequestsTitle', { rangeLabel: rangeLabel });
      }

      // 取选定区间的指标；回退兼容原有字段
      const totalTokens = summary.period_tokens ?? summary.total_tokens ?? 0;
      const promptTokens = summary.period_prompt_tokens ?? summary.prompt_tokens ?? 0;
      const completionTokens = summary.period_completion_tokens ?? summary.completion_tokens ?? 0;
      const cachedTokens = summary.period_cached_tokens ?? summary.cached_tokens ?? 0;
      const cacheHitRate = summary.period_cache_hit_rate ?? summary.cache_hit_rate ?? 0;
      const totalRequests = summary.period_requests ?? summary.requests ?? 0;
      const costUsd = summary.period_cost_usd ?? summary.cost_usd ?? 0;
      const savingsUsd = summary.period_savings_usd ?? summary.savings_usd ?? 0;

      // 1. Tokens
      const allTimeTokens = summary.all_time_tokens || 0;
      const allTimeBadge = document.getElementById('card-alltime-tokens');
      if (allTimeBadge) {
        allTimeBadge.innerText = `${t('keys.allTimeUsagePrefix')} ` + formatTokens(allTimeTokens);
        allTimeBadge.title = t('stats.allTimeTotalTitle', { value: formatTokens(allTimeTokens), value2: formatTokens(summary.today_tokens || 0) });
      }

      document.getElementById('card-total-tokens').innerText = formatTokens(totalTokens);
      document.getElementById('sub-prompt-tokens').innerText = formatTokens(promptTokens);
      document.getElementById('sub-completion-tokens').innerText = formatTokens(completionTokens);

      const uncachedP = Math.max(0, promptTokens - cachedTokens);
      const sumSub = uncachedP + cachedTokens + completionTokens;
      if (sumSub > 0) {
        document.getElementById('bar-prompt').style.width = ((uncachedP / sumSub) * 100).toFixed(1) + '%';
        document.getElementById('bar-cached').style.width = ((cachedTokens / sumSub) * 100).toFixed(1) + '%';
        document.getElementById('bar-completion').style.width = ((completionTokens / sumSub) * 100).toFixed(1) + '%';
        document.getElementById('card-tokens-ratio').innerText = t('stats.outputRatioSuffix', { value: Math.round((completionTokens / sumSub) * 100) });
      } else {
        document.getElementById('bar-prompt').style.width = '0%';
        document.getElementById('bar-cached').style.width = '0%';
        document.getElementById('bar-completion').style.width = '0%';
        document.getElementById('card-tokens-ratio').innerText = t('stats.zeroOutputPercent');
      }

      // 2. Cache
      document.getElementById('card-cache-rate').innerText = Number(cacheHitRate).toFixed(1) + '%';
      document.getElementById('card-cache-tokens').innerText = formatTokens(cachedTokens);
      const cacheHitsBadge = document.getElementById('card-cache-hits-label');
      if (cacheHitsBadge) {
        cacheHitsBadge.innerText = formatTokens(cachedTokens) + ' Hits';
        cacheHitsBadge.title = t('stats.rangePromptCacheReadTitle', { value: formatTokens(summary.today_cached_tokens || 0) });
      }

      // 3. Cost
      document.getElementById('card-cost-total').innerText = formatCost(costUsd);
      document.getElementById('card-cost-savings').innerText = formatCost(savingsUsd);
      const savingsBadge = document.getElementById('card-savings-badge');
      if (savingsBadge) {
        savingsBadge.innerText = t('stats.savedAmountPrefix', { value: formatCost(savingsUsd) });
        savingsBadge.title = t('stats.rangeSavingsTodayTitle', { value: formatCost(summary.today_cost_usd || 0), value2: formatCost(summary.today_savings_usd || 0) });
      }

      // 4. Requests
      const allTimeRequests = summary.all_time_requests || 0;
      const allTimeReqBadge = document.getElementById('card-alltime-requests');
      if (allTimeReqBadge) {
        allTimeReqBadge.innerText = `${t('keys.allTimeUsagePrefix')} ` + formatTokens(allTimeRequests);
        allTimeReqBadge.title = t('stats.allTimeTotalTitle', { value: formatTokens(allTimeRequests), value2: (summary.today_requests || 0).toLocaleString(LOCALE) });
      }

      document.getElementById('card-total-requests').innerText = totalRequests.toLocaleString(LOCALE);
      const qps = totalRequests / Math.max(1, (days === 'all' ? 30 : Number(days) || 7) * 86400);
      const qpsEl = document.getElementById('card-req-qps');
      if (qpsEl) {
        qpsEl.innerText = qps >= 0.01 ? `${qps.toFixed(2)} QPS` : `${qps.toFixed(3)} QPS`;
      }
      const successRate = totalRequests > 0 && (summary.period_success_rate ?? summary.success_rate ?? summary.today_success_rate) != null
        ? (summary.period_success_rate ?? summary.success_rate ?? summary.today_success_rate)
        : null;
      const succEl = document.getElementById('card-req-success-rate');
      if (succEl) {
        succEl.innerText = successRate == null ? '--' : `${Number(successRate).toFixed(1)}%`;
        succEl.style.color = successRate == null ? 'var(--text-muted)' : (successRate >= 99 ? 'var(--success)' : (successRate >= 95 ? 'var(--warning)' : 'var(--danger)'));
      }
      const reqSubEl = document.getElementById('card-requests-sub');
      if (reqSubEl) {
        reqSubEl.innerText = totalRequests === 0 ? t('stats.noRequests') : (successRate == null ? t('stats.statusWaitingData') : (successRate >= 99 ? t('stats.requestsSteady') : t('stats.requestsWatch')));
        reqSubEl.title = t('stats.avgSpeedTodayTitle', { value: qps >= 0.01 ? qps.toFixed(2) : qps.toFixed(3), value2: (summary.today_requests || 0).toLocaleString(LOCALE) });
      }

      // 5. Key 状态
      document.getElementById('card-keys-total').innerText = t('stats.activeKeysSuffix', { value: summary.active_keys || 0 });
      document.getElementById('card-blocked-count').innerText = summary.blocked_keys || 0;
      if (summary.blocked_keys > 0) {
        document.getElementById('card-blocked-count').style.color = 'var(--danger)';
      } else {
        document.getElementById('card-blocked-count').style.color = 'var(--text-main)';
      }
      const keysSubEl = document.getElementById('card-keys-status-sub');
      if (keysSubEl) {
        keysSubEl.innerText = `${summary.blocked_keys || 0} / ${summary.warned_keys || 0}`;
        keysSubEl.title = t('stats.hardCapBlockedTitle', { value: summary.blocked_keys || 0, value2: summary.warned_keys || 0 });
      }
    }

    // ---- 主图表渲染 (SVG 现代极简微质感) ----
    function switchChartMetric(metric) {
      activeMetric = metric;
      const tabs = document.getElementById('chart-metric-tabs').querySelectorAll('button');
      tabs.forEach(btn => btn.classList.toggle('active', btn.getAttribute('onclick').includes(`'${metric}'`)));

      if (metric === 'tokens') {
        document.getElementById('chart-main-title').innerText = t('chart.tokenCompositionTitle');
        document.getElementById('chart-main-subtitle').innerText = t('chart.tokenCompositionSubtitle');
      } else if (metric === 'cost') {
        document.getElementById('chart-main-title').innerText = t('chart.dailySpendTrendTitle');
        document.getElementById('chart-main-subtitle').innerText = t('chart.dailySpendTrendSubtitle');
      } else {
        document.getElementById('chart-main-title').innerText = t('chart.cacheHitTrendTitle');
        document.getElementById('chart-main-subtitle').innerText = t('chart.cacheHitTrendSubtitle');
      }
      if (cachedStats) renderMainChart(cachedStats);
    }

    function switchChartRange(days) {
      statsRangeDays = days;
      const tabs = document.getElementById('chart-range-tabs').querySelectorAll('button');
      tabs.forEach(btn => {
        const onclick = btn.getAttribute('onclick') || '';
        const match = onclick.includes(`(${days})`) || onclick.includes(`('${days}')`);
        btn.classList.toggle('active', match);
      });
      const sub = document.getElementById('chart-main-subtitle');
      if (sub) {
        if (days === 1 || days === '1') sub.innerText = t('chart.todayHourlyBreakdown');
        else if (days === 'all') sub.innerText = t('chart.allHistoryDailyBreakdown');
        else sub.innerText = t('stats.rangeDailyBreakdown', { days: days });
      }
      fetchStatsData();
    }

    function renderMainChart(stats) {
      const el = document.getElementById('trend-chart');
      const daily = stats.daily || [];
      const isHourly = (statsRangeDays === 1 || statsRangeDays === '1') && Array.isArray(stats.hourly) && stats.hourly.length > 0;
      if (!isHourly && daily.length === 0) {
        el.innerHTML = `<div class="trend-empty">${t('chart.noStatistics')}</div>`;
        return;
      }

      let series = [];
      if (isHourly) {
        // 今日 (1D) 视图：按 24 小时分时段 (00:00 - 23:00) 展示每小时 Token 堆叠与用量构成
        series = [...stats.hourly];
      } else if (statsRangeDays === 'all') {
        // 全量历史模式：直接采用每日真实聚合数据升序排列
        series = [...daily].sort((a, b) => a.date.localeCompare(b.date));
      } else {
        const numDays = Number(statsRangeDays) || 7;
        const map = new Map(daily.map(d => [d.date, d]));
        const start = new Date(Date.now() - (numDays - 1) * 86400000);
        for (let i = 0; i < numDays; i++) {
          const d = new Date(start.getTime() + i * 86400000);
          const key = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai' }).format(d);
          const item = map.get(key) || { date: key, prompt_tokens: 0, completion_tokens: 0, cached_tokens: 0, total_tokens: 0, cost_usd: 0, cache_hit_rate: 0 };
          series.push(item);
        }
      }

      const W = 720, H = 270, PL = 55, PR = 15, PT = 20, PB = 34;
      const innerW = W - PL - PR, innerH = H - PT - PB;
      const step = innerW / Math.max(series.length, 1);
      const barW = Math.max(2, Math.min(36, step * 0.62));

      let maxVal = 1;
      if (activeMetric === 'tokens') {
        maxVal = Math.max(...series.map(s => s.total_tokens || 0), 100);
      } else if (activeMetric === 'cost') {
        maxVal = Math.max(...series.map(s => s.cost_usd || 0), 0.1);
      } else {
        maxVal = 100; // 百分比
      }

      const niceMax = activeMetric === 'cache' ? 100 : niceCeil(maxVal);

      let svg = `<svg viewBox="0 0 ${W} ${H}" xmlns="http://www.w3.org/2000/svg">`;

      // 4 条背景参考线
      for (let i = 0; i <= 4; i++) {
        const y = PT + innerH - (innerH * i / 4);
        const val = niceMax * i / 4;
        svg += `<line x1="${PL}" y1="${y}" x2="${W - PR}" y2="${y}" stroke="rgba(186,215,242,0.4)" stroke-width="1"/>`;
        let label = activeMetric === 'cost' ? '$' + val.toFixed(val < 1 ? 2 : 1) : (activeMetric === 'cache' ? val + '%' : formatTokens(val));
        svg += `<text x="${PL - 8}" y="${y + 3.5}" text-anchor="end" font-size="9" fill="#94a3b8" font-family="monospace">${label}</text>`;
      }

      series.forEach((s, i) => {
        const x = PL + step * i + (step - barW) / 2;
        const itemLabel = isHourly ? (s.hour || s.date) : s.date;
        const axisLabel = isHourly ? (s.hour || s.date) : s.date.slice(5);

        if (activeMetric === 'tokens') {
          // 堆叠柱状：未命中输入 (底) -> 缓存命中输入 (中) -> 输出 (顶)
          // 注意：在各大模型标准口径中，cached_tokens 是 prompt_tokens 的一部分（命中缓存的输入），两者相减才是真实的未命中输入
          const uncachedPrompt = Math.max(0, (s.prompt_tokens || 0) - (s.cached_tokens || 0));
          const cachedPrompt = s.cached_tokens || 0;
          const completion = s.completion_tokens || 0;

          const hUncached = (uncachedPrompt / niceMax) * innerH;
          const hCached = (cachedPrompt / niceMax) * innerH;
          const hComp = (completion / niceMax) * innerH;

          const yUncached = PT + innerH - hUncached;
          const yCached = yUncached - hCached;
          const yComp = yCached - hComp;

          svg += `<rect x="${x}" y="${yUncached}" width="${barW}" height="${Math.max(0, hUncached)}" fill="#60a5fa" rx="1"><title>${t('stats.chartUncachedInputTooltip', { itemLabel: itemLabel, toLocaleString: uncachedPrompt.toLocaleString(LOCALE) })} Tokens</title></rect>`;
          svg += `<rect x="${x}" y="${yCached}" width="${barW}" height="${Math.max(0, hCached)}" fill="#38bdf8" rx="1"><title>${t('stats.chartCacheReadTooltip', { itemLabel: itemLabel, toLocaleString: cachedPrompt.toLocaleString(LOCALE) })} Tokens (Prompt Cache)</title></rect>`;
          svg += `<rect x="${x}" y="${yComp}" width="${barW}" height="${Math.max(0, hComp)}" fill="#34d399" rx="1"><title>${t('stats.chartOutputTooltip', { itemLabel: itemLabel, toLocaleString: completion.toLocaleString(LOCALE) })} Tokens</title></rect>`;
        } else if (activeMetric === 'cost') {
          const h = (s.cost_usd / niceMax) * innerH;
          const y = PT + innerH - h;
          svg += `<rect x="${x}" y="${y}" width="${barW}" height="${Math.max(2, h)}" fill="#34d399" rx="2"><title>${t('stats.chartCostTooltip', { itemLabel: itemLabel, cost_usd: s.cost_usd })}</title></rect>`;
        } else {
          // 缓存命中率
          const rate = s.cache_hit_rate || 0;
          const h = (rate / 100) * innerH;
          const y = PT + innerH - h;
          svg += `<rect x="${x}" y="${y}" width="${barW}" height="${Math.max(2, h)}" fill="#38bdf8" rx="2"><title>${t('stats.chartHitRateTooltip', { itemLabel: itemLabel, rate: rate })}</title></rect>`;
        }

        // X 轴标签
        if (isHourly) {
          if (i % 2 === 0) {
            svg += `<text x="${x + barW / 2}" y="${H - 10}" text-anchor="middle" font-size="9" fill="#94a3b8" font-family="monospace">${axisLabel}</text>`;
          }
        } else if (series.length <= 14 || i % 2 === 0) {
          svg += `<text x="${x + barW / 2}" y="${H - 10}" text-anchor="middle" font-size="9" fill="#94a3b8" font-family="monospace">${axisLabel}</text>`;
        }
      });

      svg += '</svg>';
      el.innerHTML = svg;
    }

    function niceCeil(v) {
      if (v <= 0) return 1;
      const pow = Math.pow(10, Math.floor(Math.log10(v)));
      const n = v / pow;
      const nice = n <= 1 ? 1 : n <= 2 ? 2 : n <= 2.5 ? 2.5 : n <= 5 ? 5 : 10;
      return nice * pow;
    }

    // 渲染模型排行
    function renderModelsList(models) {
      document.getElementById('models-summary-tag').innerText = t('stats.activeModelCount', { count: models.length });
      const container = document.getElementById('models-container');
      if (models.length === 0) {
        container.innerHTML = `<div class="models-empty">${t('stats.emptyModels')}</div>`;
        return;
      }
      const maxVal = Math.max(...models.map(m => m.total_tokens || 0), 1);
      let html = '';
      for (const m of models) {
        const pct = Math.max(2, Math.round((m.total_tokens / maxVal) * 100));
        html += `<div class="model-row">
          <span class="model-name" title="${escapeHtml(m.model)}">${escapeHtml(m.model)}</span>
          <span class="model-bar-track"><span class="model-bar" style="width:${pct}%"></span></span>
          <span class="model-tokens num" title="${m.total_tokens.toLocaleString(LOCALE)} tokens">${formatTokens(m.total_tokens)}</span>
          <span class="model-cost num">${formatCost(m.cost_usd)}</span>
        </div>`;
      }
      container.innerHTML = html;
    }

    // 渲染 AI 提供商 Token 分布与占比
    function renderProvidersList(providers) {
      const summaryTag = document.getElementById('providers-summary-tag');
      if (summaryTag) summaryTag.innerText = t('stats.activeProviderCount', { value: (providers || []).length });
      const container = document.getElementById('providers-container');
      if (!container) return;
      if (!providers || providers.length === 0) {
        container.innerHTML = `<div class="models-empty">${t('chart.noProviderStatistics')}</div>`;
        return;
      }
      const maxVal = Math.max(...providers.map(p => p.total_tokens || 0), 1);
      const totalAllProvidersTokens = providers.reduce((acc, cur) => acc + (cur.total_tokens || 0), 0);

      let html = '<div style="display:flex; flex-direction:column; gap:10px;">';
      for (const p of providers) {
        const pctMax = Math.max(2, Math.round((p.total_tokens / maxVal) * 100));
        const sharePct = totalAllProvidersTokens > 0 ? ((p.total_tokens / totalAllProvidersTokens) * 100).toFixed(1) : '0.0';
        const brand = getProviderBrand(p.provider, p.display_name);
        const avgPerReq = p.avg_tokens_per_request || (p.requests > 0 ? Math.round(p.total_tokens / p.requests) : 0);
        const cacheHitRate = p.cache_hit_rate || 0;

        html += `
          <div style="background:rgba(255,255,255,0.7); border:1px solid var(--card-border); border-radius:8px; padding:10px 14px; transition:all 0.15s ease;">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px; flex-wrap:wrap; gap:6px;">
              <div style="display:flex; align-items:center; gap:8px;">
                <div class="provider-icon-badge ${brand.iconClass}" style="width:24px; height:24px;" title="${escapeHtml(brand.name)}">
                  ${brand.iconSvg}
                </div>
                <span style="font-weight:700; font-size:13px; color:var(--text-main);">${escapeHtml(p.display_name || brand.name)}</span>
                <span class="chip" style="font-size:10.5px; padding:1px 6px; background:#eff6ff; color:#2563eb; border:1px solid #bfdbfe;">${t('stats.providerShareChip', { sharePct: sharePct })}</span>
                <span style="font-size:11px; color:var(--text-dim);">${t('stats.callCountSuffix', { value: (p.requests || 0).toLocaleString(LOCALE) })}</span>
              </div>
              <div style="display:flex; align-items:center; gap:14px; font-family:var(--mono); font-size:12px;">
                <span style="color:var(--text-muted); font-size:11px;">${t('provider.avgSpendLabel')} <b style="color:var(--text-main);">${formatTokens(avgPerReq)}</b>${t('provider.perCallSuffix')}</span>
                <span style="color:#0284c7; font-size:11px;">${t('provider.cacheRateLabel')} <b>${cacheHitRate.toFixed(1)}%</b></span>
                <span style="font-weight:700; color:var(--text-main); font-size:13px;" title="${(p.total_tokens || 0).toLocaleString(LOCALE)} tokens">${formatTokens(p.total_tokens)}</span>
                <span style="font-weight:700; color:#10b981;">${formatCost(p.cost_usd)}</span>
              </div>
            </div>
            <div style="width:100%; height:6px; background:rgba(0,0,0,0.05); border-radius:3px; overflow:hidden;">
              <div style="width:${pctMax}%; height:100%; background:linear-gradient(90deg, #38bdf8 0%, #34d399 100%); border-radius:3px; transition:width 0.3s;"></div>
            </div>
          </div>
        `;
      }
      html += '</div>';
      container.innerHTML = html;
    }
