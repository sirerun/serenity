# Eval Git migration readiness

Coordinator source inspection against e6dc3f66 (the inspected continuation checkout has byte-identical Go/module files). This records a remaining implementation lane, not source authorization or T24.30 acceptance.

Four raw Git subprocess calls remain in three non-test eval files: `evals/calibration/gen_calibration.go:66`, `evals/brainbench/publish_trend.go:135`, and `evals/hosted-load/fixtureprep/verify.go:734,775`. The first two files use `//go:build ignore`; ordinary `go test ./...` does not compile those command files. The verify calls are the read-only fixture observation closure and its repository provenance probe.

A future isolated lane should own only those three files, narrowly scoped fixtureprep tests and its receipt under a fresh source claim. Preserve missing-commit `unknown`, existing caller directory semantics, context, all existing read arguments and observational failures. Use a read-only gitrun runner pinned to the intended directory; no -C or -c forwarding, write permissions or gitrun API expansion. Current allowlist already includes rev-parse, rev-list, status and ls-files. No calibration reports, published trends, fixture data or live load runs should be regenerated as a side effect of verification.

Before edits, prove hostile inherited GIT_DIR redirects at least the actual fixture observation path using only owned temporary repositories. Validate both ignored commands through an isolated build/explicit-file harness that does not invoke their output-producing main functions. Test repository provenance failure and context behavior; record preservation controls separately from genuine pre-fix failures. Focused Go checks require current load <=10, external cache/temp and actual shared lease for multi-package checks.

T24.30 remains separate until the owning hosted migrations and these eval calls are landed. Its scanner must parse every non-test Go file in the module, including ignored command files; the existing six-package roots are insufficient. Do not exclude evals, weaken the whole-module requirement, or infer coverage from rg alone. Scanner tests should reject import aliases and literal exec Git calls rather than trust a spelling-dependent import name. Exactly which indirect executable forms need enforcement must be recorded from the task contract and actual module audit, not invented as an unbounded static analysis project.

No production source, dependencies, provider requests, deployment or spend were changed for this record.
