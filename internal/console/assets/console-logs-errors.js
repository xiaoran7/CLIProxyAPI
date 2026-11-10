/**
 * Gate Admin Console - Error & Circuit Breaker Center
 * 从 console-logs.js 拆出：错误事件拉取、分页与熔断中心表格渲染。
 */

    function errorTypeLabel(type) {
      if (!type) return 'error';
      const map = {
        quota_exceeded: t('logs.errorQuotaExceeded'),
        rate_limited: t('logs.errorRateLimited'),
        model_forbidden: t('logs.errorModelForbidden'),
        empty_response: t('logs.errorEmptyResponse'),
        stream_interrupted: t('logs.errorStreamInterrupted'),
        connection_error: t('logs.errorConnectionLost'),
        stream_loop_detected: t('logs.errorThinkingLoopBreaker'),
        payload_too_large: t('logs.errorPayloadTooLarge'),
        upstream_validation_required: t('logs.errorUpstreamVerification'),
        invalid_api_key: t('logs.errorInvalidKey'),
        gateway_auth_failed: t('logs.errorGatewayAuthFailed'),
        gateway_forbidden: t('logs.errorGatewayForbidden'),
        client_closed: t('logs.errorClientClosed')
      };
      return map[type] || type;
    }

    // ---- 错误与熔断中心 (Error Events & Circuit Breaker, 支持 SWR 缓存) ----
    async function fetchErrorsData(force = false) {
      const secret = getAdminSecret();
      if (!secret) return;
      const errorsTbody = document.getElementById('errors-tbody');
      const errorsHaveRealRows = errorsTbody ? errorsTbody.querySelector('tr:not(.ui-skeleton-row)') !== null : false;
      if (errorsTbody && !errorsHaveRealRows && typeof window.renderSkeletonTableBody === 'function') {
        window.renderSkeletonTableBody(errorsTbody, errorsPageSize, 8);
      }
      const params = new URLSearchParams({
        page: String(errorsPage),
        page_size: String(errorsPageSize)
      });
      if (errorsTimeRange && errorsTimeRange !== 'all') params.set('time_range', errorsTimeRange);

      const cacheKey = params.toString();
      const cached = getErrorsQueryCache(cacheKey);
      if (cached && !force) {
        cachedErrors = cached.data.errors || [];
        cachedErrorsTotal = Number.isFinite(cached.data.total) ? cached.data.total : cachedErrors.length;
        if (Number.isFinite(cached.data?.today_total) && typeof updateNavBadgeErrors === 'function') {
          updateNavBadgeErrors(cached.data.today_total);
        }
        renderErrorsTable(cachedErrors, cachedErrorsTotal);
        if (Date.now() - cached.ts < 4000) return;
      }

      try {
        const res = await fetch(`/console/api/errors?${params.toString()}`, { headers: getAuthHeaders() });
        if (!res.ok) return;
        const data = await res.json();
        setErrorsQueryCache(cacheKey, data);
        cachedErrors = data.errors || [];
        cachedErrorsTotal = Number.isFinite(data.total) ? data.total : cachedErrors.length;
        if (Number.isFinite(data?.today_total) && typeof updateNavBadgeErrors === 'function') {
          updateNavBadgeErrors(data.today_total);
        }
        if (errorsPage === 1 && (!errorsTimeRange || errorsTimeRange === 'all')) {
          try {
            localStorage.setItem(KEY_STORAGE_ERRORS, JSON.stringify({ errors: cachedErrors.slice(0, 50), total: cachedErrorsTotal, ts: Date.now() }));
          } catch {}
        }
        renderErrorsTable(cachedErrors, cachedErrorsTotal);
      } catch (err) {
        console.warn('Errors fetch error:', err.message);
      }
    }

    function prevErrorsPage() {
      if (errorsPage > 1) {
        errorsPage--;
        fetchErrorsData();
      }
    }

    function nextErrorsPage() {
      const totalPages = Math.max(1, Math.ceil(cachedErrorsTotal / errorsPageSize));
      if (errorsPage < totalPages) {
        errorsPage++;
        fetchErrorsData();
      }
    }

    function changeErrorsPageSize(size) {
      errorsPageSize = Number(size) || 25;
      errorsPage = 1;
      fetchErrorsData();
    }

    function changeErrorsTimeRange(val) {
      errorsTimeRange = val || 'all';
      errorsPage = 1;
      fetchErrorsData();
    }

    function renderErrorsTable(errors, total) {
      const totalFiltered = Number.isFinite(total) ? total : (errors ? errors.length : 0);
      const totalPages = Math.max(1, Math.ceil(totalFiltered / errorsPageSize));
      if (errorsPage > totalPages) errorsPage = totalPages;

      const pageInfo = document.getElementById('errors-page-info');
      const prevBtn = document.getElementById('errors-prev-btn');
      const nextBtn = document.getElementById('errors-next-btn');
      if (pageInfo) pageInfo.innerText = t('logs.errorsPageCounter', { errorsPage: errorsPage, totalPages: totalPages, totalFiltered: totalFiltered });
      if (prevBtn) prevBtn.disabled = errorsPage <= 1;
      if (nextBtn) nextBtn.disabled = errorsPage >= totalPages;

      const tbody = document.getElementById('errors-tbody');
      if (!tbody) return;
      if (!errors || errors.length === 0 || totalFiltered === 0) {
        tbody.innerHTML = `<tr><td colspan="8" style="text-align: center; color: var(--text-dim); padding: 32px;">${t('logs.emptyIncidents')}</td></tr>`;
        return;
      }

      const now = Date.now();
      let html = '';
      for (const ev of errors) {
        // 兼容自适应秒级或毫秒级时间戳，杜绝重复乘 1000 导致时间错乱与溢出
        const tsMs = ev.ts > 1e11 ? ev.ts : ev.ts * 1000;
        const cst = typeof getCstParts === 'function' ? getCstParts(tsMs) : null;
        const d = cst ? cst.d : new Date(tsMs);
        const dateStr = cst ? cst.dateStr : `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
        const timeStr = cst ? cst.timeStr : d.toLocaleTimeString(LOCALE, { hour12: false, timeZone: 'Asia/Shanghai' });
        const fullTimeStr = cst ? cst.fullDateTimeStr : `${dateStr} ${timeStr}`;
        const keyInfo = ev.key_alias ? `${escapeHtml(ev.key_alias)} (#${ev.key_id || '—'})` : (ev.key_id ? `#${ev.key_id}` : '—');

        let statusClass = 'chip-red';
        if (ev.status === 429) statusClass = 'chip-yellow';

        // 熔断与退避倒计时
        let circuitStatus = `<span style="color:var(--text-dim);">${t('logs.statusEnded')}</span>`;
        if (ev.recover_at && ev.recover_at > now) {
          const remainSec = Math.ceil((ev.recover_at - now) / 1000);
          circuitStatus = `<span class="chip chip-red" style="font-weight:600;">${t('logs.circuitBrokenChip', { remainSec: remainSec })}</span>`;
        } else if (ev.retry_after_ms > 0) {
          circuitStatus = `<span class="chip chip-gray">${t('logs.backoffChip', { value: Math.round(ev.retry_after_ms / 1000) })}s</span>`;
        }

        // 上游账号与凭据身份直观展示
        const shortAuth = ev.auth_index ? ev.auth_index.slice(0, 16) : '';
        const shortId = ev.auth_index ? ev.auth_index.slice(0, 8) : '';
        let authName = ev.auth_name || (ev.auth_index ? getAuthDisplayName(ev.auth_index) : '') || '';
        if (authName && typeof authName === 'string') {
          authName = authName.replace(/\.json$/i, '');
        }
        const brand = getProviderBrand(resolveEventProvider(ev), authName);
        if (ev.auth_index && (!authName || authName === ev.auth_index)) {
          authName = (brand && brand.id !== 'other')
            ? t('logs.channelByShortId', { name: brand.name, shortId: shortId })
            : t('logs.deletedAccountByShortId', { shortId: shortId });
        }
        let authDisplay = '<span style="color:var(--text-dim);">—</span>';
        if (ev.auth_index) {
          authDisplay = `
            <div style="display:flex; flex-direction:column; gap:2px;">
              <div style="display:flex; align-items:center; gap:5px;">
                <span class="provider-pill ${brand.pillClass}" style="font-size:9.5px; padding:0 4px;">${escapeHtml(brand.name)}</span>
                <span style="font-weight:600; color:var(--text-main); font-size:12px; max-width:210px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${escapeHtml(authName || ev.auth_index)}">
                  ${escapeHtml(authName || ev.auth_index)}
                </span>
              </div>
              <div style="font-size:10.5px; font-family:var(--mono); color:var(--text-dim);">ID: ${escapeHtml(shortAuth)}</div>
            </div>
          `;
        }

        let bodySnippet = ev.error_body ? escapeHtml(ev.error_body.slice(0, 200)) + (ev.error_body.length > 200 ? '…' : '') : '—';

        if (ev.error_type === 'upstream_validation_required') {
          statusClass = 'chip-red';
          const urlMatch = (ev.error_body || '').match(/https:\/\/accounts\.google\.com\/signin\/continue\?[^\s"'\\]+/);
          if (urlMatch) {
            const rawUrl = urlMatch[0];
            bodySnippet = `<span style="color:var(--danger); font-weight:700;">${t('logs.googleVerificationBlocked')}</span> · <a href="${escapeHtml(rawUrl)}" target="_blank" rel="noopener noreferrer" style="color:#2563eb; font-weight:700; text-decoration:underline;">${t('logs.openVerificationPageLink')}</a>`;
          }
        }

        html += `<tr>
          <td class="num" style="color:var(--text-dim); font-size:11.5px; white-space:nowrap;" title="${d.toISOString()}">${fullTimeStr}</td>
          <td>${keyInfo}</td>
          <td>${authDisplay}</td>
          <td><span class="tag-pill">${escapeHtml(ev.model || '—')}</span></td>
          <td><span class="chip ${statusClass}">${ev.status}</span></td>
          <td><span class="flag-pill" style="${ev.error_type === 'upstream_validation_required' ? 'background:rgba(239,68,68,0.15); color:var(--danger); border-color:rgba(239,68,68,0.4);' : ''}" title="${escapeHtml(ev.error_type || '')}">${escapeHtml(errorTypeLabel(ev.error_type))}</span></td>
          <td>${circuitStatus}</td>
          <td style="max-width:320px; font-size:11px; color:var(--text-muted); line-height:1.4; word-break:break-all;">${bodySnippet}</td>
        </tr>`;
      }
      tbody.innerHTML = html;
    }
