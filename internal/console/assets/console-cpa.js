(() => {
  const nav = document.getElementById('main-nav-tabs');
  const workspace = document.getElementById('workspace');
  if (!nav || !workspace) return;

  const pages = [
    // 明确职责边界：Gate 完全聚焦流量统计、调度与审计，CPA 页面（日志、配置、账号接入、提供商）
    // 统一通过底部「原生面板 ↗」安全跳转至受保护的原生 CPA 控制台，不再向 Gate 侧边栏和工作区注入内嵌页面。
  ];
  for (const [id, title, eyebrow, description, iconSvg, insertAfterId, addDividerAfter] of pages) {
    pageMeta[id] = { title, eyebrow, description };
    const btnHtml = `<button class="nav-tab-btn cpa-nav" id="tab-btn-${id}" type="button" data-cpa-nav="${id}">${iconSvg}<span>${title}</span></button>${addDividerAfter ? '<div class="sidebar-section-divider"></div>' : ''}`;
    const anchorBtn = insertAfterId ? document.getElementById(insertAfterId) : null;
    if (anchorBtn && anchorBtn.parentNode === nav) {
      anchorBtn.insertAdjacentHTML('afterend', btnHtml);
    } else {
      nav.insertAdjacentHTML('beforeend', btnHtml);
    }
    workspace.insertAdjacentHTML('beforeend', `<div class="tab-page cpa-workspace" id="tab-page-${id}"></div>`);
  }

  const $ = id => document.getElementById(id);
  const escape = value => String(value ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);
  // 日志水位：24 小时档实测 42314 行，所以行数上限取 60000 覆盖全档位；
  // 单次 DOM 渲染上限取 8000 行——6MB 文本一次性塞进 <pre> 会卡死浏览器，
  // 超出部分逐段追加。两个上限服务不同目的，别合并。
  const LOG_LINES_LIMIT = 60000;
  const LOG_RENDER_LIMIT = 8000;
  const state = { yaml: '', yamlEtag: '', logAfter: Math.floor(Date.now() / 1000), logLines: [], logRangeHours: 1, logRenderLimit: LOG_RENDER_LIMIT, logTruncated: false, viewingErrorName: '', keyModels: [] };
  const button = (action, label, cls = 'btn btn-secondary btn-sm') => `<button class="${cls}" type="button" data-cpa-action="${action}">${label}</button>`;
  const CPA_ICONS = {
    warn: '<svg class="cpa-svg-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/><line x1="12" x2="12" y1="9" y2="13"/><line x1="12" x2="12.01" y1="17" y2="17"/></svg>',
    eye: '<svg class="cpa-svg-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>',
    copy: '<svg class="cpa-svg-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>'
  };

  // 现代定制浮动下拉组件与原生 select 双向绑定管理器
  const cpaDropdowns = new Map();
  function syncCpaDropdown(selectId, optionsOverride) {
    const select = $(selectId);
    if (!select) return;
    let entry = cpaDropdowns.get(selectId);
    if (!entry) {
      const container = document.createElement('div');
      container.className = 'dropdown';
      container.id = selectId + '-custom-dropdown';
      select.classList.add('cpa-select-hidden');
      select.parentNode.insertBefore(container, select);
      entry = { container, isOpen: false };
      cpaDropdowns.set(selectId, entry);

      container.addEventListener('click', e => {
        const trigger = e.target.closest('.dropdown-trigger');
        if (trigger) {
          e.stopPropagation();
          cpaDropdowns.forEach((val, id) => {
            if (id !== selectId && val.isOpen) {
              val.isOpen = false;
              val.container.classList.remove('open');
              val.container.querySelector('.dropdown-trigger')?.setAttribute('aria-expanded', 'false');
            }
          });
          entry.isOpen = !entry.isOpen;
          container.classList.toggle('open', entry.isOpen);
          trigger.setAttribute('aria-expanded', String(entry.isOpen));
          return;
        }
        const item = e.target.closest('.dropdown-item');
        if (item) {
          e.stopPropagation();
          const val = item.getAttribute('data-value');
          if (val !== select.value) {
            select.value = val;
            select.dispatchEvent(new Event('change', { bubbles: true }));
          }
          entry.isOpen = false;
          container.classList.remove('open');
          container.querySelector('.dropdown-trigger')?.setAttribute('aria-expanded', 'false');
          render();
        }
      });

      select.addEventListener('change', () => { render(); });
      document.addEventListener('click', e => {
        if (!container.contains(e.target) && entry.isOpen) {
          entry.isOpen = false;
          container.classList.remove('open');
          container.querySelector('.dropdown-trigger')?.setAttribute('aria-expanded', 'false');
        }
      });
    }

    function render() {
      const opts = optionsOverride || Array.from(select.options).map(o => ({ value: o.value, label: o.textContent }));
      const current = opts.find(o => String(o.value) === String(select.value)) || opts[0];
      const label = current ? current.label : t('cpa.pleaseSelectLabel');
      entry.container.innerHTML = `
        <button type="button" class="dropdown-trigger" aria-label="${escape(label)}" aria-expanded="${entry.isOpen}" aria-haspopup="listbox">
          <span class="dropdown-label">${escape(label)}</span>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="6 9 12 15 18 9"></polyline>
          </svg>
        </button>
        <div class="dropdown-menu" role="listbox">
          ${opts.map(o => {
            const isSelected = String(o.value) === String(select.value);
            return `
              <div class="dropdown-item ${isSelected ? 'selected' : ''}" role="option" aria-selected="${isSelected}" data-value="${escape(o.value)}">
                <span>${escape(o.label)}</span>
                <svg class="check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                  <polyline points="20 6 9 17 4 12"></polyline>
                </svg>
              </div>`;
          }).join('')}
        </div>
      `;
    }

    render();
  }

  const settings = [
    ['request-retry', t('cpa.settingRequestRetry'), 'number'], ['max-retry-interval', t('cpa.settingMaxRetryInterval'), 'number'],
    ['proxy-url', t('cpa.settingProxyUrl'), 'text'], ['request-log', t('cpa.settingRequestLog'), 'boolean'],
    ['logging-to-file', t('cpa.settingLoggingToFile'), 'boolean'], ['ws-auth', t('cpa.settingWsAuth'), 'boolean'],
    ['usage-statistics-enabled', t('cpa.settingUsageStatistics'), 'boolean'],
    ['quota-exceeded/switch-project', t('cpa.settingQuotaSwitchProject'), 'boolean'],
    ['quota-exceeded/switch-preview-model', t('cpa.settingQuotaSwitchPreviewModel'), 'boolean'],
    ['error-logs-max-files', t('cpa.settingErrorLogsMaxFiles'), 'number']
  ];
  const configPage = $('tab-page-cpa-config');
  if (configPage) configPage.innerHTML = `
    <section class="cpa-panel" id="cpa-routing-panel"><h3>${t('cpa.routingTitle')}</h3><p>${t('cpa.routingDesc')}</p>
      <div class="cpa-grid">
        <div class="cpa-field">
          <label for="cpa-routing-strategy">${t('cpa.routingStrategyLabel')}</label>
          <select id="cpa-routing-strategy">
            <option value="round-robin">${t('cpa.strategyRoundRobin')}</option>
            <option value="weighted-round-robin">${t('cpa.strategyWeightedRoundRobin')}</option>
            <option value="fill-first">${t('cpa.strategyFillFirst')}</option>
            <option value="smart">${t('cpa.strategySmartQuota')}</option>
            <option value="pace">${t('cpa.strategyPaceQuota')}</option>
          </select>
        </div>
        <div class="cpa-field cpa-field-checkbox" style="align-self:end;margin-bottom:10px;">
          <label><input type="checkbox" id="cpa-routing-affinity"/> ${t('cpa.routingAffinityLabel')}</label>
        </div>
      </div>
      <div class="cpa-grid">
        <div class="cpa-field">
          <label for="cpa-routing-ttl">${t('cpa.routingTtlLabel')}</label>
          <input type="number" id="cpa-routing-ttl" min="0" placeholder="${t('cpa.ttlPlaceholderExample')}"/>
        </div>
        <div class="cpa-actions" style="align-self:end;margin-bottom:8px;">
          ${button('routing-load', t('cpa.refreshRoutingConfig'))}
          ${button('routing-save', t('cpa.saveRoutingConfig'), 'btn btn-primary btn-sm')}
        </div>
      </div>
      <div class="cpa-status" id="cpa-routing-status"></div>
    </section>
    <section class="cpa-panel"><h3>${t('cpa.commonSettingsTitle')}</h3><p>${t('cpa.commonSettingsDesc')}</p>
      <div class="cpa-actions">${button('settings-load', t('cpa.refreshSettings'))}${button('version-check', t('cpa.checkNewVersion'))}</div><div class="cpa-status" id="cpa-version-status"></div>
      <div id="cpa-settings-rows">${settings.map(([key, label, type]) => `
        <div class="cpa-setting-row" data-cpa-setting="${key}">
          <label>${label}</label>
          ${type === 'boolean' ? `
            <label class="cpa-toggle-wrapper">
              <input class="cpa-setting-value cpa-toggle-input" type="checkbox"/>
              <span class="cpa-toggle-track"><span class="cpa-toggle-thumb"></span></span>
              <span class="cpa-toggle-state-text">${t('cpa.closeLabel')}</span>
            </label>
          ` : `<input class="cpa-setting-value" type="${type}"/>`}
          ${button('setting-save', t('cpa.saveButton'))}
        </div>
      `).join('')}</div>
      <div class="cpa-status" id="cpa-settings-status"></div>
    </section>
    <section class="cpa-panel">
      <div style="display:flex;justify-content:space-between;align-items:flex-start;flex-wrap:wrap;gap:12px;margin-bottom:12px;">
        <div>
          <h3>${t('cpa.availableModelsTitle')}</h3>
          <p style="margin-bottom:0;">${t('cpa.availableModelsDesc')}</p>
        </div>
        <div id="cpa-key-models-count" style="font-size:12px;color:#50708d;font-weight:600;align-self:center;"></div>
      </div>
      <div style="display:flex;flex-wrap:wrap;align-items:flex-end;gap:12px;margin-bottom:14px;">
        <div class="cpa-field" style="flex:1;min-width:240px;margin:0;">
          <label for="cpa-model-key">API Key</label>
          <select id="cpa-model-key"></select>
        </div>
        <div class="cpa-field" style="flex:1.2;min-width:240px;margin:0;">
          <label for="cpa-key-models-search">${t('cpa.modelFilterLabel')}</label>
          <input type="search" id="cpa-key-models-search" placeholder="${t('cpa.modelSearchPlaceholder')}"/>
        </div>
        <div class="cpa-actions" style="margin:0;padding-bottom:1px;">
          ${button('models-load-keys', t('cpa.refreshKeys'))}
          ${button('models-load', t('cpa.requeryModels'), 'btn btn-primary btn-sm')}
        </div>
      </div>
      <div class="cpa-scroll-table-wrap">
        <table class="cpa-table">
          <thead>
            <tr>
              <th style="min-width:220px;">${t('cpa.modelId')}</th>
              <th style="width:140px;">${t('quotas.providerColumn')}</th>
              <th style="width:120px;text-align:right;">${t('cpa.actionsColumnLabel')}</th>
            </tr>
          </thead>
          <tbody id="cpa-model-rows">
            <tr><td colspan="3" style="text-align:center;padding:24px;color:#6b8194;">${t('cpa.selectKeyToQuery')}</td></tr>
          </tbody>
        </table>
      </div>
      <div class="cpa-status" id="cpa-model-status"></div>
    </section>
    <details class="cpa-panel cpa-danger-panel" id="cpa-yaml-details">
      <summary class="cpa-danger-summary">
        <div class="cpa-danger-summary-title">
          <span class="cpa-danger-icon">${CPA_ICONS.warn}</span>
          <div>
            <strong style="color:#b4233a;font-size:14px;">${t('cpa.yamlDangerTitle')}</strong>
            <div style="font-size:12.5px;color:#6b8194;font-weight:normal;margin-top:2px;">
              ${t('cpa.yamlDangerDesc')}
            </div>
          </div>
        </div>
        <span class="cpa-danger-expand-btn">${t('cpa.expandEditor')}</span>
      </summary>
      <div class="cpa-danger-content">
        <div style="background:#fff7f6;border:1px solid #ffd0d0;border-radius:10px;padding:10px 14px;margin-bottom:14px;font-size:12.5px;color:#99242a;line-height:1.5;">
          <strong>${t('cpa.yamlNotesPrefix')}</strong>${t('cpa.yamlNotesBody')}
        </div>
        <div class="cpa-actions">
          ${button('yaml-load', t('cpa.reloadYaml'))}
          ${button('yaml-preview', t('cpa.viewChanges'))}
          ${button('yaml-save', t('cpa.saveYamlButton'), 'btn btn-danger btn-sm')}
        </div>
        <div class="cpa-field">
          <label for="cpa-yaml-editor">${t('cpa.yamlEditorLabel')}</label>
          <textarea class="cpa-code" id="cpa-yaml-editor" spellcheck="false" style="min-height:280px;"></textarea>
        </div>
        <pre class="cpa-diff" id="cpa-yaml-diff" hidden></pre>
        <div class="cpa-status" id="cpa-yaml-status"></div>
      </div>
    </details>`;

  const logsPage = $('tab-page-cpa-logs');
  if (logsPage) logsPage.innerHTML = `
    <section class="cpa-panel"><h3>${t('cpa.serviceLogTitle')}</h3><p>${t('cpa.logRangeIntro')}</p>
      <div class="cpa-actions">${button('logs-load-1h', t('cpa.logRange1h'))}${button('logs-load-6h', t('cpa.logRange6h'))}${button('logs-load-24h', t('cpa.logRange24h'))}${button('logs-refresh', t('cpa.fetchNewLogs'))}${button('logs-clear', t('cpa.clearLogs'), 'btn btn-danger btn-sm')}</div>
      <div class="cpa-field"><label for="cpa-log-filter">${t('cpa.logFilterLabel')}</label><input id="cpa-log-filter" placeholder="${t('cpa.logFilterPlaceholder')}" type="search"/></div>
      <label class="cpa-inline-check"><input id="cpa-hide-management" type="checkbox" checked/>${t('cpa.hideManagementRequests')}</label>
      <pre class="cpa-log-view" id="cpa-log-view">${t('cpa.waitingToLoad')}</pre><button class="btn btn-secondary btn-sm" type="button" id="cpa-logs-more" data-cpa-action="logs-more" hidden style="margin-top:6px;"></button><div class="cpa-logs-truncated" id="cpa-logs-truncated" role="alert" hidden style="margin-top:6px; padding:8px 10px; border:1px solid var(--danger, #d9534f); border-radius:6px; color:var(--danger, #d9534f); font-size:12.5px; line-height:1.5;"></div><div class="cpa-status" id="cpa-logs-status"></div>
    </section>
    <section class="cpa-panel"><div class="cpa-actions"><h3 style="margin-right:auto">${t('cpa.errorFilesTitle')}</h3>${button('error-logs-refresh', t('cpa.refreshFiles'))}</div>
      <div id="cpa-error-files">${t('cpa.waitingToLoad')}</div>
      <div id="cpa-error-viewer" class="cpa-log-viewer-modal" hidden>
        <div class="cpa-log-viewer-header">
          <div>
            <h4 id="cpa-error-viewer-title" style="margin:0;font-size:14px;font-family:var(--mono);color:#1e3a8a;"></h4>
            <span id="cpa-error-viewer-meta" style="font-size:12px;color:var(--text-dim);"></span>
          </div>
          <div class="cpa-actions" style="margin:0;">
            ${button('error-viewer-copy', t('cpa.copyAll'), 'btn btn-secondary btn-sm')}
            ${button('error-viewer-download', t('cpa.downloadRaw'), 'btn btn-secondary btn-sm')}
            ${button('error-viewer-close', t('cpa.closeLabel'), 'btn btn-secondary btn-sm')}
          </div>
        </div>
        <pre class="cpa-error-log-body" id="cpa-error-viewer-content"></pre>
      </div>
    </section>`;

  async function request(path, options = {}) {
    const headers = { ...getAuthHeaders(), ...(options.headers || {}) };
    const response = await fetch(`/console/api/cpa/${path}`, { ...options, headers });
    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new Error(error.error || `HTTP ${response.status}`);
    }
    return response;
  }
  async function json(path, method = 'GET', body) {
    const response = await request(path, { method, headers: body === undefined ? {} : { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
    return response.json();
  }
  function status(id, message, kind = '') { const node = $(id); if (node) { node.textContent = message; node.className = `cpa-status ${kind}`; } }

  async function download(path, name) {
    const response = await request(path);
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url; link.download = name; link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  async function loadYaml() {
    try {
      const response = await request('config-yaml');
      state.yaml = await response.text(); state.yamlEtag = response.headers.get('etag') || '';
      $('cpa-yaml-editor').value = state.yaml;
      $('cpa-yaml-diff').hidden = true;
      status('cpa-yaml-status', t('cpa.configLoaded'), 'success');
    } catch (error) { status('cpa-yaml-status', error.message, 'error'); }
  }
  function previewYaml() {
    const oldLines = state.yaml.split('\n');
    const newLines = $('cpa-yaml-editor').value.split('\n');
    const changes = [];
    for (let i = 0; i < Math.max(oldLines.length, newLines.length); i++) {
      if (oldLines[i] === newLines[i]) continue;
      changes.push(t('cpa.diffLineChange', { value: i + 1, value2: oldLines[i] ?? '', value3: newLines[i] ?? '' }));
      if (changes.length >= 80) { changes.push(t('cpa.diffTruncated')); break; }
    }
    $('cpa-yaml-diff').textContent = changes.join('\n') || t('cpa.noChanges');
    $('cpa-yaml-diff').hidden = false;
    return changes.length;
  }
  async function saveYaml() {
    if (!state.yamlEtag) return status('cpa-yaml-status', t('cpa.loadConfigFirst'), 'error');
    if (!previewYaml()) return status('cpa-yaml-status', t('cpa.noChangesToSave'));
    if (!confirm(t('cpa.dangerOverwriteConfirm'))) return;
    try {
      const response = await request('config-yaml', { method: 'PUT', headers: { 'Content-Type': 'application/yaml', 'If-Match': state.yamlEtag }, body: $('cpa-yaml-editor').value });
      await response.json(); await loadYaml();
      status('cpa-yaml-status', t('cpa.yamlSavedValidated'), 'success');
    } catch (error) { status('cpa-yaml-status', error.message, 'error'); }
  }

  async function loadSettings() {
    const results = await Promise.allSettled(settings.map(async ([key, , type]) => {
      const data = await json(`settings/${key}`);
      const row = [...document.querySelectorAll('[data-cpa-setting]')].find(node => node.dataset.cpaSetting === key);
      const input = row?.querySelector('.cpa-setting-value');
      const rawVal = data[key.split('/').at(-1)] ?? data.value ?? '';
      if (input) {
        if (type === 'boolean') {
          const isChecked = rawVal === true || String(rawVal) === 'true';
          input.checked = isChecked;
          const textEl = row.querySelector('.cpa-toggle-state-text');
          if (textEl) textEl.textContent = isChecked ? t('overview.runtimeSwitchOn') : t('cpa.closeLabel');
        } else {
          input.value = String(rawVal);
        }
      }
    }));
    const failures = results.filter(result => result.status === 'rejected');
    status('cpa-settings-status', failures.length ? t('cpa.settingsLoadFailed', { count: failures.length, value: failures[0].reason.message }) : t('cpa.commonSettingsSynced'), failures.length ? 'error' : 'success');
  }

  async function loadModelKeys() {
    try {
      const response = await fetch('/console/api/keys', { headers: getAuthHeaders() });
      if (!response.ok) throw new Error(t('cpa.readKeysFailed', { status: response.status }));
      const data = await response.json();
      const keys = Array.isArray(data.keys) ? data.keys : [];
      const selector = $('cpa-model-key');
      if (selector) {
        const prevValue = selector.value;
        selector.replaceChildren(...keys.map(key => {
          const option = document.createElement('option');
          option.value = key.api_key;
          option.textContent = `${key.key_alias || key.display_key} · ${key.display_key}`;
          return option;
        }));
        if (keys.length > 0) {
          if (prevValue && keys.some(k => k.api_key === prevValue)) {
            selector.value = prevValue;
          } else {
            selector.value = keys[0].api_key;
          }
          syncCpaDropdown('cpa-model-key');
        } else {
          syncCpaDropdown('cpa-model-key', [{ value: '', label: t('cpa.noApiKeyAvailable') }]);
        }
      }
      status('cpa-model-status', keys.length ? t('cpa.selectableKeysCount', { count: keys.length }) : t('cpa.createApiKeyFirst'), keys.length ? 'success' : '');
      if (keys.length > 0) {
        await loadModels();
      }
    } catch (error) { status('cpa-model-status', error.message, 'error'); }
  }

  function renderKeyModels() {
    const query = ($('cpa-key-models-search')?.value || '').trim().toLowerCase();
    const allModels = state.keyModels || [];
    const filtered = query
      ? allModels.filter(m => String(m.id || '').toLowerCase().includes(query) || String(m.owned_by || '').toLowerCase().includes(query))
      : allModels;

    const countEl = $('cpa-key-models-count');
    if (countEl) {
      countEl.textContent = query
        ? t('cpa.modelsCountLabel', { count: filtered.length, count2: allModels.length })
        : t('cpa.availableModelsCount', { count: allModels.length });
    }

    const tbody = $('cpa-model-rows');
    if (!tbody) return;

    if (!allModels.length) {
      tbody.innerHTML = `<tr><td colspan="3" style="text-align:center;padding:24px;color:#6b8194;">${t('cpa.noModelsForKey')}</td></tr>`;
      return;
    }

    if (!filtered.length) {
      tbody.innerHTML = `<tr><td colspan="3" style="text-align:center;padding:24px;color:#6b8194;">${t('cpa.noMatchingModels', { value: escape(query) })}</td></tr>`;
      return;
    }

    tbody.innerHTML = filtered.map(model => {
      const id = escape(model.id);
      const provider = escape(model.owned_by || 'upstream');
      return `<tr>
        <td>
          <div style="display:flex;align-items:center;gap:8px;">
            <code style="font-family:var(--mono, monospace);font-size:12.5px;color:#19334c;font-weight:600;">${id}</code>
            <button class="btn btn-secondary btn-sm" type="button" data-cpa-action="copy-model-id" data-model="${id}" style="min-height:24px;padding:1px 6px;font-size:11px;" title="${t('cpa.copyModelIdTitle')}">${t('cpa.copyButton')}</button>
          </div>
        </td>
        <td><span class="cpa-badge">${provider}</span></td>
        <td style="text-align:right;">
          <button class="btn btn-secondary btn-sm" type="button" data-cpa-action="copy-model-id" data-model="${id}" style="min-height:26px;padding:2px 8px;font-size:11.5px;">${t('cpa.copyId')}</button>
        </td>
      </tr>`;
    }).join('');
  }

  async function loadModels() {
    const key = $('cpa-model-key')?.value;
    if (!key) return status('cpa-model-status', t('cpa.selectApiKeyFirst'), 'error');
    status('cpa-model-status', t('cpa.loadingModels'), '');
    try {
      const response = await fetch('/v1/models', { headers: { Authorization: `Bearer ${key}` } });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error?.message || `HTTP ${response.status}`);
      state.keyModels = Array.isArray(data.data) ? data.data : [];
      renderKeyModels();
      status('cpa-model-status', t('cpa.modelsReturned', { value: state.keyModels.length }), 'success');
    } catch (error) {
      status('cpa-model-status', error.message, 'error');
    }
  }

  async function saveSetting(row) {
    const key = row.dataset.cpaSetting;
    const input = row.querySelector('.cpa-setting-value');
    const type = settings.find(item => item[0] === key)?.[2];
    const value = type === 'boolean' ? input.checked : type === 'number' ? Number(input.value) : input.value.trim();
    if (type === 'number' && (!Number.isFinite(value) || value < 0)) return status('cpa-settings-status', t('cpa.needNonNegativeNumber'), 'error');
    try { await json(`settings/${key}`, 'PATCH', { value }); status('cpa-settings-status', t('cpa.settingSaved', { key: key }), 'success'); }
    catch (error) { status('cpa-settings-status', error.message, 'error'); }
  }

  // after 是 CPA 的秒级时间戳游标，绝不能传 0——那等于请求「从 1970 年到现在」
  // 的全部日志。生产 CPA 日志已积累 320MB（31 个轮转文件 311M + 当前 main.log
  // 2.9M），全量请求会被 Gate 的 8MB 上限拦掉。
  // 超上限时 Gate 侧不再整体失败，而是保留已收到的部分并把 truncated 置真，
  // 所以这里必须显式告知用户看到的是残缺数据，不能让人以为这就是全部。
  // 返回本次新增行数，失败返回 null，供分档加载判断要不要覆盖状态文案。
  async function loadLogs() {
    try {
      const data = await json(`logs?after=${state.logAfter}`);
      const lines = Array.isArray(data.lines) ? data.lines : [];
      state.logLines.push(...lines);
      state.logTruncated = data.truncated === true;
      // 上限对齐最大档位：24 小时档实测 42314 行，旧的 3000 行上限会把大部分
      // 数据静默丢掉，用户点「最近 24 小时」却只看到一小截，筛选也在残片上做。
      if (state.logLines.length > LOG_LINES_LIMIT) state.logLines.splice(0, state.logLines.length - LOG_LINES_LIMIT);
      renderLogs();
      state.logAfter = Number(data['latest-timestamp']) || state.logAfter;
      status('cpa-logs-status', t('cpa.newLinesCount', { count: lines.length }), 'success');
      return lines.length;
    } catch (error) { status('cpa-logs-status', error.message, 'error'); return null; }
  }

  // 按小时分档加载。重置游标到「N 小时前」而不是 0，既不丢历史也不炸请求。
  async function loadLogsRange(hours) {
    const safeHours = [1, 6, 24].includes(Number(hours)) ? Number(hours) : 1;
    state.logRangeHours = safeHours;
    state.logAfter = Math.floor(Date.now() / 1000) - safeHours * 3600;
    state.logLines = [];
    state.logTruncated = false;
    const added = await loadLogs();
    if (added !== null) {
      status(
        'cpa-logs-status',
        state.logTruncated
          ? t('cpa.logsLoadedTruncatedSummary', { safeHours: safeHours, value: state.logLines.length })
          : t('cpa.logsLoadedSummary', { safeHours: safeHours, value: state.logLines.length }),
        state.logTruncated ? 'error' : 'success'
      );
    }
  }

  function renderLogs() {
    const query = $('cpa-log-filter').value.toLowerCase();
    const visible = state.logLines.filter(line => (!query || String(line).toLowerCase().includes(query)) && (!$('cpa-hide-management').checked || !String(line).includes('/v0/management/')));
    const view = $('cpa-log-view');
    // 24 小时档实测 42314 行，一次性 join 成 6MB 文本再塞进 <pre> 会明显卡顿。
    // 改为只渲染末尾 LOG_RENDER_LIMIT 行，其余靠「显示更多」逐段追加，
    // 既保住全量筛选能力（筛选在完整数组上进行），又不让 DOM 爆炸。
    const shown = visible.slice(Math.max(0, visible.length - state.logRenderLimit));
    view.textContent = shown.join('\n') || t('cpa.noLogs');
    const hidden = visible.length - shown.length;
    const more = $('cpa-logs-more');
    if (more) {
      if (hidden > 0) {
        more.hidden = false;
        more.textContent = t('cpa.logLinesTruncatedMore', { count: shown.length, hidden: hidden });
      } else {
        more.hidden = true;
        more.textContent = '';
      }
    }
    // 截断提示必须常驻：用户一筛选一滚动就看不到加载时那句状态栏了，
    // 会把残缺数据当成该时间范围的全部日志。
    const banner = $('cpa-logs-truncated');
    if (banner) {
      banner.hidden = !state.logTruncated;
      if (state.logTruncated) {
        banner.textContent = t('cpa.logsTruncatedBanner', { value: state.logLines.length });
      }
    }
    view.scrollTop = view.scrollHeight;
  }

  async function loadErrorFiles() {
    try {
      const data = await json('request-error-logs');
      const files = Array.isArray(data.files) ? data.files : [];
      $('cpa-error-files').innerHTML = files.length ? files.map(file => `
        <div class="cpa-setting-row" style="align-items:center;">
          <span style="font-family:var(--mono);word-break:break-all;"><strong>${escape(file.name)}</strong> · ${t('cpa.fileMetaSizeBytes', { value: escape(file.size) })}</span>
          <span style="color:var(--text-dim);font-size:12px;">${file.modified ? new Date(Number(file.modified) * 1000).toLocaleString(LOCALE) : ''}</span>
          <div class="cpa-actions" style="margin:0;">
            <button class="btn btn-primary btn-sm" data-cpa-action="error-preview" data-name="${escape(file.name)}" data-size="${escape(file.size)}" data-modified="${escape(file.modified || '')}">${t('cpa.viewOnline')}</button>
            <button class="btn btn-secondary btn-sm" data-cpa-action="error-download" data-name="${escape(file.name)}">${t('cpa.download')}</button>
          </div>
        </div>`).join('') : t('cpa.noRequestErrorFiles');
    } catch (error) { $('cpa-error-files').textContent = error.message; }
  }

  async function previewErrorLog(name, size, modified) {
    if (!name) return;
    state.viewingErrorName = name;
    const viewer = $('cpa-error-viewer');
    const title = $('cpa-error-viewer-title');
    const meta = $('cpa-error-viewer-meta');
    const content = $('cpa-error-viewer-content');
    if (!viewer) return;

    title.textContent = name;
    meta.textContent = `${size ? t('cpa.fileListSizeBytes', { count: size }) : ''}${modified ? ` ${t('cpa.modifiedAtPrefix', { value: new Date(Number(modified) * 1000).toLocaleString(LOCALE) })}` : ''}`;
    content.textContent = t('cpa.readingErrorLogContext');
    viewer.hidden = false;
    viewer.scrollIntoView({ block: 'nearest' });

    try {
      const response = await fetch(`/console/api/cpa/request-error-logs/preview?name=${encodeURIComponent(name)}`, {
        headers: getAuthHeaders()
      });
      if (response.status === 413) {
        content.textContent = t('cpa.previewTooLargeNotice');
        showToast(t('cpa.previewTooLargeToast'), 'error');
        return;
      }
      if (!response.ok) {
        const errJson = await response.json().catch(() => ({}));
        throw new Error(errJson.error || `HTTP ${response.status}`);
      }
      const text = await response.text();
      content.textContent = text || t('cpa.emptyLogContent');
    } catch (error) {
      content.textContent = t('cpa.loadLogFailed', { msg: error.message });
      showToast(t('cpa.loadErrorLogFailed', { msg: error.message }), 'error');
    }
  }

  // --- 路由策略与会话粘性 (Routing Strategy & Affinity) ---
  async function loadRoutingConfig() {
    try {
      let data;
      try {
        data = await json('runtime-config');
      } catch {
        const res = await fetch('/console/api/runtime-config', { headers: getAuthHeaders() });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        data = await res.json();
      }
      const routing = data.routing?.value || data.routing || {};
      if ($('cpa-routing-strategy')) {
        $('cpa-routing-strategy').value = routing.strategy || 'round-robin';
        syncCpaDropdown('cpa-routing-strategy');
      }
      if ($('cpa-routing-affinity')) {
        $('cpa-routing-affinity').checked = routing.session_affinity === true;
      }
      if ($('cpa-routing-ttl')) {
        $('cpa-routing-ttl').value = routing.session_affinity_ttl ?? '';
      }
      status('cpa-routing-status', t('cpa.routingConfigLoaded'), 'success');
    } catch (err) {
      status('cpa-routing-status', t('cpa.readRoutingFailed', { msg: err.message }), 'error');
    }
  }

  async function saveRoutingConfig() {
    try {
      const strategy = $('cpa-routing-strategy')?.value || 'round-robin';
      const session_affinity = Boolean($('cpa-routing-affinity')?.checked);
      const session_affinity_ttl = $('cpa-routing-ttl')?.value?.trim() || '';
      const payload = { strategy, session_affinity, session_affinity_ttl };

      const res = await fetch('/console/api/upstream-routing', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', ...getAuthHeaders() },
        body: JSON.stringify(payload)
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
      status('cpa-routing-status', t('cpa.routingStrategyUpdated'), 'success');
      showToast(t('cpa.routingStrategySaved'), 'success');
      await loadRoutingConfig();
    } catch (err) {
      status('cpa-routing-status', t('cpa.saveFailed', { msg: err.message }), 'error');
      showToast(t('cpa.saveRoutingStrategyFailed', { msg: err.message }), 'error');
    }
  }

  const loaded = new Set();
  async function loadPage(id) {
    if (loaded.has(id)) return;
    loaded.add(id);
    if (id === 'cpa-config') await Promise.all([loadYaml(), loadSettings(), loadRoutingConfig(), loadModelKeys()]);
    if (id === 'cpa-logs') await Promise.all([loadLogsRange(state.logRangeHours), loadErrorFiles()]);
  }

  document.addEventListener('click', async event => {
    const navButton = event.target.closest('[data-cpa-nav]');
    if (navButton) {
      if (isMemberAuth()) return;
      switchNavTab(navButton.dataset.cpaNav);
      loadPage(navButton.dataset.cpaNav);
      return;
    }
    const control = event.target.closest('[data-cpa-action]');
    if (!control) return;
    const action = control.dataset.cpaAction;
    control.disabled = true;
    try {
      if (action === 'yaml-load') await loadYaml();
      else if (action === 'yaml-preview') previewYaml();
      else if (action === 'yaml-save') await saveYaml();
      else if (action === 'settings-load') await loadSettings();
      else if (action === 'models-load-keys') await loadModelKeys();
      else if (action === 'models-load') await loadModels();
      else if (action === 'copy-model-id') {
        const id = control.dataset.model;
        if (id) {
          if (typeof copyToClipboard === 'function') {
            copyToClipboard(id, t('cpa.modelIdCopied', { id: id }));
          } else if (navigator.clipboard) {
            await navigator.clipboard.writeText(id).catch(() => {});
            showToast(t('cpa.modelIdCopied', { id: id }), 'success');
          }
        }
      }
      else if (action === 'version-check') { const version = await json('latest-version'); status('cpa-version-status', JSON.stringify(version), 'success'); }
      else if (action === 'setting-save') await saveSetting(control.closest('[data-cpa-setting]'));
      else if (action === 'logs-refresh') await loadLogs();
      else if (action === 'logs-load-1h') await loadLogsRange(1);
      else if (action === 'logs-load-6h') await loadLogsRange(6);
      else if (action === 'logs-load-24h') await loadLogsRange(24);
      else if (action === 'logs-more') { state.logRenderLimit += LOG_RENDER_LIMIT; renderLogs(); }
      // 清空后同样不能用 0 重置游标：CPA 日志文件可能仍在，传 0 会再次请求全量。
      else if (action === 'logs-clear' && confirm(t('cpa.confirmClearLogs'))) { await json('logs', 'DELETE'); state.logAfter = Math.floor(Date.now() / 1000) - state.logRangeHours * 3600; state.logLines = []; state.logTruncated = false; await loadLogs(); }
      else if (action === 'error-logs-refresh') await loadErrorFiles();
      else if (action === 'error-preview') await previewErrorLog(control.dataset.name, control.dataset.size, control.dataset.modified);
      else if (action === 'error-viewer-close') { const v = $('cpa-error-viewer'); if (v) v.hidden = true; state.viewingErrorName = ''; }
      else if (action === 'error-viewer-copy') {
        const text = $('cpa-error-viewer-content')?.textContent;
        if (text) {
          if (typeof copyToClipboard === 'function') {
            copyToClipboard(text, t('cpa.logCopiedToClipboard'));
          } else if (navigator.clipboard) {
            await navigator.clipboard.writeText(text).catch(() => {});
            showToast(t('cpa.logCopiedToClipboard'), 'success');
          }
        }
      }
      else if (action === 'error-viewer-download') {
        if (state.viewingErrorName) await download(`request-error-logs/download?name=${encodeURIComponent(state.viewingErrorName)}`, state.viewingErrorName);
      }
      else if (action === 'error-download') await download(`request-error-logs/download?name=${encodeURIComponent(control.dataset.name)}`, control.dataset.name);
      else if (action === 'routing-load') await loadRoutingConfig();
      else if (action === 'routing-save') await saveRoutingConfig();
    } catch (error) { showToast(t('cpa.operationFailed', { msg: error.message }), 'error'); }
    finally { control.disabled = false; }
  });

  $('cpa-log-filter')?.addEventListener('input', renderLogs);
  $('cpa-hide-management')?.addEventListener('change', renderLogs);
  $('cpa-settings-rows')?.addEventListener('change', event => {
    const toggle = event.target.closest('.cpa-toggle-input');
    if (!toggle) return;
    const textEl = toggle.closest('.cpa-setting-row')?.querySelector('.cpa-toggle-state-text');
    if (textEl) textEl.textContent = toggle.checked ? t('overview.runtimeSwitchOn') : t('cpa.closeLabel');
  });

  $('cpa-key-models-search')?.addEventListener('input', renderKeyModels);
  $('cpa-model-key')?.addEventListener('change', () => {
    loadModels();
  });

  setInterval(() => { if (activeNavTab === 'cpa-logs' && !isMemberAuth()) loadLogs(); }, 15000);

  // 初始化所有已声明的静态 select 元素为现代浮动下拉组件
  ['cpa-model-key', 'cpa-routing-strategy'].forEach(id => {
    syncCpaDropdown(id);
  });
})();
