/**
 * Gate Admin Console - Global State, Persistent Caches & Query Pools
 * 全局状态、localStorage 持久化缓存、流水/错误查询缓存池与 TTL 管理。
 */


    // ---- 状态与持久化配置 ----
    // 与 lib/config.mjs 的 AUTH_PRIORITY_MIN/MAX 对齐（与 CPA 同一区间）
    const PRIORITY_MIN = 0;
    const PRIORITY_MAX = 1000;

    let activeNavTab = 'quotas'; // quotas | overview | keys | logs
    // 持久化存储与短效多级缓存配置
    const KEY_STORAGE_KEYS = 'gate_cache_keys_v2';
    const KEY_STORAGE_LOGS = 'gate_cache_logs_v2';
    const KEY_STORAGE_ERRORS = 'gate_cache_errors_v2';
    const KEY_STORAGE_TODAY_BADGES = 'gate_cache_today_badges_v1';

    // 格式化上海时区日期字符串 (YYYY-MM-DD)，用于今日计数隔天自愈归零
    function getShanghaiDateStr(d = new Date()) {
      try {
        return new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai' }).format(d);
      } catch {
        const pad = n => String(n).padStart(2, '0');
        return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
      }
    }

    // 格式化徽标显示数字（上限 9999+）
    function formatBadgeCount(val) {
      const num = Number(val);
      if (!Number.isFinite(num) || num <= 0) return '0';
      return num > 9999 ? '9999+' : String(num);
    }

    function getTodayBadgesFromStorage() {
      try {
        const raw = localStorage.getItem(KEY_STORAGE_TODAY_BADGES);
        if (!raw) return null;
        const parsed = JSON.parse(raw);
        const todayStr = getShanghaiDateStr();
        if (parsed && parsed.date === todayStr) {
          return parsed;
        }
      } catch {}
      return null;
    }

    function saveTodayBadgesToStorage(partial) {
      try {
        const todayStr = getShanghaiDateStr();
        const current = getTodayBadgesFromStorage() || { date: todayStr, logs: 0, errors: 0 };
        const updated = {
          date: todayStr,
          logs: partial.logs !== undefined ? partial.logs : current.logs,
          errors: partial.errors !== undefined ? partial.errors : current.errors,
          ts: Date.now()
        };
        localStorage.setItem(KEY_STORAGE_TODAY_BADGES, JSON.stringify(updated));
      } catch {}
    }

    function updateNavBadgeLogs(count) {
      const num = Number(count);
      if (!Number.isFinite(num)) return;
      const badge = document.getElementById('nav-badge-logs');
      if (badge) badge.innerText = formatBadgeCount(num);
      saveTodayBadgesToStorage({ logs: num });
    }

    function updateNavBadgeErrors(count) {
      const num = Number(count);
      if (!Number.isFinite(num)) return;
      const badge = document.getElementById('nav-badge-errors');
      if (badge) badge.innerText = formatBadgeCount(num);
      saveTodayBadgesToStorage({ errors: num });
    }

    // 完整 API Key 一律不落 localStorage：列表接口只给 display_key，
    // 完整明文仅在点击「复制」时按需拉取。这里在写盘前再剥一次，
    // 兜住任何将来不小心把 api_key 塞进 cachedKeys 的路径。
    function sanitizeKeysForStorage(keys) {
      if (!Array.isArray(keys)) return [];
      return keys.map(k => {
        if (!k || typeof k !== 'object') return k;
        const { api_key, ...rest } = k;
        return rest;
      });
    }

    function persistKeysToStorage(keys) {
      try {
        localStorage.setItem(KEY_STORAGE_KEYS, JSON.stringify({ keys: sanitizeKeysForStorage(keys), ts: Date.now() }));
      } catch {}
    }

    // 全量 Key 列表（用于流水/趋势筛选下拉）。keyMetaMap 与 /console/api/keys 都是全量的，
    // 不依赖当前 Tab 是否访问过「API 密钥」页。
    let cachedKeys = [];
    let cachedKeysLoadedAt = 0;
    const CACHED_KEYS_TTL_MS = 30000;

    // 前端内存查询缓存池 (TTL 15s, 支持 SWR 快速翻页与筛选)
    const logsQueryCache = new Map();
    const errorsQueryCache = new Map();
    const QUERY_CACHE_TTL_MS = 15000;

    function getLogsQueryCache(k) {
      const item = logsQueryCache.get(k);
      if (!item) return null;
      if (Date.now() - item.ts > QUERY_CACHE_TTL_MS) {
        logsQueryCache.delete(k);
        return null;
      }
      return item;
    }

    function setLogsQueryCache(k, data) {
      logsQueryCache.set(k, { data, ts: Date.now() });
      if (logsQueryCache.size > 80) {
        const firstKey = logsQueryCache.keys().next().value;
        logsQueryCache.delete(firstKey);
      }
    }

    function clearLogsQueryCache() {
      logsQueryCache.clear();
    }

    function getErrorsQueryCache(k) {
      const item = errorsQueryCache.get(k);
      if (!item) return null;
      if (Date.now() - item.ts > QUERY_CACHE_TTL_MS) {
        errorsQueryCache.delete(k);
        return null;
      }
      return item;
    }

    function setErrorsQueryCache(k, data) {
      errorsQueryCache.set(k, { data, ts: Date.now() });
      if (errorsQueryCache.size > 80) {
        const firstKey = errorsQueryCache.keys().next().value;
        errorsQueryCache.delete(firstKey);
      }
    }

    function clearErrorsQueryCache() {
      errorsQueryCache.clear();
    }

    function invalidateKeysFrontendCache() {
      cachedKeysLoadedAt = 0;
      try { localStorage.removeItem(KEY_STORAGE_KEYS); } catch {}
      clearLogsQueryCache();
    }

    // 首屏持久化水合：从 localStorage 恢复 Keys、最近流水和故障记录，避免白屏卡顿
    function hydratePersistentCaches() {
      try {
        const rawKeys = localStorage.getItem(KEY_STORAGE_KEYS);
        if (rawKeys) {
          const parsed = JSON.parse(rawKeys);
          if (Array.isArray(parsed.keys) && parsed.keys.length > 0) {
            // 旧版本可能已把完整 api_key 写进盘上缓存，读回时一律剥掉
            cachedKeys = sanitizeKeysForStorage(parsed.keys);
            cachedKeysLoadedAt = parsed.ts || 0;
            const navBadge = document.getElementById('nav-badge-keys');
            if (navBadge) navBadge.innerText = cachedKeys.length;
          }
        }
      } catch {}

      try {
        const rawLogs = localStorage.getItem(KEY_STORAGE_LOGS);
        if (rawLogs) {
          const parsed = JSON.parse(rawLogs);
          if (Array.isArray(parsed.events)) {
            cachedEvents = parsed.events;
            cachedEventsTotal = Number.isFinite(parsed.total) ? parsed.total : cachedEvents.length;
          }
        }
      } catch {}

      try {
        const rawErrors = localStorage.getItem(KEY_STORAGE_ERRORS);
        if (rawErrors) {
          const parsed = JSON.parse(rawErrors);
          if (Array.isArray(parsed.errors)) {
            cachedErrors = parsed.errors;
            cachedErrorsTotal = Number.isFinite(parsed.total) ? parsed.total : cachedErrors.length;
          }
        }
      } catch {}

      // 统一水合今日流水与今日故障徽标（仅展示当天的计数，隔天自动归零，不再展示历史全量）
      try {
        const todayBadges = getTodayBadgesFromStorage();
        const badgeLogs = document.getElementById('nav-badge-logs');
        if (badgeLogs) {
          badgeLogs.innerText = formatBadgeCount(todayBadges ? todayBadges.logs : 0);
        }
        const badgeErrors = document.getElementById('nav-badge-errors');
        if (badgeErrors) {
          badgeErrors.innerText = formatBadgeCount(todayBadges ? todayBadges.errors : 0);
        }
      } catch {}
    }

    // 确保全量 Key 列表已加载：带 TTL 节流，避免每次刷新流水都重打一次 /console/api/keys。
    // 并写入持久化缓存与触发筛选下拉更新。
    async function ensureKeysLoaded(force = false) {
      if (isMemberAuth()) return;
      if (!force && cachedKeys.length > 0 && (Date.now() - cachedKeysLoadedAt) < CACHED_KEYS_TTL_MS) return;
      try {
        const res = await fetch('/console/api/keys', { headers: getAuthHeaders() });
        if (!res.ok) return;
        const data = await res.json();
        cachedKeys = data.keys || [];
        cachedKeysLoadedAt = Date.now();
        persistKeysToStorage(cachedKeys);
        updateLogsKeySelect(cachedEvents);
        updateStatsKeyDropdown(cachedKeys);
      } catch (err) {
        console.warn('ensureKeysLoaded error:', err.message);
      }
    }
    let cachedStats = null;
    let cachedEvents = [];
    let cachedErrors = [];
    let cachedUpstream = { accounts: [] };

    // 未显式设置 priority 的凭据由服务端下发的默认档位接管，前端不硬编码
    function defaultAuthPriority() {
      const n = Number(cachedUpstream?.default_priority);
      return Number.isFinite(n) ? n : 0;
    }
    // 缺字段按默认档位收敛；越界同样回落。注意 0 是合法档位（P0 兜底），
    // 不能用 `|| default` 判定。
    function normalizeAuthPriority(value) {
      if (value === null || value === undefined || value === '') return defaultAuthPriority();
      const n = typeof value === 'number' ? value : Number.parseInt(value, 10);
      if (!Number.isFinite(n)) return defaultAuthPriority();
      if (n < PRIORITY_MIN || n > PRIORITY_MAX) return defaultAuthPriority();
      return Math.trunc(n);
    }
    let upstreamLoaded = false;
    let cachedStatusSummary = null;
    let cachedUpstreamStats = null;
    let activeUpstreamStatsRange = 'all'; // 5h | 24h | 7d | all
    let activeUpstreamFilter = 'all'; // all | antigravity | codex | claude | workbuddy
    let activeUpstreamViewMode = localStorage.getItem('cpa_upstream_view_mode') || 'compact'; // compact | cards
    let upstreamSearchQuery = '';
    let upstreamHideDisabled = localStorage.getItem('cpa_upstream_hide_disabled') === 'true';
    let activeMetric = 'tokens'; // tokens | cost | cache
    let statsRangeDays = 7;
    let statsKeyFilter = 'all'; // all | key_id
    let currentKeyId = null;
    let currentSelectedModels = [];
    let pollingTimer = null;
    let countdownInterval = null;
    const DEFAULT_POLLING_INTERVAL_MS = 15000;
    let pollingIntervalMs = DEFAULT_POLLING_INTERVAL_MS;
    try {
      const savedPoll = localStorage.getItem('gate_admin_polling_interval');
      if (savedPoll !== null && !Number.isNaN(Number(savedPoll))) {
        pollingIntervalMs = Number(savedPoll);
      }
    } catch {}
    let nextPollTime = Date.now() + (pollingIntervalMs > 0 ? pollingIntervalMs : 60000);
    let eventFilter = 'all';
    let logsKeyFilter = 'all'; // all | key_id
    let logsProviderFilter = 'all'; // all | provider_key
    let logsAuthFilter = 'all'; // all | auth_index
    let logsTimeRange = 'all'; // all | today | 24h | 3d | 7d | 30d
    let logsSearchQuery = '';
    let logsPage = 1;
    let logsPageSize = 25; // 25 条每页
    let cachedEventsTotal = 0;
    let logsSearchTimer = null;
    let errorsPage = 1;
    let errorsPageSize = 25;
    let errorsTimeRange = 'all'; // all | today | 24h | 3d | 7d | 30d
    let cachedErrorsTotal = 0;
    let createdKeyRaw = null;

    // pageMeta 存的是「字典 key」而非已翻译文案，切换语言时重算
    const pageMeta = {
      quotas: { title: 'nav.quotas', eyebrow: 'UPSTREAM OPERATIONS', description: 'page.quotas.desc' },
      status: { title: 'nav.status', eyebrow: 'SERVICE STATUS', description: 'page.status.desc' },
      'upstream-stats': { title: 'nav.upstreamStats', eyebrow: 'IDENTITY ANALYTICS', description: 'page.upstreamStats.desc' },
      validations: { title: 'nav.validations', eyebrow: 'SECURITY & VALIDATIONS', description: 'page.validations.desc' },
      overview: { title: 'nav.overview', eyebrow: 'USAGE ANALYTICS', description: 'page.overview.desc' },
      keys: { title: 'nav.keys', eyebrow: 'ACCESS CONTROL', description: 'page.keys.desc' },
      logs: { title: 'nav.logs', eyebrow: 'REQUEST ACTIVITY', description: 'page.logs.desc' },
      errors: { title: 'nav.errors', eyebrow: 'INCIDENTS & CIRCUIT BREAKERS', description: 'page.errors.desc' }
    };

    // pageMeta 条目里的 title/description 若是字典 key，取当前语言文案
    function pageMetaText(value) {
      if (typeof value !== 'string') return value || '';
      return /^[a-zA-Z][\w]*(\.[a-zA-Z][\w]*)+$/.test(value) && typeof t === 'function' ? t(value) : value;
    }

    function closeMobileNav() {
      document.body.classList.remove('nav-open');
      document.getElementById('mobile-menu-button')?.setAttribute('aria-expanded', 'false');
    }

    function toggleMobileNav() {
      const isOpen = document.body.classList.toggle('nav-open');
      document.getElementById('mobile-menu-button')?.setAttribute('aria-expanded', String(isOpen));
    }

    function updatePageContext(tabId = activeNavTab) {
      const meta = pageMeta[tabId];
      if (!meta) return;
      const isMember = isMemberAuth();
      const title = isMember && tabId === 'quotas' ? t('member.myQuotaTitle') : isMember && tabId === 'logs' ? t('member.myLogsTitle') : pageMetaText(meta.title);
      document.getElementById('breadcrumb-current').textContent = title;
      const eyebrowEl = document.getElementById('page-eyebrow');
      if (eyebrowEl) eyebrowEl.textContent = meta.eyebrow || '';
      document.getElementById('page-title').textContent = title;
      document.getElementById('page-description').textContent = isMember && tabId === 'quotas' ? t('member.memberDesc') : pageMetaText(meta.description);
    }

