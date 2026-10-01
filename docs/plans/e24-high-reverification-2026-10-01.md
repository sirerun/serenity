# E24 High-finding re-verification — 2026-10-01

This is a fresh source-and-test review of the five verified High findings in
[`001-full-codebase.md`](../deep-reviews/001-full-codebase.md), against the
implementation at `7b0baad24f39ce2888fc8a2f801421a83dabc389` (the worktree
contains documentation-only commits after that source baseline). The review
followed each hostile input through the relevant entry point, guard, sink and
existing regression tests. I inspected the tests but did not rerun them; other
lanes are already running focused race suites. No hosted instance or live
deployment was accessed.

The local code closes the core attack paths for SEC-H01, SEC-H02 and the
static-file case in SEC-H04. SEC-H03 now blocks new model-controlled slug
injection, but a parseable pre-fix alias-injection artifact can remain active
if one was written before the fix. SEC-H05 is only partially closed: local
Git calls use the hardened runner, while hosted production packages still
spawn Git directly. Its connector-root extension also comes from the synced
config whose trust is in question. These findings do not establish overall
T24.39 completion.

| Finding | Local source review | Remaining evidence or path |
|---|---|---|
| SEC-H01 — root Caddy/admin API | Checked-in service and Caddy config now use an unprivileged account and a mode-0600 Unix admin socket. | Static unit/bootstrap tests do not prove the deployed host is running them. No live `:2019` listener or service identity was checked. |
| SEC-H02 — OAuth availability | The old global counter no longer covers refresh/token/revoke; source-prefix limits run first, and state creation has a higher, separate ceiling. Consent capacity is per client. | Registration and authorization still share a deliberate 5,000-per-minute global ceiling. No live traffic or database-load qualification was performed. |
| SEC-H03 — model subject to path/YAML | New model subjects, writer inputs and publication paths are checked; frontmatter is YAML-marshaled; malformed pages are quarantined across read paths. | A validly parsed alias-injection page created before the fix is not necessarily quarantined or reported by the current page audit. No evidence establishes whether such an artifact exists in any brain. |
| SEC-H04 — Git connector symlinks | Static tracked and nested symlinks that escape the repository are skipped before their target bytes enter source ingestion. | The check resolves a path and later reads it by name, not through an already-open confined file handle. Concurrent local mutation of an ancestor is outside the static fixture coverage. |
| SEC-H05 — synced config and Git execution | Local config decoding is strict; connector paths are checked; local Git runs disable `core.fsmonitor`; foreign Git also disables hooks and limits commands. | The connector-root allowlist is itself in synced `serenity.yml`, and production `internal/hosted` Git commands remain outside `internal/gitrun`. |

## SEC-H01 — Caddy privilege and admin API

`deploy/hosted/caddy.service` now runs as `caddy`, grants only
`CAP_NET_BIND_SERVICE`, and applies `NoNewPrivileges`, `ProtectSystem=strict`,
`ProtectHome`, and private temp/state/runtime directories. The Caddy global
block in `deploy/hosted/Caddyfile` binds its admin API to
`unix//run/caddy/admin.sock|0600`, and both the unit's `ExecReload` and
`deploy.sh` address that socket. `bootstrap.sh` creates the `caddy` account
and migrates existing certificate state before the unit starts.

The static regression coverage is in `deploy/hosted/tests/test_units.py`:
`CaddyServiceTests.test_runs_as_the_caddy_user_with_only_the_bind_capability`,
`test_filesystem_is_sandboxed`, `test_reload_uses_the_unix_admin_socket`,
`CaddyfileTests.test_admin_api_is_a_0600_unix_socket`, and
`BootstrapTests.test_migrates_root_certificate_storage_before_the_unit_starts`.
These pin the files and migration script, but they do not inspect an installed
host. The plan's live criterion still requires observing Caddy as `caddy` and
no TCP listener on port 2019 after deployment; this review did not do that.

## SEC-H02 — OAuth rate limits and consent storage

