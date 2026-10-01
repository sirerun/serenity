# Hosted recovery implementation — 2026-10-01

Baseline main: `42f6dfd8615b193fffb797175daf4b5f5d02dc94` (PR325). Local validation is authorized by ADR024; Actions account lock is not a test failure. Existing checkout material and draft PR273 are preserved.

## Ownership and delivered patches

- Coordinator owns integration, migration9, operation ledger lookup, Git runner and shared records. Migration9 preserves applied migrations1–8 and scopes retry identity to original quota period. Store/ledger race and lint passed; schemas1–8 upgrade twice, partner data retained, future10 rejected. Old lookup reproduction failed by replaying an earlier period.
- Luna billing lane owns `internal/hosted/billing` and T23.47 evidence. Persisted reads now fail closed except absent rows; grace is returned from the committed transaction. Coordinator independently discarded read errors and observed reconciliation/webhook regression failures, then restored and passed billing race. Fixture statement failures exercise real SQLite transactions, not injected disk I/O or live Stripe.
- Luna canonical lane owns gateway, server memory, writer and canonical memory payload. Trusted context carries the ledger ID/callback; validation happens before the queued source-write callback, and the durable payload keeps a separate internal ID. Client JSON cannot set that identity. Coordinator removed the callback and observed both boundary/refusal regressions fail; restored focused race passed.
- Luna backup lane recovered only backup-package commits from PR273; its 37-test package race and lint passed. It remains separate from journal and shared callers. Generic quarantine clone was rejected in review; the narrowed CloneBundle helper runs from a fresh private workspace with ancestor discovery disabled. The backup lane is adapting to that API before integration.

## Review and gates

The first integrated gate at `13ae1a4` passed full race (78 tested packages, five packages without tests, six explicit test skips), full vet and full lint (zero issues). The final integrated gate at `8dbf9bf` passed full race in78 tested packages (five packages without tests and six explicit test skips), full vet and full lint (zero issues). An earlier attempt held before claiming the build lease because one-minute load was above10; the subsequent run executed below10 and released its exact lease.

Independent review reproduced a repository-specific transport-policy override in the initial Git helper. The followup adds explicit built-in transport overrides, disables lazy fetching, refuses generic quarantine cloning, and provides a regular-file-only CloneBundle API from a fresh private workspace. Helper race/lint passed; independent followup review is pending. This helper alone does not migrate production backup callsites.

## Remaining implementation contracts

- T23.44 remains partial: production canonical checker, fence through Flush, startup/ticker reconciliation, operator resolution and OS-enforced physical staging bounds are absent. The payload marker may be removed by forget/history purge; future absence proofs must not infer that a mutation never landed merely because its memory source is gone. A durable content-free operation marker or an explicit unknown outcome is required.
- Period-scoped ledger keys do not alone qualify gateway/client-key normalization, durable writer retry semantics or the complete fingerprint fields. Those remain explicit integration work.
- T23.47 remains partial: production reconciliation and resumable closure assembly, complete provider lifecycle/portal fixtures and live qualification are open. Production billing remains disabled.
- T23.49 remains partial: real independent T23.48 journal, build identity and partner-aware service/CLI wiring are required. Never fabricate a zero watermark or reuse the expiring backup bucket as a journal substrate.
- No new hosted binary, ingress lockdown, live charge or public launch is implied by these patches. Draft PR273 remains held for recovery review.

## Assembled canonical regression

The coordinator added `TestInvalidRememberReleasesOperationBeforeCanonicalEntry` through real service assembly and SQLite: invalid TTL releases the reservation without entering canonical state, a corrected same-key retry commits one write, and its source carries the committed ledger ID. Restoring the old gateway implementation makes that test fail at runtime with `phase=pending_review`; restoring the fix passes the focused service race check. This regression is supplemental to the source-pinned full gate above; no new implementation changed after that gate.

## Review followup assignment

Independent review confirmed the initial two-field remember fingerprint permits a changed visibility/provenance/TTL/entity/kind to replay a committed key before the writer can reject it. The coordinator holds the canonical-write merge for this fix. The Luna backup/reviewer lane now owns gateway fingerprint normalization and a shared pure server-memory preflight helper, plus standalone request/replay regression tests. No schema or writer edits are assigned in this followup. The fix must preserve normalized default equivalence and reject changed or invalid payloads before replay; T23.44 reconciliation/storage conditions stay open.
