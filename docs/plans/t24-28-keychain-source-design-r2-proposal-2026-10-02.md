# T24.28 source design revision 2: original tuple, atomic Add, legacy classification

Date: 2026-10-02. This is a new design revision; the prior design artifact remains unchanged. No repository source, keychain, provider, or existing item was changed or queried in this work. The runtime evidence referenced below is from earlier explicitly owned disposable keychains and is summarized in `/Volumes/BuildOffload/serenity-t24-28-native-readiness-and-legacy-blocker-20261002.md`.

## Updated recommendation

Keep the original `(Service, account)` pair for every daemon token and profile token. Keep the public `Service` constant, profile-account derivation, and API-level token values stable. Create newly absent items with native `SecItemAdd` and a Serenity-executable ACL, in that exact service/account slot. Do not use a second native service namespace.

A second namespace would isolate old items from native reads, but it also lets old released binaries miss new tokens and permits two records for the same logical account. Keeping one tuple retains discovery by old binaries and lets Keychain's duplicate-item constraint refuse a concurrent legacy/native create on the same attributes. The new code must never use go-keyring's `Set`/`-U` for native records. `SecItemAdd` plus `errSecDuplicateItem` gives an atomic no-overwrite barrier; after duplicate, classify the existing exact record and return or fail, never update it as an Add retry.

This is the better compatibility boundary, but it is not proof that every old release can read or rotate a new item. The old go-keyring backend invokes `/usr/bin/security`; an old binary should find the same tuple and receive the keychain authorization prompt. If the user permits access, go-keyring must decode the stored value correctly. The denied/no-UI reader probe established access denial with zero bytes, not that the system displayed a prompt. The visible prompt and old-release decode/rotation behavior still need an isolated, signed fixture acceptance test.

## Exact stored value format

The pinned `github.com/zalando/go-keyring v0.2.8` Darwin implementation writes:

`go-keyring-base64:` + `base64.StdEncoding.EncodeToString([]byte(value))`

and its `Get` recognizes that prefix and returns the decoded original string. Store this complete prefixed ASCII envelope as the native item's `kSecValueData`, rather than storing a new private raw format. Native reads decode the same envelope and return the same logical token string as before. For current daemon/profile tokens this is the same 64-character hex credential callers already use. This lets an older go-keyring reader, after the user grants access, see the same item and decode the same token without a second record. Malformed prefixed data is a typed format error; it is not returned as an opaque token or treated as not-found.

The evidence for this encoding is the pinned module source at `/Volumes/BuildOffload/go/pkg/mod/github.com/zalando/go-keyring@v0.2.8/keyring_darwin.go` (the `Get` prefix decoder and `Set` prefix/base64 encoder). It was not re-tested against an old released binary in this turn. A fixture-only test must compare the native stored bytes to the v0.2.8 envelope and verify the same CLI output/decode path before claiming downgrade compatibility.

## Exact tuple classification and safe routing

The same tuple contains both historical CLI items and newly restricted native items. Do not infer the backend from a failed data read. In particular, a native `errSecAuthFailed` or `errSecInteractionNotAllowed` must never cause a call to the legacy reader.

Before data access, perform an exact service+account metadata lookup, without requesting password bytes. The classifier should use a version marker written only by the native creator (for example a dedicated generic attribute) plus the item's ACL identity metadata. The version marker identifies an item as native even if the currently running executable no longer matches its original ACL after an upgrade. For a marked native item, attempt the native read; any access denial remains an access denial and is returned without fallback. For a positively identified legacy item, use the old go-keyring path so its existing interaction behavior remains intact. For an absent marker with ambiguous/unreadable ACL metadata, return a typed unknown-policy/denied error; do not guess legacy and do not create or overwrite.

Classifier decisions:

| Exact metadata result | Read behavior | Ensure/create behavior |
| --- | --- | --- |
| Exact item is marked native; ACL identity matches current Serenity | Native read; return typed result | Reuse item; never Add |
| Exact item is marked native; ACL identity does not match (including a possible release upgrade) | Return typed access/identity error; no legacy fallback | Do not overwrite or recreate |
| Exact item is positively identified as historical CLI format | Existing go-keyring backend, retaining its prompt/error behavior | Return the existing logical token; do not create a second item |
| Exact tuple is definitely absent (`errSecItemNotFound`) | Not-found | Generate value, encode v0.2.8 envelope, call native `SecItemAdd` |
| Duplicate Add caused a race | Re-run metadata classification on the same tuple | Reuse a confirmed record or return its typed error; never issue `SecItemUpdate` |
| Metadata query fails, ACL/marker is malformed, or item kind is ambiguous | Preserve a typed OSStatus/classification error | Fail closed; no fallback and no creation |

A native marker alone is not enough to classify marker-less data as legacy: an old binary's `security add -U`, user changes, or corruption could alter metadata. Before implementation, determine whether a stable marker survives that old update path using only an owned fixture. Combine marker state with a known legacy metadata/ACL signature, and make every unrecognized combination `ErrItemPolicyUnknown`. If the historical CLI signature cannot be identified robustly, there is no safe automatic classifier on the single tuple; the unresolved cases need an explicit migration/re-entry decision rather than a fallback. The legacy probe did show that metadata-only find/provenance/ACL inspection can succeed while a data read is denied, but it did not capture a durable fingerprint for the legacy trusted application. Do not overstate that result as a complete classifier.

