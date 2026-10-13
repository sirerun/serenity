# Chrome consent handoff regression — 2026-10-12

Implementation: shared browserflow helper `d797fba`, OAuth consumer and regression `d402055`.
Task: paid-auth T1.3. Local synthetic browser evidence only; no Gumloop account,
production authorization, provider credentials or deployment was used.

## Before

With OAuth production source unchanged at `f1aada6`, adding only the
`TestChromeConsentHandoff` fixture reproduced both approve and deny failures:

- Approve: callback visits 1; successful authorization-code exchanges 1;
  final application navigation failed; Chromium reported form-action violation.
- Deny: callback visits 1; exchanges 0; same final navigation failure.

The fixture uses three separate local origins: Serenity, the callback (which
redeems the code), and the application to which that callback redirects.
This establishes a browser mechanism matching the reported first-click symptom.
It does not prove that Gumloop's live redirect chain is identical.

## After

The POST now commits a strict, no-store HTML continuation document, with an
escaped meta-refresh and fallback link to the exact OAuth-generated target.
The subsequent navigation is no longer a form redirect. Consent retains
`form-action 'self'`; the continuation disallows forms and scripts. No new
redirect endpoint or callback-origin wildcard was introduced. Cross-origin
referrers are suppressed.

Both approve and deny completed from one click in the same Chromium tab.
Approval exchanged its code once; a second exchange failed. Denial returned
access_denied without a code. Both consumed consent records. A separate
self-only-CSP form fixture also completed an external callback and third-origin
redirect, preserving encoded callback query values and sending no referrer.

## Reproduction

Use installed Playwright and Chromium, with task-owned Go caches:

```sh
SERENITY_PLAYWRIGHT=<installed-playwright-module> \
SERENITY_CHROMIUM=<installed-chromium-executable> \
GOMAXPROCS=2 GOFLAGS=-p=2 \
go test ./internal/hosted/oauth ./internal/hosted/browserflow -count=1 -v
```

Results: full OAuth package passed (5.088s); browserflow package passed (0.632s).
After relocating the shared browser script to browserflow/testdata, targeted
Chrome and continuation-policy tests passed again (OAuth 1.526s; browserflow
0.693s). Browser tests skip explicitly when SERENITY_PLAYWRIGHT is unset.
Hosted CI and live client acceptance remain separate, unverified gates.