`internal/hosted/oauth/hosted.go` applies the per-prefix limiter around the
OAuth mux. Registration and authorization additionally share the
`StateCreating` ceiling (5,000 per minute); `/oauth/token` and `/oauth/revoke`
have per-prefix limits but no state-creation global limiter. In
`ratelimit.go`, a saturated per-prefix request is rejected before it can
charge the global counter, and the key is IPv4 `/24` or IPv6 `/56`. The trusted
client-IP helper accepts `X-Serenity-Client-IP` only from a loopback peer; the
deployed Caddy config replaces that header with the address resolved from
trusted Cloudflare ranges. `oauth/store.go` caps live consents at 500 per
client and evicts expired rows during insert, so one client cannot fill a
10,000-row consent table for every client.

The regression tests include
`TestSECH02SeventeenAddressFloodLeavesRefreshAdmitted`,
`TestSECH02IPv6RotationWithinPrefixIsOneKey`,
`TestSECH02ConsentCapIsPerClient`,
`TestSECH02TableCapNoLongerAppliesToConsents`,
`TestSECH02GlobalCeilingCoversOnlyRegisterAndAuthorize`, and
`TestSECH02GlobalCeilingIsAtLeastTenTimesPerKey` in
`internal/hosted/oauth/ratelimit_test.go`. The test suite exercises the prior
17-address exhaustion shape, refresh admission and the consent-table boundary.
The 5,000 state-creation ceiling remains a deliberate shared limit: a
distributed flood can still defer new registration/authorization until its
window resets, while existing refresh grants remain outside that ceiling.
Source review establishes the limiter wiring, not service capacity under live
traffic.

## SEC-H03 — model-controlled entity subject

`internal/domain/slug.go` defines the canonical slug grammar. The extraction
candidate filter in `internal/extract/extract.go` drops invalid subjects
individually, preserving the rest of a response batch. Ingest revalidates
observations before paths are formed; `FenceWriter.RenderEntity` validates
both type and slug and serializes frontmatter with `yaml.Marshal`; publication
paths reject control characters and symlink components. These stop the
newline/colon subject from becoming either a path segment or a YAML key.

The new-write guards have direct coverage in
`TestValidSlug` (`internal/domain/slug_test.go`),
`TestExtractDropsUnsafeSubjectAndControlObjectKeepsBatch`
(`internal/extract/slug_test.go`),
`TestBatchDropsUnsafeObservationsAndPublishesRest` and
`TestReviewDropsUnsafeSubjectAndKeepsBatch` (`internal/ingest/slug_test.go`),
and `TestRenderEntityRefusesInvalidSlugOrType` plus
`TestFrontmatterIsYAMLMarshalled` (`internal/store/slug_test.go`).

The corrupt-page guard is tested across consumers by
`TestRebuildQuarantinesUnparsablePage` and
`TestAuditPagesReportsQuarantineAndNonConformingSlugs`
(`internal/index/quarantine_test.go`),
`TestAllClaimsQuarantinesUnparsablePage` (`internal/compose/quarantine_test.go`),
and `TestEntityToolQuarantinesUnparsablePage`
(`internal/server/memory/entity_quarantine_test.go`). However,
`index.WalkEntityPages` quarantines parse errors and empty slugs; the audit
reports non-conforming *frontmatter slugs*. It does not compare a page's
filename to its parsed slug or establish alias provenance. The original
finding also described a variant that parses and carries an injected alias.
If that variant was already written before the guard landed, the shared page
walk can still index it and the entity handler can still consider it. The
tests above quarantine the duplicate-key case, not that parseable legacy
variant. This is a conditional migration/incident residue, not evidence that
new model output can still create it. There is no repository-wide evidence
here that a poisoned page exists.

## SEC-H04 — symlinked Git connector files

`internal/connector/gitrepo/gitrepo.go:readContained` checks the listed path
with `Lstat`, refuses non-regular files, resolves the path and repository
root, and refuses a resolved path outside the root. `Poll` records such a path
as skipped and continues without returning its bytes. The listing uses
`gitrun.Foreign`, so the same connector path does not invoke repository
`core.fsmonitor`, hooks, global/system Git configuration, or write-capable Git
subcommands.