`SecItemAdd` should include exact service/account, generic-password class, encoded data, the native version marker, and `kSecAttrAccess`; it must not use `kSecUseKeychain` on a read/update query. A duplicate is not absence. Only `errSecItemNotFound` from the exact query is an absence signal; `errSecAuthFailed`, `errSecInteractionNotAllowed`, duplicate, timeout, malformed metadata, or any other OSStatus is not. Apple defines these statuses separately in its [Security Framework Result Codes](https://developer.apple.com/documentation/security/security-framework-result-codes?changes=_8).

## Public API behavior and compatibility exception

Keep `DaemonToken`, `EnsureDaemonToken`, `RotateDaemonToken`, profile APIs, and their signatures unchanged. Keep the account names unchanged. Keep `ErrNotFound` meaning exact absence only; add typed access-denied, identity-mismatch, malformed-value, and ambiguous-policy errors that preserve the underlying OSStatus.

- `Get`: classify first. Native item means native read only. Confirmed legacy item means existing backend read, so old credentials remain addressable. Do not use a native data denial as a trigger for legacy fallback.
- `Ensure`: reuse either confirmed existing kind without changing its backend. Only exact absence allows native creation. If `SecItemAdd` returns duplicate, reclassify the winner. Never turn an unknown or denied result into a new token.
- `Rotate`: keep the current caller-visible behavior of producing a new token. Update a confirmed native item with data only, retaining its ACL and encoding. Keep a confirmed legacy item on the existing legacy rotation path until migration is reviewed. Create a restricted native item only when the exact tuple is definitely absent. If the legacy operation prompts or fails, preserve that error rather than creating a competing record.

The unavoidable compatibility exception is that grandfathered CLI items keep their prior ACL and may still be readable by the same-user CLI without the new restriction. T24.28's ACL/prompt objective applies to newly created native items until a separate, reviewed migration exists. Likewise, a legacy operation can lose access in a headless/no-UI context; preserve the legacy backend's behavior and return an error rather than silently minting a replacement. A same-user process that is not on the new ACL should be prompted for a new native item; an operator may authorize it, which can change the ACL, so document that user choice as a possible trust expansion.

## Executable identity and old/new binaries

Use the canonical `os.Executable()` path to create the trusted-application reference. Store the native version marker so an ACL mismatch after an executable update remains classified as a native identity failure, never as an invitation to fall back to the CLI. Apple describes trusted-application data as opaque identity data that can include a cryptographic hash ([API reference](https://developer.apple.com/documentation/security/sectrustedapplicationcopydata%28_%3A_%3A%29?language=objc)). The scratch proof verified a canonical creator identity and a distinct reader, but not release-to-release continuity.

Before rollout, run two actually signed Serenity releases against an owned fixture at the same install path: Release A creates; Release B reads and data-only rotates with the intended stable release signing identity; a different designated requirement is denied. Also run an older-version reader against that exact service/account and assert a prompt in an interactive fixture environment plus the v0.2.8 envelope decode. A separate `security` process or different identity may ask the user; no-ui denial alone is not proof of a visible prompt. Do not use the user's default keychain for these tests. A stale native ACL must produce an explicit recovery error; never reset, re-create, or overwrite the token automatically.

## Process interaction state

The native call must run with keychain interaction disabled because the tested file-keychain path did not honor the query flag as a sufficient UI guard. The setting is process-wide. Protect every `internal/secrets` native operation and every lookup/create transition with one package mutex; save the prior state, disable and verify, perform the exact operation, restore prior state in a defer, then unlock. Return a restoration failure and never leave the process silently disabled. Keep random generation outside the guard. Legacy CLI fallback occurs only after the native scope restores interaction and the item is positively classified as legacy. The current Go tree has no other direct Security.framework caller; `go-keyring` invokes the `security` subprocess. If another in-process Security caller is added, it must share the gate or the concurrency guarantee must be revisited.

## Frozen acceptance and test plan

The frozen T24.28 acceptance still requires: the item's ACL lists the Serenity executable path; an off-Darwin skip with a reason; corrected provider-key/environment language in the threat model; a genuine red test against unchanged behavior before the fix; and the listed race test, gofmt, vet, and changed-package lint checks under their required lease.

Minimal later implementation files:

- `internal/secrets/secrets.go`: preserve public API/account names, add a native/legacy classifier/router and typed errors, keep mock injection, serialize Ensure/Rotate routing.
- `internal/secrets/keychain_darwin.go`: exact-tuple metadata classifier, version marker, restricted `SecItemAdd`, envelope encode/decode, native value-only update, OSStatus mapping, scoped interaction guard.
- `internal/secrets/keychain_other.go`: existing go-keyring backend for non-Darwin unchanged.
- `internal/secrets/secrets_test.go`: injectable fake tests for tuple classification, native-denial no-fallback, exact-notfound creation, legacy reuse, duplicate Add, malformed marker/envelope, profile names, rotations, and concurrent Ensure.
- `internal/secrets/keychain_darwin_test.go` and a small different-identity fixture reader under `internal/secrets/testdata/`: owned temporary keychain only; exact tuple, ACL app identity, no-UI denial/no bytes, state restoration, v0.2.8 encoding, duplicate Add refusal, `security` reader/decode, and exact cleanup. Skip off Darwin with a reason. Keep a separate gated GUI test for the visible prompt.
- `docs/threat-model.md`: preserve the environment-variable description for provider API keys; distinguish restricted newly created daemon/profile items from grandfathered CLI items and connector credentials. Do not claim every existing macOS item is restricted.

For genuine red without a default-keychain query, first provide a behavior-neutral fixture/provider injection seam. Add the routing/ACL test before changing its behavior, run it against the unchanged production backend with an owned temp keychain adapter, and record the predicted failure. If the current go-keyring path cannot be redirected into that fixture without modifying behavior, keep the test-seam change behavior-neutral and report that limitation for review; never create a unique record in the user's default keychain to obtain a red result.

After review and implementation, run the unchanged-code red and fixed green, then the frozen verification list in `docs/tasks/deep-review-001/T24.28.md`. No production patch is authorized by this revision.
