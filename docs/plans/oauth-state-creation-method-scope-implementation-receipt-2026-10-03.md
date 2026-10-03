# OAuth shared state-budget method scope implementation receipt

**Frozen contract:** `oauth-state-method-scope-v1`, source freeze `9d5ec33c6472f3468886a11c2b40d470f006ba8b`; proposal contract commit `6bfe7fcaa93ef4907b27693cd2c58d743bc84376`, independently CLEAR for scope only. Implementation base is exact freeze commit.

## Change

The shared state-creation limiter now charges exactly `POST /oauth/register` and `GET|POST /oauth/authorize`. Other methods continue to the existing downstream handlers without consuming that shared counter. The outer per-prefix limiter and register-specific limiter retain their existing nesting and behavior. In particular, valid GET authorization still creates/persists consent before the callback; POST authorization consumes the shared budget and still returns the pinned dependency's 405; HEAD authorization remains uncharged and keeps its existing 405. Defaults, dependency version, service/gateway behavior, 5000/minute capacity policy, consent policy and hosted/live acceptance are unchanged.

New regressions prove excluded methods on both paths do not exhaust the shared budget and have byte-for-byte status/body plus `Allow` compatibility with direct downstream handling. A valid GET creates a stored consent and is metered; registration POST is metered; authorization POST preserves the existing 405 while being metered. The package's existing limit/refresh/consent tests remain in the full package run.

## Validation evidence

All Go stages used the durable `run_worker_stage.py` runner with a fresh load check, exact WON lease, durable stdout/stderr/JSON result, source fingerprint, and same-process finally release. Runtime tests used `-exec 'env TMPDIR=<owned-APFS-runtime-fixture>/tmp SERENITY_RECOVERY_TEST_TMPDIR=<owned-APFS-runtime-fixture>/tmp'`.

- Full OAuth package race: `go test -race -count=1 ... ./internal/hosted/oauth`; exit 0. Evidence `package-race-01`.
- Focused race of the two new regressions: exit 0. Evidence `focused-race-01`.
- `go vet ./internal/hosted/oauth`: exit 0. Evidence `vet-01`.
- `golangci-lint run ./internal/hosted/oauth`: exit 0, `0 issues`. Evidence `lint-01`.
- Genuine compiled behavioral mutant, shared guard charges every method: focused test exit 1 at assertion `excluded method PUT /oauth/register consumed shared state budget`. Evidence `mutant-all-methods-02`.
- Genuine compiled behavioral mutant, HEAD added to authorize charged set: focused test exit 1 at assertion `POST /oauth/register after excluded methods: 429`. This shows `HEAD /oauth/authorize` incorrectly consumed the shared slot. Evidence `mutant-head-01`.
- Both mutant stages report Go test assertion failures after successful compilation; they are behavioral RED, not compile failures. Both restored-source checks above passed.
- An earlier attempted mutant stage `mutant-all-methods-01` mistakenly ran with the primary main checkout as its shell working directory. Its replacement needle was absent there, so the write call left `internal/hosted/oauth/ratelimit.go` byte-identical to its committed main blob; the primary checkout at main `189cdabe89ff296cfab42c72d7e3b8af030f332c` had no tracked diff. For `internal/hosted/oauth/ratelimit.go`, `git rev-parse HEAD:path` and `git hash-object path` both returned blob `5524f0b2b636a8a3c7fd8300ec1bb87eb5bf78a5`; its SHA-256 was `ee6ab6a04dc13f24a10489d24b517613f4e93569d564832db2a2586c0fcab852`. That stage's runner operated on the assigned clone's unchanged bytes, so it is retained as a non-mutant run and is not counted as RED evidence. No other path was targeted.
- `go.mod` remains at `github.com/ajent-social/go v0.0.0-20260924042100-b90bbb417d9d`.

Evidence bundle: `serenity-oauth-state-method-scope-implementation-evidence-20261003` (external SSD), with per-stage JSON and stdout/stderr records keyed by the evidence labels above.

## Scope and limits

Only `internal/hosted/oauth/hosted.go`, `internal/hosted/oauth/ratelimit.go`, `internal/hosted/oauth/ratelimit_test.go`, and this receipt are owned. Local package validation does not establish the full-module gates, production approval, live 17-prefix/refresh acceptance, deployed service behavior, cross-process aggregation, 5000/minute capacity policy, or full SEC-H02/T24.39 acceptance. Root owns full-module verification, independent exact-head review, merge and landed verification.
