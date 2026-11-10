/**
 * Gate Admin Console - Request Logs & Error Auditing Module
 * 包含请求流水真实分页、服务商反查与推断、错误与熔断中心审计
 * 数据拉取与筛选拆入 console-logs-data.js，错误中心拆入 console-logs-errors.js。
 */

    // 0. 时间范围滑动条与多维过滤状态
    let logsTimeMinPercent = 0;
    let logsTimeMaxPercent = 100;
    let logsTimeBufferMinTs = 0;
    let logsTimeBufferMaxTs = 0;
    let logsActiveTimePreset = 'all';
    let logsSelectedIP = '';
    let logsSelectedModel = '';

    function updateLogsSliderProgressUI() {
      const progress = document.getElementById('logs-slider-progress');
      const currentLabel = document.getElementById('logs-time-label-current');
      if (!progress) return;

      progress.style.left = `${logsTimeMinPercent}%`;
      progress.style.right = `${100 - logsTimeMaxPercent}%`;

      if (currentLabel) {
        if (logsTimeMinPercent === 0 && logsTimeMaxPercent === 100) {
          currentLabel.textContent = '全量跨度';
        } else if (logsTimeBufferMinTs > 0 && logsTimeBufferMaxTs > 0) {
          const span = logsTimeBufferMaxTs - logsTimeBufferMinTs;
          const startT = Math.round(logsTimeBufferMinTs + span * (logsTimeMinPercent / 100));
          const endT = Math.round(logsTimeBufferMinTs + span * (logsTimeMaxPercent / 100));
          currentLabel.textContent = `${new Date(startT * 1000).toLocaleTimeString()} ➔ ${new Date(endT * 1000).toLocaleTimeString()}`;
        }
      }
    }

    function onLogsTimeSliderInput() {
      const minInput = document.getElementById('logs-time-min');
      const maxInput = document.getElementById('logs-time-max');
      if (!minInput || !maxInput) return;

      let minVal = parseInt(minInput.value, 10);
      let maxVal = parseInt(maxInput.value, 10);

      if (minVal > maxVal) {
        const tmp = minVal;
        minVal = maxVal;
        maxVal = tmp;
      }

      logsTimeMinPercent = minVal;
      logsTimeMaxPercent = maxVal;
      updateLogsSliderProgressUI();

      document.querySelectorAll('.logs-preset-btn').forEach(b => b.classList.remove('active'));
      logsPage = 1;
      fetchEventsData();
    }

    function setLogsTimePreset(preset) {
      logsActiveTimePreset = preset;
      document.querySelectorAll('.logs-preset-btn').forEach(b => {
        b.classList.toggle('active', b.id === `btn-logs-preset-${preset}`);
      });

      const minInput = document.getElementById('logs-time-min');
      const maxInput = document.getElementById('logs-time-max');
      if (!minInput || !maxInput) return;

      if (preset === 'all' || logsTimeBufferMaxTs <= logsTimeBufferMinTs) {
        logsTimeMinPercent = 0;
        logsTimeMaxPercent = 100;
      } else {
        const span = Math.max(1, logsTimeBufferMaxTs - logsTimeBufferMinTs);
        let durationSec = 3600;
        if (preset === '24h') durationSec = 86400;
        else if (preset === '6h') durationSec = 21600;
        else if (preset === '1h') durationSec = 3600;
        else if (preset === '15m') durationSec = 900;
        else if (preset === '5m') durationSec = 300;

        const targetStartTs = Math.max(logsTimeBufferMinTs, logsTimeBufferMaxTs - durationSec);
        logsTimeMinPercent = Math.max(0, Math.min(100, Math.round(((targetStartTs - logsTimeBufferMinTs) / span) * 100)));
        logsTimeMaxPercent = 100;
      }

      minInput.value = String(logsTimeMinPercent);
      maxInput.value = String(logsTimeMaxPercent);

      updateLogsSliderProgressUI();
      logsPage = 1;
      fetchEventsData();
    }

    function changeLogsIPFilter(val) {
      logsSelectedIP = val || '';
      logsPage = 1;
      fetchEventsData();
    }

    function changeLogsModelFilter(val) {
      logsSelectedModel = val || '';
      logsPage = 1;
      fetchEventsData();
    }

    window.filterLogsByIP = function(ip) {
      if (!ip || ip === '-') return;
      logsSelectedIP = ip;
      logsPage = 1;
      if (dropdownInstances['logs-ip-dropdown']) {
        dropdownInstances['logs-ip-dropdown'].setValue(ip, false);
      }
      fetchEventsData();
      if (typeof showToast === 'function') {
        showToast(`已仅筛选客户端 IP: ${ip}`, 'success');
      }
    };

    function resetLogsFilters() {
      logsSelectedIP = '';
      logsSelectedModel = '';
      logsKeyFilter = 'all';
      logsProviderFilter = 'all';
      logsTimeRange = 'all';
      logsAuthFilter = 'all';
      logsSearchQuery = '';
      eventFilter = 'all';
      logsTimeMinPercent = 0;
      logsTimeMaxPercent = 100;
      logsPage = 1;

      const searchInput = document.getElementById('logs-search');
      if (searchInput) searchInput.value = '';
      const minInput = document.getElementById('logs-time-min');
      const maxInput = document.getElementById('logs-time-max');
      if (minInput) minInput.value = '0';
      if (maxInput) maxInput.value = '100';
      document.querySelectorAll('.logs-preset-btn').forEach(b => {
        b.classList.toggle('active', b.id === 'btn-logs-preset-all');
      });
      updateLogsSliderProgressUI();

      if (dropdownInstances['logs-ip-dropdown']) dropdownInstances['logs-ip-dropdown'].setValue('', false);
      if (dropdownInstances['logs-key-dropdown']) dropdownInstances['logs-key-dropdown'].setValue('all', false);
      if (dropdownInstances['logs-model-dropdown']) dropdownInstances['logs-model-dropdown'].setValue('', false);
      if (dropdownInstances['logs-provider-dropdown']) dropdownInstances['logs-provider-dropdown'].setValue('all', false);
      if (dropdownInstances['logs-time-dropdown']) dropdownInstances['logs-time-dropdown'].setValue('all', false);

      const tabs = document.getElementById('events-filter-tabs')?.querySelectorAll('button');
      tabs?.forEach(btn => btn.classList.toggle('active', btn.getAttribute('onclick')?.includes("'all'")));

      updateAuthFilterNotice();
      fetchEventsData(true);
      if (typeof showToast === 'function') {
        showToast('已重置所有流水筛选', 'info');
      }
    }


    function renderEventRowHtml(ev, isNew = false) {
      const rowClassAttr = isNew ? ' class="live-row-enter live-row-highlight"' : '';
      const tsMs = ev.ts > 1e11 ? ev.ts : ev.ts * 1000;
      const cst = typeof getCstParts === 'function' ? getCstParts(tsMs) : null;
      const d = cst ? cst.d : new Date(tsMs);
      const timeStr = cst ? cst.timeStr : d.toLocaleTimeString(LOCALE, { hour12: false, timeZone: 'Asia/Shanghai' });
      const dateStr = cst ? cst.dateStr : `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
      const fullIso = Number.isNaN(d.getTime()) ? '' : d.toISOString();

      let statusClass = 'chip-green';
      if (ev.status === 429 || ev.status === 403) statusClass = 'chip-red';
      else if (ev.status >= 500) statusClass = 'chip-red';
      else if (ev.status >= 400) statusClass = 'chip-yellow';

      let cacheTag = '<span style="color:var(--text-dim);">—</span>';
      if (ev.cached_tokens > 0) {
        cacheTag = `<span class="chip chip-cache" title="${t('logs.promptCacheHitTitle', { value: ev.cached_tokens.toLocaleString(LOCALE) })} Tokens">${formatTokens(ev.cached_tokens)} (${ev.cache_hit_rate}%)</span>`;
      } else if (ev.status >= 200 && ev.status < 400 && (ev.prompt_tokens > 0 || ev.total_tokens > 0)) {
        cacheTag = `<span style="color:var(--text-dim); font-size:11.5px; font-family:var(--mono);" title="${t('logs.cacheMissTitle')}">0%</span>`;
      }

      const costTag = ev.cost_usd > 0 ? `<span class="num" style="color:#34d399;">${formatCost(ev.cost_usd)}</span>` : '<span style="color:var(--text-dim);">$0.00</span>';

      const flags = [];
      if (ev.stream) flags.push(`<span class="flag-pill">${t('logs.streamFlag')}</span>`);
      if (ev.estimated) flags.push(`<span class="flag-pill">${t('logs.estimatedFlag')}</span>`);
      if (ev.error_type) {
        flags.push(`<span class="flag-pill" style="background:#fef2f2;color:#ef4444;border-color:#fecaca;font-weight:600;" title="${t('logs.errorTypeTitle', { value: escapeHtml(errorTypeLabel(ev.error_type)) })}">${escapeHtml(errorTypeLabel(ev.error_type))}</span>`);
      }

      const totalLatStr = ev.latency_ms >= 1000 ? (ev.latency_ms / 1000).toFixed(1) + 's' : ev.latency_ms + 'ms';
      const ttftStr = ev.ttft_ms ? (ev.ttft_ms >= 1000 ? (ev.ttft_ms / 1000).toFixed(1) + 's' : ev.ttft_ms + 'ms') : '-';
      const latencyHtml = `<div class="num" style="font-weight:500;">${totalLatStr}</div>` +
        (ev.stream && ev.ttft_ms ? `<div style="font-size:10.5px; color:var(--text-dim);">${t('logs.ttftLabel', { ttftStr: ttftStr })}</div>` : '');

      const ks = getKeyStyle(ev.key_id, ev.display_key);
      const filterKeyTarget = ev.key_id != null ? ev.key_id : ev.display_key;
      const mainKeyText = ev.key_alias || ev.display_key;
      const subKeyText = ev.key_alias ? ev.display_key : '—';

      // 上游承接账号
      const shortAuth = ev.auth_index ? ev.auth_index.slice(0, 8) : '';
      let authName = ev.auth_name || (ev.auth_index ? getAuthDisplayName(ev.auth_index) : '') || '';
      if (authName && typeof authName === 'string') {
        authName = authName.replace(/\.json$/i, '');
      }
      const authBrand = getProviderBrand(resolveEventProvider(ev), authName);
      if (ev.auth_index && (!authName || authName === ev.auth_index)) {
        authName = (authBrand && authBrand.id !== 'other')
          ? t('logs.channelByShortAuth', { name: authBrand.name, shortAuth: shortAuth })
          : t('logs.deletedAccountByShortAuth', { shortAuth: shortAuth });
      }
      let authTdHtml = '<span style="color:var(--text-dim); font-size:11.5px;">—</span>';
      if (ev.auth_index) {
        authTdHtml = `
          <div style="cursor:pointer;" onclick="filterBySpecificAuth('${escapeHtml(ev.auth_index)}')" title="${t('logs.filterByUpstreamAccountHint')}">
            <div style="display:flex; align-items:center; gap:5px;">
              <span class="provider-pill ${authBrand.pillClass}" style="font-size:9.5px; padding:0 4px; cursor:pointer;" onclick="event.stopPropagation(); filterBySpecificProvider('${escapeHtml(authBrand.id)}');" title="${t('logs.filterByProviderHint')}">${escapeHtml(authBrand.name)}</span>
              <span style="font-weight:600; color:var(--text-main); font-size:11.5px; max-width:160px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${escapeHtml(authName || ev.auth_index)}">
                ${escapeHtml(authName || ev.auth_index)}
              </span>
            </div>
            <div style="font-size:10px; font-family:var(--mono); color:var(--text-dim); margin-top:1px;">ID: ${escapeHtml(shortAuth)}</div>
          </div>
        `;
      }

      const statusTitle = ev.status >= 400
        ? `HTTP ${ev.status}${ev.error_type ? ` · ${errorTypeLabel(ev.error_type)}` : ''}`
        : `HTTP ${t('logs.httpSuccessStatus', { status: ev.status })}`;

      // 思考强度标签 (Reasoning Effort / Thinking Budget)
      let reasoningHtml = '<span style="color:var(--text-dim); font-size:11.5px;">—</span>';
      if (ev.reasoning_effort) {
        reasoningHtml = `<span class="chip chip-purple" title="${t('logs.reasoningEffortTitle', { value: escapeHtml(ev.reasoning_effort) })}">${escapeHtml(ev.reasoning_effort)}</span>`;
      }

      // 速率 (Tokens/s 生成吞吐)
      let tpsHtml = '<span style="color:var(--text-dim); font-size:11.5px;">—</span>';
      let tpsNum = Number(ev.tokens_per_sec);
      if ((!Number.isFinite(tpsNum) || tpsNum <= 0) && ev.completion_tokens > 0 && ev.latency_ms > 0) {
        tpsNum = Math.min(500, Math.round((ev.completion_tokens / (ev.latency_ms / 1000)) * 10) / 10);
      }
      if (Number.isFinite(tpsNum) && tpsNum > 0) {
        let speedColor = '#0284c7';
        if (tpsNum >= 100) speedColor = '#10b981';
        else if (tpsNum >= 50) speedColor = '#06b6d4';
        else if (tpsNum < 20) speedColor = '#f59e0b';

        const isPureStream = ev.stream && ev.ttft_ms > 0 && ev.latency_ms > ev.ttft_ms &&
          (ev.latency_ms - ev.ttft_ms) >= 100 &&
          (ev.completion_tokens / ((ev.latency_ms - ev.ttft_ms) / 1000) <= 500);

        let tpsTitle = '';
        let tpsPrefix = '';
        if (isPureStream) {
          tpsTitle = t('logs.generationRateTitle', { tpsNum: tpsNum });
        } else if (ev.stream) {
          tpsPrefix = '~';
          tpsTitle = t('logs.rateBurstEstimateTitle', { tpsNum: tpsNum });
        } else {
          tpsTitle = t('logs.nonStreamRateTitle', { tpsNum: tpsNum });
        }
        tpsHtml = `<span class="speed-badge" style="color:${speedColor}; font-weight:600; font-size:11.5px;" title="${tpsTitle}">${tpsPrefix}${tpsNum} <span style="font-size:10px; font-weight:400; opacity:0.8;">tok/s</span></span>`;
      }

      // 客户端 IP、来源地域与 ASN 组织
      let ipTdHtml = '<span style="color:var(--text-dim); font-size:11.5px;">—</span>';
      if (ev.client_ip && ev.client_ip !== '-') {
        ipTdHtml = `<span class="traffic-pill traffic-pill-ip" onclick="filterLogsByIP('${escapeHtml(ev.client_ip)}')" title="点击筛选此 IP">${escapeHtml(ev.client_ip)}</span>`;
      }

      // 地域单元格：紧凑格式 + 首帧骨架
      //
      // 后端返回的是「国家 · 省 · 市」（如 中国 · 湖南 · 株洲），三段式在窄列里
      // 会被 CSS 按「·」折行成竖排，把整行行高撑到数倍。这里在渲染层收敛成
      // 两段：国内省略国家名只留「省 · 市」，海外保留「国家 · 城市」。
      // 无 geo 数据时渲染骨架条而不是「未知地域」——先显示一个错值再跳变
      // 比短暂占位更糟。
      const isProxy = Boolean(ev.geo && (ev.geo.is_proxy || ev.geo.is_vpn || ev.geo.is_tor));
      let geoTdHtml;
      if (ev.geo) {
        const locText = compactGeoLabel(ev.geo);
        geoTdHtml = `<span class="traffic-pill traffic-pill-geo" title="${escapeHtml(ev.geo.formatted || locText)}">${escapeHtml(locText)}</span>${isProxy ? ' <span class="traffic-pill traffic-pill-danger" style="font-size:9px;padding:0 4px;">Proxy</span>' : ''}`;
      } else if (ev.client_ip && ev.client_ip !== '-') {
        // 内网地址由后端 MatchSpecialIP 直接给出 formatted，走到这里说明
        // 该 IP 尚未完成解析或本就取不到地域：用骨架占位等待下一次轮询。
        geoTdHtml = '<span class="ui-skeleton ui-skeleton-pill" style="width:92px;height:20px;" aria-label="地域解析中"></span>';
      } else {
        geoTdHtml = '<span style="color:var(--text-dim); font-size:11.5px;">—</span>';
      }

      const asnText = ev.geo && ev.geo.asn ? `AS${ev.geo.asn} ${ev.geo.asn_org || ''}`.trim() : (ev.geo?.asn_org || '—');
      let asnTdHtml = asnText !== '—'
        ? `<span style="color:var(--text-muted);font-size:11px;font-family:var(--mono, monospace);max-width:140px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;display:inline-block;" title="${escapeHtml(asnText)}">${escapeHtml(asnText)}</span>`
        : '<span style="color:var(--text-dim); font-size:11.5px;">—</span>';

      return `<tr${rowClassAttr} data-id="${escapeHtml(String(ev.id || ''))}">
        <td class="num" style="color:var(--text-dim); font-size:11.5px;" title="${escapeHtml(fullIso ? `${fullIso} (UTC) · CST (UTC+8)` : '')}">
          <div class="live-cell-content"><span style="color:var(--text-muted); margin-right:3px;">${dateStr}</span>${timeStr}</div>
        </td>
        <td><div class="live-cell-content">${ipTdHtml}</div></td>
        <td><div class="live-cell-content">${geoTdHtml}</div></td>
        <td><div class="live-cell-content">${asnTdHtml}</div></td>
        <td>
          <div class="live-cell-content">
            <div class="key-badge" style="--key-bg:${ks.bg}; --key-border:${ks.border}; --key-color:${ks.color};"
                 data-target="${escapeHtml(String(filterKeyTarget))}"
                 onclick="filterBySpecificKey(this.dataset.target)"
                 title="${t('logs.filterByKeyHint')}">
              <span>${escapeHtml(mainKeyText)}</span>
            </div>
            <div class="key-display" style="font-size:10px; color:var(--text-dim); margin-top:2px; height:14px; line-height:14px;">${escapeHtml(subKeyText)}</div>
          </div>
        </td>
        <td><div class="live-cell-content">${authTdHtml}</div></td>
        <td><div class="live-cell-content">
          <span class="tag-pill" title="${escapeHtml(ev.model)}">${escapeHtml(ev.model || '-')}</span>
          ${(ev.request_model && ev.request_model !== ev.model) ? `<div style="font-size:10px; color:var(--text-dim); margin-top:2px;" title="客户端请求模型: ${escapeHtml(ev.request_model)}">(${escapeHtml(ev.request_model)})</div>` : ''}
        </div></td>
        <td><div class="live-cell-content">${reasoningHtml}</div></td>
        <td><div class="live-cell-content"><span class="chip ${statusClass}" title="${escapeHtml(statusTitle)}">${ev.status}</span></div></td>
        <td><div class="live-cell-content">${tpsHtml}</div></td>
        <td class="num"><div class="live-cell-content">${(ev.prompt_tokens || 0).toLocaleString(LOCALE)} / <b>${(ev.completion_tokens || 0).toLocaleString(LOCALE)}</b></div></td>
        <td class="num"><div class="live-cell-content">${cacheTag}</div></td>
        <td><div class="live-cell-content">${costTag}</div></td>
        <td><div class="live-cell-content">${latencyHtml}</div></td>
        <td><div class="live-cell-content">${flags.join('') || '—'}</div></td>
      </tr>`;
    }

    // 把后端的三段式地域收敛为两段，供窄列表格单行展示。
    // 国内（country 为「中国」）省去国家名，海外保留「国家 · 城市」。
    function compactGeoLabel(geo) {
      const formatted = (geo && geo.formatted) || '';
      const country = (geo && geo.country) || '';
      const region = (geo && geo.region) || '';
      const city = (geo && geo.city) || '';

      const segs = formatted ? formatted.split(/\s*·\s*/).filter(Boolean) : [];
      const isCN = country === '中国' || country === 'China' || country === 'CN';
      const hasCountryFirst = segs.length > 0 && country && segs[0] === country;

      if (segs.length >= 2) {
        if (isCN && hasCountryFirst) {
          // 中国 · 湖南 · 株洲 -> 湖南 · 株洲
          return segs.slice(1).slice(0, 2).join(' · ');
        }
        // 日本 · 东京都 · 千代田 -> 日本 · 千代田（省掉与城市同级的中间层）
        if (segs.length >= 3) return `${segs[0]} · ${segs[segs.length - 1]}`;
        return segs.join(' · ');
      }
      if (segs.length === 1) return segs[0];

      // formatted 缺失时退回结构化字段
      if (isCN) return [region, city].filter(Boolean).join(' · ') || '未知地域';
      return [country, city || region].filter(Boolean).join(' · ') || '未知地域';
    }

    function renderEventsTable(events, total, newlyAddedIds = null) {
      const pageEvents = Array.isArray(events) ? events : [];
      const totalFiltered = Number.isFinite(total) ? total : pageEvents.length;
      const totalPages = Math.max(1, Math.ceil(totalFiltered / logsPageSize));
      if (logsPage > totalPages) logsPage = totalPages;

      const pageInfo = document.getElementById('logs-page-info');
      const prevBtn = document.getElementById('logs-prev-btn');
      const nextBtn = document.getElementById('logs-next-btn');
      if (pageInfo) pageInfo.innerText = t('logs.pageCounter', { logsPage: logsPage, totalPages: totalPages, totalFiltered: totalFiltered });
      if (prevBtn) prevBtn.disabled = logsPage <= 1;
      if (nextBtn) nextBtn.disabled = logsPage >= totalPages;

      const tbody = document.getElementById('events-tbody');
      if (!tbody) return;
      if (totalFiltered === 0 || pageEvents.length === 0) {
        tbody.innerHTML = `<tr><td colspan="15" style="text-align: center; color: var(--text-dim); padding: 32px;">${t('logs.emptyFiltered')}</td></tr>`;
        return;
      }

      // 平滑无感增量插入：当在第1页且检测到有新增行时，仅向顶部平滑插入新行，未变动行保持不动
      const hasExistingRows = tbody.children.length > 0 && !tbody.querySelector('td[colspan]');
      if (logsPage === 1 && newlyAddedIds && newlyAddedIds.size > 0 && hasExistingRows) {
        const eventsToInsert = pageEvents.filter(ev => newlyAddedIds.has(ev.id) && !tbody.querySelector(`tr[data-id="${ev.id}"]`));
        if (eventsToInsert.length > 0) {
          const temp = document.createElement('tbody');
          let newRowsHtml = '';
          for (const ev of eventsToInsert) {
            newRowsHtml += renderEventRowHtml(ev, true);
          }
          temp.innerHTML = newRowsHtml;
          const rowsToInsert = Array.from(temp.children);
          for (let i = rowsToInsert.length - 1; i >= 0; i--) {
            tbody.insertBefore(rowsToInsert[i], tbody.firstElementChild);
          }
          while (tbody.children.length > logsPageSize) {
            tbody.lastElementChild.remove();
          }
          setTimeout(() => {
            rowsToInsert.forEach(row => {
              row.classList.remove('live-row-enter');
              row.classList.remove('live-row-highlight');
            });
          }, 2600);
          return;
        }
      }

      let html = '';
      for (const ev of pageEvents) {
        const isNewRow = newlyAddedIds && newlyAddedIds.has(ev.id);
        html += renderEventRowHtml(ev, isNewRow);
      }
      tbody.innerHTML = html;
      if (newlyAddedIds && newlyAddedIds.size > 0) {
        setTimeout(() => {
          tbody.querySelectorAll('.live-row-enter, .live-row-highlight').forEach(row => {
            row.classList.remove('live-row-enter');
            row.classList.remove('live-row-highlight');
          });
        }, 2600);
      }
    }

