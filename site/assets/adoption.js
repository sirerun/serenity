/* Public, fixed-vocabulary events. No transport, storage, identifiers or content. */
(function () {
  'use strict';
  if (window.serenityAdoption) return;
  const names = ['install_cta', 'docs_open', 'chat_started', 'chat_answered', 'chat_failed'];
  const counts = Object.fromEntries(names.map(name => [name, 0]));
  function pageKind() {
    const path = location.pathname;
    if (path === '/') return 'home';
    if (path === '/get-started/') return 'install';
    if (path.startsWith('/docs/')) return 'docs';
    if (path === '/chat/') return 'chat';
    return 'other';
  }
  function emit(name, outcome) {
    if (!names.includes(name)) return;
    const detail = { version: 1, name, page: pageKind() };
    if (name === 'chat_answered') detail.outcome = outcome === 'search' ? 'search' : 'answer';
    if (name === 'chat_failed') detail.outcome = ['rate_limit', 'unavailable', 'timeout', 'network', 'invalid_response', 'configuration'].includes(outcome) ? outcome : 'unknown';
    counts[name] += 1;
    window.dispatchEvent(new CustomEvent('serenity:adoption', { detail: Object.freeze(detail) }));
  }
  window.serenityAdoption = Object.freeze({ emit, counts: () => Object.freeze({ ...counts }) });
  document.addEventListener('click', event => {
    const link = event.target.closest && event.target.closest('a[href]');
    if (!link || event.defaultPrevented) return;
    let target;
    try { target = new URL(link.href); } catch { return; }
    if (target.origin !== location.origin) return;
    if (target.pathname === '/get-started/') emit('install_cta');
  });
  // A document opening is measured once, including direct links and reloads.
  // It is not a unique visitor count and does not include the URL or referrer.
  if (pageKind() === 'docs' || pageKind() === 'install') emit('docs_open');
})();
