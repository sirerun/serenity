# T23.44 signed operator-case initial review hold

Reviewed source pin: `e55563bf1f62443bf276cb933dfd763d5d935d2d`, isolated clone `/Volumes/BuildOffload/worktrees/serenity-signed-case-independent-review-20261001`. Contract references: `849ff76` and the current-key admission clarification `81a70ca` (also imported at `22c15ee`). This is a static review only; no Go tests, mutations, or provider calls were run.

The source has three admission blockers at this pin:

1. `operatorreview.New` performs filesystem I/O through `privatefs.ValidateDirectory(context.Background(), ...)` and offers no caller context (`internal/hosted/operatorreview/admission.go:73-86`). This defeats cancellation and violates the context-first filesystem contract. Root requested a context-taking constructor and cancellation regression.
2. The signed `key_id` is only checked as generic ASCII opaque text before `TrustedKeySource.Lookup`; the exact `ed25519-sha256:<64 lowercase hex>` digest check occurs after the lookup (`admission.go:115-140`). A malformed case can therefore invoke the trust source before its identifier is structurally valid. Root requested exact-format rejection before lookup and a no-lookup regression.
3. The age and lifetime checks use `time.Sub` against user-supplied positive durations (`admission.go:142-146`). `time.Time.Sub` saturates at the largest representable `time.Duration`; an extreme positive policy can therefore accept a case whose actual age/lifetime exceeds that bound. The code also lacks explicit rejection of zero `Now`, approval, or expiry times. Root requested overflow-safe instant comparisons and zero-time cases with runtime RED controls.

One qualification-test concern: `TestCanceledAndUntrustedDirectoriesFailClosed` treats any error from `ownershipEnforced("/Volumes/BuildOffload")` as proof ownership is disabled (`privatefs/privatefs_test.go:77-82`). If that mount is absent, the test passes on the resulting lookup error. The test should verify the intended mount exists and that the specific ownership-disabled flag is present, or skip with an explicit reason. The production Darwin check uses `MNT_IGNORE_OWNERSHIP`; this finding concerns test evidence.

The implementation otherwise appears consistent with the frozen contract in the reviewed paths: peer credentials precede case/key reads; the case hash and canonical signed payload are bound; parsing rejects duplicate, unknown, aliased and trailing JSON; each admission performs a fresh injected key lookup; and the deterministic test reflects the stated snapshot-before-revocation policy. This does not establish a production trust source, human review, external immutable issuance, or deployment readiness. A valid signature proves attribution to the supplied key, not that a human inspected the case. Those authorities remain absent and are not supplied by this package.

No merge or runtime qualification recommendation until the three admission blockers are corrected and reviewed. Root owns integration and all production activation gates.

## Corrected-source independent qualification

Corrected candidate reviewed: `4cec36e6e517be0a7449f3cea8e19ba6f374d965`. The three admission blockers are corrected: `New(ctx, ...)` checks and passes the caller context to filesystem validation; `validKeyID` runs before the trust-source lookup; age/lifetime bounds use instant comparisons and zero clock/approval/expiry values are denied. The corrected Darwin mount test reads `SERENITY_PRIVATEFS_UNOWNED_TEST_PATH`, confirms `Statfs` succeeds and `MNT_IGNORE_OWNERSHIP` is actually set, then requires the guard to refuse the mount.

Review-only runtime mutations produced RED for context cancellation, malformed key ID reaching Lookup, MaxInt64 `time.Sub` saturation, zero approval/expiry timestamps, stale-key reuse after revocation, ignored revocation, signature bypass, duplicate/case-alias JSON acceptance, noncanonical timestamp offsets, writable ancestor acceptance, and removing the ownership-disabled mount guard. The zero approval/expiry test was a temporary review-only harness, removed after the check; it is not part of candidate `4cec36e`. The committed suite covers zero `Now`, but has no permanent zero approval/expiry regression yet. This is a test-coverage follow-up, not a remaining bypass in the reviewed implementation.

Restored-source verification passed separately on the three owned packages with `TMPDIR=/Volumes/SerenityPrivateFixture20261001/tmp`, external Go caches, and the load guard at each launch: `go test -race ./internal/hosted/operatorreview -count=1` (`ok`, 1.349s, including the temporary zero-time harness), `go test -race ./internal/hosted/privatefs -count=1` (`ok`, 1.329s), and `go test -race ./internal/hosted/admintransport -count=1` (`ok`, 1.296s). The disabled-mount test ran explicitly, not skipped: `go test -v ./internal/hosted/privatefs -run '^TestOwnershipDisabledMountRejected$' -count=1` reported PASS against `/Volumes/BuildOffload`; the mount guard removal mutation made it fail. The privatefs race run includes the writable-ancestor check, and disabling that check made `TestWritableLeafAncestorIsRejected` fail. Every deliberate source mutation was restored, the temporary harness was removed, and the review clone returned to exact source `4cec36e` before this receipt update. No multi-package test, provider call, or production activation was performed.

