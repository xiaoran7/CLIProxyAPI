/**
 * Gate Admin Console - 总览页 · 共享基础件与图表簇
 * 从 console-overview.js 拆出：状态/格式化/鉴权头等共享基础件，以及吞吐图、迷你图与提示框渲染。
 * 下游需要的符号统一经 window.GATE_OVW 导出。
 */
(() => {
  'use strict';

  const PAGE_ID = 'runtime';
  const $ = id => document.getElementById(id);
  const escapeHtml = value => String(value ?? '').replace(/[&<>"']/g, c =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);

  const DEFAULT_WINDOW = '1h';
  const getSavedWindow = () => {
    try {
      const saved = localStorage.getItem('gate_ovw_window');
      if (saved === '3h20m') return '1h';
      if (saved === '5h') return '6h';
      if (['1h', '6h', '1d', '3d', '7d', '30d'].includes(saved)) return saved;
    } catch (_) {}
    return DEFAULT_WINDOW;
  };

  const state = {
    facts: null,
    config: null,
    loading: false,
    lastError: null,
    pollMs: 0,
    window: getSavedWindow()
  };

  function authHeaders() {
    const secret = typeof getAdminSecret === 'function' ? getAdminSecret() : '';
    return secret ? { Authorization: 'Bearer ' + secret } : {};
  }

  function setText(id, value) {
    const el = $(id);
    if (el) el.textContent = value;
  }

  function fmtInt(n) {
    if (n == null || !Number.isFinite(Number(n))) return '--';
    return Math.round(Number(n)).toLocaleString(LOCALE);
  }

  function fmtPct(n) {
    if (n == null || !Number.isFinite(Number(n))) return '--';
    return `${Number(n).toFixed(2)}%`;
  }

  function fmtWindow(minutes) {
    const m = Number(minutes) || 0;
    if (m <= 0) return t('overview.runtimeWindowUnknown');
    if (m < 60) return t('overview.runtimeWindowMinutes', { m: m });
    const hours = Math.floor(m / 60);
    const rest = m % 60;
    if (hours >= 24 && rest === 0 && hours % 24 === 0) {
      return t('overview.runtimeWindowDays', { value: hours / 24 });
    }
    return rest ? t('overview.runtimeWindowHoursMinutes', { hours: hours, rest: rest }) : t('overview.runtimeWindowHours', { hours: hours });
  }

  function fmtBucketSpan(minutes) {
    const m = Number(minutes) || 10;
    if (m < 60) return t('overview.runtimeBucketSpanMinutes', { m: m });
    const hours = Math.floor(m / 60);
    const rest = m % 60;
    return rest ? t('overview.runtimeBucketSpanHoursMinutes', { hours: hours, rest: rest }) : t('overview.runtimeBucketSpanHours', { hours: hours });
  }

  // 判定口径与 CPA 面板一致：无流量单独成态，不按 0% 失败率误判为告警。
  function verdictOf(totals) {
    if (!totals) return { key: 'idle', label: t('status.runtimeVerdictIdle') };
    const total = totals.total_requests || 0;
    if (total === 0) return { key: 'idle', label: t('status.runtimeVerdictIdle') };
    const rate = totals.success_rate;
    if (rate == null) return { key: 'idle', label: t('status.runtimeVerdictIdle') };
    if (rate >= 99) return { key: 'good', label: t('stats.requestsSteady') };
    if (rate >= 95) return { key: 'warning', label: t('status.runtimeVerdictWarning') };
    return { key: 'critical', label: t('status.runtimeVerdictCritical') };
  }

  function sparkline(buckets, opts = {}) {
    const width = opts.width || 260;
    const height = opts.height || 48;
    const values = (buckets || []).map(b => (b.success || 0) + (b.failed || 0));
    if (values.length === 0) return '';
    const max = Math.max(1, ...values);
    const step = width / Math.max(values.length - 1, 1);
    const y = v => height - 3 - (v / max) * (height - 8);
    const points = values.map((v, i) => `${(i * step).toFixed(2)},${y(v).toFixed(2)}`);
    const area = `0,${height} ` + points.join(' ') + ` ${width},${height}`;
    const stroke = opts.stroke || '#38bdf8';
    return `<svg viewBox="0 0 ${width} ${height}" preserveAspectRatio="none" style="width:100%;height:${height}px" aria-hidden="true">
      <polygon points="${area}" fill="${stroke}" fill-opacity="0.12"></polygon>
      <polyline points="${points.join(' ')}" fill="none" stroke="${stroke}" stroke-width="1.8" stroke-linejoin="round" stroke-linecap="round"></polyline>
    </svg>`;
  }

  let sharedOvwTooltip = null;
  function getOvwTooltip() {
    if (!sharedOvwTooltip) {
      sharedOvwTooltip = document.createElement('div');
      sharedOvwTooltip.className = 'ovw-tooltip';
      document.body.appendChild(sharedOvwTooltip);
    }
    return sharedOvwTooltip;
  }

  function positionTooltip(e, tip) {
    const pad = 12;
    let left = e.clientX + pad;
    let top = e.clientY + pad;
    const rect = tip.getBoundingClientRect();
    if (left + rect.width > window.innerWidth - pad) {
      left = e.clientX - rect.width - pad;
    }
    if (top + rect.height > window.innerHeight - pad) {
      top = e.clientY - rect.height - pad;
    }
    tip.style.left = Math.max(pad, left) + 'px';
    tip.style.top = Math.max(pad, top) + 'px';
  }

  function bucketTooltipHtml(bucket, facts) {
    const sum = (bucket.success || 0) + (bucket.failed || 0);
    const bucketMins = facts?.bucket_minutes || facts?.totals?.bucket_minutes || 10;
    let timeLabel = escapeHtml(bucket.time || t('overview.runtimeBucketTimeFallback'));
    if (bucketMins >= 1440) {
      timeLabel = t('overview.runtimeChartBucketAllDay', { value: escapeHtml(bucket.time) });
    }
    if (sum === 0) {
      return `<div style="font-size:12px;font-weight:700;color:#1e293b;margin-bottom:3px;">${timeLabel}</div><div style="color:#64748b;font-size:11.5px;">${t('chart.runtimeBucketNoTraffic')}</div>`;
    }
    const rate = sum > 0 ? ((bucket.success / sum) * 100).toFixed(1) : '100.0';
    let failHtml = '';
    if (bucket.failed > 0) {
      // 归集具体发生失败的凭据与提供商
      const failedCreds = (facts?.credentials || []).map(c => {
        const b = (c.buckets || []).find(item => item.time === bucket.time);
        return {
          name: c.name || c.email || c.provider,
          email: c.email || '',
          provider: c.provider || 'unknown',
          failed: b ? b.failed : 0
        };
      }).filter(c => c.failed > 0);

      const provMap = new Map();
      for (const fc of failedCreds) {
        const item = provMap.get(fc.provider) || { total: 0, count: 0, names: [] };
        item.total += fc.failed;
        item.count += 1;
        if (item.names.length < 2) item.names.push(fc.name);
        provMap.set(fc.provider, item);
      }

      const provItems = Array.from(provMap.entries()).map(([p, info]) => {
        const brand = typeof getProviderBrand === 'function' ? getProviderBrand(p) : { name: p, pillClass: 'provider-pill-default' };
        const shortNames = info.names.length > 0
          ? `<div style="font-size:10px;font-family:var(--mono, monospace);color:#64748b;margin-top:2px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${t('overview.runtimeAccountPrefix', { value: escapeHtml(info.names.map(n => n.replace(/\.json$/i, '')).join(', ')) })}${info.count > info.names.length ? ` ${t('overview.runtimeAccountEtcSuffix')}` : ''}</div>`
          : '';
        return `<div style="padding:4px 8px;background:#fef2f2;border:1px solid #fecaca;border-radius:6px;margin-top:4px;">
          <div style="display:flex;align-items:center;justify-content:space-between;gap:6px;">
            <span class="provider-pill ${brand.pillClass}" style="font-size:9.5px;padding:0 5px;">${escapeHtml(brand.name || p)}</span>
            <span style="color:#dc2626;font-weight:700;font-family:var(--mono, monospace);font-size:11px;">${t('overview.runtimeAnomalyCountSuffix', { total: info.total })}</span>
          </div>
          ${shortNames}
        </div>`;
      }).join('');

      failHtml = `
        <div style="margin-top:6px;">
          <div style="font-size:11px;font-weight:600;color:#991b1b;">${t('overview.runtimeAnomalyAttribution', { count: failedCreds.length })}</div>
          ${provItems || `<div style="font-size:11px;color:#dc2626;">${t('overview.runtimeUpstreamAnomalyTotal', { failed: bucket.failed })}</div>`}
        </div>
        <div style="margin-top:8px;padding-top:6px;border-top:1px dashed #e2e8f0;color:#2563eb;font-weight:600;font-size:11px;display:flex;align-items:center;justify-content:space-between;">
          <span>${t('chart.runtimeClickBarRequestLog')}</span>
          <span style="font-family:var(--mono, monospace);">↗</span>
        </div>
      `;
    } else {
      failHtml = `
        <div style="margin-top:6px;font-size:11px;color:#059669;font-weight:600;">
          ${t('chart.runtimeAllRequestsSucceeded')}
        </div>
        <div style="margin-top:6px;padding-top:5px;border-top:1px dashed #e2e8f0;color:#2563eb;font-size:11px;">
          ${t('chart.runtimeClickBarWindowLog')}
        </div>
      `;
    }

    const windowTag = bucketMins < 60 ? `${bucketMins}m window` : (bucketMins >= 1440 ? `${bucketMins / 1440}d window` : `${bucketMins / 60}h window`);

    return `
      <div style="font-size:12.5px;font-weight:700;margin-bottom:5px;color:#0f172a;display:flex;align-items:center;justify-content:space-between;">
        <span>${timeLabel}</span>
        <span style="font-size:10.5px;color:#64748b;font-weight:400;font-family:var(--mono, monospace);">${windowTag}</span>
      </div>
      <div style="display:flex;align-items:center;gap:8px;font-size:11.5px;margin-bottom:4px;padding-bottom:5px;border-bottom:1px solid #f1f5f9;">
        <span>${t('overview.runtimeSuccessLabel')} <b style="color:#059669;">${fmtInt(bucket.success)}</b></span>
        <span>${t('overview.runtimeFailureLabel')} <b style="color:${bucket.failed > 0 ? '#dc2626' : '#94a3b8'};">${fmtInt(bucket.failed)}</b></span>
        <span style="margin-left:auto;color:#475569;">${t('stats.successRate')} <b style="color:${bucket.failed > 0 ? '#b45309' : '#059669'};">${rate}%</b></span>
      </div>
      ${failHtml}
    `;
  }

  window.ovwJumpToErrorLogs = function(timeStr, failCount) {
    if (sharedOvwTooltip) sharedOvwTooltip.classList.remove('is-visible');
    if (typeof window.jumpToErrorLogs === 'function') {
      window.jumpToErrorLogs(timeStr, failCount);
    } else if (typeof switchNavTab === 'function') {
      switchNavTab('logs');
      if (typeof filterEvents === 'function') {
        filterEvents(failCount > 0 ? 'errors' : 'all');
      }
      if (typeof showToast === 'function') {
        showToast(failCount > 0 ? `${t('logs.jumpedToErrorLogs')}${timeStr ? ` ${t('overview.runtimeTimeRangeHint', { timeStr: timeStr })}` : ''}` : `${t('overview.runtimeJumpedToRequestLog')}${timeStr ? ` ${t('overview.runtimeTimeRangeHint', { timeStr: timeStr })}` : ''}`, 'info');
      }
    }
  };

  function renderThroughputChart(facts) {
    const holder = $('ovw-chart-holder');
    if (!holder) return;
    const totals = facts?.totals;
    const buckets = totals ? aggregateBuckets(facts) : [];

    if (!buckets.length) {
      holder.innerHTML = `<div class="ovw-empty">${t('chart.runtimeEmptyNoTrafficHint')}</div>`;
      return;
    }

    const width = 1000;
    const height = 190;
    const padL = 46, padR = 8, padT = 14, padB = 26;
    const innerW = width - padL - padR;
    const innerH = height - padT - padB;

    // 纵轴上限取「好看的」刻度，且尽量贴近峰值：0 / 半轴 / 上限 三档都是整数，
    // 柱高不会只占画布的一半。原实现直接用 max/2，max=118 时中线落在 59 这种
    // 没人读得懂的数上。取值规则见 niceCeil。
    const peak = Math.max(0, ...buckets.map(b => b.success + b.failed));
    const niceMax = niceCeil(peak);
    const slot = innerW / buckets.length;
    const barW = Math.max(3, Math.min(22, slot * 0.62));

    const bars = buckets.map((b, i) => {
      const x = padL + i * slot + (slot - barW) / 2;
      const hSuccess = niceMax > 0 ? (b.success / niceMax) * innerH : 0;
      const hFailed = niceMax > 0 ? (b.failed / niceMax) * innerH : 0;
      const ySuccess = padT + innerH - hSuccess;
      const yFailed = ySuccess - hFailed;
      const hasFailed = b.failed > 0;
      return `<g class="ovw-chart-bar ${hasFailed ? 'has-failed' : ''}" data-bucket-idx="${i}" tabindex="0" role="button" aria-label="${t('overview.runtimeChartBucketSummary', { value: escapeHtml(b.time), success: b.success, failed: b.failed })}">
        <rect x="${x.toFixed(1)}" y="${padT}" width="${barW.toFixed(1)}" height="${innerH}" fill="transparent"></rect>
        ${hSuccess > 0 ? `<rect x="${x.toFixed(1)}" y="${ySuccess.toFixed(1)}" width="${barW.toFixed(1)}" height="${Math.max(hSuccess, 1).toFixed(1)}" fill="#10b981" rx="1.5"></rect>` : ''}
        ${hFailed > 0 ? `<rect class="ovw-chart-bar-failed" x="${x.toFixed(1)}" y="${yFailed.toFixed(1)}" width="${barW.toFixed(1)}" height="${Math.max(hFailed, 1).toFixed(1)}" fill="#ef4444" rx="1.5"></rect>` : ''}
      </g>`;
    }).join('');

    const tickValues = [0, niceMax / 2, niceMax];
    const ticks = tickValues.map(v => {
      const y = padT + innerH - (v / niceMax) * innerH;
      const isBase = v === 0;
      return `<line x1="${padL}" y1="${y.toFixed(1)}" x2="${width - padR}" y2="${y.toFixed(1)}" stroke="${isBase ? '#c3d8ea' : '#e9f2fa'}" stroke-width="${isBase ? 1.2 : 1}"></line>
              <text class="ovw-chart-axis" x="${padL - 8}" y="${(y + 3.5).toFixed(1)}" text-anchor="end">${fmtInt(Math.round(v))}</text>`;
    }).join('');

    const bucketMinutes = facts?.bucket_minutes || totals?.bucket_minutes || 10;
    const bucketSpanText = fmtBucketSpan(bucketMinutes);

    // 动态生成均匀美观的 X 轴刻度点（5 ~ 8 个标签，避免拥挤，杜绝单调 3 点）
    const len = buckets.length;
    let labelIndices = [];
    if (len <= 10) {
      labelIndices = Array.from({ length: len }, (_, i) => i);
    } else if (len === 12) { // 1h (每5分钟)
      labelIndices = [0, 2, 4, 6, 8, 10, 11];
    } else if (len === 20) { // 3h20m (每10分钟, legacy)
      labelIndices = [0, 5, 10, 15, 19];
    } else if (len === 24) { // 6h (每15分钟), 1d (每小时) 或 3d (每3小时)
      labelIndices = [0, 4, 8, 12, 16, 20, 23];
    } else if (len === 28) { // 7d (每天4桶，每6小时)
      labelIndices = [0, 4, 8, 12, 16, 20, 24, 27];
    } else if (len === 30) { // 30d (每天1桶) 或 5h (每10分钟, legacy)
      labelIndices = [0, 5, 10, 15, 20, 25, 29];
    } else {
      const step = Math.max(1, Math.floor(len / 6));
      for (let i = 0; i < len; i += step) labelIndices.push(i);
      if (labelIndices[labelIndices.length - 1] !== len - 1) labelIndices.push(len - 1);
    }

    const xLabels = labelIndices.map((idx, pos) => {
      const fullTime = buckets[idx]?.time || '';
      let startLabel = '';
      if (bucketMinutes >= 1440) {
        // 每日桶：直接显示月日（如 "09-18", "10-17"）
        const m = fullTime.match(/^(\d{2}-\d{2})/);
        startLabel = m ? m[1] : fullTime.split(' ')[0] || fullTime;
      } else if (bucketMinutes >= 60) {
        // 小时级桶（如 1d, 3d, 7d）
        if (bucketMinutes === 60) {
          // 1d: 提取纯时分 "HH:mm"
          const m = fullTime.match(/\b(\d{2}:\d{2})\b/);
          startLabel = m ? m[1] : fullTime.split('-')[0] || '';
        } else if (bucketMinutes === 360) {
          // 7d: 提取日期 "MM-DD"
          const m = fullTime.match(/^(\d{2}-\d{2})/);
          startLabel = m ? m[1] : fullTime.split(' ')[0] || '';
        } else {
          // 3d 等多小时级: 提取 "MM-DD HH:mm" 或 "MM-DD"
          const m = fullTime.match(/^(\d{2}-\d{2}\s+\d{2}:\d{2})/);
          startLabel = m ? m[1] : fullTime.split('-')[0] || '';
        }
      } else {
        // 分钟级桶 (<60m): 提取起始时分 "HH:mm"
        const m = fullTime.match(/^(\d{2}:\d{2})/);
        startLabel = m ? m[1] : fullTime.split('-')[0] || '';
      }

      let anchor = 'middle';
      let xPos = padL + idx * slot + slot / 2;
      if (pos === 0) {
        anchor = 'start';
        xPos = padL;
      } else if (pos === labelIndices.length - 1) {
        anchor = 'end';
        xPos = width - padR;
      }

      // 超窄移动端防挤：首尾刻度始终保留，中间奇数位置标签标记类名并在小屏媒体查询下隐藏
      const isSparseHide = pos % 2 === 1 && pos !== labelIndices.length - 1;
      const sparseClass = isSparseHide ? ' ovw-xaxis-sparse-hide' : '';

      return `<text class="ovw-chart-axis ovw-chart-xaxis${sparseClass}" x="${xPos.toFixed(1)}" y="${height - 7}" text-anchor="${anchor}">${escapeHtml(startLabel)}</text>`;
    }).join('');

    holder.innerHTML = `<svg class="ovw-chart" viewBox="0 0 ${width} ${height}" role="img" aria-label="${t('overview.runtimeChartThroughputTitle', { bucketSpanText: bucketSpanText })}">
      ${ticks}${bars}${xLabels}
    </svg>
    <div class="ovw-chart-legend">
      <span><i class="ovw-swatch is-success"></i> ${t('overview.runtimeSuccessLabel')}</span>
      <span><i class="ovw-swatch is-failure"></i> ${t('overview.runtimeFailureLabel')}</span>
      <span style="margin-left:auto">${t('overview.runtimeChartBucketLegendHint', { bucketSpanText: bucketSpanText })}</span>
    </div>`;

    const tip = getOvwTooltip();
    holder.querySelectorAll('.ovw-chart-bar').forEach(bar => {
      const idx = Number(bar.dataset.bucketIdx);
      const b = buckets[idx];
      if (!b) return;

      bar.addEventListener('mouseenter', e => {
        tip.innerHTML = bucketTooltipHtml(b, facts);
        tip.classList.add('is-visible');
        positionTooltip(e, tip);
      });

      bar.addEventListener('mousemove', e => {
        positionTooltip(e, tip);
      });

      bar.addEventListener('mouseleave', () => {
        tip.classList.remove('is-visible');
      });

      bar.addEventListener('click', () => {
        window.ovwJumpToErrorLogs(b.time, b.failed);
      });

      bar.addEventListener('keydown', e => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          window.ovwJumpToErrorLogs(b.time, b.failed);
        }
      });
    });
  }

  // 纵轴上限取「好看」的偶数，且尽量贴近峰值。
  //
  // 关键在于**不**用 top/2 当中线——那样只能在偶数里挑，峰值 110 只能跳到 200，
  // 柱高只占画布 55%，白白浪费一半纵向空间。改成先选一个「好看的半轴」h，
  // 再令上限 = 2h：中线恒为 h（本身就是整数刻度），上限恒为偶数。
  // 步长取得细，是为了让 110 → h=60 → 上限 120（92% 填充）而不是 200（55%）。
  function niceCeil(peak) {
    if (!(peak > 0)) return 2;
    // mag 由**半轴** peak/2 决定，不是由 peak 决定。用 peak 取数量级会让步进粗
    // 10 倍：峰值 110 的 mag 是 100，候选半轴从 100 起跳，上限直接变 200（55% 填充）；
    // 按 peak/2=55 取 mag=10 才能落到半轴 60 → 上限 120（92% 填充）。
    const ref = peak / 2;
    const mag = Math.max(1, Math.pow(10, Math.floor(Math.log10(ref))));
    // half = step×mag，必须是整数，否则 0/half/2half 三档刻度不可读
    // （个位数档只保留整数 step，×10 之后 1.2/1.5 这类小数才有意义）。
    const steps = mag < 10 ? [1, 2, 3, 4, 5, 6, 7, 8, 9, 10] : [1, 1.2, 1.5, 2, 2.5, 3, 3.5, 4, 5, 6, 7, 8, 10];
    for (const step of steps) {
      const half = step * mag;
      if (half * 2 >= peak) return half * 2;
    }
    return 20 * mag;
  }

  // 注：这里刻意不标「末桶进行中」。CPA 的桶是裸时间串 "19:20-19:30"，不带时区，
  // 而网关进程跑 UTC、CPA 出桶是 UTC+8，两者对不上且无法从 payload 判定末桶是否
  // 仍在累积——硬标就是猜，猜错会让柱高被误读成最终值。

  // 总览吞吐直接用服务端算好的桶：服务端已按同一套对齐规则求和过一遍，
  // 前端再重算一次就是两套口径，迟早对不上。
  function aggregateBuckets(facts) {
    return facts?.totals?.buckets || [];
  }


  window.GATE_OVW = {
    PAGE_ID,
    $,
    escapeHtml,
    DEFAULT_WINDOW,
    getSavedWindow,
    state,
    authHeaders,
    setText,
    fmtInt,
    fmtPct,
    fmtWindow,
    fmtBucketSpan,
    verdictOf,
    sparkline,
    getOvwTooltip,
    renderThroughputChart,
    aggregateBuckets,
  };
})();
