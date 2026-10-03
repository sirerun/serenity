# Eval read-only Git caller source receipt — 2026-10-02

Source branch: `feature/eval-git-migration-20261002` in the isolated SSD worktree. Baseline: `4b2fd8e656e448d712b2c7d9fa2bc28b9d78c469`. Before source edits, the assigned canonical claim was verified against `git@github.com:sirerun/serenity.git`: `R-eval-git-migration 9220d83cc56c4053132cc309649a62aca8868cb8`.

Coordinator inventory amendment was committed separately first as `ef931ec` (`docs: amend eval git source assignment inventory`). A direct production-source audit confirmed four literal raw Git subprocess call sites across four files: calibration, BrainBench `gen_trend`, BrainBench `publish_trend`, and fixtureprep `verify`. The three fixtureprep Git argument sets share one subprocess closure. The original readiness note's fourth location, `verify.go:775`, was erroneous; the appended coordinator correction preserves that initial observation as historical and expands this lane to `gen_trend.go` under the same claim.

The four callers now use `gitrun.Foreign` pinned to their prior caller directory and preserve their read-only arguments and context where the caller accepts one. The ignored command helpers preserve `GITHUB_SHA` precedence where present and return `unknown` on lookup failure. Fixture observations continue to report Git errors as observations. No gitrun API/allowlist, scanner, module, generated report/trend/fixture data, provider request or load run changed.

The genuine pre-fix hostile inherited-`GIT_DIR` RED used two owned temporary repositories. Before correction, fixtureprep's observer followed the foreign repository: expected tracked source count was 1, but it reported 0 tracked sources and 2 dirty paths. The immutable captured output is in `/Volumes/BuildOffload/validation/serenity-eval-git-migration-20261002/logs/pre-fix-hostile-gitdir.log`, SHA256 `6bcf0dcd4af7c3c7cbf16da0fed78d76ba022da88115b1982b7a63e71228af16`. After correction, hostile-environment and canceled-context focused tests passed.

Validation used the external SSD Go cache, module cache and temporary directory. `uptime` was checked before each Go command; one-minute load stayed below 10. Every test/lint command covered one package or one explicit-file command package, so no shared multi-package build lease was needed.

- `go test ./evals/hosted-load/fixtureprep -count=1` passed; log SHA256 `9b579bfc2eb71beaa22c274591c263db1949bdb9bc93ef375e6f470c226ffaca`.
- `go test evals/calibration/gen_calibration.go evals/calibration/gen_calibration_test.go -count=1` passed; log SHA256 `9f3cee47baa86e9819e62d97e8c40f25f462985935d96e70c56b4012cae787da`.
- `go test evals/brainbench/gen_trend.go evals/brainbench/gen_trend_test.go -count=1` passed; log SHA256 `91d9cfa1179fa2bb4fa2c58309499e58b0ea4f9e784a84960c17ad00ccc29ded`.
- `go test evals/brainbench/publish_trend.go evals/brainbench/publish_trend_test.go -count=1` passed; log SHA256 `8fc3f7011fed2db72d44952b4fec17bc35855a5653c59ce915848ae8077d3e0a`.
- `golangci-lint run ./evals/hosted-load/fixtureprep` passed with zero issues; log SHA256 `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47`.

All logs are retained under `/Volumes/BuildOffload/validation/serenity-eval-git-migration-20261002/logs/`. `git diff --check` passed. Explicit-file tests compile and exercise ignored command helpers without calling their report-generating `main` functions.

This receipt records source-only caller evidence. Whole-module Git scanner coverage, independent review, coordinator integration, shared multi-package qualification, and T24.30 acceptance remain separate and open. The canonical source claim remains held pending the coordinator's banked handoff instruction.
