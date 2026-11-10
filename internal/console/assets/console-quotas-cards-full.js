/**
 * Gate Admin Console - 上游凭证完整卡片模式
 * 从 console-quotas-render.js 拆出：分组明细渲染与各上游完整卡片区块。
 */

    // 完整卡片的模型组区块：组标题 + 每组配额桶指示条

    function renderUpstreamGroupsHtml(acc, safeFilename) {
      const sortedGroups = [...(acc.groups || [])].filter(grp => Array.isArray(grp.buckets) && grp.buckets.some(bkt => Number.isFinite(bkt.remaining_percent)));
      sortedGroups.sort((a, b) => {
        const getRank = (grp) => {
          const s = String(grp.name || '').toLowerCase();
          if (s.includes('argon') || s.includes('gemini 4') || s.includes('gemini-4') || s.includes('gemini4')) return 2;
          if (s.includes('gemini')) return 1;
          if (s.includes('claude') || s.includes('gpt') || s.includes('3p')) return 3;
          return 4;
        };
        return getRank(a) - getRank(b);
      });

      const groupsHtml = sortedGroups.map(grp => {
        const gnameLower = String(grp.name || '').toLowerCase();
        const isGemini4 = gnameLower.includes('argon') || gnameLower.includes('gemini 4') || gnameLower.includes('gemini-4') || gnameLower.includes('gemini4');
        const isGemini = !isGemini4 && gnameLower.includes('gemini');
        const isClaude = gnameLower.includes('claude') || gnameLower.includes('gpt') || gnameLower.includes('3p');
        const isCodex = gnameLower.includes('codex') || acc.provider === 'codex';
        const isQoder = gnameLower.includes('qoder') || (acc.provider && acc.provider.toLowerCase().startsWith('qoder'));
        const isZCode = gnameLower.includes('zcode') || (acc.provider && acc.provider.toLowerCase().startsWith('zcode'));
        const isWorkbuddy = gnameLower.includes('workbuddy') || (acc.provider && acc.provider.toLowerCase().startsWith('workbuddy'));

        let poolBadge = '';
        let displayTitle = escapeHtml(grp.name);

        if (isGemini4) {
          poolBadge = `<span class="quota-pool-badge pool-gemini4">Gemini 4</span>`;
          displayTitle = t('quotas.group.gemini4Models');
        } else if (isGemini) {
          poolBadge = `<span class="quota-pool-badge pool-gemini">Gemini</span>`;
          displayTitle = t('quotas.group.geminiModels');
        } else if (isClaude) {
          poolBadge = `<span class="quota-pool-badge pool-claude">Claude / GPT</span>`;
          displayTitle = t('quotas.group.claudeModels');
        } else if (isCodex) {
          poolBadge = `<span class="quota-pool-badge pool-codex">Codex</span>`;
        } else if (isQoder) {
          poolBadge = `<span class="quota-pool-badge pool-qoder">Qoder</span>`;
        } else if (isZCode) {
          poolBadge = `<span class="quota-pool-badge pool-zcode">ZCode</span>`;
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
            </div>
            ${bucketsHtml}
          </div>
        `;
      }).join('');
      return groupsHtml;
    }

    // opencode 完整卡片：渠道接入配置与状态
    function renderUpstreamFullOpenCodeSection(acc) {
      return `
        <div class="model-group-section" style="margin-top:10px;">
          <div class="model-group-header">
            <div class="model-group-title-wrap">
              <span class="quota-pool-badge pool-opencode">OpenCode</span>
              <span class="model-group-title">API 凭据直连渠道</span>
            </div>
          </div>
          <div style="background:var(--bg-subtle, rgba(234,243,255,0.5)); border:1px solid var(--card-border-subtle, rgba(212,228,245,0.6)); border-radius:8px; padding:12px 16px; margin-top:8px; display:flex; justify-content:space-between; align-items:center;">
            <div>
              <div style="font-size:11px; color:var(--text-muted); margin-bottom:2px;">接入协议</div>
              <div style="font-weight:650; color:var(--text-main); font-size:13px;">OpenCode Native Client</div>
            </div>
            <div style="text-align:right;">
              <div style="font-size:11px; color:var(--text-muted); margin-bottom:2px;">凭据通道状态</div>
              <div style="font-weight:650; color:var(--success); font-size:13px;">健康就绪</div>
            </div>
          </div>
          ${acc.raw_error ? `<div class="account-health-warning" style="color:var(--danger); margin-top:8px; font-size:11px;">${escapeHtml(acc.raw_error)}</div>` : ''}
        </div>
      `;
    }

    // openrouter 完整卡片：渠道接入配置与状态
    function renderUpstreamFullOpenRouterSection(acc) {
      return `
        <div class="model-group-section" style="margin-top:10px;">
          <div class="model-group-header">
            <div class="model-group-title-wrap">
              <span class="quota-pool-badge pool-openrouter">OpenRouter</span>
              <span class="model-group-title">聚合模型路由渠道</span>
            </div>
          </div>
          <div style="background:var(--bg-subtle, rgba(234,243,255,0.5)); border:1px solid var(--card-border-subtle, rgba(212,228,245,0.6)); border-radius:8px; padding:12px 16px; margin-top:8px; display:flex; justify-content:space-between; align-items:center;">
            <div>
              <div style="font-size:11px; color:var(--text-muted); margin-bottom:2px;">接入协议</div>
              <div style="font-weight:650; color:var(--text-main); font-size:13px;">OpenRouter Multi-Model Gateway</div>
            </div>
            <div style="text-align:right;">
              <div style="font-size:11px; color:var(--text-muted); margin-bottom:2px;">凭据通道状态</div>
              <div style="font-weight:650; color:var(--success); font-size:13px;">健康就绪</div>
            </div>
          </div>
          ${acc.raw_error ? `<div class="account-health-warning" style="color:var(--danger); margin-top:8px; font-size:11px;">${escapeHtml(acc.raw_error)}</div>` : ''}
        </div>
      `;
    }

    // WorkBuddy 完整卡片：积分余量、消耗与资源包到期明细
    function renderUpstreamFullWorkBuddySection(acc) {
      let quotaSectionHtml = '';
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
                <span>${expInfo.isAllLongTerm ? t('quotas.points.quotaLabel') : (expInfo.hasMultiplePackages ? `${t('quotas.workbuddy.pointsBatchExpiry')} <span style="font-size:10px; opacity:0.8;">${t('quotas.workbuddy.packageCountSuffix', { packageCount: expInfo.packageCount })}</span>` : t('quotas.workbuddy.pointsExpiryAt'))}</span>
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
      return quotaSectionHtml;
    }

    // 完整卡片配额区：凭证阻断 / 提供商专属 / 模型组 / 上游错误兜底
    function renderUpstreamQuotaSection(acc, groupsHtml, health, isCredentialBlocked, cooldownBanner) {
      let quotaSectionHtml = '';
      if (isCredentialBlocked && !groupsHtml) {
        quotaSectionHtml = `<div class="account-health-warning">${escapeHtml(health.detail || health.label || t('quotas.cpaCredentialUnavailable'))} ${t('quotas.quotaHiddenSuffix')}</div>`;
      } else if (acc.provider === 'opencode') {
        quotaSectionHtml = renderUpstreamFullOpenCodeSection(acc);
      } else if (acc.provider === 'openrouter') {
        quotaSectionHtml = renderUpstreamFullOpenRouterSection(acc);
      } else if (acc.provider === 'workbuddy' || acc.provider === 'workbuddy-cn') {
        quotaSectionHtml = renderUpstreamFullWorkBuddySection(acc);
      } else if (groupsHtml) {
        quotaSectionHtml = cooldownBanner + groupsHtml;
      } else if (acc.raw_error) {
        quotaSectionHtml = `<div class="account-health-warning" style="color:var(--danger);">${escapeHtml(acc.raw_error)}</div>`;
      } else {
        quotaSectionHtml = cooldownBanner + `<div class="account-quota-unknown">${t('quotas.quotaUnknownCpa')}</div>`;
      }
      return quotaSectionHtml;
    }

    // 完整卡片单卡：头部操作区 + 配额区
    function renderUpstreamFullCard(acc) {
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
      const stRaw = String(acc.status || '').toLowerCase();
      const stColor = (isCredentialBlocked || isDisabled || (!isPartialCooldown && (stRaw.includes('error') || stRaw.includes('invalid')))) ? 'var(--danger)' : (isPartialCooldown ? '#d97706' : 'var(--success)');
      const prio = normalizeAuthPriority(acc.priority);
      const prioCls = prio >= 20 ? 'p-high' : (prio >= 10 ? 'p-normal' : 'p-low');

      const groupsHtml = renderUpstreamGroupsHtml(acc, safeFilename);

      const valInfo = acc.validation;
      const hasValidation = Boolean(valInfo && valInfo.status === 'pending');
      const safeValUrl = valInfo && valInfo.validation_url ? escapeHtml(valInfo.validation_url) : '';

      let cooldownBanner = '';
      if (health.cooldown && health.cooldown.scope === 'models') {
        cooldownBanner = `<div class="account-health-warning" style="margin-bottom:8px; font-size:11.5px; background:rgba(245,158,11,0.08); border:1px solid rgba(245,158,11,0.3); color:#d97706;">${escapeHtml(health.detail || health.label)}</div>`;
      }
      const quotaSectionHtml = renderUpstreamQuotaSection(acc, groupsHtml, health, isCredentialBlocked, cooldownBanner);

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
                  ${!isDisabled && (isBlocked || isPartialCooldown) ? `<span class="chip ${isPartialCooldown ? 'chip-orange' : 'chip-red'}" style="font-size:10.5px; padding:1px 6px; line-height:1.2; ${isPartialCooldown ? 'background:rgba(245,158,11,0.15); color:#d97706; border:1px solid rgba(245,158,11,0.3);' : ''}">${escapeHtml(health.label || (isPartialCooldown ? '部分模型冷却' : ''))}</span>` : ''}
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
              ${!isDisabled && acc.provider && acc.provider.toLowerCase().startsWith('qoder') ? `<button class="priority-btn" style="color:#6366f1; border-color:rgba(99,102,241,0.35); background:rgba(99,102,241,0.06);" data-filename="${safeFilename}" onclick="triggerQoderCheckin(this.dataset.filename)" title="执行 Qoder 每日签到获取积分">
                ${t('quotas.qoderCheckinAll')}
              </button>` : ''}
              <button class="priority-btn" data-filename="${safeFilename}" onclick="openPriorityModal(this.dataset.filename)" title="${t('quotas.priority.adjustAccountTitle')}">
                <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" style="width:12px;height:12px;"><path stroke-linecap="round" stroke-linejoin="round" d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4"/></svg>
                ${t('quotas.priority.adjustBtn')}
              </button>
              ${isCredentialBlocked ? '' : `<span class="account-card-plan ${planClass}">${escapeHtml(acc.plan_name || acc.plan_type)}</span>`}
            </div>
          </div>
          <div class="account-card-subinfo">
            <span title="${safeFilename}">${t('quotas.credentialLabel', { value: acc.auth_index ? '<b class="num">' + escapeHtml(String(acc.auth_index).slice(0, 8)) + '</b>' : '—' })}</span>
            <span>${t('quotas.statusLabel')} <b style="color:${stColor};">${isBlocked ? escapeHtml(health.label) : (isPartialCooldown ? escapeHtml(health.label || '部分模型冷却') : (isDisabled ? t('quotas.disabledBadge') : escapeHtml(acc.status)))}</b></span>
          </div>
          <div class="account-card-subinfo" style="margin-top:2px; font-size:11px; color:var(--text-muted);">
            <span>${t('quotas.telemetryCard', {
              reqs: `<b style="color:${(acc.today_requests || 0) > 0 ? 'var(--text-main)' : 'var(--text-dim)'};">${acc.today_requests || 0}</b>`,
              tokens: `<b style="color:${(acc.today_tokens || 0) > 0 ? '#0284c7' : 'var(--text-dim)'};">${formatTokens(acc.today_tokens || 0)}</b>`
            })}${(acc.five_hour_requests || 0) > 0 ? t('quotas.telemetryCard5hSuffix', { reqs: acc.five_hour_requests }) : ''}</span>
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
        </div>
      `;
    }

    // 完整卡片视图容器：切换显隐并渲染全部卡片
    function renderUpstreamFullCards(cardsContainer, compactContainer, filtered) {
      if (compactContainer) compactContainer.style.display = 'none';
      if (cardsContainer) {
        cardsContainer.style.display = 'grid';
        cardsContainer.innerHTML = filtered.map(acc => renderUpstreamFullCard(acc)).join('');
      }
    }
