# OAuth method-scope source review

**Verdict: CLEAR for the frozen source slice only.**  
**Exact reviewed commit:** `9948c633a95d7af93e1451656e0cfc21e104241a`  
**Source fingerprint:** `388ea86c4b5847db4eb7513735b84e3221a73f210405068fb78e7bd3bd8e1e32`  
**Review clone:** detached fresh SSD clone; clean after all checks. No source or module changes remain.

The code matches `oauth-state-method-scope-v1`: the shared state limiter charges only POST `/oauth/register` and GET|POST `/oauth/authorize`. Unsupported methods continue to the same downstream handler without spending that shared counter. The existing outer per-prefix limiter remains around every OAuth request, register's per-prefix limiter remains outside its shared-budget guard, and token/revoke retain their prior per-prefix limiters (`internal/hosted/oauth/hosted.go:58-83`). The methodless `withRateLimit` wrapper delegates to the new helper with an empty method list, which retains all-method charging for its existing call sites; explicit method matches alone enter the limited branch (`ratelimit.go:120-149`).

I checked the exact pinned dependency source `github.com/ajent-social/go v0.0.0-20260924042100-b90bbb417d9d`, not the newer cached version. `mcpoauth.AuthorizeHandler` permits GET only and returns its existing 405 for other methods; `RegisterHandler` permits POST only. Thus POST `/oauth/authorize` is contractually metered while still returning that handler's 405, and HEAD remains excluded while preserving the dependency's existing 405. The source freeze limits the change to these three OAuth files and the receipt, and explicitly keeps the 5000/minute capacity policy and live acceptance open (`docs/plans/oauth-state-creation-method-scope-source-freeze-2026-10-03.md:3-7`).

The new regressions exercise the important boundaries (`ratelimit_test.go:330-407`). The excluded-method table compares status, body, and `Allow` against direct downstream handlers and checks that those requests leave a one-request shared budget available for POST registration. A valid GET authorization then verifies one consent row persisted before the callback redirects to login. POST authorization is compared with the pinned dependency's direct 405 response, and a subsequent POST confirms it spent the shared slot. No response-path expansion or dependency change was found.

Independent focused checks ran through the required build lease runner on the exact source fingerprint:

- Focused race tests passed before mutation and after restoring exact source (`focused-pass-01`, `focused-restored-pass-02`); both exits 0 and both recorded the clean source fingerprint.
- A compiled all-method behavioral mutant failed at `excluded method PUT /oauth/register consumed shared state budget` (`mutant-any-method-red-02`, exit 1 after test assertion).
- A compiled mutant that added HEAD to the authorize set failed when the later allowed registration received 429 (`mutant-head-charged-red-01`, exit 1 after test assertion).
- An earlier all-method attempt (`mutant-any-method-red-01`) failed at compile time because the loop variable became unused. It is explicitly excluded from behavioral RED evidence. The corrected mutation produced the meaningful RED above.
- All five stage leases were released successfully. The final restored stage returned to the same clean source fingerprint.

The coordinator separately reports exact-head full qualification at `9948c633`: 3,084 passing tests across 84 packages, full vet/lint/Linux ARM64 exit 0, and all four build leases released. This report covers independent source review and the focused behavioral controls; it does not infer live deployment, the 17-prefix refresh acceptance, 5000/minute capacity acceptance, or full SEC-H02/T24.39 closure. The new source slice is CLEAR; current plan steps and the narrow source claim remain governed by the coordinator's delivery workflow.

Original external report SHA-256: `cc27cd879660e242d1ce31a50375bf64181ee2e4bd9c9545a2853276b3154e62`; local locators redacted for publication.