Preserved validation logs are in `/Volumes/BuildOffload/tmp/`:

| Evidence | SHA-256 |
| --- | --- |
| `signedcase-context-red.log` | `689c9a30816a24e6ee673a9cfbab2afe08de9b5ad19193aa244b71403967036f` |
| `signedcase-keyid-red.log` | `e70dc42e2ce22225dbf9c2f2cb35200bfe0bbb708ab732b8e88d1b27d01ac267` |
| `signedcase-duration-red.log` | `595d99d2b7d2f80ec09a5f49cb6623ecec5b2357a9393ff15543082497f46f02` |
| `signedcase-zero-red.log` | `dac65154de3e8f5d272fa1d38dcd81d4ccfd689820771ab0438d8061ade950ff` |
| `signedcase-strict-red.log` | `322fc4930e307862d1f93154e0dfdc46cd0d71c0a09127861b79b924db96d19e` |
| `signedcase-signature-red.log` | `b167c6c627659a50afc3ca56c97fc8df229c965dd17c943a52f954cbf0975996` |
| `signedcase-revocation-red.log` | `809e51fd412f4546fdb1b30195e71e3a029ce482911ebdf1400ac480de2740ab` |
| `signedcase-stale-key-red.log` | `14b52fbfeacfe07a4759c4d61a945945526ff063c562d6b323abc9ac436b52aa` |
| `signedcase-canonical-time-red.log` | `ce5eed6e44eb601d1da93bc5f263f6711699a6464c639f80988b62c74e7a4d3c` |
| `signedcase-ancestor-red.log` | `41d22a72511b3f69f99915df746493be96d842140ffae0a105706ef546b1ba5e` |
| `signedcase-unowned-mount-red.log` | `e25cb0f51f6abea13caff95f3dec2d986091ee76d7dbbee4212dabdd3f64ed6f` |
| `signedcase-operatorreview-final-green.log` | `5fa1580fd60b88b1f5d221a8262cb6d6f1919033fd8c3aede72f6b068a5dfcbe` |
| `signedcase-privatefs-green.log` | `7b24aaccbd2e42e6b56d0104c84de56235a55070aecf868fe821da1147e2b8ba` |
| `signedcase-privatefs-mount-green.log` | `eded5dfe3cf901fac6a45b091d27f115b429a6c9540868b599dee1fe481b2f31` |
| `signedcase-admintransport-green.log` | `b6ccbecacd580998c175873d4a8a9bfbf4d9e9b64d7007532289b10e1a3b9dd9` |
| `signedcase-operatorreview-green.log` | `28e4f01751e2163d2f9f19389ef528e918f2409acab4242bf56cae0914715d16` |

The corrected code has no additional source blocker from this independent review. The original e55563 hold above is retained as review history. This clearance only addresses this local verifier slice: a valid signature attributes the decision to the injected key and does not establish that a human inspected the case. No production key source, issuance authority, human-review evidence, or activation approval was provided.

## Permanent zero-time regression follow-up

Test-only commit `fb3a0b9ead420fe0723fa941db1f265008150b19` adds permanent subtests for a year-0001 zero approval time and a year-0001 zero expiry time. The test deliberately sets key validity across years 0000–0002 and uses year-boundary `now` values so the values pass the ordinary ordering and configured age/lifetime bounds; the explicit zero-time rejection is the condition under test. These are the same two concrete timestamp cases used by the earlier temporary mutation probe recorded above. With the zero-time guard removed, that probe admitted both cases (RED log SHA-256 `dac65154de3e8f5d272fa1d38dcd81d4ccfd689820771ab0438d8061ade950ff`); the corrected-source probe denied both (GREEN log SHA-256 `64d9e425e29302b711bd350303ea2988953b598038bd9c99972c9a13f9f56d88`). The permanent regression now keeps this coverage in the package suite.

On the permanent-test commit, `go test -race ./internal/hosted/operatorreview -count=1` passed (`ok`, 1.774s) and `golangci-lint run ./internal/hosted/operatorreview` reported `0 issues`. Each launch followed an uptime load check below 10, used external Go caches and the owners-enabled fixture TMPDIR, and ran only the changed package. No production code changed; no multi-package build, provider call, or production activation was performed.
