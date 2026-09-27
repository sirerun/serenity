# E24 -- Deep review 001 remediation

Acceptance: every finding in `docs/deep-reviews/001-full-codebase.md` has a recorded status (closed with a PR, deferred to T24.40, or accepted risk with David's name and date) in `docs/deep-reviews/001-remediation-status.md`; the five High findings are re-verified by a fresh trace; the three live-exposure fixes (T24.1, T24.2, T24.3) are verified on the hosted host, not only in tests.
fidelity: executable

## Context

Deep review 001 (2026-09-27, read-only, 14 agents, HEAD `0f55bf0`) found no Critical and five
verified High findings, about twenty Medium, sixteen Low and four Info, across security, AI and
agentic safety, supply chain, architecture, functional correctness, concurrency, infrastructure and
privacy. The review's own prioritized roadmap and per-finding fix code are the input to this epic;
finding ids (SEC-H01, FUN-04, ...) are stable and every task below names the ids it closes.

Two facts changed the priority since the review was written. `ajent.social` (2026-09-26) records
that the hosted candidate `v0.1.12-hosted-candidate` at `0f55bf0` is deployed and serving behind
Cloudflare, so SEC-H01 (root Caddy with an open admin API), SEC-H02 (OAuth denial from ~17
addresses), FUN-02 (every hosted remember with a relative TTL fails and burns quota) and FUN-01
(rejections strand quota) are live conditions, not pre-launch ones; `docs/launch/hosted-status.md`
still says no public service exists and is stale on that point. The same entry records that GitHub
Actions is blocked by an organization billing lock, so CI cannot run on pull requests until David
clears it (T24.41); until then E24 PRs carry local verification output and merge only with explicit
clearance.

David's rulings on 2026-09-27, each with an ADR: full erasure semantics for forget and delete
(ADR 019); trusted-proxy fix now and origin security-group lock-down after the ACME path is
confirmed (ADR 020); overlapping findings are amended into the owning hosted-completion contracts
and this epic owns the rest plus the idle deploy resources (ownership rule below); first-seen
machine claims from untrusted connector kinds wait in the inbox (ADR 022). ADR 018 records the
hardened git runner and the synced-config trust boundary; ADR 021 the redaction chokepoint.

Ownership rule. The hosted-completion epic (`docs/launch/hosted-plan.md`, tasks T23.41-T23.74,
registry `docs/launch/hosted-completion/tasks.json`) holds exclusive write claims on the hosted
packages, `stack.json`, the service units and the CI files. Findings inside those claims were
written into the owning contract's `steps` and `acceptance` on 2026-09-27 (T23.44, T23.45, T23.46,
T23.47, T23.48, T23.50, T23.51, T23.52, T23.54, T23.56, T23.57, T23.59) and each has a thin
verification row here (T24.31-T24.38) that is `blocked:` until the registry marks the task
accepted. Two exceptions are owned here because their lanes are idle and the exposure is live:
the Caddy and service units (T24.1, T24.2, claim `R-hosted-release`) and the release workflow
hygiene (T24.10, T24.11, same claim); T23.57 rebases onto them. Everything outside a hosted claim
(the OAuth package, the local product, the router, the docs-chat Lambda) is owned here directly.

Non-goals: no new product surface; no change to the four public hosted tools; no vendoring of
mcpoauth (deferred); no history rewrite of already-shipped release tags.

Success: milestones MS-R1 through MS-R4 below; the review's attack chains AC-1 through AC-4 each
have at least one link closed and recorded.

## Findings to tasks

