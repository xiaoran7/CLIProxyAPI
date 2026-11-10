/**
 * Gate Admin Console - 上游额度计 · 单窗口上游
 * 从 console-quotas-render.js 拆出：OpenCode / OpenRouter / WorkBuddy / Qoder / ZCode 紧凑额度计。
 */

    function renderCompactOpenCodeMeter(acc) {
      return `
        <div class="compact-meter-wrap" style="display:flex; flex-direction:column; justify-content:center; gap:3px;">
          <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
            <span style="display:inline-flex; align-items:center; gap:5px;">
              <span class="quota-pool-badge pool-opencode">OpenCode</span>
              <span style="color:var(--text-muted); font-size:10.5px;">API 凭据直连</span>
            </span>
            <span style="font-family:var(--mono); font-size:11px; font-weight:600; color:var(--success);">
              正常就绪
            </span>
          </div>
          <div class="quota-meter-bar">
            <div class="quota-meter-fill meter-high" style="width: 100%;"></div>
          </div>
        </div>
        <div class="compact-meter-wrap" style="display:flex; flex-direction:column; justify-content:center; gap:3px;">
          <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
            <span style="display:inline-flex; align-items:center; gap:5px;">
              <span class="quota-pool-badge pool-opencode">OpenCode</span>
              <span style="color:var(--text-muted); font-size:10.5px;">通道健康度</span>
            </span>
            <span style="font-family:var(--mono); font-size:11px; font-weight:600; color:var(--success);">
              实时连通
            </span>
          </div>
          <div class="quota-meter-bar">
            <div class="quota-meter-fill meter-high" style="width: 100%;"></div>
          </div>
        </div>
      `;
    }

    // openrouter 紧凑视图：渠道就绪状态
    function renderCompactOpenRouterMeter(acc) {
      return `
        <div class="compact-meter-wrap" style="display:flex; flex-direction:column; justify-content:center; gap:3px;">
          <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
            <span style="display:inline-flex; align-items:center; gap:5px;">
              <span class="quota-pool-badge pool-openrouter">OpenRouter</span>
              <span style="color:var(--text-muted); font-size:10.5px;">聚合路由直连</span>
            </span>
            <span style="font-family:var(--mono); font-size:11px; font-weight:600; color:var(--success);">
              正常就绪
            </span>
          </div>
          <div class="quota-meter-bar">
            <div class="quota-meter-fill meter-high" style="width: 100%;"></div>
          </div>
        </div>
        <div class="compact-meter-wrap" style="display:flex; flex-direction:column; justify-content:center; gap:3px;">
          <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
            <span style="display:inline-flex; align-items:center; gap:5px;">
              <span class="quota-pool-badge pool-openrouter">OpenRouter</span>
              <span style="color:var(--text-muted); font-size:10.5px;">路由健康度</span>
            </span>
            <span style="font-family:var(--mono); font-size:11px; font-weight:600; color:var(--success);">
              实时连通
            </span>
          </div>
          <div class="quota-meter-bar">
            <div class="quota-meter-fill meter-high" style="width: 100%;"></div>
          </div>
        </div>
      `;
    }

    // WorkBuddy 紧凑视图：积分余量与到期徽标
    function renderCompactWorkBuddyMeter(acc) {
      const remainCredits = Number(acc.remaining_credits) || 0;
      const totalCredits = Number(acc.total_credits) || 0;
      const pct = totalCredits > 0 ? Math.max(0, Math.min(100, Math.round((remainCredits / totalCredits) * 1000) / 10)) : (remainCredits > 0 ? 100 : 0);
      let cls = 'meter-high'; let col = '#10b981';
      if (pct <= 20) { cls = 'meter-low'; col = '#ef4444'; }
      else if (pct <= 50) { cls = 'meter-medium'; col = '#f59e0b'; }

      const expInfo = getWorkBuddyExpirationInfo(acc);
      const title = expInfo.tooltip || t('quotas.workbuddy.officialPointsTooltip', { remainCredits: remainCredits, totalCredits: totalCredits });
      const wbResetBadge = expInfo.hasExpiration ? buildCompactResetBadge({ reset_time: expInfo.targetTs, reset_countdown: expInfo.countdown }) : '';
      const expLabel = expInfo.hasExpiration ? (expInfo.countdown ? `${expInfo.shortText} (${expInfo.countdown})` : expInfo.shortText) : (expInfo.shortText || '官方套餐');
      const expCol = expInfo.color || 'var(--text-main)';

      return `
        <div class="compact-meter-wrap" title="${escapeHtml(title)}">
          <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
            <span style="display:inline-flex; align-items:center; gap:4px;">
              <span class="quota-pool-badge pool-workbuddy">WorkBuddy</span>
              <span style="color:var(--text-muted); font-size:10px;">${t('quotas.workbuddy.packPoints')}</span>
            </span>
            <span style="display:inline-flex; align-items:center; gap:6px;">
              <span style="font-family:var(--mono); font-weight:700; color:${col};">
                ${t('quotas.points.remainingPoints', { value: remainCredits.toLocaleString(undefined, {minimumFractionDigits: 0, maximumFractionDigits: 2}) })}
              </span>
            </span>
          </div>
          <div class="quota-meter-bar">
            <div class="quota-meter-fill ${cls}" style="width: ${pct}%"></div>
          </div>
        </div>

        <div class="compact-meter-wrap" title="${escapeHtml(expInfo.tooltip || expInfo.fullText || '有效期与周期')}">
          <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
            <span style="display:inline-flex; align-items:center; gap:4px;">
              <span class="quota-pool-badge pool-workbuddy">WorkBuddy</span>
              <span style="color:var(--text-muted); font-size:10px;">有效期</span>
            </span>
            <span style="display:inline-flex; align-items:center; gap:6px;">
              ${wbResetBadge}
              <span style="font-family:var(--mono); font-weight:600; color:${expCol}; font-size:11px;">
                ${escapeHtml(expLabel)}
              </span>
            </span>
          </div>
          <div class="quota-meter-bar">
            <div class="quota-meter-fill meter-high" style="width: 100%;"></div>
          </div>
        </div>
      `;
    }

    // Qoder 紧凑视图：资源点数余量
    function renderCompactQoderMeter(acc, fallbackBucket) {
      const remainCredits = Number(acc.remaining_credits) || 0;
      const totalCredits = Number(acc.total_credits) || 0;
      const pct = totalCredits > 0 ? Math.max(0, Math.min(100, Math.round((remainCredits / totalCredits) * 1000) / 10)) : (remainCredits > 0 ? 100 : (fallbackBucket && Number.isFinite(fallbackBucket.remaining_percent) ? fallbackBucket.remaining_percent : 0));
      let cls = 'meter-high'; let col = '#10b981';
      if (pct <= 20) { cls = 'meter-low'; col = '#ef4444'; }
      else if (pct <= 50) { cls = 'meter-medium'; col = '#f59e0b'; }

      const pointsLabel = totalCredits > 0 ? `${remainCredits.toLocaleString(undefined, {minimumFractionDigits: 0, maximumFractionDigits: 1})} / ${totalCredits.toLocaleString(undefined, {minimumFractionDigits: 0, maximumFractionDigits: 1})}` : `${remainCredits.toLocaleString(undefined, {minimumFractionDigits: 0, maximumFractionDigits: 1})} pts`;
      const qoderResetBadge = buildCompactResetBadge(fallbackBucket || acc);
      const resetText = (fallbackBucket && (fallbackBucket.reset_countdown || fallbackBucket.reset_time))
        ? (fallbackBucket.reset_countdown || formatResetAbs(fallbackBucket.reset_time))
        : '按月重置';

      return `
        <div class="compact-meter-wrap" title="Qoder 资源额度点数 (${escapeHtml(pointsLabel)})">
          <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
            <span style="display:inline-flex; align-items:center; gap:4px;">
              <span class="quota-pool-badge pool-qoder">Qoder</span>
              <span style="color:var(--text-muted); font-size:10px;">资源点数</span>
            </span>
            <span style="font-family:var(--mono); font-weight:700; color:${col};">
              ${escapeHtml(pointsLabel)}
            </span>
          </div>
          <div class="quota-meter-bar">
            <div class="quota-meter-fill ${cls}" style="width: ${pct}%"></div>
          </div>
        </div>

        <div class="compact-meter-wrap" title="Qoder 额度周期刷新">
          <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
            <span style="display:inline-flex; align-items:center; gap:4px;">
              <span class="quota-pool-badge pool-qoder">Qoder</span>
              <span style="color:var(--text-muted); font-size:10px;">重置周期</span>
            </span>
            <span style="display:inline-flex; align-items:center; gap:6px;">
              ${qoderResetBadge}
              <span style="font-family:var(--mono); font-weight:600; color:var(--text-main); font-size:11px;">
                ${escapeHtml(resetText)}
              </span>
            </span>
          </div>
          <div class="quota-meter-bar">
            <div class="quota-meter-fill meter-high" style="width: 100%;"></div>
          </div>
        </div>
      `;
    }

    // ZCode 紧凑视图：最多两条真实额度指示条。
    // 编码套餐与 Start Plan 赠送池各占一条；同一池内有多个模型窗口时按窗口分别呈现，
    // 只有当上游确实只报了一个窗口时才用「有效期」条补位。
    function renderCompactZCodeMeter(acc, fallbackBucket) {
      const codingBuckets = [];
      const giftBuckets = [];

      for (const grp of (acc.groups || [])) {
        const gname = ((grp.name || '') + ' ' + (grp.displayName || '')).toLowerCase();
        const isGiftGrp = gname.includes('start') || gname.includes('gift') || gname.includes('trial');
        for (const bkt of (grp.buckets || [])) {
          if (!Number.isFinite(bkt.remaining_percent)) continue;
          const bname = ((bkt.name || '') + ' ' + (bkt.displayName || '')).toLowerCase();
          const isGiftBkt = isGiftGrp || bkt.gift === true || bname.includes('trial') || bname.includes('start') || bname.includes('gift');
          if (isGiftBkt) giftBuckets.push(bkt);
          else codingBuckets.push(bkt);
        }
      }

      // 主池在前、赠送池在后；同池内多个模型窗口按剩余额度升序，最紧张的排在前面。
      const byRemaining = (a, b) => a.remaining_percent - b.remaining_percent;
      const pools = [...codingBuckets.sort(byRemaining), ...giftBuckets.sort(byRemaining)];
      if (pools.length === 0 && fallbackBucket && Number.isFinite(fallbackBucket.remaining_percent)) {
        pools.push(fallbackBucket);
      }

      const planName = acc.plan_name || acc.plan_type || t('quotas.zcode.codingPlan');
      const bucketLabel = (bkt) => bkt.window || formatBucketLabel(bkt.name, bkt.window) || t('quotas.zcode.codingPlan');
      const meterClass = (rem) => {
        if (rem <= 20) return { cls: 'meter-low', col: '#ef4444' };
        if (rem <= 50) return { cls: 'meter-medium', col: '#f59e0b' };
        return { cls: 'meter-high', col: '#10b981' };
      };

      const buildBar = (bkt) => {
        const rem = bkt.remaining_percent;
        const { cls, col } = meterClass(rem);
        const label = bucketLabel(bkt);
        const isGift = giftBuckets.includes(bkt);
        const title = `ZCode ${planName} · ${label}${isGift ? ` (${t('quotas.zcode.trialPool')})` : ''} (${rem}% 可用)`;
        return `
          <div class="compact-meter-wrap" title="${escapeHtml(title)}">
            <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
              <span style="display:inline-flex; align-items:center; gap:4px;">
                <span class="quota-pool-badge pool-zcode">ZCode</span>
                <span style="color:var(--text-muted); font-size:10px;">${escapeHtml(label)}</span>
              </span>
              <span style="display:inline-flex; align-items:center; gap:6px;">
                ${buildCompactResetBadge(bkt)}
                <span style="font-family:var(--mono); font-weight:700; color:${col};">
                  ${rem}%
                </span>
              </span>
            </div>
            <div class="quota-meter-bar">
              <div class="quota-meter-fill ${cls}" style="width: ${rem}%"></div>
            </div>
          </div>
        `;
      };

      const bars = pools.slice(0, 2).map(buildBar);
      if (bars.length < 2) {
        // 上游只报了一个窗口：第二条补有效期，避免紧凑行出现半格空位。
        const expText = acc.expires_at ? formatResetAbs(acc.expires_at) : '有效就绪';
        bars.push(`
          <div class="compact-meter-wrap" title="ZCode ${escapeHtml(planName)} 有效期">
            <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
              <span style="display:inline-flex; align-items:center; gap:4px;">
                <span class="quota-pool-badge pool-zcode">ZCode</span>
                <span style="color:var(--text-muted); font-size:10px;">${escapeHtml(t('quotas.zcode.planExpiry'))}</span>
              </span>
              <span style="font-family:var(--mono); font-size:11px; font-weight:600; color:var(--success);">
                ${escapeHtml(expText)}
              </span>
            </div>
            <div class="quota-meter-bar">
              <div class="quota-meter-fill meter-high" style="width: 100%;"></div>
            </div>
          </div>
        `);
      }

      return bars.join('');
    }

    // Antigravity 紧凑视图：Gemini 5h / 周额度双指示条
    function renderCompactAntigravityMeters(gemini5hBucket, geminiWeeklyBucket, fallbackBucket, health, cooldownCompactNotice) {
      let metersHtml = '';
      const hasGemini5h = gemini5hBucket && Number.isFinite(gemini5hBucket.remaining_percent);
      const hasGeminiWeekly = geminiWeeklyBucket && Number.isFinite(geminiWeeklyBucket.remaining_percent);

      const buildAntigravityMeter = (bkt, weeklyBkt, defaultLabel, isWeeklyMeter = false) => {
        if (!bkt || !Number.isFinite(bkt.remaining_percent)) return '';
        const isModelCooling = health.cooldown && health.cooldown.scope === 'models' && (
          (health.cooldown.model_keys || []).some(k => {
            const kl = String(k || '').toLowerCase();
            const isG4 = kl.includes('argon') || kl.includes('gemini-4') || kl.includes('gemini4') || kl.includes('gemini 4');
            return !isG4 && (/gemini|flash/i.test(kl) || (/pro/i.test(kl) && !/claude/i.test(kl)));
          })
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

        const resetBadge = buildCompactResetBadge(bkt);
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
            <div class="quota-meter-bar">
              <div class="quota-meter-fill ${cls}" style="width: ${rem}%"></div>
            </div>
          </div>
        `;
      };

      if (hasGemini5h) {
        // 简短展示专属：严格只展示 Gemini 的 5h 与周额度，标明池子标识
        const col2Html = hasGeminiWeekly
          ? buildAntigravityMeter(geminiWeeklyBucket, null, t('quotas.bucket.weeklyLimit'), true)
          : (claude5hBucket && Number.isFinite(claude5hBucket.remaining_percent)
              ? `
                <div class="compact-meter-wrap" title="Claude 备用池">
                  <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                    <span style="display:inline-flex; align-items:center; gap:4px;">
                      <span class="quota-pool-badge pool-claude">Claude</span>
                      <span style="color:var(--text-muted); font-size:10px;">备用池</span>
                    </span>
                    <span style="font-family:var(--mono); font-weight:700; color:#10b981;">
                      ${claude5hBucket.remaining_percent}%
                    </span>
                  </div>
                  <div class="quota-meter-bar">
                    <div class="quota-meter-fill meter-high" style="width: ${claude5hBucket.remaining_percent}%;"></div>
                  </div>
                </div>
              `
              : `
                <div class="compact-meter-wrap" title="Gemini 周额度：未单独设限">
                  <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                    <span style="display:inline-flex; align-items:center; gap:4px;">
                      <span class="quota-pool-badge pool-gemini">Gemini</span>
                      <span style="color:var(--text-muted); font-size:10px;">${t('quotas.bucket.weeklyLimit')}</span>
                    </span>
                    <span style="font-family:var(--mono); font-size:10.5px; color:var(--text-dim);">无独立周限</span>
                  </div>
                  <div class="quota-meter-bar">
                    <div class="quota-meter-fill meter-high" style="width: 100%;"></div>
                  </div>
                </div>
              `);
        metersHtml = `
          ${buildAntigravityMeter(gemini5hBucket, geminiWeeklyBucket, t('quotas.bucket.fiveHourLimit'), false)}
          ${col2Html}
        `;
      } else if (hasGeminiWeekly) {
        metersHtml = `
          ${buildAntigravityMeter(geminiWeeklyBucket, null, t('quotas.bucket.weeklyLimit'), true)}
          <div class="compact-meter-wrap" title="Gemini 滚动 5h 限额">
            <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
              <span style="display:inline-flex; align-items:center; gap:4px;">
                <span class="quota-pool-badge pool-gemini">Gemini</span>
                <span style="color:var(--text-muted); font-size:10px;">${t('quotas.bucket.fiveHourLimit')}</span>
              </span>
              <span style="font-family:var(--mono); font-size:10.5px; color:var(--text-dim);">随调用刷新</span>
            </div>
            <div class="quota-meter-bar">
              <div class="quota-meter-fill meter-high" style="width: 100%;"></div>
            </div>
          </div>
        `;
      } else if (fallbackBucket && Number.isFinite(fallbackBucket.remaining_percent)) {
        metersHtml = `
          ${buildAntigravityMeter(fallbackBucket, null, formatBucketLabel(fallbackBucket.name, fallbackBucket.window) || '5h', false)}
          <div class="compact-meter-wrap" title="上游状态">
            <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
              <span style="display:inline-flex; align-items:center; gap:4px;">
                <span class="quota-pool-badge pool-gemini">Gemini</span>
                <span style="color:var(--text-muted); font-size:10px;">服务状态</span>
              </span>
              <span style="font-family:var(--mono); font-size:11px; font-weight:600; color:var(--success);">正常就绪</span>
            </div>
            <div class="quota-meter-bar">
              <div class="quota-meter-fill meter-high" style="width: 100%;"></div>
            </div>
          </div>
        `;
      } else {
        metersHtml = cooldownCompactNotice + `<div class="account-quota-unknown">${t('quotas.quotaUnknownUpstream')}</div>`;
      }
      return metersHtml;
    }

    // Codex 紧凑视图：5h 与周额度双指示条
