/* Motyw działa lokalnie; w pamięci przeglądarki zapisywana jest wyłącznie nazwa motywu. */
(function () {
  'use strict';
  const KEY = 'synergia-pocket-theme-v1';
  const VALID = ['light','dark','system'];
  const media = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)');
  function read() {
    try { const v=window.localStorage.getItem(KEY); return VALID.includes(v) ? v : 'system'; }
    catch (_) { return 'system'; }
  }
  let choice=read();
  function resolved() { return choice==='system' ? (media && media.matches ? 'dark' : 'light') : choice; }
  function paint() {
    const next=resolved();
    document.documentElement.setAttribute('data-theme', next);
    document.documentElement.setAttribute('data-theme-choice', choice);
    const color=document.querySelector('meta[name="theme-color"]');
    if (color) color.content=next==='dark'?'#0d1829':'#ffffff';
    document.querySelectorAll('[data-theme-choice]').forEach(button => {
      const active=button.getAttribute('data-theme-choice')===choice;
      button.setAttribute('aria-pressed',String(active));
    });
    document.querySelectorAll('[data-theme-trigger]').forEach(button => {
      button.textContent=next==='dark'?'☾':'☀';
      button.title='Motyw: '+({'light':'jasny','dark':'ciemny','system':'automatyczny'}[choice]);
    });
  }
  function select(value) {
    if (!VALID.includes(value)) return;
    choice=value;
    try {window.localStorage.setItem(KEY, value);} catch (_) {}
    paint();
  }
  if (media) {
    if (media.addEventListener) media.addEventListener('change', paint);
    else if (media.addListener) media.addListener(paint);
  }
  window.SynergiaTheme={get:()=>choice,set:select,paint};
  paint();
  document.addEventListener('DOMContentLoaded',paint);
})();