`TestPollSkipsTrackedSymlinkOutsideRepository`, `TestPollSkipsNestedSymlinks`,
and `TestPollReadsRegularFilesUnchanged` in
`internal/connector/gitrepo/gitrepo_test.go` cover the tracked escape,
nested file/directory links, and normal-file control case. The previously
reported static symlink escape is closed. `Lstat`/`EvalSymlinks` followed by
`os.ReadFile(real)` still form a path-based check/use sequence. A local actor
able to replace an ancestor during that sequence could race it; the tests use
static fixtures and do not exercise concurrent mutation. This is a narrower
local race boundary than the reviewed remote symlink fixture.

## SEC-H05 — synced config and hardened Git execution

The local config half now rejects unknown keys recursively (`KnownFields(true)`
and the strict key walk in `internal/config/config.go`), uses typed connector
configuration, validates paths at connector construction, and requires
loopback HTTP bind unless `server.allow_lan` is explicit. Existing tests
include `TestLoadRejectsUnknownTopLevelKey`,
`TestLoadRejectsUnknownNestedKey`, `TestLoadRejectsUnknownConnectorKind`,
`TestBuildConnectorsRejectsGitRepoPathOutsideRoots`,
`TestBuildConnectorsRejectsTraversalOutOfRoot`, and
`TestServeHTTPRefusesNonLoopbackBindWithoutAllowLAN`.

`internal/gitrun/gitrun.go` adds `-c core.fsmonitor=false` and
`-c protocol.ext.allow=never` to both runner types. `Foreign` additionally
disables hooks, global/system config and optional index writes, and allows only
read-only subcommands. Its environment drops inherited `GIT_*` variables
except `GIT_SSH_COMMAND`, pins terminal prompts off, and rejects caller options
that could redirect or reconfigure the runner. `TestHostileFsmonitorNeverSpawns`,
`TestHardeningPrefixAndEnvironment`, `TestForeignAllowsOnlyReadOnlySubcommands`,
and `TestNoRawGitExecOutsideGitrun` in `internal/gitrun/gitrun_test.go` cover
the monitor payload, argument/environment contract and the local-product
call-site inventory. The first test has an unhardened control that must run
the fixture monitor before asserting that `Brain` and `Foreign` do not.

Two boundaries remain material:

1. `connectors.roots` is an explicit allowlist extension in accepted ADR 018,
   but it is read from the same synced `serenity.yml` as `git_repo[].path`.
   `connectorRoots` appends any absolute configured root, and
   `TestBuildConnectorsAcceptsPathUnderConfiguredRoot` confirms an
   out-of-home path is accepted when added there. Thus it is an accepted
   configuration policy, not an independent machine-local trust boundary. If
   the SEC-H05 attacker can publish synced config, that attacker can also
   publish the root extension. The policy needs an explicit trust decision
   before path containment can be described as protecting against hostile
   synced config; ADR 018 currently permits the extension.
2. The local drift test explicitly scans local-product packages only. Raw
   production Git commands remain in `internal/hosted/gateway/lifecycle.go`,
   `internal/hosted/pool/pool.go`, `internal/hosted/provision/canonical.go`,
   and `internal/hosted/backup/backup.go`. These paths do not inherit
   `internal/gitrun`'s `core.fsmonitor` override, environment scrub or, where
   applicable, Foreign command restrictions. Their reachability from
   attacker-controlled hosted repository state needs its own fresh trace;
   until the hosted call sites migrate or are shown to operate only on
   separately trusted state, the module-wide SEC-H05 claim is not closed.

The current `TestNoRawGitExecOutsideGitrun` comment says its roots cover the
local packages and that T24.30 widens it to the module after hosted migration.
`docs/plans/deep-review-001-remediation.md` still records T24.30 as blocked on
T23.49. Evaluation utilities also contain direct Git invocations, but they
are not production connector/service paths and are not included in this
finding's runtime residue.

## Verification boundary

This pass was source-first and read the existing regression tests; it did not
run them, contact a hosted endpoint, inspect an installed service, or establish
that any pre-fix hostile brain was scanned and repaired. The project record
`ajent.social` was read; the Ajent feed MCP/tool was unavailable in this
environment. These findings are a fresh local trace, not live qualification
and not completion of T24.39.
