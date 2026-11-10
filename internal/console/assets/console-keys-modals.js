/**
 * Gate Admin Console - Key 弹窗
 * 从 console-keys.js 拆出：改名/编辑弹窗、服务商标签与模型标签渲染。
 */

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

    function getModelProviderInfo(modelName) {
      if (!modelName) return { key: 'upstream', label: '上游', color: '#64748b', bg: 'rgba(100,116,139,0.1)' };
      const raw = String(modelName).trim();
      const lower = raw.toLowerCase();

      if (Array.isArray(cachedUpstreamModelsList)) {
        const found = cachedUpstreamModelsList.find(item => {
          const id = typeof item === 'string' ? item : item.id;
          return id && id.toLowerCase() === lower;
        });
        if (found && typeof found === 'object' && found.provider) {
          return formatProviderLabel(found.provider);
        }
      }

      if (lower.startsWith('workbuddy-cn/') || lower.startsWith('copilot-tencent/')) {
        return { key: 'workbuddy-cn', label: 'WorkBuddy (国内)', color: '#d97706', bg: 'rgba(245,158,11,0.1)' };
      }
      if (lower.startsWith('workbuddy/')) {
        return { key: 'workbuddy', label: 'WorkBuddy', color: '#d97706', bg: 'rgba(245,158,11,0.1)' };
      }
      if (lower.startsWith('qoder-cn/') || lower.startsWith('qoder/')) {
        return { key: 'qoder', label: 'Qoder', color: '#059669', bg: 'rgba(16,185,129,0.1)' };
      }
      if (lower.startsWith('zcode-cn/') || lower.startsWith('zcode/')) {
        return { key: 'zcode', label: 'ZCode', color: '#0284c7', bg: 'rgba(2,132,199,0.1)' };
      }
      if (lower.includes('minimax') || lower.includes('abab')) {
        return { key: 'minimax', label: 'MiniMax', color: '#ec4899', bg: 'rgba(236,72,153,0.1)' };
      }
      if (lower.includes('claude')) {
        return { key: 'claude', label: 'Anthropic', color: '#8b5cf6', bg: 'rgba(139,92,246,0.1)' };
      }
      if (lower.startsWith('antigravity/') || lower.includes('argon') || lower.startsWith('gemini-4') || lower.startsWith('gemini-3.8') || lower.startsWith('gemini-3.5') || lower.startsWith('gemini-3.1') || lower.startsWith('gemini-3-')) {
        return { key: 'antigravity', label: '反重力 Antigravity', color: '#6366f1', bg: 'rgba(99,102,241,0.1)' };
      }
      if (lower.includes('gemini')) {
        return { key: 'gemini', label: 'Google Gemini', color: '#3b82f6', bg: 'rgba(59,130,246,0.1)' };
      }
      if (lower.includes('gpt') || lower.includes('o1') || lower.includes('o3') || lower.includes('codex')) {
        return { key: 'openai', label: 'OpenAI', color: '#10b981', bg: 'rgba(16,185,129,0.1)' };
      }
      if (lower.includes('deepseek')) {
        return { key: 'deepseek', label: 'DeepSeek', color: '#2563eb', bg: 'rgba(37,99,235,0.1)' };
      }
      if (lower.includes('glm')) {
        return { key: 'zhipu', label: '智谱 GLM', color: '#0284c7', bg: 'rgba(2,132,199,0.1)' };
      }
      if (lower.includes('qwen')) {
        return { key: 'qwen', label: '通义千问', color: '#059669', bg: 'rgba(16,185,129,0.1)' };
      }
      if (lower.includes('grok') || lower.includes('xai')) {
        return { key: 'xai', label: 'xAI', color: '#475569', bg: 'rgba(71,85,105,0.1)' };
      }
      if (lower.includes('step')) {
        return { key: 'stepfun', label: '阶跃星辰', color: '#0891b2', bg: 'rgba(8,145,178,0.1)' };
      }
      return { key: 'upstream', label: '直连上游', color: '#64748b', bg: 'rgba(100,116,139,0.1)' };
    }

    function formatProviderLabel(rawProvider) {
      const p = String(rawProvider || '').toLowerCase().trim();
      switch (p) {
        case 'minimax':
          return { key: 'minimax', label: 'MiniMax', color: '#ec4899', bg: 'rgba(236,72,153,0.12)' };
        case 'workbuddy-cn':
          return { key: 'workbuddy-cn', label: 'WorkBuddy (国内)', color: '#d97706', bg: 'rgba(245,158,11,0.12)' };
        case 'workbuddy':
        case 'workbuddy-intl':
          return { key: 'workbuddy', label: 'WorkBuddy', color: '#d97706', bg: 'rgba(245,158,11,0.12)' };
        case 'claude':
        case 'anthropic':
          return { key: 'claude', label: 'Anthropic', color: '#8b5cf6', bg: 'rgba(139,92,246,0.12)' };
        case 'openai':
        case 'codex':
          return { key: 'openai', label: 'OpenAI', color: '#10b981', bg: 'rgba(16,185,129,0.12)' };
        case 'antigravity':
          return { key: 'antigravity', label: '反重力 Antigravity', color: '#6366f1', bg: 'rgba(99,102,241,0.12)' };
        case 'gemini':
          return { key: 'gemini', label: 'Google Gemini', color: '#3b82f6', bg: 'rgba(59,130,246,0.12)' };
        case 'deepseek':
          return { key: 'deepseek', label: 'DeepSeek', color: '#2563eb', bg: 'rgba(37,99,235,0.12)' };
        case 'qoder':
        case 'qoder-cn':
          return { key: 'qoder', label: 'Qoder', color: '#059669', bg: 'rgba(16,185,129,0.12)' };
        case 'zcode':
        case 'zcode-cn':
        case 'zhipu':
          return { key: 'zcode', label: 'ZCode / 智谱', color: '#0284c7', bg: 'rgba(2,132,199,0.12)' };
        case 'qwen':
          return { key: 'qwen', label: '通义千问', color: '#059669', bg: 'rgba(16,185,129,0.12)' };
        case 'stepfun':
          return { key: 'stepfun', label: '阶跃星辰', color: '#0891b2', bg: 'rgba(8,145,178,0.12)' };
        case 'opencode':
          return { key: 'opencode', label: 'OpenCode', color: '#0284c7', bg: 'rgba(2,132,199,0.12)' };
        default:
          return { key: p || 'upstream', label: rawProvider || '上游', color: '#64748b', bg: 'rgba(100,116,139,0.12)' };
      }
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
        const pInfo = getModelProviderInfo(m);
        const provBadge = `<span style="font-size:9.5px;padding:1px 5px;border-radius:4px;background:${pInfo.bg};color:${pInfo.color};font-weight:600;margin-right:5px;border:1px solid ${pInfo.color}33;">${escapeHtml(pInfo.label)}</span>`;
        html += `<span class="tag-item" style="display:inline-flex;align-items:center;">${provBadge}<span>${escapeHtml(m)}</span><span class="tag-close" onclick="removeModel(${idx})">&times;</span></span>`;
      });
      box.innerHTML = html;
    }
    function removeModel(idx) {
      currentSelectedModels.splice(idx, 1);
      renderModelTags();
      renderModelPickerList();
    }

    // ---- 现代 Aozora UI 模型搜索与选择下拉交互 ----
