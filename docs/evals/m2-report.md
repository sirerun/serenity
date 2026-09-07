# M2 exit verification report (T2.22)

Real run on David's laptop, 2026-09-07, against `main` at `f89b64e` (T2.13
merged, all six M2-exit deps -- T2.3, T2.4, T2.5, T2.8, T2.11, T2.13 --
checked done in `docs/plans/E2-m2-reconcile.md`). RFC 0001 §17's M2 AC line
names five clauses (`verifies: [UC-014, UC-015, UC-017, UC-026, UC-019]`);
each is run here for real against real fixtures -- a real temp git repo, a
real SQLite-backed disposition store, the real race detector -- not
re-stated from an existing CI badge. Every command below was executed in
this session; every output block is pasted verbatim, unedited.

## Run record

- **Machine**: macOS 26.6.2 (build 25G83), arm64 (laptop).
- **Go**: `go1.27.1 darwin/arm64`.
- **Repo state**: worktree `wave-pool-T2.22` off `origin/main` @
  `f89b64e459d4335ff2a1b93e6d291a1767913e94`.
- **Scope**: each of the five clauses maps to the package that owns it
  (T2.2/reconcile, T2.3/supersede, T2.13/entities, T2.8/disposition,
  T2.11/ladder). Where the existing acceptance test for that task's own acc
  line *is* the RFC AC verbatim (T2.2, T2.8, T2.11), that test is run
  directly with `-v` and its real output captured. For UC-015 (supersession
  "visible in `git diff`"), the existing integration test only asserts on
  diff line counts/content, not printed diff text -- so a small standalone
  driver (not committed, deleted after this report) reran the identical
  fixture and printed the literal `git diff` output a human would see,
  since the RFC AC explicitly names that as the observable artifact.

## AC1 (UC-014): contradicting-balance fixture -> correct A/B disposition item

RFC: "an injected contradicting-balance fixture produces a correct A/B
disposition item." Two active claims on `(acme-corp, has_balance)`, objects
`$500` vs `$700`, no validity window or scope qualifier -- textbook
same-time contradiction (RFC §10.2).

Command:

```
go test -v -run TestContradictingBalanceFixtureProducesExactlyOneConflictItem ./internal/reconcile/...
```

Observed output:

```
=== RUN   TestContradictingBalanceFixtureProducesExactlyOneConflictItem
--- PASS: TestContradictingBalanceFixtureProducesExactlyOneConflictItem (0.01s)
PASS
ok  	github.com/sirerun/serenity/internal/reconcile	0.289s
```

The test drives `reconcile.Engine.Process` against a real SQLite-backed
`disposition.Store` (`index.Open` on a temp file, not a mock), asserts
`Detection.Verdict == VerdictConflict`, exactly one `KindReconcile` item is
staged (via `Store.List`, not just the return value), and the item's
payload carries both sides' distinct provenance (`sha-source-a` /
`sha-source-b`). **Clears.**

## AC2 (UC-015): accept -> supersession line in shard + updated fence head, visible in `git diff`

RFC: "on accept, a correct supersession line in the shard plus an updated
fence head visible in `git diff`."

Command (existing integration test, real temp git repo):

```
go test -v -run TestApplyShardTierSupersedingLineAndRegeneratedHead ./internal/supersede/...
```

Observed output:

```
=== RUN   TestApplyShardTierSupersedingLineAndRegeneratedHead
--- PASS: TestApplyShardTierSupersedingLineAndRegeneratedHead (0.18s)
PASS
ok  	github.com/sirerun/serenity/internal/supersede	0.626s
```

That test seeds a real `git init` repo with a shard line (`has_balance:
$500`, id `claim-b`) and its regenerated fence head, commits, then calls
`supersede.Writer.Apply(claim-a: $700, claim-b)` and `writer.Flush`, and
asserts the shard file gained exactly one appended line
(`"supersedes":"claim-b"`) and the fence head row now shows `claim-a`. To
see the actual diff a human would see -- the RFC AC's own words -- a
standalone driver reran the identical fixture and printed real `git diff`
output:

```
=== git diff HEAD~1 HEAD -- <shard file> ===
diff --git a/brain/claims/acme-corp/has_balance.jsonl b/brain/claims/acme-corp/has_balance.jsonl
index 16af008..2c69cc5 100644
--- a/brain/claims/acme-corp/has_balance.jsonl
+++ b/brain/claims/acme-corp/has_balance.jsonl
@@ -1 +1,2 @@
 {"id":"claim-b","subject":"acme-corp","predicate":"has_balance","object":"$500","object_key":"$500","confidence":0.9,"state":"active","src":"e1#1","family":"has_balance","provenance":{"source_sha256":"sha-claim-b","span":"0-10","model":"test-model@v1","observed_at":"2026-09-07T12:00:00Z","actor":"machine"}}
+{"id":"claim-a","subject":"acme-corp","predicate":"has_balance","object":"$700","object_key":"$700","confidence":0.9,"state":"active","supersedes":"claim-b","src":"e1#1","family":"has_balance","provenance":{"source_sha256":"sha-claim-a","span":"0-10","model":"test-model@v1","observed_at":"2026-09-07T13:00:00Z","actor":"machine"}}

=== git diff HEAD~1 HEAD -- <fence page> ===
diff --git a/brain/entities/topic/acme-corp.md b/brain/entities/topic/acme-corp.md
index d135dc3..e7803d4 100644
--- a/brain/entities/topic/acme-corp.md
+++ b/brain/entities/topic/acme-corp.md
@@ -11,7 +11,7 @@ slug: acme-corp
 <!-- serenity:claims:begin -->
 | id | predicate | object | conf | valid | src | state |
 |----|-----------|--------|------|-------|-----|-------|
-| claim-b | has_balance | $500 | 0.90 |  | shard | active |
+| claim-a | has_balance | $700 | 0.90 |  | shard | active |
 <!-- serenity:claims:end -->
 
 <!-- serenity:timeline:begin -->
```

