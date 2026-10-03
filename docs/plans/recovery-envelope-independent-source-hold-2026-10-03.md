# Independent pure recovery-envelope source review

Verdict: HOLD at `eb10eda801e36ab29fae9d9ca017e69be27a2460` for the frozen `recovery-envelope-v1` decoder bounds contract.

I reviewed the exact source in an isolated detached clone on `[external evidence path omitted] with no source modifications. The six new `internal/hosted/recovery/envelope*.go` files and the implementation receipt are the component scope. The embedded frozen contract hash matches `424a1dfa2159f0e591d830dd21949eedc903edf1858526c61c4add24d677ed75`.

## Blocking finding: field and collection limits are applied after allocation

`DecodeRecoveryEnvelopeV1` checks the 4 MiB input cap and UTF-8, then calls `scanStrictEnvelopeJSON`, which enforces nesting depth, integer syntax and duplicate object keys. It does not cap per-field string bytes, array lengths, object member counts, or known path-specific collection sizes. It also has no context check within token scanning. The subsequent `json.Decoder.Decode(&wire)` constructs the typed slices and strings before `ValidateRecoveryEnvelopeV1` checks the envelope's 10,000-account/brain/disposition limits, 100 eligible IDs, per-brain/aggregate head ceilings, and field-specific string limits. `DisallowUnknownFields` likewise rejects unknown keys only during this later typed decode, after the strict scanner has already accumulated every key in an unbounded `seen` map for that object.

A <=4 MiB input can therefore encode a very large array of `null` elements for a struct slice such as `snapshot_account_inventory`; `encoding/json` accepts null for struct elements and allocates the typed slice before the later count check. Similar expansion is possible for large arrays of empty strings and object-member maps. This violates the frozen contract's explicit requirement that bounds be checked before decoding nested payloads or allocating slices, and that individual string limits be enforced. The whole-input cap makes the work finite, but does not make it satisfy the specified pre-allocation limits.

Fix by adding a bounded preflight parser that tracks field path and enforces exact string, array, member, depth, and aggregate limits before typed decoding, with cancellation checked during the traversal; or use an equivalent decoder that can enforce these limits during allocation. Unknown keys should fail in preflight too. Add a behavioral regression showing over-limit input is rejected before constructing oversized slices, plus the existing valid maximum-bound acceptance cases.

## Other reviewed areas

The remaining pure component logic appears aligned with the frozen contract:

- Private ordered wire structs preserve exact field order and tags. Decode rejects invalid UTF-8, duplicate keys (including keys equal after JSON escape decoding), unknown fields, noninteger number syntax, trailing values, and noncanonical re-encoding. `DisallowUnknownFields`, typed decoding and canonical-byte comparison are present.
- Journal `M_w`, positive `C`, `H`, and old writer `G` remain separate. Structural bounds allow historical `C` (`C.Generation <= G`) and historical `H` without inventing ancestry. Generation-one genesis and later successor allocation identities are structurally represented. These values remain inert; no source authority is inferred.
- The tagged union excludes cross-arm data. Frozen dispositions cover the complete inventory only in `FROZEN_ONLY`; eligible IDs are a sorted subset of active/restore-pending snapshot rows and the allowlist, with exact contract-plan account equality.
- Contract plan hash is recomputed with the existing `CanonicalContractRecoveryPlanHash`; legacy artifact and outer envelope hashes remain distinct. The two inventory domains use their prescribed `serenity.recovery-brain-inventory.v1\x00` and `serenity.recovery-snapshot-inventory.v1\x00` prefixes; the outer domain is separate. The summary hash is recomputed from fields the envelope actually carries, while the full-row helper checks brain projection and artifact count/byte arithmetic with checked addition. No digest reconstructs verified source inspection.
- UTC RFC3339Nano round-trip, closed statuses/dispositions, source token vs integer schema version, nested slice deep copies, nil/empty slice rules, and the 1 MiB RCP plan limit appear represented. The RCP domain behavior is not modified.

## Validation scope

I did not run builds or tests because the shared build lane was reserved for the root full-module qualification at review time. The author receipt reports focused race/vet/lint and behavioral hash/union/duplicate-key mutants; those are author evidence, not independent execution by this reviewer. This HOLD is based on direct source/contract mismatch and does not imply a failure in authority-bearing startup, provider, READY, restore or admission work; those remain outside this pure codec's scope.