| Finding | Severity | Owner here | Where the fix lands |
|---|---|---|---|
| SEC-H01 Caddy root + admin API | High 7.8 | T24.1 | deploy/hosted units (this epic, R-hosted-release) |
| SEC-H02 OAuth global caps | High 7.5 | T24.3 | internal/hosted/oauth (unclaimed) |
| SEC-H03 subject -> path/YAML | High 7.1 | T24.4, T24.5, T24.19 | extract, ingest, store, writer, index, compose |
| SEC-H04 gitrepo symlink | High 6.8 | T24.6 | connector/gitrepo |
| SEC-H05 serenity.yml trust + git exec | High 7.5 | T24.7, T24.8, T24.30 | internal/gitrun, config, cli; hosted sites via T23.45/46/48/49 |
| FUN-01, FUN-02, SEC-L05 | High impact | T24.31 verifies | T23.44 amendment |
| FUN-04 restore lockout | High impact | T24.36 verifies | T23.50 amendment |
| PRIV-01 erasure | High for any deletion claim | T24.21, T24.22, T24.23; T24.35 verifies | writer, index, store, docs; hosted via T23.48, T23.52, T23.54, T23.56 |
| ARC-H01 recovery layer dead | High impact | T24.31, T24.36 verify | T23.44 (reconciler), T23.50 (reactivation) |
| CON-01..04, ARC-M01..03 | Medium | T24.32 verifies | T23.45 amendment |
| CON-05, FUN-05, ARC-M04 | Medium/Low | T24.34 verifies | T23.47 amendment |
| CON-06 writer panic | Medium | T24.9 | internal/writer |
| SEC-M01, M02, M04, L02, L03 | Medium/Low | T24.33 verifies | T23.46 amendment |
| SEC-M03 edge IP keying | Medium | T24.1 | Caddyfile trusted_proxies |
| SEC-M05, AI-05 | Medium | T24.14 | internal/neutralize, compose, cli |
| SEC-L06 recall caps | Low | T24.18 | server/memory |
| SEC-L08 keychain ACL | Low | T24.28 | internal/secrets |
| SEC-INF-SECRET, SUP-02, SUP-01 | Medium/Low | T24.10, T24.11 | .gitignore, workflows, goreleaser |
| SUP-03 mcpoauth pin | Low | T24.40 (deferred) | -- |
| AI-01 planted claims | Medium | T24.16 | config, ingest, inbox, compose |
| AI-02, PRIV-02 redaction | Medium | T24.12 | redact, router, config, threat model |
| AI-03, AI-L02 actor forgery | Medium | T24.15 | server/disposition, direction, cli |
| AI-04 spend ceiling | Medium | T24.13 | router, eval, synthesize |
| AI-06 classifier | Medium | T24.17 | direction/check |
| AI-L01 gate | Info | T24.19 | internal/gate |
| AI-L03 forget authz | Low | T24.20; T24.32 verifies scope | server/memory, writer; gateway via T23.45 |
| AI-L04 system role | Low | T24.25 | router |
| FUN-03 batch abort | Medium | T24.4 | ingest |
| FUN-06 unwired features | Medium | T24.26; T24.34 verifies hosted | cli, spend, docs; billing via T23.47 |
| FUN-07 truncated export | Low | T24.35 verifies | T23.48 amendment |
| INF-01, INF-02, INF-04, INF-05 (stack) | Medium/Low | T24.37 verifies; T24.42 prerequisite | T23.54 and T23.52 amendments |
| INF-03, INF-05 (units, deploy) | Medium/Low | T24.2 | service units, deploy.sh |
| INF-06 bootstrap test | Low | T24.38 verifies | T23.51 amendment |
| INF-07 docs-chat | Low | T24.27 | deploy/chat |
| PRIV-03 absolute paths | Low | T24.24 | connectors, ingest, index |
| ARC-L01 contract scaffolding | Low | T24.40 (deferred) | -- |
| `_ =` audit, router_test masking | hygiene | T24.29, T24.13 | various |

## Waves

### Wave 1: Live exposure and the five Highs, all unblocked (9 agents)

Nine agent tasks plus one founder task. T24.1 and T24.10 claim `R-hosted-release`; run T24.10 after T24.1 lands if the same session holds the claim. Everything else is in unclaimed packages.

