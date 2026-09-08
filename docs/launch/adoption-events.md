# Adoption event contract

Version 1 provides an observable, fixed-vocabulary browser event contract for the static website. Listen for `serenity:adoption` on `window`. It is deliberately **browser-local**: there is no analytics collector, network transport, cookie, local/session storage, identifier, cross-page session or durable conversion dashboard. `window.serenityAdoption.counts()` returns in-memory counters for the current document only. Navigation resets them.

This supplies testable adoption instrumentation while keeping content out of analytics. Production observation can verify event behavior; it cannot establish unique visitors, a conversion rate or a fleet-wide baseline. T7.6 must disclose that limitation. Chat's separate operational metrics are described in the [operations runbook](https://github.com/sirerun/serenity/blob/main/deploy/chat/README.md#operations).

| Event | Trigger | Extra field |
| --- | --- | --- |
| `install_cta` | Activation of a same-origin link to `/get-started/`, before navigation | None |
| `docs_open` | Each document load under `/docs/` or at `/get-started/`, including direct links and reloads | None |
| `chat_started` | A valid submitted/suggested question starts a request; each retry is another attempt | None |
| `chat_answered` | A nonempty response is rendered | `outcome`: `answer` or `search` |
| `chat_failed` | A request fails and the recovery action appears | `outcome`: `rate_limit`, `unavailable`, `timeout`, `network`, `invalid_response`, `configuration`, or `unknown` |

Every event has exactly `version: 1`, `name`, `page`, and the optional fixed outcome above. `page` is one of `home`, `install`, `docs`, `chat`, `other`. No URL, query, fragment, referrer, link label, question, history, response, exception text, credential, timestamp or user identifier is included. Arbitrary event names are discarded; arbitrary outcomes map to a fixed category. The public emitter is not an authenticity boundary: page scripts can call it, so counts are not billing/security evidence.

```js
window.addEventListener('serenity:adoption', ({ detail }) => {
  // Inspect only this fixed-schema object, never the chat form or transcript.
  console.log(detail);
});
window.serenityAdoption.counts();
```

Register before page scripts to observe `docs_open`, which emits on load. The Playwright suite uses an initialization listener and captures events across navigations in the test process; production code itself persists nothing.

## Browser verification

```sh
npm ci --ignore-scripts
npx playwright install chromium
npm run test:site
node tests/site/check-results.js
```

Seven flows run in fresh Chromium contexts at 390, 1024, 1440 and 2880 pixels: landing install/docs navigation, keyboard chat success, rate-error retry, labeled search fallback, network/malformed-response recovery, payload/transport exclusion, and responsive long-content/reduced-motion layout. Browser requests to the model endpoint are intercepted in this suite; it verifies production HTML/JS, not provider availability. Live backend checks are separate. Screenshots and traces on failure are written under ignored `test-results/`; CI uploads them and refuses zero/skipped/missing coverage. Only invented public test questions are used.

The page's approved visual styling is unchanged. A long answer now scrolls the focused input into view when necessary, avoiding an off-screen follow-up field on shorter laptop viewports. Keyboard navigation remains native anchors/buttons, chat status uses its existing live region, and reduced motion retains static artwork.


Local verification on 2026-09-08: all 28 browser cases passed with zero skips; the result gate confirmed seven distinct flows in each viewport. Adding the full URL to events failed the privacy case; setting executed count to zero failed the CI evidence gate. Restored code/report passed. An additional dark-mode 50% zoom inspection passed overflow and reduced-motion checks. Visible text matched the base revision on all 16 pages; no CSS tokens or styling values changed.
