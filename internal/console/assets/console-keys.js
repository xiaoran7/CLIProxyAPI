/**
 * Gate Admin Console - Keys & Pricing Management Module
 * 包含 API Key CRUD (创建/轮换/别名/删除/模型权限) 及模型定价规则配置与渲染
 * 弹窗拆入 console-keys-modals.js，模型选择器拆入 console-keys-picker.js，定价拆入 console-keys-pricing.js。
 */

    // 安全解析模型白名单（兼容 JSON 数组字符串、逗号/空格分隔字符串以及数组形态）
    function parseModelList(raw) {
      if (!raw) return [];
      if (Array.isArray(raw)) {
        const res = [];
        raw.forEach(item => {
          if (typeof item === 'string') {
            item.split(/[\s,]+/).map(s => s.trim().toLowerCase()).filter(Boolean).forEach(m => {
              if (!res.includes(m)) res.push(m);
            });
          }
        });
        return res;
      }
      if (typeof raw === 'string') {
        const trimmed = raw.trim();
        if (!trimmed || trimmed === '*' || trimmed === '["*"]') return [];
        try {
          const parsed = JSON.parse(trimmed);
          if (Array.isArray(parsed) || typeof parsed === 'string') {
            return parseModelList(parsed);
          }
        } catch (e) {}
        return trimmed.split(/[\s,]+/).map(s => s.trim().toLowerCase()).filter(Boolean).filter((m, idx, arr) => arr.indexOf(m) === idx);
      }
      return [];
    }

    function normalizeKey(k) {
      if (!k || typeof k !== 'object') return k;
      const id = k.key_id != null ? k.key_id : (k.id != null ? k.id : null);
      const alias = k.key_alias != null ? k.key_alias : (k.alias != null ? k.alias : '');
      const quota = k.quota || {};
      const todayTokens = typeof k.today_usage === 'number' ? k.today_usage : (k.today_usage?.tokens != null ? Number(k.today_usage.tokens) : 0);
      const todayReqs = typeof k.today_requests === 'number' ? k.today_requests : (k.today_usage?.requests != null ? Number(k.today_usage.requests) : 0);
      const allTimeTokens = typeof k.all_time_usage === 'number' ? k.all_time_usage : (k.all_time?.tokens != null ? Number(k.all_time.tokens) : 0);
      const allTimeReqs = typeof k.all_time_requests === 'number' ? k.all_time_requests : (k.all_time?.requests != null ? Number(k.all_time.requests) : 0);
      const enabled = k.enabled != null ? Boolean(k.enabled) : Boolean(quota.enabled);
      const dailyLimit = k.daily_limit != null ? Number(k.daily_limit) : (quota.daily_limit != null ? Number(quota.daily_limit) : 0);
      const hardRatio = k.hard_ratio != null ? Number(k.hard_ratio) : (quota.hard_ratio != null ? Number(quota.hard_ratio) : 1.2);
      const rpmLimit = k.rpm_limit != null ? Number(k.rpm_limit) : (quota.rpm_limit != null ? Number(quota.rpm_limit) : 0);
      const hardLimit = k.hard_limit != null ? Number(k.hard_limit) : (dailyLimit > 0 ? Math.floor(dailyLimit * hardRatio) : 0);
      const isBlocked = k.is_blocked != null ? Boolean(k.is_blocked) : (enabled && dailyLimit > 0 && todayTokens >= hardLimit);
      const modelWhitelist = k.model_whitelist != null ? k.model_whitelist : (quota.model_whitelist || []);

      return {
        ...k,
        key_id: id,
        id: id,
        key_alias: alias,
        alias: alias,
        today_usage: todayTokens,
        today_requests: todayReqs,
        all_time_usage: allTimeTokens,
        all_time_requests: allTimeReqs,
        enabled: enabled,
        daily_limit: dailyLimit,
        hard_ratio: hardRatio,
        hard_limit: hardLimit,
        rpm_limit: rpmLimit,
        is_blocked: isBlocked,
        model_whitelist: modelWhitelist,
        cost_usd: Number(k.cost_usd) || 0,
        cache_hit_rate: Number(k.cache_hit_rate) || 0,
      };
    }

    // 2. 获取 Key 列表 (带 SWR 缓存与即时持久化)
    async function fetchKeysData(force = false) {
      if (!force && cachedKeys.length > 0 && (Date.now() - cachedKeysLoadedAt) < 10000) {
        renderKeysTable(cachedKeys);
        return;
      }
      try {
        const res = await fetch('/console/api/keys', { headers: getAuthHeaders() });
        if (res.status === 401) {
          document.getElementById('keys-tbody').innerHTML = `<tr><td colspan="9" style="text-align: center; color: var(--danger); padding: 32px;">${t('keys.authFailedHint')}</td></tr>`;
          return;
        }
        if (!res.ok) return;
        const data = await res.json();
        cachedKeys = (data.keys || []).map(normalizeKey);
        cachedKeysLoadedAt = Date.now();
        persistKeysToStorage(cachedKeys);
        renderKeysTable(cachedKeys);
        updateStatsKeyDropdown(cachedKeys);
        // Key 列表变更后同步流水筛选下拉（新建/删除 Key 后不用切 Tab 才生效）
        updateLogsKeySelect(cachedEvents);
      } catch (err) {
        console.warn('Fetch keys error:', err.message);
      }
    }

    function renderKeysTable(keys) {
      document.getElementById('table-keys-count').innerText = t('keys.countKeysSuffix', { count: keys.length });
      const navBadge = document.getElementById('nav-badge-keys');
      if (navBadge) navBadge.innerText = keys.length;
      const tbody = document.getElementById('keys-tbody');
      if (keys.length === 0) {
        tbody.innerHTML = `<tr><td colspan="9" style="text-align: center; color: var(--text-dim); padding: 32px;">${t('keys.emptyState')}</td></tr>`;
        return;
      }
      let html = '';
      for (const rawK of keys) {
        const k = normalizeKey(rawK);
        const hardLimit = Number(k.hard_limit) || (k.daily_limit ? Math.floor(Number(k.daily_limit) * (Number(k.hard_ratio) || 1.2)) : 0);
        const usage = Number(k.today_usage) || 0;
        const ratio = hardLimit > 0 ? Math.max(0, Math.min(1, usage / hardLimit)) : 0;
        const barColorClass = k.is_blocked ? 'danger' : (ratio >= 0.85 ? 'warn' : '');

        let statusChip = `<span class="chip chip-gray">${t('keys.statusNoLimit')}</span>`;
        if (k.is_blocked) {
          statusChip = `<span class="chip chip-red">${t('keys.statusHardCapBlocked')}</span>`;
        } else if (k.enabled) {
          if (ratio >= 0.85) statusChip = `<span class="chip chip-yellow">${t('keys.statusNearLimit')}</span>`;
          else statusChip = `<span class="chip chip-green">${t('keys.statusRunningOk')}</span>`;
        }

        let modelHtml = `<span class="model-perm-badge all" title="${t('keys.allModelsAllowedTitle')}"><span class="perm-dot all"></span>${t('keys.modelsAllBadge')}</span>`;
        if (k.model_whitelist) {
          const list = parseModelList(k.model_whitelist);
          if (list.length === 1) {
            const pInfo = getModelProviderInfo(list[0]);
            modelHtml = `<span class="model-perm-badge limited" title="[${escapeHtml(pInfo.label)}] ${escapeHtml(list[0])}"><span class="perm-dot limited"></span>${escapeHtml(list[0])} <small style="opacity:0.75;font-size:10px;">(${escapeHtml(pInfo.label)})</small></span>`;
          } else if (list.length > 1) {
            const titleWithProv = list.map(m => `[${getModelProviderInfo(m).label}] ${m}`).join(', ');
            modelHtml = `<span class="model-perm-badge limited" title="${escapeHtml(titleWithProv)}"><span class="perm-dot limited"></span>${t('keys.allowedModelsCount', { count: list.length })}</span>`;
          }
        }

        const cacheRate = k.cache_hit_rate > 0 ? `<span class="chip-cache">${k.cache_hit_rate}%</span>` : '<span class="chip-cache zero">0%</span>';

        let progressHtml = '';
        if (hardLimit > 0) {
          const pct = Math.max(0, Math.min(100, Math.round(ratio * 100)));
          progressHtml = `
            <div class="usage-progress-wrap" title="${t('keys.todayUsageProgressTitle', { pct: pct, value: formatTokens(k.daily_limit || 0), value2: formatTokens(hardLimit) })}">
              <div class="usage-progress-bar">
                <div class="usage-progress-fill ${barColorClass}" style="width:${pct}%;"></div>
              </div>
            </div>`;
        } else {
          progressHtml = `
            <div class="usage-progress-wrap" title="${t('keys.noDailyLimitTitle')}">
              <div class="usage-progress-bar unlimited"></div>
            </div>`;
        }

        const keyCopyBtn = `
          <button class="key-action-icon-btn" onclick="copyFullKey(${k.key_id})" title="${t('keys.copyFullKeyTitle')}">
            <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg>
          </button>`;

        const pctNum = hardLimit > 0 ? Math.max(0, Math.min(100, Math.round(ratio * 100))) : 0;
        const limitTag = hardLimit > 0 ? `<span style="color:${pctNum >= 85 ? 'var(--warning)' : 'var(--text-dim)'};">${pctNum}%</span>` : `<span style="color:var(--text-dim); opacity:0.8;">${t('keys.unlimitedLabel')}</span>`;

        const limitDisplay = k.daily_limit > 0
          ? (k.enabled ? formatTokens(k.daily_limit) : `<span class="cell-unlimited" style="color:var(--text-dim);" title="${t('keys.dailyLimitHardCapOffHint')}">${t('keys.dailyLimitNotEnabledSuffix', { value: formatTokens(k.daily_limit) })}</span>`)
          : `<span class="cell-unlimited">${t('keys.unlimitedLabel')}</span>`;
        const rpmDisplay = k.rpm_limit > 0 ? `${k.rpm_limit} RPM` : `<span class="cell-unlimited">${t('keys.unlimitedLabel')}</span>`;

        html += `<tr>
          <td>
            <div class="key-display-row">
              <div class="key-display" style="user-select:all; cursor:pointer;" title="${t('keys.fullKeyCopyHint')}">${escapeHtml(k.display_key)}</div>
              ${keyCopyBtn}
            </div>
            <div class="key-alias-row">
              <span class="key-alias ${k.key_alias ? '' : 'empty'}">${escapeHtml(k.key_alias || t('keys.aliasEmpty'))}</span>
              <button class="alias-quick-edit-btn" onclick="openRenameModal(${k.key_id})" title="${t('keys.renameAliasButtonTitle')}">
                <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z"/></svg>
              </button>
            </div>
          </td>
          <td>${modelHtml}</td>
          <td>
            <div class="key-usage-cell">
              <div class="key-usage-primary">
                <span class="key-usage-tokens">${formatTokens(k.today_usage)}</span>
                <span class="key-usage-requests">${t('stats.requestCountSuffix', { value: (k.today_requests || 0).toLocaleString(LOCALE) })}</span>
              </div>
              <div class="key-usage-sub">
                <span class="key-usage-alltime">${t('keys.allTimeUsageLabel', { value: formatTokens(k.all_time_usage || 0) })}</span>
                ${limitTag}
              </div>
              ${progressHtml}
            </div>
          </td>
          <td class="num">${cacheRate}</td>
          <td class="num" style="color:#34d399; font-weight:500;">${formatCost(k.cost_usd)}</td>
          <td class="num">${limitDisplay}</td>
          <td class="num">${rpmDisplay}</td>
          <td>${statusChip}</td>
          <td style="text-align: right;">
            <div class="btn-action-group">
              <button class="btn btn-secondary btn-sm" onclick="openEditModal(${k.key_id})">${t('keys.configureButton')}</button>
              <button class="btn btn-secondary btn-sm" onclick="rotateKey(${k.key_id})">${t('keys.rotateButton')}</button>
              <button class="btn btn-danger btn-sm" onclick="deleteKey(${k.key_id})">${t('quotas.actionDelete')}</button>
            </div>
          </td>
        </tr>`;
      }
      tbody.innerHTML = html;
    }

    // ---- 快捷修改别名 ----

    async function saveKeyRule() {
      if (!currentKeyId) return;
      const alias = document.getElementById('edit-alias').value.trim();
      let enabled = document.getElementById('edit-enabled').checked;
      const limit = Number(document.getElementById('edit-limit').value) || 0;
      const ratio = Number(document.getElementById('edit-ratio').value) || 1.2;
      const rpm = Number(document.getElementById('edit-rpm').value) || 0;

      // 联动保护：如果输入了每日限额 > 0，但未勾选启用开关，智能自动开启并同步 UI
      if (limit > 0 && !enabled) {
        enabled = true;
        const toggle = document.getElementById('edit-enabled');
        if (toggle) toggle.checked = true;
      }

      try {
        const resAlias = await fetch(`/console/api/keys/${currentKeyId}/alias`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json', ...getAuthHeaders() },
          body: JSON.stringify({ alias })
        });
        if (!resAlias.ok) {
          const errData = await resAlias.json().catch(() => ({}));
          throw new Error(errData.error || t('editModal.saveAliasFailed'));
        }

        const resQuota = await fetch(`/console/api/keys/${currentKeyId}/quota`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json', ...getAuthHeaders() },
          body: JSON.stringify({
            enabled,
            daily_limit: limit,
            hard_ratio: ratio,
            rpm_limit: rpm,
            model_whitelist: currentSelectedModels
          })
        });
        if (!resQuota.ok) {
          const errData = await resQuota.json().catch(() => ({}));
          throw new Error(errData.error || t('editModal.saveQuotaFailed'));
        }

        invalidateKeysFrontendCache();
        closeModal('edit-modal');
        showToast(t('editModal.savedToast'), 'success');
        manualRefresh();
      } catch (err) {
        showToast(`${t('editModal.saveFailedPrefix')} ` + err.message, 'error');
      }
    }

    // ---- 新建与删除 Key ----
    function openCreateModal() {
      document.getElementById('create-form').style.display = 'block';
      document.getElementById('create-result').style.display = 'none';
      document.getElementById('create-alias').value = '';
      document.getElementById('create-custom-key').value = '';
      document.getElementById('create-enabled').checked = false;
      document.getElementById('create-limit').value = '';
      toggleCreateLimit();
      openModal('create-modal');
    }
    function toggleCreateLimit() {
      document.getElementById('create-limit-group').style.display = document.getElementById('create-enabled').checked ? 'block' : 'none';
    }
    async function submitCreateKey() {
      const alias = document.getElementById('create-alias').value.trim();
      const apiKey = document.getElementById('create-custom-key').value.trim();
      const enabled = document.getElementById('create-enabled').checked;
      const limit = Number(document.getElementById('create-limit').value) || 0;
      try {
        const res = await fetch('/console/api/keys', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', ...getAuthHeaders() },
          body: JSON.stringify({ alias, api_key: apiKey, enabled, daily_limit: limit })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || t('keysModal.createFailed'));
        invalidateKeysFrontendCache();
        createdKeyRaw = data.key?.api_key || data.api_key;
        if (!createdKeyRaw) throw new Error(t('keysModal.createNoSecretError'));
        document.getElementById('created-key-val').innerText = createdKeyRaw;
        document.getElementById('create-form').style.display = 'none';
        document.getElementById('create-result').style.display = 'block';
        manualRefresh();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }
    function copyCreatedKey() {
      if (!createdKeyRaw) return;
      navigator.clipboard.writeText(createdKeyRaw).then(() => showToast(t('keys.copiedToast'), 'success'));
    }
    function finishCreateKey() {
      closeModal('create-modal');
    }
    async function rotateKey(keyId) {
      if (!confirm(t('keysModal.rotateConfirm'))) return;
      try {
        const res = await fetch(`/console/api/keys/${keyId}/rotate`, { method: 'POST', headers: getAuthHeaders() });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || t('keysModal.rotateFailed'));
        invalidateKeysFrontendCache();
        createdKeyRaw = data.key?.api_key;
        if (!createdKeyRaw) throw new Error(t('keysModal.rotateNoSecretError'));
        document.getElementById('created-key-val').innerText = createdKeyRaw;
        document.getElementById('create-form').style.display = 'none';
        document.getElementById('create-result').style.display = 'block';
        openModal('create-modal');
        manualRefresh();
      } catch (err) { showToast(`${t('keysModal.rotateFailedPrefix')} ` + err.message, 'error'); }
    }

    async function deleteKey(keyId) {
      if (!confirm(t('keysModal.deleteConfirm'))) return;
      try {
        const res = await fetch(`/console/api/keys/${keyId}`, {
          method: 'DELETE',
          headers: getAuthHeaders()
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || t('keysModal.deleteFailed'));
        invalidateKeysFrontendCache();
        showToast(t('keysModal.deleteSuccess'), 'success');
        manualRefresh();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }
