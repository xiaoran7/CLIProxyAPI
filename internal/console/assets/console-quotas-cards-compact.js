/**
 * Gate Admin Console - 上游凭证紧凑列表模式
 * 从 console-quotas-render.js 拆出：紧凑列表行渲染与紧凑卡片装配。
 */

    // 紧凑视图配额区：按提供商分派到对应的指示条渲染器

    function renderUpstreamCompactMeters(acc, buckets, health, cooldownCompactNotice, isCredentialBlocked) {
      const { gemini5hBucket, geminiWeeklyBucket, claude5hBucket, claudeWeeklyBucket, codex5hBucket, codexWeeklyBucket, fallbackBucket } = buckets;
      let metersHtml = '';
      if (isCredentialBlocked && !gemini5hBucket && !claude5hBucket && !codex5hBucket) {
        metersHtml = `<div class="account-health-warning">${escapeHtml(health.detail || health.label || t('quotas.cpaCredentialUnavailable'))} ${t('quotas.quotaHiddenSuffix')}</div>`;
      } else if (acc.raw_error) {
        metersHtml = `<div class="account-health-warning" style="color:var(--danger); font-size:11px;" title="${escapeHtml(acc.raw_error)}">${escapeHtml(acc.raw_error)}</div>`;
      } else if ((acc.provider === 'antigravity' && !Number.isFinite(gemini5hBucket?.remaining_percent) && !Number.isFinite(claude5hBucket?.remaining_percent) && !Number.isFinite(fallbackBucket?.remaining_percent)) || (acc.provider === 'codex' && !Number.isFinite(codex5hBucket?.remaining_percent) && !Number.isFinite(fallbackBucket?.remaining_percent))) {
        metersHtml = cooldownCompactNotice + `<div class="account-quota-unknown">${t('quotas.quotaUnknownUpstream')}</div>`;
      } else if (acc.provider === 'opencode') {
        metersHtml = renderCompactOpenCodeMeter(acc);
      } else if (acc.provider === 'openrouter') {
        metersHtml = renderCompactOpenRouterMeter(acc);
      } else if (acc.provider === 'workbuddy' || acc.provider === 'workbuddy-cn') {
        metersHtml = renderCompactWorkBuddyMeter(acc);
      } else if (acc.provider && acc.provider.toLowerCase().startsWith('qoder')) {
        metersHtml = renderCompactQoderMeter(acc, fallbackBucket);
      } else if (acc.provider && acc.provider.toLowerCase().startsWith('zcode')) {
        metersHtml = renderCompactZCodeMeter(acc, fallbackBucket);
      } else if (!Number.isFinite(fallbackBucket?.remaining_percent)) {
        metersHtml = `<div class="account-quota-unknown">${t('quotas.quotaUnknownCpa')}</div>`;
      } else if (acc.provider === 'antigravity') {
        metersHtml = renderCompactAntigravityMeters(gemini5hBucket, geminiWeeklyBucket, fallbackBucket, health, cooldownCompactNotice);
      } else if (acc.provider === 'codex') {
        metersHtml = renderCompactCodexMeters(codex5hBucket, codexWeeklyBucket, fallbackBucket, cooldownCompactNotice);
      } else if (acc.provider === 'claude') {
        metersHtml = renderCompactClaudeMeters(claude5hBucket, claudeWeeklyBucket, fallbackBucket, cooldownCompactNotice);
      } else if (fallbackBucket && Number.isFinite(fallbackBucket.remaining_percent)) {
        metersHtml = renderCompactFallbackMeter(fallbackBucket);
      } else {
        metersHtml = cooldownCompactNotice + `<div class="account-quota-unknown">${t('quotas.quotaUnknownUpstream')}</div>`;
      }
      return metersHtml;
    }

    // 紧凑视图单行：账号标识 + 配额指示条 + 本地真实用量 + 操作按钮
    function renderUpstreamCompactRow(acc) {
      const safeFilename = escapeHtml(acc.filename || acc.id || '');
      const brand = getProviderBrand(acc.provider, acc.display_name || acc.name);
      const planType = (acc.plan_type || 'free').toLowerCase();
      let planClass = 'plan-free';
      if (planType.includes('pro') || planType.includes('ultra')) planClass = 'plan-pro';
      else if (planType.includes('plus')) planClass = 'plan-plus';

      const isDisabled = Boolean(acc.disabled || acc.status === 'disabled');
      const health = acc.health || {};
      const isBlocked = health.blocked === true;
      const isPartialCooldown = (health.state === 'partial_cooldown') || (health.cooldown && health.cooldown.scope === 'models');
      const isCredentialBlocked = isBlocked && (!health.cooldown || health.cooldown.scope === 'credential' || health.cooldown.scope === 'credential_and_models');
      const prio = normalizeAuthPriority(acc.priority);
      const prioCls = prio >= 20 ? 'p-high' : (prio >= 10 ? 'p-normal' : 'p-low');

      const buckets = extractUpstreamCompactBuckets(acc);

      let cooldownCompactNotice = '';
      if (isPartialCooldown) {
        cooldownCompactNotice = `<div style="font-size:10px; color:#d97706; margin-bottom:2px; font-family:var(--mono);">${escapeHtml(health.detail || health.label || '部分模型冷却')}</div>`;
      }

      const metersHtml = renderUpstreamCompactMeters(acc, buckets, health, cooldownCompactNotice, isCredentialBlocked);

      const valInfo = acc.validation;
      const hasValidation = Boolean(valInfo && valInfo.status === 'pending');
      const safeValUrl = valInfo && valInfo.validation_url ? escapeHtml(valInfo.validation_url) : '';
      const accountTitle = (acc.display_name || acc.email || acc.filename || '').replace(/\.json$/i, '');
      const fullTip = acc.email && acc.email !== accountTitle ? `${accountTitle} (${acc.email})` : accountTitle;

      return `
        <div class="compact-account-row ${isDisabled ? 'is-disabled' : ''}" style="${hasValidation ? 'border-color:rgba(239,68,68,0.5); background:rgba(239,68,68,0.03);' : ''}">
          <div class="compact-left">
            <div class="provider-icon-badge ${brand.iconClass}" title="${escapeHtml(brand.name)}">${brand.iconSvg}</div>
            <div style="display:flex; flex-direction:column; min-width:0; flex:1 1 auto; gap:3px;">
              <div style="display:flex; align-items:center; gap:6px; flex-wrap:nowrap; min-width:0;">
                <span class="provider-pill ${brand.pillClass}" style="font-size:10px; padding:1.5px 6px; flex-shrink:0;">${escapeHtml(brand.name)}</span>
                <span class="priority-badge ${prioCls}" style="flex-shrink:0;">P${prio}</span>
                <span style="font-weight:600; font-size:13px; color:var(--text-main); overflow:hidden; text-overflow:ellipsis; white-space:nowrap; min-width:0; flex:1 1 auto;" title="${escapeHtml(fullTip)}">
                  ${escapeHtml(accountTitle)}
                </span>
                ${isDisabled ? `<span class="chip chip-red" style="font-size:10px; padding:1.5px 6px; line-height:1.2; white-space:nowrap; flex-shrink:0;">${t('quotas.disabledBadge')}</span>` : ''}
                ${!isDisabled && (isBlocked || isPartialCooldown) ? `<span class="chip ${isPartialCooldown ? 'chip-orange' : 'chip-red'}" style="font-size:10px; padding:1.5px 6px; line-height:1.2; flex-shrink:0; ${isPartialCooldown ? 'background:rgba(245,158,11,0.15); color:#d97706; border:1px solid rgba(245,158,11,0.3);' : ''}">${escapeHtml(health.label || (isPartialCooldown ? '部分模型冷却' : ''))}</span>` : ''}
                ${hasValidation ? `<span class="chip chip-red" style="font-size:10px; padding:1.5px 6px; line-height:1.2; white-space:nowrap; flex-shrink:0; background:rgba(239,68,68,0.15); border:1px solid rgba(239,68,68,0.3); animation:pulse 2s infinite;">${t('quotas.validation.needGoogleBadge')}</span>` : ''}
              </div>
              <div style="font-size:11px; font-family:var(--mono); color:var(--text-dim); display:flex; align-items:center; gap:8px; flex-wrap:wrap;">
                <span>ID: ${acc.auth_index ? escapeHtml(String(acc.auth_index).slice(0, 12)) : '—'}</span>
                ${isCredentialBlocked ? '' : `<span class="account-card-plan ${planClass}" style="padding:1px 6px; font-size:10px;">${escapeHtml(acc.plan_name || acc.plan_type)}</span>`}
                <span class="compact-usage-telemetry" style="color:var(--text-muted); font-size:10.5px;">
                  ${t('quotas.telemetryCompact', {
                    reqs: `<b style="color:${(acc.today_requests || 0) > 0 ? 'var(--text-main)' : 'var(--text-dim)'};">${acc.today_requests || 0}</b>`,
                    tokens: `<b style="color:${(acc.today_tokens || 0) > 0 ? '#0284c7' : 'var(--text-dim)'};">${formatTokens(acc.today_tokens || 0)}</b>`
                  })}${(acc.five_hour_requests || 0) > 0 ? `<span style="color:var(--text-dim); font-size:9.5px; opacity:0.8;">${t('quotas.telemetry5hSuffix', { reqs: acc.five_hour_requests })}</span>` : ''}
                </span>
              </div>
            </div>
          </div>

          <div class="compact-mid">
            ${metersHtml}
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
            ${!isDisabled && acc.provider && acc.provider.toLowerCase().startsWith('qoder') ? `<button class="priority-btn" style="color:#6366f1; border-color:rgba(99,102,241,0.3); background:rgba(99,102,241,0.06); padding:3px 8px; font-size:11px;" data-filename="${safeFilename}" onclick="triggerQoderCheckin(this.dataset.filename)" title="执行 Qoder 每日签到获取积分">
              ${t('quotas.qoderCheckinAll')}
            </button>` : ''}
            <button class="priority-btn" style="padding:3px 8px; font-size:11px;" data-filename="${safeFilename}" onclick="openPriorityModal(this.dataset.filename)" title="${t('quotas.priority.adjustTitle')}">
              ${t('quotas.priority.adjustBtn')}
            </button>
          </div>
        </div>
      `;
    }

    // 紧凑视图容器：切换显隐并渲染全部行
    function renderUpstreamCompactCards(cardsContainer, compactContainer, filtered) {
      if (cardsContainer) cardsContainer.style.display = 'none';
      if (compactContainer) {
        compactContainer.style.display = 'flex';
        compactContainer.innerHTML = filtered.map(acc => renderUpstreamCompactRow(acc)).join('');
      }
    }
