/**
 * Gate Admin Console - Upstream credential priority modal and OpenRouter account management
 * 优先级弹窗交互与 OpenRouter 独立账号管理
 */


    // ---- 优先级弹窗交互 ----
    let currentEditingAuth = null;

    function openPriorityModal(filename) {
      const accounts = cachedUpstream.accounts || [];
      const acc = accounts.find(a => a.filename === filename || a.id === filename);
      if (!acc) {
        showToast(t('quotas.credentialNotFound'), 'error');
        return;
      }
      currentEditingAuth = acc;
      document.getElementById('prio-modal-filename').innerText = acc.filename;
      const dispName = (acc.display_name || acc.nickname || acc.email || acc.filename || '').replace(/\.json$/i, '');
      document.getElementById('prio-modal-email').innerText = dispName || acc.email;
      document.getElementById('prio-modal-input').value = normalizeAuthPriority(acc.priority);
      // 保存总是会把当前数值作为显式字段写进 auths/<name>.json——哪怕一个键都没碰。
      // 而服务端已把 "未设" 归一化成默认档位，前端无从区分显式与未设，因此不能只对
      // 未设的号提示。默认档位一并说明，让人知道不设等于什么。
      document.getElementById('prio-modal-default-hint').innerText =
        t('quotas.priority.saveHint', { value: normalizeAuthPriority(acc.priority), value2: defaultAuthPriority() });

      const toggle = document.getElementById('prio-modal-status-toggle');
      const isEnabled = !acc.disabled && acc.status !== 'disabled';
      toggle.checked = isEnabled;

      openModal('upstream-priority-modal');
    }

    function setPresetPriority(val) {
      document.getElementById('prio-modal-input').value = val;
    }

    async function saveUpstreamPriority() {
      if (!currentEditingAuth) return;
      const secret = getAdminSecret();
      if (!secret) {
        showToast(t('quotas.secretNotConfigured'), 'error');
        return;
      }
      const btn = document.getElementById('prio-modal-save-btn');
      btn.disabled = true;
      btn.innerText = t('quotas.savingProgressLabel');

      const prioVal = Number(document.getElementById('prio-modal-input').value);
      const isEnabled = document.getElementById('prio-modal-status-toggle').checked;
      const filename = currentEditingAuth.filename || currentEditingAuth.id;

      try {
        // 1. 更新优先级
        const prioRes = await fetch('/console/api/upstream-auth/priority', {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + secret
          },
          body: JSON.stringify({
            name: filename,
            priority: prioVal
          })
        });
        if (!prioRes.ok) {
          const errData = await prioRes.json().catch(() => ({}));
          throw new Error(errData.error || t('quotas.priority.updateFailed'));
        }

        // 2. 更新启用/停用状态
        const statusRes = await fetch('/console/api/upstream-auth/status', {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + secret
          },
          body: JSON.stringify({
            name: filename,
            disabled: !isEnabled
          })
        });
        if (!statusRes.ok) {
          const errData = await statusRes.json().catch(() => ({}));
          throw new Error(errData.error || t('quotas.updateAccountStatusFailed'));
        }

        showToast(t('quotas.priority.updatedToast', { email: currentEditingAuth.email, prioVal: prioVal }), 'success');
        closeModal('upstream-priority-modal');
        // 本地立即刷新配额与状态卡
        await fetchUpstreamQuotas(false);
      } catch (err) {
        showToast(err.message, 'error');
      } finally {
        btn.disabled = false;
        btn.innerText = t('quotas.saveAndApply');
      }
    }

    // ---- OpenRouter 原生账号管理交互 (Gate 自治) ----
