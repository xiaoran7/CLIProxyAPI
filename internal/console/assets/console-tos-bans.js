/**
 * Gate Admin Console - TOS 封禁监控
 * TOS 403 执法台账：日志为封禁记录的唯一权威来源，gate.db 提供 200/403 行为对比。
 */
(function () {
  let tosData = null;
  let tosAccountFilter = 'all';
  let tosModelFilter = 'all';
  let tosAccountDropdown = null;
  let tosModelDropdown = null;

  function tosEscape(value) {
    if (value === undefined || value === null) return '';
    return String(value)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function tosFormatTs(ts) {
    if (!ts || Number(ts) <= 0) return '—';
    const d = new Date(Number(ts) * 1000);
    if (Number.isNaN(d.getTime())) return '—';
    const pad = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
  }

  function tosRelative(ts) {
    if (!ts || Number(ts) <= 0) return '';
    const diff = Math.floor(Date.now() / 1000 - Number(ts));
    if (diff < 0) return '刚刚';
    if (diff < 60) return `${diff} 秒前`;
    if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`;
    if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`;
    return `${Math.floor(diff / 86400)} 天前`;
  }

  // ---- 汇总卡与侧边栏角标 ----
  function renderTOSSummary(summary) {
    const accountsEl = document.getElementById('tos-stat-accounts');
    const el24 = document.getElementById('tos-stat-24h');
    const el7 = document.getElementById('tos-stat-7d');
    if (accountsEl) accountsEl.textContent = String(summary?.banned_accounts || 0);
    if (el24) el24.textContent = String(summary?.bans_24h || 0);
    if (el7) el7.textContent = String(summary?.bans_7d || 0);

    const badge = document.getElementById('nav-badge-tos-bans');
    if (badge) {
      const count = Number(summary?.banned_accounts || 0);
      badge.textContent = String(count);
      badge.style.display = count > 0 ? '' : 'none';
    }

    const meta = document.getElementById('tos-scan-meta');
    if (meta) {
      const parts = [];
      if (summary?.log_dir) parts.push(`日志目录 ${summary.log_dir}`);
      if (Number.isFinite(summary?.files_scanned)) parts.push(`${summary.files_scanned} 个日志文件`);
      if (Number.isFinite(summary?.record_count)) parts.push(`${summary.record_count} 条封禁记录`);
      if (Number.isFinite(summary?.truncated_records) && summary.truncated_records > 0) {
        parts.push(`${summary.truncated_records} 条上游响应被日志截断`);
      }
      if (summary?.scanned_at) parts.push(`扫描于 ${tosFormatTs(summary.scanned_at)}`);
      if (summary?.scan_error) parts.push(`日志扫描异常：${summary.scan_error}`);
      meta.textContent = parts.join(' · ') || '未发现封禁记录';
    }
  }

  // ---- 过滤下拉框 ----
  function buildTOSFilterOptions() {
    const accounts = Array.isArray(tosData?.accounts) ? tosData.accounts : [];
    const accountOpts = [{ value: 'all', label: `全部账号 (${accounts.length})` }];
    const modelSet = new Set();
    accounts.forEach((acc) => {
      accountOpts.push({ value: acc.account, label: acc.account });
      (acc.models || []).forEach((m) => modelSet.add(m));
    });
    const modelOpts = [{ value: 'all', label: `全部模型 (${modelSet.size})` }];
    Array.from(modelSet).sort().forEach((m) => modelOpts.push({ value: m, label: m }));
    return { accountOpts, modelOpts };
  }

  function syncTOSDropdowns() {
    const { accountOpts, modelOpts } = buildTOSFilterOptions();
    if (tosAccountDropdown) {
      tosAccountDropdown.updateOptions(accountOpts, tosAccountFilter);
    } else {
      const container = document.getElementById('tos-account-dropdown');
      if (container && typeof CustomDropdown === 'function') {
        tosAccountDropdown = new CustomDropdown({
          containerId: 'tos-account-dropdown',
          options: accountOpts,
          value: tosAccountFilter,
          onChange: (val) => { tosAccountFilter = val; renderTOSAccounts(); }
        });
      }
    }
    if (tosModelDropdown) {
      tosModelDropdown.updateOptions(modelOpts, tosModelFilter);
    } else {
      const container = document.getElementById('tos-model-dropdown');
      if (container && typeof CustomDropdown === 'function') {
        tosModelDropdown = new CustomDropdown({
          containerId: 'tos-model-dropdown',
          options: modelOpts,
          value: tosModelFilter,
          onChange: (val) => { tosModelFilter = val; renderTOSAccounts(); }
        });
      }
    }
  }

  // ---- 单条 403 记录 ----
  function renderTOSRecord(rec) {
    const statusBadge = Number(rec.status) === 200
      ? '<span class="tos-status-badge tos-status-ok">200 OK</span>'
      : `<span class="tos-status-badge tos-status-forbidden">${tosEscape(rec.status || 403)} ${tosEscape(rec.error_code || 'FORBIDDEN')}</span>`;

    const truncatedTag = rec.truncated
      ? '<span class="tos-truncated-tag" title="上游响应在写入应用日志时已被截断为 300 字符，完整报文需从上游侧复核">日志截断</span>'
      : '';

    const validationRow = rec.validation_url
      ? `<div class="tos-record-row"><span class="tos-record-key">validation_url</span><span class="tos-record-val"><a href="${tosEscape(rec.validation_url)}" target="_blank" rel="noopener" class="tos-validation-link">${tosEscape(rec.validation_url)}</a></span></div>`
      : '';

    return `
      <div class="tos-record">
        <div class="tos-record-head">
          <span class="tos-record-time" title="${tosEscape(rec.time_text || '')}">${tosFormatTs(rec.ts)}</span>
          <span class="tos-record-ago">${tosRelative(rec.ts)}</span>
          ${statusBadge}
          ${rec.model ? `<span class="tos-record-model">${tosEscape(rec.model)}</span>` : ''}
          ${truncatedTag}
        </div>
        <div class="tos-record-body">
          <div class="tos-record-row"><span class="tos-record-key">错误详情</span><span class="tos-record-val tos-record-message">${tosEscape(rec.message || '—')}</span></div>
          ${rec.reason ? `<div class="tos-record-row"><span class="tos-record-key">reason</span><span class="tos-record-val">${tosEscape(rec.reason)}</span></div>` : ''}
          ${rec.provider ? `<div class="tos-record-row"><span class="tos-record-key">provider</span><span class="tos-record-val">${tosEscape(rec.provider)}</span></div>` : ''}
          ${validationRow}
          ${rec.raw ? `<details class="tos-record-raw"><summary>上游原始日志行</summary><pre>${tosEscape(rec.raw)}</pre></details>` : ''}
        </div>
      </div>
    `;
  }

  // ---- 单个账号卡片 ----
  function renderTOSAccountCard(acc) {
    const records = Array.isArray(acc.records) ? acc.records : [];
    const authStatus = acc.auth_status
      ? `<span class="tos-auth-badge ${acc.auth_disabled ? 'tos-auth-disabled' : 'tos-auth-active'}">${tosEscape(acc.auth_status)}</span>`
      : '';

    const lastSuccess = acc.last_success_at
      ? `${tosFormatTs(acc.last_success_at)} <span class="tos-record-ago">${tosRelative(acc.last_success_at)}</span>`
      : '<span class="tos-record-muted">无成功记录</span>';

    // 200 vs 403 对比：用宽度条直观表达“是否彻底死亡”
    const ok7 = Number(acc.success_7d || 0);
    const bad7 = Number(acc.forbidden_7d || 0);
    const total7 = ok7 + bad7;
    const okPct = total7 > 0 ? Math.round((ok7 / total7) * 100) : 0;

    return `
      <div class="tos-account-card">
        <div class="tos-account-head">
          <div class="tos-account-identity">
            <span class="tos-account-email">${tosEscape(acc.account)}</span>
            ${authStatus}
            ${acc.auth_index ? `<span class="tos-auth-index">${tosEscape(acc.auth_index)}</span>` : ''}
          </div>
          <div class="tos-account-counts">
            <span class="tos-count-chip tos-count-danger">${Number(acc.ban_count || 0)} 次封禁</span>
            <span class="tos-count-chip">首次 ${tosFormatTs(acc.first_banned_at)}</span>
            <span class="tos-count-chip">最近 ${tosFormatTs(acc.last_banned_at)}</span>
          </div>
        </div>

        <div class="tos-account-stats">
          <div class="tos-stat-block">
            <span class="tos-stat-label">最近成功请求</span>
            <span class="tos-stat-value">${lastSuccess}</span>
          </div>
          <div class="tos-stat-block">
            <span class="tos-stat-label">近 7 天 200 / 403</span>
            <span class="tos-stat-value"><b style="color:var(--success,#059669);">${ok7}</b> / <b style="color:var(--danger);">${bad7}</b></span>
          </div>
          <div class="tos-stat-block">
            <span class="tos-stat-label">近 24h 200 / 403</span>
            <span class="tos-stat-value"><b style="color:var(--success,#059669);">${Number(acc.success_24h || 0)}</b> / <b style="color:var(--danger);">${Number(acc.forbidden_24h || 0)}</b></span>
          </div>
          <div class="tos-stat-block">
            <span class="tos-stat-label">全量 200 / 403</span>
            <span class="tos-stat-value">${Number(acc.total_success || 0)} / ${Number(acc.total_forbidden || 0)}</span>
          </div>
        </div>

        <div class="tos-ratio-bar" title="近 7 天成功 ${ok7} 次 / 403 ${bad7} 次">
          <div class="tos-ratio-ok" style="width:${okPct}%;"></div>
        </div>

        <div class="tos-records">
          ${records.map(renderTOSRecord).join('') || '<div class="tos-record-muted" style="padding:12px;">无记录</div>'}
        </div>
      </div>
    `;
  }

  // ---- 主列表 ----
  function renderTOSAccounts() {
    const container = document.getElementById('tos-accounts-list');
    if (!container) return;
    const all = Array.isArray(tosData?.accounts) ? tosData.accounts : [];

    const filtered = all
      .filter((acc) => tosAccountFilter === 'all' || acc.account === tosAccountFilter)
      .filter((acc) => tosModelFilter === 'all' || (acc.models || []).includes(tosModelFilter));

    if (filtered.length === 0) {
      const hasAny = all.length > 0;
      container.innerHTML = `
        <div style="text-align:center; color: var(--text-dim); padding: 40px;">
          ${hasAny ? '当前筛选条件下没有匹配的封禁账号' : '未在应用日志中发现 TOS 封禁记录 (403 / TOS_VIOLATION)'}
        </div>`;
      return;
    }
    container.innerHTML = filtered.map(renderTOSAccountCard).join('');
  }

  // ---- 数据拉取 ----
  async function fetchTOSBans(force = false) {
    if (typeof getAdminSecret === 'function' && !getAdminSecret()) return;
    try {
      const res = await fetch('/console/api/tos-bans', { headers: getAuthHeaders() });
      if (!res.ok) {
        if (typeof showToast === 'function') showToast(`封禁监控拉取失败: HTTP ${res.status}`, 'error');
        return;
      }
      tosData = await res.json();
      renderTOSSummary(tosData.summary);
      syncTOSDropdowns();
      renderTOSAccounts();
      if (force && typeof showToast === 'function') showToast('已重新扫描应用日志', 'success');
    } catch (err) {
      console.warn('[TOS] fetch failed:', err && err.message);
    }
  }

  function resetTOSFilters() {
    tosAccountFilter = 'all';
    tosModelFilter = 'all';
    if (tosAccountDropdown) tosAccountDropdown.setValue('all');
    if (tosModelDropdown) tosModelDropdown.setValue('all');
    renderTOSAccounts();
  }

  window.fetchTOSBans = fetchTOSBans;
  window.renderTOSAccounts = renderTOSAccounts;
  window.resetTOSFilters = resetTOSFilters;
})();
