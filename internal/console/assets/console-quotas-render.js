/**
 * Gate Admin Console - Upstream credential card rendering (compact list and full card grid)
 * 上游凭证卡片渲染（紧凑列表与完整卡片网格）
 */

    function formatBucketLabel(name, window) {
      if (!name) return t('quotas.bucket.standardQuota');
      const n = String(name).trim();
      const nl = n.toLowerCase();
      if (nl.includes('five hour') || nl.includes('5 hour') || nl.includes('5h') || nl.includes(t('quotas.bucket.fiveHour')) || window === '5h') {
        return t('quotas.bucket.fiveHourLimit');
      }
      if (nl.includes('weekly') || nl.includes('week') || nl.includes(t('quotas.bucket.weeklyLimit')) || window === 'weekly') {
        return t('quotas.bucket.weeklyLimit');
      }
      return n;
    }

    function renderUpstreamCards() {
      renderOperationsStrip();
      const cardsContainer = document.getElementById('upstream-cards-container');
      const compactContainer = document.getElementById('upstream-compact-container');
      const accounts = cachedUpstream.accounts || [];

      // 更新导航栏徽章
      const navBadge = document.getElementById('nav-badge-quotas');
      if (navBadge) navBadge.innerText = accounts.length;

      // 初始化 checkbox 状态
      const hideCheck = document.getElementById('upstream-hide-disabled');
      if (hideCheck && hideCheck.checked !== upstreamHideDisabled) {
        hideCheck.checked = upstreamHideDisabled;
      }

      // 初始化视图切换高亮
      const viewTabs = document.querySelectorAll('#upstream-view-mode-tabs button');
      viewTabs.forEach(btn => {
        btn.classList.toggle('active', btn.getAttribute('onclick')?.includes(`'${activeUpstreamViewMode}'`));
      });

      let filtered = accounts;
      if (activeUpstreamFilter !== 'all') {
        filtered = accounts.filter(a => {
          const prov = (a.provider || '').toLowerCase();
          if (activeUpstreamFilter === 'workbuddy') {
            return prov === 'workbuddy' || prov === 'workbuddy-cn';
          }
          if (activeUpstreamFilter === 'qoder') {
            return prov === 'qoder' || prov === 'qoder-cn';
          }
          return prov === activeUpstreamFilter;
        });
      }
      if (upstreamHideDisabled) {
        filtered = filtered.filter(a => !a.disabled && a.status !== 'disabled');
      }
      if (upstreamSearchQuery) {
        filtered = filtered.filter(a => {
          const email = (a.email || '').toLowerCase();
          const authIdx = (a.auth_index || '').toLowerCase();
          const fname = (a.filename || a.id || '').toLowerCase();
          const disp = (a.display_name || '').toLowerCase();
          const nick = (a.nickname || '').toLowerCase();
          return email.includes(upstreamSearchQuery) ||
                 authIdx.includes(upstreamSearchQuery) ||
                 fname.includes(upstreamSearchQuery) ||
                 disp.includes(upstreamSearchQuery) ||
                 nick.includes(upstreamSearchQuery);
        });
      }

      if (filtered.length === 0) {
        const emptyHtml = `<div class="upstream-empty-state">${t('quotas.emptyFilteredCredentials')}</div>`;
        if (cardsContainer) cardsContainer.innerHTML = emptyHtml;
        if (compactContainer) compactContainer.innerHTML = emptyHtml;
        return;
      }

      // 切换显隐容器
      if (activeUpstreamViewMode === 'compact') {
        if (cardsContainer) cardsContainer.style.display = 'none';
        if (compactContainer) {
          compactContainer.style.display = 'flex';
          compactContainer.innerHTML = filtered.map(acc => {
            const safeFilename = escapeHtml(acc.filename || acc.id || '');
            const brand = getProviderBrand(acc.provider, acc.display_name || acc.name);
            const planType = (acc.plan_type || 'free').toLowerCase();
            let planClass = 'plan-free';
            if (planType.includes('pro') || planType.includes('ultra')) planClass = 'plan-pro';
            else if (planType.includes('plus')) planClass = 'plan-plus';

            const isDisabled = Boolean(acc.disabled || acc.status === 'disabled');
            const health = acc.health || {};
            const isBlocked = health.blocked === true;
            const isCredentialBlocked = isBlocked && (!health.cooldown || health.cooldown.scope === 'credential' || health.cooldown.scope === 'credential_and_models');
            const prio = normalizeAuthPriority(acc.priority);
            const prioCls = prio >= 20 ? 'p-high' : (prio >= 10 ? 'p-normal' : 'p-low');

            // 智能提取配额桶：针对 Antigravity 提取 Claude 5h/周限额 与 Gemini 5h/周限额；针对 Codex 提取 5h 与周限额
            let gemini5hBucket = null;
            let geminiWeeklyBucket = null;
            let claude5hBucket = null;
            let claudeWeeklyBucket = null;
            let codex5hBucket = null;
            let codexWeeklyBucket = null;
            let fallbackBucket = null;

            for (const grp of (acc.groups || [])) {
              const gname = ((grp.name || '') + ' ' + (grp.displayName || '')).toLowerCase();
              for (const bkt of (grp.buckets || [])) {
                if (!fallbackBucket && Number.isFinite(bkt.remaining_percent)) fallbackBucket = bkt;
                const bname = ((bkt.name || '') + ' ' + (bkt.displayName || '')).toLowerCase();
                const bid = (bkt.bucket_id || '').toLowerCase();
                const bwin = (bkt.window || '').toLowerCase();
                const isWeekly = bwin === 'weekly' || bname.includes('weekly') || bname.includes('week') || bname.includes(t('quotas.bucket.weekly')) || bid.includes('weekly');
                const is5h = bwin === '5h' || bname.includes('5h') || bname.includes('5 hour') || bname.includes('five') || bid.includes('5h') || !isWeekly;

                if (gname.includes('gemini') || bname.includes('gemini') || bid.includes('gemini')) {
                  if (isWeekly) geminiWeeklyBucket = bkt;
                  else if (!gemini5hBucket || is5h) gemini5hBucket = bkt;
                } else if (gname.includes('claude') || bname.includes('claude') || bid.includes('claude') || gname.includes('gpt') || bname.includes('gpt') || gname.includes('3p')) {
                  if (isWeekly) claudeWeeklyBucket = bkt;
                  else if (!claude5hBucket || is5h) claude5hBucket = bkt;
                } else if (acc.provider === 'codex') {
                  if (isWeekly) codexWeeklyBucket = bkt;
                  else if (!codex5hBucket || is5h) codex5hBucket = bkt;
                } else {
                  if (isWeekly && !claudeWeeklyBucket) claudeWeeklyBucket = bkt;
                  else if (!claude5hBucket) claude5hBucket = bkt;
                }
              }
            }

            if (acc.provider === 'antigravity') {
              if (gemini5hBucket) fallbackBucket = gemini5hBucket;
              else if (geminiWeeklyBucket) fallbackBucket = geminiWeeklyBucket;
              else if (claude5hBucket) fallbackBucket = claude5hBucket;
            }

            // 构建精细的双指示条渲染
            let metersHtml = '';
            let cooldownCompactNotice = '';
            if (health.cooldown && health.cooldown.scope === 'models') {
              cooldownCompactNotice = `<div style="font-size:10px; color:#d97706; margin-bottom:2px; font-family:var(--mono);">${escapeHtml(health.detail || health.label)}</div>`;
            }

            const buildResetBadge = (bkt) => {
              if (!bkt) return '';
              const rTime = bkt.reset_time || bkt.resetTime || (typeof bkt.targetTs === 'number' ? bkt.targetTs : '');
              const rCountdown = bkt.reset_countdown || bkt.countdown || '';
              if (!rTime && !rCountdown) return '';
              const fn = typeof formatCompactCountdown === 'function' ? formatCompactCountdown : (window.formatCompactCountdown || null);
              const text = fn ? fn(rTime, rCountdown) : (rCountdown || '');
              if (!text) return '';
              const absFn = typeof formatResetAbs === 'function' ? formatResetAbs : (window.formatResetAbs || (s => s));
              let absStr = '';
              if (rTime) {
                try {
                  absStr = typeof rTime === 'number' ? new Date(rTime).toLocaleString() : absFn(rTime);
                } catch (_) {}
              }
              const fullTip = absStr
                ? (t('quotas.estimatedResetHint') + ': ' + absStr + (rCountdown ? ` (${rCountdown})` : ''))
                : (t('quotas.estimatedResetHint') + ': ' + rCountdown);
              const dataAttr = rTime ? `data-reset-time-compact="${escapeHtml(String(rTime))}"` : '';
              return `
                <span class="quota-reset-badge" ${dataAttr} title="${escapeHtml(fullTip)}">
                  <svg class="quota-reset-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg><span class="quota-reset-val">${escapeHtml(text)}</span>
                </span>
              `;
            };
            if (isCredentialBlocked && !gemini5hBucket && !claude5hBucket && !codex5hBucket) {
              metersHtml = `<div class="account-health-warning">${escapeHtml(health.detail || health.label || t('quotas.cpaCredentialUnavailable'))} ${t('quotas.quotaHiddenSuffix')}</div>`;
            } else if (acc.raw_error) {
              metersHtml = `<div class="account-health-warning" style="color:var(--danger); font-size:11px;" title="${escapeHtml(acc.raw_error)}">${escapeHtml(acc.raw_error)}</div>`;
            } else if ((acc.provider === 'antigravity' && !Number.isFinite(gemini5hBucket?.remaining_percent) && !Number.isFinite(claude5hBucket?.remaining_percent) && !Number.isFinite(fallbackBucket?.remaining_percent)) || (acc.provider === 'codex' && !Number.isFinite(codex5hBucket?.remaining_percent) && !Number.isFinite(fallbackBucket?.remaining_percent))) {
              metersHtml = cooldownCompactNotice + `<div class="account-quota-unknown">${t('quotas.quotaUnknownUpstream')}</div>`;
            } else if (acc.provider === 'opencode') {
              const todayCount = Number.isFinite(acc.today_requests) ? acc.today_requests : 0;
              const todayTokens = Number.isFinite(acc.today_tokens) ? acc.today_tokens : 0;

              metersHtml = `
                <div class="compact-meter-wrap" style="display:flex; flex-direction:column; justify-content:center; gap:2px;" title="${t('quotas.opencode.todayCallVolumeStats')}">
                  <div style="display:flex; justify-content:space-between; align-items:center; font-size:11.5px;">
                    <span style="color:var(--text-muted); font-size:11px;">${t('quotas.opencode.todayConsumed')}</span>
                    <span style="font-family:var(--mono); font-weight:700; color:var(--text-main);">
                      <span style="color:#0ea5e9; font-size:13px;">${formatTokens(todayTokens)}</span>
                      <span style="color:var(--text-dim); font-size:10.5px;"> Tokens</span>
                    </span>
                  </div>
                  <div style="display:flex; justify-content:space-between; align-items:center; font-size:10.5px; color:var(--text-dim);">
                    <span>${t('quotas.todayRequestsLabel')} <b style="font-family:var(--mono); color:var(--text-muted);">${todayCount}</b> ${t('quotas.timesUnit')}</span>
                    <span>${t('quotas.naturalDayCumulative')}</span>
                  </div>
                </div>
              `;
            } else if (acc.provider === 'openrouter') {
              const todayCount = Number.isFinite(acc.today_requests) ? acc.today_requests : 0;
              const resetTxt = (acc.reset_time || acc.reset_countdown) ? escapeHtml(acc.reset_countdown || (formatResetAbs(acc.reset_time) + t('quotas.actionRefresh'))) : '00:00 UTC';

              metersHtml = `
                <div class="compact-meter-wrap" style="display:flex; flex-direction:column; justify-content:center; gap:2px;" title="${t('quotas.openrouter.todayModelCallStats')}">
                  <div style="display:flex; justify-content:space-between; align-items:center; font-size:11.5px;">
                    <span style="color:var(--text-muted); font-size:11px;">${t('quotas.todayCallsLabel')}</span>
                    <span style="font-family:var(--mono); font-weight:700; color:var(--text-main);">
                      <span style="color:#3b82f6; font-size:13.5px;">${todayCount}</span>
                      <span style="color:var(--text-dim); font-size:11px;"> ${t('quotas.timesUnit')}</span>
                    </span>
                  </div>
                  <div style="display:flex; justify-content:space-between; align-items:center; font-size:10.5px; color:var(--text-dim);">
                    <span>${t('quotas.statsCycleUtcDay')}</span>
                    <span style="font-family:var(--mono);" ${acc.reset_time ? `data-reset-time="${escapeHtml(acc.reset_time)}"` : ''}>${t('quotas.resetLabel', { resetTxt: resetTxt })}</span>
                  </div>
                </div>
              `;
            } else if (acc.provider === 'workbuddy' || acc.provider === 'workbuddy-cn') {
              const remainCredits = Number(acc.remaining_credits) || 0;
              const totalCredits = Number(acc.total_credits) || 0;
              const usedCredits = Number(acc.used_credits) || 0;
              const pct = totalCredits > 0 ? Math.max(0, Math.min(100, Math.round((remainCredits / totalCredits) * 1000) / 10)) : (remainCredits > 0 ? 100 : 0);
              let cls = 'meter-high'; let col = '#10b981';
              if (pct <= 20) { cls = 'meter-low'; col = '#ef4444'; }
              else if (pct <= 50) { cls = 'meter-medium'; col = '#f59e0b'; }

              const expInfo = getWorkBuddyExpirationInfo(acc);
              const title = expInfo.tooltip || t('quotas.workbuddy.officialPointsTooltip', { remainCredits: remainCredits, totalCredits: totalCredits });
              const wbResetBadge = expInfo.hasExpiration ? buildResetBadge({ reset_time: expInfo.targetTs, reset_countdown: expInfo.countdown }) : '';

              metersHtml = `
                <div class="compact-meter-wrap" title="${escapeHtml(title)}">
                  <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                    <span style="display:inline-flex; align-items:center; gap:4px;">
                      <span class="quota-pool-badge pool-workbuddy">WorkBuddy</span>
                      <span style="color:var(--text-muted); font-size:10px;">${t('quotas.workbuddy.packPoints')}</span>
                    </span>
                    <span style="display:inline-flex; align-items:center; gap:6px;">
                      ${wbResetBadge}
                      <span style="font-family:var(--mono); font-weight:700; color:${col};">
                        ${t('quotas.points.remainingPoints', { value: remainCredits.toLocaleString(undefined, {minimumFractionDigits: 0, maximumFractionDigits: 2}) })}
                      </span>
                    </span>
                  </div>
                  <div class="quota-meter-bar" style="height:4px;">
                    <div class="quota-meter-fill ${cls}" style="width: ${pct}%"></div>
                  </div>
                </div>
              `;
            } else if (acc.provider && acc.provider.toLowerCase().startsWith('qoder')) {
              const remainCredits = Number(acc.remaining_credits) || 0;
              const totalCredits = Number(acc.total_credits) || 0;
              const pct = totalCredits > 0 ? Math.max(0, Math.min(100, Math.round((remainCredits / totalCredits) * 1000) / 10)) : (remainCredits > 0 ? 100 : (fallbackBucket && Number.isFinite(fallbackBucket.remaining_percent) ? fallbackBucket.remaining_percent : 0));
              let cls = 'meter-high'; let col = '#10b981';
              if (pct <= 20) { cls = 'meter-low'; col = '#ef4444'; }
              else if (pct <= 50) { cls = 'meter-medium'; col = '#f59e0b'; }

              const pointsLabel = totalCredits > 0 ? `${remainCredits.toLocaleString(undefined, {minimumFractionDigits: 0, maximumFractionDigits: 1})} / ${totalCredits.toLocaleString(undefined, {minimumFractionDigits: 0, maximumFractionDigits: 1})}` : `${remainCredits.toLocaleString(undefined, {minimumFractionDigits: 0, maximumFractionDigits: 1})} pts`;
              const qoderResetBadge = buildResetBadge(fallbackBucket || acc);

              metersHtml = `
                <div class="compact-meter-wrap" title="Qoder 资源额度点数 (${escapeHtml(pointsLabel)})">
                  <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                    <span style="display:inline-flex; align-items:center; gap:4px;">
                      <span class="quota-pool-badge pool-qoder">Qoder</span>
                      <span style="color:var(--text-muted); font-size:10px;">资源点数</span>
                    </span>
                    <span style="display:inline-flex; align-items:center; gap:6px;">
                      ${qoderResetBadge}
                      <span style="font-family:var(--mono); font-weight:700; color:${col};">
                        ${escapeHtml(pointsLabel)}
                      </span>
                    </span>
                  </div>
                  <div class="quota-meter-bar" style="height:4px;">
                    <div class="quota-meter-fill ${cls}" style="width: ${pct}%"></div>
                  </div>
                </div>
              `;
            } else if (!Number.isFinite(fallbackBucket?.remaining_percent)) {
              metersHtml = `<div class="account-quota-unknown">${t('quotas.quotaUnknownCpa')}</div>`;
            } else if (acc.provider === 'antigravity') {
              const hasGemini5h = gemini5hBucket && Number.isFinite(gemini5hBucket.remaining_percent);
              const hasGeminiWeekly = geminiWeeklyBucket && Number.isFinite(geminiWeeklyBucket.remaining_percent);

              const buildAntigravityMeter = (bkt, weeklyBkt, defaultLabel, isWeeklyMeter = false) => {
                if (!bkt || !Number.isFinite(bkt.remaining_percent)) return '';
                const isModelCooling = health.cooldown && health.cooldown.scope === 'models' && (
                  (health.cooldown.model_keys || []).some(k => /gemini|flash/i.test(k) && !/claude/i.test(k))
                );

                const rawRem = bkt.remaining_percent;
                const isBktDisabled = bkt.disabled === true;
                const weeklyRem = (weeklyBkt && Number.isFinite(weeklyBkt.remaining_percent)) ? weeklyBkt.remaining_percent : null;
                const isWeeklyBottleneck = !isWeeklyMeter && weeklyRem !== null && (isBktDisabled || weeklyRem < rawRem);
                const effectiveRem = isBktDisabled ? 0 : (isWeeklyBottleneck ? weeklyRem : rawRem);

                let rem = isModelCooling ? 0 : effectiveRem;
                let cls = 'meter-high'; let col = '#10b981';
                if (rem <= 25) { cls = 'meter-low'; col = '#ef4444'; }
                else if (rem <= 60) { cls = 'meter-medium'; col = '#f59e0b'; }

                let title = isWeeklyMeter ? t('quotas.bucket.geminiWeeklyTotal') : t('quotas.bucket.geminiRolling5h');
                if (bkt?.reset_time || bkt?.reset_countdown) {
                  const cd = bkt.reset_countdown || formatResetAbs(bkt.reset_time);
                  title += ` (${cd})`;
                }
                if (weeklyBkt && Number.isFinite(weeklyBkt.remaining_percent) && !isWeeklyMeter) {
                  const cdSuffix = weeklyBkt.reset_countdown ? t('quotas.bucket.weeklyCountdownSuffix', { time: weeklyBkt.reset_countdown }) : '';
                  title += t('quotas.bucket.weeklyRemainingDetail', { percent: weeklyBkt.remaining_percent, countdown: cdSuffix });
                }
                if (isBktDisabled) {
                  title += t('quotas.bucket.upstreamDisabledNotice');
                } else if (isWeeklyBottleneck) {
                  title += t('quotas.bucket.bottleneckConstraintNotice', { rem5h: rawRem, remWeekly: weeklyRem });
                }

                const resetBadge = buildResetBadge(bkt);
                return `
                  <div class="compact-meter-wrap" title="${escapeHtml(title)}">
                    <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                      <span style="display:inline-flex; align-items:center; gap:4px;">
                        <span class="quota-pool-badge pool-gemini">Gemini</span>
                        <span style="color:var(--text-muted); font-size:10px;">${escapeHtml(defaultLabel)}</span>
                      </span>
                      <span style="display:inline-flex; align-items:center; gap:6px;">
                        ${resetBadge}
                        <span style="font-family:var(--mono); font-weight:700; color:${col};">${t('quotas.bucket.firstRemainingPercent', { rem1: rem })}</span>
                      </span>
                    </div>
                    <div class="quota-meter-bar" style="height:4px;">
                      <div class="quota-meter-fill ${cls}" style="width: ${rem}%"></div>
                    </div>
                  </div>
                `;
              };

              if (hasGemini5h) {
                // 简短展示专属：严格只展示 Gemini 的 5h 与周额度，标明池子标识
                const col2Html = hasGeminiWeekly
                  ? buildAntigravityMeter(geminiWeeklyBucket, null, t('quotas.bucket.weeklyLimit'), true)
                  : '';
                metersHtml = `
                  ${buildAntigravityMeter(gemini5hBucket, geminiWeeklyBucket, t('quotas.bucket.fiveHourLimit'), false)}
                  ${col2Html}
                `;
              } else if (hasGeminiWeekly) {
                metersHtml = `
                  ${buildAntigravityMeter(geminiWeeklyBucket, null, t('quotas.bucket.weeklyLimit'), true)}
                `;
              } else if (fallbackBucket && Number.isFinite(fallbackBucket.remaining_percent)) {
                metersHtml = `
                  ${buildAntigravityMeter(fallbackBucket, null, formatBucketLabel(fallbackBucket.name, fallbackBucket.window) || '5h', false)}
                `;
              } else {
                metersHtml = cooldownCompactNotice + `<div class="account-quota-unknown">${t('quotas.quotaUnknownUpstream')}</div>`;
              }
            } else if (acc.provider === 'codex') {
              const b1 = (codex5hBucket && Number.isFinite(codex5hBucket.remaining_percent)) ? codex5hBucket : fallbackBucket;
              const b2 = codexWeeklyBucket;

              if (!b1 || !Number.isFinite(b1.remaining_percent)) {
                metersHtml = cooldownCompactNotice + `<div class="account-quota-unknown">${t('quotas.quotaUnknownUpstream')}</div>`;
              } else {
                const rem1 = b1.remaining_percent;
                let cls1 = 'meter-high'; let col1 = '#10b981';
                if (rem1 <= 25) { cls1 = 'meter-low'; col1 = '#ef4444'; }
                else if (rem1 <= 60) { cls1 = 'meter-medium'; col1 = '#f59e0b'; }

                let title1 = t('quotas.bucket.codexRolling5h');
                if (b1?.reset_time || b1?.reset_countdown) {
                  title1 += ` (${b1.reset_countdown || formatResetAbs(b1.reset_time)})`;
                }

                let col2Html = '';
                if (b2 && Number.isFinite(b2.remaining_percent)) {
                  const rem2 = b2.remaining_percent;
                  let cls2 = 'meter-high'; let col2 = '#10b981';
                  if (rem2 <= 25) { cls2 = 'meter-low'; col2 = '#ef4444'; }
                  else if (rem2 <= 60) { cls2 = 'meter-medium'; col2 = '#f59e0b'; }

                  let title2 = t('quotas.bucket.codexWeekly');
                  if (b2?.reset_time || b2?.reset_countdown) {
                    title2 += ` (${b2.reset_countdown || formatResetAbs(b2.reset_time)})`;
                  }

                  const resetBadge2 = buildResetBadge(b2);
                  col2Html = `
                    <div class="compact-meter-wrap" title="${escapeHtml(title2)}">
                      <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                        <span style="display:inline-flex; align-items:center; gap:4px;">
                          <span class="quota-pool-badge pool-codex">Codex</span>
                          <span style="color:var(--text-muted); font-size:10px;">${t('quotas.bucket.weeklyLimit')}</span>
                        </span>
                        <span style="display:inline-flex; align-items:center; gap:6px;">
                          ${resetBadge2}
                          <span style="font-family:var(--mono); font-weight:700; color:${col2};">${t('quotas.bucket.secondRemainingPercent', { rem2: rem2 })}</span>
                        </span>
                      </div>
                      <div class="quota-meter-bar" style="height:4px;">
                        <div class="quota-meter-fill ${cls2}" style="width: ${rem2}%"></div>
                      </div>
                    </div>
                  `;
                }

                const resetBadge1 = buildResetBadge(b1);
                metersHtml = `
                  <div class="compact-meter-wrap" title="${escapeHtml(title1)}">
                    <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                      <span style="display:inline-flex; align-items:center; gap:4px;">
                        <span class="quota-pool-badge pool-codex">Codex</span>
                        <span style="color:var(--text-muted); font-size:10px;">${t('quotas.bucket.fiveHourLimit')}</span>
                      </span>
                      <span style="display:inline-flex; align-items:center; gap:6px;">
                        ${resetBadge1}
                        <span style="font-family:var(--mono); font-weight:700; color:${col1};">${t('quotas.bucket.firstRemainingPercent', { rem1: rem1 })}</span>
                      </span>
                    </div>
                    <div class="quota-meter-bar" style="height:4px;">
                      <div class="quota-meter-fill ${cls1}" style="width: ${rem1}%"></div>
                    </div>
                  </div>
                  ${col2Html}
                `;
              }
            } else if (acc.provider === 'claude') {
              const b1 = (claude5hBucket && Number.isFinite(claude5hBucket.remaining_percent)) ? claude5hBucket : fallbackBucket;
              const b2 = claudeWeeklyBucket;

              if (!b1 || !Number.isFinite(b1.remaining_percent)) {
                metersHtml = cooldownCompactNotice + `<div class="account-quota-unknown">${t('quotas.quotaUnknownUpstream')}</div>`;
              } else {
                const rem1 = b1.remaining_percent;
                let cls1 = 'meter-high'; let col1 = '#10b981';
                if (rem1 <= 25) { cls1 = 'meter-low'; col1 = '#ef4444'; }
                else if (rem1 <= 60) { cls1 = 'meter-medium'; col1 = '#f59e0b'; }

                let title1 = 'Claude 5-Hour Limit';
                if (b1?.reset_time || b1?.reset_countdown) {
                  title1 += ` (${b1.reset_countdown || formatResetAbs(b1.reset_time)})`;
                }

                let col2Html = '';
                if (b2 && Number.isFinite(b2.remaining_percent)) {
                  const rem2 = b2.remaining_percent;
                  let cls2 = 'meter-high'; let col2 = '#10b981';
                  if (rem2 <= 25) { cls2 = 'meter-low'; col2 = '#ef4444'; }
                  else if (rem2 <= 60) { cls2 = 'meter-medium'; col2 = '#f59e0b'; }

                  let title2 = 'Claude 7-Day Limit';
                  if (b2?.reset_time || b2?.reset_countdown) {
                    title2 += ` (${b2.reset_countdown || formatResetAbs(b2.reset_time)})`;
                  }

                  const resetBadge2 = buildResetBadge(b2);
                  col2Html = `
                    <div class="compact-meter-wrap" title="${escapeHtml(title2)}">
                      <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                        <span style="display:inline-flex; align-items:center; gap:4px;">
                          <span class="quota-pool-badge pool-claude">Claude</span>
                          <span style="color:var(--text-muted); font-size:10px;">${t('quotas.bucket.weeklyLimit')}</span>
                        </span>
                        <span style="display:inline-flex; align-items:center; gap:6px;">
                          ${resetBadge2}
                          <span style="font-family:var(--mono); font-weight:700; color:${col2};">${t('quotas.bucket.secondRemainingPercent', { rem2: rem2 })}</span>
                        </span>
                      </div>
                      <div class="quota-meter-bar" style="height:4px;">
                        <div class="quota-meter-fill ${cls2}" style="width: ${rem2}%"></div>
                      </div>
                    </div>
                  `;
                }

                const resetBadge1 = buildResetBadge(b1);
                metersHtml = `
                  <div class="compact-meter-wrap" title="${escapeHtml(title1)}">
                    <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                      <span style="display:inline-flex; align-items:center; gap:4px;">
                        <span class="quota-pool-badge pool-claude">Claude</span>
                        <span style="color:var(--text-muted); font-size:10px;">${t('quotas.bucket.fiveHourLimit')}</span>
                      </span>
                      <span style="display:inline-flex; align-items:center; gap:6px;">
                        ${resetBadge1}
                        <span style="font-family:var(--mono); font-weight:700; color:${col1};">${t('quotas.bucket.firstRemainingPercent', { rem1: rem1 })}</span>
                      </span>
                    </div>
                    <div class="quota-meter-bar" style="height:4px;">
                      <div class="quota-meter-fill ${cls1}" style="width: ${rem1}%"></div>
                    </div>
                  </div>
                  ${col2Html}
                `;
              }
            } else if (fallbackBucket && Number.isFinite(fallbackBucket.remaining_percent)) {
              const b1 = fallbackBucket;
              const rem1 = b1.remaining_percent;
              let cls1 = 'meter-high'; let col1 = '#10b981';
              if (rem1 <= 25) { cls1 = 'meter-low'; col1 = '#ef4444'; }
              else if (rem1 <= 60) { cls1 = 'meter-medium'; col1 = '#f59e0b'; }
              const bname = formatBucketLabel(b1.name, b1.window) || t('quotas.bucket.standardQuota');

              let title1 = bname;
              if (b1?.reset_time || b1?.reset_countdown) {
                title1 += ` (${b1.reset_countdown || formatResetAbs(b1.reset_time)})`;
              }

              const resetBadge1 = buildResetBadge(b1);
              metersHtml = `
                <div class="compact-meter-wrap" title="${escapeHtml(title1)}">
                  <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                    <span style="color:var(--text-muted); font-size:10.5px;">${escapeHtml(bname)}</span>
                    <span style="display:inline-flex; align-items:center; gap:6px;">
                      ${resetBadge1}
                      <span style="font-family:var(--mono); font-weight:700; color:${col1};">${t('quotas.bucket.firstRemainingPercent', { rem1: rem1 })}</span>
                    </span>
                  </div>
                  <div class="quota-meter-bar" style="height:4px;">
                    <div class="quota-meter-fill ${cls1}" style="width: ${rem1}%"></div>
                  </div>
                </div>
              `;
            } else {
              metersHtml = cooldownCompactNotice + `<div class="account-quota-unknown">${t('quotas.quotaUnknownUpstream')}</div>`;
            }

            const todayCount = Number.isFinite(acc.today_requests) ? acc.today_requests : 0;
            const todayTokens = Number.isFinite(acc.today_tokens) ? acc.today_tokens : 0;
            const fiveHTokens = Number.isFinite(acc.five_hour_tokens) ? acc.five_hour_tokens : 0;
            const fiveHRequests = Number.isFinite(acc.five_hour_requests) ? acc.five_hour_requests : 0;

            const localUsageHtml = `
              <div class="compact-meter-wrap compact-local-usage" style="background:rgba(241, 245, 249, 0.65); border:1px solid rgba(186, 215, 242, 0.4); border-radius:6px; padding:4px 8px; justify-content:center; gap:2px;" title="${t('quotas.localUsageTooltip', { today: todayTokens.toLocaleString(LOCALE), todayReq: todayCount, fiveH: fiveHTokens.toLocaleString(LOCALE), fiveHReq: fiveHRequests })}">
                <div style="display:flex; justify-content:space-between; align-items:center; font-size:10.5px;">
                  <span style="color:var(--text-muted); font-weight:600;">${t('quotas.localAuthenticToday')}</span>
                  <span style="font-family:var(--mono); font-weight:700; color:#0284c7;">
                    ${formatTokens(todayTokens)} <span style="font-size:9.5px; font-weight:normal; color:var(--text-dim);">Tokens (${todayCount}${t('quotas.timesUnit')})</span>
                  </span>
                </div>
                <div style="display:flex; justify-content:space-between; align-items:center; font-size:10px; color:var(--text-dim); font-family:var(--mono);">
                  <span>${t('quotas.localAuthentic5h')}</span>
                  <span style="color:#6366f1; font-weight:600;">${formatTokens(fiveHTokens)} <span style="font-size:9px; font-weight:normal; color:var(--text-dim);">Tokens (${fiveHRequests}${t('quotas.timesUnit')})</span></span>
                </div>
              </div>
            `;

            const valInfo = acc.validation;
            const hasValidation = Boolean(valInfo && valInfo.status === 'pending');
            const safeValUrl = valInfo && valInfo.validation_url ? escapeHtml(valInfo.validation_url) : '';
            const accountTitle = (acc.display_name || acc.email || acc.filename || '').replace(/\.json$/i, '');
            const fullTip = acc.email && acc.email !== accountTitle ? `${accountTitle} (${acc.email})` : accountTitle;

            return `
              <div class="compact-account-row ${isDisabled ? 'is-disabled' : ''}" style="${hasValidation ? 'border-color:rgba(239,68,68,0.5); background:rgba(239,68,68,0.03);' : ''}">
                <div class="compact-left">
                  <div class="provider-icon-badge ${brand.iconClass}" title="${escapeHtml(brand.name)}">${brand.iconSvg}</div>
                  <div style="display:flex; flex-direction:column; min-width:0; gap:2px;">
                    <div style="display:flex; align-items:center; gap:6px; flex-wrap:nowrap;">
                      <span class="provider-pill ${brand.pillClass}" style="font-size:10px; padding:1px 5px;">${escapeHtml(brand.name)}</span>
                      <span class="priority-badge ${prioCls}">P${prio}</span>
                      <span style="font-weight:600; font-size:12.5px; color:var(--text-main); overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${escapeHtml(fullTip)}">
                        ${escapeHtml(accountTitle)}
                      </span>
                      ${isDisabled ? `<span class="chip chip-red" style="font-size:10px; padding:1px 5px; line-height:1.2; white-space:nowrap; flex-shrink:0;">${t('quotas.disabledBadge')}</span>` : ''}
                      ${isBlocked && !isDisabled ? `<span class="chip ${health.cooldown && health.cooldown.scope === 'models' ? 'chip-orange' : 'chip-red'}" style="font-size:10px; padding:1px 5px; line-height:1.2; ${health.cooldown && health.cooldown.scope === 'models' ? 'background:rgba(245,158,11,0.15); color:#d97706; border:1px solid rgba(245,158,11,0.3);' : ''}">${escapeHtml(health.label)}</span>` : ''}
                      ${hasValidation ? `<span class="chip chip-red" style="font-size:10px; padding:1px 5px; line-height:1.2; white-space:nowrap; flex-shrink:0; background:rgba(239,68,68,0.15); border:1px solid rgba(239,68,68,0.3); animation:pulse 2s infinite;">${t('quotas.validation.needGoogleBadge')}</span>` : ''}
                    </div>
                    <div style="font-size:10.5px; font-family:var(--mono); color:var(--text-dim);">
                      ID: ${acc.auth_index ? escapeHtml(String(acc.auth_index).slice(0, 12)) : '—'} ${isCredentialBlocked ? '' : `· <span class="account-card-plan ${planClass}" style="padding:0 5px; font-size:9.5px;">${escapeHtml(acc.plan_name || acc.plan_type)}</span>`}
                    </div>
                  </div>
                </div>

                <div class="compact-mid">
                  ${metersHtml}
                  ${localUsageHtml}
                </div>

                <div class="compact-right">
                  ${hasValidation ? `
                    <a href="${safeValUrl}" target="_blank" rel="noopener noreferrer" class="priority-btn" style="color:#ef4444; border-color:rgba(239,68,68,0.4); background:rgba(239,68,68,0.08); padding:3px 9px; font-size:11px; text-decoration:none; display:inline-flex; align-items:center; gap:4px;" title="${t('quotas.validation.openLinkHint')}">
                      ${t('quotas.validation.goVerifyBtn')}
                    </a>
                    <button class="priority-btn" style="color:#10b981; border-color:rgba(16,185,129,0.3); background:rgba(16,185,129,0.06); padding:3px 7px; font-size:11px;" onclick="resolveValidation(${valInfo.id})" title="${t('quotas.validation.clearedHint')}">
                      ${t('quotas.validation.verifiedBadge')}
                    </button>
                  ` : ''}
                  ${isDisabled ? '' : `<button class="priority-btn" style="color:#2563eb; border-color:rgba(37,99,235,0.3); background:rgba(37,99,235,0.06); padding:3px 9px; font-size:11px;" data-filename="${safeFilename}" data-provider="${escapeHtml(acc.provider || '')}" data-authindex="${escapeHtml(acc.auth_index || '')}" onclick="triggerActivateQuotaWindow(this.dataset.filename, this.dataset.provider, this.dataset.authindex)" title="${t('quotas.requeryQuotaTitle')}">
                    ${t('quotas.refreshQuotaBtn')}
                  </button>`}
                  <button class="priority-btn" style="padding:3px 8px; font-size:11px;" data-filename="${safeFilename}" onclick="openPriorityModal(this.dataset.filename)" title="${t('quotas.priority.adjustTitle')}">
                    ${t('quotas.priority.adjustBtn')}
                  </button>
                </div>
              </div>
            `;
          }).join('');
        }
      } else {
        if (compactContainer) compactContainer.style.display = 'none';
        if (cardsContainer) {
          cardsContainer.style.display = 'grid';
          cardsContainer.innerHTML = filtered.map(acc => {
            const safeFilename = escapeHtml(acc.filename || acc.id || '');
            const brand = getProviderBrand(acc.provider, acc.display_name || acc.name);
            const planType = (acc.plan_type || 'free').toLowerCase();
            let planClass = 'plan-free';
            if (planType.includes('pro') || planType.includes('ultra')) planClass = 'plan-pro';
            else if (planType.includes('plus')) planClass = 'plan-plus';

            const isDisabled = Boolean(acc.disabled || acc.status === 'disabled');
            const health = acc.health || {};
            const isBlocked = health.blocked === true;
            const isCredentialBlocked = isBlocked && (!health.cooldown || health.cooldown.scope === 'credential' || health.cooldown.scope === 'credential_and_models');
            const stRaw = String(acc.status || '').toLowerCase();
            const stColor = (isCredentialBlocked || isDisabled || stRaw.includes('error') || stRaw.includes('invalid')) ? 'var(--danger)' : (health.cooldown && health.cooldown.scope === 'models' ? '#d97706' : 'var(--success)');
            const prio = normalizeAuthPriority(acc.priority);
            const prioCls = prio >= 20 ? 'p-high' : (prio >= 10 ? 'p-normal' : 'p-low');

            // 渲染模型组
            const sortedGroups = [...(acc.groups || [])].filter(grp => Array.isArray(grp.buckets) && grp.buckets.some(bkt => Number.isFinite(bkt.remaining_percent)));
            sortedGroups.sort((a, b) => {
              const aLow = String(a.name || '').toLowerCase();
              const bLow = String(b.name || '').toLowerCase();
              const aIsGemini = aLow.includes('gemini');
              const bIsGemini = bLow.includes('gemini');
              if (aIsGemini && !bIsGemini) return -1;
              if (!aIsGemini && bIsGemini) return 1;
              return 0;
            });

            const groupsHtml = sortedGroups.map(grp => {
              const gnameLower = String(grp.name || '').toLowerCase();
              const isGemini = gnameLower.includes('gemini');
              const isClaude = gnameLower.includes('claude') || gnameLower.includes('gpt') || gnameLower.includes('3p');
              const isCodex = gnameLower.includes('codex') || acc.provider === 'codex';
              const isQoder = gnameLower.includes('qoder') || (acc.provider && acc.provider.toLowerCase().startsWith('qoder'));
              const isWorkbuddy = gnameLower.includes('workbuddy') || (acc.provider && acc.provider.toLowerCase().startsWith('workbuddy'));

              let poolBadge = '';
              let displayTitle = escapeHtml(grp.name);
              let displayDesc = '';

              if (isGemini) {
                poolBadge = `<span class="quota-pool-badge pool-gemini">Gemini</span>`;
                displayTitle = t('quotas.group.geminiModels');
                displayDesc = t('quotas.group.geminiDesc');
              } else if (isClaude) {
                poolBadge = `<span class="quota-pool-badge pool-claude">Claude / GPT</span>`;
                displayTitle = t('quotas.group.claudeModels');
                displayDesc = t('quotas.group.claudeDesc');
              } else if (isCodex) {
                poolBadge = `<span class="quota-pool-badge pool-codex">Codex</span>`;
              } else if (isQoder) {
                poolBadge = `<span class="quota-pool-badge pool-qoder">Qoder</span>`;
              } else if (isWorkbuddy) {
                poolBadge = `<span class="quota-pool-badge pool-workbuddy">WorkBuddy</span>`;
              }

              const sortedBuckets = [...(grp.buckets || [])].filter(bkt => Number.isFinite(bkt.remaining_percent));
              sortedBuckets.sort((a, b) => {
                const aIs5h = (a.window === '5h') || String(a.name || '').toLowerCase().includes('5h') || String(a.name || '').toLowerCase().includes('five');
                const bIs5h = (b.window === '5h') || String(b.name || '').toLowerCase().includes('5h') || String(b.name || '').toLowerCase().includes('five');
                if (aIs5h && !bIs5h) return -1;
                if (!aIs5h && bIs5h) return 1;
                return 0;
              });

              const bucketsHtml = sortedBuckets.map(bkt => {
                const isBktDisabled = bkt.disabled === true;
                const rem = isBktDisabled ? 0 : bkt.remaining_percent;
                let meterClass = 'meter-high';
                let remColor = '#10b981';
                if (rem <= 25) { meterClass = 'meter-low'; remColor = '#ef4444'; }
                else if (rem <= 60) { meterClass = 'meter-medium'; remColor = '#f59e0b'; }

                const bktDispName = formatBucketLabel(bkt.name, bkt.window);
                const disabledTag = isBktDisabled ? ` <span style="font-size:10px; font-weight:normal; opacity:0.85;">${t('quotas.bucket.tagUpstreamDisabled')}</span>` : '';

                return `
                  <div class="quota-meter-row">
                    <div class="quota-meter-header">
                      <span class="quota-window-name">${escapeHtml(bktDispName)}${disabledTag}</span>
                      <span class="quota-window-rem" style="color:${remColor};">${t('quotas.bucket.remainingPercent', { rem: rem })}</span>
                    </div>
                    <div class="quota-meter-bar">
                      <div class="quota-meter-fill ${meterClass}" style="width: ${rem}%"></div>
                    </div>
                    ${(bkt.reset_time || bkt.reset_countdown) ? `
                      <div class="quota-meter-footer">
                        <span class="quota-reset-abs" title="${t('quotas.estimatedResetHint')}">${bkt.reset_time ? `${t('quotas.estimatedPrefix')} ` + escapeHtml(formatResetAbs(bkt.reset_time)) + ` ${t('quotas.actionRefresh')}` : '&nbsp;'}</span>
                        <div style="display:flex; align-items:center; gap:6px;">
                          <span ${bkt.reset_time ? `data-reset-time="${escapeHtml(bkt.reset_time)}"` : ''}>${escapeHtml(bkt.reset_countdown || '')}</span>
                          <button class="btn-activate-5h btn-mini" data-filename="${safeFilename}" data-provider="${escapeHtml(acc.provider || '')}" data-authindex="${escapeHtml(acc.auth_index || '')}" onclick="triggerActivateQuotaWindow(this.dataset.filename, this.dataset.provider, this.dataset.authindex)" title="${t('quotas.requeryQuotaTitle')}">
                            ${t('quotas.refreshQuotaBtn')}
                          </button>
                        </div>
                      </div>
                    ` : `
                      <div class="quota-meter-footer" style="justify-content:flex-end;">
                        <button class="btn-activate-5h btn-mini" data-filename="${safeFilename}" data-provider="${escapeHtml(acc.provider || '')}" data-authindex="${escapeHtml(acc.auth_index || '')}" onclick="triggerActivateQuotaWindow(this.dataset.filename, this.dataset.provider, this.dataset.authindex)" title="${t('quotas.requeryQuotaTitle')}">
                          ${t('quotas.refreshQuotaBtn')}
                        </button>
                      </div>
                    `}
                  </div>
                `;
              }).join('');

              return `
                <div class="model-group-section">
                  <div class="model-group-header">
                    <div class="model-group-title-wrap">
                      ${poolBadge}
                      <span class="model-group-title">${displayTitle}</span>
                    </div>
                    ${displayDesc ? `<span class="model-group-desc">${escapeHtml(displayDesc)}</span>` : ''}
                  </div>
                  ${bucketsHtml}
                </div>
              `;
            }).join('');

            const valInfo = acc.validation;
            const hasValidation = Boolean(valInfo && valInfo.status === 'pending');
            const safeValUrl = valInfo && valInfo.validation_url ? escapeHtml(valInfo.validation_url) : '';

            let quotaSectionHtml = '';
            let cooldownBanner = '';
            if (health.cooldown && health.cooldown.scope === 'models') {
              cooldownBanner = `<div class="account-health-warning" style="margin-bottom:8px; font-size:11.5px; background:rgba(245,158,11,0.08); border:1px solid rgba(245,158,11,0.3); color:#d97706;">${escapeHtml(health.detail || health.label)}</div>`;
            }

            if (isCredentialBlocked && !groupsHtml) {
              quotaSectionHtml = `<div class="account-health-warning">${escapeHtml(health.detail || health.label || t('quotas.cpaCredentialUnavailable'))} ${t('quotas.quotaHiddenSuffix')}</div>`;
            } else if (acc.provider === 'opencode') {
              const todayCount = Number.isFinite(acc.today_requests) ? acc.today_requests : 0;
              const todayTokens = Number.isFinite(acc.today_tokens) ? acc.today_tokens : 0;

              quotaSectionHtml = `
                <div class="model-group-section" style="margin-top:8px;">
                  <div class="model-group-title" style="display:flex; justify-content:space-between; align-items:center;">
                    <span>${t('quotas.opencode.todayCallStats')}</span>
                    <span style="font-size:11px; font-weight:normal; color:var(--text-dim);">${t('quotas.naturalDayCumulative')}</span>
                  </div>
                  <div style="background:var(--bg-subtle, rgba(0,0,0,0.02)); border:1px solid var(--border-subtle, rgba(0,0,0,0.06)); border-radius:8px; padding:12px 14px; margin-top:6px; display:flex; justify-content:space-between; align-items:center;">
                    <div>
                      <div style="font-size:11px; color:var(--text-muted); margin-bottom:2px;">${t('quotas.todayTokenConsumption')}</div>
                      <div style="font-family:var(--mono); line-height:1.2;">
                        <span style="font-size:24px; font-weight:700; color:#0ea5e9;">${formatTokens(todayTokens)}</span>
                        <span style="font-size:12px; color:var(--text-dim); font-weight:normal;" title="${todayTokens.toLocaleString(LOCALE)} Tokens"> Tokens</span>
                      </div>
                    </div>
                    <div style="text-align:right;">
                      <div style="font-size:11px; color:var(--text-muted); margin-bottom:2px;">${t('quotas.todayRequestCountLabel')}</div>
                      <div style="font-family:var(--mono); line-height:1.2;">
                        <span style="font-size:22px; font-weight:700; color:var(--text-main);">${todayCount}</span>
                        <span style="font-size:12px; color:var(--text-dim); font-weight:normal;"> ${t('quotas.timesUnit')}</span>
                      </div>
                    </div>
                  </div>
                  ${acc.raw_error ? `<div class="account-health-warning" style="color:var(--danger); margin-top:6px; font-size:11px;">${escapeHtml(acc.raw_error)}</div>` : ''}
                </div>
              `;
            } else if (acc.provider === 'openrouter') {
              const todayCount = Number.isFinite(acc.today_requests) ? acc.today_requests : 0;
              const resetTxt = (acc.reset_time || acc.reset_countdown) ? escapeHtml(acc.reset_countdown || (formatResetAbs(acc.reset_time) + t('quotas.actionRefresh'))) : '00:00 UTC';

              quotaSectionHtml = `
                <div class="model-group-section" style="margin-top:8px;">
                  <div class="model-group-title" style="display:flex; justify-content:space-between; align-items:center;">
                    <span>${t('quotas.openrouter.modelCallStats')}</span>
                    <span style="font-size:11px; font-weight:normal; color:var(--text-dim);" ${acc.reset_time ? `data-reset-time="${escapeHtml(acc.reset_time)}"` : ''}>${t('quotas.resetLabel', { resetTxt: resetTxt })}</span>
                  </div>
                  <div style="background:var(--bg-subtle, rgba(0,0,0,0.02)); border:1px solid var(--border-subtle, rgba(0,0,0,0.06)); border-radius:8px; padding:12px 14px; margin-top:6px; display:flex; justify-content:space-between; align-items:center;">
                    <div>
                      <div style="font-size:11px; color:var(--text-muted); margin-bottom:2px;">${t('quotas.todayCalledLabel')}</div>
                      <div style="font-family:var(--mono); line-height:1.2;">
                        <span style="font-size:24px; font-weight:700; color:#3b82f6;">${todayCount}</span>
                        <span style="font-size:13px; color:var(--text-dim); font-weight:normal;"> ${t('quotas.timesUnit')}</span>
                      </div>
                    </div>
                    <div style="text-align:right;">
                      <div style="font-size:11px; color:var(--text-dim);">${t('quotas.statsRangeLabel')}</div>
                      <div style="font-size:11.5px; font-family:var(--mono); color:var(--text-muted); margin-top:2px;">UTC 00:00 - 24:00</div>
                    </div>
                  </div>
                  ${acc.raw_error ? `<div class="account-health-warning" style="color:var(--danger); margin-top:6px; font-size:11px;">${escapeHtml(acc.raw_error)}</div>` : ''}
                </div>
              `;
            } else if (acc.provider === 'workbuddy' || acc.provider === 'workbuddy-cn') {
              const remainCredits = Number(acc.remaining_credits) || 0;
              const totalCredits = Number(acc.total_credits) || 0;
              const usedCredits = Number(acc.used_credits) || 0;
              const pct = totalCredits > 0 ? Math.max(0, Math.min(100, Math.round((remainCredits / totalCredits) * 1000) / 10)) : (remainCredits > 0 ? 100 : 0);
              let meterClass = 'meter-high'; let remColor = '#10b981';
              if (pct <= 20) { meterClass = 'meter-low'; remColor = '#ef4444'; }
              else if (pct <= 50) { meterClass = 'meter-medium'; remColor = '#f59e0b'; }

              const expInfo = getWorkBuddyExpirationInfo(acc);

              quotaSectionHtml = `
                <div class="model-group-section" style="margin-top:8px;">
                  <div class="model-group-title" style="display:flex; justify-content:space-between; align-items:center;">
                    <span>${t('quotas.workbuddy.packsAndPointsTitle')}</span>
                    <span style="font-size:11px; font-weight:normal; color:var(--text-dim);">${t('quotas.workbuddy.officialMetering')}</span>
                  </div>
                  <div style="background:var(--bg-subtle, rgba(0,0,0,0.02)); border:1px solid var(--border-subtle, rgba(0,0,0,0.06)); border-radius:8px; padding:12px 14px; margin-top:6px;">
                    <div style="display:flex; justify-content:space-between; align-items:center;">
                      <div>
                        <div style="font-size:11px; color:var(--text-muted); margin-bottom:2px;">${t('quotas.points.currentAvailable')}</div>
                        <div style="font-family:var(--mono); line-height:1.2;">
                          <span style="font-size:24px; font-weight:700; color:${remColor};">${remainCredits.toLocaleString(undefined, {minimumFractionDigits: 0, maximumFractionDigits: 2})}</span>
                          <span style="font-size:13px; color:var(--text-dim); font-weight:normal;"> ${t('quotas.points.unit')}</span>
                        </div>
                      </div>
                      <div style="text-align:right;">
                        <div style="font-size:11px; color:var(--text-dim);">${t('quotas.workbuddy.capacityAndRate')}</div>
                        <div style="font-size:13px; font-family:var(--mono); color:var(--text-main); font-weight:600; margin-top:2px;">
                          ${t('quotas.points.totalAmountSuffix', { toLocaleString: totalCredits.toLocaleString(LOCALE) })} <span style="color:${remColor};">${pct}%</span>
                        </div>
                      </div>
                    </div>
                    <div class="quota-meter-bar" style="margin-top:10px; height:6px;">
                      <div class="quota-meter-fill ${meterClass}" style="width: ${pct}%"></div>
                    </div>
                    <div style="display:flex; justify-content:space-between; align-items:center; margin-top:6px; font-size:10.5px; color:var(--text-dim); font-family:var(--mono);">
                      <span>${t('quotas.points.consumedPoints', { value: usedCredits.toLocaleString(undefined, {minimumFractionDigits: 0, maximumFractionDigits: 2}) })}</span>
                      <span>${t('quotas.workbuddy.cyclePacksLabel')}</span>
                    </div>
                    <div style="display:flex; justify-content:space-between; align-items:center; margin-top:8px; padding-top:8px; border-top:1px dashed var(--border-subtle, rgba(0,0,0,0.06)); font-size:11px;">
                      <span style="color:var(--text-muted); display:inline-flex; align-items:center; gap:4px;">
                        <span>⏱️ ${expInfo.isAllLongTerm ? t('quotas.points.quotaLabel') : (expInfo.hasMultiplePackages ? `${t('quotas.workbuddy.pointsBatchExpiry')} <span style="font-size:10px; opacity:0.8;">${t('quotas.workbuddy.packageCountSuffix', { packageCount: expInfo.packageCount })}</span>` : t('quotas.workbuddy.pointsExpiryAt'))}</span>
                      </span>
                      <span style="font-family:var(--mono); font-weight:600; color:${expInfo.color};" title="${escapeHtml(expInfo.tooltip)}">
                        ${expInfo.hasMultiplePackages && !expInfo.isAllLongTerm ? `${t('quotas.workbuddy.firstBatchLabel')} ` : ''}${escapeHtml(expInfo.shortText)}${expInfo.countdown ? ` <span style="font-weight:normal; font-size:10px; opacity:0.85;">(${escapeHtml(expInfo.countdown)})</span>` : ''}
                      </span>
                    </div>
                    ${expInfo.hasMultiplePackages ? `
                      <div style="margin-top:8px; display:flex; flex-direction:column; gap:5px; padding:8px 10px; background:var(--bg-subtle, rgba(0,0,0,0.02)); border-radius:6px; border:1px solid var(--border-subtle, rgba(0,0,0,0.06)); font-size:10.5px; font-family:var(--mono);">
                        ${expInfo.packages.map((pkg, pIdx) => `
                          <div style="display:flex; justify-content:space-between; align-items:center; gap:8px;" title="${t('quotas.workbuddy.packTooltip', { value: escapeHtml(pkg.name), points: pkg.points, value2: escapeHtml(pkg.fullText || pkg.shortText) })}">
                            <span style="color:var(--text-muted); overflow:hidden; text-overflow:ellipsis; white-space:nowrap; max-width:${pkg.isLongTerm ? '72%' : '55%'};">
                              <b style="color:var(--text-dim); font-weight:600;">${pIdx + 1}.</b> ${escapeHtml(pkg.name)} <span style="font-weight:600; color:var(--text-main);">${t('quotas.points.countSuffix', { points: pkg.points })}</span>
                            </span>
                            <span style="color:${pkg.color}; font-weight:500; flex-shrink:0;">
                              ${escapeHtml(pkg.shortText)}${pkg.countdown ? ` <span style="font-size:9.5px; opacity:0.85;">(${escapeHtml(pkg.countdown)})</span>` : ''}
                            </span>
                          </div>
                        `).join('')}
                      </div>
                    ` : ''}
                  </div>
                  ${acc.raw_error ? `<div class="account-health-warning" style="color:var(--danger); margin-top:6px; font-size:11px;">${escapeHtml(acc.raw_error)}</div>` : ''}
                </div>
              `;
            } else if (groupsHtml) {
              quotaSectionHtml = cooldownBanner + groupsHtml;
            } else if (acc.raw_error) {
              quotaSectionHtml = `<div class="account-health-warning" style="color:var(--danger);">${escapeHtml(acc.raw_error)}</div>`;
            } else {
              quotaSectionHtml = cooldownBanner + `<div class="account-quota-unknown">${t('quotas.quotaUnknownCpa')}</div>`;
            }

            const todayCount = Number.isFinite(acc.today_requests) ? acc.today_requests : 0;
            const todayTokens = Number.isFinite(acc.today_tokens) ? acc.today_tokens : 0;
            const fiveHTokens = Number.isFinite(acc.five_hour_tokens) ? acc.five_hour_tokens : 0;
            const fiveHRequests = Number.isFinite(acc.five_hour_requests) ? acc.five_hour_requests : 0;

            let cardLocalUsageHtml = '';
            if (acc.provider !== 'opencode') {
              cardLocalUsageHtml = `
                <div class="account-card-local-usage" style="margin-top:12px; padding:10px 12px; background:var(--bg-subtle, #f1f5f9); border:1px solid var(--card-border-subtle, rgba(186, 215, 242, 0.45)); border-radius:8px;">
                  <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px;">
                    <span style="font-size:11px; font-weight:600; color:var(--text-muted); display:inline-flex; align-items:center; gap:4px;">
                      <span>📊</span> ${t('quotas.localAuthenticUsageTitle')}
                    </span>
                    <span style="font-size:10px; color:var(--text-dim);" title="${t('quotas.localGroundTruthTip')}">
                      ${t('quotas.localGroundTruthTip')}
                    </span>
                  </div>
                  <div style="display:grid; grid-template-columns:1fr 1fr; gap:8px;">
                    <div style="background:var(--card-bg, #fff); padding:6px 10px; border-radius:6px; border:1px solid var(--card-border-subtle, rgba(186, 215, 242, 0.3));">
                      <div style="font-size:10px; color:var(--text-dim); margin-bottom:2px;">${t('quotas.localAuthenticToday')}</div>
                      <div style="display:flex; align-items:baseline; gap:4px;">
                        <span style="font-family:var(--mono); font-size:13.5px; font-weight:700; color:#0284c7;">${formatTokens(todayTokens)}</span>
                        <span style="font-size:10px; color:var(--text-dim); font-family:var(--mono);">(${todayCount}${t('quotas.timesUnit')})</span>
                      </div>
                    </div>
                    <div style="background:var(--card-bg, #fff); padding:6px 10px; border-radius:6px; border:1px solid var(--card-border-subtle, rgba(186, 215, 242, 0.3));">
                      <div style="font-size:10px; color:var(--text-dim); margin-bottom:2px;">${t('quotas.localAuthentic5h')}</div>
                      <div style="display:flex; align-items:baseline; gap:4px;">
                        <span style="font-family:var(--mono); font-size:13.5px; font-weight:700; color:#6366f1;">${formatTokens(fiveHTokens)}</span>
                        <span style="font-size:10px; color:var(--text-dim); font-family:var(--mono);">(${fiveHRequests}${t('quotas.timesUnit')})</span>
                      </div>
                    </div>
                  </div>
                </div>
              `;
            }

            const accountCardTitle = (acc.display_name || acc.email || acc.filename || '').replace(/\.json$/i, '');
            const cardFullTip = acc.email && acc.email !== accountCardTitle ? `${accountCardTitle} (${acc.email})` : accountCardTitle;

            return `
              <div class="account-card ${isDisabled ? 'is-disabled' : ''}" style="${hasValidation ? 'border-color:rgba(239,68,68,0.5); box-shadow:0 0 12px rgba(239,68,68,0.15);' : ''}">
                <div class="account-card-header">
                  <div class="account-card-title-wrap">
                    <div class="provider-icon-badge ${brand.iconClass}" title="${escapeHtml(brand.name)}">${brand.iconSvg}</div>
                    <div style="display:flex; flex-direction:column; min-width:0; gap:3px;">
                      <div style="display:flex; align-items:center; gap:6px; flex-wrap:wrap;">
                        <span class="provider-pill ${brand.pillClass}">${escapeHtml(brand.name)}</span>
                        <span class="priority-badge ${prioCls}" title="${t('quotas.priority.valueHint')}">P${prio}</span>
                        ${isDisabled ? `<span class="chip chip-red" style="font-size:10.5px; padding:1px 6px; line-height:1.2; white-space:nowrap; flex-shrink:0;">${t('quotas.disabledBadge')}</span>` : ''}
                        ${isBlocked && !isDisabled ? `<span class="chip ${health.cooldown && health.cooldown.scope === 'models' ? 'chip-orange' : 'chip-red'}" style="font-size:10.5px; padding:1px 6px; line-height:1.2; ${health.cooldown && health.cooldown.scope === 'models' ? 'background:rgba(245,158,11,0.15); color:#d97706; border:1px solid rgba(245,158,11,0.3);' : ''}">${escapeHtml(health.label)}</span>` : ''}
                        ${hasValidation ? `<span class="chip chip-red" style="font-size:10.5px; padding:1px 6px; line-height:1.2; white-space:nowrap; flex-shrink:0; background:rgba(239,68,68,0.15); border:1px solid rgba(239,68,68,0.3); animation:pulse 2s infinite;">${t('quotas.validation.needGoogleBadge')}</span>` : ''}
                      </div>
                      <div class="account-card-email" title="${escapeHtml(cardFullTip)}">${escapeHtml(accountCardTitle)}</div>
                    </div>
                  </div>
                  <div style="display:flex; align-items:center; gap:6px;">
                    ${hasValidation ? `
                      <a href="${safeValUrl}" target="_blank" rel="noopener noreferrer" class="priority-btn" style="color:#ef4444; border-color:rgba(239,68,68,0.4); background:rgba(239,68,68,0.08); text-decoration:none; display:inline-flex; align-items:center; gap:4px;" title="${t('quotas.validation.openLinkHint')}">
                        ${t('quotas.validation.goVerifyBtn')}
                      </a>
                      <button class="priority-btn" style="color:#10b981; border-color:rgba(16,185,129,0.3); background:rgba(16,185,129,0.06);" onclick="resolveValidation(${valInfo.id})" title="${t('quotas.validation.clearedHint')}">
                        ${t('quotas.validation.verifiedBadge')}
                      </button>
                    ` : ''}
                    ${isDisabled ? '' : `<button class="priority-btn" style="color:#2563eb; border-color:rgba(37,99,235,0.35); background:rgba(37,99,235,0.06);" data-filename="${safeFilename}" data-provider="${escapeHtml(acc.provider || '')}" data-authindex="${escapeHtml(acc.auth_index || '')}" onclick="triggerActivateQuotaWindow(this.dataset.filename, this.dataset.provider, this.dataset.authindex)" title="${t('quotas.requeryQuotaTitle')}">
                      ${t('quotas.refreshQuotaBtn')}
                    </button>`}
                    <button class="priority-btn" data-filename="${safeFilename}" onclick="openPriorityModal(this.dataset.filename)" title="${t('quotas.priority.adjustAccountTitle')}">
                      <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" style="width:12px;height:12px;"><path stroke-linecap="round" stroke-linejoin="round" d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4"/></svg>
                      ${t('quotas.priority.adjustBtn')}
                    </button>
                    ${isCredentialBlocked ? '' : `<span class="account-card-plan ${planClass}">${escapeHtml(acc.plan_name || acc.plan_type)}</span>`}
                  </div>
                </div>
                <div class="account-card-subinfo">
                  <span title="${safeFilename}">${t('quotas.credentialLabel', { value: acc.auth_index ? '<b class="num">' + escapeHtml(String(acc.auth_index).slice(0, 8)) + '</b>' : '—' })}</span>
                  <span>${t('quotas.statusLabel')} <b style="color:${stColor};">${isBlocked ? escapeHtml(health.label) : (isDisabled ? t('quotas.disabledBadge') : escapeHtml(acc.status))}</b></span>
                </div>
                ${hasValidation ? `
                  <div style="background:rgba(239,68,68,0.06); border:1px solid rgba(239,68,68,0.2); border-radius:6px; padding:8px 10px; margin:4px 0 10px 0; font-size:11.5px; display:flex; justify-content:space-between; align-items:center;">
                    <div style="overflow:hidden; text-overflow:ellipsis; white-space:nowrap; max-width:70%;">
                      <span style="color:var(--danger); font-weight:600;">${t('quotas.validation.requirementsLabel')}</span>
                      <span style="color:var(--text-main); margin-left:4px;">${escapeHtml(valInfo.error_message || 'Verify your account')}</span>
                    </div>
                    <a href="${safeValUrl}" target="_blank" rel="noopener noreferrer" style="color:var(--accent); text-decoration:underline; font-weight:600;">${t('quotas.validation.openPageBtn')}</a>
                  </div>
                ` : ''}
                ${quotaSectionHtml}
                ${cardLocalUsageHtml}
              </div>
            `;
          }).join('');
        }
      }
    }

    // 0.6 获取上游凭证用量与成功率统计 (对齐 Keeper 统计视图，支持 5h / 24h / 7d / all)
