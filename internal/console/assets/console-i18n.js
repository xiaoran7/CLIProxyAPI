/**
 * Gate Admin Console - i18n Runtime & Translation Catalog
 * 控制台中英双语运行时与翻译字典。
 * 字典本体按域拆入 console-i18n-dict-*.js，经 window.GATE_I18N_DICT 汇总；
 * 本文件只保留运行时机制：语言状态、持久化、t() 查表与插值。
 */
(function () {
  'use strict';

  // ---- 翻译字典（按域分节，随各前端文件改造增量补齐）----
  const DICT = window.GATE_I18N_DICT;

  // ---- 语言状态与持久化 ----
  const STORAGE_KEY = 'gate_lang';
  const DEFAULT_LANG = 'zh';
  const LOCALE_MAP = { zh: 'zh-CN', en: 'en-US' };
  const HTML_LANG_MAP = { zh: 'zh-CN', en: 'en' };

  let currentLang = DEFAULT_LANG;
  const listeners = [];
  // 需要在切换语言时重建的一次性模板（IIFE 文件注册的回调）
  const rebuilders = [];

  function detectLang() {
    let saved = null;
    try { saved = localStorage.getItem(STORAGE_KEY); } catch (e) { saved = null; }
    if (saved === 'zh' || saved === 'en') return saved;
    let nav = '';
    try { nav = (navigator.language || navigator.userLanguage || '').toLowerCase(); } catch (e) { nav = ''; }
    if (nav.indexOf('zh') === 0) return 'zh';
    if (nav.indexOf('en') === 0) return 'en';
    return DEFAULT_LANG;
  }

  function interpolate(str, vars) {
    if (!vars) return str;
    return String(str).replace(/\{(\w+)\}/g, function (match, name) {
      return Object.prototype.hasOwnProperty.call(vars, name) ? String(vars[name]) : match;
    });
  }

  /**
   * 取当前语言文案。缺 key 时回落到中文原文并告警，绝不抛异常、
   * 绝不返回 undefined —— 控制台渲染链路上任何异常都会导致整页白屏。
   */
  function t(key, vars) {
    const entry = DICT[key];
    if (!entry) {
      console.warn('[i18n] missing translation key:', key);
      return key;
    }
    let text = entry[currentLang];
    if (text == null || text === '') text = entry.zh;
    if (text == null) return key;
    return interpolate(text, vars);
  }

  function applyStaticI18n(root) {
    const scope = root || document;
    if (!scope || typeof scope.querySelectorAll !== 'function') return;
    scope.querySelectorAll('[data-i18n]').forEach(function (el) {
      el.textContent = t(el.getAttribute('data-i18n'));
    });
    scope.querySelectorAll('[data-i18n-title]').forEach(function (el) {
      el.setAttribute('title', t(el.getAttribute('data-i18n-title')));
    });
    scope.querySelectorAll('[data-i18n-placeholder]').forEach(function (el) {
      el.setAttribute('placeholder', t(el.getAttribute('data-i18n-placeholder')));
    });
    scope.querySelectorAll('[data-i18n-aria-label]').forEach(function (el) {
      el.setAttribute('aria-label', t(el.getAttribute('data-i18n-aria-label')));
    });
  }

  function setLocale(lang) {
    currentLang = lang === 'en' ? 'en' : 'zh';
    try { localStorage.setItem(STORAGE_KEY, currentLang); } catch (e) { /* 隐私模式/配额受限：忽略 */ }
    window.LOCALE = LOCALE_MAP[currentLang];
    window.GATE_LOCALE = currentLang;
    if (document.documentElement) document.documentElement.lang = HTML_LANG_MAP[currentLang];
    applyStaticI18n(document);
    rebuilders.forEach(function (fn) {
      try { fn(currentLang); } catch (e) { console.warn('[i18n] rebuilder failed:', e && e.message); }
    });
    listeners.forEach(function (fn) {
      try { fn(currentLang); } catch (e) { console.warn('[i18n] listener failed:', e && e.message); }
    });
  }

  function onLocaleChange(fn) {
    if (typeof fn !== 'function') return;
    listeners.push(fn);
  }

  /**
   * 注册「切换语言后需要重建一次性 DOM 模板」的回调。
   * admin-cpa.js / admin-overview.js 用 IIFE + 局部 state 渲染一次性模板，
   * 它们必须重建才能拿到新语言的文案。
   */
  function onLocaleRebuild(fn) {
    if (typeof fn !== 'function') return;
    rebuilders.push(fn);
  }

  function toggleLocale() {
    setLocale(currentLang === 'zh' ? 'en' : 'zh');
  }

  // ---- 暴露全局接口 ----
  window.t = t;
  window.setLocale = setLocale;
  window.toggleLocale = toggleLocale;
  window.onLocaleChange = onLocaleChange;
  window.onLocaleRebuild = onLocaleRebuild;
  window.applyStaticI18n = applyStaticI18n;
  window.GATE_LOCALE = currentLang;
  window.LOCALE = LOCALE_MAP[currentLang];

  // 首屏立即确定语言（不改 DOM，只定状态；DOM 应用留给启动序列）
  currentLang = detectLang();
  window.GATE_LOCALE = currentLang;
  window.LOCALE = LOCALE_MAP[currentLang];
})();
