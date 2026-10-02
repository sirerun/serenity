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
