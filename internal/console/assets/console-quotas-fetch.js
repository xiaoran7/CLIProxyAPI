/**
 * Gate Admin Console - Upstream quota fetch, refresh, filtering and probe triggering
 * 上游配额拉取、刷新、过滤与探针触发
 */

    let cachedUpstreamValidations = [];

    // 获取上游验证链接列表与渲染
    async function fetchUpstreamValidations(showTip = false) {
      const secret = getAdminSecret();
      if (!secret || isMemberAuth()) return;
      try {
        const res = await fetch('/console/api/upstream-validations', {
          headers: getAuthHeaders()
        });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const text = await res.text();
        let data = { validations: [] };
        try {
          data = text ? JSON.parse(text) : { validations: [] };
        } catch (_) {}
        cachedUpstreamValidations = (data && Array.isArray(data.validations)) ? data.validations : [];
        renderValidationsBanner();
        renderValidationsPage();
        if (showTip) showToast(t('quotas.validation.listUpdatedToast'), 'success');
      } catch (err) {
        console.warn('[Gate] fetchUpstreamValidations error:', err.message);
        if (showTip) showToast(`${t('quotas.validation.fetchListFailed')} ` + err.message, 'error');
      }
    }

    function renderValidationsBanner() {
      const banner = document.getElementById('upstream-validations-banner');
      const container = document.getElementById('upstream-validations-container');
      const opsBox = document.getElementById('ops-validations-box');
      const opsCount = document.getElementById('ops-validations-count');
      const navBadge = document.getElementById('nav-badge-validations');

      const pendings = cachedUpstreamValidations.filter(v => v.status === 'pending');

      if (navBadge) {
        navBadge.textContent = String(pendings.length);
        if (pendings.length > 0) {
          navBadge.style.background = '#ef4444';
          navBadge.style.color = '#ffffff';
          navBadge.style.fontWeight = '700';
        } else {
          navBadge.style.background = 'rgba(148, 163, 184, 0.2)';
          navBadge.style.color = 'var(--text-dim)';
          navBadge.style.fontWeight = '500';
        }
      }

      if (opsBox && opsCount) {
        if (pendings.length > 0) {
          opsBox.style.display = 'flex';
          opsCount.textContent = String(pendings.length);
        } else {
          opsBox.style.display = 'none';
        }
      }

      if (!banner || !container) return;

      if (pendings.length === 0) {
        banner.style.display = 'none';
        container.innerHTML = '';
        return;
      }

      banner.style.display = 'block';
      container.innerHTML = pendings.map(v => {
        const timeStr = typeof formatCstTime === 'function' ? formatCstTime(v.updated_at) : new Date(v.updated_at).toLocaleTimeString(LOCALE, { hour12: false, timeZone: 'Asia/Shanghai' });
        const nameDisp = escapeHtml(v.account_name || v.auth_index || t('quotas.unknownAccount'));
        const urlSafe = escapeHtml(v.validation_url);
        return `
          <div style="background:var(--bg-card); border:1px solid rgba(239,68,68,0.25); border-radius:6px; padding:10px 14px; display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:8px;">
            <div style="display:flex; flex-direction:column; gap:2px; min-width:0; max-width:70%;">
              <div style="display:flex; align-items:center; gap:8px;">
                <span class="chip chip-red" style="font-size:10.5px; font-weight:700;">${t('quotas.validation.pendingGoogle')}</span>
                <strong style="color:var(--text-main); font-size:13px;">${nameDisp}</strong>
                <span style="font-family:var(--mono); font-size:11px; color:var(--text-dim);">ID: ${escapeHtml(String(v.auth_index).slice(0, 8))} · ${timeStr}</span>
              </div>
              <div style="font-size:11px; color:var(--text-muted); overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">
                ${escapeHtml(v.error_message || 'Verify your account to continue.')}
              </div>
            </div>
            <div style="display:flex; align-items:center; gap:8px;">
              <a href="${urlSafe}" target="_blank" rel="noopener noreferrer" class="btn btn-sm" style="background:#ef4444; color:#fff; text-decoration:none; display:inline-flex; align-items:center; gap:4px; padding:4px 12px; font-weight:600;" title="${t('quotas.validation.openOfficialPage')}">
                ${t('quotas.validation.openLinkBtn')}
              </a>
              <button class="btn btn-secondary btn-sm" onclick="resolveValidation(${v.id})" title="${t('quotas.validation.resolveHint')}">
                ${t('quotas.validation.doneActionTitle')}
              </button>
            </div>
          </div>
        `;
      }).join('');
    }

    // 渲染 "Google 验证" 专属工作区页面
    function renderValidationsPage() {
      const pendings = cachedUpstreamValidations.filter(v => v.status === 'pending');
      const resolved = cachedUpstreamValidations.filter(v => v.status === 'resolved');
      const antigravityAccounts = (cachedUpstream.accounts || []).filter(a => a.provider === 'antigravity');

      // 1. 顶部指标
      const pendingCountEl = document.getElementById('val-pending-count');
      const totalAntigravityCountEl = document.getElementById('val-total-antigravity-count');
      const resolvedCountEl = document.getElementById('val-resolved-count');
      const pendingChipEl = document.getElementById('val-pending-chip');

      if (pendingCountEl) pendingCountEl.textContent = String(pendings.length);
      if (totalAntigravityCountEl) totalAntigravityCountEl.textContent = String(antigravityAccounts.length);
      if (resolvedCountEl) resolvedCountEl.textContent = String(resolved.length);
      if (pendingChipEl) {
        pendingChipEl.style.display = pendings.length > 0 ? 'inline-flex' : 'none';
        pendingChipEl.textContent = t('quotas.pendingValidationCountSuffix', { count: pendings.length });
      }

      // 2. 待验证卡片列表
      const pendingList = document.getElementById('validations-pending-list');
      if (pendingList) {
        if (pendings.length === 0) {
          pendingList.innerHTML = `
            <div class="val-empty-box">
              <svg style="width:36px;height:36px;color:#10b981;" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/>
              </svg>
              <div class="val-empty-title">${t('quotas.validation.allHealthy')}</div>
              <div class="val-empty-desc">
                ${t('quotas.validation.noAccountGotUpstream')} <code>403 VALIDATION_REQUIRED</code> ${t('quotas.validation.riskControlHint')}
              </div>
            </div>
          `;
        } else {
          pendingList.innerHTML = pendings.map(v => {
            const timeStr = typeof formatCstDateTime === 'function' ? formatCstDateTime(v.updated_at) : new Date(v.updated_at).toLocaleString(LOCALE, { hour12: false, timeZone: 'Asia/Shanghai' });
            const nameDisp = escapeHtml(v.account_name || v.auth_index || t('quotas.validation.googleAccount'));
            const targetEmail = v.account_name && v.account_name.includes('@') ? v.account_name.trim() : null;
            const standardUrl = safeGoogleLink(v.validation_url);
            const userSpecificUrl = targetEmail ? safeGoogleLink(v.validation_url, targetEmail) : '';
            const errSafe = escapeHtml(v.error_message || 'Verify your account to continue.');
            const isCompletedAuto = v.validation_url && !v.validation_url.includes('continue=') && standardUrl.includes('continue=');

            return `
              <div class="val-pending-card">
                <div class="val-pending-header">
                  <div class="val-pending-title">
                    <span class="chip chip-red" style="font-weight:700;">${t('quotas.validation.pendingGoogle')}</span>
                    <span class="val-pending-name">${nameDisp}</span>
                    <span style="font-family:var(--mono); font-size:11.5px; color:var(--text-dim);">Auth: ${escapeHtml(v.auth_index)}</span>
                  </div>
                  <div style="font-size:11.5px; color:var(--text-dim); font-family:var(--mono);">
                    ${t('quotas.validation.capturedAt', { timeStr: timeStr })}
                  </div>
                </div>
                <div class="val-pending-msg">
                  <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" style="width:13px;height:13px;color:var(--danger);vertical-align:-1px;display:inline-block;margin-right:4px;"><path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/></svg>${errSafe}
                </div>
                <div style="font-size:12px; color:var(--text-muted); word-break:break-all; background:rgba(0,0,0,0.02); padding:8px 10px; border-radius:6px; border:1px dashed var(--card-border);">
                  <strong>${t('quotas.validation.officialLinkLabel')}</strong>
                  ${standardUrl ? `<a href="${standardUrl}" target="_blank" rel="noopener noreferrer" style="color:var(--accent); text-decoration:underline;">${standardUrl}</a>` : `<span style="color:var(--danger);">${t('quotas.validation.invalidLink')}</span>`}
                  ${isCompletedAuto ? `<span class="chip chip-green" style="font-size:10px; margin-left:6px;">${t('quotas.validation.autoCallbackBadge')}</span>` : ''}
                </div>
                <div class="val-pending-actions" style="display:flex; flex-wrap:wrap; gap:8px; align-items:center; margin-top:10px;">
                  ${standardUrl ? `
                    <a href="${standardUrl}" target="_blank" rel="noopener noreferrer" class="val-btn-go">
                      <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" style="width:14px;height:14px;"><path stroke-linecap="round" stroke-linejoin="round" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"/></svg>
                      ${t('quotas.validation.officialDirectLink')}
                    </a>
                  ` : ''}
                  ${userSpecificUrl && userSpecificUrl !== standardUrl ? `
                    <a href="${userSpecificUrl}" target="_blank" rel="noopener noreferrer" class="btn btn-secondary btn-sm" title="${t('quotas.validation.accountParamHint')}">
                      ${t('quotas.validation.openForAccountBtn', { value: escapeHtml(targetEmail) })}) ↗
                    </a>
                  ` : ''}
                  ${standardUrl ? `
                    <button class="btn btn-secondary btn-sm" onclick="copyToClipboard('${standardUrl.replace(/'/g, "\\'")}', '验证链接已复制，可粘贴至无痕窗口')">
                      ${t('quotas.validation.copyLinkBtn')}
                    </button>
                  ` : ''}
                  <button class="btn btn-secondary btn-sm" onclick="resolveValidation(${v.id})" style="background:rgba(16,185,129,0.08); color:#10b981; border-color:rgba(16,185,129,0.3);">
                    <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" style="width:13px;height:13px;"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/></svg>
                    ${t('quotas.validation.resolveBtn')}
                  </button>
                  <button class="btn btn-secondary btn-sm" onclick="dismissValidation(${v.id})" style="color:var(--text-dim);" title="${t('quotas.validation.temporarilyDismissTitle')}">
                    ${t('quotas.validation.dismissTitle')}
                  </button>
                  <button class="btn btn-secondary btn-sm" onclick="deleteValidation(${v.id})" style="color:var(--danger);">
                    ${t('quotas.validation.deleteRecordBtn')}
                  </button>
                </div>
              </div>
            `;
          }).join('');
        }
      }

      // 3. 所有 Antigravity 账号巡检表
      const accountsTbody = document.getElementById('antigravity-accounts-tbody');
      if (accountsTbody) {
        if (antigravityAccounts.length === 0) {
          accountsTbody.innerHTML = `<tr><td colspan="6" style="text-align:center; color:var(--text-dim); padding:24px;">${t('quotas.validation.noLoadingAccounts')}</td></tr>`;
        } else {
          accountsTbody.innerHTML = antigravityAccounts.map(acc => {
            const hasPending = pendings.find(p => p.auth_index === acc.auth_index || (acc.email && p.account_name === acc.email));
            const statusBadge = acc.health?.blocked
              ? `<span class="chip chip-red">${escapeHtml(acc.health.label)}</span>`
              : `<span class="chip chip-green">${t('quotas.runningBadge')}</span>`;

            const healthBadge = hasPending
              ? `<span class="chip chip-red" style="font-weight:700; animation:pulse 1.8s infinite;">${t('quotas.validation.needManualBadge')}</span>`
              : (acc.health?.blocked ? `<span class="chip chip-red">${escapeHtml(acc.health.detail)}</span>` : `<span class="chip chip-green">${t('quotas.healthOkBadge')}</span>`);

            const updateTime = acc.updated_at ? (typeof formatCstTime === 'function' ? formatCstTime(acc.updated_at) : new Date(acc.updated_at).toLocaleTimeString(LOCALE, { hour12: false, timeZone: 'Asia/Shanghai' })) : '--';

            let actionHtml = '';
            if (hasPending) {
              actionHtml = `
                <a href="${escapeHtml(hasPending.validation_url)}" target="_blank" rel="noopener noreferrer" class="btn btn-sm" style="background:#ef4444; color:#fff; text-decoration:none;">${t('quotas.validation.goVerifyBtn')}</a>
                <button class="btn btn-secondary btn-sm" onclick="resolveValidation(${hasPending.id})">${t('quotas.validation.doneAction')}</button>
                <button class="btn btn-secondary btn-sm" onclick="dismissValidation(${hasPending.id})" style="color:var(--text-dim);" title="${t('quotas.validation.dismissTitle')}">${t('quotas.validation.dismissAction')}</button>
              `;
            } else {
              actionHtml = `
                <button class="btn btn-secondary btn-sm" onclick="quickFillManualValidation('${escapeHtml(acc.email || '')}', '${escapeHtml(acc.auth_index)}')">${t('quotas.validation.manualEntryBtn')}</button>
              `;
            }

            return `
              <tr>
                <td>
                  <div style="font-weight:600; color:var(--text-main); font-size:13px;">${escapeHtml((acc.display_name || acc.email || acc.filename || acc.id || '').replace(/\.json$/i, ''))}</div>
                  <div style="font-size:11px; color:var(--text-dim);">${escapeHtml(acc.plan_name || 'Google AI Pro')}</div>
                </td>
                <td style="font-family:var(--mono); font-size:12px; color:var(--text-muted);">${escapeHtml(acc.auth_index || '--')}</td>
                <td>${statusBadge}</td>
                <td>${healthBadge}</td>
                <td style="font-family:var(--mono); font-size:11.5px; color:var(--text-dim);">${updateTime}</td>
                <td style="text-align:right;">
                  <div style="display:inline-flex; align-items:center; gap:6px;">
                    ${actionHtml}
                  </div>
                </td>
              </tr>
            `;
          }).join('');
        }
      }

      // 4. 历史验证记录流水表
      const histTbody = document.getElementById('validations-history-tbody');
      if (histTbody) {
        if (cachedUpstreamValidations.length === 0) {
          histTbody.innerHTML = `<tr><td colspan="8" style="text-align:center; color:var(--text-dim); padding:24px;">${t('quotas.emptyCaptureHistory')}</td></tr>`;
        } else {
          histTbody.innerHTML = cachedUpstreamValidations.slice(0, 50).map(v => {
            const timeStr = typeof formatCstDateTime === 'function' ? formatCstDateTime(v.ts) : new Date(v.ts).toLocaleString(LOCALE, { hour12: false, timeZone: 'Asia/Shanghai' });
            const upTimeStr = typeof formatCstDateTime === 'function' ? formatCstDateTime(v.updated_at) : new Date(v.updated_at).toLocaleString(LOCALE, { hour12: false, timeZone: 'Asia/Shanghai' });
            const statusLabel = v.status === 'pending'
              ? `<span class="chip chip-red">${t('quotas.validation.pendingBadge')}</span>`
              : (v.status === 'resolved' ? `<span class="chip chip-green">${t('quotas.validation.verifiedBadge')}</span>` : `<span class="chip chip-gray">${t('quotas.validation.ignoredBadge')}</span>`);

            return `
              <tr>
                <td style="font-family:var(--mono); font-size:11.5px;">#${v.id}</td>
                <td style="font-family:var(--mono); font-size:11.5px; color:var(--text-muted);">${timeStr}</td>
                <td style="font-weight:600; font-size:12.5px;">${escapeHtml(v.account_name || '--')}</td>
                <td style="font-family:var(--mono); font-size:11.5px; color:var(--text-dim);">${escapeHtml(v.auth_index)}</td>
                <td style="font-size:11.5px; max-width:260px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${escapeHtml(v.error_message || '')}">
                  ${escapeHtml(v.error_message || '--')}
                </td>
                <td>${statusLabel}</td>
                <td style="font-family:var(--mono); font-size:11.5px; color:var(--text-dim);">${upTimeStr}</td>
                <td style="text-align:right; white-space:nowrap;">
                  ${safeGoogleLink(v.validation_url, v.account_name) ? `<a href="${safeGoogleLink(v.validation_url, v.account_name)}" target="_blank" rel="noopener noreferrer" class="btn btn-secondary btn-sm" style="text-decoration:none;">${t('quotas.validation.visitLinkBtn')}</a>` : ''}
                  <button class="btn btn-secondary btn-sm" onclick="deleteValidation(${v.id})" style="color:var(--danger); margin-left:4px;" title="${t('quotas.validation.purgeRecordTitle')}">${t('quotas.actionDelete')}</button>
                </td>
              </tr>
            `;
          }).join('');
        }
      }
    }

    function openManualValidationModal() {
      document.getElementById('manual-val-account').value = '';
      document.getElementById('manual-val-url').value = '';
      openModal('manual-val-modal');
    }

    function quickFillManualValidation(email, authIndex) {
      document.getElementById('manual-val-account').value = email || authIndex;
      document.getElementById('manual-val-url').value = '';
      openModal('manual-val-modal');
      setTimeout(() => document.getElementById('manual-val-url').focus(), 100);
    }

    async function submitManualValidation() {
      const account = document.getElementById('manual-val-account').value.trim();
      const url = document.getElementById('manual-val-url').value.trim();
      if (!url) {
        showToast(t('quotas.validation.urlRequired'), 'error');
        return;
      }
      try {
        const res = await fetch('/console/api/upstream-validations/manual', {
          method: 'POST',
          headers: {
            ...getAuthHeaders(),
            'Content-Type': 'application/json'
          },
          body: JSON.stringify({
            account_name: account,
            validation_url: url
          })
        });
        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          throw new Error(errData.error || `HTTP ${res.status}`);
        }
        const data = await res.json();
        const openUrl = data.validation_url || url;
        showToast(t('quotas.validation.linkSavedToast'), 'success');
        closeModal('manual-val-modal');
        window.open(openUrl, '_blank', 'noopener,noreferrer');
        await fetchUpstreamValidations();
        await fetchUpstreamQuotas(false);
      } catch (err) {
        showToast(`${t('quotas.validation.recordFailed')} ` + err.message, 'error');
      }
    }

    async function resolveValidation(id) {
      const secret = getAdminSecret();
      if (!secret) return;
      try {
        const res = await fetch(`/console/api/upstream-validations/${id}/resolve`, {
          method: 'POST',
          headers: getAuthHeaders()
        });
        if (res.ok) {
          showToast(t('quotas.validation.resolveToast'), 'success');
          await fetchUpstreamValidations();
          await fetchUpstreamQuotas(true);
        } else {
          showToast(t('quotas.validation.markFailed'), 'error');
        }
      } catch (err) {
        showToast(`${t('quotas.requestErrorLabel')} ` + err.message, 'error');
      }
    }

    async function dismissValidation(id) {
      const secret = getAdminSecret();
      if (!secret) return;
      try {
        const res = await fetch(`/console/api/upstream-validations/${id}/dismiss`, {
          method: 'POST',
          headers: getAuthHeaders()
        });
        if (res.ok) {
          showToast(t('quotas.validation.ignoredToast'), 'info');
          await fetchUpstreamValidations();
          await fetchUpstreamQuotas(false);
        } else {
          showToast(t('quotas.operationFailed'), 'error');
        }
      } catch (err) {
        showToast(`${t('quotas.requestErrorLabel')} ` + err.message, 'error');
      }
    }

    async function deleteValidation(id) {
      if (!confirm(t('quotas.validation.purgeConfirm'))) return;
      try {
        const res = await fetch(`/console/api/upstream-validations/${id}`, {
          method: 'DELETE',
          headers: getAuthHeaders()
        });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        showToast(t('quotas.validation.recordDeletedToast'), 'success');
        await fetchUpstreamValidations(true);
        await fetchUpstreamQuotas(false);
      } catch (err) {
        showToast(t('quotas.deleteFailedToast', { msg: err.message }), 'error');
      }
    }

    // 0. 获取上游凭证配额 (Antigravity & Codex 账号卡片)
    let isFetchingUpstreamQuotas = false;
    function setUpstreamPlaceholder(html) {
      const cards = document.getElementById('upstream-cards-container');
      const compact = document.getElementById('upstream-compact-container');
      if (cards) cards.innerHTML = html;
      if (compact) compact.innerHTML = html;
    }

    async function fetchUpstreamQuotas(force = false) {
      if (isMemberAuth()) return;
      if (isFetchingUpstreamQuotas) return;
      isFetchingUpstreamQuotas = true;
      const secret = getAdminSecret();
      if (!secret) {
        setUpstreamPlaceholder(`
          <div class="upstream-empty-state" style="padding: 32px 20px;">
            <div style="font-size: 14px; color: var(--text-muted); margin-bottom: 8px;">${t('quotas.secretNotConfiguredTitle')}</div>
            <div style="font-size: 12px; color: var(--text-dim); margin-bottom: 16px;">${t('quotas.secretRequiredNotice')}</div>
            <button class="btn btn-secondary btn-sm" onclick="openAuthModal()">${t('quotas.setSecretNowBtn')}</button>
          </div>
        `);
        isFetchingUpstreamQuotas = false;
        return;
      }
      try {
        const url = '/console/api/upstream-quotas' + (force ? '?refresh=1' : '');
        const res = await fetch(url, {
          headers: { 'Authorization': 'Bearer ' + secret }
        });
        if (res.status === 200) {
          const text = await res.text();
          let data = { accounts: [] };
          try {
            data = text ? JSON.parse(text) : { accounts: [] };
          } catch (parseErr) {
            console.warn('Upstream quota JSON parse error:', parseErr.message);
          }
          cachedUpstream = (data && Array.isArray(data.accounts)) ? data : { accounts: [] };
          upstreamLoaded = true;
          renderUpstreamCards();
          renderStatusCards();
        } else if (res.status === 401 || res.status === 403) {
          upstreamLoaded = false;
          cachedUpstream = { accounts: [] };
          setUpstreamPlaceholder(`
            <div class="upstream-empty-state" style="padding: 32px 20px;">
              <div style="font-size: 14px; color: var(--danger); margin-bottom: 8px;">${t('quotas.secretAuthFailed', { status: res.status })}</div>
              <div style="font-size: 12px; color: var(--text-dim); margin-bottom: 16px;">${t('quotas.secretMismatchNotice')}</div>
              <button class="btn btn-secondary btn-sm" onclick="openAuthModal()">${t('quotas.reenterSecretBtn')}</button>
            </div>
          `);
        } else {
          upstreamLoaded = false;
          cachedUpstream = { accounts: [] };
          setUpstreamPlaceholder(`<div class="upstream-empty-state" style="color:var(--danger);">${t('quotas.fetchQuotaHttpFailed', { status: res.status })}</div>`);
        }
      } catch (err) {
        upstreamLoaded = false;
        cachedUpstream = { accounts: [] };
        console.warn('Upstream quota fetch error:', err.message);
        setUpstreamPlaceholder(`<div class="upstream-empty-state" style="color:var(--danger);">${t('quotas.fetchNetworkError', { value: escapeHtml(err.message) })}</div>`);
      } finally {
        isFetchingUpstreamQuotas = false;
        try {
          renderOperationsStrip();
        } catch (_) {}
      }
    }

    async function refreshUpstreamQuotas(force = false) {
      setUpstreamPlaceholder(`<div class="upstream-empty-state">${t('quotas.loadingUpstreamData')}</div>`);
      await fetchUpstreamQuotas(force);
      showToast(t('quotas.quotaUpdatedToast'), 'success');
    }

    // 向上游重新查询额度，然后刷新卡片。
    async function triggerActivateQuotaWindow(filename, provider, authIndex) {
      const secret = getAdminSecret();
      if (!secret) {
        showToast(t('quotas.secretNotConfiguredHint'), 'error');
        return;
      }
      try {
        showToast(t('quotas.queryingQuotaProgress', { value: filename || provider }), 'info');
        const res = await fetch('/console/api/upstream-auth/activate-window', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + secret
          },
          body: JSON.stringify({ filename, provider, authIndex })
        });
        const text = await res.text();
        let data = {};
        try {
          data = text ? JSON.parse(text) : {};
        } catch (_) {}
        if (!res.ok || data.success === false) throw new Error(data.message || data.error || t('quotas.quotaQueryFailed'));
        showToast(t('quotas.quotaQueryDoneToast'), 'success');
        // 前端主动触发刷新获取最新配额与重置时刻
        await fetchUpstreamQuotas(true);
      } catch (err) {
        showToast(`${t('quotas.quotaQueryFailedLabel')} ` + err.message, 'error');
      }
    }

    // 触发 Qoder 每日签到 (单个凭证或全部)
    async function triggerQoderCheckin(authId) {
      const secret = getAdminSecret();
      if (!secret) {
        showToast(t('quotas.secretNotConfiguredHint'), 'error');
        return;
      }
      try {
        const targetLabel = authId ? authId : '全部 Qoder 账号';
        showToast(`正在为 ${targetLabel} 执行 Qoder 每日签到...`, 'info');
        const payload = authId ? { auth_id: authId } : {};
        const res = await fetch('/console/api/cpa/v8/qoder/checkin', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + secret
          },
          body: JSON.stringify(payload)
        });
        const text = await res.text();
        let data = {};
        try {
          data = text ? JSON.parse(text) : {};
        } catch (_) {}
        if (!res.ok) throw new Error(data.message || data.error || '签到请求失败');
        showToast(`Qoder 每日签到完成 (${targetLabel})`, 'success');
        await fetchUpstreamQuotas(true);
      } catch (err) {
        showToast('Qoder 每日签到失败: ' + err.message, 'error');
      }
    }

    // 批量查询当前筛选 Provider（或全部可查询 Provider）凭证额度
    async function triggerActivateFilteredAccounts() {
      const secret = getAdminSecret();
      if (!secret) {
        showToast(t('quotas.secretNotConfiguredHint'), 'error');
        return;
      }
      const filter = (activeUpstreamFilter || 'all').toLowerCase();
      const accounts = (cachedUpstream.accounts || []).filter(a => {
        if (a.disabled || a.status === 'disabled') return false;
        if (a.health?.blocked && a.health?.cooldown?.scope !== 'models') return false;
        if (filter !== 'all') {
          if (filter === 'workbuddy') {
            const p = (a.provider || '').toLowerCase();
            return p === 'workbuddy' || p === 'workbuddy-cn';
          }
          if (filter === 'qoder') {
            const p = (a.provider || '').toLowerCase();
            return p.startsWith('qoder');
          }
          if (filter === 'zcode') {
            const p = (a.provider || '').toLowerCase();
            return p === 'zcode' || p === 'zcode-zai' || (p.startsWith('zcode') && !p.includes('cn') && !p.includes('bigmodel'));
          }
          if (filter === 'zcode-cn' || filter === 'bigmodel') {
            const p = (a.provider || '').toLowerCase();
            return p === 'zcode-cn' || p === 'zcode-bigmodel' || p === 'bigmodel';
          }
          return (a.provider || '').toLowerCase() === filter;
        }
        const prov = (a.provider || '').toLowerCase();
        return prov === 'codex' || prov === 'antigravity' || prov === 'claude' ||
          prov === 'workbuddy' || prov === 'workbuddy-cn' ||
          prov === 'qoder' || prov === 'qoder-cn' || prov === 'qoder-intl' || prov === 'qoder-en' ||
          prov === 'copilot-tencent' || prov === 'kimi' || prov === 'xai' || prov === 'devin' ||
          prov === 'zcode' || prov === 'zcode-cn' || prov === 'zcode-zai' || prov === 'zcode-bigmodel' ||
          prov === 'openrouter' || prov === 'opencode' ||
          Boolean(a.supports_quota || a.quota_provider || a.quota_plugin_id);
      });

      const filterLabel = filter === 'all' ? t('quotas.filter.all') : (filter === 'antigravity' ? 'Antigravity' : (filter === 'codex' ? 'Codex' : (filter === 'claude' ? 'Claude' : (filter === 'workbuddy' ? 'WorkBuddy' : (filter === 'qoder' ? 'Qoder' : (filter === 'openrouter' ? 'OpenRouter' : (filter === 'opencode' ? 'OpenCode' : filter)))))));
      if (accounts.length === 0) {
        showToast(t('quotas.noQueryableCredentials', { filterLabel: filterLabel }), 'warning');
        return;
      }

      showToast(t('quotas.batchQueryingProgress', { count: accounts.length, filterLabel: filterLabel }), 'info');
      let successCount = 0;
      const concurrency = 4;
      for (let i = 0; i < accounts.length; i += concurrency) {
        const batch = accounts.slice(i, i + concurrency);
        await Promise.allSettled(batch.map(async acc => {
          try {
            const res = await fetch('/console/api/upstream-auth/activate-window', {
              method: 'POST',
              headers: {
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ' + secret
              },
              body: JSON.stringify({ filename: acc.filename || acc.id, provider: acc.provider, authIndex: acc.auth_index })
            });
            if (res.ok) successCount++;
          } catch (e) {}
        }));
      }
      showToast(t('quotas.refreshAllProgressToast', { successCount: successCount }), 'success');
      await fetchUpstreamQuotas(true);
    }

    function onUpstreamSearch(query) {
      upstreamSearchQuery = (query || '').trim().toLowerCase();
      renderUpstreamCards();
    }

    function toggleHideDisabled(hide) {
      upstreamHideDisabled = Boolean(hide);
      localStorage.setItem('cpa_upstream_hide_disabled', String(upstreamHideDisabled));
      renderUpstreamCards();
    }

    function switchUpstreamViewMode(mode) {
      activeUpstreamViewMode = mode;
      localStorage.setItem('cpa_upstream_view_mode', mode);
      const tabs = document.querySelectorAll('#upstream-view-mode-tabs button');
      tabs.forEach(btn => {
        btn.classList.toggle('active', btn.getAttribute('onclick')?.includes(`'${mode}'`));
      });
      renderUpstreamCards();
    }

    function updateActivateProviderButton() {
      const btn = document.getElementById('btn-activate-provider');
      if (!btn) return;
      const f = (activeUpstreamFilter || 'all').toLowerCase();
      if (f === 'all') {
        btn.textContent = t('quotas.refreshAllQuotaBtn');
        btn.title = t('quotas.refreshAllGenericQuotaTitle');
      } else if (f === 'antigravity') {
        btn.textContent = t('quotas.refreshAllAntigravityQuota');
        btn.title = t('quotas.refreshAllAntigravityQuotaTitle');
      } else if (f === 'codex') {
        btn.textContent = t('quotas.refreshAllCodexQuota');
        btn.title = t('quotas.refreshAllCodexQuotaTitle');
      } else if (f === 'claude') {
        btn.textContent = t('quotas.refreshAllClaudeQuota');
        btn.title = t('quotas.refreshAllClaudeQuotaTitle');
      } else if (f === 'workbuddy' || f === 'workbuddy-cn') {
        btn.textContent = t('quotas.refreshAllWorkBuddyQuota');
        btn.title = t('quotas.refreshAllWorkBuddyQuotaTitle');
      } else if (f === 'qoder' || f === 'qoder-cn') {
        btn.textContent = t('quotas.refreshAllQoderQuota');
        btn.title = t('quotas.refreshAllQoderQuotaTitle');
      } else if (f === 'zcode') {
        btn.textContent = t('quotas.refreshAllZCodeQuota');
        btn.title = t('quotas.refreshAllZCodeQuotaTitle');
      } else if (f === 'zcode-cn' || f === 'bigmodel') {
        btn.textContent = t('quotas.refreshAllBigModelQuota');
        btn.title = t('quotas.refreshAllBigModelQuotaTitle');
      } else if (f === 'openrouter') {
        btn.textContent = t('quotas.refreshAllOpenRouterQuota');
        btn.title = t('quotas.refreshAllOpenRouterQuotaTitle');
      } else if (f === 'opencode') {
        btn.textContent = t('quotas.refreshAllOpenCodeQuota');
        btn.title = t('quotas.refreshAllOpenCodeQuotaTitle');
      } else {
        const brandName = f.charAt(0).toUpperCase() + f.slice(1);
        btn.textContent = t('quotas.refreshAllBrandQuota', { brandName: brandName });
        btn.title = t('quotas.refreshAllBrandQuotaTitle', { brandName: brandName });
      }
      const openrouterBtn = document.getElementById('btn-manage-openrouter');
      if (openrouterBtn) {
        openrouterBtn.style.display = f === 'openrouter' ? 'inline-flex' : 'none';
      }
      const qoderCheckinBtn = document.getElementById('btn-qoder-checkin');
      if (qoderCheckinBtn) {
        qoderCheckinBtn.style.display = (f === 'qoder' || f === 'qoder-cn') ? 'inline-flex' : 'none';
      }
    }

    function switchUpstreamFilter(filter) {
      activeUpstreamFilter = filter;
      const tabs = document.querySelectorAll('#upstream-filter-tabs button');
      tabs.forEach(t => t.classList.remove('active'));
      const activeBtn = Array.from(tabs).find(b => b.getAttribute('onclick')?.includes(`'${filter}'`));
      if (activeBtn) activeBtn.classList.add('active');
      updateActivateProviderButton();
      renderUpstreamCards();
    }
