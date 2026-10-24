/**
 * Gate Admin Console - Formatting, Clipboard, Toast & Provider Brands
 * 通用格式化辅助、剪贴板、Toast 提示、品牌矢量图标与显式标签定义。
 */

    // ---- 通用格式化辅助 ----
    function formatTokens(n) {
      if (!n) return '0';
      if (typeof n === 'object' && n !== null && n.tokens != null) n = n.tokens;
      const num = Number(n);
      if (isNaN(num) || num <= 0) return '0';
      if (num >= 1e9) return (num / 1e9).toFixed(2) + ' B';
      if (num >= 1e6) return (num / 1e6).toFixed(2) + ' M';
      if (num >= 1e3) return (num / 1e3).toFixed(1) + ' K';
      return num.toLocaleString(LOCALE);
    }
    function formatCost(val) {
      const n = Number(val) || 0;
      if (n === 0) return '$0.00';
      if (n < 0.01) return '$' + n.toFixed(4);
      return '$' + n.toFixed(2);
    }
    function formatLatencyMs(ms) {
      const n = Number(ms) || 0;
      if (n <= 0) return '--';
      if (n >= 60000) return (n / 60000).toFixed(1) + ' min';
      if (n >= 1000) return (n / 1000).toFixed(n >= 10000 ? 0 : 1) + ' s';
      return String(Math.round(n)) + ' ms';
    }
    // 绝对刷新时刻（客户端本地时区）：9月13日 18:05（若跨年则带年份）
    function formatResetAbs(iso) {
      if (!iso) return '';
      const d = new Date(iso);
      if (isNaN(d.getTime())) return '';
      const hh = String(d.getHours()).padStart(2, '0');
      const mm = String(d.getMinutes()).padStart(2, '0');
      const now = new Date();
      const vars = {
        year: d.getFullYear(),
        month: d.getMonth() + 1,
        day: d.getDate(),
        hh,
        mm
      };
      if (d.getFullYear() !== now.getFullYear()) {
        return t('reset.absDateTime', vars);
      }
      return t('reset.absDateTimeNoYear', vars);
    }
    // 客户端实时重置倒计时计算
    function formatResetCountdownLive(targetTime) {
      if (!targetTime) return '';
      const target = typeof targetTime === 'number' ? targetTime : new Date(targetTime).getTime();
      if (Number.isNaN(target)) return '';
      const now = Date.now();
      const diff = target - now;
      if (diff <= 0) return t('reset.justReset');
      const hours = Math.floor(diff / (1000 * 60 * 60));
      const mins = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));
      if (hours > 24) {
        const days = Math.floor(hours / 24);
        if (days >= 365) {
          const years = Math.floor(days / 365);
          if (years >= 3) return t('reset.longTerm');
          const remMonths = Math.floor((days % 365) / 30);
          return remMonths >= 1
            ? t('reset.yearsMonths', { years, remMonths })
            : t('reset.yearsOnly', { years });
        }
        const remHours = hours % 24;
        return t('reset.daysHours', { days, remHours });
      }
      return t('reset.hoursMinutes', { hours, mins });
    }
    // 客户端极简紧凑重置倒计时（例如 2h 51m / 1d 9h / 45m / 长期 / 即将重置）
    function formatCompactCountdownLive(targetTime) {
      if (!targetTime) return '';
      let target;
      if (typeof targetTime === 'number') {
        target = targetTime;
      } else {
        const s = String(targetTime).trim();
        if (/^\d{10,}$/.test(s)) {
          target = Number(s);
        } else {
          target = new Date(s).getTime();
        }
      }
      if (Number.isNaN(target)) return '';
      const now = Date.now();
      const diff = target - now;
      if (diff <= 0) return t('reset.soon');
      const totalMins = Math.floor(diff / (1000 * 60));
      const hours = Math.floor(totalMins / 60);
      const mins = totalMins % 60;
      if (hours >= 24) {
        const days = Math.floor(hours / 24);
        if (days >= 365) {
          const years = Math.floor(days / 365);
          if (years >= 2) return t('reset.compactLongTerm') || t('reset.longTerm');
          return `${years}y`;
        }
        const remHours = hours % 24;
        return remHours > 0 ? `${days}d ${remHours}h` : `${days}d`;
      }
      if (hours > 0) {
        return `${hours}h ${mins}m`;
      }
      return `${Math.max(1, mins)}m`;
    }
    window.formatCompactCountdownLive = formatCompactCountdownLive;

    // 兼容 ISO 时间与后端纯文本倒计时的极简格式化
    function formatCompactCountdown(targetTime, fallbackCountdown) {
      if (targetTime) {
        const res = formatCompactCountdownLive(targetTime);
        if (res) return res;
      }
      if (!fallbackCountdown || typeof fallbackCountdown !== 'string') return '';
      let str = fallbackCountdown.trim();
      const dMatch = str.match(/(\d+)\s*天/);
      const hMatch = str.match(/(\d+)\s*小时/);
      const mMatch = str.match(/(\d+)\s*分/);
      if (dMatch || hMatch || mMatch) {
        if (dMatch && parseInt(dMatch[1], 10) >= 365 * 2) {
          return t('reset.compactLongTerm') || t('reset.longTerm');
        }
        const parts = [];
        if (dMatch) parts.push(`${dMatch[1]}d`);
        if (hMatch) parts.push(`${hMatch[1]}h`);
        if (mMatch && !dMatch) parts.push(`${mMatch[1]}m`);
        return parts.join(' ');
      }
      return str;
    }
    window.formatCompactCountdown = formatCompactCountdown;
    function updateLiveCountdowns() {
      const nodes = document.querySelectorAll('[data-reset-time]');
      nodes.forEach(el => {
        const timeStr = el.dataset.resetTime;
        if (!timeStr) return;
        const text = formatResetCountdownLive(timeStr);
        if (text && el.textContent !== text) {
          el.textContent = text;
        }
      });
      const compactNodes = document.querySelectorAll('[data-reset-time-compact]');
      compactNodes.forEach(el => {
        const timeStr = el.dataset.resetTimeCompact;
        if (!timeStr) return;
        const text = formatCompactCountdownLive(timeStr);
        const valSpan = el.querySelector('.quota-reset-val');
        const targetEl = valSpan || el;
        if (text && targetEl.textContent !== text) {
          targetEl.textContent = text;
        }
      });
    }
    function escapeHtml(str) {
      if (!str) return '';
      return String(str).replace(/[&<>"']/g, m => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[m]);
    }

    function safeGoogleLink(value, accountEmail = null) {
      if (!value) return '';
      try {
        let clean = String(value).trim()
          .replace(/&amp;/gi, '&')
          .replace(/\\u(0026|003d|003f|0025)/gi, (_, hex) => String.fromCharCode(parseInt(hex, 16)))
          .replace(/\\\//g, '/');
        const url = new URL(clean);
        if (url.protocol !== 'https:' || url.hostname !== 'accounts.google.com') return '';
        if (url.pathname === '/signin/continue' && (!url.searchParams.get('plt') || !url.searchParams.get('continue'))) return '';
        if (accountEmail && typeof accountEmail === 'string' && accountEmail.includes('@') && !url.searchParams.has('authuser')) {
          url.searchParams.set('authuser', accountEmail.trim());
        }
        return escapeHtml(url.href);
      } catch (_) { return ''; }
    }

    function copyToClipboard(text, successMsg = t('runtime.clipboardCopied')) {
      if (!text) return;
      if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(text).then(() => {
          showToast(successMsg, 'success');
        }).catch(() => fallbackCopyText(text, successMsg));
      } else {
        fallbackCopyText(text, successMsg);
      }
    }

    function fallbackCopyText(text, successMsg) {
      const textarea = document.createElement('textarea');
      textarea.value = text;
      textarea.style.position = 'fixed';
      textarea.style.opacity = '0';
      document.body.appendChild(textarea);
      textarea.select();
      try {
        document.execCommand('copy');
        showToast(successMsg, 'success');
      } catch (err) {
        showToast(t('runtime.clipboardFailed'), 'error');
      }
      document.body.removeChild(textarea);
    }

    let lastToastMsg = '';
    let lastToastTime = 0;
    function showToast(msg, type = 'info') {
      const now = Date.now();
      if (msg === lastToastMsg && now - lastToastTime < 1500) {
        return; // 防抖去重，避免重复点击产生连排重复提示
      }
      lastToastMsg = msg;
      lastToastTime = now;
      const box = document.getElementById('toast-box');
      if (!box) return;
      const el = document.createElement('div');
      el.className = 'toast ' + type;
      el.textContent = msg;
      box.appendChild(el);
      setTimeout(() => {
        el.classList.add('toast-out');
        setTimeout(() => el.remove(), 220);
      }, 3200);
    }

    function openModal(id) { document.getElementById(id).style.display = 'flex'; }
    function closeModal(id) { document.getElementById(id).style.display = 'none'; }
    // ---- 提供商矢量品牌图标与显式标签定义 ----
    function getProviderBrand(providerRaw, displayName) {
      const p = (providerRaw || '').toLowerCase();
      if (p.includes('antigravity')) {
        return {
          id: 'antigravity',
          name: t('provider.antigravity'),
          shortName: t('provider.antigravityShort'),
          iconClass: 'provider-icon-antigravity',
          pillClass: 'provider-pill-antigravity',
          // Google Antigravity 高清全彩官方图标
          iconSvg: `<img src="data:image/png;base64,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" alt="Antigravity" />`
        };
      }
      if (p.includes('codex') || p.includes('chatgpt') || p.includes('openai')) {
        return {
          id: 'codex',
          name: t('provider.codex'),
          shortName: 'Codex',
          iconClass: 'provider-icon-codex',
          pillClass: 'provider-pill-codex',
          // OpenAI / Codex 官方原版六边形涡轮图标
          iconSvg: `<img src="data:image/png;base64,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" alt="Codex" />`
        };
      }
      if (p.includes('claude') || p.includes('anthropic')) {
        return {
          id: 'claude',
          name: t('provider.claude'),
          shortName: 'Claude',
          iconClass: 'provider-icon-claude',
          pillClass: 'provider-pill-claude',
          // Anthropic Claude 官方珊瑚橙太阳花图标
          iconSvg: `<img src="data:image/png;base64,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" alt="Claude" />`
        };
      }
      if (p.includes('gemini')) {
        return {
          id: 'gemini',
          name: 'Google Gemini',
          shortName: 'Gemini',
          iconClass: 'provider-icon-gemini',
          pillClass: 'provider-pill-antigravity',
          iconSvg: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2C12 7.52 7.52 12 2 12c5.52 0 10 4.48 10 10 0-5.52 4.48-10 10-10-5.52 0-10-4.48-10-10z"/></svg>`
        };
      }
      if (p.includes('stepfun') || p.includes('step')) {
        return {
          id: 'stepfun',
          name: t('provider.stepfun'),
          shortName: 'StepFun',
          iconClass: 'provider-icon-stepfun',
          pillClass: 'provider-pill-stepfun',
          iconSvg: `<svg viewBox="0 0 24 24" fill="none" stroke="#0ea5e9" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>`
        };
      }
      if (p.includes('opencode')) {
        return {
          id: 'opencode',
          name: 'OpenCode Zen',
          shortName: 'OpenCode',
          iconClass: 'provider-icon-opencode',
          pillClass: 'provider-pill-opencode',
          iconSvg: `<svg viewBox="0 0 24 24" fill="currentColor" fill-rule="evenodd"><path d="M3 2h18v20H3V2zm4 4h10v12H7V6z"/></svg>`
        };
      }
      if (p.includes('stealth')) {
        return {
          id: 'stealth',
          name: 'stealth',
          shortName: 'stealth',
          iconClass: 'provider-icon-other',
          pillClass: 'provider-pill-other',
          iconSvg: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>`
        };
      }
      if (p.includes('deepseek')) {
        return {
          id: 'deepseek',
          name: 'DeepSeek',
          shortName: 'DeepSeek',
          iconClass: 'provider-icon-deepseek',
          pillClass: 'provider-pill-stepfun',
          iconSvg: `<svg viewBox="0 0 24 24" fill="none" stroke="#0284c7" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>`
        };
      }
      if (p.includes('qwen') || p.includes('qianwen')) {
        return {
          id: 'qwen',
          name: t('provider.qwen'),
          shortName: 'Qwen',
          iconClass: 'provider-icon-qwen',
          pillClass: 'provider-pill-other',
          iconSvg: `<svg viewBox="0 0 24 24" fill="none" stroke="#6366f1" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>`
        };
      }
      if (p.includes('kimi') || p.includes('moonshot')) {
        return {
          id: 'moonshot',
          name: 'Moonshot Kimi',
          shortName: 'Kimi',
          iconClass: 'provider-icon-kimi',
          pillClass: 'provider-pill-other',
          iconSvg: `<svg viewBox="0 0 24 24" fill="none" stroke="#3b82f6" stroke-width="2"><circle cx="12" cy="12" r="9"/></svg>`
        };
      }
      if (p.includes('zcode')) {
        const isCn = p.includes('cn') || p.includes('bigmodel');
        return {
          id: isCn ? 'zcode-cn' : 'zcode',
          name: isCn ? t('provider.zcodeCn') : t('provider.zcode'),
          shortName: t('provider.zcodeShort'),
          iconClass: 'provider-icon-zcode',
          pillClass: 'provider-pill-zcode',
          iconSvg: `<svg viewBox="0 0 24 24" fill="none" stroke="#0ea5e9" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4 4h16v4H8v8h12v4H4V4z"/></svg>`
        };
      }
      if (p.includes('glm') || p.includes('zhipu')) {
        return {
          id: 'zhipu',
          name: 'Zhipu GLM',
          shortName: 'GLM',
          iconClass: 'provider-icon-glm',
          pillClass: 'provider-pill-other',
          iconSvg: `<svg viewBox="0 0 24 24" fill="none" stroke="#0ea5e9" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>`
        };
      }
      if (p.includes('openrouter')) {
        return {
          id: 'openrouter',
          name: 'OpenRouter',
          shortName: 'OpenRouter',
          iconClass: 'provider-icon-openrouter',
          pillClass: 'provider-pill-other',
          iconSvg: `<svg viewBox="0 0 24 24" fill="none" stroke="#8b5cf6" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>`
        };
      }
      if (p.includes('meta') || p.includes('llama')) {
        return {
          id: 'meta',
          name: t('provider.meta'),
          shortName: 'Llama',
          iconClass: 'provider-icon-other',
          pillClass: 'provider-pill-other',
          iconSvg: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2C6.477 2 2 6.477 2 12c0 4.991 3.657 9.128 8.438 9.879V14.89h-2.54V12h2.54V9.797c0-2.506 1.492-3.89 3.777-3.89 1.094 0 2.238.195 2.238.195v2.46h-1.26c-1.243 0-1.63.771-1.63 1.562V12h2.773l-.443 2.89h-2.33v6.989C18.343 21.129 22 16.99 22 12c0-5.523-4.477-10-10-10z"/></svg>`
        };
      }
      if (p.includes('xai') || p.includes('grok')) {
        return {
          id: 'xai',
          name: t('provider.xai'),
          shortName: 'Grok',
          iconClass: 'provider-icon-other',
          pillClass: 'provider-pill-other',
          iconSvg: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/></svg>`
        };
      }
      if (p.includes('mistral') || p.includes('mixtral') || p.includes('codestral')) {
        return {
          id: 'mistral',
          name: t('provider.mistral'),
          shortName: 'Mistral',
          iconClass: 'provider-icon-other',
          pillClass: 'provider-pill-other',
          iconSvg: `<svg viewBox="0 0 24 24" fill="#f97316"><path d="M3 3h4v4H3V3zm14 0h4v4h-4V3zM3 10h4v4H3v-4zm7 0h4v4h-4v-4zm7 0h4v4h-4v-4zM3 17h18v4H3v-4z"/></svg>`
        };
      }
      if (p.includes('minimax') || p.includes('abab')) {
        return {
          id: 'minimax',
          name: t('provider.minimax'),
          shortName: 'MiniMax',
          iconClass: 'provider-icon-other',
          pillClass: 'provider-pill-other',
          iconSvg: `<svg viewBox="0 0 24 24" fill="none" stroke="#ec4899" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 2v20M17 5v14M7 9v6M22 10v4M2 11v2"/></svg>`
        };
      }
      if (p.includes('workbuddy') || p.includes('codebuddy')) {
        const isCn = p.includes('cn') || p.includes('codebuddy');
        return {
          id: 'workbuddy',
          name: isCn ? 'WorkBuddy (CodeBuddy)' : t('provider.workbuddyCn'),
          shortName: isCn ? 'CodeBuddy' : 'WorkBuddy',
          iconClass: 'provider-icon-workbuddy',
          pillClass: 'provider-pill-workbuddy',
          iconSvg: `<img src="data:image/jpeg;base64,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" alt="WorkBuddy" />`
        };
      }
      if (p.includes('qoder') || p.includes('lingma')) {
        const isCn = p.includes('cn') || p.includes('lingma');
        return {
          id: isCn ? 'qoder-cn' : 'qoder',
          name: isCn ? t('provider.qoderCn') : t('provider.qoder'),
          shortName: isCn ? t('provider.qoderCnShort') : 'Qoder',
          iconClass: 'provider-icon-qoder',
          pillClass: 'provider-pill-qoder',
          iconSvg: `<img src="data:image/png;base64,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" alt="Qoder" />`
        };
      }
      if (p.includes('gate')) {
        return {
          id: 'gate',
          name: t('provider.gateway'),
          shortName: t('provider.gatewayShort'),
          iconClass: 'provider-icon-gate',
          pillClass: 'provider-pill-other',
          iconSvg: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>`
        };
      }

      // 区分独立自定义/兼容上游渠道，避免全部回退为笼统的“其他上游”
      const isGenericOther = !displayName || displayName === t('provider.other') || displayName === '其他上游' || displayName === 'Other Upstream' || displayName === 'Other Upstreams' || displayName === 'other';
      if (p.startsWith('compat:') || p.startsWith('upstream:') || (!isGenericOther && p !== 'unknown')) {
        let cleanName = (!isGenericOther ? displayName : null);
        if (!cleanName) {
          if (p.startsWith('compat:')) {
            const raw = p.substring(7).replace(/_/g, ' ');
            cleanName = raw.charAt(0).toUpperCase() + raw.slice(1);
          } else if (p.startsWith('upstream:')) {
            const upLabel = t('provider.upstreamChannel') || '上游渠道';
            cleanName = `${upLabel} ${p.substring(9)}`;
          } else {
            cleanName = providerRaw;
          }
        }
        return {
          id: p,
          name: cleanName,
          shortName: cleanName,
          iconClass: 'provider-icon-custom',
          pillClass: 'provider-pill-custom',
          iconSvg: `<svg viewBox="0 0 24 24" fill="none" stroke="#0ea5e9" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"/></svg>`
        };
      }

      return {
        id: 'other',
        name: t('provider.other'),
        shortName: 'Upstream',
        iconClass: 'provider-icon-other',
        pillClass: 'provider-pill-other',
        iconSvg: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M5 12h14M12 5l7 7-7 7"/></svg>`
      };
    }

