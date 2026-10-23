/**
 * Gate Admin Console - Bootstrap, Initial Exec & Event Binding
 * 首屏初始化执行、URL 参数处理、语言切换重建与全局事件绑定。
 */

    // ---- 初始化执行 ----
    const urlParams = new URLSearchParams(window.location.search);
    if (urlParams.get('error') === 'auth_required') {
      showToast(t('auth.adminAuthRequired') || '请先配置管理密钥以访问控制台', 'warning');
      openAuthModal();
      try {
        window.history.replaceState({}, document.title, window.location.pathname);
      } catch {}
    }

    const initialKey = urlParams.get('key');
    if (initialKey) {
      const trimmed = initialKey.trim();
      if (trimmed.startsWith('sk-') || window.location.pathname.startsWith('/member')) {
        // 成员 Key：低权限、可撤销，落 localStorage 便于刷新后仍生效
        localStorage.setItem('gate_member_key', trimmed);
      } else {
        // 管理密钥：只进 sessionStorage（关标签页即失效）。
        // 绝不写 localStorage——URL 会留在历史与 Referer 里，
        // 再落一份持久化等于把万能凭证留在机器上。
        adminSecretStore.set(trimmed);
        document.cookie = 'gate_admin_token=' + encodeURIComponent(trimmed) + '; path=/; max-age=2592000; SameSite=Lax';
      }
      // 立刻把 ?key= 从地址栏抹掉，避免被截图、分享或带进 Referer
      try {
        window.history.replaceState({}, document.title, window.location.pathname);
      } catch {}
    }

    // 初始化时检测：若未配置管理员密钥且非成员凭据，弹窗提示配置
    if (!isMemberAuth() && !getAdminSecret()) {
      const msg = `
        <div class="upstream-empty-state" style="padding: 32px 20px;">
          <div style="font-size: 14px; color: var(--text-muted); margin-bottom: 8px;">${t('auth.noSecretConfigured')}</div>
          <div style="font-size: 12px; color: var(--text-dim); margin-bottom: 16px;">${t('auth.noSecretHint')}</div>
          <button class="btn btn-secondary btn-sm" onclick="openAuthModal()">${t('auth.setSecretNow')}</button>
        </div>
      `;
      const container = document.getElementById('upstream-cards-container');
      if (container) container.innerHTML = msg;
      const compactContainer = document.getElementById('upstream-compact-container');
      if (compactContainer) compactContainer.innerHTML = msg;
    }

    // ---- 语言切换：静态文案已由 admin-i18n.js 应用，这里负责重建动态渲染 ----
    function relocalizeDynamicUI() {
      // 1. 语言切换按钮自身文案
      const localeBtn = document.getElementById('btn-locale-text');
      // i18n-exempt: 语言切换按钮始终以「目标语言自己的语言」标注（中文态显示 EN，英文态显示 中文），不随 UI locale 翻译
      if (localeBtn) localeBtn.textContent = (window.GATE_LOCALE === 'en') ? '中文' : 'EN';

      // 2. 页面标题 / 面包屑 / 成员模式标签
      if (typeof applyAuthModeUI === 'function') applyAuthModeUI();
      if (typeof updatePageContext === 'function') updatePageContext();

      // 3. 下拉框选项（标签来自字典，需重建）
      if (typeof initCustomDropdowns === 'function') initCustomDropdowns();

      // 3.5 实时同步徽标文案（含秒数，随语言重建）
      if (typeof updateLiveSyncBadges === 'function') updateLiveSyncBadges();

      // 4. 各 Tab 的动态内容：用内存缓存重绘，不重新请求网络
      try {
        if (activeNavTab === 'overview' && cachedStats) renderMainChart(cachedStats);
        if (activeNavTab === 'keys' && cachedKeys && cachedKeys.length > 0) renderKeysTable(cachedKeys);
        if (activeNavTab === 'logs' && cachedEvents && cachedEvents.length > 0) renderEventsTable(cachedEvents, cachedEventsTotal);
        if (activeNavTab === 'errors' && cachedErrors && cachedErrors.length > 0) renderErrorsTable(cachedErrors, cachedErrorsTotal);
        if (activeNavTab === 'validations') renderValidationsPage();
        if (activeNavTab === 'quotas') {
          renderUpstreamCards();
          renderUpstreamStatsTable();
          renderOpenRouterAccountsTable();
        }
        if (activeNavTab === 'status') renderStatusCards();
        if (activeNavTab === 'upstream-stats') renderUpstreamStatsTable();
      } catch (e) {
        console.warn('[i18n] relocalize render failed:', e && e.message);
      }
    }

    if (typeof onLocaleChange === 'function') {
      onLocaleChange(function () { relocalizeDynamicUI(); });
    }

    // 首屏应用语言：静态文案 + <html lang> + 切换按钮文案
    (function applyInitialLocale() {
      if (typeof setLocale === 'function') {
        setLocale(window.GATE_LOCALE || 'zh');
      } else if (typeof applyStaticI18n === 'function') {
        applyStaticI18n(document);
      }
      const localeBtn = document.getElementById('btn-locale-text');
      // i18n-exempt: 语言切换按钮始终以「目标语言自己的语言」标注（中文态显示 EN，英文态显示 中文），不随 UI locale 翻译
      if (localeBtn) localeBtn.textContent = (window.GATE_LOCALE === 'en') ? '中文' : 'EN';
    })();

    hydratePersistentCaches();
    applyAuthModeUI();
    initCustomDropdowns();
    switchNavTab(activeNavTab);
    if (!isMemberAuth()) {
      ensureKeysLoaded().catch(() => {});
    }
    // 立即激活单一权威全局自动轮询与倒计时圆环（热加载就绪）
    resetPolling();
    document.querySelectorAll('.modal-backdrop').forEach(backdrop => {
      backdrop.addEventListener('click', e => { if (e.target === backdrop) backdrop.style.display = 'none'; });
    });
    document.addEventListener('keydown', e => {
      if (e.key === 'Escape') {
        closeMobileNav();
        document.querySelectorAll('.modal-backdrop').forEach(m => m.style.display = 'none');
      }
    });

    // 监听网络恢复与休眠唤醒（前后台切换自愈机制，增加 5 秒节流防抖根除频繁切屏导致的风暴与掉帧）
    let lastVisibilityRefreshTime = 0;
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden) {
        const now = Date.now();
        if (now - lastVisibilityRefreshTime > 5000) {
          lastVisibilityRefreshTime = now;
          if (typeof resetPolling === 'function') resetPolling();
          refreshCurrentTab();
        }
      }
    });
    window.addEventListener('online', () => {
      showToast(t('runtime.networkRestored'), 'info');
      refreshCurrentTab(true);
    });
    window.addEventListener('offline', () => {
      const badge = document.getElementById('quota-probe-badge');
      const text = document.getElementById('quota-probe-text');
      if (badge) {
        badge.className = 'status-badge error';
        badge.title = t('runtime.networkOfflineBadge');
      }
      if (text) text.innerText = t('runtime.networkOfflineBadgeText');
      showToast(t('runtime.networkOffline'), 'warning');
    });
