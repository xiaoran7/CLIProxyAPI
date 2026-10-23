/**
 * Gate Admin Console - Google Antigravity validation banner, page and audit flow
 * Google 验证横幅、验证页与历史审计流水
 */

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
