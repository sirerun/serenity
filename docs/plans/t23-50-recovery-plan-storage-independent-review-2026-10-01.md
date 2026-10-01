# T23.50 recovery-plan storage independent security review

Reviewed exact source `cad27ddf00453ed25e00ff8d06fb3ae6ae1dfb34` in an isolated clone at `/Volumes/BuildOffload/worktrees/serenity-t23-50-recovery-plan-independent-review-20261001`. Source commits: `685f9c4`, `93cae51`, `8559211`; tests: `099516a`, `0552fe4`, `363e2f0`.

## Verdict: blocked on ancestor path replacement

`privateDirectoryPath` resolves and Lstats every canonical path component, checks that the plan directory is mode 0700/current-user owned, and rejects group/world write on the immediate parent unless sticky. It does not apply the write/sticky check to higher ancestors. If a higher ancestor is writable by an attacker and non-sticky, the attacker can rename/replace the checked immediate-parent directory after validation. For example, the accepted chain can be `/tmp/attacker-writable-0777/secure-parent-0700/private-plan-0700`: only `secure-parent` is the checked immediate parent. Replacing that ancestor's `secure-parent` entry with a symlink between validation and use redirects the returned path string. Subsequent `os.CreateTemp` and `os.Link` follow the intermediate symlink; `O_NOFOLLOW` is used only for the final file on load. This violates the private no-symlink storage boundary and can redirect plan publication outside the intended directory. Check write/sticky safety for every ancestor or use an opened directory handle with no-follow relative operations, and add a higher-ancestor replacement control. I sent this exact condition to the author and coordinator; the candidate is held pending correction.

The current tests reject a final directory symlink and verify a symlink target is not overwritten, but do not cover a swappable higher ancestor. `TestLoadPlanRejectsUnchangedHashMismatch` also uses an alternate expected hash whose filename does not exist, so that assertion exercises open failure rather than expected-hash mismatch. The subsequent modified-payload test is a valid tamper/hash check; a stronger wrong-hash test would copy valid bytes under an alternate hash filename.

## Other reviewed properties

- PlanHash covers canonical payload fields while excluding PlanHash itself; `LoadPlan` recomputes the digest against the supplied expected hash and requires canonical document bytes.
- The decoder explicitly rejects duplicate keys, unknown/case-aliased keys, malformed nested structure, and trailing JSON. Account scope is bounded, sorted, unique, nonempty, and rejects wildcard/all aliases; timestamp must be canonical UTC; watermark shape and fence-generation ordering are checked.
- Create uses a private temp file, sync, same-directory no-overwrite hard link, and directory sync. Load opens the final plan file with `O_NOFOLLOW`, then checks regular-file type, mode and owner.
- The frozen contracts remain unchanged. The receipt correctly states that the digest is not an authentication signature or proof of provider/snapshot truth, and no production adapter or activation is introduced.

The author's receipt records the recovery package race test passing (`3.650s`) at reported load `8.50`. I did not independently run Go checks on this held candidate; review was source/test/receipt inspection only. No provider, live service, or cloud actions were performed.

## Final re-review: `a085f431ebe9a8971bdffd78290f7f4ef4d9ee60`

The source fixes the held ancestor issue and the subsequent FIFO, disabled-ownership-mount, invalid-account-encoding, and sync-retry findings. `privateDirectoryPath` validates root and every path component for trusted ownership, writable non-sticky mode, and filesystem ownership enforcement. Darwin checks `MNT_IGNORE_OWNERSHIP`; Linux uses its platform-specific implementation; unsupported platforms fail closed. Plan-file open uses `O_NOFOLLOW|O_NONBLOCK`, followed by regular-file, owner, and mode checks. Account scope now rejects invalid UTF-8, Unicode whitespace and controls in addition to reserved aliases and separators. An identical existing plan now retries directory sync, and file/directory close errors are propagated.

I independently tested the controls on the exact source in an isolated clone. Each guard mutation was made only in this review clone and restored before final verification:

- Disabling the writable-ancestor predicate made `TestCreatePlanRejectsWritableHigherAncestor` fail because `CreatePlan` accepted the writable ancestor.
- Disabling the Darwin ownership flag check made `TestVerifyFilesystemOwnershipRejectsDisabledMount` fail. With `SERENITY_RECOVERY_UNOWNED_TEST_PATH=/Volumes/BuildOffload`, the unmutated test passed against the actual ownership-disabled volume.
- Removing `O_NONBLOCK` made `TestLoadPlanRejectsFIFOWithoutBlocking` fail after the 500 ms context deadline because opening the FIFO blocked.
- Comparing the computed digest only to the artifact's embedded hash allowed valid bytes copied under another approved hash; `TestLoadPlanRejectsUnchangedHashMismatch` failed under that mutation.
- Disabling `utf8.ValidString` allowed the invalid account bytes through and `TestCreatePlanRejectsNonUTF8AccountBeforePublication` failed. Disabling Unicode space/control rejection made `TestCreatePlanRejectsMalformedInputs` fail on its NBSP case.
- Skipping sync on the identical-existing-plan path made `TestCreatePlanRetriesDirectorySyncForExistingIdenticalPlan` fail because the injected sync failure was silently acknowledged.

After restoring the exact source, `git diff --check` was clean and `go test -race ./internal/hosted/recovery -count=1` passed in `1.493s`, with both `SERENITY_RECOVERY_TEST_TMPDIR=/Volumes/SerenityPrivateFixture20261001/tmp` and the disabled-volume test path set. The independent check is Darwin-only; I did not run a full repository gate. Close-error propagation was inspected in source, but I did not inject an OS close failure.

**Verdict: clear for this bounded local artifact-storage component.** This does not authenticate the plan hash as operator approval or prove snapshot/provider truth, journal completeness, generation adoption, or production activation. No frozen contract or production adapter changed, and no cloud/provider action was performed. The initial hold above remains as historical evidence and is resolved only for the exact corrected source pin stated here.
