# OAuth state-creation method scope: proposed source contract

**Status:** proposal for independent review; no source change is made here.

**Source pin:** `main` at `c2d5a5439675fb7a964aadedb1a0a461f0fce386` (tree `0ec437d42305603f90ca671998ae174fc92cc10a`). This proposal is limited to the method scope of the in-process state-creation rate limiter in `internal/hosted/oauth`. It does not change the limiter threshold, storage policy, service topology, gateway admission, dependency version, or hosted configuration.

## Contract authority and discrepancy

The accepted task contract in [`docs/tasks/deep-review-001/T24.3.md`](../tasks/deep-review-001/T24.3.md), acceptance criterion 2, states that the global counter covers **only `POST /oauth/register` and `GET|POST /oauth/authorize`**. That is the normative method set for this proposal.

Merged [PR #285](https://github.com/sirerun/serenity/pull/285) implemented the prefix-first limit, refresh exemption, raised shared ceiling, and per-client consent cap. Its historical description says the shared ceiling covers register and `/oauth/authorize` “(any method).” That wording conflicts with the exact T24.3 acceptance criterion. This proposal treats the task contract as controlling and records the discrepancy for review; the PR description is historical implementation context, not a replacement acceptance criterion. T24.3's separate genuine-red requirement and historical PR results are not recreated or upgraded by this document.

## Current behavior and narrow correction

At this source pin, `Hosted.Handler` in `internal/hosted/oauth/hosted.go` wraps the methodless `/oauth/authorize` route in the shared `StateCreating` limiter. It likewise wraps methodless `/oauth/register` after the route-specific register limiter. As a result, methods ultimately rejected by the downstream OAuth handler can consume the shared counter first. Go's `ServeMux` also treats a `GET /path` pattern as matching `HEAD`, so the desired exact method set must explicitly exclude `HEAD` from shared charging. Separately, the pinned `mcpoauth` version (`v0.0.0-20260924042100-b90bbb417d9d`) currently accepts only `GET` in `AuthorizeHandler`; `POST /oauth/authorize` receives that handler's existing `405 Method Not Allowed` response. T24.3 nevertheless explicitly includes POST in the counter's method set. “Metered” describes the contractually charged method, not a promise that the downstream protocol handler accepts it.

The source change, if separately frozen and assigned, should make only this adjustment: charge the shared state-creation counter for exactly these request pairs:

| Path | Methods charged to shared state-creation counter |
| --- | --- |
| `/oauth/register` | `POST` |
| `/oauth/authorize` | `GET`, `POST` |

For any other method on either path, skip the shared counter and pass the request to the same downstream handler path that currently determines its response. In particular, do not introduce a new method guard that chooses a status code or body; preserving the existing downstream unsupported-method status and body is part of the compatibility requirement. `POST /oauth/authorize` is the intentional exception to the ordinary meaning of “unsupported”: it is contractually charged by T24.3 and must retain the pinned downstream handler's existing 405 response. `HEAD /oauth/authorize` must remain uncharged by the shared counter and retain its existing 405 response, despite `ServeMux` GET-pattern matching behavior.

Keep the outer per-prefix limiter and route-specific register limiter in their current order and scope. Thus an unsupported `/oauth/register` request may still be limited by the existing per-prefix or register-specific bucket; this proposal only says it must not consume the shared state-creation counter. Preserve response headers and normal rate-limit responses for requests that are actually charged and denied by that shared counter.

No new paths are added. `/oauth/token` and `/oauth/revoke` remain outside the shared state-creation counter and retain their existing per-prefix/token limits. A known refresh-token grant remains exempt from the shared counter and subject to existing token/per-prefix rules. `/oauth/consent` remains on its authenticated existing path and outside this shared budget. The inspected project-owned OAuth router has no `/oauth/device` or device-code route; this proposal makes no claim about behavior internal to the third-party OAuth dependency.

## Compatibility and capacity boundaries

Keep the current default limits unchanged: `PerPrefix=120`, `Register=10`, `Token=30`, and `StateCreating=5000` per minute. Do not change the 500-live-consents-per-client policy or its expired-row eviction behavior. The limiter is currently in-memory on a `Hosted` handler; this proposal does not claim a cross-process or fleet-wide aggregate budget.

The 5,000/minute shared ceiling remains an explicit capacity/policy question. It can still defer valid registration and authorization traffic when aggregate traffic reaches the limit, including traffic distributed across source prefixes, until the fixed window resets. This proposal does not lower, raise, remove, partition, or otherwise reinterpret that threshold, and it does not establish that the current capacity is acceptable. Any change to the ceiling, its aggregation scope, or acceptable outage bound requires a separately owned product/founder policy decision and evidence.

The following remain out of scope as specified by T24.3: `internal/hosted/gateway/admission.go` (T23.45 / ARC-M03) and the `mcpoauth` dependency version. No provider, deployment, traffic, database-load, or live-acceptance evidence is claimed. SEC-H02/T24.3 live acceptance remains open as recorded by the finding ledger.

## Proposed regression contract for a later source change

Tests should distinguish the shared counter from the existing per-prefix and register-specific buckets. Use a low test-only shared limit and requests from distinct prefixes so the proof is about shared-counter consumption rather than another limiter.

1. Send methods outside the contract set to `/oauth/register` (`GET`, `PUT`, and `HEAD`) and `/oauth/authorize` (`PUT`, `DELETE`, and `HEAD`) from enough distinct prefixes to exceed the test shared limit. Then send a contractually charged method from a fresh prefix and verify it is still admitted by the shared limiter. This catches accidental charging by excluded methods and the `GET`/`HEAD` ServeMux interaction.
2. In a fresh handler, prove each contractually charged method is charged: `POST /oauth/register`, `GET /oauth/authorize`, and `POST /oauth/authorize`. Use a valid GET authorization request so downstream protocol validation does not obscure the counter behavior. For POST authorization, assert the existing `405 Method Not Allowed` response and unchanged body while verifying it consumes one shared slot; after the configured test allowance is consumed, the next contractually charged request must receive the existing shared-limit response.
3. For every unsupported method/path case, compare status and body against the same downstream route behavior with shared charging disabled or otherwise unsaturated. The guard must not synthesize a different 405, status, or response body.
4. Retain and run the existing `TestSECH02GlobalCeilingCoversOnlyRegisterAndAuthorize`, the 17-prefix/known-refresh regression, IPv6 `/56` prefix regression, per-client consent-cap and expired-consent eviction tests, and consent/revocation flow test. Add explicit assertions that token, revoke, consent, and any future non-listed route do not consume the shared counter.
5. Keep the existing default-limit ratio test and assert this narrow method correction leaves all production defaults unchanged.

A meaningful red control should mutate the candidate method predicate so one unsupported method is charged (including `HEAD`) and show the focused behavioral assertion fail after successful compilation. Restore the predicate and show the same assertion pass. A compile error is not a behavioral red. Record commands and outputs at the exact candidate head; do not represent these proposed tests as already run.

## Decision requested from independent review

Confirm that T24.3 acceptance criterion 2 controls over PR #285's broader historical “any method” prose, and that the compatibility rule should bypass only shared charging for unsupported methods while leaving downstream response selection and other existing per-prefix limits unchanged. If the task contract does not authorize this narrow reconciliation, hold implementation and request an explicit owner decision. The 5,000/minute capacity question remains open in either case.
