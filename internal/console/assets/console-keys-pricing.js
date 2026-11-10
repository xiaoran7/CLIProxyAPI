/**
 * Gate Admin Console - 模型定价管理
 * 从 console-keys.js 拆出：定价规则列表、编辑表单与增删改。
 */

    let cachedModelPrices = [];

    function openPricesModal() {
      resetPriceForm();
      fetchModelPrices();
      openModal('prices-modal');
    }

    async function fetchModelPrices() {
      const secret = getAdminSecret();
      if (!secret) return;
      try {
        const res = await fetch('/console/api/model-prices', { headers: getAuthHeaders() });
        if (!res.ok) return;
        const data = await res.json();
        cachedModelPrices = data.prices || [];
        renderPricesTable(cachedModelPrices);
      } catch (err) {
        console.warn('Model prices fetch error:', err.message);
      }
    }

    function renderPricesTable(prices) {
      const tbody = document.getElementById('prices-tbody');
      if (!tbody) return;
      if (!prices || prices.length === 0) {
        tbody.innerHTML = `<tr><td colspan="6" style="text-align: center; color: var(--text-dim); padding: 24px;">${t('pricesModal.emptyState')}</td></tr>`;
        return;
      }
      let html = '';
      for (const p of prices) {
        const pat = p.pattern || p.model_pattern || '';
        html += `<tr>
          <td><code style="font-family:var(--font-mono); color:var(--accent);">${escapeHtml(pat)}</code></td>
          <td class="num">$${Number(p.prompt_price || 0).toFixed(2)}</td>
          <td class="num">$${Number(p.completion_price || 0).toFixed(2)}</td>
          <td class="num">$${Number(p.cache_read_price || 0).toFixed(3)}</td>
          <td class="num"><span class="chip chip-gray">${Number(p.multiplier || 1.0).toFixed(1)}x</span></td>
          <td style="text-align: right;">
            <button class="btn btn-secondary btn-sm" onclick="editPriceRule(${p.id})">${t('quotas.actionEdit')}</button>
            <button class="btn btn-danger btn-sm" style="margin-left:4px;" onclick="deletePriceRule(${p.id})">${t('quotas.actionDelete')}</button>
          </td>
        </tr>`;
      }
      tbody.innerHTML = html;
    }

    function resetPriceForm() {
      document.getElementById('price-form-id').value = '';
      document.getElementById('price-form-title').innerText = t('keys.addEditPricingRuleTitle');
      document.getElementById('price-pattern').value = '';
      document.getElementById('price-prompt').value = '';
      document.getElementById('price-completion').value = '';
      document.getElementById('price-cache').value = '';
      document.getElementById('price-multiplier').value = '1.0';
    }

    function editPriceRule(id) {
      const p = cachedModelPrices.find(item => item.id === id);
      if (!p) return;
      const pat = p.pattern || p.model_pattern || '';
      document.getElementById('price-form-id').value = p.id;
      document.getElementById('price-form-title').innerText = t('keys.editPricingRuleTitle', { pat: pat });
      document.getElementById('price-pattern').value = pat;
      document.getElementById('price-prompt').value = p.prompt_price;
      document.getElementById('price-completion').value = p.completion_price;
      document.getElementById('price-cache').value = p.cache_read_price;
      document.getElementById('price-multiplier').value = p.multiplier;
    }

    async function saveModelPrice() {
      const id = document.getElementById('price-form-id').value;
      const pattern = document.getElementById('price-pattern').value.trim();
      if (!pattern) {
        showToast(t('pricesModal.patternRequired'), 'error');
        return;
      }
      const promptPrice = Number(document.getElementById('price-prompt').value) || 0;
      const completionPrice = Number(document.getElementById('price-completion').value) || 0;
      const cachePrice = Number(document.getElementById('price-cache').value) || 0;
      const multiplier = Number(document.getElementById('price-multiplier').value) || 1.0;

      try {
        const payload = {
          id: id ? Number(id) : undefined,
          pattern,
          model_pattern: pattern,
          prompt_price: promptPrice,
          completion_price: completionPrice,
          cache_read_price: cachePrice,
          multiplier
        };
        const res = await fetch('/console/api/model-prices', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', ...getAuthHeaders() },
          body: JSON.stringify(payload)
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || t('pricesModal.saveFailed'));
        showToast(t('pricesModal.saveSuccess'), 'success');
        resetPriceForm();
        fetchModelPrices();
        fetchStatsData();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }

    async function deletePriceRule(id) {
      if (!confirm(t('pricesModal.deleteConfirm'))) return;
      try {
        const res = await fetch(`/console/api/model-prices/${id}`, {
          method: 'DELETE',
          headers: getAuthHeaders()
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || t('keysModal.deleteFailed'));
        showToast(t('pricesModal.deleteSuccess'), 'success');
        fetchModelPrices();
        fetchStatsData();
      } catch (err) {
        showToast(err.message, 'error');
      }
    }
