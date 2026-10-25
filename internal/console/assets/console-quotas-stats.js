/**
 * Gate Admin Console - Upstream credential usage and success-rate statistics table
 * 上游凭证用量与成功率统计视图
 */

    async function fetchUpstreamAuthStats() {
      const secret = getAdminSecret();
      if (!secret) return;
      try {
        const range = activeUpstreamStatsRange || 'all';
        const res = await fetch(`/console/api/upstream-auth-stats?range=${encodeURIComponent(range)}`, {
          headers: { 'Authorization': 'Bearer ' + secret }
        });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const text = await res.text();
        let data = {};
        try {
          data = text ? JSON.parse(text) : {};
        } catch (_) {}
        cachedUpstreamStats = data;
        renderUpstreamStatsTable();
      } catch (err) {
        const container = document.getElementById('upstream-stats-cards-container');
        if (container) {
          container.innerHTML = `<div style="grid-column: 1 / -1; text-align:center; padding:36px 16px; color:var(--text-dim);">${t('quotas.statsUnavailable')}</div>`;
        }
        const archiveContainer = document.getElementById('upstream-stats-archive-container');
        if (archiveContainer) {
          archiveContainer.style.display = 'none';
          archiveContainer.innerHTML = '';
        }
        const tbody = document.getElementById('upstream-stats-table-body');
        if (tbody) {
          tbody.innerHTML = `<tr><td colspan="8" style="text-align:center; padding:24px;">${t('quotas.statsUnavailable')}</td></tr>`;
        }
        console.warn('Upstream auth stats fetch error:', err.message);
      }
    }

    function switchUpstreamStatsRange(range) {
      activeUpstreamStatsRange = range;
      const tabs = document.getElementById('upstream-stats-range-tabs');
      if (tabs) {
        tabs.querySelectorAll('button').forEach(btn => {
          const onclickAttr = btn.getAttribute('onclick') || '';
          if (onclickAttr.includes(`'${range}'`)) {
            btn.classList.add('active');
          } else {
            btn.classList.remove('active');
          }
        });
      }

      // 更新副标题提示
      const subtitle = document.getElementById('upstream-stats-subtitle');
      if (subtitle) {
        if (range === '5h') {
          subtitle.innerText = t('quotas.sourceNote');
        } else if (range === '24h') {
          subtitle.innerText = t('quotas.sourceNote');
        } else if (range === '7d') {
          subtitle.innerText = t('quotas.sourceNote');
        } else {
          subtitle.innerText = t('quotas.localBaselineEstimateNote');
        }
      }

      const container = document.getElementById('upstream-stats-cards-container');
      if (container) {
        container.innerHTML = `<div style="grid-column: 1 / -1; text-align:center; padding:36px 16px; color:var(--text-dim);">${t('quotas.loadingUsageStats')}</div>`;
      }
      const archiveContainer = document.getElementById('upstream-stats-archive-container');
      if (archiveContainer) {
        archiveContainer.style.display = 'none';
        archiveContainer.innerHTML = '';
      }
      const tbody = document.getElementById('upstream-stats-table-body');
      if (tbody) {
        tbody.innerHTML = `<tr><td colspan="8" style="text-align:center; padding:24px; color:var(--text-dim);">${t('quotas.loadingUsageStats')}</td></tr>`;
      }

      fetchUpstreamAuthStats();
    }

    function renderUpstreamStatsTable() {
      const container = document.getElementById('upstream-stats-cards-container');
      const archiveContainer = document.getElementById('upstream-stats-archive-container');
      const tbody = document.getElementById('upstream-stats-table-body');
      const data = cachedUpstreamStats;
      const rawItems = (data && (Array.isArray(data.items) ? data.items : (Array.isArray(data.auth_stats) ? data.auth_stats : null))) || [];

      if (!data || rawItems.length === 0) {
        if (container) {
          container.innerHTML = `<div style="grid-column: 1 / -1; text-align:center; padding:36px 16px; color:var(--text-dim);">${t('quotas.emptyUsageStats')}</div>`;
        }
        if (archiveContainer) {
          archiveContainer.style.display = 'none';
          archiveContainer.innerHTML = '';
        }
        if (tbody) tbody.innerHTML = `<tr><td colspan="8" style="text-align:center; padding:24px; color:var(--text-dim);">${t('quotas.emptyUsageStats')}</td></tr>`;
        return;
      }

      const filterVal = (document.getElementById('upstream-stats-filter')?.value || '').trim().toLowerCase();
      const filteredItems = filterVal
        ? rawItems.filter(it => {
            const name = String(it.display_name || it.name || '').toLowerCase();
            const provider = String(it.provider_label || it.provider || '').toLowerCase();
            const model = String(it.last_model || '').toLowerCase();
            const id = String(it.auth_index || '').toLowerCase();
            return name.includes(filterVal) || provider.includes(filterVal) || model.includes(filterVal) || id.includes(filterVal);
          })
        : rawItems;

      if (filteredItems.length === 0) {
        if (container) {
          container.innerHTML = `<div style="grid-column: 1 / -1; text-align:center; padding:36px 16px; color:var(--text-dim);">${t('quotas.noMatchingCard', { value: escapeHtml(filterVal) })}</div>`;
        }
        if (archiveContainer) {
          archiveContainer.style.display = 'none';
          archiveContainer.innerHTML = '';
        }
        if (tbody) tbody.innerHTML = `<tr><td colspan="8" style="text-align:center; padding:24px; color:var(--text-dim);">${t('quotas.noMatchingCredentials')}</td></tr>`;
        return;
      }

      // 核心拆分：活跃有效凭证 vs 已从 CPA 移除的历史留痕账号（避免历史账号大面积霸屏）
      const activeItems = [];
      const deletedItems = [];

      filteredItems.forEach(it => {
        const isDel = Boolean(it.is_deleted || it.is_placeholder_name);
        if (isDel) {
          deletedItems.push(it);
        } else {
          activeItems.push(it);
        }
      });

      // 1. 渲染主卡片网格（仅呈现正常活跃账号）
      let activeCardsHtml = '';
      if (activeItems.length > 0) {
        activeCardsHtml = activeItems.map(it => {
          const brand = getProviderBrand(it.provider, it.display_name || it.name);
          const prio = normalizeAuthPriority(it.priority);
          const prioCls = prio >= 20 ? 'p-high' : (prio >= 10 ? 'p-normal' : 'p-low');
          const isDisabled = Boolean(it.disabled || it.status === 'disabled');
          const successRate = it.success_rate != null ? it.success_rate.toFixed(1) + '%' : '100.0%';
          const reqSourceTag = it.runtime_source === 'cpa_runtime'
            ? '<span class="ovw-source-tag">CPA</span>'
            : '';

          let rateColor = '#059669'; // 绿
          let rateBorder = '#a7f3d0';
          let rateBg = '#ecfdf5';
          if (it.success_rate < 80) {
            rateColor = '#dc2626'; // 红
            rateBorder = '#fecaca';
            rateBg = '#fef2f2';
          } else if (it.success_rate < 95) {
            rateColor = '#d97706'; // 橙
            rateBorder = '#fde68a';
            rateBg = '#fffbeb';
          }

          // 周期额度与配额水位渲染：
          // 1. WorkBuddy: 资源包积分额度 (剩余/总额/已用)
          // 2. OpenRouter: 今日请求次数与 800 次每日限额
          // 3. Antigravity / Codex: 5h 与周限额滑动窗口
          let quotaHtml = `<div style="color:var(--text-dim); font-size:11px; text-align:center; padding:3px 0;">${t('quotas.noLiveQuotaWatermark')}</div>`;

          // 辅助凭证对象兜底查询 (以防 stats 接口未刷新时已有 cachedUpstream.accounts)
          const fallbackAcc = (cachedUpstream.accounts || []).find(a =>
            (it.auth_index && a.auth_index === it.auth_index) ||
            a.id === it.name ||
            a.filename === it.name ||
            (it.provider === 'openrouter' && (a.id === it.auth_index || a.name === it.name || `openrouter-${a.id}` === it.auth_index)) ||
            (it.provider === 'opencode' && (a.id === it.auth_index || a.filename === it.name || a.email === it.name))
          );

          const isWorkBuddy = it.provider === 'workbuddy' || it.provider === 'workbuddy-cn' || String(it.name || '').toLowerCase().startsWith('workbuddy');
          const isOpenRouter = it.provider === 'openrouter' || String(it.name || '').toLowerCase().startsWith('openrouter');
          const isOpenCode = it.provider === 'opencode' || String(it.name || '').toLowerCase().startsWith('opencode');

          if (isWorkBuddy) {
            const totalCredits = it.total_credits != null ? Number(it.total_credits) : (fallbackAcc?.total_credits != null ? Number(fallbackAcc.total_credits) : null);
            const remainCredits = it.remaining_credits != null ? Number(it.remaining_credits) : (fallbackAcc?.remaining_credits != null ? Number(fallbackAcc.remaining_credits) : null);
            const usedCredits = it.used_credits != null ? Number(it.used_credits) : (fallbackAcc?.used_credits != null ? Number(fallbackAcc.used_credits) : 0);

            if (remainCredits != null || totalCredits != null) {
              const total = totalCredits || 0;
              const remain = remainCredits || 0;
              const used = usedCredits || 0;
              const remPct = total > 0 ? Math.max(0, Math.min(100, Math.round((remain / total) * 1000) / 10)) : (remain > 0 ? 100 : 0);
              let barColor = '#10b981'; // 充裕 (绿)
              if (remPct <= 20) barColor = '#ef4444'; // 耗尽或濒危 (红)
              else if (remPct <= 50) barColor = '#f59e0b'; // 警戒 (黄)

              const remainFormatted = remain.toLocaleString(undefined, { minimumFractionDigits: 0, maximumFractionDigits: 2 });
              const totalFormatted = total.toLocaleString(LOCALE);
              const usedFormatted = used.toLocaleString(undefined, { minimumFractionDigits: 0, maximumFractionDigits: 2 });

              const targetObj = (it && (it.expires_at || it.reset_time)) ? it : (fallbackAcc || it);
              const expInfo = getWorkBuddyExpirationInfo(targetObj);

              quotaHtml = `
                <div class="quota-box-row">
                  <span class="quota-box-title">${t('quotas.workbuddy.packPointsTitle')} <b style="color:${barColor};">${t('quotas.workbuddy.remainingPointsLabel', { remainFormatted: remainFormatted })}</b></span>
                  <span class="quota-box-countdown">${t('quotas.workbuddy.remainingPercentLabel', { value: remPct.toFixed(1) })}</span>
                </div>
                <div class="quota-progress-track">
                  <div class="quota-progress-fill" style="width:${Math.max(0, Math.min(100, remPct))}%; background:${barColor};"></div>
                </div>
                <div class="quota-weekly-row" style="display:flex; justify-content:space-between; align-items:center;">
                  <span>${t('quotas.points.totalLabel')} <b>${totalFormatted}</b> ${t('quotas.points.usedWithPointsLabel')} <b>${usedFormatted}</b></span>
                  <span style="color:${expInfo.color}; font-size:11px;" title="${escapeHtml(expInfo.tooltip)}">
                    ${expInfo.isAllLongTerm ? '' : (expInfo.hasMultiplePackages ? `${t('quotas.workbuddy.firstBatchExpiryLabel')} ` : `${t('quotas.points.expiresAtLabel')} `)}<b>${escapeHtml(expInfo.shortText)}</b>${expInfo.countdown ? ` (${escapeHtml(expInfo.countdown)})` : ''}${expInfo.hasMultiplePackages && !expInfo.isAllLongTerm ? ` <span style="font-size:9.5px; opacity:0.8;">${t('quotas.workbuddy.packageCountCompactSuffix', { packageCount: expInfo.packageCount })}</span>` : ''}
                  </span>
                </div>
              `;
            }
          } else if (isOpenRouter) {
            const todayCount = it.today_requests != null ? Number(it.today_requests) : (fallbackAcc?.today_requests != null ? Number(fallbackAcc.today_requests) : null);
            const dailyLimit = it.daily_limit != null ? Number(it.daily_limit) : (fallbackAcc?.daily_limit != null ? Number(fallbackAcc.daily_limit) : 800);
            const remainingReq = it.remaining_requests != null ? Number(it.remaining_requests) : (fallbackAcc?.remaining_requests != null ? Number(fallbackAcc.remaining_requests) : (todayCount != null ? Math.max(0, dailyLimit - todayCount) : null));
            const resetCountdown = it.reset_countdown || fallbackAcc?.reset_countdown || '';

            if (todayCount != null) {
              const remCount = remainingReq != null ? remainingReq : Math.max(0, dailyLimit - todayCount);
              const remPct = dailyLimit > 0 ? Math.max(0, Math.min(100, Math.round((remCount / dailyLimit) * 1000) / 10)) : 100;
              let barColor = '#10b981'; // 充裕 (绿)
              if (remPct <= 20) barColor = '#ef4444'; // 耗尽或濒危 (红)
              else if (remPct <= 50) barColor = '#f59e0b'; // 警戒 (黄)

              const countdownText = resetCountdown ? (resetCountdown.includes(t('quotas.actionReset')) ? resetCountdown : t('quotas.resetCountdownLabel', { resetCountdown: resetCountdown })) : t('quotas.resetAtUtcMidnight');

              quotaHtml = `
                <div class="quota-box-row">
                  <span class="quota-box-title">${t('quotas.todayRequestsLabel')} <b style="color:#3b82f6;">${todayCount}</b> <span style="font-size:10px; color:var(--text-dim); font-weight:normal;">/ ${t('quotas.openrouter.dailyLimitSuffix', { dailyLimit: dailyLimit })}</span></span>
                  <span class="quota-box-countdown">${escapeHtml(countdownText)}</span>
                </div>
                <div class="quota-progress-track">
                  <div class="quota-progress-fill" style="width:${Math.max(0, Math.min(100, remPct))}%; background:${barColor};"></div>
                </div>
                <div class="quota-weekly-row">${t('quotas.todayRemainingLabel')} <b style="color:${barColor};">${remCount}</b> ${t('quotas.openrouter.availableTimesCycle')} <b>${t('quotas.bucket.utcDay')}</b></div>
              `;
            }
          } else if (isOpenCode) {
            const todayCount = it.today_requests != null ? Number(it.today_requests) : (fallbackAcc?.today_requests != null ? Number(fallbackAcc.today_requests) : 0);
            const todayTokens = it.today_tokens != null ? Number(it.today_tokens) : (fallbackAcc?.today_tokens != null ? Number(fallbackAcc.today_tokens) : 0);

            quotaHtml = `
              <div class="quota-box-row">
                <span class="quota-box-title">${t('quotas.opencode.todayConsumedTitle')} <b style="color:#0ea5e9;">${formatTokens(todayTokens)}</b> Tokens</span>
                <span class="quota-box-countdown">${t('quotas.todayRequestsLabel')} <b style="color:var(--text-main); font-weight:700;">${todayCount}</b> ${t('quotas.timesUnit')}</span>
              </div>
              <div class="quota-weekly-row" style="margin-top:4px;">${t('quotas.naturalDayCumulativeLabel')} <b>${todayTokens.toLocaleString(LOCALE)}</b> Tokens · <b>${todayCount}</b> ${t('quotas.requestCountSuffix')}</div>
            `;
          } else if (it.quota_5h) {
            const rem5h = it.quota_5h.remaining_percent;
            let barColor = '#10b981'; // 充裕 (绿)
            if (rem5h < 20) barColor = '#ef4444'; // 耗尽或濒危 (红)
            else if (rem5h < 50) barColor = '#f59e0b'; // 警戒 (黄)

            const countdownText = it.quota_5h.reset_countdown || (rem5h >= 100 ? t('quotas.bucket.fullQuotaReady') : '');
            const title5h = it.quota_5h.name ? `${escapeHtml(it.quota_5h.name)} ${t('quotas.bucket.fiveHourRemaining')}` : t('quotas.bucket.fiveHourRemaining');
            const titleWeekly = it.quota_weekly?.name ? `${escapeHtml(it.quota_weekly.name)}:` : t('quotas.bucket.weeklyRemaining');
            quotaHtml = `
              <div class="quota-box-row">
                <span class="quota-box-title">${title5h} <b style="color:${barColor};">${rem5h.toFixed(1)}%</b></span>
                <span class="quota-box-countdown">${escapeHtml(countdownText)}</span>
              </div>
              <div class="quota-progress-track">
                <div class="quota-progress-fill" style="width:${Math.max(0, Math.min(100, rem5h))}%; background:${barColor};"></div>
              </div>
              ${it.quota_weekly ? `<div class="quota-weekly-row">${titleWeekly} <b>${it.quota_weekly.remaining_percent.toFixed(1)}%</b> ${it.quota_weekly.reset_countdown ? `· ${escapeHtml(it.quota_weekly.reset_countdown)}` : ''}</div>` : ''}
            `;
          } else if (Array.isArray(it.quota_pools) && it.quota_pools.length > 0) {
            // 非 5h/周窗口的提供商（ZCode 模型额度桶、Qoder 点数等）：
            // 逐条渲染上游真实窗口，最紧张的排在最前面。
            const pools = [...it.quota_pools].sort((a, b) => a.remaining_percent - b.remaining_percent).slice(0, 2);
            const poolRows = pools.map(pool => {
              const rem = Math.max(0, Math.min(100, Number(pool.remaining_percent) || 0));
              let barColor = '#10b981';
              if (rem < 20) barColor = '#ef4444';
              else if (rem < 50) barColor = '#f59e0b';
              const countdown = pool.reset_countdown ? escapeHtml(pool.reset_countdown) : '';
              return `
                <div class="quota-box-row">
                  <span class="quota-box-title">${escapeHtml(pool.name)} <b style="color:${barColor};">${rem.toFixed(1)}%</b></span>
                  <span class="quota-box-countdown">${countdown}</span>
                </div>
                <div class="quota-progress-track">
                  <div class="quota-progress-fill" style="width:${rem}%; background:${barColor};"></div>
                </div>
              `;
            }).join('');
            const hidden = it.quota_pools.length - pools.length;
            quotaHtml = poolRows + (hidden > 0
              ? `<div class="quota-weekly-row">${t('quotas.moreQuotaPools', { count: hidden })}</div>`
              : '');
          }

          // 分时段 Token 矩阵提取
          const pStats = it.period_stats || {};
          const tok5hStr = formatTokens(pStats.tokens_5h || 0);
          const tok24hStr = formatTokens(pStats.tokens_24h || 0);
          const tokAllStr = formatTokens(pStats.tokens_all || it.total_tokens || 0);
          const shortId = it.auth_index ? String(it.auth_index).slice(0, 16) : '—';

          return `
            <div class="upstream-stat-card ${isDisabled ? 'is-disabled' : ''}">
              <!-- 卡片顶栏：图标 + 名称 + 状态 + 优先级 + 成功率大徽章 -->
              <div class="stat-card-header">
                <div class="stat-card-identity">
                  <div class="provider-icon-badge ${brand.iconClass}" title="${escapeHtml(brand.name)}">
                    ${brand.iconSvg}
                  </div>
                  <div class="stat-card-title-wrap">
                    <div class="stat-card-title-row">
                      <span class="provider-pill ${brand.pillClass}">${escapeHtml(brand.name)}</span>
                      <span class="stat-card-name" title="${escapeHtml(it.name)}">
                        ${escapeHtml(it.name)}
                      </span>
                      ${isDisabled ? `<span class="status-tag tag-disabled">${t('quotas.disabledBadge')}</span>` : ''}
                    </div>
                    <div class="stat-card-sub-row">
                      <span class="stat-card-id" title="${escapeHtml(it.auth_index)}">ID: ${escapeHtml(shortId)}</span>
                      <span class="priority-badge ${prioCls}">P${prio}</span>
                    </div>
                  </div>
                </div>
                <div class="stat-card-rate-badge" style="color:${rateColor}; border-color:${rateBorder}; background:${rateBg};" title="${t('quotas.successRateTooltip')}">
                  <div class="stat-card-rate-val">${successRate}</div>
                  <div class="stat-card-rate-lbl">${t('stats.successRate')}</div>
                </div>
              </div>

              <!-- 周期额度条 (5h / 周限额) -->
              <div class="stat-card-quota-box">
                ${quotaHtml}
              </div>

              <!-- 核心指标 3 格 (总请求数 | Token 总数 | 缓存率) -->
              <div class="stat-card-metrics-grid">
                <div class="stat-metric-cell">
                  <div class="metric-cell-label">${t('quotas.totalRequestsWithSource', { reqSourceTag: reqSourceTag })}</div>
                  <div class="metric-cell-value num">${formatTokens(it.total_requests)}</div>
                  <div class="metric-cell-sub">
                    <span style="color:#10b981;">${t('quotas.successCountLabel', { value: formatTokens(it.success_count) })}</span> / <span style="color:#ef4444;">${t('quotas.failureCountLabel', { value: formatTokens(it.failure_count) })}</span>
                  </div>
                </div>
                <div class="stat-metric-cell">
                  <div class="metric-cell-label">${t('quotas.tokenConsumption')} <span class="ovw-source-tag is-local">${t('quotas.localSourceTag')}</span></div>
                  <div class="metric-cell-value num" title="${it.total_tokens.toLocaleString(LOCALE)}">${formatTokens(it.total_tokens)}</div>
                  <div class="metric-cell-sub">
                    ${t('quotas.tokenInOutBreakdown', { value: formatTokens(it.prompt_tokens), value2: formatTokens(it.completion_tokens) })}
                  </div>
                </div>
                <div class="stat-metric-cell">
                  <div class="metric-cell-label">${t('stats.cacheHitRate')} <span class="ovw-source-tag is-local">${t('quotas.localSourceTag')}</span></div>
                  <div class="metric-cell-value num" style="color:#0284c7;">${it.cache_rate != null ? it.cache_rate.toFixed(1) + '%' : '0.0%'}</div>
                  <div class="metric-cell-sub" title="${t('quotas.cachedTokensLabel', { value: Number(it.cached_tokens || 0).toLocaleString(LOCALE) })}">
                    ${t('quotas.cacheHitCount', { value: formatTokens(it.cached_tokens) })}
                  </div>
                </div>
              </div>

              <!-- 分时段 Token 水位带 (5h / 24h / 历史总数) -->
              <div class="stat-card-period-bar">
                <div class="period-item" title="${t('quotas.bucket.tokenWithin5h')}"><span class="period-dot dot-5h"></span>5h: <b>${tok5hStr}</b></div>
                <div class="period-item" title="${t('quotas.bucket.tokenWithin24h')}"><span class="period-dot dot-24h"></span>24h: <b>${tok24hStr}</b></div>
                <div class="period-item" title="${t('quotas.bucket.allHistoryTokens')}"><span class="period-dot dot-all"></span>${t('quotas.cumulativeLabel')} <b>${tokAllStr}</b></div>
              </div>

              <!-- 底部：最近活跃模型 -->
              <div class="stat-card-footer">
                <span class="footer-model-label">${t('quotas.recentModelLabel')}</span>
                <span class="footer-model-value" title="${escapeHtml(it.last_model || '—')}">${escapeHtml(it.last_model || '—')}</span>
              </div>
            </div>
          `;
        }).join('');
      } else {
        activeCardsHtml = `<div style="grid-column: 1 / -1; text-align:center; padding:32px 16px; color:var(--text-dim); font-size:12.5px;">${t('quotas.emptyActiveCards')}${deletedItems.length > 0 ? t('quotas.archive.archivedHint') : ''}</div>`;
      }

      if (container) {
        container.innerHTML = activeCardsHtml;
      }

      // 2. 渲染已删除账号归档区（收纳至底部折叠面板，绝不占据主网格视线）
      if (archiveContainer) {
        if (deletedItems.length === 0) {
          archiveContainer.style.display = 'none';
          archiveContainer.innerHTML = '';
        } else {
          archiveContainer.style.display = 'block';
          const wasOpen = Boolean(archiveContainer.querySelector('.deleted-accounts-archive')?.open);

          const rowsHtml = deletedItems.map(it => {
            const brand = getProviderBrand(it.provider, it.display_name || it.name);
            const pStats = it.period_stats || {};
            const tok5hStr = formatTokens(pStats.tokens_5h || 0);
            const tok24hStr = formatTokens(pStats.tokens_24h || 0);
            const tokAllStr = formatTokens(pStats.tokens_all || it.total_tokens || 0);
            const shortId = it.auth_index ? String(it.auth_index).slice(0, 16) : '—';
            const successRate = it.success_rate != null ? it.success_rate.toFixed(1) + '%' : '100.0%';

            let rateColor = '#059669';
            if (it.success_rate < 80) rateColor = '#dc2626';
            else if (it.success_rate < 95) rateColor = '#d97706';

            return `
              <tr>
                <td>
                  <div class="deleted-row-name">
                    <span>${escapeHtml(it.name)}</span>
                    <span class="status-tag tag-deleted" style="font-size:9.5px; padding:1px 5px;">${t('quotas.archive.historyTag')}</span>
                  </div>
                  <div class="deleted-row-id" title="${escapeHtml(it.auth_index)}">ID: ${escapeHtml(shortId)}</div>
                </td>
                <td>
                  <span class="provider-pill ${brand.pillClass}" style="font-size:10.5px;">${escapeHtml(brand.name)}</span>
                </td>
                <td>
                  <div style="font-family:var(--mono); font-weight:600; font-size:11.5px;">${formatTokens(it.total_requests)}</div>
                  <div style="font-size:10px; font-family:var(--mono); color:var(--text-dim);">
                    <span style="color:#10b981;">${t('quotas.successCountLabel', { value: formatTokens(it.success_count) })}</span> / <span style="color:#ef4444;">${t('quotas.failureCountLabel', { value: formatTokens(it.failure_count) })}</span>
                  </div>
                </td>
                <td>
                  <span style="font-family:var(--mono); font-weight:700; color:${rateColor}; font-size:11.5px;">${successRate}</span>
                </td>
                <td>
                  <div style="font-family:var(--mono); font-weight:700; font-size:11.5px;" title="${it.total_tokens.toLocaleString(LOCALE)}">${formatTokens(it.total_tokens)}</div>
                  <div style="font-size:10px; font-family:var(--mono); color:var(--text-dim);">
                    ${t('quotas.tokenInOutBreakdown', { value: formatTokens(it.prompt_tokens), value2: formatTokens(it.completion_tokens) })}${it.cached_tokens ? ` ${t('quotas.cachedTokensSuffix', { value: formatTokens(it.cached_tokens) })}` : ''}
                  </div>
                </td>
                <td>
                  <div style="font-size:10.5px; font-family:var(--mono); color:var(--text-dim); display:flex; gap:8px;">
                    <span>5h: <b style="color:var(--text-main);">${tok5hStr}</b></span>
                    <span>24h: <b style="color:var(--text-main);">${tok24hStr}</b></span>
                    <span>${t('quotas.cumulativeLabel')} <b style="color:var(--text-main);">${tokAllStr}</b></span>
                  </div>
                </td>
                <td>
                  <span class="footer-model-value" style="font-size:10px;" title="${escapeHtml(it.last_model || '—')}">${escapeHtml(it.last_model || '—')}</span>
                </td>
              </tr>
            `;
          }).join('');

          archiveContainer.innerHTML = `
            <details class="deleted-accounts-archive" ${wasOpen ? 'open' : ''}>
              <summary class="deleted-accounts-summary">
                <div class="deleted-summary-left">
                  <svg class="deleted-archive-icon" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
                    <path d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" stroke-linecap="round" stroke-linejoin="round"></path>
                  </svg>
                  <span class="deleted-summary-title">${t('quotas.archive.historyTitle')}</span>
                  <span class="deleted-summary-badge">${t('quotas.accountCountSuffix', { count: deletedItems.length })}</span>
                  <span class="deleted-summary-desc">${t('quotas.archive.removedFromCpaHint')}</span>
                </div>
                <div class="deleted-summary-right">
                  <span class="deleted-summary-hint">${t('quotas.archive.expandToggleHint')}</span>
                  <svg class="deleted-chevron" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
                    <path d="M19 9l-7 7-7-7" stroke-linecap="round" stroke-linejoin="round"></path>
                  </svg>
                </div>
              </summary>
              <div class="deleted-accounts-body">
                <div class="deleted-accounts-table-wrap">
                  <table class="deleted-accounts-table">
                    <thead>
                      <tr>
                        <th style="min-width:170px;">${t('quotas.credentialIdColumn')}</th>
                        <th>${t('quotas.providerColumn')}</th>
                        <th>${t('quotas.totalRequestsColumn')}</th>
                        <th>${t('stats.successRate')}</th>
                        <th>${t('quotas.tokenConsumptionBreakdown')}</th>
                        <th>${t('quotas.periodConsumptionColumn')}</th>
                        <th>${t('quotas.recentModelColumn')}</th>
                      </tr>
                    </thead>
                    <tbody>
                      ${rowsHtml}
                    </tbody>
                  </table>
                </div>
              </div>
            </details>
          `;
        }
      }

      if (tbody) {
        tbody.innerHTML = `<tr><td colspan="8" style="text-align:center; padding:12px; color:var(--text-dim);">${t('quotas.switchedToCardView')}</td></tr>`;
      }
    }
