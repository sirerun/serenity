# Independent review: eval read-only Git callers

**Decision: CLEAR for source candidate `6f6b388f15336fb63b206e0d317cb227fdc16c06` against baseline `4b2fd8e656e448d712b2c7d9fa2bc28b9d78c469`.** The candidate is parented by inventory amendment `ef931eca3b1babecd29cb0431cc4e522e38e1c29`; its tree is `cdc844c0aa926b92fdf6ea5671f5f83c2370b27b`.

I reviewed the four assigned production callers and their adjacent tests: calibration, BrainBench `gen_trend`, BrainBench `publish_trend`, and hosted-load fixtureprep. Each caller now uses `gitrun.Foreign` rooted at its caller repository and invokes the same read-only Git subcommands with the original context. Fixtureprep retains `rev-list --count HEAD`, `status --porcelain -z`, and `ls-files -z -- brain/sources`; its three calls share one rooted runner. The two BrainBench command helpers retain `GITHUB_SHA` preference and return `unknown` on fallback failure. Calibration also retains `unknown` on missing provenance. Fixture observation still reports Git failures as observations.

The new controls cover inherited `GIT_DIR` for all four callers, caller-directory selection, missing-commit fallback, both BrainBench `GITHUB_SHA` preferences, and cancellation. The ignored command tests were run with explicit file lists containing only the command source and its adjacent test. Those invocations passed without calling either report-generating `main` function or regenerating reports, trends, or fixtures.

I independently ran all four focused test commands; each passed. I also restored each production caller from baseline and reran its hostile-`GIT_DIR` test. All four failed for the intended reason: the raw baseline Git invocation read the foreign repository (the fixture observer instead reported zero tracked sources and two dirty paths). For the three ignored command tests, I temporarily adapted only the test call sites to the old helper signatures so the exact baseline command files compiled. Candidate source and test files were restored byte-for-byte from review `HEAD` afterward. The review worktree is clean, and `git diff --check` passes.

Logs are under `/Volumes/BuildOffload/serenity-eval-git-review-evidence-20261002/logs/`:

| Evidence | SHA-256 |
|---|---|
| `fixtureprep-focused.log` | `300b1247d45e7a68eef43c94542fbbf9d3c4455162dc8f6427faeaada1a62e7a` |
| `calibration-focused.log` | `e3b1734072a3b4519652704ba3cb667fd690caf88d8f3c7c31eb210702d7c9f1` |
| `gen-trend-focused.log` | `fbd015b71bf7b41585ea329aaa04de648ac34787ed8139cfcaafa1cb1d29c402` |
| `publish-trend-focused.log` | `f0a13604343647869c5c1405eb876ab26f315d55af553f2ef7999283726eb9e0` |
| `mutation-calibration.log` | `702a70b8274f7b37bb266c0f37492ba2d294c571cd638675ec417fb1f77913ef` |
| `mutation-gen-trend.log` | `efcb0f727f2de0ca2a7d4c78fe1ebe5f8a4d71c39802fbbb94038619c4a54d14` |
| `mutation-publish-trend.log` | `00d299fcc42b20365f64c330131712d4b540e2182680f12818f5a27d222d2f83` |
| `mutation-fixtureprep.log` | `c1e5f9398512470fa724320b34f1adc99a61b69d2359b1080c97dfbb823c069c` |

This CLEAR covers only the four caller changes and their focused tests. It does not claim whole-module Git scanner coverage, multi-package qualification, integration, or T24.30 acceptance.