- [ ] T24.1 Caddy: drop root, admin API on a 0600 unix socket, trusted_proxies with CF-Connecting-IP  Owner: pool  Est: 90m  verifies: [UC-072]  acc: [deploy/hosted/tests/test_units.py passes asserting caddy.service sets User=caddy, NoNewPrivileges=yes and RuntimeDirectory=caddy, the Caddyfile global block sets admin unix//run/caddy/admin.sock|0600 and trusted_proxies with client_ip_headers CF-Connecting-IP, and deploy.sh reloads with --address unix//run/caddy/admin.sock]
- [ ] T24.3 OAuth limiter: per-prefix buckets first, refresh grants exempt, consents capped per client  Owner: pool  Est: 90m  verifies: [UC-073, UC-054]  acc: [go test ./internal/hosted/oauth passes a table test where 17 addresses each sending 120 requests per minute leave a refresh_token grant from an 18th address admitted, and 10000 live consents for one client_id do not block authorize for another client_id]
- [ ] T24.4 Slug validator at the model boundary and the writer; reject per observation, not per batch  Owner: pool  Est: 90m  verifies: [UC-074, UC-005]  acc: [go test ./internal/extract ./internal/ingest ./internal/store ./internal/writer passes new cases where a subject containing a newline or colon is dropped by filterCandidates with the batch continuing, safePart and publicationPath reject it, and RenderEntity returns an error instead of writing it]
- [ ] T24.5 Quarantine a single unparsable page instead of failing the brain; audit non-conforming slugs  Owner: pool  Est: 60m  verifies: [UC-074, UC-002, UC-011]  acc: [go test ./internal/index ./internal/compose ./internal/server/memory passes cases where one page with a duplicate frontmatter key is quarantined with a logged path while Rebuild and AllClaims succeed for every other page, and serenity check lists pages whose slug fails domain.ValidSlug]
- [ ] T24.6 git_repo connector: Lstat, regular-file check, symlink containment before every read  Owner: pool  Est: 45m  verifies: [UC-075, UC-008]  acc: [go test ./internal/connector/gitrepo passes a fixture where a tracked symlink to a file outside the repository yields a skipped entry and no bytes from the target]
- [ ] T24.7 internal/gitrun hardened git runner; migrate every local-product git call site  Owner: pool  Est: 90m  verifies: [UC-076, UC-040]  acc: [go test ./internal/gitrun passes a test where a repository whose config sets core.fsmonitor to a marker script never spawns it across ls-files, status and diff, and every non-test git exec under internal/cli, internal/connector, internal/writer, internal/direction, internal/supersede and internal/import routes through gitrun]
- [ ] T24.8 Config trust: KnownFields, connector path containment, loopback bind unless allow_lan  Owner: pool  Est: 60m  verifies: [UC-076, UC-001]  acc: [go test ./internal/config ./internal/cli passes cases where an unknown top-level key fails Load naming the key, a git_repo path outside the allowlisted roots is rejected at connector build, and a non-loopback server bind without allow_lan is refused]
- [ ] T24.9 Writer queue: recover a Render panic and return it as an error  Owner: pool  Est: 30m  verifies: [UC-055, UC-025]  acc: [go test ./internal/writer passes a case where a job whose Render panics returns an error to the caller and the queue keeps draining subsequent jobs]
- [ ] T24.10 Release hygiene: .env ignored, write token on the publish job only, pinned goreleaser, no tidy at release  Owner: pool  Est: 45m  verifies: [infrastructure]  acc: [.gitignore lists .env; .github/workflows/release.yml grants contents: write only on the publish job and contents: read at workflow scope; .goreleaser.yaml pins an exact goreleaser version and has no mod tidy hook]
- [ ] T24.41 Restore GitHub Actions billing so pull requests get CI again  Owner: David  Est: 15m  verifies: [infrastructure]  kind: human  acc: [gh run list --limit 1 shows a completed run on a pull request opened after the billing fix]  blocked: GitHub Actions billing lock reported 2026-09-26; only David can clear it

### Wave 2: Sprint and quarter tier outside hosted claims (19 agents)

Split into 2a (first ten rows) and 2b (the rest) if fewer than nineteen agents are available; no row in this wave depends on another except T24.2 on T24.1, T24.11 on T24.10, T24.19 on T24.4 and T24.42 on T24.1.

- [ ] T24.2 App unit denies IMDS; backup unit has an OnFailure handler; deploy.sh rolls back on failed readiness  Owner: pool  Est: 60m  verifies: [UC-067, UC-072]  deps: [T24.1]  acc: [deploy/hosted/tests/test_units.py asserts serenity-hosted.service contains IPAddressDeny=169.254.169.254 and serenity-backup.service does not, serenity-backup.service has an OnFailure= unit, and deploy.sh restores the previous binary when readiness fails within the retry window]
- [ ] T24.11 CI supply chain: SHA-pinned actions, govulncheck and gitleaks jobs, dependabot, cosign signatures, SBOM, provenance  Owner: pool  Est: 90m  verifies: [UC-083]  deps: [T24.10]  acc: [every uses: in .github/workflows is pinned to a 40-hex SHA with a version comment; a govulncheck job and a gitleaks job run on pull requests; .github/dependabot.yml covers gomod and github-actions; .goreleaser.yaml produces an SBOM and cosign keyless signatures for every archive and checksums file]
- [ ] T24.12 Redaction at the router chokepoint with widened key patterns and configurable rules  Owner: pool  Est: 90m  verifies: [UC-043]  acc: [go test ./internal/redact ./internal/router passes cases where sk-ant-, sk-proj-, sk-svcacct-, sk-or-, ghp_, xox[bpa]-, AIza, sk_live_, rk_live_ and AKIA shapes are redacted, and every provider Complete and Embed call in the router applies redaction before the request body is built]
- [ ] T24.13 Real CostUSD from a price table, MaxUSD enforced, ctx-aware retry, synthesize limits  Owner: pool  Est: 90m  verifies: [UC-029]  acc: [go test ./internal/router ./internal/server/memory passes cases where Anthropic and OpenAI-compatible responses yield a non-zero CostUSD from the price table, a Budget MaxUSD trips on that cost without a test-injected value, retry returns immediately on a cancelled context, and synthesize rejects the 61st call in a minute per account]
- [ ] T24.14 Output neutralizer: strip control sequences on every print and drop URLs absent from evidence  Owner: pool  Est: 60m  verifies: [UC-079, UC-011]  acc: [go test ./internal/compose ./internal/cli passes cases where a composer answer containing a markdown image URL absent from retrieved evidence is emitted without the URL, and ask, search and inbox print paths strip ESC and C0/C1 control characters from source and model text]
- [ ] T24.15 Disposition actor from the transport principal; precepts accepted only through the CLI; no provider error echo  Owner: pool  Est: 90m  verifies: [UC-080, UC-026, UC-041]  lane: agent  acc: [go test ./internal/server/disposition ./internal/direction ./internal/cli passes cases where POST /disposition/dispose with actor human:x over the bearer transport records actor agent:<principal>, a precept_draft accepted over HTTP is refused, inbox --apply prints the payload before publishing a precept, and check_plan returns a status code instead of a provider body on provider error]
- [ ] T24.16 Connector-kind trust: first-seen machine claims from untrusted connectors wait in the inbox; prompt carries actor and trust  Owner: pool  Est: 90m  verifies: [UC-078, UC-012]  lane: agent  acc: [go test ./internal/ingest ./internal/cli ./internal/compose passes cases where a first-seen non-conflicting machine claim from an imap or git_repo source lands in the inbox instead of state active, the same claim from a file source activates, and the composer prompt line for a machine claim carries its actor and trust]
- [ ] T24.17 Plan classifier fails closed and validates cited evidence against the plan text  Owner: pool  Est: 60m  verifies: [UC-021]  acc: [go test ./internal/direction/check passes cases where an empty action list, a below-threshold confidence, an out-of-set action and evidence not found in the plan text each yield unverified rather than no_applicable_constraints or pass]
- [ ] T24.18 Recall: cap query bytes and limit before any embedder call  Owner: pool  Est: 30m  verifies: [UC-055, UC-010]  acc: [go test ./internal/server/memory passes cases where a recall query over 4096 bytes and a limit over 100 return invalid_params without an embedder call]
- [ ] T24.42 Confirm the ACME challenge path and record it as evidence for the origin lock-down  Owner: pool  Est: 45m  verifies: [UC-072]  deps: [T24.1]  kind: any  acc: [docs/launch/evidence/E24/acme-path.md records the challenge type Caddy used for the last issuance from its log, whether renewal succeeds with the origin closed to non-Cloudflare sources, and the exact Cloudflare range source used]
- [ ] T24.19 Adversarial release gate runs the corpus through the real extract and compose pipeline; scanners cover pkg/ and cmd/  Owner: pool  Est: 90m  verifies: [UC-046]  deps: [T24.4]  acc: [go test ./internal/gate passes with every corpus document run through extract and compose against a scripted provider, a SEC-H03 subject payload fails the gate when the slug validator is disabled, and the AST scanner covers pkg/ and cmd/]
- [ ] T24.20 Facts record their writer; forget is restricted to the writer or a human; memory:forget scope  Owner: pool  Est: 60m  verifies: [UC-057, UC-035]  acc: [go test ./internal/server/memory ./internal/writer passes cases where a remembered fact records its writer principal and forget by a different non-human principal returns forbidden]
- [ ] T24.21 Forget and source tombstone delete index rows and working-tree bytes in the same flush  Owner: pool  Est: 60m  verifies: [UC-077, UC-035]  acc: [go test ./internal/index ./internal/store ./internal/writer passes cases where forget removes the fact's FTS and vector rows in the same flush and Tombstone removes a source's bytes and meta from the working tree]
- [ ] T24.24 Source URIs carry repository-relative or hashed identifiers, never absolute paths  Owner: pool  Est: 45m  verifies: [UC-040, UC-006]  acc: [go test ./internal/connector ./internal/ingest passes cases where a file and git_repo source record carries no absolute filesystem path]
- [ ] T24.25 Provider requests use a system role and per-call nonce-delimited documents  Owner: pool  Est: 45m  verifies: [UC-005, UC-011]  acc: [go test ./internal/router passes cases where Anthropic and OpenAI-compatible requests carry the instructions in a system role and document delimiters include a per-call random nonce]
- [ ] T24.26 Wire check_plan's router and disposed-effect application; document what serve does not start  Owner: pool  Est: 90m  verifies: [UC-021, UC-041]  lane: agent  acc: [go test ./internal/cli ./internal/server/direction passes cases where serve --http builds check_plan with a router and a disposed effect proposal is applied through spend.ApplyDisposedEffect, and docs/operator/mcp.md and the serve package doc no longer claim the cron daemon or unwired routes]
- [ ] T24.27 Docs-chat Lambda: ignore client-supplied assistant turns, per-IP daily cap, salt from a secret  Owner: pool  Est: 60m  verifies: [UC-039]  acc: [deploy/chat tests pass cases where client-supplied assistant turns in history are ignored, a single IP is capped per day, and RATE_SALT is read from a secret reference rather than a plain env value]
- [ ] T24.28 Keychain item ACL restricted to the serenity binary; document environment-variable key handling  Owner: pool  Est: 45m  verifies: [UC-030]  acc: [go test ./internal/secrets passes on darwin a case asserting the keychain item is created with an ACL naming the current executable, and docs/threat-model.md describes provider keys as environment variables]
- [ ] T24.29 Audit the 79 swallowed-error sites; fix the material ones outside hosted claims  Owner: pool  Est: 60m  verifies: [infrastructure]  acc: [docs/deep-reviews/001-remediation-status.md contains a table listing every non-test `_ =` assignment with a disposition, and the material ones outside hosted file claims are fixed with tests]

### Wave 3: History purge, drift test and verification of the amended hosted contracts (11 agents)

T24.22 and T24.23 run as soon as their deps land. T24.30 through T24.38 unblock one by one as the hosted-completion registry marks T23.44, T23.45, T23.46, T23.47, T23.48, T23.49, T23.50, T23.51 and T23.54 accepted; a session picks each up the day its block clears.

- [ ] T24.22 Forget rewrites brain history to drop the fact path; force-with-lease push with a warning  Owner: pool  Est: 90m  verifies: [UC-077]  deps: [T24.7, T24.21]  lane: agent  acc: [go test ./internal/writer passes a case where after forget no commit in git rev-list --all contains the fact path, the reflog is expired, and the post-commit hook pushes with --force-with-lease and prints the rewrite warning]
- [ ] T24.23 Disclosure alignment: threat model, README and operator docs state purge semantics and the backup window  Owner: pool  Est: 45m  verifies: [UC-065]  deps: [T24.22]  acc: [docs/threat-model.md, README and docs/operator describe forget as a purge with history rewrite, export default as history-free, and backups expiring within 31 days, and a doc test finds no remaining 'logical', '60 day' or unimplemented deletion-chain text]
- [ ] T24.30 Drift test: no raw git exec outside internal/gitrun anywhere in the module  Owner: pool  Est: 30m  verifies: [UC-076]  deps: [T24.7, T24.32, T24.33, T24.35]  acc: [go test ./internal/gitrun passes a repository-wide test asserting that exec.Command or exec.CommandContext with a git argument appears in no non-test Go file outside internal/gitrun]  blocked: T23.49 accepted in docs/launch/hosted-completion/tasks.json (backup.go call sites)
- [ ] T24.31 Verify T23.44 closed FUN-01, FUN-02 and SEC-L05 with named regression tests  Owner: pool  Est: 45m  verifies: [UC-081, UC-071, UC-060]  acc: [go test ./internal/hosted/gateway passes tests named for FUN-01, FUN-02 and SEC-L05: a keyed hosted remember with ttl 30d stores the fact, a limit-exceeded rejection leaves no pending_review row and releases the reservation, and a retry with the same key and a different visibility returns the writer conflict]  blocked: T23.44 accepted in docs/launch/hosted-completion/tasks.json
- [ ] T24.32 Verify T23.45 closed CON-01..CON-04, ARC-M01..ARC-M03 and the hosted forget scope  Owner: pool  Est: 45m  verifies: [UC-068, UC-058, UC-064]  acc: [go test ./internal/hosted/pool ./internal/hosted/gateway passes tests named for CON-01 through CON-04, ARC-M01 and ARC-M02: a failed runtime close is evicted and does not poison Acquire, contention returns no false capacity error, cold open and embedding run outside the pool and gateway locks, an export streams from a temp file after releasing its slot, and a brain with a missing .git fails closed]  blocked: T23.45 accepted in docs/launch/hosted-completion/tasks.json
- [ ] T24.33 Verify T23.46 closed SEC-M01, SEC-M02, SEC-M04, SEC-L02 and SEC-L03  Owner: pool  Est: 45m  verifies: [UC-049, UC-050]  acc: [go test ./internal/hosted/identity ./internal/hosted/dashboard ./internal/hosted/service passes tests named for SEC-M01, SEC-M02, SEC-M04, SEC-L02 and SEC-L03: the magic link is consumed only by a POST bound to a nonce cookie, unknown addresses under invite-only get the same 200 page, the account cap counts only active accounts, and sessions carry an absolute ceiling and a __Host- cookie]  blocked: T23.46 accepted in docs/launch/hosted-completion/tasks.json
- [ ] T24.34 Verify T23.47 closed FUN-05, CON-05 and ARC-M04  Owner: pool  Est: 45m  verifies: [UC-063]  acc: [go test ./internal/hosted/billing passes tests named for FUN-05 and CON-05: a failing Scan propagates instead of feeding blank prior state into graceDeadline, and a webhook that pages Stripe does not hold the mutex that checkout needs]  blocked: T23.47 accepted in docs/launch/hosted-completion/tasks.json
- [ ] T24.35 Verify T23.48 closed the hosted half of PRIV-01 and FUN-07  Owner: pool  Est: 45m  verifies: [UC-065, UC-064]  acc: [go test ./internal/hosted/gateway ./internal/hosted/deletion passes tests named for PRIV-01 and FUN-07: account and brain deletion purge bytes, history and index rows through the journal, export defaults to a history-free archive with --with-history opt-in, and a mid-stream export failure returns an error status with a log line]  blocked: T23.48 accepted in docs/launch/hosted-completion/tasks.json
- [ ] T24.36 Verify T23.50 closed FUN-04  Owner: pool  Est: 45m  verifies: [UC-082, UC-066]  acc: [go test ./internal/hosted/recovery ./internal/hosted/backup passes tests named for FUN-04: restore writes into a temp directory and renames on success so a partial failure leaves an empty destination and a rerun is accepted, and after reconciliation a fresh magic-link login succeeds for a surviving account]  blocked: T23.50 accepted in docs/launch/hosted-completion/tasks.json
- [ ] T24.37 Verify T23.54 and T23.52 closed INF-01, INF-02, INF-04 and INF-05  Owner: pool  Est: 45m  verifies: [UC-066, UC-067]  acc: [python3 -m unittest deploy/hosted/tests/test_stack.py passes tests named for INF-01, INF-02, INF-04 and INF-05: the AMI id is a pinned literal with UserData that re-bootstraps, the security group ingress on 80 and 443 lists only Cloudflare ranges, the bucket lifecycle expires noncurrent versions within 1 day, and every alarm has an AlarmActions topic]  blocked: T23.54 accepted in docs/launch/hosted-completion/tasks.json
- [ ] T24.38 Verify T23.51 closed INF-06 and installs Caddy under the unprivileged unit  Owner: pool  Est: 45m  verifies: [infrastructure]  acc: [deploy/hosted/test_bootstrap.py passes in CI with the 64-hex checksum regex, and bootstrap creates the caddy user and state directories that T24.1's unit expects]  blocked: T23.51 accepted in docs/launch/hosted-completion/tasks.json

### Wave 4: Closure (1 agents)

One frontier subagent; it must not have implemented any High fix.

- [ ] T24.39 Closure: re-verify every High, record remediation status, and hand the residue to the deferred tier  Owner: pool  Est: 60m  verifies: [infrastructure]  deps: [T24.1, T24.2, T24.3, T24.4, T24.5, T24.6, T24.7, T24.8, T24.9, T24.22, T24.23, T24.31, T24.36]  lane: agent  acc: [docs/deep-reviews/001-remediation-status.md lists every finding id from the review with status closed, deferred or accepted-risk, a PR link for each closed one, and the five High findings each re-verified by a fresh trace recorded in the file]

### Deferred (outline) -- tech-debt tier

Intent: the review's tier-4 items that do not gate safe operation: SUP-03 (vendor or fork a
reviewed mcpoauth revision once upstream tags exist), ARC-L01 (mark or gate the unconsumed
contract scaffolding in `internal/hosted/contracts`), the `sk_live_` credential prefix rename to
stop Stripe-scanner collisions (hosted credential package), coverage gaps the review named
(`dashboard_test.go`, `gateway_test.go` for callBound, lifecycle tests) not already owned by
T23.55 and T23.59, and any `_ =` sites T24.29 classified as material inside hosted claims.
Exit: each item has a row with an acc line or a recorded accepted-risk.

- [ ] T24.40 PLAN: expand the deferred tech-debt tier to executable fidelity (informed by T24.39's residue)  Owner: pool  Est: 1h  kind: plan  delivers: [docs/plans/deep-review-001-remediation.md deferred section at fidelity: executable]  deps: [T24.39]  acc: [parse_plan.py sees E24 with at least 5 new tasks under the deferred section, every task carries an acc line and a verifies link, deps resolve]

## Milestones

| ID | Milestone | Exit criteria | Depends on |
|---|---|---|---|
| MS-R1 | Live exposure closed | T24.1, T24.2, T24.3 merged, deployed through deploy.sh, and verified on the hosted host (no :2019 listener, Caddy as caddy, IMDS refused, 17-IP test green); T23.44 dispatched with the FUN-01/FUN-02 steps | T24.1, T24.2, T24.3, T24.42 |
| MS-R2 | Highs closed | T24.4-T24.9 merged; T24.31 and T24.36 green (FUN-01/02, FUN-04 verified in the accepted hosted tasks); T24.30 drift test green | wave 1, T24.31, T24.36, T24.30 |
| MS-R3 | Quarter tier closed | every wave 2 row merged; T24.22, T24.23 merged; T24.32-T24.35, T24.37, T24.38 green | wave 2, wave 3 |
| MS-R4 | Review closed | T24.39 status file complete; roadmap Shipped entry; T24.40 planned | T24.39 |

## Risks

| ID | Risk | Impact | Likelihood | Mitigation |
|---|---|---|---|---|
| RR1 | Caddy certificate re-issuance during the privilege drop hits Let's Encrypt rate limits | TLS gap on the live host | Medium | bootstrap migrates `/root/.local/share/caddy` before the first start under the new unit; verify no issuance in the Caddy log; keep the previous unit for rollback (T24.1) |
| RR2 | CI billing lock stays; PRs merge on local checks | Regression slips | High until T24.41 | `ci-blocked` label, local `go test -race` and lint output in every PR body, no merge without explicit clearance per PR |
| RR3 | Hosted-completion tasks that carry the amended findings are weeks away (T23.44 has no claim; T23.50 sits behind T23.47, T23.48, T23.49) while FUN-02 burns live quota today | Customer-visible failure persists | High | Hand-off note asks the hosted coordinator to dispatch T23.44 first; if it is not claimed within 3 days David is asked whether E24 should take `R-hosted-runtime` for a scoped FUN-02 fix |
| RR4 | History rewrite on forget surprises a user with a shared brain remote | Broken clones | Low | force-with-lease plus a printed warning; documented in forget help and ADR 019 |
| RR5 | KnownFields(true) breaks existing brains with unknown keys | Load failure | Medium | error names the key; `serenity check` reports it; release notes call it out |
| RR6 | Redaction at the router changes extraction eval scores | Eval drift | Low | cached eval run in T24.12 and T24.25; thresholds unchanged |
| RR7 | The Cloudflare range allowlist goes stale | Requests from new Cloudflare ranges are treated as direct | Low | the test pins the source URL and date; T24.40 adds a refresh check |

## Operating notes

Definition of done, claims, worktrees, build lease and merge gate follow `docs/plan.md` section
8. In addition for E24: every PR body carries the genuine-red output (the new test failing on
the unchanged code) and the green output; every row's contract is at
`docs/tasks/deep-review-001/<row-id>.md`; a row whose files sit under a hosted claim is never
edited directly, it is an integration request to the owning T23 task. Hosted rows verified on the
host use the read-only commands in the contract; no mutating action on the host outside
`deploy.sh` and the reviewed path.

## Progress log

- 2026-09-27 Groomed from `docs/deep-reviews/001-full-codebase.md`: 42 rows (T24.1-T24.42 including the planning row T24.40), 41 with acc lines, 5 `lane: agent`, 1 `kind: human`, 1 `kind: any`, 10 blocked rows (8 registry-blocked verification rows, T24.30 on T23.49, T24.41 on billing); ADR 018-022 written; T23.44/45/46/47/48/50/51/52/54/56/57/59 amended in `docs/launch/hosted-completion/tasks.json` and re-rendered; UC-072-UC-083 added to the manifest; contracts under `docs/tasks/deep-review-001/`.
