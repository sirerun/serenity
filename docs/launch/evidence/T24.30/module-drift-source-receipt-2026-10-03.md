# T24.30 whole-module Git drift source receipt

Base: assignment commit `e4469a6a47bc670a2000758fc262cfd5591220ed`, based on landed PR348 main `2fee7e636e9149fe42ad214866117bc971cc8f84`. Work is limited to new `internal/gitrun/drift_test.go` and this receipt. Existing runner production and test source hashes are unchanged from the assignment base. No claim operations were performed.

## Enforcement and source audit

`TestNoRawGitExecAcrossModule` starts at the discovered `go.mod` root and parsed 351 non-test `.go` files on the clean tree. It parses source regardless of Go build selection, including ignored command files, then reports sorted module-relative `file:line` findings. The only package source exception is the exact `internal/gitrun` directory. `.git` exists and is skipped as repository metadata; `vendor` and `node_modules` are absent. The scanner also skips those two dependency directories if present.

The AST check resolves default and aliased `os/exec` imports and dot imports, both `Command` and `CommandContext`, and literal executable strings (including raw strings and paths whose basename is `git` or `git.exe`). It accounts for local function parameters and declarations that shadow those imported names. Fixtures cover an ignored build-tag command, alias/dot imports, raw and path literals, exact allowed-directory scope versus `internal/gitrunner`, deterministic ordering, lexical shadows, non-Git calls, malformed source refusal, and walk-error refusal. `internal/gitrun/gitrun.go` is the only production `os/exec` invocation found in the source audit; it is inside the permitted runner package. The only other textual `exec.Command` match is a comment there.

This is a bounded syntactic check. It does not follow wrapper functions, variables or computed executable names, `exec.LookPath` results, shell command strings, or process APIs outside imported `os/exec`. The source audit found no current non-test use of `StartProcess` or `syscall.Exec`; future indirect forms would need a separate explicit analyzer decision.

## Genuine mutation control

Before cleanup, an owned untracked fixture was created at `evals/.t24-30-drift-red/mutation.go`, outside the previous subset roots. Its SHA-256 was `967c0aa17e29cafaf804bfc0b981d1b02ed5de4a73d1ccaac24a1517556f3f6b`. With scanner source SHA-256 `20aa37225003aec16156932a37d2523e2bc0b5c70298afde23a8c9b222e4e42b`, the command `go test -count=1 -run '^TestNoRawGitExecAcrossModule$' ./internal/gitrun` failed as intended and reported `evals/.t24-30-drift-red/mutation.go:5`. The exact owned file was removed and its now-empty fixture directory removed. A post-cleanup focused scanner and fixture run passed; the full package suite had also passed after cleanup.

## Local verification

- `go test -v -count=1 ./internal/gitrun` passed, including the whole-module scanner and existing package tests. Scanner logged 351 parsed non-test files.
- After exact mutation cleanup, `go test -count=1 -run '^(TestNoRawGitExecAcrossModule|TestDriftScan)' ./internal/gitrun` passed.
- `go vet ./internal/gitrun` passed.
- `golangci-lint run ./internal/gitrun` passed with 0 issues.
- `gofmt -l ./internal ./cmd ./pkg` returned no paths.
- All Go commands used the assigned external-SSD cache/temp paths and were run with fresh one-minute load below 10. Checks were scoped to the single package; no full-module race or multi-package gate was run.

The root `ajent.social` feed file and coordinator board were read at task start; Ajent MCP tooling was unavailable in this environment. This receipt reports source-level local checks only. It does not claim registry acceptance, hosted/provider/live qualification, deployment, or full T24.30 acceptance. Independent exact-head review and coordinator full scoped gate remain outstanding.

## Callsite binding correction (supplements commit 04cf9ff)

A follow-up regression found that the initial 351-file GREEN did not cover a lexical false negative. The committed `04cf9ff` scanner aggregated same-name declarations over a function body; that hid imported calls before a later declaration and after a nested block ended. The source was otherwise unchanged when `TestDriftScanShadowsOnlyAtCallsite` was added. Against that scanner, the test failed: only `pkg/callsite.go:18` and `:33` were found, while imported Git calls at lines 6, 15, 21 and 30 were missed. These fixtures exercise both aliased package calls and dot-import calls; the other-function calls at lines 18 and 33 are positive controls.

The correction uses the parser's callsite `ast.Ident.Obj` binding: default and aliased import package uses and unresolved dot-import names have no local object, while a local parameter or declaration links to its object. Regression fixtures verify this behavior for later declarations, nested block shadows, and separate functions. Calls are now classified at the expression itself, without a function- or file-wide name set. The targeted `go test -v -count=1 -run '^TestDriftScan' ./internal/gitrun` passes after correction.

The original whole-module GREEN and its 351-file count are preserved as historical evidence only; they do not establish that the original scanner caught these lexical cases. Final whole-module/package verification is repeated with the correction before this supplemental commit. The original mutation evidence, exact cleanup, and PR348 assignment baseline above remain unchanged.