Exactly one appended (never-mutated) shard line carrying `supersedes`, and
the fence head row swapped from the old id to the new one -- both real,
both visible in `git diff` exactly as the AC requires. **Clears.**

## AC3 (UC-017): entity merge round-trips

RFC: "entity merge round-trips." Read as: merging two fixture entities
re-points every claim onto one page, and undo restores the pre-merge state
byte-identically -- T2.13's own acc line, the direct source of this clause.

Command:

```
go test -v -run 'TestMergeRepointsEveryClaimAndRendersOnePage|TestUndoRestoresPriorStateByteIdentically' ./internal/entities/...
```

Observed output:

```
=== RUN   TestMergeRepointsEveryClaimAndRendersOnePage
--- PASS: TestMergeRepointsEveryClaimAndRendersOnePage (0.07s)
=== RUN   TestUndoRestoresPriorStateByteIdentically
--- PASS: TestUndoRestoresPriorStateByteIdentically (0.10s)
PASS
ok  	github.com/sirerun/serenity/internal/entities	0.519s
```

The first test merges two real fixture entity pages (4 claims total across
both), asserts all 4 claims land on the survivor's page and the absorbed
page's file is gone; the second undoes that merge and asserts both pages'
bytes are restored identical to their pre-merge snapshots (not just
field-equal -- literal byte comparison of the on-disk file). Merge then
undo is the round trip; both directions are real writes through
`internal/writer`, not in-memory only. **Clears.**

## AC4 (UC-026): two-client dispose race resolves to one durable verdict

RFC: "a two-client dispose race resolves to one durable verdict."

Command (real race detector, single package -- no fleet build lease
required):

```
go test -race -v -run TestTwoClientDisposeRaceExactlyOneWins ./internal/disposition/...
```

Observed output:

```
=== RUN   TestTwoClientDisposeRaceExactlyOneWins
--- PASS: TestTwoClientDisposeRaceExactlyOneWins (0.69s)
PASS
ok  	github.com/sirerun/serenity/internal/disposition	2.058s
```

100 iterations, two real goroutines racing `Store.Dispose` on the same item
per iteration (one accept, one reject) against a real SQLite-backed store,
under `-race`. Every iteration: exactly one winner and one loser, the loser
gets `AlreadyDisposed=true` carrying the *winner's* verdict (not its own
attempted one), and exactly one history row is ever appended. Zero data
races reported across all 100 iterations. **Clears.**

## AC5 (UC-019): ladder calibration report exists and shipped defaults match it

RFC: "calibration report exists" (the plan doc's own paraphrase of T2.11's
acc line adds "and the shipped defaults match it," which is what the test
below actually asserts, so both are checked here).

Commands:

```
ls -la evals/calibration/
go test -v -run TestDefaultConfigMatchesCommittedCalibrationReport ./internal/ladder/...
```

Observed output:

```
total 336
drwxr-xr-x@ 4 dndungu  staff     128 Sep  7 01:05 .
drwxr-xr-x@ 6 dndungu  staff     192 Sep  7 01:05 ..
-rw-r--r--@ 1 dndungu  staff    2076 Sep  7 01:05 gen_calibration.go
-rw-r--r--@ 1 dndungu  staff  165129 Sep  7 01:05 report.json

=== RUN   TestDefaultConfigMatchesCommittedCalibrationReport
--- PASS: TestDefaultConfigMatchesCommittedCalibrationReport (0.00s)
PASS
ok  	github.com/sirerun/serenity/internal/ladder	1.881s
```

`evals/calibration/report.json` exists (165 KB, the full 180-cell sweep
grid) and its `chosen` field is:

```json
{
  "min_dispositions": 50,
  "min_accept": 0.98,
  "sample_rate": 0.05
}
```

which is RFC §10.3's original prior -- the calibration sweep confirmed
rather than changed the shipped numbers (T2.11's own disclosed finding: no
production disposition history exists yet, so the sweep runs against a
disclosed synthetic scenario battery, and its own bias in that situation is
to keep the prior rather than loosen it). The test asserts
`ladder.DefaultConfig().Default` equals the report's `Chosen` field-for-
field; a field-level mismatch would fail this test, not just a
freshness/existence check. **Clears.**

## Verdict

All five M2 acceptance clauses clear, each verified by a real command run
in this session against real fixtures (a real temp git repo for AC1/AC2,
a real SQLite-backed disposition store for AC1/AC4, the real Go race
detector for AC4, byte-level file comparison for AC3, and the actual
committed calibration artifact for AC5) -- not restated from a prior green
CI run. No family/clause required remediation; there is nothing to file as
a follow-up task from this report, unlike M1's (docs/evals/m1-report.md).

**M2 is exit-verified**: this report exists and records each of the five
RFC §17 M2 ACs with the command run and the real observed output, per
T2.22's own acc line.
