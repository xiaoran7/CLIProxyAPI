/**
 * Gate Admin Console - Targeted Refresh Dispatch & UI Wiring
 * 按需精准数据拉取调度、面板折叠与全局 Dropdown 初始化。
 */

    // ---- 智能靶向数据拉取与更新 (按需精准拉取，根除并发队头阻塞与非当前Tab的DOM重绘风暴) ----
    let lastGlobalProbeTs = 0;
    const GLOBAL_PROBE_INTERVAL_MS = 15000;

    async function refreshCurrentTab(force = false) {
      applyAuthModeUI();
      try {
        if (!getStoredToken()) {
          fetchQuotaProbeStatus().catch(() => {});
          return;
        }

        // 1. 全局轻量探活与系统健康状态（异步非阻塞，带 15s 节流，杜绝切 Tab 串行等待）
        const now = Date.now();
        if (force || (now - lastGlobalProbeTs > GLOBAL_PROBE_INTERVAL_MS)) {
          lastGlobalProbeTs = now;
          Promise.allSettled([
            fetchQuotaProbeStatus(),
            fetchGateStatus()
          ]).catch(() => {});
        }

        // 2. 仅针对当前激活的 activeNavTab 进行精准靶向数据拉取与局部 DOM 渲染
        if (isMemberAuth()) {
          if (activeNavTab === 'logs') {
            await fetchEventsData(force);
          } else {
            await fetchMemberInfo();
          }
        } else {
          switch (activeNavTab) {
            case 'quotas':
              await fetchUpstreamQuotas(force);
              break;
            case 'overview':
              await fetchStatsData();
              break;
            case 'logs':
              await fetchEventsData(force);
              break;
            case 'errors':
              await fetchErrorsData(force);
              break;
            case 'keys':
              await fetchKeysData(force);
              break;
            case 'validations':
              await fetchUpstreamValidations();
              break;
            case 'upstream-stats':
              await fetchUpstreamAuthStats();
              break;
            case 'status':
              // 状态页需要即时网关状态
              await fetchGateStatus();
              break;
            case 'runtime':
              if (window.ovwRuntime) {
                if (typeof window.ovwRuntime.fetch === 'function') {
                  try { window.ovwRuntime.fetch(); } catch (err) { console.warn('[Overview] fetch error:', err.message); }
                }
                if (typeof window.ovwRuntime.fetchConfig === 'function') {
                  try { window.ovwRuntime.fetchConfig(); } catch (err) { console.warn('[Overview] fetchConfig error:', err.message); }
                }
              }
              break;
            default:
              await fetchUpstreamQuotas(force);
              break;
          }
        }
      } catch (e) {
        console.warn('Targeted refresh error:', e.message);
      }
    }

    // 兼容历史调用名
    async function refreshAll() {
      await refreshCurrentTab(true);
    }

    function togglePanelFold(bodyId, btn) {
      const el = document.getElementById(bodyId);
      if (!el) return;
      const isCollapsed = el.classList.toggle('collapsed');
      if (btn) {
        btn.classList.toggle('folded', isCollapsed);
        btn.setAttribute('aria-expanded', String(!isCollapsed));
      }
    }

    // ---- 初始化下拉组件与默认值 ----
    function initCustomDropdowns() {
      // 1. 全局自动刷新
      dropdownInstances['polling-interval-dropdown'] = new CustomDropdown({
        containerId: 'polling-interval-dropdown',
        options: [
          { value: '10000', label: t('poll.10s') },
          { value: '15000', label: t('poll.15s') },
          { value: '30000', label: t('poll.30s') },
          { value: '60000', label: t('poll.60s') },
          { value: '300000', label: t('poll.300s') },
          { value: '0', label: t('poll.paused') }
        ],
        value: String(pollingIntervalMs),
        onChange: val => changePollingInterval(val),
        menuClass: 'menu-right'
      });
      updateLiveSyncBadges();

      // 2. 趋势图筛选 Key (基于首屏水合 cachedKeys 预填充)
      const initialChartKeyOptions = [{ value: 'all', label: t('filter.allCreds') }];
      if (Array.isArray(cachedKeys)) {
        for (const k of cachedKeys) {
          const id = String(k.key_id);
          const label = k.key_alias ? `${k.key_alias} (${k.display_key || id})` : (k.display_key || `Key #${id}`);
          initialChartKeyOptions.push({ value: id, label });
        }
      }
      dropdownInstances['chart-key-dropdown'] = new CustomDropdown({
        containerId: 'chart-key-dropdown',
        options: initialChartKeyOptions,
        value: statsKeyFilter || 'all',
        onChange: val => changeStatsKeyFilter(val),
        menuClass: 'menu-right'
      });

      // 3. 流水筛选 Key (基于首屏水合 cachedKeys 预填充，杜绝未点 API 密钥页时选项为空)
      const initialLogsKeyOptions = [{ value: 'all', label: t('filter.allKeys') }];
      const initialKeyMap = new Map();
      if (Array.isArray(cachedKeys)) {
        for (const k of cachedKeys) {
          initialKeyMap.set(String(k.key_id), { id: String(k.key_id), alias: k.key_alias || '', display: k.display_key || '' });
        }
      }
      if (Array.isArray(cachedEvents)) {
        for (const ev of cachedEvents) {
          const id = ev.key_id != null ? String(ev.key_id) : (ev.display_key || '');
          if (id && !initialKeyMap.has(id)) {
            initialKeyMap.set(id, { id, alias: ev.key_alias || '', display: ev.display_key || '' });
          }
        }
      }
      for (const [id, info] of initialKeyMap.entries()) {
        const label = info.alias ? `${info.alias} (${info.display || id})` : (info.display || `Key #${id}`);
        initialLogsKeyOptions.push({ value: id, label });
      }

      dropdownInstances['logs-key-dropdown'] = new CustomDropdown({
        containerId: 'logs-key-dropdown',
        options: initialLogsKeyOptions,
        value: logsKeyFilter || 'all',
        onChange: val => changeLogsKeyFilter(val)
      });

      // 3.5 流水服务商筛选
      if (document.getElementById('logs-provider-dropdown')) {
        dropdownInstances['logs-provider-dropdown'] = new CustomDropdown({
          containerId: 'logs-provider-dropdown',
          options: [
            { value: 'all', label: t('filter.allProviders') },
            { value: 'antigravity', label: t('provider.antigravity') },
            { value: 'codex', label: 'OpenAI Codex' },
            { value: 'gemini', label: 'Google Gemini' },
            { value: 'claude', label: 'Anthropic Claude' },
            { value: 'stepfun', label: t('provider.stepfun') },
            { value: 'opencode', label: 'OpenCode Zen' },
            { value: 'workbuddy', label: t('provider.workbuddy') },
            { value: 'workbuddy-cn', label: 'WorkBuddy (CodeBuddy)' },
            { value: 'qoder', label: t('provider.qoder') },
            { value: 'qoder-cn', label: t('provider.qoderCn') },
            { value: 'zcode', label: t('provider.zcode') },
            { value: 'zcode-cn', label: t('provider.zcodeCn') },
            { value: 'deepseek', label: 'DeepSeek' },
            { value: 'openrouter', label: 'OpenRouter' },
            { value: 'qwen', label: t('provider.qwen') },
            { value: 'moonshot', label: 'Moonshot Kimi' },
            { value: 'zhipu', label: 'Zhipu GLM' },
            { value: 'stealth', label: 'stealth' },
            { value: 'other', label: t('provider.other') }
          ],
          value: logsProviderFilter || 'all',
          onChange: val => changeLogsProviderFilter(val)
        });
      }

      // 4. 流水时间跨度筛选
      if (document.getElementById('logs-time-dropdown')) {
        dropdownInstances['logs-time-dropdown'] = new CustomDropdown({
          containerId: 'logs-time-dropdown',
          options: [
            { value: 'all', label: t('filter.allTime') },
            { value: 'today', label: t('filter.today') },
            { value: '24h', label: t('filter.last24h') },
            { value: '3d', label: t('filter.last3d') },
            { value: '7d', label: t('filter.last7d') },
            { value: '30d', label: t('filter.last30d') }
          ],
          value: 'all',
          onChange: val => changeLogsTimeRange(val)
        });
      }

      // 5. 流水分页条数
      dropdownInstances['logs-page-size-dropdown'] = new CustomDropdown({
        containerId: 'logs-page-size-dropdown',
        options: [
          { value: '25', label: t('filter.page25') },
          { value: '50', label: t('filter.page50') },
          { value: '100', label: t('filter.page100') }
        ],
        value: '25',
        onChange: val => changeLogsPageSize(val),
        menuClass: 'dropup'
      });

      // 5.5 故障中心时间跨度与分页条数
      if (document.getElementById('errors-time-dropdown')) {
        dropdownInstances['errors-time-dropdown'] = new CustomDropdown({
          containerId: 'errors-time-dropdown',
          options: [
            { value: 'all', label: t('filter.allTime') },
            { value: 'today', label: t('filter.today') },
            { value: '24h', label: t('filter.last24h') },
            { value: '3d', label: t('filter.last3d') },
            { value: '7d', label: t('filter.last7d') },
            { value: '30d', label: t('filter.last30d') }
          ],
          value: 'all',
          onChange: val => changeErrorsTimeRange(val)
        });
      }

      if (document.getElementById('errors-page-size-dropdown')) {
        dropdownInstances['errors-page-size-dropdown'] = new CustomDropdown({
          containerId: 'errors-page-size-dropdown',
          options: [
            { value: '25', label: t('filter.page25') },
            { value: '50', label: t('filter.page50') },
            { value: '100', label: t('filter.page100') }
          ],
          value: '25',
          onChange: val => changeErrorsPageSize(val),
          menuClass: 'dropup'
        });
      }
    }

    function openAuthModal() {
      const input = document.getElementById('auth-secret-input');
      document.getElementById('btn-clear-auth').style.display = getStoredToken() ? 'inline-flex' : 'none';
      if (input) {
        input.value = getStoredToken();
        input.type = 'password';
      }
      const iconOpen = document.getElementById('auth-eye-icon-open');
      const iconClosed = document.getElementById('auth-eye-icon-closed');
      if (iconOpen) iconOpen.style.display = 'none';
      if (iconClosed) iconClosed.style.display = 'block';
      openModal('auth-modal');
    }

    // 打开受 Console 保护的上游原生 CPA 管理控制台
    function openNativeCpaManagement() {
      const secret = getAdminSecret();
      if (!secret) {
        showToast(t('runtime.authSecretRequired'), 'error');
        openAuthModal();
        return;
      }
      const targetUrl = `/console/upstream/management.html?token=${encodeURIComponent(secret)}`;
      window.open(targetUrl, '_blank', 'noopener,noreferrer');
    }

