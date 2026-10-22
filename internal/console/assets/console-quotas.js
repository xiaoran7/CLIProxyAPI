/**
 * Gate Admin Console - Upstream Quotas, Validations & Credentials Module
 * 包含上游凭证配额卡片渲染、搜索过滤、Google 验证处理、OpenRouter 与优先级管理及用量统计
 */

    // ---- 优先级弹窗交互 ----
    let currentEditingAuth = null;

    function openPriorityModal(filename) {
      const accounts = cachedUpstream.accounts || [];
      const acc = accounts.find(a => a.filename === filename || a.id === filename);
      if (!acc) {
        showToast(t('quotas.credentialNotFound'), 'error');
        return;
      }
      currentEditingAuth = acc;
      document.getElementById('prio-modal-filename').innerText = acc.filename;
      const dispName = (acc.display_name || acc.nickname || acc.email || acc.filename || '').replace(/\.json$/i, '');
      document.getElementById('prio-modal-email').innerText = dispName || acc.email;
      document.getElementById('prio-modal-input').value = normalizeAuthPriority(acc.priority);
      // 保存总是会把当前数值作为显式字段写进 auths/<name>.json——哪怕一个键都没碰。
      // 而服务端已把 "未设" 归一化成默认档位，前端无从区分显式与未设，因此不能只对
      // 未设的号提示。默认档位一并说明，让人知道不设等于什么。
      document.getElementById('prio-modal-default-hint').innerText =
        t('quotas.priority.saveHint', { value: normalizeAuthPriority(acc.priority), value2: defaultAuthPriority() });

      const toggle = document.getElementById('prio-modal-status-toggle');
      const isEnabled = !acc.disabled && acc.status !== 'disabled';
      toggle.checked = isEnabled;

      openModal('upstream-priority-modal');
    }

    function setPresetPriority(val) {
      document.getElementById('prio-modal-input').value = val;
    }

    async function saveUpstreamPriority() {
      if (!currentEditingAuth) return;
      const secret = getAdminSecret();
      if (!secret) {
        showToast(t('quotas.secretNotConfigured'), 'error');
        return;
      }
      const btn = document.getElementById('prio-modal-save-btn');
      btn.disabled = true;
      btn.innerText = t('quotas.savingProgressLabel');

      const prioVal = Number(document.getElementById('prio-modal-input').value);
      const isEnabled = document.getElementById('prio-modal-status-toggle').checked;
      const filename = currentEditingAuth.filename || currentEditingAuth.id;

      try {
        // 1. 更新优先级
        const prioRes = await fetch('/console/api/upstream-auth/priority', {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + secret
          },
          body: JSON.stringify({
            name: filename,
            priority: prioVal
          })
        });
        if (!prioRes.ok) {
          const errData = await prioRes.json().catch(() => ({}));
          throw new Error(errData.error || t('quotas.priority.updateFailed'));
        }

        // 2. 更新启用/停用状态
        const statusRes = await fetch('/console/api/upstream-auth/status', {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + secret
          },
          body: JSON.stringify({
            name: filename,
            disabled: !isEnabled
          })
        });
        if (!statusRes.ok) {
          const errData = await statusRes.json().catch(() => ({}));
          throw new Error(errData.error || t('quotas.updateAccountStatusFailed'));
        }

        showToast(t('quotas.priority.updatedToast', { email: currentEditingAuth.email, prioVal: prioVal }), 'success');
        closeModal('upstream-priority-modal');
        // 本地立即刷新配额与状态卡
        await fetchUpstreamQuotas(false);
      } catch (err) {
        showToast(err.message, 'error');
      } finally {
        btn.disabled = false;
        btn.innerText = t('quotas.saveAndApply');
      }
    }

    // ---- OpenRouter 原生账号管理交互 (Gate 自治) ----
    let cachedOpenRouterAccountList = [];

    async function openOpenRouterModal() {
      openModal('modal-openrouter-accounts');
      hideOpenRouterForm();
      await loadOpenRouterAccountList();
    }

    async function loadOpenRouterAccountList() {
      const tbody = document.getElementById('openrouter-accounts-tbody');
      if (!tbody) return;
      tbody.innerHTML = `<tr><td colspan="6" style="text-align:center; color:var(--text-dim); padding:20px;">${t('quotas.loadingAccountList')}</td></tr>`;
      try {
        const res = await fetch('/console/api/openrouter/accounts', {
          headers: getAuthHeaders()
        });
        if (!res.ok) throw new Error(t('quotas.openrouter.fetchListFailed'));
        const text = await res.text();
        let data = { accounts: [] };
        try {
          data = text ? JSON.parse(text) : { accounts: [] };
        } catch (_) {}
        cachedOpenRouterAccountList = (data && Array.isArray(data.accounts)) ? data.accounts : [];
        renderOpenRouterAccountsTable();
      } catch (err) {
        tbody.innerHTML = `<tr><td colspan="6" style="text-align:center; color:var(--danger); padding:20px;">${escapeHtml(err.message)}</td></tr>`;
      }
    }

    function renderOpenRouterAccountsTable() {
      const tbody = document.getElementById('openrouter-accounts-tbody');
      if (!tbody) return;
      if (cachedOpenRouterAccountList.length === 0) {
        tbody.innerHTML = `<tr><td colspan="6" style="text-align:center; color:var(--text-dim); padding:24px;">${t('quotas.openrouter.emptyState')}</td></tr>`;
        return;
      }

      tbody.innerHTML = cachedOpenRouterAccountList.map(acc => {
        const hasMgmt = acc.hasMgmtKey;
        const mgmtBadge = hasMgmt
          ? `<span style="display:inline-flex; align-items:center; gap:3px; color:#10b981; background:rgba(16,185,129,0.1); padding:2px 6px; border-radius:4px; font-size:11px;" title="${escapeHtml(acc.mgmtKeyMasked || t('quotas.configuredBadge'))}"><svg fill="none" stroke="currentColor" stroke-width="2.5" style="width:11px;height:11px;" viewBox="0 0 24 24"><path d="M5 13l4 4L19 7" stroke-linecap="round" stroke-linejoin="round"/></svg>${t('quotas.configuredBadge')}</span>`
          : `<span style="color:var(--text-dim); background:rgba(0,0,0,0.04); padding:2px 6px; border-radius:4px; font-size:11px;" title="${t('quotas.openrouter.mgmtKeyMissing')}">${t('quotas.notConfigured')}</span>`;

        const apiMasked = acc.apiKeyMasked ? `<span style="font-family:var(--mono); color:var(--text-muted); font-size:11px;">${escapeHtml(acc.apiKeyMasked)}</span>` : `<span style="color:var(--text-dim); font-size:11px;">${t('quotas.notFilled')}</span>`;
        const todayCountTxt = Number.isFinite(acc.today_requests) ? `<strong style="color:#3b82f6; font-family:var(--mono); font-size:13px;">${acc.today_requests}</strong> ${t('quotas.timesUnit')}` : `<span style="color:var(--text-dim); font-size:11px;">${t('quotas.pendingFetch')}</span>`;

        return `
          <tr style="border-bottom:1px solid var(--border-subtle, rgba(0,0,0,0.04));">
            <td style="padding:8px 6px; font-weight:600; color:var(--text-main);">${escapeHtml(acc.name)}</td>
            <td style="padding:8px 6px; color:var(--text-muted); font-size:11px;">${escapeHtml(acc.note || '-')}</td>
            <td style="padding:8px 6px;">${apiMasked}</td>
            <td style="padding:8px 6px;">${mgmtBadge}</td>
            <td style="padding:8px 6px;">${todayCountTxt}</td>
            <td style="padding:8px 6px; text-align:right; white-space:nowrap;">
              <button class="btn btn-secondary btn-sm" onclick="editOpenRouterAccount('${escapeHtml(acc.id)}')" style="padding:2px 8px; font-size:11px; margin-right:4px;">${t('quotas.actionEdit')}</button>
              <button class="btn btn-secondary btn-sm" onclick="deleteOpenRouterAccountById('${escapeHtml(acc.id)}', '${escapeHtml(acc.name)}')" style="padding:2px 8px; font-size:11px; color:var(--danger);">${t('quotas.actionDelete')}</button>
            </td>
          </tr>
        `;
      }).join('');
    }

    function showAddOpenRouterForm(editData = null) {
      const card = document.getElementById('openrouter-form-card');
      const title = document.getElementById('openrouter-form-title');
      const idInput = document.getElementById('openrouter-form-id');
      const nameInput = document.getElementById('openrouter-form-name');
      const noteInput = document.getElementById('openrouter-form-note');
      const apiInput = document.getElementById('openrouter-form-apikey');
      const mgmtInput = document.getElementById('openrouter-form-mgmtkey');

      if (!card) return;
      card.style.display = 'block';

      if (editData) {
        title.textContent = t('quotas.openrouter.editAccountTitle', { name: editData.name });
        idInput.value = editData.id;
        nameInput.value = editData.name || '';
        noteInput.value = editData.note || '';
        apiInput.value = '';
        apiInput.placeholder = editData.hasApiKey ? t('quotas.openrouter.keepApiKeyHint') : 'sk-or-v1-...';
        mgmtInput.value = '';
        mgmtInput.placeholder = editData.hasMgmtKey ? t('quotas.openrouter.keepMgmtKeyHint') : 'sk-or-v1-...';
      } else {
        title.textContent = t('quotas.openrouter.addAccountTitle');
        idInput.value = '';
        nameInput.value = '';
        noteInput.value = '';
        apiInput.value = '';
        apiInput.placeholder = 'sk-or-v1-...';
        mgmtInput.value = '';
        mgmtInput.placeholder = t('quotas.openrouter.mgmtKeyPlaceholder');
      }
      nameInput.focus();
    }

    function hideOpenRouterForm() {
      const card = document.getElementById('openrouter-form-card');
      if (card) card.style.display = 'none';
    }

    function editOpenRouterAccount(id) {
      const acc = cachedOpenRouterAccountList.find(a => String(a.id) === String(id));
      if (!acc) return;
      showAddOpenRouterForm(acc);
    }

    async function submitOpenRouterAccount() {
      const id = document.getElementById('openrouter-form-id').value.trim();
      const name = document.getElementById('openrouter-form-name').value.trim();
      const note = document.getElementById('openrouter-form-note').value.trim();
      const apiKey = document.getElementById('openrouter-form-apikey').value.trim();
      const mgmtKey = document.getElementById('openrouter-form-mgmtkey').value.trim();
      const btn = document.getElementById('openrouter-form-submit-btn');

      if (!name) {
        showToast(t('quotas.accountNameRequired'), 'error');
        return;
      }

      btn.disabled = true;
      btn.textContent = t('quotas.savingLabel');

      try {
        const isEdit = Boolean(id);
        const url = isEdit ? `/console/api/openrouter/accounts/${encodeURIComponent(id)}` : '/console/api/openrouter/accounts';
        const method = isEdit ? 'PATCH' : 'POST';

        const payload = { name, note };
        if (apiKey) payload.apiKey = apiKey;
        if (mgmtKey) payload.mgmtKey = mgmtKey;

        const res = await fetch(url, {
          method,
          headers: {
            'Content-Type': 'application/json',
            ...getAuthHeaders()
          },
          body: JSON.stringify(payload)
        });

        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          throw new Error(errData.error || t('quotas.openrouter.saveAccountFailed'));
        }

        showToast(isEdit ? t('quotas.accountUpdatedToast', { name: name }) : t('quotas.accountAddedToast', { name: name }), 'success');
        hideOpenRouterForm();
        await loadOpenRouterAccountList();
        // 静默刷新外层大盘
        fetchUpstreamQuotas(true).catch(() => {});
      } catch (err) {
        showToast(err.message, 'error');
      } finally {
        btn.disabled = false;
        btn.textContent = t('quotas.openrouter.saveAccount');
      }
    }

    async function deleteOpenRouterAccountById(id, name) {
      if (!confirm(t('quotas.openrouter.deleteConfirm', { value: name || id }))) return;
      try {
        const res = await fetch(`/console/api/openrouter/accounts/${encodeURIComponent(id)}`, {
          method: 'DELETE',
          headers: getAuthHeaders()
        });
        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          throw new Error(errData.error || t('quotas.openrouter.deleteAccountFailed'));
        }
        showToast(t('quotas.openrouter.deleteAccountSuccess', { value: name || id }), 'success');
        await loadOpenRouterAccountList();
        fetchUpstreamQuotas(true).catch(() => {});
      } catch (err) {
        showToast(err.message, 'error');
      }
    }


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
        const timeStr = new Date(v.updated_at).toLocaleTimeString(LOCALE, { hour12: false });
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
            const timeStr = new Date(v.updated_at).toLocaleString(LOCALE, { hour12: false });
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
                  ⚠️ ${errSafe}
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

            const updateTime = acc.updated_at ? new Date(acc.updated_at).toLocaleTimeString(LOCALE, { hour12: false }) : '--';

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
            const timeStr = new Date(v.ts).toLocaleString(LOCALE, { hour12: false });
            const upTimeStr = new Date(v.updated_at).toLocaleString(LOCALE, { hour12: false });
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
            return p === 'qoder' || p === 'qoder-cn';
          }
          return (a.provider || '').toLowerCase() === filter;
        }
        const prov = (a.provider || '').toLowerCase();
        return prov === 'codex' || prov === 'antigravity' || prov === 'workbuddy' || prov === 'workbuddy-cn' || prov === 'openrouter' || prov === 'opencode';
      });

      const filterLabel = filter === 'all' ? t('quotas.filter.all') : (filter === 'antigravity' ? 'Antigravity' : (filter === 'codex' ? 'Codex' : (filter === 'claude' ? 'Claude' : (filter === 'workbuddy' ? 'WorkBuddy' : (filter === 'openrouter' ? 'OpenRouter' : (filter === 'opencode' ? 'OpenCode' : filter))))));
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

    function getWorkBuddyExpirationInfo(acc) {
      if (!acc) {
        return {
          hasExpiration: false,
          shortText: t('quotas.workbuddy.expiryUnannounced'),
          fullText: t('quotas.workbuddy.expiryUnannounced'),
          countdown: '',
          color: 'var(--text-dim)',
          tooltip: t('quotas.workbuddy.expiryUnannouncedTooltip'),
          packages: [],
          hasMultiplePackages: false,
          packageCount: 0
        };
      }

      // 提取所有候选包明细（优先 acc.expiration_packages，其次 acc.groups 各 buckets）
      let rawPackages = [];
      if (Array.isArray(acc.expiration_packages) && acc.expiration_packages.length > 0) {
        rawPackages = acc.expiration_packages;
      } else if (Array.isArray(acc.groups)) {
        for (const g of acc.groups) {
          for (const b of (g.buckets || [])) {
            if (b.expires_at || b.reset_time) {
              rawPackages.push({
                package_name: b.name,
                package_code: b.package_code || b.bucket_id,
                remain_points: b.remain_capacity != null ? Number(b.remain_capacity) : 0,
                total_points: b.total_capacity != null ? Number(b.total_capacity) : 0,
                expires_at: b.expires_at || '',
                reset_time: b.reset_time || ''
              });
            }
          }
        }
      }

      // 如果未收集到包列表，但顶层有 expires_at / reset_time，构建兜底单包
      if (rawPackages.length === 0 && (acc.expires_at || acc.reset_time)) {
        rawPackages.push({
          package_name: t('quotas.workbuddy.defaultPackName'),
          package_code: '',
          remain_points: acc.runs_out_points != null ? Number(acc.runs_out_points) : (acc.remaining_credits != null ? Number(acc.remaining_credits) : 0),
          total_points: acc.total_credits != null ? Number(acc.total_credits) : 0,
          expires_at: acc.expires_at || '',
          reset_time: acc.reset_time || ''
        });
      }

      if (rawPackages.length === 0) {
        return {
          hasExpiration: false,
          shortText: t('quotas.workbuddy.expiryUnannounced'),
          fullText: t('quotas.workbuddy.expiryUnannounced'),
          countdown: '',
          color: 'var(--text-dim)',
          tooltip: t('quotas.workbuddy.expiryUnannouncedTooltip'),
          packages: [],
          hasMultiplePackages: false,
          packageCount: 0
        };
      }

      const now = Date.now();
      const formattedPackages = [];

      for (const p of rawPackages) {
        let targetTs = null;
        if (p.reset_time) {
          try {
            const d = new Date(p.reset_time);
            if (!isNaN(d.getTime())) targetTs = d.getTime();
          } catch (_) {}
        }
        if (!targetTs && p.expires_at) {
          try {
            const s = String(p.expires_at).trim();
            const norm = /(?:Z|[+-]\d{2}(?::?\d{2})?)$/i.test(s) ? s : s.replace(' ', 'T') + '+08:00';
            const d = new Date(norm);
            if (!isNaN(d.getTime())) targetTs = d.getTime();
          } catch (_) {}
        }

        let shortText = '';
        let fullText = p.expires_at || '';
        const currentCstYear = new Date(now + 8 * 3600 * 1000).getUTCFullYear();
        if (targetTs) {
          // 全局东八区单点真理源：统一转为东八区（UTC+8 北京时间）并使用 UTC getter 提取，避免多时区偏移
          const cstDate = new Date(targetTs + 8 * 3600 * 1000);
          const yyyy = cstDate.getUTCFullYear();
          const mm = cstDate.getUTCMonth() + 1;
          const dd = cstDate.getUTCDate();
          const hh = String(cstDate.getUTCHours()).padStart(2, '0');
          const min = String(cstDate.getUTCMinutes()).padStart(2, '0');
          if (yyyy !== currentCstYear) {
            shortText = t('quotas.points.expiryDateTime', { yyyy: yyyy, mm: mm, dd: dd, hh: hh, min: min });
          } else {
            shortText = t('quotas.points.expiryShortDate', { mm: mm, dd: dd, hh: hh, min: min });
          }
          if (!fullText) {
            const ss = String(cstDate.getUTCSeconds()).padStart(2, '0');
            fullText = `${yyyy}-${String(mm).padStart(2, '0')}-${String(dd).padStart(2, '0')} ${hh}:${min}:${ss}`;
          }
        } else if (p.expires_at) {
          const s = String(p.expires_at).trim();
          const m = s.match(/^(\d{4})-(\d{1,2})-(\d{1,2})/);
          if (m && Number(m[1]) !== currentCstYear) {
            const tm = s.slice(11, 16);
            shortText = t('quotas.points.expiryFullDate', { value: m[1], value2: Number(m[2]), value3: Number(m[3]), value4: tm ? ' ' + tm : '' }).trim();
          } else {
            shortText = s.slice(5, 16).replace('T', ' ');
          }
        } else {
          shortText = t('quotas.workbuddy.unknownExpiry');
          fullText = t('quotas.workbuddy.unknownExpiry');
        }

        let countdown = '';
        let color = 'var(--text-muted)';
        let isLongTerm = false;
        const pkgRawName = p.package_name || p.name || '';
        // i18n-exempt: 匹配上游 API 返回的资源包原始名（含中文），与 lib/upstream/sync.mjs 同口径，不得翻译
        const isFreeOrTrialName = /free plan|体验|免费/i.test(pkgRawName);

        if (targetTs) {
          const diff = targetTs - now;
          if (diff <= 0) {
            countdown = t('quotas.points.expired');
            color = '#ef4444';
          } else {
            const hours = Math.floor(diff / (1000 * 3600));
            const days = Math.floor(hours / 24);
            // 识别远期基础包（如腾讯 2035 年代表永久有效的 Free Plan Subscription 或体验版）
            if (days >= 365 * 3 || (isFreeOrTrialName && days >= 365)) {
              isLongTerm = true;
              shortText = t('quotas.workbuddy.longTerm');
              countdown = '';
              color = '#10b981';
            } else if (hours < 24) {
              countdown = t('quotas.points.expiresInHours', { value: Math.max(1, hours) });
              color = '#ef4444';
            } else if (hours < 72) {
              countdown = t('quotas.points.expiresInDays', { days: days });
              color = '#f59e0b';
            } else {
              if (days < 365) {
                countdown = t('quotas.points.expiresInDays', { days: days });
              } else {
                const years = Math.floor(days / 365);
                const remMonths = Math.floor((days % 365) / 30);
                countdown = remMonths >= 1 ? t('quotas.points.expiresInYearsMonths', { years: years, remMonths: remMonths }) : t('quotas.points.expiresInYears', { years: years });
              }
              color = 'var(--text-muted)';
            }
          }
        } else if (p.expires_at) {
          const m = String(p.expires_at).trim().match(/^(\d{4})/);
          if (m && (Number(m[1]) >= currentCstYear + 3 || (isFreeOrTrialName && Number(m[1]) > currentCstYear))) {
            isLongTerm = true;
            shortText = t('quotas.workbuddy.longTerm');
            countdown = '';
            color = '#10b981';
          }
        }

        formattedPackages.push({
          name: p.package_name || p.name || t('quotas.workbuddy.defaultPackName'),
          code: p.package_code || '',
          points: Number(p.remain_points ?? p.remain_capacity ?? 0),
          totalPoints: Number(p.total_points ?? p.total_capacity ?? 0),
          targetTs,
          shortText,
          fullText,
          countdown,
          color,
          isLongTerm,
          hasExpiryText: Boolean(targetTs || p.expires_at)
        });
      }

      // 按到期时间排序（最早到期的排前面，有到期但无时间戳的其次，完全无到期时间的排末尾）
      formattedPackages.sort((a, b) => {
        if (a.targetTs && b.targetTs) return a.targetTs - b.targetTs;
        if (a.targetTs) return -1;
        if (b.targetTs) return 1;
        if (a.hasExpiryText && !b.hasExpiryText) return -1;
        if (!a.hasExpiryText && b.hasExpiryText) return 1;
        return 0;
      });

      // 检查是否有任何包包含有效到期时间（无论是 targetTs 还是非空的 expires_at）
      const hasAnyExpiration = formattedPackages.some(p => p.targetTs != null || p.hasExpiryText);
      if (!hasAnyExpiration) {
        return {
          hasExpiration: false,
          shortText: t('quotas.workbuddy.expiryUnannounced'),
          fullText: t('quotas.workbuddy.expiryUnannounced'),
          countdown: '',
          color: 'var(--text-dim)',
          tooltip: t('quotas.workbuddy.expiryUnannouncedTooltip'),
          packages: formattedPackages,
          hasMultiplePackages: false,
          packageCount: formattedPackages.length
        };
      }

      const earliest = formattedPackages[0];
      const hasMultiplePackages = formattedPackages.length > 1;
      const isAllLongTerm = hasAnyExpiration && formattedPackages.every(p => p.isLongTerm);

      let tooltip = '';
      if (!hasMultiplePackages) {
        tooltip = t('quotas.workbuddy.pointsExpiryTooltip', { fullText: earliest.fullText });
        if (earliest.points > 0) {
          tooltip += ` ${t('quotas.points.availablePointsSuffix', { points: earliest.points })}`;
        }
        if (earliest.isLongTerm) {
          tooltip += ` ${t('quotas.workbuddy.longTermSuffix')}`;
        }
      } else {
        tooltip = `${t('quotas.workbuddy.pointsBatchExpiryTooltip', { count: formattedPackages.length })}\n` +
          formattedPackages.map((p, idx) =>
            `${t('quotas.workbuddy.batchLine', { value: idx + 1, name: p.name, points: p.points, fullText: p.fullText })}${p.countdown ? ` (${p.countdown})` : (p.isLongTerm ? ` ${t('quotas.workbuddy.longTermParenthetical')}` : '')}`
          ).join('\n');
      }

      return {
        hasExpiration: true,
        targetTs: earliest.targetTs,
        shortText: isAllLongTerm ? t('quotas.workbuddy.longTerm') : earliest.shortText,
        fullText: earliest.fullText,
        countdown: isAllLongTerm ? '' : earliest.countdown,
        color: isAllLongTerm ? '#10b981' : earliest.color,
        tooltip,
        runsOutPoints: earliest.points,
        packages: formattedPackages,
        hasMultiplePackages,
        packageCount: formattedPackages.length,
        isAllLongTerm,
        isLongTerm: earliest.isLongTerm
      };
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
            const brand = getProviderBrand(acc.provider);
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
                } else if (gname.includes('claude') || bname.includes('claude') || bid.includes('claude') || gname.includes('gpt') || bname.includes('gpt')) {
                  if (isWeekly) claudeWeeklyBucket = bkt;
                  else if (!claude5hBucket || is5h) claude5hBucket = bkt;
                } else if (!gname.includes('claude') && !gname.includes('gpt') && !gname.includes('3p')) {
                  if (isWeekly) codexWeeklyBucket = bkt;
                  else if (!codex5hBucket || is5h) codex5hBucket = bkt;
                } else {
                  if (isWeekly && !claudeWeeklyBucket) claudeWeeklyBucket = bkt;
                  else if (!claude5hBucket) claude5hBucket = bkt;
                }
              }
            }

            // 构建精细的双指示条渲染
            let metersHtml = '';
            let cooldownCompactNotice = '';
            if (health.cooldown && health.cooldown.scope === 'models') {
              cooldownCompactNotice = `<div style="font-size:10px; color:#d97706; margin-bottom:2px; font-family:var(--mono);">${escapeHtml(health.detail || health.label)}</div>`;
            }
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

              metersHtml = `
                <div class="compact-meter-wrap" style="display:flex; flex-direction:column; justify-content:center; gap:3px;" title="${escapeHtml(expInfo.tooltip || t('quotas.workbuddy.officialPointsTooltip', { remainCredits: remainCredits, totalCredits: totalCredits }))}">
                  <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                    <span style="color:var(--text-muted); font-size:10.5px;">${t('quotas.workbuddy.packPoints')}</span>
                    <span style="font-family:var(--mono); font-weight:700; color:${col};">
                      ${t('quotas.points.remainingPoints', { value: remainCredits.toLocaleString(undefined, {minimumFractionDigits: 0, maximumFractionDigits: 2}) })}
                    </span>
                  </div>
                  <div class="quota-meter-bar" style="height:4px;">
                    <div class="quota-meter-fill ${cls}" style="width: ${pct}%"></div>
                  </div>
                  <div style="display:flex; justify-content:space-between; align-items:center; font-size:10px; color:var(--text-dim); font-family:var(--mono);">
                    <span>${t('quotas.points.totalPoints', { toLocaleString: totalCredits.toLocaleString(LOCALE) })}</span>
                    <span>${t('quotas.points.usedLabel', { value: usedCredits.toLocaleString(undefined, {minimumFractionDigits: 0, maximumFractionDigits: 2}) })}</span>
                  </div>
                  <div style="display:flex; justify-content:space-between; align-items:center; font-size:9.5px; color:var(--text-dim); font-family:var(--mono); margin-top:1px; padding-top:2px; border-top:1px dashed var(--border-subtle, rgba(0,0,0,0.06));" title="${escapeHtml(expInfo.tooltip)}">
                    <span>${expInfo.isAllLongTerm ? t('quotas.points.quotaLabel') : (expInfo.hasMultiplePackages ? `${t('quotas.workbuddy.batchedExpiry')} <span style="font-size:9px; opacity:0.8;">(${t('quotas.workbuddy.packageCountTightSuffix', { packageCount: expInfo.packageCount })}</span>` : t('quotas.workbuddy.expiresAt'))}</span>
                    <span style="color:${expInfo.color}; font-weight:600;">
                      ${expInfo.hasMultiplePackages && !expInfo.isAllLongTerm ? `${t('quotas.workbuddy.firstBatchLabel')} ` : ''}${escapeHtml(expInfo.shortText)}${expInfo.countdown ? ` <span style="font-size:9px; font-weight:normal; opacity:0.85;">(${escapeHtml(expInfo.countdown)})</span>` : ''}
                    </span>
                  </div>
                </div>
              `;
            } else if (!Number.isFinite(fallbackBucket?.remaining_percent)) {
              metersHtml = `<div class="account-quota-unknown">${t('quotas.quotaUnknownCpa')}</div>`;
            } else if (acc.provider === 'antigravity') {
              const hasClaude = claude5hBucket && Number.isFinite(claude5hBucket.remaining_percent);
              const hasGemini = gemini5hBucket && Number.isFinite(gemini5hBucket.remaining_percent);

              const buildAntigravityMeter = (bkt, weeklyBkt, defaultLabel, rollingTipKey) => {
                if (!bkt || !Number.isFinite(bkt.remaining_percent)) return '';
                const isClaude = defaultLabel.toLowerCase().includes('claude');
                const isGemini = defaultLabel.toLowerCase().includes('gemini');
                const isModelCooling = health.cooldown && health.cooldown.scope === 'models' && (
                  (isClaude && (health.cooldown.model_keys || []).some(k => /claude|gpt|sonnet|opus|haiku/i.test(k))) ||
                  (isGemini && (health.cooldown.model_keys || []).some(k => /gemini|flash/i.test(k) && !/claude/i.test(k)))
                );

                const rawRem = bkt.remaining_percent;
                const isBktDisabled = bkt.disabled === true;
                const weeklyRem = (weeklyBkt && Number.isFinite(weeklyBkt.remaining_percent)) ? weeklyBkt.remaining_percent : null;
                const isWeeklyBottleneck = weeklyRem !== null && (isBktDisabled || weeklyRem < rawRem);
                const effectiveRem = isBktDisabled ? 0 : (isWeeklyBottleneck ? weeklyRem : rawRem);

                let rem = isModelCooling ? 0 : effectiveRem;
                let cls = 'meter-high'; let col = '#10b981';
                if (rem <= 25) { cls = 'meter-low'; col = '#ef4444'; }
                else if (rem <= 60) { cls = 'meter-medium'; col = '#f59e0b'; }

                let txt = '';
                if (isModelCooling) {
                  txt = escapeHtml(health.cooldown.remaining_text ? t('quotas.bucket.cooldownResume', { time: health.cooldown.remaining_text }) : t('quotas.bucket.cooling'));
                } else if (isBktDisabled || (weeklyRem !== null && weeklyRem <= 0)) {
                  const rTime = weeklyBkt?.reset_time || bkt?.reset_time;
                  const rCd = weeklyBkt?.reset_countdown || bkt?.reset_countdown;
                  txt = (rTime || rCd) ? t('quotas.bucket.weeklyExhaustedWithReset', { reset: escapeHtml(rCd || formatResetAbs(rTime)) }) : t('quotas.bucket.weeklyExhausted');
                } else if (isWeeklyBottleneck) {
                  const rTime = weeklyBkt?.reset_time || bkt?.reset_time;
                  const rCd = weeklyBkt?.reset_countdown || bkt?.reset_countdown;
                  txt = (rTime || rCd) ? t('quotas.bucket.weeklyBottleneckWithReset', { reset: escapeHtml(rCd || formatResetAbs(rTime)) }) : t('quotas.bucket.weeklyBottleneck', { percent: weeklyRem });
                } else if (bkt?.reset_time || bkt?.reset_countdown) {
                  txt = escapeHtml(bkt.reset_countdown || (formatResetAbs(bkt.reset_time) + t('quotas.actionRefresh')));
                } else {
                  txt = t('quotas.bucket.pendingActivation');
                }

                let title = t(rollingTipKey);
                if (weeklyBkt && Number.isFinite(weeklyBkt.remaining_percent)) {
                  const cdSuffix = weeklyBkt.reset_countdown ? t('quotas.bucket.weeklyCountdownSuffix', { time: weeklyBkt.reset_countdown }) : '';
                  title += t('quotas.bucket.weeklyRemainingDetail', { percent: weeklyBkt.remaining_percent, countdown: cdSuffix });
                }
                if (isBktDisabled) {
                  title += t('quotas.bucket.upstreamDisabledNotice');
                } else if (isWeeklyBottleneck) {
                  title += t('quotas.bucket.bottleneckConstraintNotice', { rem5h: rawRem, remWeekly: weeklyRem });
                }

                const suffixTag = (isWeeklyBottleneck && !isModelCooling && !isBktDisabled) ? ` <span style="font-size:9px; font-weight:normal; opacity:0.85;">${t('quotas.bucket.tagWeeklyBottleneck')}</span>` : (isBktDisabled ? ` <span style="font-size:9px; font-weight:normal; opacity:0.85;">${t('quotas.bucket.tagExhausted')}</span>` : '');

                return `
                  <div class="compact-meter-wrap" title="${escapeHtml(title)}">
                    <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                      <span style="color:var(--text-muted); font-size:10.5px;">${escapeHtml(defaultLabel)}</span>
                      <span style="font-family:var(--mono); font-weight:700; color:${col};">${t('quotas.bucket.firstRemainingPercent', { rem1: rem })}${suffixTag}</span>
                    </div>
                    <div class="quota-meter-bar" style="height:4px;">
                      <div class="quota-meter-fill ${cls}" style="width: ${rem}%"></div>
                    </div>
                    <span style="font-family:var(--mono); font-size:10px; color:var(--text-dim); text-align:right;" ${bkt?.reset_time ? `data-reset-time="${escapeHtml(bkt.reset_time)}"` : ''}>${txt}</span>
                  </div>
                `;
              };

              if (hasClaude && hasGemini) {
                // 双主力模型组均存在：展示 Claude 5h 与 Gemini 5h，并在悬浮提示中展示对应周限额
                metersHtml = `
                  ${buildAntigravityMeter(claude5hBucket, claudeWeeklyBucket, 'Claude 5h', 'quotas.bucket.claudeRolling5h')}
                  ${buildAntigravityMeter(gemini5hBucket, geminiWeeklyBucket, 'Gemini 5h', 'quotas.bucket.geminiRolling5h')}
                `;
              } else if (hasClaude) {
                const col2Html = claudeWeeklyBucket && Number.isFinite(claudeWeeklyBucket.remaining_percent)
                  ? buildAntigravityMeter(claudeWeeklyBucket, null, t('quotas.bucket.claudeWeekly'), 'quotas.bucket.claudeWeeklyTotal')
                  : '';
                metersHtml = `
                  ${buildAntigravityMeter(claude5hBucket, claudeWeeklyBucket, 'Claude 5h', 'quotas.bucket.claudeRolling5h')}
                  ${col2Html}
                `;
              } else if (hasGemini) {
                const col2Html = geminiWeeklyBucket && Number.isFinite(geminiWeeklyBucket.remaining_percent)
                  ? buildAntigravityMeter(geminiWeeklyBucket, null, t('quotas.bucket.geminiWeekly'), 'quotas.bucket.geminiWeeklyTotal')
                  : '';
                metersHtml = `
                  ${buildAntigravityMeter(gemini5hBucket, geminiWeeklyBucket, 'Gemini 5h', 'quotas.bucket.geminiRolling5h')}
                  ${col2Html}
                `;
              } else if (fallbackBucket && Number.isFinite(fallbackBucket.remaining_percent)) {
                metersHtml = `
                  ${buildAntigravityMeter(fallbackBucket, null, formatBucketLabel(fallbackBucket.name, fallbackBucket.window) || '5h 额度', 'quotas.bucket.claudeRolling5h')}
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
                const txt1 = (b1?.reset_time || b1?.reset_countdown) ? escapeHtml(b1.reset_countdown || (formatResetAbs(b1.reset_time) + t('quotas.actionRefresh'))) : t('quotas.bucket.pendingActivation');

                let col2Html = '';
                if (b2 && Number.isFinite(b2.remaining_percent)) {
                  const rem2 = b2.remaining_percent;
                  let cls2 = 'meter-high'; let col2 = '#10b981';
                  if (rem2 <= 25) { cls2 = 'meter-low'; col2 = '#ef4444'; }
                  else if (rem2 <= 60) { cls2 = 'meter-medium'; col2 = '#f59e0b'; }
                  const txt2 = (b2?.reset_time || b2?.reset_countdown) ? escapeHtml(b2.reset_countdown || (formatResetAbs(b2.reset_time) + t('quotas.actionRefresh'))) : t('quotas.bucket.pendingActivation');

                  col2Html = `
                    <div class="compact-meter-wrap" title="${t('quotas.bucket.codexWeekly')}">
                      <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                        <span style="color:var(--text-muted); font-size:10.5px;">${t('quotas.bucket.weeklyLimit')}</span>
                        <span style="font-family:var(--mono); font-weight:700; color:${col2};">${t('quotas.bucket.secondRemainingPercent', { rem2: rem2 })}</span>
                      </div>
                      <div class="quota-meter-bar" style="height:4px;">
                        <div class="quota-meter-fill ${cls2}" style="width: ${rem2}%"></div>
                      </div>
                      <span style="font-family:var(--mono); font-size:10px; color:var(--text-dim); text-align:right;" ${b2?.reset_time ? `data-reset-time="${escapeHtml(b2.reset_time)}"` : ''}>${txt2}</span>
                    </div>
                  `;
                }

                metersHtml = `
                  <div class="compact-meter-wrap" title="${t('quotas.bucket.codexRolling5h')}">
                    <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                      <span style="color:var(--text-muted); font-size:10.5px;">${t('quotas.bucket.fiveHourLimit')}</span>
                      <span style="font-family:var(--mono); font-weight:700; color:${col1};">${t('quotas.bucket.firstRemainingPercent', { rem1: rem1 })}</span>
                    </div>
                    <div class="quota-meter-bar" style="height:4px;">
                      <div class="quota-meter-fill ${cls1}" style="width: ${rem1}%"></div>
                    </div>
                    <span style="font-family:var(--mono); font-size:10px; color:var(--text-dim); text-align:right;" ${b1?.reset_time ? `data-reset-time="${escapeHtml(b1.reset_time)}"` : ''}>${txt1}</span>
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
              const txt1 = (b1?.reset_time || b1?.reset_countdown) ? escapeHtml(b1.reset_countdown || (formatResetAbs(b1.reset_time) + t('quotas.actionRefresh'))) : t('quotas.bucket.pendingActivation');

              metersHtml = `
                <div class="compact-meter-wrap">
                  <div style="display:flex; justify-content:space-between; align-items:center; font-size:11px;">
                    <span style="color:var(--text-muted); font-size:10.5px;">${escapeHtml(bname)}</span>
                    <span style="font-family:var(--mono); font-weight:700; color:${col1};">${t('quotas.bucket.firstRemainingPercent', { rem1: rem1 })}</span>
                  </div>
                  <div class="quota-meter-bar" style="height:4px;">
                    <div class="quota-meter-fill ${cls1}" style="width: ${rem1}%"></div>
                  </div>
                  <span style="font-family:var(--mono); font-size:10px; color:var(--text-dim); text-align:right;" ${b1?.reset_time ? `data-reset-time="${escapeHtml(b1.reset_time)}"` : ''}>${txt1}</span>
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
            const brand = getProviderBrand(acc.provider);
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
            const groupsHtml = (acc.groups || []).filter(grp => Array.isArray(grp.buckets) && grp.buckets.some(bkt => Number.isFinite(bkt.remaining_percent))).map(grp => {
              const gname = escapeHtml(grp.name);
              const bucketsHtml = (grp.buckets || []).filter(bkt => Number.isFinite(bkt.remaining_percent)).map(bkt => {
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
                  <div class="model-group-title">${gname}</div>
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

      if (!data || !Array.isArray(data.items) || data.items.length === 0) {
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
        ? data.items.filter(it => {
            const name = String(it.name || '').toLowerCase();
            const provider = String(it.provider || '').toLowerCase();
            const model = String(it.last_model || '').toLowerCase();
            const id = String(it.auth_index || '').toLowerCase();
            return name.includes(filterVal) || provider.includes(filterVal) || model.includes(filterVal) || id.includes(filterVal);
          })
        : data.items;

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
          const brand = getProviderBrand(it.provider);
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
            const brand = getProviderBrand(it.provider);
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
