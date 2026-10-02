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
