# T24.28 source design: restricted new items, legacy compatibility boundary

Date: 2026-10-02. Design only; no repository files, keychains, providers, or existing items were changed or queried. Probe evidence is summarized in `/Volumes/BuildOffload/serenity-t24-28-native-readiness-and-legacy-blocker-20261002.md`.

## Recommendation

Keep every exported `internal/secrets` function, token value, `Service` constant, and logical account name unchanged. On Darwin, create new credentials in a separate, versioned native service namespace using Security.framework `SecItemAdd` and a `SecAccessCreate` ACL that names the canonical running Serenity executable. Keep existing `security`/go-keyring items in their current backend; do not read them and rewrite them into native items automatically. This is creation-time protection, not retroactive migration.

The successful disposable-keychain probe establishes the new-item path: native Add, exact fixture provenance, creator-only trusted-app identity, silent creator read, data-only update, and a separately signed reader denial all passed with interaction disabled. The legacy CLI item behaved differently: the no-UI native read returned `errSecAuthFailed`, and two bounded metadata-only ACL setters did not return within 15 seconds. The denied-reader result is evidence of ACL denial with no bytes returned; it does not itself demonstrate that an interactive prompt was displayed. Apple describes the interaction setting as controlling whether keychain calls may show UI, and trusted-application data as opaque identity data that can include a cryptographic hash ([interaction API](https://developer.apple.com/documentation/security/keychains?language=objc), [trusted-application identity](https://developer.apple.com/documentation/security/sectrustedapplicationcopydata%28_%3A_%3A%29?language=objc)).

## Namespace and routing

Use an internal service name such as `serenity.native.v1`; leave public `Service = "serenity"` and `profileAccountKey` untouched. Keep the existing daemon and profile account names as the logical identifiers inside the new namespace. Native values should be the same API-level token strings currently returned by `DaemonToken` and `ProfileDaemonToken`; do not expose go-keyring's storage encoding to callers or rewrite old values.

Route lookups deterministically:

1. Look in the restricted native namespace first. If found, it is authoritative. If access is denied or another Security error occurs, return that error; do not fall back to a legacy value and mask it.
2. Only a typed native `ErrNotFound` permits looking up the old `(serenity, account)` item through the existing backend. If found, return it unchanged and leave its ACL/backend untouched. If the old backend reports an error, preserve it; do not treat it as absence.
3. `Ensure*` may create a native item only after both native and legacy lookups report genuine not-found. If the legacy lookup is denied or indeterminate, return that failure instead of minting a second token.
4. Use native `SecItemAdd`, never an upsert, for creation. On `errSecDuplicateItem`, re-read the native item and return its existing value for Ensure; do not overwrite. For a native credential, rotation updates only the value so its existing ACL stays intact. For a legacy credential, retain the existing legacy rotation behavior until a reviewed migration policy changes it.

A separate namespace makes native creation unable to overwrite the legacy record and makes routing explicit. It does not guarantee one physical record across old and new executable versions: an old process can create a legacy record after a new process's legacy check and native Add because the two services have no cross-backend transaction. In that race, native-first lookup is the deterministic winner; leave the older record untouched and report the possibility of an orphaned duplicate. Do not automatically delete or migrate it. Preventing even that cross-version race would require an inter-version lock or a same-attribute single namespace, both of which need separate design and compatibility review. Within one process, a package-level mutex serializes lookup/create/rotation transitions.

The unavoidable compatibility exception is explicit: an existing CLI-format item remains under its existing access policy until a reviewed migration. It has not been shown to satisfy T24.28's same-user prompt goal. `Ensure` must not silently duplicate it, and a denied legacy lookup must never be interpreted as not-found. If the legacy backend itself prompts or denies, this design preserves its existing result rather than claiming silent access. Rotation of such an item remains on the legacy backend, so the rotation retains that policy too. That exception is limited to grandfathered entries and should be documented as such.

## Interaction state and concurrent callers

`SecKeychainSetUserInteractionAllowed` changes interaction behavior for keychain calls in the process; a per-query `kSecUseAuthenticationUIFail` flag alone is not the proven guard for the tested file-keychain path. Wrap each native operation in one package-wide `sync.Mutex` guard:

- acquire the lock;
- read the prior interaction state and fail before the item operation if that read fails;
- set interaction to false and verify it;
- perform only the targeted native operation;
- restore the exact prior state in a defer before unlocking, and surface restore failure alongside any operation error.

All public `internal/secrets` calls must go through this guard; Ensure's lookup-plus-add decision must hold the lock across the whole transition. Keep random token generation outside the guard. Do not leave interaction globally disabled after a call. The checked-in Go tree has no other direct Security.framework caller; connector keyring calls use the separate go-keyring backend. If another in-process Security.framework consumer is added later, it must share the same interaction coordinator or this scoping claim must be revisited. The mutex cannot coordinate arbitrary future callers that bypass it.

## Executable identity and upgrades

At creation, resolve `os.Executable()` through symlinks and use that canonical path for `SecTrustedApplicationCreateFromPath`. The scratch probe's exact-identity check used `SecTrustedApplicationCopyData`; comparing opaque refs with `CFEqual` was insufficient in the first attempt. The acceptance check should compare copied trusted-application data against a reference created from that canonical executable and ensure the distinct reader is absent.

Do not assume that the path alone makes an upgrade equivalent. Apple says trusted-app data identifies the application and may include a cryptographic hash. The disposable probe proved a creator and a different signed reader, not that a later Serenity release retains access. Before claiming upgrade continuity, test two actual release-style signed Serenity binaries at the same canonical install path: create under release A, then read under release B with the intended stable signing identity; also verify a different designated requirement remains denied. If B is denied, the release needs an explicit reauthorization/recovery design before rollout. Do not silently recreate, reset, or replace a token when an upgrade identity check fails.

## Error contract

Preserve `ErrNotFound` for only a definite absence (`errSecItemNotFound` for native reads and the old backend's documented not-found sentinel). Add a typed access-denied/interaction error for `errSecAuthFailed` and `errSecInteractionNotAllowed`, retaining the OSStatus in the wrapped error. `Ensure*` already creates only when `errors.Is(err, ErrNotFound)`; keep that rule. Duplicate, malformed query, keychain unavailable, UI-state failure, and ACL denial must all remain distinct errors and must never trigger minting or fallback. A native denial is not evidence of not-found and is not permission to call a weaker backend.

The public rotation methods currently generate a new token and store it even when no read is performed first. Preserve that caller-visible contract: rotate an existing native item with a value-only native update; for an existing legacy item keep the legacy write path until migration is explicitly reviewed; for a truly absent account create a restricted native item. If storage fails, return the error and do not return a token as successfully rotated. Do not use `SecItemUpdate` to change the ACL on existing legacy items; the two setter stalls make that path unsuitable.

## Frozen T24.28 acceptance and proof gap

The frozen acceptance requires: (a) the ACL lists the Serenity executable path, (b) an off-Darwin test skips with a reason, (c) threat-model language reflects where provider API keys live, (d) a genuine red run against unchanged behavior and a recorded predicted failure before the fix, then a green run, and (e) `go test -race -count=1 ./internal/secrets`, `gofmt -l ./internal ./cmd ./pkg`, `go vet ./...`, and changed-package golangci-lint under the required build lease.

The scratch proof verified a canonical creator identity and different-reader denial for its probe executable. It did not demonstrate a visible prompt, and it did not test an actual release-installed Serenity binary. A no-UI reader result of `errSecAuthFailed` must not be described as a shown prompt. Keep a Darwin fixture-only automated test for creator access and a separate code identity's denied/no-data result; add a logged-in GUI acceptance step or an appropriately isolated UI test to verify that an interaction-allowed reader actually receives a prompt. Never use the user's default keychain for that proof.

There is also a genuine-red test-design constraint: the unchanged go-keyring implementation launches `security` against the user's normal keychain search context. Running it to create or query a test service/account would still query the default keychain, even if the account name were unique. To preserve the no-default-item rule, first add a behavior-neutral injectable backend seam, or a fixture-only isolated adapter, then write the failing API-routing/ACL assertion through that seam before changing native behavior. Record the red output from an owned temporary keychain. Do not manufacture a baseline red by probing the user's default keychain. If the review interprets “unchanged code” as forbidding that behavior-neutral test seam, flag the conflict rather than violating the fixture boundary.

## Minimal file and test plan after design review

- `internal/secrets/secrets.go`: preserve exported names and logical accounts; add an injectable internal backend/router, deterministic precedence, typed errors, Ensure no-overwrite rules, and data-only native rotation. Keep `MockForTesting` isolated from real OS access.
- `internal/secrets/keychain_darwin.go`: native namespaced Add/Get/update operations, canonical executable ACL creation, exact OSStatus mapping, explicit search/target selectors, and scoped interaction-state guard.
- `internal/secrets/keychain_other.go`: preserve current non-Darwin go-keyring behavior behind the same internal interface.
- `internal/secrets/secrets_test.go`: fake-backend tests for native-first reads, legacy preservation, no duplicate creation when legacy exists, no fallback/mint on access denied, duplicate Add recovery, stable profile account mapping, rotations on each backend, and concurrent Ensure serialization.
- `internal/secrets/keychain_darwin_test.go` plus a tiny separately built test reader under `internal/secrets/testdata/`: unique fixture keychain only; assert exact canonical creator ACL identity, creator read/update, different-identity no-UI denial with zero returned bytes, interaction state restoration, and exact fixture cleanup. Skip off Darwin with a stated reason. Add a separately gated GUI prompt check; it must not run in headless CI.
- `docs/threat-model.md`: keep provider API keys explicitly described as process-environment values. Distinguish new native restricted daemon/profile items from grandfathered go-keyring items and connector credentials; do not claim all macOS keychain items already name only the Serenity binary.

After the red/green test sequence, run the frozen verification list from `docs/tasks/deep-review-001/T24.28.md` under its required lease. No production patch is authorized by this design artifact.
