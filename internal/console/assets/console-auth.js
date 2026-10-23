/**
 * Gate Admin Console - Auth Secret Storage & Access Mode
 * 管理密钥/成员 Key 存储策略、登录态判定、鉴权模式 UI 与密钥按需拉取。
 */

    let dropdownInstances = {};
    window.CustomDropdown = CustomDropdown;
    window.dropdownInstances = dropdownInstances;

    function toggleAuthInputVisibility() {
      const input = document.getElementById('auth-secret-input');
      const iconOpen = document.getElementById('auth-eye-icon-open');
      const iconClosed = document.getElementById('auth-eye-icon-closed');
      if (!input) return;
      if (input.type === 'password') {
        input.type = 'text';
        if (iconOpen) iconOpen.style.display = 'block';
        if (iconClosed) iconClosed.style.display = 'none';
      } else {
        input.type = 'password';
        if (iconOpen) iconOpen.style.display = 'none';
        if (iconClosed) iconClosed.style.display = 'block';
      }
    }

    // 管理密钥是万能凭证（建/删/轮换 Key、改配额全放行），
    // 只存 sessionStorage：关掉标签页即失效，不在机器上跨会话留存。
    // 历史上它存 localStorage，关浏览器后依然可被同源脚本读取。
    // 兼容读取旧的 localStorage 键，读到即迁移并删除旧值。
    const ADMIN_SECRET_STORAGE_KEY = 'gate_admin_secret';
    const ADMIN_SECRET_LEGACY_KEY = 'cpa_admin_secret';
    const adminSecretStore = {
      get() {
        try {
          const s = sessionStorage.getItem(ADMIN_SECRET_STORAGE_KEY);
          if (s) return s;
          const legacy = localStorage.getItem(ADMIN_SECRET_STORAGE_KEY)
            || localStorage.getItem(ADMIN_SECRET_LEGACY_KEY);
          if (legacy) {
            try { sessionStorage.setItem(ADMIN_SECRET_STORAGE_KEY, legacy); } catch {}
            try {
              localStorage.removeItem(ADMIN_SECRET_STORAGE_KEY);
              localStorage.removeItem(ADMIN_SECRET_LEGACY_KEY);
            } catch {}
            return legacy;
          }
        } catch {}
        return '';
      },
      set(val) {
        try {
          sessionStorage.setItem(ADMIN_SECRET_STORAGE_KEY, val);
          try {
            localStorage.removeItem(ADMIN_SECRET_STORAGE_KEY);
            localStorage.removeItem(ADMIN_SECRET_LEGACY_KEY);
          } catch {}
        } catch {}
      },
      clear() {
        try { sessionStorage.removeItem(ADMIN_SECRET_STORAGE_KEY); } catch {}
        try {
          localStorage.removeItem(ADMIN_SECRET_STORAGE_KEY);
          localStorage.removeItem(ADMIN_SECRET_LEGACY_KEY);
        } catch {}
      }
    };

    function getStoredToken() {
      // 若处于 /member 路由，优先使用 member key 或 URL 参数中的 key
      if (window.location.pathname.startsWith('/member')) {
        const params = new URLSearchParams(window.location.search);
        const urlKey = params.get('key');
        if (urlKey) return urlKey.trim();
        return localStorage.getItem('gate_member_key') || '';
      }

      // 管理员模式：若本地显式配置了管理员密码，优先遵循管理员模式
      const adminSecret = adminSecretStore.get();
      if (adminSecret) {
        return adminSecret;
      }

      // 检查 Cookie 中是否已有有效管理员凭据
      const cookieMatch = document.cookie.match(/(?:^|;\s*)gate_admin_token=([^;]+)/);
      if (cookieMatch && cookieMatch[1]) {
        try {
          const cookieVal = decodeURIComponent(cookieMatch[1]).trim();
          if (cookieVal) {
            adminSecretStore.set(cookieVal);
            return cookieVal;
          }
        } catch {}
      }

      // 否则检查 URL query key
      const params = new URLSearchParams(window.location.search);
      const urlKey = params.get('key');
      if (urlKey) return urlKey.trim();

      const memberKey = localStorage.getItem('gate_member_key');
      return memberKey || '';
    }

    function isMemberAuth(token = getStoredToken()) {
      if (window.location.pathname.startsWith('/member')) return true;
      const adminSecret = adminSecretStore.get();
      // 如果已配置管理员密码，且 token 等于管理员密码，则必定不是 memberAuth
      if (adminSecret && token === adminSecret) return false;
      if (!token) return false;
      if (token.startsWith('sk-') || localStorage.getItem('gate_member_key') === token) {
        return true;
      }
      return false;
    }

    function getAdminSecret() {
      if (isMemberAuth()) return '';
      return adminSecretStore.get();
    }

    function getMemberKey() {
      if (window.location.pathname.startsWith('/member')) {
        const params = new URLSearchParams(window.location.search);
        const urlKey = params.get('key');
        if (urlKey) return urlKey.trim();
        return localStorage.getItem('gate_member_key') || '';
      }
      const params = new URLSearchParams(window.location.search);
      const urlKey = params.get('key');
      if (urlKey && urlKey.startsWith('sk-')) return urlKey.trim();
      return localStorage.getItem('gate_member_key') || (isMemberAuth() ? getStoredToken() : '');
    }

    function getAuthHeaders() {
      const token = getStoredToken();
      return token ? { 'Authorization': 'Bearer ' + token } : {};
    }

    // 完整 API Key 只在用户显式点击「复制」时按需拉取，不随 Key 列表下发，
    // 也就不进 cachedKeys 缓存与 localStorage。取到即用，不做任何持久化。
    async function fetchFullKey(keyId) {
      const res = await fetch(`/console/api/keys/${encodeURIComponent(keyId)}/reveal`, { headers: getAuthHeaders() });
      if (!res.ok) throw new Error(t('runtime.fullKeyRevealFailed'));
      const data = await res.json();
      return data.api_key || '';
    }

    async function copyFullKey(keyId) {
      try {
        const raw = await fetchFullKey(keyId);
        if (!raw) throw new Error('empty');
        copyToClipboard(raw, t('runtime.fullKeyCopied'));
      } catch (e) {
        showToast(t('runtime.fullKeyFetchFailed'), 'error');
      }
    }

    function saveAuthSecret() {
      const val = document.getElementById('auth-secret-input').value.trim();
      if (!val) { showToast(t('runtime.manageSecretRequired'), 'error'); return; }
      if (window.location.pathname.startsWith('/member') || val.startsWith('sk-')) {
        localStorage.setItem('gate_member_key', val);
        adminSecretStore.clear();
        document.cookie = 'gate_admin_token=; path=/; max-age=0; SameSite=Lax';
        showToast(t('runtime.memberModeEnabled'), 'success');
      } else {
        adminSecretStore.set(val);
        localStorage.removeItem('gate_member_key');
        // 同步写 Cookie，使得原生 CPA 面板、iframe 与各上游路由均能保持登录凭证
        document.cookie = 'gate_admin_token=' + encodeURIComponent(val) + '; path=/; max-age=2592000; SameSite=Lax';
        // 清理掉 URL 中残留的 ?key= 参数，防止页面被锁定在旧的成员 key
        if (window.location.search) {
          try {
            const cleanUrl = window.location.pathname;
            window.history.replaceState({}, document.title, cleanUrl);
          } catch (e) {}
        }
        showToast(t('runtime.manageSecretSaved'), 'success');
      }
      closeModal('auth-modal');
      applyAuthModeUI();
      manualRefresh();
    }

    function clearAuthSecret() {
      adminSecretStore.clear();
      localStorage.removeItem('gate_member_key');
      document.cookie = 'gate_admin_token=; path=/; max-age=0; SameSite=Lax';
      if (window.location.search) {
        try {
          const cleanUrl = window.location.pathname;
          window.history.replaceState({}, document.title, cleanUrl);
        } catch (e) {}
      }
      closeModal('auth-modal');
      showToast(t('runtime.authCleared'), 'info');
      applyAuthModeUI();
      manualRefresh();
    }

    function applyAuthModeUI() {
      const isMember = isMemberAuth();
      const needsAuth = !getStoredToken();
      document.body.classList.toggle('needs-auth', needsAuth);
      document.body.classList.toggle('member-mode', isMember);
      if (needsAuth && activeNavTab !== 'quotas') switchNavTab('quotas');
      const badge = document.getElementById('member-mode-badge');
      const btnCreate = document.getElementById('btn-create-key');
      const btnPrices = document.getElementById('btn-prices');
      const btnAuthText = document.getElementById('btn-auth-text');
      const tabOverview = document.getElementById('tab-btn-overview');
      const tabValidations = document.getElementById('tab-btn-validations');
      const tabKeys = document.getElementById('tab-btn-keys');
      const tabErrors = document.getElementById('tab-btn-errors');
      const tabTextQuotas = document.getElementById('tab-text-quotas');
      const tabTextLogs = document.getElementById('tab-text-logs');
      const memberPanel = document.getElementById('member-quota-panel');
      const upstreamPanel = document.getElementById('upstream-panel');
      const upstreamStatsPanel = document.getElementById('upstream-stats-panel');
      const keyFilterContainer = document.getElementById('logs-key-filter-container');

      if (isMember) {
        if (badge) {
          badge.style.display = 'inline-flex';
          badge.innerText = t('member.title');
        }
        if (btnCreate) btnCreate.style.display = 'none';
        if (btnPrices) btnPrices.style.display = 'none';
        if (btnAuthText) btnAuthText.innerText = needsAuth ? t('common.key') : t('common.switchKey');
        if (tabValidations) tabValidations.style.display = 'none';
        if (tabOverview) tabOverview.style.display = 'none';
        if (tabKeys) tabKeys.style.display = 'none';
        if (tabErrors) tabErrors.style.display = 'none';
        // 运行总览聚合的是全站上游调度事实，成员不得查看
        const tabRuntime = document.getElementById('tab-btn-runtime');
        if (tabRuntime) tabRuntime.style.display = 'none';
        if (activeNavTab === 'runtime') switchNavTab('quotas');
        if (tabTextQuotas) tabTextQuotas.innerText = t('member.myQuotaTitle');
        if (tabTextLogs) tabTextLogs.innerText = t('member.myLogsTitle');
        if (memberPanel) memberPanel.style.display = 'flex';
        // 彻底隔离：成员模式下完全移除或隐藏上游卡片与统计面板，绝不显示空加载状态
        if (upstreamPanel) {
          upstreamPanel.style.display = 'none';
          const uContainer = document.getElementById('upstream-cards-container');
          if (uContainer) uContainer.innerHTML = '';
        }
        if (upstreamStatsPanel) {
          upstreamStatsPanel.style.display = 'none';
          const sTbody = document.getElementById('upstream-stats-table-body');
          if (sTbody) sTbody.innerHTML = '';
          const sCards = document.getElementById('upstream-stats-cards-container');
          if (sCards) sCards.innerHTML = '';
          const sArchive = document.getElementById('upstream-stats-archive-container');
          if (sArchive) { sArchive.style.display = 'none'; sArchive.innerHTML = ''; }
        }
        if (keyFilterContainer) keyFilterContainer.style.display = 'none';
        // 如果当前处在不可见的 tab，切回 quotas
        if (['overview', 'keys', 'errors', 'validations', 'status', 'upstream-stats'].includes(activeNavTab)) {
          switchNavTab('quotas');
        }
      } else {
        if (badge) badge.style.display = 'none';
        if (btnCreate) btnCreate.style.display = 'inline-flex';
        if (btnPrices) btnPrices.style.display = 'inline-flex';
        if (btnAuthText) btnAuthText.innerText = t('common.key');
        if (tabValidations) tabValidations.style.display = 'inline-flex';
        if (tabOverview) tabOverview.style.display = 'inline-flex';
        if (tabKeys) tabKeys.style.display = 'inline-flex';
        if (tabErrors) tabErrors.style.display = 'inline-flex';
        if (tabTextQuotas) tabTextQuotas.innerText = t('nav.quotas');
        if (tabTextLogs) tabTextLogs.innerText = t('nav.logs');
        if (memberPanel) memberPanel.style.display = 'none';
        if (upstreamPanel) upstreamPanel.style.display = 'block';
        if (upstreamStatsPanel) upstreamStatsPanel.style.display = 'block';
        if (keyFilterContainer) keyFilterContainer.style.display = 'flex';
      }
      updatePageContext();
    }

