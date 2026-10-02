# Independent review: T24.28 Keychain ACL source design r2

Date: 2026-10-02

## Disposition

The design is conservative and materially improves on a silent fallback or second-namespace migration: exact service/account tuple, native-only atomic add on definite absence, data-only update, duplicate reclassification, and typed fail-closed behavior preserve existing tokens and profile API shape. The design is **not yet implementation-ready for automatic legacy classification**. It correctly identifies that blocker but leaves the legacy signature/marker-preservation proof open. It also does not provide a bounded-call guarantee; that needs to be stated as an explicit limitation or addressed before promising one.

This is a design review only. I made no repository/source/keychain/provider changes and ran no tests. No full T24.28 acceptance or rollout qualification is claimed.

## Identity and artifacts

Reviewed design: `/Volumes/BuildOffload/serenity-t24-28-keychain-acl-source-design-r2-20261002.md`, SHA-256 `b2d105595411e371d7a58f103691af6a59c2c4b983e44706f1c7eb82c0d843b8`.

Reviewed readiness evidence: `/Volumes/BuildOffload/serenity-t24-28-native-readiness-and-legacy-blocker-20261002.md`, SHA-256 `36feb18525dcb40d0f88cb82d5305327001ddaa694be34024f048a364e7ba849`.

I also read the frozen T24.28 acceptance at `docs/tasks/deep-review-001/T24.28.md` on checkout HEAD `c3d491b03980310d7d59688d092b8119640e5250`. The requested artifacts have no implementation SHA to review.

## Review findings

- **Legacy versus native classification remains the key blocker.** The exact-tuple classifier and no-fallback behavior are sound. A positively identified legacy record can continue through the old backend; a marked native record stays native even when the current executable no longer matches; ambiguous/malformed metadata must error rather than fall through or mint a replacement. But the readiness note establishes that the old CLI item’s metadata/ACL can be inspected while a no-UI read is denied, and that the legacy ACL setter stalled in two bounded runs. It does not establish a stable, durable signature that distinguishes old CLI items from marker-lost/modified native items. Before wiring automatic legacy routing, prove (in an owned fixture) the legacy signature and whether old `security add -U` preserves or removes the native marker. If they cannot be distinguished robustly, retain “unknown” as a hard stop and require explicit migration/re-entry; do not guess legacy.

- **Old-token/API/Linux behavior is specified well, but compatibility is not yet demonstrated.** Keeping one tuple and the pinned go-keyring v0.2.8 `go-keyring-base64:` envelope is a reasonable compatibility choice. The plan correctly avoids claiming that old releases can read, prompt for, or rotate new native items until two signed releases and an old reader are run against an isolated fixture. Test old-reader decoding of the exact stored envelope and interactive prompt separately. Treat the documented same-user legacy ACL as grandfathered; the design cannot claim the ACL objective for those existing items.

- **No bounded-native-call behavior is designed.** The one package mutex serializes package-owned native operations and the saved/restored interaction setting prevents overlap among those operations, assuming every in-process Security caller shares that gate. It does not bound a waiting goroutine or a synchronous Security.framework call. Apple documents `SecItemAdd` as blocking its calling thread; the readiness evidence also records a legacy ACL setter that did not return within 15 seconds. A Go context cannot cancel a blocked native call, and timing out a wrapper goroutine would leave the call—and potentially the interaction setting—running. If bounded latency is a requirement, specify a safe isolation/containment design before asserting it. Otherwise document that calls may block and avoid representing the mutex as a timeout. [Apple SecItemAdd](https://developer.apple.com/documentation/security/secitemadd%28_%3A_%3A%29?changes=_4)

- **Upgrade identity handling is appropriately cautious.** Apple describes trusted-application identity bytes as opaque and potentially including a cryptographic hash; the host probe establishes only a creator-versus-different-reader denial. It does not establish same-path release continuity. The requested signed Release A/B fixture and different-designated-requirement denial are necessary before claiming upgrade compatibility. A stale native ACL should remain an explicit recovery error with no automatic reset or replacement. [Apple trusted-application identity](https://developer.apple.com/documentation/security/sectrustedapplicationcopydata%28_%3A_%3A%29)

- **The creation/race and API boundaries are coherent.** Use of the original service/account names avoids a namespace split. Exact not-found is the only creation condition; duplicate Add is reclassified; native denial never invokes legacy fallback; rotation changes value only and preserves ACL. Ensure/Rotate serialization and profile-name/API preservation are specified. Confirm tests cover identical tuple races, duplicate with winner both native and legacy, and profile token routing without widening access.

- **Fixture and genuine-RED plan is safe in intent but remains a future proof obligation.** The readiness evidence is explicitly limited to disposable file-keychains and says default/pre-existing items were not queried or changed. Keep that invariant for source tests. The frozen task requires a real failing test against unchanged behavior, followed by green after the fix. The proposed behavior-neutral seam is acceptable only if the RED exercises the unchanged production behavior against a unique temporary keychain and records the actual assertion failure; a mocked expectation or a test that bypasses the production router would not establish that. Pin fixture search/add operations to the owned keychain and verify cleanup by exact path/identity.

## Apple API note

Apple’s current documentation marks the legacy macOS Keychain ACL/trusted-application APIs used by this proposal as deprecated. The target is a macOS-only daemon, so this is not by itself a reason to reject the design; document the supported OS/toolchain scope and preserve tests around the deprecated API boundary. Apple also documents ACLs as unavailable for iOS and macOS apps using iCloud Keychain, which is outside this daemon design. [Apple Access Control Lists](https://developer.apple.com/documentation/security/access-control-lists?changes=lat_1_1%2Clat_1_1)

## Evidence limits

The readiness report’s new-item result is strong fixture evidence for ACL creation, exact fixture lookup, no-UI creator read/data-only update, and denial with no bytes to a differently signed reader. It does not prove visible prompts, old-release decode/rotation, cross-release continuity, robust legacy classification, behavior on every macOS/keychain configuration, default-keychain safety in future test code, or bounded native-call latency. Frozen gofmt/race/vet/lint and genuine-red acceptance remain unrun for any implementation.
