# Keyed memory recovery review record

Local qualification, 2026-09-10. Extends `remember` through the existing writer
and immutable memory source. See [contract](../plans/keyed-memory-recovery.md).

- Targeted store/writer/memory race suites pass, including concurrent retries,
  durable restart/withdrawal recovery, all changed durable fields, key format
  validation and conflicting merged keys.
- `go vet ./...`, `go build ./...` and full `go test -race -count=1 -timeout
  600s ./...` pass: 1,793 test/subtest passes, zero failures, six skips.
- Skips: opt-in 10K import performance, opt-in deliberately broken conformance
  self-check, subprocess crash helper, external gbrain conformance, and two
  read-facade drift cases without supported structured actions. None is claimed
  as executed evidence.
- First targeted run had a test-helper signature error, corrected and rerun.
  First full run caught omitted request/response schema properties; published
  schemas and the error enum were updated, and the full suite passed on rerun.
- Lint passes with zero issues. Its first run requested a simpler key validator;
  changed to a positive switch and reran store race tests and lint successfully.
- Real disposable CLI/MCP replay passed recovery of the same withdrawn ID,
  `expired=true`, continued recall exclusion and `operation_conflict` for changed
  attribution. No embedding provider or external service was used.

Process exclusion is still absent in this change. The protocol probe confirmed
a second process offers write tools; do not deploy multiple writer processes to
one brain. Keys do not grant access or turn raw material into accepted claims.
This is not a document revision API or a shared Ajent brain release.
