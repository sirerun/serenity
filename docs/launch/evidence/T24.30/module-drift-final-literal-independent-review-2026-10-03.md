# T24.30 whole-module Git drift scanner independent review

Review scope: exact candidate commit `04cf9ff7604bc0c2a988e235a91b525d6a677270` in branch `review/gitrun-module-drift-20261003`; tracked worktree remained clean. `internal/gitrun/drift_test.go` SHA-256: `20aa37225003aec16156932a37d2523e2bc0b5c70298afde23a8c9b222e4e42b`.

## Verdict: HOLD

`lexicallyShadowed` is function-wide rather than callsite-lexical. It gathers every `ValueSpec`, short declaration, and range declaration below the containing `FuncDecl`, without checking scope ancestry or declaration position. This causes both false negatives and false positives:

- A real imported `os/exec` Git call is silently omitted when a later nested block declares the same alias (`sh`), default name (`exec`), or dot-imported function (`Command`).
- A nested function literal's later local alias declaration also suppresses a real imported call outside that literal.
- A parameter named like the imported alias or dot-imported function in a function literal is not collected at all, so its local call is falsely reported as an imported Git process.
- A declaration in a separate `FuncDecl` does not suppress a call in another function; this neighboring control behaved correctly.

Fixtures use valid Go syntax and invoke the unmodified scanner copied from the exact candidate. Both RED logs and the harness are under `harness/`: `review_test.go`, `drift_test.go`, `callsite-shadow-red.log`, and `function-literal-parameter-red.log`. The lexical mutation test failed on all four in-function false-negative cases and passed the separate-function control; the inverse-shadow test failed for both function-literal parameter cases.

## Local checks

Fresh uptime before build lease: one-minute load `2.67`, below the repo threshold. External SSD was mounted, writable, and reported 762 GiB available. Lease claim succeeded with receipt `WON: R-build-lease c14191dd36acaaac24141230086899087675d2eb` and was released immediately after Go checks. Commands ran with the task's assigned external SSD Go cache/module/temp/lint paths and `GOFLAGS=-p=2`:

- `go test -count=1 ./internal/gitrun` passed.
- `go vet ./internal/gitrun` passed.
- `golangci-lint run ./internal/gitrun` passed with 0 issues.

These green package checks do not cover the demonstrated scope bug. No tracked source was edited in the review worktree.

## Source coverage and bounds

The test contract itself walks all module directories, parses non-test `.go` files independent of build selection, and skips only `.git`, `vendor`, `node_modules`, and the exact `internal/gitrun` subtree. The receipt reports 351 parsed Go source files; my focused package run passed the scanner on the clean source tree. Fixtures already check ignored build-tag source, alias/dot import, literal executable forms, exact subtree exemption, deterministic output, malformed parse refusal, and missing-root refusal. They do not check nested lexical scope boundaries or function-literal parameter shadows.

Source inspection found the production literal raw runner call `exec.CommandContext(ctx, "git", full...)` in the exempt `internal/gitrun/gitrun.go`; `internal/writer/history.go` obtains commands through the runner interface. Other `os/exec` imports found in production are process configuration/type/error uses (including platform-specific `*exec.Cmd` configuration and `*exec.ExitError` handling), not direct `Command` calls. No non-test `StartProcess`, `syscall.Exec`, `unix.Exec`, `exec.LookPath`, or shell-launch form was found by source search. This inventory does not prove the absence of wrapper variables, computed executable names, or shell command strings elsewhere: those are outside this AST check's stated limits. The receipt's limits on wrappers, computed executable names, `LookPath`, shell strings, and process APIs outside `os/exec` remain appropriate.

Ajent MCP tools were unavailable. I read the actual root `ajent.social` feed and coordinator board at task start; the feed entry recorded this scanner assignment, and no trusted hold/lift signal was found in the entry inspected.

## Corrected-candidate future controls

Retain these fixtures as required tests on the author correction: a real outer imported call must remain reported despite a later nested local alias/default/dot declaration or nested function-literal local; local function-literal alias/dot parameters must not be reported; a same-named local in a separate function must not hide a real call. The exact original candidate bytes remain present here for reproducibility; a corrected commit has not yet been reviewed in this worktree.

## Corrected candidate review: final verdict CLEAR for exact head `48abce76e7c513df3014a0d5a638395977c076ae`

I fetched only the corrected branch from the author's clone into this disposable review clone, then reviewed it on branch `review/gitrun-module-drift-20261003-corrected`. The checkout is clean. `internal/gitrun/drift_test.go` SHA-256 is exactly `aa4619da8b30c9ccefe14117d581c288c74bdf446ccd8380d1ce84203bc59347`, matching the requested source hash. The receipt is supplemental: it preserves the original 04cf9ff evidence and records the correction and missed-line regression.

The scanner now tests the callsite identifier's parser-resolved `ast.Ident.Obj` binding rather than accumulating same-name identifiers across a function body. That distinction passed the independent harness unchanged against the corrected source:

