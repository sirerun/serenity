# Evidence and qualification manifests

Each task owns `docs/launch/evidence/T23.N/`. Create no pretend PASS receipt during planning. `result.json` must match [result.schema.json](result.schema.json). Keep large artifacts in the agreed private artifact store with SHA256 references; Git contains sanitized summaries only.

Required receipt information:

- Task/profile/status; full tested source SHA and UTC; dependency commits.
- Exact resolved command and environment, test counts/pass/fail/skip with each skip reason.
- Expected vs observed acceptance for every checklist item; fixture/local/live/human distinction.
- Binary SHA256, configuration/corpus hashes where applicable; provider mode/model/dimensions, no key values.
- Artifact paths/links and checksums; cost incurred vs approved maximum; authorization record IDs for external actions.
- Known limits and concrete blocker owner/input. A no-test docs task explicitly says not-applicable; it cannot claim zero tests as testing success.

Statuses: `PASS`, `PARTIAL`, `FAIL`, `BLOCKED`, `NOT_RUN`. PASS only when every task criterion is satisfied; evidence level describes what it proves. A fixture task can PASS its local contract, but cannot satisfy a dependent live gate. Task69 alone permits terminal NOT_RUN for unavailable humans under task70’s recorded exception. Other required tasks may not silently waive evidence.

Record current source before build (`git rev-parse HEAD`) and ensure clean tree or a recorded patch hash; build the CLI from that source; hash the binary before and after external tests. Final acceptance requires a clean, committed revision. Any changed input invalidates affected evidence. Do not rerun unrelated suites merely to inflate counts.

## Harness command contract

New Python qualification harnesses in the task registry must support the specified modes, `--manifest PATH`, and `--output PATH`. The [example manifest](qualification.example.json) is a non-executable template: null budgets/empty authority deliberately block live execution. Workers implement validation, not defaults that turn missing fields into unlimited access.

- `--fixtures`: local synthetic provider only; external network calls disabled except explicitly permitted local hosts. It tests harness mechanics, not semantic quality or actual costs.
- `--live`: requires explicit environment allowlist, exact source/artifact, all task gates, key references, fixed request/token/time/cost limits and provider selection. Refuse production targets unless the task explicitly authorizes them. Stop with a partial receipt at the first exhausted budget.
- `--test-mode`: Stripe task validates livemode=false and account/object ownership before operations.
- `--plan`/`--apply`: apply checks the unchanged plan hash and scoped authority. A metadata-only preflight must not incur model calls or send messages.
- Exit0 = every required assertion passed; exit1 = tested failure; exit2 = blocked/missing prerequisite or exhausted budget; exit3 = invalid input. Emit result JSON even on failure. Missing/zero cases never exit0.
- Harness reports exact planned/executed cases. Live semantic run requires100 corpus cases with the95/5 split. Client run covers every listed scenario and10 timed signup runs. Load requires3 repetitions of the frozen manifest. Other suites carry a versioned explicit case list; omission is a failure, not a skip.
- Retry policy is explicit and bounded; provider/key budget takes precedence. Include readiness and re-embedding/recovery consumption. Avoid concurrent live tasks sharing a global allowance without a partitioned cap.

Set variables in the task’s private shell, never secrets:

```sh
export QUALIFICATION_MANIFEST=/absolute/private/path/to/approved-manifest.json
export EVIDENCE_DIR="$PWD/docs/launch/evidence/$TASK_ID"
```

Manifest secret references point to the existing approved secret manager or0700/0600 files. They do not contain credential values. Provider logs, browser traces and screenshots can leak login links or memory: inspect/redact synthetic artifacts before publication. Production customer data is never an evaluation fixture.

## Semantic scoring details

The95 positive queries each have one target ID: a hit means that ID appears in top5 eligible results. Score per-category and overall. The5 expected-empty cases use isolated disposable brains containing only a forgotten/expired fact, so arbitrary unrelated recall results cannot make the scoring ambiguous. A forbidden expired/deleted/foreign fact is always failure, regardless of aggregate score. Lexical negative controls use a separate local read-side test hook and never expose an unauthenticated production endpoint. History inside Git exports is assessed separately from live facts.

## Qualification inputs are frozen

Commit corpus/expected IDs and workload manifests before obtaining live results; record reviewer/date/hash. Include Unicode/multilingual facts, similar names, negation and contradictory distractors. Do not tune after observing misses without creating a new reviewed evaluation revision and preserving the failed evidence.

Load manifests specify profile and actual cardinalities: paid=1Scale+3Builder+10Free at their ratified live-fact limits (90,000 total facts across the14 accounts), agreed number of brains within plan limits, storage/history near the published caps, specified traffic mix/concurrency/rate/duration/burst,3 repetitions. Pilot cardinalities match its separately approved cohort/caps. Keep boundary exhaustion tests separate from the eligible steady-state throughput sample. Use precomputed/cached embeddings for large infrastructure-only datasets if necessary; label that limitation and run a separate live-provider sample. Do not count synthetic entitlements or cached vectors as live billing/provider proof.
