# Cached import budget

Run the measured 10,000-message workload without credentials or model spending:

```sh
SERENITY_IMPORT_BUDGET_OUTPUT=/tmp/import-budget.json \
  GOMAXPROCS=2 go test -p 1 -count=1 -timeout 4h -v \
  -run '^TestImportBudget10K$' ./evals/importbudget
go run ./evals/importbudget/check \
  -current /tmp/import-budget.json -markdown /tmp/import-budget.md
```

Add `-previous /path/to/previous.json` to compare with the previous recorded run.
Use the same runner class and `GOMAXPROCS`; set
`SERENITY_IMPORT_BUDGET_ENVIRONMENT` to identify that class. A missing baseline
is an explicit first-run bootstrap, never a silent fallback for a corrupt or
unreadable file. The comparator requires exactly 10,000 messages, complete
stage/count evidence, zero live model calls, a total below four hours, and a
slowdown below 20%. Different corpus/cache/runner identities require an explicit
baseline migration instead of a misleading comparison.

The workload is the seeded generator from `evals/gen/messages`. Its checksum
manifest is verified before loading a test-only loopback IMAP server. The real
IMAP connector, source store and Git writer, chunker, extraction file cache,
reconciliation engine/disposition store, claim writer, SQLite rebuild, and
`ReembedMissing` execute. The report verifies persisted source/claim/vector
counts and committed canonical files. A 25-message smoke test runs in normal CI;
the larger performance test explicitly skips unless its output path is supplied.

Cached extraction candidates are synthetic golden fixtures derived from the
invented message template, using one preference per message. Embedding fixtures
are deterministic 512-dimensional vectors. Neither fixture claims to be an
actual model response or evidence of semantic quality. Cache misses fail; there
is no live provider fallback. All server/model doubles live in `_test.go` files.
The benchmark calls production components directly; it does not invoke the CLI
or certify the CLI's provider configuration.

Measured stages are poll, source storage plus commit, chunk/extract, reconcile,
claim writes plus commit, index, and embed. The total is the sum of those measured
stages. Corpus generation, mailbox loading, extraction-cache population, and
embedding-cache preparation are excluded. This measures pipeline overhead on
cached outputs, not provider latency, inference speed, or real-mailbox throughput.
The real-message laptop acceptance run remains a separate manual release gate.

`import-budget-nightly.yml` runs on Ubuntu 24.04 with two Go processors, uploads
JSON test evidence and the timing table, then appends successful reports to the
separate `results/import-budget` branch. Failed regressions retain their artifacts
and do not replace the last accepted baseline. An unavailable remote is an error,
not permission to bootstrap. Workflow concurrency serializes baseline updates.


The benchmark database uses `<brain>/.serenity/index.db`, matching production, so
embedding checks the correct brain's canonical eligibility. Native claim projection
adds per-claim search entries to the raw-source and entity-page entries; reports
therefore count the actual increased vector workload. The vector cache algorithm,
corpus and model pins are unchanged. Performance comparisons retain their baseline
and disclose this added work rather than resetting the gate to hide it.
