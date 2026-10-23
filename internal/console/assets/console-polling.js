/**
 * Gate Admin Console - Global Polling, Countdown & Refresh Scheduling
 * 全局轮询状态指示、倒计时圆环、安全间隔计算与定时刷新调度。
 */

    // ---- 全局轮询状态指示联动 ----
    function updateLiveSyncBadges() {
      const logsBadge = document.getElementById('logs-sync-badge');
      const logsText = document.getElementById('logs-sync-text');
      const logsLabel = document.getElementById('logs-sync-label');
      const errorsBadge = document.getElementById('errors-sync-badge');
      const errorsText = document.getElementById('errors-sync-text');

      const isPaused = pollingIntervalMs <= 0;
      const sec = Math.round(pollingIntervalMs / 1000);
      const text = isPaused ? t('logs.liveSyncPaused') : t('logs.liveSyncInterval', { sec });

      if (logsText) logsText.textContent = text;
      if (logsLabel) logsLabel.textContent = text;
      if (errorsText) errorsText.textContent = text;

      if (logsBadge) logsBadge.classList.toggle('is-paused', isPaused);
      if (errorsBadge) errorsBadge.classList.toggle('is-paused', isPaused);
    }

    // ---- 倒计时与轮询控制 ----
    function getSafePollingInterval() {
      // 用量趋势 (overview) 为重度 SQL 统计分析页，高频轮询时自适应节流至不少于 30 秒，保护后端性能
      if (activeNavTab === 'overview' && pollingIntervalMs > 0 && pollingIntervalMs < 30000) {
        return 30000;
      }
      return pollingIntervalMs;
    }

    function changePollingInterval(val, persist = true) {
      pollingIntervalMs = Number(val);
      if (persist) {
        try { localStorage.setItem('gate_admin_polling_interval', String(pollingIntervalMs)); } catch {}
      }
      if (dropdownInstances['polling-interval-dropdown'] && dropdownInstances['polling-interval-dropdown'].value !== String(pollingIntervalMs)) {
        dropdownInstances['polling-interval-dropdown'].setValue(String(pollingIntervalMs), false);
      }
      updateLiveSyncBadges();
      resetPolling();
    }

    function isUserBusyInteracting() {
      // 1. 存在打开的弹窗
      const openModal = document.querySelector('.modal-backdrop[style*="display: block"], .modal-backdrop[style*="display: flex"]');
      if (openModal) return true;
      // 2. 焦点位于正在打字的输入框
      const el = document.activeElement;
      if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') && !el.readOnly) return true;
      // 3. 流水页正在看深层分页（非第 1 页）
      if (activeNavTab === 'logs' && logsPage > 1) return true;
      // 4. 运行总览 tooltip 正在悬停
      const tip = document.getElementById('ovw-tooltip');
      if (tip && tip.classList.contains('is-visible')) return true;
      return false;
    }

    function resetPolling() {
      if (pollingTimer) clearTimeout(pollingTimer);
      if (countdownInterval) clearInterval(countdownInterval);
      const effectiveInterval = getSafePollingInterval();
      if (effectiveInterval <= 0) {
        const bar = document.getElementById('countdown-bar');
        if (bar) bar.style.strokeDashoffset = '56.5';
        document.querySelectorAll('.status-refresh-countdown').forEach(el => { el.innerText = t('runtime.refreshPaused'); });
        return;
      }
      nextPollTime = Date.now() + effectiveInterval;
      countdownInterval = setInterval(updateCountdownCircle, 1000);
      pollingTimer = setTimeout(runPeriodicRefresh, effectiveInterval);
      updateCountdownCircle();
    }

    function updateCountdownCircle() {
      updateLiveCountdowns();
      const countdownEls = document.querySelectorAll('.status-refresh-countdown');
      const effectiveInterval = getSafePollingInterval();
      if (effectiveInterval <= 0) {
        countdownEls.forEach(el => { el.innerText = t('runtime.refreshPaused'); });
        return;
      }
      const left = Math.max(0, nextPollTime - Date.now());
      const fraction = left / effectiveInterval;
      const circumference = 56.5;
      const bar = document.getElementById('countdown-bar');
      if (bar) bar.style.strokeDashoffset = (circumference * (1 - fraction)).toFixed(1);
      const secText = t('runtime.refreshInSec', { sec: Math.ceil(left / 1000) });
      countdownEls.forEach(el => { el.innerText = secText; });
    }

    async function runPeriodicRefresh() {
      if (!document.hidden) {
        if (!isUserBusyInteracting()) {
          try {
            await refreshCurrentTab(false);
          } catch (e) {
            console.warn('[Gate] Periodic refresh warning:', e);
          }
        }
      }
      resetPolling();
    }

    function manualRefresh() {
      refreshAll();
      resetPolling();
    }

