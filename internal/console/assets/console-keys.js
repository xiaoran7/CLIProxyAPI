/**
 * Gate Admin Console - Keys & Pricing Management Module
 * 包含 API Key CRUD (创建/轮换/别名/删除/模型权限) 及模型定价规则配置与渲染
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
        cachedKeys = data.keys || [];
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
      for (const k of keys) {
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
            modelHtml = `<span class="model-perm-badge limited" title="${escapeHtml(list[0])}"><span class="perm-dot limited"></span>${escapeHtml(list[0])}</span>`;
          } else if (list.length > 1) {
            modelHtml = `<span class="model-perm-badge limited" title="${escapeHtml(list.join(', '))}"><span class="perm-dot limited"></span>${t('keys.allowedModelsCount', { count: list.length })}</span>`;
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
    let currentRenameKeyId = null;
    function openRenameModal(keyId) {
      const item = cachedKeys.find(k => k.key_id === keyId);
      if (!item) return;
      currentRenameKeyId = keyId;
      document.getElementById('rename-modal-title').innerText = t('keys.renameModalTitle', { display_key: item.display_key });
      const input = document.getElementById('rename-alias-input');
      input.value = item.key_alias || '';
      openModal('rename-modal');
      setTimeout(() => { input.focus(); input.select(); }, 50);
    }

    document.getElementById('rename-alias-input')?.addEventListener('keydown', e => {
      if (e.key === 'Enter') {
        e.preventDefault();
        submitRenameKey();
      }
    });

    async function submitRenameKey() {
      if (!currentRenameKeyId) return;
      const input = document.getElementById('rename-alias-input');
      const alias = input.value.trim();
      const saveBtn = document.getElementById('rename-save-btn');
      if (saveBtn) saveBtn.disabled = true;

      try {
        const res = await fetch(`/console/api/keys/${currentRenameKeyId}/alias`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json', ...getAuthHeaders() },
          body: JSON.stringify({ alias })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || t('keysModal.renameUpdateFailed'));

        // 本地更新缓存以即时反馈
        const item = cachedKeys.find(k => k.key_id === currentRenameKeyId);
        if (item) item.key_alias = alias;
        persistKeysToStorage(cachedKeys);
        clearLogsQueryCache();
        renderKeysTable(cachedKeys);
        updateLogsKeySelect(cachedEvents);
        renderEventsTable(cachedEvents, cachedEventsTotal);

        closeModal('rename-modal');
        showToast(alias ? t('keys.aliasUpdatedToast', { alias: alias }) : t('keysModal.renameCleared'), 'success');
      } catch (err) {
        showToast(`${t('keysModal.renameFailedPrefix')} ` + err.message, 'error');
      } finally {
        if (saveBtn) saveBtn.disabled = false;
      }
    }

    // ---- Key 规则编辑逻辑 ----
    function openEditModal(keyId) {
      const item = cachedKeys.find(k => k.key_id === keyId);
      if (!item) return;
      currentKeyId = keyId;

      document.getElementById('edit-modal-title').innerText = t('keys.editKeyRuleTitle', { value: item.key_alias || item.display_key });
      const rawGroup = document.getElementById('edit-key-raw-group');
      const rawInput = document.getElementById('edit-key-raw-input');
      // 完整 Key 不在列表里下发，改为打开弹窗时按需拉取；
      // 拉取失败就只显示截断形，不阻塞规则编辑。
      rawGroup.style.display = 'none';
      rawInput.value = '';
      if (item.key_id != null) {
        fetchFullKey(item.key_id).then(raw => {
          if (currentKeyId !== keyId) return;
          if (raw) {
            rawInput.value = raw;
            rawGroup.style.display = 'block';
          }
        }).catch(() => {});
      }
      document.getElementById('edit-alias').value = item.key_alias || '';
      document.getElementById('edit-enabled').checked = Boolean(item.enabled);
      document.getElementById('edit-limit').value = item.daily_limit || '';
      document.getElementById('edit-ratio').value = item.hard_ratio || 1.2;
      document.getElementById('edit-rpm').value = item.rpm_limit || '';

      currentSelectedModels = parseModelList(item.model_whitelist);
      renderModelTags();
      // 打开 Modal 时重置并准备好上游模型下拉组件
      resetModelPicker();
      if (!cachedUpstreamModelsList) {
        fetchUpstreamModelsForPicker(false);
      } else {
        renderModelPickerList();
      }
      openModal('edit-modal');
    }

    function renderModelTags() {
      const box = document.getElementById('edit-tags-box');
      if (!box) return;
      if (currentSelectedModels.length === 0) {
        box.innerHTML = `<span class="tag-empty">${t('editModal.modelsEmptyTag')}</span>`;
        return;
      }
      let html = '';
      currentSelectedModels.forEach((m, idx) => {
        html += `<span class="tag-item"><span>${escapeHtml(m)}</span><span class="tag-close" onclick="removeModel(${idx})">&times;</span></span>`;
      });
      box.innerHTML = html;
    }
    function removeModel(idx) {
      currentSelectedModels.splice(idx, 1);
      renderModelTags();
      renderModelPickerList();
    }

    function setPreset(type) {
      if (type === 'all') currentSelectedModels = [];
      else if (type === 'flash') currentSelectedModels = ['gemini-2.5-flash', 'gemini-2.0-flash', 'gpt-4o-mini', 'claude-3-5-haiku', 'deepseek-chat'];
      else if (type === 'claude') currentSelectedModels = ['claude-3-7-sonnet', 'claude-3-5-sonnet', 'claude-3-5-haiku'];
      else if (type === 'reasoning') currentSelectedModels = ['deepseek-reasoner', 'o1', 'o3-mini', 'claude-3-7-sonnet'];
      renderModelTags();
      renderModelPickerList();
    }
    function clearModels() {
      currentSelectedModels = [];
      renderModelTags();
      renderModelPickerList();
    }

    // ---- 现代 Aozora UI 模型搜索与选择下拉交互 ----
    let cachedUpstreamModelsList = null;
    let isFetchingUpstreamModels = false;

    function toggleModelPickerDropdown() {
      const container = document.getElementById('model-picker-container');
      if (!container) return;
      const isOpen = container.classList.contains('open');
      if (isOpen) {
        closeModelPickerDropdown();
      } else {
        openModelPickerDropdown();
      }
    }

    function openModelPickerDropdown() {
      const container = document.getElementById('model-picker-container');
      if (!container) return;
      container.classList.add('open');
      renderModelPickerList();
      const searchInput = document.getElementById('model-picker-search');
      if (searchInput) {
        setTimeout(() => searchInput.focus(), 60);
      }
      if (!cachedUpstreamModelsList && !isFetchingUpstreamModels) {
        fetchUpstreamModelsForPicker(false);
      }
    }

    function closeModelPickerDropdown() {
      const container = document.getElementById('model-picker-container');
      if (container) container.classList.remove('open');
    }

    function resetModelPicker() {
      closeModelPickerDropdown();
      const searchInput = document.getElementById('model-picker-search');
      if (searchInput) searchInput.value = '';
      const clearBtn = document.getElementById('model-picker-clear-btn');
      if (clearBtn) clearBtn.style.display = 'none';
      const label = document.getElementById('model-picker-label');
      if (label) {
        label.innerText = cachedUpstreamModelsList && cachedUpstreamModelsList.length > 0
          ? t('keys.modelPickerPlaceholder', { count: cachedUpstreamModelsList.length })
          : t('editModal.modelPickerPlaceholder');
      }
    }

    async function fetchUpstreamModelsForPicker(showSuccessToast = true) {
      if (isFetchingUpstreamModels) return;
      const btn = document.getElementById('btn-fetch-models');
      const listEl = document.getElementById('model-picker-list');
      const label = document.getElementById('model-picker-label');

      isFetchingUpstreamModels = true;
      if (btn) {
        btn.classList.add('loading');
        btn.disabled = true;
      }
      if (label && (!cachedUpstreamModelsList || cachedUpstreamModelsList.length === 0)) {
        label.innerText = t('editModal.modelPickerSyncing');
      }
      if (listEl && (!cachedUpstreamModelsList || cachedUpstreamModelsList.length === 0)) {
        listEl.innerHTML = `<div class="model-picker-empty">${t('editModal.modelPickerFetching')}</div>`;
      }

      try {
        const res = await fetch('/console/api/cpa/models', { headers: getAuthHeaders() });
        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          throw new Error(errData.error || `HTTP ${res.status}`);
        }
        const data = await res.json();
        const rawList = Array.isArray(data.models) ? data.models : (Array.isArray(data.data) ? data.data : []);
        const modelIds = rawList.map(m => typeof m === 'string' ? m : (m.id || m.name)).filter(Boolean);
        const uniqueModels = [...new Set(modelIds)].sort();

        cachedUpstreamModelsList = uniqueModels;
        if (label) {
          label.innerText = t('keys.modelPickerPlaceholder', { count: uniqueModels.length });
        }
        renderModelPickerList();
        if (showSuccessToast) {
          showToast(t('keys.modelSyncSuccess', { count: uniqueModels.length }), 'success');
        }
      } catch (err) {
        console.warn('Fetch upstream models failed:', err);
        if (listEl && (!cachedUpstreamModelsList || cachedUpstreamModelsList.length === 0)) {
          listEl.innerHTML = `<div class="model-picker-empty" style="color:var(--danger);">${t('keys.modelSyncFailed', { value: escapeHtml(err.message) })}</div>`;
        }
        showToast(`${t('editModal.modelPickerSyncToastPrefix')} ` + err.message, 'error');
      } finally {
        isFetchingUpstreamModels = false;
        if (btn) {
          btn.classList.remove('loading');
          btn.disabled = false;
        }
      }
    }

    function renderModelPickerList(filterText = '') {
      const listEl = document.getElementById('model-picker-list');
      if (!listEl) return;
      const query = (filterText || '').trim().toLowerCase();

      if (!cachedUpstreamModelsList) {
        listEl.innerHTML = `<div class="model-picker-empty">${t('editModal.modelPickerLoading')}</div>`;
        return;
      }

      if (cachedUpstreamModelsList.length === 0) {
        listEl.innerHTML = `<div class="model-picker-empty">${t('editModal.modelPickerEmptyUpstream')}</div>`;
        return;
      }

      let matched = cachedUpstreamModelsList;
      if (query) {
        matched = cachedUpstreamModelsList.filter(m => m.toLowerCase().includes(query));
      }

      let html = '';
      if (matched.length === 0) {
        html += `<div class="model-picker-empty">${t('editModal.modelPickerNoMatch')}</div>`;
      } else {
        matched.forEach(m => {
          const isAdded = currentSelectedModels.includes(m.toLowerCase());
          html += `
            <div class="model-picker-item ${isAdded ? 'is-selected' : ''}" onclick="selectModelFromPicker('${escapeHtml(m)}')">
              <div class="model-picker-item-left">
                <span class="perm-dot ${isAdded ? 'limited' : 'all'}"></span>
                <span class="model-picker-item-name" title="${escapeHtml(m)}">${escapeHtml(m)}</span>
              </div>
              <div class="model-picker-item-right">
                ${isAdded ? `<span class="model-picker-badge-added">${t('editModal.modelBadgeInWhitelist')}</span>` : `<span class="model-picker-badge-add">${t('editModal.modelBadgeAdd')}</span>`}
              </div>
            </div>
          `;
        });
      }

      // 如果用户输入了自定义模型ID，且上游列表中未精确匹配，支持一键添加自定义模型
      if (query && !cachedUpstreamModelsList.some(m => m.toLowerCase() === query)) {
        html += `
          <div class="model-picker-custom-add" onclick="selectModelFromPicker('${escapeHtml(query)}')">
            <svg style="width:13px; height:13px; flex-shrink:0;" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M12 4v16m8-8H4"></path></svg>
            <span>${t('editModal.modelCustomAdd')} <strong>${escapeHtml(query)}</strong></span>
          </div>
        `;
      }

      listEl.innerHTML = html;
    }

    function filterUpstreamModels(val) {
      const clearBtn = document.getElementById('model-picker-clear-btn');
      if (clearBtn) {
        clearBtn.style.display = val ? 'inline-block' : 'none';
      }
      renderModelPickerList(val);
    }

    function clearModelPickerSearch(e) {
      if (e) e.stopPropagation();
      const searchInput = document.getElementById('model-picker-search');
      if (searchInput) {
        searchInput.value = '';
        searchInput.focus();
      }
      const clearBtn = document.getElementById('model-picker-clear-btn');
      if (clearBtn) clearBtn.style.display = 'none';
      renderModelPickerList('');
    }

    function selectModelFromPicker(modelName) {
      const val = String(modelName || '').trim().toLowerCase();
      if (!val) return;
      if (!currentSelectedModels.includes(val)) {
        currentSelectedModels.push(val);
        renderModelTags();
        showToast(t('keys.modelAddedToast', { val: val }), 'info');
      } else {
        // 已存在时点击则移除（切换行为）
        const idx = currentSelectedModels.indexOf(val);
        if (idx !== -1) {
          currentSelectedModels.splice(idx, 1);
          renderModelTags();
          showToast(t('keys.modelRemovedToast', { val: val }), 'info');
        }
      }
      renderModelPickerList(document.getElementById('model-picker-search')?.value || '');
    }

    // 全局点击遮罩外关闭下拉
    document.addEventListener('click', (e) => {
      const container = document.getElementById('model-picker-container');
      if (container && !container.contains(e.target)) {
        closeModelPickerDropdown();
      }
    });

    // 输入每日限额时智能自动勾选启用硬顶开关
    document.getElementById('edit-limit')?.addEventListener('input', (e) => {
      const val = Number(e.target.value) || 0;
      if (val > 0) {
        const toggle = document.getElementById('edit-enabled');
        if (toggle && !toggle.checked) {
          toggle.checked = true;
        }
      }
    });

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

    // ---- 模型定价与倍率管理 (Pricing Rules Modal) ----
    let cachedModelPrices = [];

    function openPricesModal() {
      resetPriceForm();
      fetchModelPrices();
      openModal('prices-modal');
    }

    async function fetchModelPrices() {
      const secret = getAdminSecret();
      if (!secret) return;
      try {
        const res = await fetch('/console/api/model-prices', { headers: getAuthHeaders() });
        if (!res.ok) return;
        const data = await res.json();
        cachedModelPrices = data.prices || [];
        renderPricesTable(cachedModelPrices);
      } catch (err) {
        console.warn('Model prices fetch error:', err.message);
      }
    }

    function renderPricesTable(prices) {
      const tbody = document.getElementById('prices-tbody');
      if (!tbody) return;
      if (!prices || prices.length === 0) {
        tbody.innerHTML = `<tr><td colspan="6" style="text-align: center; color: var(--text-dim); padding: 24px;">${t('pricesModal.emptyState')}</td></tr>`;
        return;
      }
      let html = '';
      for (const p of prices) {
        const pat = p.pattern || p.model_pattern || '';
        html += `<tr>
          <td><code style="font-family:var(--font-mono); color:var(--accent);">${escapeHtml(pat)}</code></td>
          <td class="num">$${Number(p.prompt_price || 0).toFixed(2)}</td>
          <td class="num">$${Number(p.completion_price || 0).toFixed(2)}</td>
          <td class="num">$${Number(p.cache_read_price || 0).toFixed(3)}</td>
          <td class="num"><span class="chip chip-gray">${Number(p.multiplier || 1.0).toFixed(1)}x</span></td>
          <td style="text-align: right;">
            <button class="btn btn-secondary btn-sm" onclick="editPriceRule(${p.id})">${t('quotas.actionEdit')}</button>
            <button class="btn btn-danger btn-sm" style="margin-left:4px;" onclick="deletePriceRule(${p.id})">${t('quotas.actionDelete')}</button>
          </td>
        </tr>`;
      }
      tbody.innerHTML = html;
    }

    function resetPriceForm() {
      document.getElementById('price-form-id').value = '';
      document.getElementById('price-form-title').innerText = t('keys.addEditPricingRuleTitle');
      document.getElementById('price-pattern').value = '';
      document.getElementById('price-prompt').value = '';
      document.getElementById('price-completion').value = '';
      document.getElementById('price-cache').value = '';
      document.getElementById('price-multiplier').value = '1.0';
    }

    function editPriceRule(id) {
      const p = cachedModelPrices.find(item => item.id === id);
      if (!p) return;
      const pat = p.pattern || p.model_pattern || '';
      document.getElementById('price-form-id').value = p.id;
      document.getElementById('price-form-title').innerText = t('keys.editPricingRuleTitle', { pat: pat });
      document.getElementById('price-pattern').value = pat;
      document.getElementById('price-prompt').value = p.prompt_price;
      document.getElementById('price-completion').value = p.completion_price;
      document.getElementById('price-cache').value = p.cache_read_price;
      document.getElementById('price-multiplier').value = p.multiplier;
    }

    async function saveModelPrice() {
      const id = document.getElementById('price-form-id').value;
      const pattern = document.getElementById('price-pattern').value.trim();
      if (!pattern) {
        showToast(t('pricesModal.patternRequired'), 'error');
        return;
      }
      const promptPrice = Number(document.getElementById('price-prompt').value) || 0;
      const completionPrice = Number(document.getElementById('price-completion').value) || 0;
      const cachePrice = Number(document.getElementById('price-cache').value) || 0;
      const multiplier = Number(document.getElementById('price-multiplier').value) || 1.0;

      try {
        const payload = {
          id: id ? Number(id) : undefined,
          pattern,
          model_pattern: pattern,
          prompt_price: promptPrice,
          completion_price: completionPrice,
          cache_read_price: cachePrice,
          multiplier
        };
        const res = await fetch('/console/api/model-prices', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', ...getAuthHeaders() },
          body: JSON.stringify(payload)
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || t('pricesModal.saveFailed'));
        showToast(t('pricesModal.saveSuccess'), 'success');
        resetPriceForm();
        fetchModelPrices();
        fetchStatsData();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function deletePriceRule(id) {
      if (!confirm(t('pricesModal.deleteConfirm'))) return;
      try {
        const res = await fetch(`/console/api/model-prices/${id}`, {
          method: 'DELETE',
          headers: getAuthHeaders()
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || t('keysModal.deleteFailed'));
        showToast(t('pricesModal.deleteSuccess'), 'success');
        fetchModelPrices();
        fetchStatsData();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }
