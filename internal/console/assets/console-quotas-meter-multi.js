/**
 * Gate Admin Console - 上游额度计 · 多窗口上游
 * 从 console-quotas-render.js 拆出：Antigravity / Codex / Claude 多窗口额度计、回退计与紧凑计量装配。
 */

    function renderCompactCodexMeters(codex5hBucket, codexWeeklyBucket, fallbackBucket, cooldownCompactNotice) {
      let metersHtml = '';
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

          const isMonthly2 = (b2?.window === 'monthly') || String(b2?.name || '').toLowerCase().includes('month');
          let title2 = isMonthly2 ? 'Codex 月度限额' : t('quotas.bucket.codexWeekly');
          if (b2?.reset_time || b2?.reset_countdown) {
            title2 += ` (${b2.reset_countdown || formatResetAbs(b2.reset_time)})`;
          }

          const secLabel = isMonthly2 ? t('quotas.bucket.monthlyLimit') : t('quotas.bucket.weeklyLimit');
          const resetBadge2 = buildCompactResetBadge(b2);
          col2Html = `
            <div class="compact-meter-wrap" title="${escapeHtml(title2)}">
              <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                <span style="display:inline-flex; align-items:center; gap:4px;">
                  <span class="quota-pool-badge pool-codex">Codex</span>
                  <span style="color:var(--text-muted); font-size:10px;">${secLabel}</span>
                </span>
                <span style="display:inline-flex; align-items:center; gap:6px;">
                  ${resetBadge2}
                  <span style="font-family:var(--mono); font-weight:700; color:${col2};">${t('quotas.bucket.secondRemainingPercent', { rem2: rem2 })}</span>
                </span>
              </div>
              <div class="quota-meter-bar">
                <div class="quota-meter-fill ${cls2}" style="width: ${rem2}%"></div>
              </div>
            </div>
          `;
        } else {
          col2Html = `
            <div class="compact-meter-wrap" title="Codex 周额度：未单独设限">
              <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                <span style="display:inline-flex; align-items:center; gap:4px;">
                  <span class="quota-pool-badge pool-codex">Codex</span>
                  <span style="color:var(--text-muted); font-size:10px;">${t('quotas.bucket.weeklyLimit')}</span>
                </span>
                <span style="font-family:var(--mono); font-size:10.5px; color:var(--text-dim);">标准配额</span>
              </div>
              <div class="quota-meter-bar">
                <div class="quota-meter-fill meter-high" style="width: 100%;"></div>
              </div>
            </div>
          `;
        }

        const resetBadge1 = buildCompactResetBadge(b1);
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
            <div class="quota-meter-bar">
              <div class="quota-meter-fill ${cls1}" style="width: ${rem1}%"></div>
            </div>
          </div>
          ${col2Html}
        `;
      }
      return metersHtml;
    }

    // Claude 紧凑视图：5h 与 7 天额度双指示条
    function renderCompactClaudeMeters(claude5hBucket, claudeWeeklyBucket, fallbackBucket, cooldownCompactNotice) {
      let metersHtml = '';
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

          const resetBadge2 = buildCompactResetBadge(b2);
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
              <div class="quota-meter-bar">
                <div class="quota-meter-fill ${cls2}" style="width: ${rem2}%"></div>
              </div>
            </div>
          `;
        } else {
          col2Html = `
            <div class="compact-meter-wrap" title="Claude 周期额度">
              <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                <span style="display:inline-flex; align-items:center; gap:4px;">
                  <span class="quota-pool-badge pool-claude">Claude</span>
                  <span style="color:var(--text-muted); font-size:10px;">${t('quotas.bucket.weeklyLimit')}</span>
                </span>
                <span style="font-family:var(--mono); font-size:10.5px; color:var(--text-dim);">循环配额</span>
              </div>
              <div class="quota-meter-bar">
                <div class="quota-meter-fill meter-high" style="width: 100%;"></div>
              </div>
            </div>
          `;
        }

        const resetBadge1 = buildCompactResetBadge(b1);
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
            <div class="quota-meter-bar">
              <div class="quota-meter-fill ${cls1}" style="width: ${rem1}%"></div>
            </div>
          </div>
          ${col2Html}
        `;
      }
      return metersHtml;
    }

    // 通用紧凑视图：单一配额桶指示条
    function renderCompactFallbackMeter(fallbackBucket) {
      let metersHtml = '';
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

      const resetBadge1 = buildCompactResetBadge(b1);
      metersHtml = `
        <div class="compact-meter-wrap" title="${escapeHtml(title1)}">
          <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
            <span style="color:var(--text-muted); font-size:10.5px;">${escapeHtml(bname)}</span>
            <span style="display:inline-flex; align-items:center; gap:6px;">
              ${resetBadge1}
              <span style="font-family:var(--mono); font-weight:700; color:${col1};">${t('quotas.bucket.firstRemainingPercent', { rem1: rem1 })}</span>
            </span>
          </div>
          <div class="quota-meter-bar">
            <div class="quota-meter-fill ${cls1}" style="width: ${rem1}%"></div>
          </div>
        </div>
        <div class="compact-meter-wrap" title="服务通道就绪状态">
          <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
            <span style="color:var(--text-muted); font-size:10.5px;">通道可用性</span>
            <span style="font-family:var(--mono); font-size:11px; font-weight:600; color:var(--success);">正常就绪</span>
          </div>
          <div class="quota-meter-bar">
            <div class="quota-meter-fill meter-high" style="width: 100%;"></div>
          </div>
        </div>
      `;
      return metersHtml;
    }

    // 紧凑视图配额区：按提供商分派到对应的指示条渲染器
