# Independent current-main five-High delta and finding-ledger review

**Recommendation: HOLD the original proposal as a final or integratable T24.39 record.** Its 59-row inventory is exact, but its current-main claims are stale after PR #348, and its ledger marks 21 findings as formally deferred even though the remediation plan does not place them in the named deferred tier. Preserve the original proposal as a bounded historical proposal; do not treat this review as T24.39 acceptance, founder risk acceptance, hosted acceptance, or a change to the shared status ledger.

## Review basis and limits

Reviewed current `main` commit `8081632ac88bddf68e4e7f73cd670a8b5434bc5b` (PR #349, tree `01721c8c8173ba2b36b038631722124131d63095`) and assignment commit `5911ab415bf150c7bec2775a02a8e4eb663d6a58`. The bounded original proposal is commit `bf96d3cfaa6f4258a47e69a487a509a0f11b973e`, file `docs/plans/high-retrace-review-proposal-2026-10-02.md`, blob SHA-1 `8038366f1012d1f5f7520b3ef18065f7529a709e`; it says its source basis is `4b2fd8e656e448d712b2c7d9fa2bc28b9d78c469` (PR #347). The original deep review is `docs/deep-reviews/001-full-codebase.md`; the frozen contract is `docs/tasks/deep-review-001/T24.39.md`.

This was a source and recorded-receipt review. I did not run Go builds/tests, query a hosted machine/provider, deploy, or inspect customer data. Actual Ajent feed and coordination board were checked; no Ajent MCP was available. The feed and board record PR #348 as landed at `2fee7e636e9149fe42ad214866117bc971cc8f84`, with source tree `f011c37a1fc1f9581351c776c6a7542e1da32b00` matching the reviewed candidate. PR #349 then produced current `main` `8081632`. The module-wide scanner remains a separate assigned lane and is not source or task acceptance here.

## Five High findings on current main

| Finding | Current-main source trace and delta | Bounded disposition |
|---|---|---|
| SEC-H01 — Caddy privilege/admin API | `deploy/hosted/caddy.service` runs as `caddy`; `deploy/hosted/Caddyfile` uses the mode-0600 Unix admin socket and trusted proxy configuration; `deploy/hosted/deploy.sh` reloads through that socket. E24 T24.1 records PR #284. | **Implemented in source; live acceptance open.** This review does not prove the deployed host has these files/processes, that TCP `:2019` is absent, or that reload/ACME behavior passed on-host. |
| SEC-H02 — OAuth global-cap denial of service | `internal/hosted/oauth/hosted.go` applies the per-prefix limiter first, then a 5,000/minute global limit only to registration/authorization state creation. Token and revoke calls have separate per-prefix ceilings and do not consume that shared state-creation budget. `internal/hosted/oauth/store.go` caps live consents per client at 500; the generic 10,000-row cap remains for other tables. T24.3 records PR #285. | **Source mitigation implemented; closure/live qualification open.** The proposal's shared registration/authorization ceiling is still a possible state-creation bottleneck, but the original cross-endpoint refresh denial and cross-client consent-cap mechanism are no longer accurate descriptions of current behavior. Do not call the task formally deferred. |
| SEC-H03 — model subject/path/YAML and alias hijack | T24.4/#299 validates subjects at model/write boundaries; T24.5/#292 quarantines parse failures and reports invalid slugs; T24.19/#312 exercises the corpus through extraction/composition. Current parsing still accepts syntactically valid aliases into entities; page audit does not establish filename-to-parsed-slug identity or alias provenance. | **Partial; open/unverified.** Legacy or manually authored pages with valid syntax and injected aliases remain a conditional artifact question. No source review establishes that such an artifact exists in a real brain. |
| SEC-H04 — git_repo symlink escape | Current `internal/connector/gitrepo/gitrepo.go` pins a non-symlink repository root with `os.OpenRoot`, checks the opened root identity, retains it across Poll, checks root identity before reads, opens entries through that root, and validates the opened descriptor. Linux/Darwin use nonblocking/no-follow flags. PR #348 landed the corrected connector source. | **Closed for merged local source scope; broader/live acceptance remains separate.** Do not repeat the original proposal's path-check/read race as current-main behavior. The prior control history matters: the first unchanged-source single-run fixture did not fail; the preserved 50-run baseline produced 10 actual outside reads. Initial candidate `ce5ce63` was held for late root opening and uncounted raced-leaf errors. Corrected root pin and skip behavior was independently cleared at `a501ed56`; PR #348 then landed after combined local qualification. Root replacement and counted-skip controls are recorded. Platform-specific no-follow guarantee is limited to Linux/Darwin; mount points and hard links were not claimed as blocked. |
| SEC-H05 — synced config trust and Git execution | The trust concern remains: `internal/config/config.go` validates schema, while `internal/cli/connectorbuild.go` adds absolute `connectors.roots` from brain-synced config to the allowlist. A synced config can therefore nominate a broad root; strict parsing alone does not make the policy independently trusted. Delta: all four identified eval raw-Git callers (fixtureprep, BrainBench `gen_trend`, BrainBench `publish_trend`, calibration) were migrated to `gitrun.Foreign` in PR #348, and those changes are in current main. | **Partial; open/unverified.** The four-caller raw Git claim in the original proposal is stale. T24.30 whole-module scanner/acceptance is still open and separately assigned after PR #349; do not infer scanner enforcement from caller migration. No hostile synced-config end-to-end case or hosted data path was verified here. |

The H04 evidence distinguishes (a) merged current-main source and its local qualification, (b) prior held candidate evidence, and (c) live/product acceptance. The scanner lane is distinct from H04 connector fix status. The source-level H04 hold on the earlier candidate was later corrected and lifted only for the exact PR #348 candidate; historical holds are not evidence that current main is still held, and a scanner hold/unfinished scanner is not evidence that the scanner merged.

## Exact 59-ID set and classification audit

I compared the original review's distinct finding IDs against the proposal's finding-ID table. Result: **59 source IDs, 59 proposal rows, 59 unique proposal IDs, no missing IDs, no extra IDs, and no duplicate table IDs.** The command result and complete sorted set are preserved as `finding-id-set.txt` in the external evidence bundle.

The rows below retain the original proposal's classification and assess whether the stated basis survives the plan/receipt check. “Closed (local)” is not live acceptance. “Open / unverified” is an uncertainty label, not the T24.39-required disposition of deferred or named accepted risk. `T24.40` is only a planning task; the current deferred outline names SUP-03 and ARC-L01, plus generic follow-ups. It does not formally defer every open finding.

| ID | Original proposal classification | Independent assessment |
|---|---|---|
| AI-01 | Closed (local) | Supported as local-only: T24.16/#303 records connector-kind trust work. |
| AI-02 | Closed (local) | Supported as local-only: T24.12/#294 records router redaction. |
| AI-03 | Closed (local) | Supported as local-only: T24.15/#306 records transport actor binding and CLI-only precept acceptance. |
| AI-04 | Closed (local) | Supported as local-only: T24.13/#301 records budget enforcement. |
| AI-05 | Closed (local) | Supported as local-only: T24.14/#304 records output neutralization. |
| AI-06 | Open / unverified | Stale/incomplete: T24.17/#291 records the classifier fail-closed change. Local closure evidence exists; broader acceptance remains outside this source review. |
| AI-08 | Open / unverified | No closure receipt was established in the reviewed task mapping; retain as open pending disposition evidence. |
| AI-L01 | Deferred (recorded) | **Incorrect formal deferral.** T24.19/#312 records the release-gate change; AI-L01 is not listed as a current named deferred item. Assess local closure from that task receipt. |
| AI-L02 | Deferred (recorded) | **Incorrect formal deferral.** T24.15/#306 covers actor forgery; not in the named deferred tier. Assess local closure from that receipt. |
| AI-L03 | Deferred (recorded) | **Incorrect formal deferral.** T24.20/#308 covers writer identity/forget authorization; not in the named deferred tier. Assess local closure from that receipt. |
| AI-L04 | Deferred (recorded) | **Incorrect formal deferral.** T24.25/#300 covers system role and nonce-delimited documents; not in the named deferred tier. |
| ARC-H01 | Open / unverified | Correctly remains open; T24.31/T24.36 are blocked on hosted task acceptance. Not formally deferred. |
| ARC-L01 | Deferred (recorded) | Supported as an outline-level deferral: named explicitly in T24.40 deferred outline. T24.40 itself remains unchecked. |
| ARC-M01 | Deferred (recorded) | **Incorrect formal deferral.** T24.32 is a blocked hosted verification row; the deferred outline does not name ARC-M01. |
| ARC-M02 | Deferred (recorded) | **Incorrect formal deferral.** T24.32 is blocked; not listed as deferred. |
| ARC-M03 | Deferred (recorded) | **Incorrect formal deferral.** T24.32 is blocked; not listed as deferred. |
| ARC-M04 | Deferred (recorded) | **Incorrect formal deferral.** T24.34 is blocked; not listed as deferred. |
| CON-01 | Open / unverified | Open label supported; T24.32 verification is blocked. The record should cite that blocked task rather than say no closure record exists. |
| CON-02 | Open / unverified | Open label supported; T24.32 verification is blocked. |
| CON-03 | Open / unverified | Open label supported; T24.32 verification is blocked. |
| CON-04 | Open / unverified | Open label supported; T24.32 verification is blocked. |
| CON-05 | Deferred (recorded) | **Incorrect formal deferral.** T24.34/T23.47 is the explicit owner and remains blocked/held; the generic deferred outline does not convert this into accepted deferred work. |
| CON-06 | Open / unverified | Open label supported; no closure evidence was established in this review. |
| FUN-01 | Open / unverified | Open label supported; T24.31 verification is blocked on T23.44. |
| FUN-02 | Open / unverified | Open label supported; T24.31 verification is blocked on T23.44. |
| FUN-03 | Closed (local) | Supported as local-only: T24.4/#299 records per-observation rejection. |
| FUN-04 | Open / unverified | Open label supported; T24.36 verification is blocked on T23.50. |
| FUN-05 | Deferred (recorded) | **Overstated.** T24.29 says hosted billing Scan errors are owned by T23.47; T24.34 verification is blocked and the feed records the lane held. Do not report accepted/closed deferral. |
| FUN-06 | Deferred (recorded) | **Incorrect formal deferral.** T24.26/#298 records local feature wiring/docs; any hosted billing residue remains governed by its separate task, not formal deferral of FUN-06. |
| FUN-07 | Deferred (recorded) | **Incorrect formal deferral.** T24.35/T23.48 is blocked; the deferred outline does not name FUN-07. |
| INF-01 | Open / unverified | Open label supported; T24.37 is blocked on hosted infrastructure acceptance. |
| INF-02 | Open / unverified | Open label supported; T24.37 is blocked. |
| INF-03 | Implemented locally; live acceptance open | Supported at this evidence boundary; deployed-host state was not queried. |
| INF-04 | Deferred (recorded) | **Incorrect formal deferral.** T24.37 is blocked; not named in deferred outline. |
| INF-05 | Deferred (recorded) | **Incorrect and conflates two mappings.** T24.37 is blocked for stack evidence while T24.2/#310 records local unit/deploy source work; neither makes INF-05 formally deferred. |
| INF-06 | Deferred (recorded) | **Incorrect formal deferral.** T24.38 is blocked; not named in deferred outline. |
| INF-07 | Deferred (recorded) | **Incorrect formal deferral.** T24.27/#290 records local docs-chat hardening, while E24 continuation explicitly says live Lambda still lacks the change; this is local implementation with live acceptance open. |
| PRIV-01 | Partial; open/unverified | Supported as partial: local deletion work does not satisfy blocked hosted T24.35/T23.48 evidence. |
| PRIV-02 | Closed (local) | Supported as local-only: T24.12/#294 records router redaction. |
| PRIV-03 | Deferred (recorded) | **Incorrect formal deferral.** T24.24/#297 records repository-relative/hashed source URI change; not named as deferred. |
| SEC-H01 | Implemented locally; live acceptance open | Supported; see High trace and T24.1. |
| SEC-H02 | Defer closure | **Needs relabeling.** Source mitigations are in T24.3/#285; describe the remaining state-creation/live qualification as open. It is not a recorded deferred disposition. |
| SEC-H03 | Partial; open/unverified | Supported as conditional residue; distinguish a possible legacy artifact from a demonstrated artifact. |
| SEC-H04 | Partial; open/unverified | **Stale after PR #348.** The corrected contained-read implementation is in current main with independent root-replacement and counted-skip controls plus combined local qualification. The original pathname-race claim must not be carried forward as current. |
| SEC-H05 | Partial; open/unverified | Residual trust issue is supported, but the four raw eval Git callsites are stale after PR #348; whole-module scanner remains open. |
| SEC-I01 | Open / unverified | No closure or named deferral evidence was established here; do not call this task-complete. |
| SEC-L02 | Deferred (recorded) | **Incorrect formal deferral.** T24.33 is a blocked hosted verification row; not named as deferred. |
| SEC-L03 | Deferred (recorded) | **Incorrect formal deferral.** T24.33 is blocked; not named as deferred. |
| SEC-L05 | Deferred (recorded) | **Incorrect formal deferral.** T24.31 is blocked; not named as deferred. |
| SEC-L06 | Open / unverified | Stale/incomplete: T24.18/#289 records recall byte/limit caps. Local closure evidence exists; broader acceptance is separate. |
| SEC-L08 | Deferred (recorded) | **Incorrect formal deferral.** T24.28 is explicitly unchecked; keychain ACL work is an open task, not a recorded deferral. |
| SEC-M01 | Open / unverified | Open label supported; T24.33 verification is blocked. |
| SEC-M02 | Open / unverified | Open label supported; T24.33 verification is blocked. |
| SEC-M03 | Partial; live acceptance open | Supported; T24.1 records local proxy configuration, but origin ingress/live edge behavior remains open. |
| SEC-M04 | Open / unverified | Open label supported; T24.33 verification is blocked. |
| SEC-M05 | Closed (local) | Supported as local-only: T24.14/#304 records terminal-control stripping. |
| SUP-01 | Closed (local) | Supported as local-only: T24.11/#311 records release controls. |
| SUP-02 | Closed (local) | Supported as local-only: T24.11/#311 records workflow token-scope changes. |
| SUP-03 | Deferred (recorded) | Supported as outline-level deferral: named in T24.40 deferred outline; T24.40 is still unchecked. |

## Material corrections before any final ledger use

1. Replace the H04 row with the current-main PR #348 source outcome and preserve the first non-failing single run, 50-repeat baseline result (10 real outside reads), held `ce5ce63` findings, corrected root/skip controls, and exact merged-source boundary. Do not claim a live or whole-task acceptance from it.
2. Replace the H05 four-raw-eval-callsite statement with the four migrated callers on current main. Keep synced-root trust as an open residue, and keep T24.30 scanner status separate and open.
3. Change the 21 “Deferred (recorded)” labels identified above to the evidence-backed local/blocked/open labels. Only ARC-L01 and SUP-03 are explicitly named in the deferred outline; FUN-05 is a hosted-owner hold, not accepted deferred work. The outline itself is not yet executable and T24.40 is unchecked.
4. Add the actual PR reference/link for every finding classified closed in any T24.39 final table. The original proposal's text references task/PR numbers in reasons but is not a complete acceptance ledger, and it does not provide named accepted risks for unresolved findings.
5. Preserve the exact 59-ID set, but distinguish classification from evidence level: a command grant or completed row checkbox cannot establish David's risk acceptance, live T24.1/.2/.3, hosted task acceptance, or T24.39 completion.

## T24.39 contract result

T24.39 requires every finding once with closed/deferred/accepted-risk status, a PR link for every closed item, fresh five-High traces, and live verification for T24.1, T24.2, and T24.3. This review provides source-level delta and finds the proposal's table inventory exact, but it does not satisfy the live requirement or resolve every open finding into a named deferral or explicitly named/date-stamped acceptance. **T24.39 remains open.** This report is a review handoff only; do not edit the shared ledger or infer acceptance from it.
