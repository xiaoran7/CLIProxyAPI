/**
 * Gate Admin Console - Upstream credential card rendering (compact list and full card grid)
 * 上游凭证卡片渲染（紧凑列表与完整卡片网格）
 * 各上游额度计与两种卡片模式分别拆入 console-quotas-meter-*.js / console-quotas-cards-*.js。
 */

    function formatBucketLabel(name, window) {
      if (!name) return t('quotas.bucket.standardQuota');
      const n = String(name).trim();
      const nl = n.toLowerCase();
      if (nl.includes('five hour') || nl.includes('5 hour') || nl.includes('5h') || nl.includes(t('quotas.bucket.fiveHour')) || window === '5h') {
        return t('quotas.bucket.fiveHourLimit');
      }
      if (nl.includes('weekly') || nl.includes('week') || nl.includes(t('quotas.bucket.weeklyLimit')) || window === 'weekly') {
        return t('quotas.bucket.weeklyLimit');
      }
      if (nl.includes('monthly') || nl.includes('month') || window === 'monthly') {
        return t('quotas.bucket.monthlyLimit');
      }
      return n;
    }

    // 按提供商 / 隐藏停用 / 关键字过滤上游凭证
    function filterUpstreamAccounts(accounts) {
      let filtered = accounts;
      if (activeUpstreamFilter !== 'all') {
        filtered = accounts.filter(a => {
          const prov = (a.provider || '').toLowerCase();
          if (activeUpstreamFilter === 'workbuddy') {
            return prov === 'workbuddy' || prov === 'workbuddy-cn';
          }
          if (activeUpstreamFilter === 'qoder') {
            return prov === 'qoder' || prov === 'qoder-cn';
          }
          if (activeUpstreamFilter === 'zcode') {
            return prov === 'zcode' || prov === 'zcode-zai' || (prov.startsWith('zcode') && !prov.includes('cn') && !prov.includes('bigmodel'));
          }
          if (activeUpstreamFilter === 'zcode-cn' || activeUpstreamFilter === 'bigmodel') {
            return prov === 'zcode-cn' || prov === 'zcode-bigmodel' || prov === 'bigmodel';
          }
          return prov === activeUpstreamFilter;
        });
      }
      if (upstreamHideDisabled) {
        filtered = filtered.filter(a => !a.disabled && a.status !== 'disabled');
      }
      if (upstreamSearchQuery) {
        filtered = filtered.filter(a => {
          const email = (a.email || '').toLowerCase();
          const authIdx = (a.auth_index || '').toLowerCase();
          const fname = (a.filename || a.id || '').toLowerCase();
          const disp = (a.display_name || '').toLowerCase();
          const nick = (a.nickname || '').toLowerCase();
          return email.includes(upstreamSearchQuery) ||
                 authIdx.includes(upstreamSearchQuery) ||
                 fname.includes(upstreamSearchQuery) ||
                 disp.includes(upstreamSearchQuery) ||
                 nick.includes(upstreamSearchQuery);
        });
      }
      return filtered;
    }

    // 构建配额重置徽标（实时倒计时 + 悬浮绝对时间）
    function buildCompactResetBadge(bkt) {
      if (!bkt) return '';
      const rTime = bkt.reset_time || bkt.resetTime || (typeof bkt.targetTs === 'number' ? bkt.targetTs : '');
      const rCountdown = bkt.reset_countdown || bkt.countdown || '';
      if (!rTime && !rCountdown) return '';
      const fn = typeof formatCompactCountdown === 'function' ? formatCompactCountdown : (window.formatCompactCountdown || null);
      const text = fn ? fn(rTime, rCountdown) : (rCountdown || '');
      if (!text) return '';
      const absFn = typeof formatResetAbs === 'function' ? formatResetAbs : (window.formatResetAbs || (s => s));
      let absStr = '';
      if (rTime) {
        try {
          absStr = typeof rTime === 'number' ? new Date(rTime).toLocaleString() : absFn(rTime);
        } catch (_) {}
      }
      const fullTip = absStr
        ? (t('quotas.estimatedResetHint') + ': ' + absStr + (rCountdown ? ` (${rCountdown})` : ''))
        : (t('quotas.estimatedResetHint') + ': ' + rCountdown);
      const dataAttr = rTime ? `data-reset-time-compact="${escapeHtml(String(rTime))}"` : '';
      return `
        <span class="quota-reset-badge" ${dataAttr} title="${escapeHtml(fullTip)}">
          <svg class="quota-reset-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg><span class="quota-reset-val">${escapeHtml(text)}</span>
        </span>
      `;
    }

    // 智能提取紧凑视图的核心配额桶：Antigravity 的 Gemini/Claude 5h 与周额度、Codex 的 5h 与周额度
    function extractUpstreamCompactBuckets(acc) {
      let gemini5hBucket = null;
      let geminiWeeklyBucket = null;
      let claude5hBucket = null;
      let claudeWeeklyBucket = null;
      let codex5hBucket = null;
      let codexWeeklyBucket = null;
      let fallbackBucket = null;

      for (const grp of (acc.groups || [])) {
        const gname = ((grp.name || '') + ' ' + (grp.displayName || '')).toLowerCase();
        for (const bkt of (grp.buckets || [])) {
          if (!fallbackBucket && Number.isFinite(bkt.remaining_percent)) fallbackBucket = bkt;
          const bname = ((bkt.name || '') + ' ' + (bkt.displayName || '')).toLowerCase();
          const bid = (bkt.bucket_id || '').toLowerCase();
          const bwin = (bkt.window || '').toLowerCase();
          const isWeekly = bwin === 'weekly' || bname.includes('weekly') || bname.includes('week') || bname.includes(t('quotas.bucket.weekly')) || bid.includes('weekly');
          const isMonthly = bwin === 'monthly' || bname.includes('monthly') || bname.includes('month') || bid.includes('monthly');
          const isLongTerm = isWeekly || isMonthly;
          const is5h = bwin === '5h' || bname.includes('5h') || bname.includes('5 hour') || bname.includes('five') || bid.includes('5h') || (!isLongTerm && bwin !== 'points');

          const isArgon = gname.includes('argon') || gname.includes('gemini 4') || gname.includes('gemini-4') || gname.includes('gemini4') || bname.includes('argon') || bid.includes('argon');
          if (isArgon) {
            // Keep argon distinct from standard gemini compact meter
          } else if (gname.includes('gemini') || bname.includes('gemini') || bid.includes('gemini')) {
            if (isWeekly) geminiWeeklyBucket = bkt;
            else if (!gemini5hBucket || is5h) gemini5hBucket = bkt;
          } else if (gname.includes('claude') || bname.includes('claude') || bid.includes('claude') || gname.includes('gpt') || bname.includes('gpt') || gname.includes('3p')) {
            if (isWeekly) claudeWeeklyBucket = bkt;
            else if (!claude5hBucket || is5h) claude5hBucket = bkt;
          } else if (acc.provider === 'codex') {
            if (isLongTerm) codexWeeklyBucket = bkt;
            else if (!codex5hBucket || is5h) codex5hBucket = bkt;
          } else {
            if (isWeekly && !claudeWeeklyBucket) claudeWeeklyBucket = bkt;
            else if (!claude5hBucket) claude5hBucket = bkt;
          }
        }
      }

      if (acc.provider === 'antigravity') {
        if (gemini5hBucket) fallbackBucket = gemini5hBucket;
        else if (geminiWeeklyBucket) fallbackBucket = geminiWeeklyBucket;
        else if (claude5hBucket) fallbackBucket = claude5hBucket;
      }
      return { gemini5hBucket, geminiWeeklyBucket, claude5hBucket, claudeWeeklyBucket, codex5hBucket, codexWeeklyBucket, fallbackBucket };
    }

    // 上游凭证卡片入口：读取状态 → 过滤 → 空态 → 按视图模式分派

    function renderUpstreamCards() {
      renderOperationsStrip();
      const cardsContainer = document.getElementById('upstream-cards-container');
      const compactContainer = document.getElementById('upstream-compact-container');
      const accounts = cachedUpstream.accounts || [];

      // 更新导航栏徽章
      const navBadge = document.getElementById('nav-badge-quotas');
      if (navBadge) navBadge.innerText = accounts.length;

      // 初始化 checkbox 状态
      const hideCheck = document.getElementById('upstream-hide-disabled');
      if (hideCheck && hideCheck.checked !== upstreamHideDisabled) {
        hideCheck.checked = upstreamHideDisabled;
      }

      // 初始化视图切换高亮
      const viewTabs = document.querySelectorAll('#upstream-view-mode-tabs button');
      viewTabs.forEach(btn => {
        btn.classList.toggle('active', btn.getAttribute('onclick')?.includes(`'${activeUpstreamViewMode}'`));
      });

      const filtered = filterUpstreamAccounts(accounts);

      if (filtered.length === 0) {
        const emptyHtml = `<div class="upstream-empty-state">${t('quotas.emptyFilteredCredentials')}</div>`;
        if (cardsContainer) cardsContainer.innerHTML = emptyHtml;
        if (compactContainer) compactContainer.innerHTML = emptyHtml;
        return;
      }

      // 切换显隐容器
      if (activeUpstreamViewMode === 'compact') {
        renderUpstreamCompactCards(cardsContainer, compactContainer, filtered);
      } else {
        renderUpstreamFullCards(cardsContainer, compactContainer, filtered);
      }
    }
