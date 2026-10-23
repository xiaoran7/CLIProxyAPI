/**
 * Gate Admin Console - Navigation, Page Meta & Custom Dropdown
 * 工作区页面切换、页面元信息、移动端导航与统一轻量 Dropdown 组件。
 */

    // ---- 工作区页面切换 ----
    function switchNavTab(tabId) {
      if (!pageMeta[tabId]) return;
      activeNavTab = tabId;
      document.body.dataset.activeTab = tabId;
      updatePageContext(tabId);
      document.querySelectorAll('#main-nav-tabs .nav-tab-btn').forEach(btn => {
        const selected = btn.id === `tab-btn-${tabId}`;
        btn.classList.toggle('active', selected);
        btn.setAttribute('aria-current', selected ? 'page' : 'false');
      });
      document.querySelectorAll('.tab-page').forEach(page => {
        page.classList.toggle('active', page.id === `tab-page-${tabId}`);
      });
      closeMobileNav();
      window.scrollTo({ top: 0, behavior: 'auto' });

      if (tabId === 'overview' && cachedStats) {
        renderMainChart(cachedStats);
      } else if (tabId === 'keys' && cachedKeys && cachedKeys.length > 0) {
        renderKeysTable(cachedKeys);
      } else if (tabId === 'logs' && cachedEvents && cachedEvents.length > 0) {
        renderEventsTable(cachedEvents, cachedEventsTotal);
      } else if (tabId === 'errors' && cachedErrors && cachedErrors.length > 0) {
        renderErrorsTable(cachedErrors, cachedErrorsTotal);
      } else if (tabId === 'validations') {
        renderValidationsPage();
      }

      // 靶向触发当前激活 Tab 专属数据拉取与更新 (按需精准拉取，杜绝并发阻塞)
      refreshCurrentTab();
      // 切页时重置倒计时圆环，让新页面获得完整的轮询周期
      resetPolling();
    }

    // ---- 现代轻量定制 Dropdown 统一驱动类 (支持浮动菜单、键盘与外部点击关闭、选中高亮、无原生弹窗) ----
    class CustomDropdown {
      constructor({ containerId, options = [], value, onChange, menuClass = '' }) {
        this.container = document.getElementById(containerId);
        this.options = options;
        this.value = value !== undefined ? String(value) : (options[0]?.value ? String(options[0].value) : '');
        this.onChange = onChange || (() => {});
        this.menuClass = menuClass;
        this.isOpen = false;
        this.init();
      }

      init() {
        if (!this.container) return;
        this.container.classList.add('dropdown');
        this.render();
        this.bindEvents();
      }

      updateOptions(options, selectedValue) {
        const nextVal = selectedValue !== undefined ? String(selectedValue) : this.value;
        const isSameOptions = Array.isArray(this.options) && Array.isArray(options) &&
          this.options.length === options.length &&
          this.options.every((opt, i) => String(opt.value) === String(options[i]?.value) && opt.label === options[i]?.label);

        if (isSameOptions && String(this.value) === String(nextVal)) {
          // 选项和选中项完全一致，无需重绘 DOM，保留用户正在展开/操作的下拉菜单及滚动位置
          return;
        }

        const menuEl = this.container?.querySelector('.dropdown-menu');
        const prevScrollTop = menuEl ? menuEl.scrollTop : 0;

        this.options = options;
        if (selectedValue !== undefined) {
          this.value = String(selectedValue);
        } else if (!this.options.some(o => String(o.value) === String(this.value))) {
          this.value = this.options[0]?.value !== undefined ? String(this.options[0].value) : '';
        }
        this.render();

        if (this.isOpen) {
          this.container.classList.add('open');
          const newMenuEl = this.container.querySelector('.dropdown-menu');
          if (newMenuEl && prevScrollTop > 0) {
            newMenuEl.scrollTop = prevScrollTop;
          }
        }
      }

      setValue(val, triggerChange = false) {
        const hadFocus = this.container?.contains(document.activeElement);
        this.value = String(val);
        this.render();
        if (hadFocus) this.container.querySelector('.dropdown-trigger')?.focus();
        if (triggerChange) this.onChange(this.value);
      }

      getValue() {
        return this.value;
      }

      render() {
        if (!this.container) return;
        const currentOpt = this.options.find(o => String(o.value) === String(this.value)) || this.options[0];
        const displayLabel = currentOpt ? currentOpt.label : t('runtime.selectPrompt');

        let itemsHtml = '';
        for (const opt of this.options) {
          const isSelected = String(opt.value) === String(this.value);
          itemsHtml += `
            <div class="dropdown-item ${isSelected ? 'selected' : ''}" role="option" aria-selected="${isSelected}" data-value="${escapeHtml(String(opt.value))}">
              <span>${escapeHtml(opt.label)}</span>
              <svg class="check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                <polyline points="20 6 9 17 4 12"></polyline>
              </svg>
            </div>`;
        }

        this.container.innerHTML = `
          <button type="button" class="dropdown-trigger" aria-label="${escapeHtml(this.container.getAttribute('aria-label') || t('runtime.selectPrompt'))}" aria-expanded="${this.isOpen}" aria-haspopup="listbox">
            <span class="dropdown-label">${escapeHtml(displayLabel)}</span>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
              <polyline points="6 9 12 15 18 9"></polyline>
            </svg>
          </button>
          <div class="dropdown-menu ${this.menuClass}" role="listbox">
            ${itemsHtml}
          </div>
        `;
      }

      bindEvents() {
        if (!this.container) return;
        // 点击 trigger 展开/收起
        this.container.addEventListener('click', e => {
          const trigger = e.target.closest('.dropdown-trigger');
          if (trigger) {
            e.stopPropagation();
            this.toggle();
            return;
          }

          const item = e.target.closest('.dropdown-item');
          if (item) {
            e.stopPropagation();
            const val = item.getAttribute('data-value');
            if (val !== this.value) {
              this.value = val;
              this.render();
              this.onChange(this.value);
            }
            this.close();
          }
        });

        this.container.addEventListener('keydown', e => {
          if (e.key === 'Escape' && this.isOpen) {
            e.preventDefault();
            this.close();
            this.container.querySelector('.dropdown-trigger')?.focus();
          } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault();
            if (!this.options.length) return;
            if (!this.isOpen) { this.open(); return; }
            const index = this.options.findIndex(o => String(o.value) === this.value);
            const next = (index + (e.key === 'ArrowDown' ? 1 : -1) + this.options.length) % this.options.length;
            if (this.options[next]) this.setValue(this.options[next].value, true);
          }
        });

        // 点击页面其他区域关闭
        document.addEventListener('click', e => {
          if (!this.container.contains(e.target) && this.isOpen) {
            this.close();
          }
        });
      }

      toggle() {
        if (this.isOpen) this.close();
        else this.open();
      }

      open() {
        // 关闭其他已打开的 dropdown
        Object.values(dropdownInstances).forEach(dropdown => {
          if (dropdown !== this && dropdown.isOpen) dropdown.close();
        });

        // 智能方向计算：若下方视口剩余空间不足容纳下拉菜单，且上方空间更充裕，则自适应向上展开
        if (this.container) {
          const rect = this.container.getBoundingClientRect();
          const menuHeight = 270; // 预估最大菜单高度 260px + 间距
          const spaceBelow = window.innerHeight - rect.bottom;
          const spaceAbove = rect.top;
          if (spaceBelow < menuHeight && spaceAbove > spaceBelow) {
            this.container.classList.add('dropup');
          } else {
            this.container.classList.remove('dropup');
          }
        }

        this.container.classList.add('open');
        this.isOpen = true;
        this.container.closest('.logs-controls')?.classList.add('has-open-dropdown');
        this.container.querySelector('.dropdown-trigger')?.setAttribute('aria-expanded', 'true');
      }

      close() {
        this.container.classList.remove('open');
        this.isOpen = false;
        const parentControls = this.container.closest('.logs-controls');
        if (parentControls && !parentControls.querySelector('.dropdown.open')) {
          parentControls.classList.remove('has-open-dropdown');
        }
        this.container.querySelector('.dropdown-trigger')?.setAttribute('aria-expanded', 'false');
      }
    }

    // 全局 dropdown 实例映射
