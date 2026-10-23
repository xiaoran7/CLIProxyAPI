/**
 * Gate Admin Console - Member Portal Data Fetch & Render
 * 成员专属用量看板的数据拉取与渲染。
 */

    // ---- 成员专属数据拉取与渲染 ----
    let cachedMemberInfo = null;

    async function fetchMemberInfo() {
      const memberKey = getMemberKey();
      if (!memberKey) return;
      try {
        const res = await fetch('/api/member/info', { headers: getAuthHeaders() });
        if (!res.ok) {
          if (res.status === 401) {
            showToast(t('runtime.memberKeyInvalid'), 'error');
          }
          return;
        }
        cachedMemberInfo = await res.json();
        renderMemberPanel(cachedMemberInfo);
      } catch (err) {
        console.warn('Member info fetch error:', err.message);
      }
    }

    function renderMemberPanel(info) {
      if (!info) return;
      const aliasEl = document.getElementById('member-alias-display');
      if (aliasEl) aliasEl.innerText = info.key_alias || t('member.myKeyAlias');
      
      const statusPill = document.getElementById('member-status-pill');
      if (statusPill) {
        if (info.enabled) {
          statusPill.className = 'chip chip-green';
          statusPill.innerText = t('member.runningStatus');
        } else {
          statusPill.className = 'chip chip-red';
          statusPill.innerText = t('member.disabledStatus');
        }
      }

      const keyDisp = document.getElementById('member-key-display');
      if (keyDisp) keyDisp.innerText = info.display_key || 'sk-cpa-***';

      const modelsEl = document.getElementById('member-models-display');
      if (modelsEl) {
        if (Array.isArray(info.model_whitelist) && info.model_whitelist.length > 0) {
          modelsEl.innerText = info.model_whitelist.join(', ');
        } else {
          modelsEl.innerText = t('member.allModelsAvailable');
        }
      }

      document.getElementById('member-tokens-total').innerText = formatTokens(info.today_tokens);
      document.getElementById('member-tokens-io').innerText = `${formatTokens(info.today_prompt_tokens)} / ${formatTokens(info.today_completion_tokens)}`;
      document.getElementById('member-tokens-cache').innerText = `${formatTokens(info.today_cached_tokens)} (${(info.cache_hit_rate || 0).toFixed(1)}%)`;
      document.getElementById('member-cost-total').innerText = formatCost(info.today_cost_usd);

      const limitBox = document.getElementById('member-limit-box');
      if (info.daily_limit && info.daily_limit > 0) {
        limitBox.style.display = 'block';
        const used = Number(info.today_tokens) || 0;
        const limit = Number(info.daily_limit) || 0;
        const ratio = limit > 0 ? (used / limit) : 0;
        const pct = Math.min(100, Math.round(ratio * 100));
        const rem = Math.max(0, limit - used);

        const pctEl = document.getElementById('member-limit-pct');
        if (pctEl) {
          pctEl.innerText = `${pct}%`;
          if (pct >= 100) pctEl.className = 'member-progress-pct danger';
          else if (pct >= 80) pctEl.className = 'member-progress-pct warn';
          else pctEl.className = 'member-progress-pct';
        }

        const fracEl = document.getElementById('member-limit-fraction');
        if (fracEl) fracEl.innerText = `${formatTokens(used)} / ${formatTokens(limit)}`;

        const usedDescEl = document.getElementById('member-limit-used-desc');
        if (usedDescEl) usedDescEl.innerText = formatTokens(used);

        const remEl = document.getElementById('member-limit-remaining');
        if (remEl) remEl.innerText = `${formatTokens(rem)} Tokens`;

        const bar = document.getElementById('member-limit-bar');
        if (bar) {
          bar.style.width = Math.max(pct > 0 ? pct : (used > 0 ? 1 : 0), 0) + '%';
          if (pct >= 100) {
            bar.className = 'member-progress-fill danger';
          } else if (pct >= 80) {
            bar.className = 'member-progress-fill warn';
          } else {
            bar.className = 'member-progress-fill';
          }
        }
      } else {
        limitBox.style.display = 'none';
      }

      if (Number.isFinite(info.today_requests)) {
        updateNavBadgeLogs(info.today_requests);
      }
    }