- Real imported Git calls remained findings with later nested same-name declarations for aliased package, default package, and dot-import forms.
- A real imported call remained a finding with a nested function-literal local alias.
- Same-name declarations in another function did not hide the imported call.
- Function-literal parameters shadowing an alias or dot-imported function produced no false positives.

The corrected control test command passed all seven cases: `go test -count=1 -run 'TestReview(LexicalShadowingMutationControls|FunctionLiteralParameterShadowFutureControl)$' -v` in the external review harness, after replacing its scanner copy with the corrected exact source.

At exact 48abce, `go test -count=1 ./internal/gitrun`, `go vet ./internal/gitrun`, `golangci-lint run ./internal/gitrun`, and `gofmt -l internal/gitrun/drift_test.go` all passed. Fresh one-minute load was 7.97 before these scoped package checks. No build lease was used because this is one explicit package. `git diff --check 04cf9ff..HEAD` passed and the tracked worktree is clean.

The corrected candidate closes the confirmed callsite binding bug and passes independent false-negative and false-positive controls. Verdict is CLEAR for this source-level candidate only. This is not registry acceptance, deployment, provider qualification, or the coordinator's separate full scoped gate.

## Follow-up literal-form review of exact 48abce: narrow HOLD

The callsite-shadowing CLEAR above remains valid and preserved, but two bounded literal spellings are also missed by `isGitExecutable` and should be fixed before final merge:

1. `exec.Command(("git"), "status")` parses with an `ast.ParenExpr` around the string literal. `isGitExecutable` accepts only `*ast.BasicLit`, so the direct literal call is omitted.
2. An ignored source file containing `exec.Command(` + raw literal `` `C:\Program Files\Git\bin\git.exe` `` + `, "status")` is parsed on Darwin, but `filepath.ToSlash` and `filepath.Base` use the host's path separators. Backslashes remain in the basename, so `git.exe` is not recognized.

Both are valid direct literal executable forms, with the second also checking platform-independent scanning. The independent fixture and failing log are `harness/literal_review_test.go` and `harness/bounded-literal-red.log`. Running `go test -count=1 -run TestReviewBoundedLiteralForms -v` against the exact corrected scanner exited 1 and reported no findings for either expected line.

Recommended bounded correction: unwrap `ParenExpr` nodes until reaching the literal expression, and normalize executable strings using separator rules that recognize both `/` and `\\` (or use a platform-independent basename normalizer), then continue the exact `git`/`git.exe` basename match. No computed constants, concatenation, wrappers, or broader expression evaluation are implicated or requested.

Review checkout remains exact `48abce76e7c513df3014a0d5a638395977c076ae`, with tracked scanner hash `aa4619da8b30c9ccefe14117d581c288c74bdf446ccd8380d1ce84203bc59347`; no production files were edited. Current verdict for final merge is HOLD narrowly for these literal forms. The seven lexical shadowing controls and their prior CLEAR result remain intact as historical scope evidence.

## Final literal-normalization correction review: CLEAR for exact head `c16156d99d2555fc61cc4cd57ee60c4a2864a6f5`

The exact candidate was fetched from the author's branch into a new review branch `review/gitrun-module-drift-20261003-literals-corrected`, preserving the 48abce checkout history and prior HOLD evidence. The review worktree is clean. `internal/gitrun/drift_test.go` SHA-256 is `bffe063e83966124d446da5b21deaa0feb9c66e898e24e27a677191e2c67309e`, matching the requested source hash.

Independent review of the change confirms that `isGitExecutable` strips enclosing `ast.ParenExpr` nodes, converts both slash characters to `/` before taking the basename, and compares `git`/`git.exe` case-insensitively. The focused test suite includes a Windows-tagged fixture for parenthesized `git`, a Windows `git.exe` path, and uppercase `GIT.EXE`.

Against the exact corrected source copied into the external harness, all nine independent controls passed: the two bounded literal cases, five lexical callsite cases (later nested alias/default/dot declarations, nested function local, separate function), and two function-literal parameter shadow cases. `go test -count=1 ./internal/gitrun` passed at the exact candidate head. The explicit whole-module scanner test passed and logged 351 non-test Go files parsed. Fresh one-minute load was 5.78 before the focused package test and 4.94 before the scanner-only run; external SSD had 762 GiB available. No shared build lease was used for this single-package work. `git diff --check 48abce..HEAD` passed and the tracked review worktree is clean.

Final independent source-level verdict: CLEAR for exact `c16156d99d2555fc61cc4cd57ee60c4a2864a6f5`, including the previously confirmed lexical binding and literal-form cases. The prior 04cf9ff and 48abce HOLDs remain documented above as chronology. Production source inventory and AST-scan bounds are unchanged: the direct literal raw runner call remains in the exact exempt `internal/gitrun` subtree; wrapper/computed executable and shell-string forms remain outside the analyzer's stated scope. This review did not rerun vet or lint after the bounded correction; coordinator full scoped gates remain separate, and this verdict does not claim registry acceptance, provider qualification, deployment, or hosted/live qualification.
