# Memory explorer

Source for the embedded public synthetic demo and session-authenticated memory explorer. Install pinned packages with `npm ci --ignore-scripts`; `npm run build` emits content-hashed assets in `site/assets/explorer/` and a shell in `site/explore/`. Build/package uses the shared lease and external caches under repository instructions. Commit source, lockfile and packaged assets together. `npm test` checks state/paging/date helpers.

Public demo data is a separate dynamic import; private bootstrap never loads it after a failure. No memory/session content is stored in localStorage, IndexedDB or service workers. Graph and facets use same-origin, no-store session reads; loaded core matches and one-hop context are counted separately. Graph is capped500nodes and Load more is explicit. Browser history stores scope/year/type only, not search text, record identifiers or memory content.

For local non-customer browser checks, compile the dashboard test binary under a fresh lease, then run its opt-in TestExplorerBrowserFixture with SERENITY_EXPLORER_FIXTURE_RECEIPT pointing to an external-SSD receipt. The localhost-only helper exposes synthetic-account login/expiry paths, never production routes. Stop the owned helper after acceptance. Performance qualification uses TestExplorerTenThousandRecordProjection; local results do not qualify production host headroom.
