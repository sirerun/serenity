# Serenity qualification source fixtures

`evidence.py` encodes and verifies complete stdout/stderr artifacts for the Go
and Python components of a Serenity qualification run. QEV1 binds each component
to an input-bundle digest, an ordered sequence of base64 chunks, matching
BEGIN/END metadata, a byte count, a SHA256 digest and its actual exit code.

The reader refuses incomplete, malformed, duplicate, reordered or foreign-run
frames. It also refuses responses over16MiB, components over4MiB, a partial final
line, or100000 or more newline-delimited records. Legacy completion markers do
not establish completeness. Verified artifacts can still report a nonzero exit;
callers must check their own terminal state and every required component result.

`test_evidence.py` contains positive and negative protocol fixtures, including
the truncated-tail case where legacy success markers survive. To run them:

```sh
python3 -m unittest discover -s scripts/qualification -p 'test_*.py' -v
```

The codec has no network, subprocess, environment, credential or job-control
integration. Caller-owned authorization, capture, immutable snapshot storage and
runtime receipts remain separate responsibilities. Generic engineering execution
and orchestration stay with the owning Foundry/APRL interfaces; this directory
is a product-local source fixture, not another runner or lifecycle scheduler.

`Dockerfile.protected-home` is an UNBUILT test-environment recipe based on a pinned
Go image. It adds a UID1000-owned mode0700 HOME without relaxing credential-path
checks. Building or using a derived image requires an approved builder route,
bounded admission, actual immutable image/platform/toolchain/UID/capability and
full-ancestor permission evidence. Source presence grants none of these effects.

This checkpoint does not install or activate these fixtures in a hosted release
or qualification pipeline. The extracted source requires its own review and
checks; historical checks on its antecedent private source do not certify this
new revision. No activation, signing, rollback, host/session configuration,
authorization records, operational packets or credential data are included.
