# BrainBench trend data (plan T5.10)

This branch holds only `evals/brainbench-trend.json`, an append-only
history of BrainBench retrieval-quality score rows produced by the
`brainbench-trend-nightly` workflow in `main`
(`evals/brainbench/publish_trend.go`, `internal/eval/brainbench`).

It carries no application code and is never merged into `main`.
