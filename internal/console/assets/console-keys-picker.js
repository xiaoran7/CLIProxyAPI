/**
 * Gate Admin Console - 上游模型选择器
 * 从 console-keys.js 拆出：上游模型拉取、下拉浮层渲染、搜索与选中。
 */

    let cachedUpstreamModelsList = null;
    let isFetchingUpstreamModels = false;

    // ---- 现代 Aozora UI 模型搜索与选择下拉交互 ----

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
        const modelItems = [];
        const seen = new Set();
        for (const m of rawList) {
          const id = typeof m === 'string' ? m : (m.id || m.name);
          if (id && !seen.has(id.toLowerCase())) {
            seen.add(id.toLowerCase());
            modelItems.push({
              id: id,
              provider: (typeof m === 'object' && m.provider) ? m.provider : ''
            });
          }
        }
        modelItems.sort((a, b) => a.id.localeCompare(b.id));

        cachedUpstreamModelsList = modelItems;
        if (label) {
          label.innerText = t('keys.modelPickerPlaceholder', { count: modelItems.length });
        }
        renderModelPickerList();
        if (showSuccessToast) {
          showToast(t('keys.modelSyncSuccess', { count: modelItems.length }), 'success');
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
        matched = cachedUpstreamModelsList.filter(item => {
          const id = (typeof item === 'string' ? item : item.id).toLowerCase();
          const prov = (typeof item === 'object' && item.provider ? item.provider : '').toLowerCase();
          return id.includes(query) || prov.includes(query);
        });
      }

      let html = '';
      if (matched.length === 0) {
        html += `<div class="model-picker-empty">${t('editModal.modelPickerNoMatch')}</div>`;
      } else {
        matched.forEach(item => {
          const m = typeof item === 'string' ? item : item.id;
          const prov = typeof item === 'object' && item.provider ? item.provider : '';
          const pInfo = prov ? formatProviderLabel(prov) : getModelProviderInfo(m);
          const isAdded = currentSelectedModels.includes(m.toLowerCase());
          const provBadge = `<span style="font-size:10px;padding:2px 7px;border-radius:6px;background:${pInfo.bg};color:${pInfo.color};font-weight:600;border:1px solid ${pInfo.color}33;flex-shrink:0;">${escapeHtml(pInfo.label)}</span>`;
          html += `
            <div class="model-picker-item ${isAdded ? 'is-selected' : ''}" onclick="selectModelFromPicker('${escapeHtml(m)}')">
              <div class="model-picker-item-left" style="display:flex;align-items:center;min-width:0;flex:1;overflow:hidden;">
                <span class="perm-dot ${isAdded ? 'limited' : 'all'}"></span>
                <span class="model-picker-item-name" style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${escapeHtml(m)}">${escapeHtml(m)}</span>
              </div>
              <div class="model-picker-item-right" style="display:flex;align-items:center;gap:6px;flex-shrink:0;">
                ${provBadge}
                ${isAdded ? `<span class="model-picker-badge-added">${t('editModal.modelBadgeInWhitelist')}</span>` : `<span class="model-picker-badge-add">${t('editModal.modelBadgeAdd')}</span>`}
              </div>
            </div>
          `;
        });
      }

      // 如果用户输入了自定义模型ID，且上游列表中未精确匹配，支持一键添加自定义模型
      if (query && !cachedUpstreamModelsList.some(item => (typeof item === 'string' ? item : item.id).toLowerCase() === query)) {
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

