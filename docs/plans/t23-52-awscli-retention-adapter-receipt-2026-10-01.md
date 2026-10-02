# T23.52 AWS CLI retention adapter receipt

Candidate implementation: `hosted/retention-cli-worker-20261001` at the
revision recorded by this receipt. The adapter is an unwired source-only
implementation of the existing retention `Storage` seam. It invokes an
explicit trusted AWS CLI executable with explicit region, bucket owner, and
temporary credentials; it does not select ambient profiles or endpoints.
Calls are manually paginated and bounded by explicit timeout and output limits.
Delete requests use bounded JSON on stdin, set `Quiet` false, and require exact
per-version confirmations. Only an exact `ListParts` response with exit 254
and `Code=NoSuchUpload` maps to verified upload absence. Process groups are
bounded and cleaned up on timeout or retained descendant pipes.

The fake-process suite includes a success case where the child writes valid
JSON larger than the pipe chunk and exits immediately; buffered output is still
drained and parsed. A separate retained-descendant case verifies deadline
failure and owned process-group cleanup.

## Validation

- `python3 -m unittest discover -s deploy/hosted/tests -p 'test_backup_retention_awscli.py' -v`: 14 passed.
- `python3 -m unittest discover -s deploy/hosted/tests -p 'test_backup_completion.py' -v`: 20 passed.
- `python3 -m unittest discover -s tests -p 'test_backup_retention.py' -v`: 22 passed.
- `ruff check deploy/hosted/backup_retention_awscli.py deploy/hosted/tests/test_backup_retention_awscli.py`: passed.
- Genuine mutation controls failed as expected for broad NoSuchUpload mapping, omitted delete confirmation checking, disabled output cap, and extended timeout. Earlier controls also failed as expected for disabled scope validation, inherited environment, and duplicate-key parsing. Each mutation was restored and the source checksum verified.
- Offline CLI v2.35.14 skeleton checks for `delete-objects --delete file:///dev/stdin` completed without service requests. This checks local CLI argument compatibility only.

Test environment and offline CLI evidence are under
`/Volumes/BuildOffload/serenity-retention-cli-validation-20261001/`; test
bytecode cache is under `/Volumes/BuildOffload/python-cache/retention-cli-20261001`.
No AWS request, credentials, deployment, lifecycle, purge, or production
activation was performed. These results do not qualify an installed production
stack or authorize retention execution.

Source paths are limited to the new adapter and its new fake-process tests.

## Independent-review correction

The post-review correction adds strict Unicode `Cc` rejection (including C1
controls), matches the existing retention parser's strict timestamp behavior
for leap seconds, sanitizes selector-construction failures while cleaning up
the owned child, and rejects setuid/setgid CLI executables. A controlled
success case proves that a live same-PGID descendant is terminated while the
leader remains unreaped. The adapter observes exact leader exit with
`waitid(WNOWAIT)`, signals before `Popen.wait()` reaps it, and never signals
after reaping. `ECHILD` and unsupported observation fail closed without group
signals. Darwin's zombie-only group can return `EPERM`; the development path
accepts it only after exact WNOWAIT leader-exit observation and under the
trusted nonprivileged executable model. This means there are no eligible
signalable members; it does not prove that no process exists. Credential-
transitioning or MAC-restricted descendants are outside the supported model.

The SIGCHLD default-disposition check only rejects a known handler. It does not
prove the absence of a competing `waitpid`/`waitid` reaper or ensure the signal
disposition stays unchanged. Any eventual caller must independently prove
exclusive child-reaper ownership across spawn, observation, signaling, and
reap. This adapter remains unwired and is not authorized for production use.

The prior source failed focused RED controls for each review finding:

- C1 controls were accepted.
- Leap-second snapshot keys were accepted by the adapter.
- Selector setup `OSError` escaped as a non-sanitized error and could leave
  the child running.
- The group-signal test showed signaling after leader reap.

The corrected fake-process suite has 22 tests and passes with
`-W error::ResourceWarning`; the adjacent completion and retention suites
remain green (20 and 22 tests). Ruff passes. A Darwin-owned-process probe by the
independent reviewer is recorded at
`/Volumes/BuildOffload/tmp/darwin-wnowait-pgrp-probe.log` (SHA-256
`d2e1cbb8055219b5e61e008a6ab0e412ae9434a2207bdfe55a0f79cfb45beee6`). This is
local process behavior evidence, not qualification of AWS CLI or deployment
process ownership.

The corrected source and tests are pinned at commit
`c4fefdbd8d2d9722f11c400624840d60343df825`. Final SHA-256 values are
`1122505db2aaca19b30a736cea4efcc623321ba215954b536031ba49cfee1dd4` for the
adapter and
`0a13a7d9df520f1720ac86af7704feb2cd8dfcead5dcfab0668a5c27b85793da` for its
tests. The exact-pin focused checks passed: 22 adapter tests with resource
warnings treated as errors, 20 completion tests, 22 retention tests, Ruff,
and `git diff --check`. The offline AWS CLI skeleton check is limited to the
installed AWS CLI 2.35.14; no service request or real AWS qualification was
performed.

## Pre-exit EPERM correction

Independent review found that the Darwin `EPERM` handler could wait for a
leader that was still running to exit, then reinterpret the original failed
signal as the permitted zombie-only case. The correction now treats `EPERM`
as the zombie-only condition only when the caller already observed that exact
leader's exit with `WNOWAIT` before attempting the signal. This applies to
both `SIGTERM` and `SIGKILL`; no post-failure observation changes the result.
New mocked no-host-signal controls cover both signal attempts and failed on the
prior source at
`/Volumes/BuildOffload/serenity-retention-cli-validation-20261001/red-eperm-preexit.log`
(SHA-256 `83ff5955aa367940b2da4fd1859e5e263241e3d23448f870abf4dc2c8440f3d5`).

Cleanup now makes a separate bounded attempt to reap the exact child after a
signal error only after another `WNOWAIT` check confirms the process still
owns it. `ECHILD` or changed child identity skips `Popen.wait`. Reaping the
leader does not change the signal failure into success and is not evidence
that descendants were stopped. The cleanup tests assert both behaviors.

The corrected focused adapter suite passes 26 tests with
`-W error::ResourceWarning` and emitted no resource warnings. Completion and
retention suites pass (20 and 22 tests); Ruff and `git diff --check` pass.
This correction remains a fake-process local qualification only.

Final pre-exit `EPERM` correction is committed at
`76014ba215b63b47667cbd702a80501281eb78d4` (superseding the earlier source
pin above). Its adapter SHA-256 is
`619908157d2d42d2b0f7b4a17f6b8a07ee2708d99d2bd126cab77c03bd185323`; its test
SHA-256 is
`fda420d808335ef21959fc47eb2d38435611a9a4dc565bf4392830103f9e76d4`.
