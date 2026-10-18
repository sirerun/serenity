# Maintenance input qualification — T5.80–T5.85

Baseb9c8733789203e3b6915bfa311f0468362f139fd. Production source
a05f78efa7775cf2df77e16e6714bc4b7180899c; revised archive SHA256
c3275f3ea7e93005656a9babf17d6d9b6146d092dae191fe4379289af59a6697
matched on the existing Linux ARM64 runner. Final test-only tagged-switch commit
c3b7257a197bb83c1a3ce5f8b00e6fa966fd49b5 changes no production code; the exact
test overlay SHA256eaeb686cd5a7645d6e7834198629256d74818776a85c0f66e26c82df8d418d9d
was verified before its fresh focused/static checks.

`index.QualifyMaintenanceCopy` admits only an operation-owned, private, complete
CLOSED index.db family copy. It does not open the original, migrate, dispose of
copies, call providers or authenticate maintenance. Caller custody/cooperative
namespace exclusion remains required. Files are regular, single-link, user-owned
0600 under a canonical0700 directory with only index.db/WAL/SHM; aggregate family
size is32MiB, schema128rows/128KiB, inspection10seconds and SQLite busy timeout100ms.

Read-only/query-only SQLite includes WAL state. Exact sorted sqlite_schema rows,
including FTS shadows/autoindexes/triggers/views and SQL, must equal a fresh
current in-memory index; user_version/application_id must remain zero. Every
current RuntimeTables entry must be empty: jobs, disposition items/history,
spend, caches and events. Completed status is not discard permission. Derived
rows and private source-path mappings do not enter returned evidence. Results
contain schema/family hashes, sizes, fixed names and explicit sidecar effects.

The original retained canonical root is now pinned by a root descriptor during
Begin/ResumeBrainMaintenance. `ValidateFor` checks that exact root, runtime
directory, retained writer/view inodes and exact marker. Moving the complete
runtime directory into another brain cannot transfer ownership. Nil, empty,
closed/finished, wrong-pin, symlink, replacement, hardlink and unsafe-permission
states refuse without repair. This validates ownership, not journal authority.

Root audit corrections before aggregate review:

- Integrity checks follow exact schema qualification because SQLite quick_check
  evaluates CHECK constraints; unknown schema must not execute those expressions.
  See [SQLite documentation](https://www.sqlite.org/pragma.html#pragma_quick_check).
- Pinning compares the opened directory to the expected inode and rechecks the
  complete family before SQLite opens it. Substituted files/directories never
  reach the SQL phase in deterministic interleaving tests.
- Original-root descriptor retention closes the whole-runtime-move gap that
  matching lock/marker inodes alone would leave.

The initial index suite failed positive copies because read-only WAL-mode SQLite
created a zero-byte WAL and32KiB SHM. A bounded synthetic diagnostic observed
those exact effects. Only a previously absent private empty WAL may now appear;
existing WAL presence, inode and bytes must remain unchanged. New nonempty WALs
and modified/truncated/replaced/removed existing WALs refuse. Before/After records
make owned-copy side effects explicit. A negative mode fixture now uses explicit
chmod, avoiding a false expectation under restrictive umask. Original families
remain byte-identical in actual-pool and all-control-table tests.

Observed Go1.26.9/GOMAXPROCS2/private temporary storage, bounded serial commands:

- Full writer race52.663s at9b6745e0; writer production unchanged ata05f78ef.
- Full index race28.035s ata05f78ef, including canonical and ordinary pool paths.
- Focused writer owner validation race1.199s ata05f78ef.
- Test-only switch correction: focused replacement/change race1.450s,
  two-package lint zero issues. Affected vet passes; scoped govulncheck reports
  no vulnerabilities; CLI binary build passes.
- Formatting/diff checks and full Wazi177 pass; final secret scan and exact
  review/landing follow in the PR.

Independent non-author review9b6745e0 and revised WAL deltaa05f78ef found no
remaining findings. Final evidence/test-only review binds the complete candidate.
Source/synthetic checks do not qualify installed/native/provider or power-loss
behavior, runtime-artifact disposition, active installation or physical purge.
Default capture and production capabilities remain unchanged/off. T5.2/T5.3/T1.3
remain open. Two compatible outcomes under ceiling3, inherited session selection;
token/cost telemetry unavailable. Ajent MCP unavailable; repository/GitHub
coordination checked. ADR024 fallback applies only to confirmed billing-blocked
Actions without modifying protections or buying capacity.
