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

  function renderHero(facts) {
    const totals = facts?.totals;
    const verdict = verdictOf(totals);
    const node = $('ovw-verdict');
    if (node) {
      node.className = 'ovw-verdict' + (verdict.key === 'good' ? '' : ` is-${verdict.key}`);
      node.innerHTML = `<span class="dot"></span><span>${escapeHtml(verdict.label)}</span>`;
    }
    const bucketMinutes = facts?.bucket_minutes || totals?.bucket_minutes || 10;
    const bucketSpanText = fmtBucketSpan(bucketMinutes);

    setText('ovw-hero-value', fmtInt(totals?.total_requests));
    setText('ovw-hero-label', t('overview.runtimeHeroLabel'));
    setText('ovw-hero-meta', totals
      ? (totals.scope === 'cumulative'
        ? t('overview.runtimeCumulativeScopeNoTraffic', { value: fmtWindow(totals.window_minutes) })
        : `${t('overview.runtimeRollingWindow', { value: fmtWindow(totals.window_minutes) })} · ${bucketSpanText}`)
      : t('overview.runtimeAwaitingData'));
    setText('ovw-split-success', fmtInt(totals?.success_count));
    setText('ovw-split-failed', fmtInt(totals?.failure_count));

    const spark = $('ovw-hero-spark');
    if (spark) {
      const buckets = aggregateBuckets(facts);
      spark.innerHTML = buckets.length
        ? sparkline(buckets, { width: 260, height: 48 })
        : `<div class="ovw-empty" style="padding:12px 0">${t('overview.runtimeNoRequestVolume')}</div>`;
    }
  }

  function renderStats(facts) {
    const holder = $('ovw-stats');
    if (!holder) return;
    const totals = facts?.totals;
    const providers = facts?.providers || [];

    const successRate = totals?.success_rate ?? null;
    const rateClass = successRate == null ? '' : (successRate >= 99 ? ' is-good' : successRate >= 95 ? '' : ' is-bad');
    const providerKeys = providers.reduce((a, p) => a + (p.key_count || 0), 0);
    // 桶全空时服务端退回累计口径（scope=cumulative），提示语必须跟着换，
    // 否则会拿「窗口内共 N 次」去描述一个累计值。
    const scopeCumulative = totals?.scope === 'cumulative';

    const tiles = [
      {
        label: t('stats.successRate'),
        value: fmtPct(successRate),
        cls: rateClass,
        hint: totals
          ? (scopeCumulative
            ? t('overview.runtimeCumulativeNoTraffic', { value: fmtInt(totals.total_requests) })
            : `${t('stats.rangeLastPrefix')} ${fmtWindow(totals.window_minutes).replace(`${t('stats.rangeLastPrefix')} `, '')}${t('overview.runtimeRequestCountSuffix', { value: fmtInt(totals.total_requests) })}`)
          : t('stats.statusNoData'),
        meter: successRate == null ? null : Math.max(0, Math.min(100, successRate))
      },
      {
        label: t('overview.runtimeCredentialsLabel'),
        value: fmtInt(totals?.credentials_total),
        hint: totals ? t('overview.runtimeCredentialsStatusHint', { credentials_active: totals.credentials_active, credentials_disabled: totals.credentials_disabled }) : t('stats.statusNoData'),
        meter: null
      },
      {
        label: t('overview.runtimeProviderKeysLabel'),
        value: fmtInt(providerKeys),
        hint: providers.length ? t('overview.runtimeProviderCount', { count: providers.length }) : t('stats.statusNoData'),
        meter: null
      },
      {
        label: t('overview.runtimeCooldownLabel'),
        value: fmtInt(totals?.credentials_cooldown),
        cls: totals?.credentials_cooldown > 0 ? ' is-bad' : '',
        hint: totals?.credentials_blocked > 0 ? t('overview.runtimeNotSchedulableHint', { credentials_blocked: totals.credentials_blocked }) : t('overview.runtimeAllSchedulableHint'),
        meter: null
      }
    ];

    holder.innerHTML = tiles.map(t => `<div class="ovw-stat">
      <div class="ovw-stat-label">${escapeHtml(t.label)}</div>
      <div class="ovw-stat-value${t.cls || ''}">${escapeHtml(t.value)}</div>
      ${t.hint ? `<div class="ovw-stat-hint">${escapeHtml(t.hint)}</div>` : ''}
      ${t.meter != null ? `<div class="ovw-meter"><div class="ovw-meter-fill" style="width:${t.meter}%"></div></div>` : ''}
    </div>`).join('');
  }

  function renderFleet(facts) {
    const holder = $('ovw-fleet');
    if (!holder) return;
    const providers = facts?.providers || [];
    if (providers.length === 0) {
      holder.innerHTML = `<div class="ovw-empty">${t('overview.runtimeProviderNoTraffic')}</div>`;
      return;
    }
    const max = Math.max(1, ...providers.map(p => p.total || 0));
    holder.innerHTML = providers.map(p => {
      const rate = p.success_rate;
      const rateCls = rate == null ? '' : (rate >= 99 ? ' is-good' : rate >= 95 ? '' : ' is-bad');
      return `<div class="ovw-fleet-row">
        <div class="ovw-fleet-name" title="${escapeHtml(p.provider)}">${escapeHtml(p.provider)}</div>
        ${sparkline(p.buckets, { width: 300, height: 30, stroke: p.total >= max ? '#38bdf8' : '#a5c8e6' })}
        <div class="ovw-fleet-numbers">
          ${t('overview.runtimeProviderRequestSuffix', { value: fmtInt(p.total) })} <span class="ovw-fleet-rate${rateCls}">${fmtPct(rate)}</span>
        </div>
      </div>`;
    }).join('');
  }

  function readConfigValue(cfg, ...keys) {
    for (const k of keys) {
      const v = cfg?.[k];
      if (v !== undefined && v !== null && v !== '') return v;
    }
    return null;
  }

  // 选项文案取 CPA 面板的短标签：这一行只有四列栅格，长标签会把下拉框挤到裁切
  const ROUTING_STRATEGIES = [
    { value: 'round-robin', label: t('overview.runtimeStrategyRoundRobin') },
    { value: 'weighted-round-robin', label: t('overview.runtimeStrategyWeightedRoundRobin') },
    { value: 'fill-first', label: t('overview.runtimeStrategyFillFirst') },
    { value: 'smart', label: t('overview.runtimeStrategySmart') },
    { value: 'pace', label: t('overview.runtimeStrategyPace') }
  ];

  // 路由配置可编辑的前提与只读区块一致：管理员已配密钥，且不是成员视角
  function canEditRouting() {
    if (typeof isMemberAuth === 'function' && isMemberAuth()) return false;
    if (typeof getAdminSecret === 'function' && !getAdminSecret()) return false;
    return true;
  }

  function notify(message, type) {
    if (typeof showToast === 'function') showToast(message, type);
  }

  async function saveRouting(patch, control) {
    // 保存中禁用控件，避免重复提交把策略连写几次
    if (control) control.disabled = true;
    try {
      const res = await fetch('/console/api/upstream-routing', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', ...authHeaders() },
        body: JSON.stringify(patch)
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(body.error || `HTTP ${res.status}`);
      state.config = { ...(state.config || {}), routing: { ok: true, value: body.routing } };
      notify(t('route.runtimeConfigSaved'), 'success');
      return true;
    } catch (err) {
      // 回滚到服务端真值：界面不能停在「以为改了其实没改」的状态
      notify(t('overview.runtimeRoutingSaveFailed', { msg: err.message }), 'error');
      await fetchRuntimeConfig();
      return false;
    } finally {
      if (control) control.disabled = false;
    }
  }

  function renderRuntime() {
    const holder = $('ovw-runtime-grid');
    if (!holder) return;
    const cfg = state.config?.config;
    if (!cfg?.ok) {
      holder.innerHTML = `<div class="ovw-empty">${t('overview.runtimeConfigUnavailable', { value: state.config?.config?.error ? '：' + escapeHtml(state.config.config.error) : '' })}</div>`;
      return;
    }
    const v = cfg.value || {};
    const routing = state.config?.routing;
    const r = routing?.ok ? (routing.value || {}) : {};
    const version = state.config?.version?.ok ? (state.config.version.value?.['latest-version'] || t('overview.runtimeUnknownValue')) : t('overview.runtimeUnknownValue');
    const proxyUrl = readConfigValue(v, 'proxy-url');
    const editable = canEditRouting();

    const onOff = x => x === true
      ? `<span class="ovw-runtime-value is-on">${t('overview.runtimeSwitchOn')}</span>`
      : x === false
        ? `<span class="ovw-runtime-value is-off">${t('cpa.closeLabel')}</span>`
        : '<span class="ovw-runtime-value">--</span>';

    // 路由三项从纯文本升级为可编辑控件。routing 不可用时退回只读占位，
    // 不给一个改了也保存不上去的假输入框。
    let routingCells;
    if (!routing?.ok) {
      routingCells = [t('route.runtimeStrategyLabel'), '<span class="ovw-runtime-value is-off">--</span>'];
    } else if (!editable) {
      const label = ROUTING_STRATEGIES.find(s => s.value === r.strategy)?.label || r.strategy || t('overview.runtimeUnknownValue');
      routingCells = [t('route.runtimeStrategyLabel'), `<span class="ovw-runtime-value">${escapeHtml(label)}</span>`];
    } else {
      const options = ROUTING_STRATEGIES
        .map(s => `<option value="${escapeHtml(s.value)}"${s.value === r.strategy ? ' selected' : ''}>${escapeHtml(s.label)}</option>`)
        .join('');
      routingCells = [t('route.runtimeStrategyLabel'), `
        <div class="ovw-dropdown-wrap">
          <div class="dropdown ovw-routing-dropdown" id="ovw-routing-dropdown" aria-label="${t('route.runtimeStrategyLabel')}"></div>
          <select id="ovw-routing-strategy" style="display:none;" aria-hidden="true">${options}</select>
        </div>`];
    }

    const rows = [
      routingCells,
      [t('overview.runtimeRequestRetryLabel'), escapeHtml(String(readConfigValue(v, 'request-retry') ?? '--'))],
      [t('overview.runtimeLatestServerVersionLabel'), escapeHtml(String(version))],
      [t('overview.runtimeUpstreamProxyLabel'), proxyUrl ? escapeHtml(String(proxyUrl)) : `<span class="ovw-runtime-value is-off">${t('quotas.notConfigured')}</span>`],
      [t('overview.runtimeDebugModeLabel'), onOff(v.debug)],
      [t('overview.runtimeFileLogLabel'), onOff(v['logging-to-file'])],
      [t('cpa.settingRequestLog'), onOff(v['request-log'])],
      [t('cpa.settingWsAuth'), onOff(v['ws-auth'])],
      [t('overview.runtimeModelPrefixLabel'), onOff(v['force-model-prefix'])],
      [t('cpa.settingUsageStatistics'), onOff(v['usage-statistics-enabled'])]
    ];

    // 会话粘性两项只在可编辑时出现；只读视角下并入路由策略的说明文案，
    // 免得给成员看一排没用的空格子。
    if (editable && routing?.ok) {
      rows.splice(1, 0,
        [t('overview.runtimeSessionAffinityLabel'), `<label class="ovw-switch"><input type="checkbox" id="ovw-routing-affinity" aria-label="${t('overview.runtimeAffinityAria')}"${r.session_affinity ? ' checked' : ''}/><span>${r.session_affinity ? t('overview.runtimeSwitchOn') : t('cpa.closeLabel')}</span></label>`],
        [t('overview.runtimeSessionAffinityTtlLabel'), `<input class="ovw-control" id="ovw-routing-ttl" type="number" min="0" step="1" inputmode="numeric" placeholder="${t('overview.runtimeLeaveBlankToDisable')}" aria-label="${t('overview.runtimeAffinityTtlAria')}" value="${escapeHtml(String(r.session_affinity_ttl ?? ''))}"/>`]
      );
    }

    holder.innerHTML = rows.map(([label, value]) =>
      `<div class="ovw-runtime-row"><span class="ovw-runtime-label">${escapeHtml(label)}</span>${value}</div>`).join('');

    // 初始化/同步 Aozora UI 统一 CustomDropdown 组件
    if (window.CustomDropdown && $('ovw-routing-dropdown')) {
      const inst = new window.CustomDropdown({
        containerId: 'ovw-routing-dropdown',
        options: ROUTING_STRATEGIES,
        value: r.strategy || 'round-robin',
        menuClass: 'menu-right',
        onChange: async val => {
          const hiddenSelect = $('ovw-routing-strategy');
          if (hiddenSelect) hiddenSelect.value = val;
          const ok = await saveRouting({ strategy: val });
          if (ok) renderRuntime();
        }
      });
      if (window.dropdownInstances) {
        window.dropdownInstances['ovw-routing-dropdown'] = inst;
      }
    }
  }

  // 事件委托挂在 workspace 上：控件随 renderRuntime 重建，绑在控件本身会丢
  function bindRoutingControls() {
    const root = $('ovw-runtime-grid');
    if (!root) return;
    root.addEventListener('change', async event => {
      const target = event.target;
      if (!(target instanceof HTMLElement)) return;
      if (target.id === 'ovw-routing-strategy') {
        await saveRouting({ strategy: target.value }, target);
        renderRuntime();
      } else if (target.id === 'ovw-routing-affinity') {
        const on = target.checked;
        if (await saveRouting({ session_affinity: on }, target)) {
          const label = target.querySelector('span');
          if (label) label.textContent = on ? t('overview.runtimeSwitchOn') : t('cpa.closeLabel');
        }
      } else if (target.id === 'ovw-routing-ttl') {
        await saveRouting({ session_affinity_ttl: target.value.trim() }, target);
      }
    });
  }

  function renderBanner() {
    const holder = $('ovw-banner');
    if (!holder) return;
    const facts = state.facts;
    if (facts?.degraded) {
      holder.className = 'ovw-banner is-error';
      holder.innerHTML = `<strong>${t('overview.runtimeCpaUnreachable')}</strong><span>${escapeHtml(facts.error || t('overview.runtimeUpstreamApiUnreachable'))}${t('overview.runtimeDegradedDataNotice')}</span>`;
      holder.hidden = false;
      return;
    }
    if (facts?.providers_derived) {
      holder.className = 'ovw-banner';
      holder.innerHTML = `<strong>${t('overview.runtimeProviderDerivedTitle')}</strong><span>${t('overview.runtimeProviderDerivedNote')}</span>`;
      holder.hidden = false;
      return;
    }
    holder.hidden = true;
  }

  function renderSourceTag() {
    const holder = $('ovw-source-tag');
    if (!holder) return;
    const facts = state.facts;
    if (!facts) { holder.innerHTML = ''; return; }
    if (facts.degraded) {
      holder.className = 'ovw-source-tag is-warn';
      holder.innerHTML = t('overview.runtimeCpaDegradedTag');
    } else {
      holder.className = 'ovw-source-tag';
      holder.innerHTML = t('overview.runtimeCpaFactsTag');
    }
  }

  function renderWindowSwitcher() {
    const holder = $('ovw-window-switcher');
    if (!holder) return;
    const available = state.facts?.available_windows || [
      { id: '1h', short_label: '1h', label: t('overview.runtimeWindow1hLabel') },
      { id: '6h', short_label: '6h', label: t('overview.runtimeWindow6hLabel') },
      { id: '1d', short_label: '1d', label: t('overview.runtimeWindow1dLabel') },
      { id: '3d', short_label: '3d', label: t('overview.runtimeWindow3dLabel') },
      { id: '7d', short_label: '7d', label: t('stats.range7Days') },
      { id: '30d', short_label: '30d', label: t('stats.range30Days') }
    ];

    holder.innerHTML = available.map(w => {
      const isActive = (state.window || DEFAULT_WINDOW) === w.id;
      const titleLabel = (w.window_minutes ? fmtWindow(w.window_minutes) : '') || w.label || w.id;
      return `<button type="button" class="ovw-window-btn ${isActive ? 'active' : ''}" data-window-id="${escapeHtml(w.id)}" title="${escapeHtml(titleLabel)}">${escapeHtml(w.short_label || w.id)}</button>`;
    }).join('');

    holder.querySelectorAll('.ovw-window-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        const targetId = btn.dataset.windowId;
        if (!targetId || targetId === state.window) return;
        state.window = targetId;
        try {
          localStorage.setItem('gate_ovw_window', targetId);
        } catch (_) {}
        renderWindowSwitcher();
        fetchRuntimeFacts(true);
      });
    });
  }

  function renderAll() {
    renderBanner();
    renderSourceTag();
    renderWindowSwitcher();
    renderHero(state.facts);
    renderStats(state.facts);
    // 动态更新实时流量描述
    const bucketMins = state.facts?.bucket_minutes || state.facts?.totals?.bucket_minutes || 10;
    setText('ovw-chart-desc', `${t('overview.runtimeThroughputPrefix')} ${fmtBucketSpan(bucketMins).replace(`${t('overview.runtimeBucketSpanPrefix')} `, '')}${t('overview.runtimeThroughputSuffix')}`);

    // 热更新悬停保护：若用户正在查看吞吐图柱子的 Tooltip，推迟吞吐图 SVG 重建，避免悬停被打断闪烁
    const tip = getOvwTooltip();
    const isTipActive = tip && tip.classList.contains('is-visible');
    if (!isTipActive) {
      renderThroughputChart(state.facts);
    }
    renderFleet(state.facts);
    renderRuntime();
    const at = state.facts?.fetched_at;
    setText('ovw-updated', at ? (typeof formatCstTime === 'function' ? formatCstTime(at) : new Date(at).toLocaleTimeString(LOCALE, { hour12: false, timeZone: 'Asia/Shanghai' })) : '--');
  }

  async function fetchRuntimeFacts(force = false) {
    if (state.loading) return;
    if (typeof isMemberAuth === 'function' && isMemberAuth()) return;
    if (typeof getAdminSecret === 'function' && !getAdminSecret()) return;
    state.loading = true;
    try {
      if (!state.config) fetchRuntimeConfig();
      const currentWin = encodeURIComponent(state.window || DEFAULT_WINDOW);
      const url = `/console/api/runtime-facts?window=${currentWin}` + (force ? '&refresh=1' : '');
      const res = await fetch(url, { headers: authHeaders() });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      state.facts = await res.json();
      state.lastError = null;
    } catch (err) {
      state.lastError = err.message;
      console.warn('[Gate] runtime-facts error:', err.message);
    } finally {
      state.loading = false;
      renderAll();
    }
  }

  async function fetchRuntimeConfig() {
    if (typeof isMemberAuth === 'function' && isMemberAuth()) return;
    if (typeof getAdminSecret === 'function' && !getAdminSecret()) return;
    try {
      const res = await fetch('/console/api/runtime-config', { headers: authHeaders() });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      state.config = await res.json();
    } catch (err) {
      console.warn('[Gate] runtime-config error:', err.message);
      state.config = { config: { ok: false, error: err.message } };
    }
    renderRuntime();
  }

  function mount() {
    const nav = document.getElementById('main-nav-tabs');
    const workspace = document.getElementById('workspace');
    if (!nav || !workspace) return;

    // 切换语言后需要重建一次性注入的模板（侧栏按钮 + 工作区骨架），否则仍是旧语言文案。
    if (typeof onLocaleRebuild === 'function') {
      onLocaleRebuild(function () {
        const navBtn = document.getElementById(`tab-btn-${PAGE_ID}`);
        if (navBtn) {
          const label = navBtn.querySelector('span');
          if (label) label.textContent = t('overview.runtimeTitle');
        }
        if (typeof pageMeta === 'object' && pageMeta) {
          pageMeta[PAGE_ID] = {
            title: 'overview.runtimeTitle',
            eyebrow: 'RUNTIME OVERVIEW',
            description: 'overview.runtimeDescription'
          };
        }
        renderAll();
      });
    }

    // 侧栏首位；运行总览是进入工作台的第一屏（首屏切换见 promoteToFirstScreen）。
    nav.insertAdjacentHTML('afterbegin', `<button class="nav-tab-btn" id="tab-btn-${PAGE_ID}" onclick="switchNavTab('${PAGE_ID}')" type="button">
      <svg fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M3 13h8V3H3v10zm0 8h8v-6H3v6zm10 0h8V11h-8v10zm0-18v6h8V3h-8z" stroke-linecap="round" stroke-linejoin="round"></path></svg>
      <span data-i18n="overview.runtimeTitle">${t('overview.runtimeTitle')}</span>
    </button><div class="sidebar-section-divider"></div>`);

    workspace.insertAdjacentHTML('beforeend', `
<div class="tab-page ovw-workspace" id="tab-page-${PAGE_ID}">
  <div class="ovw-panel ovw-hero">
    <div class="ovw-hero-main">
      <div class="ovw-verdict" id="ovw-verdict"><span class="dot"></span><span data-i18n="stats.statusWaitingData">${t('stats.statusWaitingData')}</span></div>
      <div class="ovw-hero-figure">
        <span class="ovw-hero-value" id="ovw-hero-value">--</span>
        <span class="ovw-hero-label" id="ovw-hero-label" data-i18n="overview.runtimeHeroLabel">${t('overview.runtimeHeroLabel')}</span>
      </div>
      <div class="ovw-hero-meta" id="ovw-hero-meta" data-i18n="overview.runtimeAwaitingData">${t('overview.runtimeAwaitingData')}</div>
      <div class="ovw-hero-split">
        <span class="ovw-split-item"><i class="ovw-swatch is-success"></i><span data-i18n="overview.runtimeSuccessLabel">${t('overview.runtimeSuccessLabel')}</span> <span class="ovw-split-value" id="ovw-split-success">--</span></span>
        <span class="ovw-split-item"><i class="ovw-swatch is-failure"></i><span data-i18n="overview.runtimeFailureLabel">${t('overview.runtimeFailureLabel')}</span> <span class="ovw-split-value" id="ovw-split-failed">--</span></span>
      </div>
    </div>
    <div class="ovw-hero-spark" id="ovw-hero-spark"></div>
  </div>

  <div class="ovw-banner" id="ovw-banner" hidden></div>

  <div class="ovw-panel">
    <div class="ovw-panel-head">
      <div>
        <div class="ovw-eyebrow">Core metrics</div>
        <h3 class="ovw-title" data-i18n="overview.runtimeKeyMetricsTitle">${t('overview.runtimeKeyMetricsTitle')}</h3>
        <p class="ovw-desc" data-i18n="overview.runtimeKeyMetricsSubtitle">${t('overview.runtimeKeyMetricsSubtitle')}</p>
      </div>
      <div style="display:flex;align-items:center;gap:10px">
        <span class="ovw-source-tag" id="ovw-source-tag"></span>
        <span class="ovw-countdown" id="ovw-updated">--</span>
      </div>
    </div>
    <div class="ovw-stats" id="ovw-stats"></div>
  </div>

  <div class="ovw-panel">
    <div class="ovw-panel-head" style="align-items:center;">
      <div>
        <div class="ovw-eyebrow">Live traffic</div>
        <h3 class="ovw-title" data-i18n="overview.runtimeLiveTrafficTitle">${t('overview.runtimeLiveTrafficTitle')}</h3>
        <p class="ovw-desc" id="ovw-chart-desc" data-i18n="overview.runtimeLiveTrafficSubtitle">${t('overview.runtimeLiveTrafficSubtitle')}</p>
      </div>
      <div class="ovw-window-switcher" id="ovw-window-switcher" role="tablist" aria-label="${t('overview.runtimeWindowSwitcherAria')}"></div>
    </div>
    <div id="ovw-chart-holder"></div>
  </div>

  <div class="ovw-panel">
    <div class="ovw-panel-head">
      <div>
        <div class="ovw-eyebrow">Providers</div>
        <h3 class="ovw-title" data-i18n="overview.runtimeProviderCompareTitle">${t('overview.runtimeProviderCompareTitle')}</h3>
        <p class="ovw-desc" data-i18n="overview.runtimeProviderCompareSubtitle">${t('overview.runtimeProviderCompareSubtitle')}</p>
      </div>
    </div>
    <div class="ovw-fleet" id="ovw-fleet"></div>
  </div>

  <div class="ovw-panel">
    <div class="ovw-panel-head">
      <div>
        <div class="ovw-eyebrow">Runtime</div>
        <h3 class="ovw-title" data-i18n="overview.runtimeAtGlanceTitle">${t('overview.runtimeAtGlanceTitle')}</h3>
        <p class="ovw-desc" data-i18n="overview.runtimeAtGlanceSubtitle">${t('overview.runtimeAtGlanceSubtitle')}</p>
      </div>
    </div>
    <div class="ovw-runtime-grid" id="ovw-runtime-grid"></div>
  </div>
</div>`);

    // 供 admin.html 的导航与刷新流程调用。
    window.ovwRuntime = {
      fetch: fetchRuntimeFacts,
      fetchConfig: fetchRuntimeConfig,
      pageId: PAGE_ID
    };

    if (typeof pageMeta === 'object' && pageMeta) {
      pageMeta[PAGE_ID] = {
        title: 'overview.runtimeTitle',
        eyebrow: 'RUNTIME OVERVIEW',
        description: 'overview.runtimeDescription'
      };
    }

    promoteToFirstScreen();
    bindRoutingControls();
    fetchRuntimeConfig();
  }

  // 把运行总览提为默认首屏。
  //
  // 放在本文件而不是改 admin.html 的 activeNavTab 初值，是为了让「脚本没加载成功」
  // 这一情形仍然退回到原有 quotas 首屏，而不是停在一个半初始化的工作台上。
  // admin.html 结尾的 applyAuthModeUI() + switchNavTab() 先于本脚本执行，
  // 因此这里是在其后再次切页，两次切换都是幂等的 DOM 状态赋值。
  function promoteToFirstScreen() {
    // 成员看板与未配置管理密钥的用户看不到全站上游调度事实，保持 quotas 首屏
    const isMember = typeof isMemberAuth === 'function' && isMemberAuth();
    const hasSecret = typeof getAdminSecret === 'function' && !!getAdminSecret();
    if (isMember || !hasSecret) return;
    if (typeof switchNavTab !== 'function' || !pageMeta || !pageMeta[PAGE_ID]) return;
    // 只接管首屏；用户之后自己切走再刷新，仍尊重当前所在页
    if (document.body.dataset.activeTab && document.body.dataset.activeTab !== 'quotas') return;
    switchNavTab(PAGE_ID);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', mount);
  else mount();
})();
