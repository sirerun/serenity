# Explicit offline fixture source adoption

`adopt-source` changes only `Target.Source` and the current `CustodyEpoch` in
the private version-2 fixture manifest. It retains the namespace, both complete
accounts and brains, credential inventory and exact retained key strings.
It does not issue or revoke credentials, initialize/migrate SQLite, provision
brains, enable capture or change public authentication.

The root operator still accepts only a private root proof pathname. The child
still accepts only the six inherited custody descriptors. There is no adoption
flag, environment override or manifest-derived authority.

## Proof contract

Ordinary operations retain their complete version-1 packet and purpose
`serenity-episode-fixture-offline-v1`. They reject adoption fields. Neither
`prepare`, `reissue` nor `revoke` substitutes an older source binding.

Adoption requires all existing custody fields, with these exact differences:

```json
{
  "version": 2,
  "purpose": "serenity-episode-fixture-offline-v2",
  "operation": "adopt-source",
  "source_adoption": {
    "old_source": "<exact 40-character lowercase old source revision>",
    "old_manifest_sha256": "<SHA-256 of the exact old private manifest bytes>"
  }
}
```

This fragment is not a complete proof. Existing `tool_source` binds the current
reviewed child source and must differ from `old_source`; `tool_sha256` binds its
actual executed bytes. Both producer and consumer reject missing, unknown,
duplicate or incorrectly versioned adoption fields. Existing root ownership,
descriptor pins, expiry, boot, current hold/state/config, schema fingerprint,
authority receipt, stopped writers/probes and one-use nonce gates remain.

The child hashes and decodes the same bounded private manifest bytes while
holding the existing `fixture.lock`. Adoption refuses a missing lock, incomplete
identities, any pending intent, unexpected reserved identities, human/billing/
partner state, unknown credentials, and any recorded credential not provably
revoked. Retained secrets pass the existing `Issuer.Verify` secret check and
offline revoked-key recovery; a matching prefix alone is insufficient.

## Database and publication boundary

The adoption dispatcher uses `store.OpenOfflineReadOnly`, supplied separately
by the parent integration lane. It must not use the ordinary `store.Open`
initializer. The read-only opener requires an already checkpointed, private
database with no WAL, SHM or rollback journal. It does not checkpoint or remove
sidecars. Immutable SQLite reads are valid only under the existing cold fence;
they do not establish that fence. The child then verifies the complete pinned
schema fingerprint and fixture state through that read-only connection.

After verification, the child writes a private temporary manifest, fsyncs it,
rechecks custody and the old manifest hash, atomically renames it, and fsyncs the
directory. Temporary files left by abrupt process death are never adopted as
authority. The two retained key strings remain revoked after success. A later
reissue is a separate operation requiring fresh root custody.

## Interruption and retry

| Exit | Meaning |
| --- | --- |
| 0 | Operation and final custody checks completed. |
| 2 | Adoption refused before publication. |
| 3 | Exact old manifest hash no longer matches; this invocation did not publish. Reconcile whether a prior invocation committed. |
| 5 | Observed interruption before publication; old manifest retained. |
| 6 | Publication or final custody outcome is uncertain; retain the hold and reconcile. |
| 1 | Unclassified/ordinary custody failure; never success. |

The wrapper reports unclassified adoption interruptions, killed children or
failed final checks as uncertain. The proof is consumed before launching the
child, including on failure. A replacement proof is fresh authority, not proof
of idempotence. After committed adoption, retrying with the old hash explicitly
refuses with exit 3 and does not rewrite the manifest or database. Do not change
the old hash/source merely to turn that refusal into a successful retry.

In-package tests cover the production read-only dispatch with a synthetic
custody callback, manifest and database invariants, and interruptions before
rename, after rename and after directory fsync. Python tests model root
orchestration without real UID transitions or live systemd/provider calls.
Actual inherited-root-FD execution, host cold-state qualification and any
production adoption remain separate parent-owned gates.
