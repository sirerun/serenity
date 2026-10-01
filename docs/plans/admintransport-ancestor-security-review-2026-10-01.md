# Admin transport ancestor-containment review addendum

Corrected-source pin: `0ac66c44cb7cd05c311f70338be3b5416fb345c7` (`fix(hosted): reject unsafe admin socket ancestors and mounts`). This addendum preserves the earlier `2264c32` review receipt as historical evidence; its bounded clearance did not cover the newly identified writable-ancestor replacement flaw and must not be read as clearing that flaw.

## Correction review

The original condition was real: a group/world-writable non-sticky ancestor could permit a foreign user to rename the otherwise owner-private socket subtree after the path snapshot check. The correction walks every path component from the immediate parent to `/`, using `Lstat`, requiring a real directory owned by the creating effective UID or root, and rejecting group/other write permission unless the sticky bit protects that entry. `ownershipEnforced` is consulted for each component. Darwin checks `MNT_IGNORE_OWNERSHIP` with `statfs`; Linux relies on VFS ownership metadata; unsupported platforms fail closed. The higher-ancestor regression in `listener_test.go` asserts rejection before socket creation.

I independently isolated and exercised the Darwin mount check. A temporary reviewer-owned package test directly checked the known ownership-disabled BuildOffload volume: it passed on the corrected source. In an isolated review clone I bypassed only the `MNT_IGNORE_OWNERSHIP` predicate; the test failed because the volume unexpectedly passed. I restored the exact source and removed the temporary test. This demonstrates that the mount guard test is not vacuously passing only because BuildOffload's ancestor mode is writable. The corrected full package race suite then passed with `TMPDIR` on the ownership-enabled private APFS fixture (`1.291s`). The coordinator separately records the higher-ancestor regression red on `e0e705d` and corrected package race/vet/lint/Linux ARM64 compile green. Linux evidence is compile-only; no Linux runtime credential test is claimed.

No additional blocker was found in the ancestor or mount-ownership correction. It closes that specific containment gap. The original review's limits remain: peer credentials identify the service effective UID, not a human operator; this transport source does not itself mount the handler; and the production operator authorization/recovery path is outside this package. This is not T23.44 or deployment acceptance.

No production source was changed. The reviewer-owned temporary test and the local source mutation were removed/restored; only the SSD Go cache remains untracked.
