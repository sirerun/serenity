# T23.49 real journal caller integration design — 2026-10-01

Source baseline: canonical `origin/main` at `4f573b31ff3ee148d0a6bf38b9c05c1a1586a03e` (PR #331). This is a read-only integration design; it does not claim T23.48/T23.49 acceptance, production readiness, credential qualification, or restore activation authority.

## Finding

The current mainline has journal contracts and a storage implementation, but neither production deletion nor backup is wired to a real journal. `Service.New` constructs the store, then `Assemble` constructs the gateway and performs local deletion recovery before setting up handlers (`internal/hosted/service/service.go:208-310`). Neither method receives a `contracts.DeletionJournal`; `Gateway` has no journal field and the lifecycle methods do not append entries. `Service.Backup` takes the maintenance write lock, flushes the pool, and calls the old `backup.Create(ctx, dataDir, destination)` API (`internal/hosted/service/service.go:358-366`); that API writes the current manifest format without a journal watermark (`internal/hosted/backup/backup.go:42`).

The journal itself supports opaque account/brain IDs, `requested` and `purged` outcomes, durable append, reads, and generation seals (`internal/hosted/contracts/deletion.go:12-90`; `internal/hosted/deletion/journal.go:210-256,302-366`). It is not enough to connect only the backup reader: until production deletion appends intents and outcomes, a real but empty journal can yield a plausible watermark that says nothing about deletions.

## Minimal integration sequence

1. **Resolve production journal configuration first.** Have one production factory construct exactly one `contracts.DeletionJournal`, then inject that same instance into `Service`, `Gateway`, and backup. Preserve the current test assembly seam for explicit fakes. Do not construct separate journal objects per caller: the concrete journal carries synchronized writer/head state. A missing or unhealthy journal must fail closed for backup and deletion durability.

2. **Append intent before irreversible lifecycle effects.** For an account or brain delete, append `DeletionIntentRequested` with only the opaque subject type and ID before acknowledging the request, setting a durable deleting state as the first externally visible lifecycle transition, or beginning provider closure/purge. If append fails, leave the subject usable only according to the existing fail-closed deletion state rules and return an error; never claim durable deletion intent. Keep billing closure outside the journal lock and preserve its current retryable/pending semantics.

3. **Serialize account deletion as one maintenance operation.** `Gateway.DeleteBrain` currently holds `Maintenance.RLock` for one brain, but `Gateway.DeleteAccount` marks the account deleting and iterates brain deletions without holding a whole-operation read lock (`internal/hosted/gateway/lifecycle.go:80-115`). `Service.DeleteAccount` freezes the account and closes billing before calling the gateway (`internal/hosted/service/deletion.go:14-44`). Add a lifecycle-scoped read fence around the complete account operation, including its intent and outcome, while avoiding recursive acquisition when account deletion invokes per-brain deletion. Keep the existing backup write lock and flush (`Service.Backup`) so backup observes either the pre-operation state or the completed operation, never a partially erased account.

4. **Append completion only after all required work succeeds.** Append `DeletionOutcomePurged` only after required provider closure is confirmed, all owned brain files/history/index data have been erased, credentials/sessions/partner links have been revoked or removed, and account anonymization commits. If provider closure is pending/uncertain, retain the restricted deleting state and the durable intent; do not append `purged`. Any failed append leaves a retryable pending deletion and the API must report failure. Recovery must replay idempotently from the durable intent and append the completion record only after the same postconditions hold.

5. **Recover before serving handlers, and fail closed on journal uncertainty.** `Assemble` currently calls local provisioning recovery and `recoverDeletions` before assigning the handler (`service.go:257-310`). Keep that order, but make journal integrity/read/recovery failure abort assembly; do not fall back to SQLite-only recovery or serve readiness/handlers after the journal cannot be verified. Local `deleting` rows can guide retry work but cannot replace independent journal history.

6. **Make backup v2 consume the same journal.** Under the existing maintenance write lock, flush all runtimes, call the candidate manifest-v2 backup API with the explicit release build SHA and the injected journal, and fail before reporting/publishing a backup if `ReadThrough` cannot establish the required complete history. Do not synthesize an empty watermark. Keep backup output and journal storage separate. Verify a real appended lifecycle entry appears in the generated manifest watermark.

## Dependency and activation limits

- `deletion.NewAWSJournal` currently uses AWS SDK default configuration/credential loading (`internal/hosted/deletion/config.go:26-52`). Current hosted service policy denies IMDS, and no approved scoped credential source is wired. Do not infer that the default chain can assume an EC2 role, remove IMDS denial, or grant broad host credentials. T23.54 must define the scoped object-store credential/config contract before production construction.
- A configured static journal generation is not adoption proof. The journal protocol proves completeness relative to objects and seals visible in the object store; it does not prove the writer generation was authoritatively adopted or that an old credential was revoked. T23.50 must supply the fenced generation/adoption barrier, and T23.54 must supply the IAM/credential fence before restore activation or production lifecycle claims.
- Keep T23.52 upload/retention, T23.50 restore activation, and provider/live qualification separate. No production provider, cloud, or deployment calls are part of this design.

## Integration tests for the later implementation

- Use the real journal implementation over its filesystem object-store fixture; append an account and brain intent/outcome through production lifecycle paths, then verify backup manifest watermark includes those exact records.
- Inject append/read failures and prove backup/deletion fails closed, no success response is emitted, and pending deletion remains restricted and retryable.
- Prove the same journal instance reaches gateway and backup; test startup recovery failure prevents handler serving.
- Hold account deletion in the provider-closure pending state while backup races; assert snapshot coherence and that no `purged` event appears. Retry to completion and assert exactly one effective purge outcome.
- Keep generation/adoption and credential fencing tests owned by T23.50/T23.54; a valid local seal alone must not satisfy them.
