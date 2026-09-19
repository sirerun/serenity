# T23.60 offline fixture preparation (infrastructure only)

Status: **PARTIAL. This is preparation and an inventory. It is not a qualification.** No service ran. No provider, cloud, billing or network call happened. `--live` stays unconditionally blocked.

`evals/hosted-load/fixtureprep` builds the seeded state the paid load profile starts from, in a new local directory, and a second command counts that state back from the directory. The verifier compares the directory to the frozen workload and the plan table. It never reads the preparer's manifest.

## Read this first: reduced smoke versus full

| | Reduced smoke | Full |
|---|---|---|
| Accounts | 1 Scale, 3 Builder, 10 Free | 1 Scale, 3 Builder, 10 Free |
| Brains | all 29 | all 29 |
| Memories | 290 (10 per brain) | **90,000** (each account at its plan memory cap) |
| Satisfies the frozen cardinalities | **never** (`satisfies_full_cardinality: false`) | yes, when the verifier reports it |
| Use | Fast check of the tooling and the negative fixtures | The seeded state the decision request asks about |

Only the `full` profile can satisfy 14 accounts, 29 brains and 90,000 memories. `verify` needs `-expect full|smoke` from the caller and compares the disk to that. It refuses to call a smoke directory full.

## Three labels every receipt carries

1. **Storage and history gap, apart from the memory count.** The full fixture fills the memory count. It does not fill any storage quota. The measured bytes, their split (canonical facts, Git history, derived index) and each account's share of its quota are in [fixture-receipt.json](fixture-receipt.json). The fixture commits in batches, where production commits after every acknowledged remember, so its history is smaller than production's. The maximum production history is unmeasured.
2. **Provider quality is unqualified.** Vectors come from a hash embedder pinned as `fixture-hash-embedder-d<width>@infrastructure-only-v1` (`-dim`, 64 by default). They exist so the real index recovery code writes a real vector row for each fact. They say nothing about retrieval or provider quality, and no recall result over this fixture is evidence of either.
3. **Not the steady-state qualification.** Every account starts at its memory cap, so the gateway refuses every further nonreplay remember with `limit_exceeded` (class `quota`, outcome `tool_error`). The eligible-versus-boundary decision is open: [decision-request-full-cardinality-eligible-traffic.md](decision-request-full-cardinality-eligible-traffic.md).

## Commands

```sh
go build -o "$BIN" ./evals/hosted-load/fixtureprep
"$BIN" plan    -profile full|smoke
"$BIN" prepare -local-fixture-only -out /new/absolute/dir -profile full|smoke [-smoke-facts N] [-workers 1..4] [-commit-every N] [-dim 8..1536]
"$BIN" verify  -dir /new/absolute/dir -expect full|smoke -report report.json
```

`prepare` creates `<out>/data/control.db`, `<out>/data/brains/<id>/` for each brain, `<out>/credentials/<label>.token` (mode 0600, one synthetic credential per account, never printed or recorded), `FIXTURE-ONLY.json` and `manifest.json`. Exit status: 0 success, 1 the verifier found a difference, 2 a refusal or a usage error.

The accounts use `<label>@fixture.invalid` (`scale-0`, `builder-0..2`, `free-0..9`), the labels the load client uses.

## What is real and what is replaced

| Piece | How the fixture builds it |
|---|---|
| Accounts, brains | `hosted/store`, `hosted/provision` (`Provision`, `Additional`) |
| Credentials | `hosted/credential` `Issuer.Issue`, scopes `memory:read`, `memory:write`, bound to the default brain |
| Brain layout, config, Git baseline, index | The real `hosted/pool` open path, twice per brain: the first open creates the brain; the second runs `index.RecoverMemorySearch`, which builds the index rows and a vector for every fact |
| Canonical facts | `store.SourceStore.WriteMemoryFact` inside `writer.Queue.Submit`, under the brain's `writer.AcquireBrain` ownership; commits through `writer.Flush` |
| Embedder | **Replaced.** Infrastructure-only hash embedder. No provider call |
| Paid entitlements | **Direct SQL** into the fixture's own new control database (`subscriptions` row and `accounts.plan_id`). No ordinary interface grants a paid plan without a Stripe webhook |
| Legacy ids, duplicate handling | **Replaced.** Production allocates each id and resolves duplicates per write in `writer.MemoryFact.Remember`, which reloads the whole projection every call. The fixture assigns ids `1..N` and rejects a repeated fact SHA by construction. The verifier re-checks both |
| Git history | **Different.** Batch commits (`-commit-every`, default 500). Production commits after every remember |
| Fact text | Deterministic: a unique marker word plus words from the load client's vocabulary, 32 to 512 words (nominal, not tokens), at most 4,093 bytes, under the gateway's 4,096-byte cap. Fixed past `created_at`. Provenance `fixture:T23.60 synthetic infrastructure-only` |

## Guardrails

- `-local-fixture-only` is required. There is no endpoint, key, provider, host or environment option.
- The output path must be absolute and clean, must not exist (any file, directory, empty directory or symlink), and its parent must exist. It is refused under a service data root (`/var/lib`, `/srv`, `/opt`, `/data`, `/etc` and similar), inside any directory that holds `control.db`, another fixture's marker or a `.git`.
- `-endpoint` is optional, is only recorded in the marker and is never contacted. It accepts a bare `http(s)` origin whose host is a loopback IP literal. Hostnames (including `localhost`), private and public addresses, credentials, paths, queries and fragments are refused.
- The tool never deletes anything. A failed run leaves the marker at `status: preparing`, and `verify` refuses it. The owner removes it.
- A test scans the package source: it must import no network package (`net`, `net/http`, `net/rpc`, `crypto/tls`, `net/smtp`), call no `os.Getenv`, `os.Environ` or `os.LookupEnv`, and run no program but a literal `git`. The package links the gateway package only to call `Inventory` and `meter.Entitlement` on a scratch copy of the control database (see the gateway cross-check below); it starts no service.
- `verify` opens SQLite files immutable and read-only, runs Git with `--no-optional-locks`, and stats credential files without reading them. A test compares a directory's files, sizes and times before and after a verification.

## What the verifier checks

Expected values come from `workload.json` and `internal/hosted/plans`. Observed values come from the directory.

| Area | Check |
|---|---|
| Marker | Present, finished (`prepared`), profile equals `-expect`, workload hash equals the frozen workload's |
| Control database | Account count and plan mix; every account `active`; brain count per account equals its plan's; exactly one default brain; brain ids equal path keys; paid accounts hold one active subscription in their own plan whose period includes now, Free accounts none; one active credential per account, on the default brain, with both memory scopes |
| Credential files | One per account, regular, mode 0600, non-empty (never read); directory mode 0700 |
| Brain directories | Every database brain has a directory; no directory lacks a database brain |
| Canonical facts | Every `bytes` file hashes to its directory name, decodes as a `memory_fact`, has a unique legacy id, is at most 4,096 bytes, carries the fixture provenance, and appears in the set regenerated from the plan. No `memory_expiry`. Missing and unexpected facts are counted apart from the total |
| Projection | `store.LoadMemoryProjection` (the reader the gateway's inventory uses) counts the same active facts |
| Index | One `fact:` chunk per fact, one vector per fact under the fixture pin, each exactly the marker's width (64 floats by default), none under another pin |
| Config | Each brain's embedding pin equals the marker's |
| Git | Baseline plus `ceil(facts / commit_every)` commits, a clean tree, and two tracked source files per fact |
| Storage | Every non-directory file summed the way the gateway's `inventory` does, split into `brain` (canonical facts), `.git`, `.serenity` (derived index) and other |
| Gateway cross-check | On a scratch copy of the control database, the gateway's own `Inventory` and `meter.Entitlement` return the memory count, the storage bytes and the plan the verifier computed, for every account. The limit rule the gateway applies to a remember (`memories >= plan memories or storage >= plan storage`) is copied into the verifier, and a test pins the copy to the gateway source |

Each check reports expected and observed. The overall result passes only when all checks pass.

### A second reading in another language

`evals/hosted-load/fixture_recount.py` (stdlib Python, tests in `test_fixture_recount.py`) shares no code with the Go verifier. It reads only the control database (read-only, immutable), the frozen workload, the Go plan table and each brain's canonical fact files. It recounts accounts, brains and facts against the plan, checks that every fact file is content-addressed, decodes as a `memory_fact`, stays under 4,096 bytes and has a unique legacy id, and recomputes `fixture_content_sha256`. It says nothing about the index, vectors, Git history or storage. Its results for every retained fixture are in [fixture/](fixture/) (`recount-*.json`); on each one the digest equals the Go verifier's, and [fixture-receipt.json](fixture-receipt.json) refuses a receipt where they differ.

```sh
python3 evals/hosted-load/fixture_recount.py --dir /new/absolute/dir --expect full|smoke [--smoke-facts N] [--output recount.json]
```

## Negative fixtures

Each row is a Go test on a private copy of a prepared fixture. The untouched copy passes in the same run, so each failure is red to green.

| Drift | Check that fails |
|---|---|
| A fact directory removed | canonical count, total facts |
| A fact replaced by another well-formed fact (count unchanged) | content oracle: 1 missing, 1 unexpected |
| A fact added; untracked files | count, unexpected fact, uncommitted paths |
| Fact bytes altered by one bit | sha or decode mismatch |
| A `memory_expiry` planted | expiry count, projection count |
| Brain directory removed, orphan directory added | brain directories |
| Subscription plan, status or period changed, removed, or a Free account upgraded | entitlements |
| Account plan changed, extra account, account off the fixture domain | account checks |
| Extra brain, default flag removed | brain count, one default brain |
| Credential revoked, credential file made group-readable | credential checks |
| Vector row or chunk row deleted, vectors under another pin, wrong vector width, index file missing | index and vector checks |
| Config pin changed | config pin |
| Extra Git commit, tracked file modified | history, clean tree |
| Non-empty write-ahead log | control database not readable |
| Workload hash changed since preparation | marker |
| Smoke fixture verified as `full`, or with the wrong smoke size | marker profile, totals |
| Marker missing or unfinished | refusal (exit 2) |
| Manifest inflated or deleted | no effect: the verifier ignores it |

Guardrail tests cover the output path, the endpoint, the missing `-local-fixture-only` flag and the source scan.

The same failures at the command line, on scratch copies of a real smoke fixture and against a real interrupted full preparation, are in [fixture/negative-cli-transcript.txt](fixture/negative-cli-transcript.txt). Refusals exit 2, differences exit 1, and the untouched control copy exits 0.

## Results

Every fixture below except the interrupted per-write one was prepared offline on the build host and then checked by `verify` with the final verify binary; the reports are in [fixture/](fixture/) and the summary, with hashes, is [fixture-receipt.json](fixture-receipt.json). Sizes are file bytes summed the way the gateway's `inventory` sums them; "allocated" is the disk blocks the files occupy.

| Fixture | Memories | Commit density | Vector width | Commits | `.git` MiB | `.serenity` MiB | Total MiB | Allocated MiB | Largest share of quota | Prepare time |
|---|---:|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Reduced smoke, 10 per brain | 290 | 1 per 500 | 64 | 58 | 1 | 4 | 6 | 15 | 0.2% | 0.2 min (1 worker) |
| History sample, 100 per brain, batched | 2,900 | 1 per 500 | 64 | 58 | 5 | 14 | 24 | 73 | 0.9% | 1.6 min (1 worker) |
| History sample, 100 per brain, per write | 2,900 | 1 per 1 | 64 | 2,929 | 10 | 14 | 29 | 93 | 1.1% | 4.7 min (1 worker) |
| Full, 64-wide vectors, batched | 90,000 | 1 per 500 | 64 | 212 | 137 | 332 | 654 | 1,190 | 7.8% | 12.9 min (2 workers) |
| Full, 64-wide vectors, per write | interrupted after 15 of 29 brains; no verification (see below) | | | | | | | | | |
| Full, 1,536-wide vectors, batched | 90,000 | 1 per 500 | 1536 | 212 | 137 | 1,001 | 1,322 | 1,861 | 15.6% | 20.2 min (2 workers) |

Prepare times come from the preparer's own clock. Several fixtures ran at once on one host, so read them as indicative: the 1,536-wide full run and the reduced fixtures overlapped other preparations. The verifier does not read them.

**The per-write full preparation did not finish.** It stopped after 15 of 29 brains, so no whole-fixture history number exists and no receipt claims one. The record and what was measured on the brains that did finish are below.

### The full fixture

- 14 accounts (1 Scale, 3 Builder, 10 Free), 29 brains and 90,000 canonical memories. Each account holds exactly its plan's memory cap. 90,000 fact chunks and 90,000 vectors sit under the fixture pin.
- The gateway's own `Inventory` and `meter.Entitlement`, run on a scratch copy of the control database, return the verifier's memory count, storage bytes and plan for all 14 accounts.
- **Reproducible identity.** `fixture_content_sha256` is a digest of each brain's sorted fact hashes, in plan order. It is `6470c220e508a760a706d513ce3c95a8e6a69c587edeaac75c052f89e805acd5` for both complete full preparations (64-wide and 1,536-wide, both batched), so the facts do not depend on brain ids, credentials, timestamps or vector width. The independent Python recount reproduces the same digest from the files. The workload hash is `2aa7c9b8e266036c9e28df583e99877fd6936416c3b88723b9c7d367a324d990`, the frozen `workload.json`.
- The full fixture is at its memory cap on every account, so the gateway refuses every further nonreplay remember with `limit_exceeded`. The verifier reports 14 of 14 accounts whose gateway limit inputs refuse a remember. The eligible-versus-boundary question stays open in [decision-request-full-cardinality-eligible-traffic.md](decision-request-full-cardinality-eligible-traffic.md). T23.60 changed no number.

### Storage and history gap

The fixture fills the memory count. It fills no storage quota:

- At 64-wide vectors the largest account uses 7.8% of its plan's storage quota. No account is at or over it.
- **Vector width dominates the derived index.** The same 90,000 facts at 1,536-wide vectors grow `.serenity` from 332 MiB to 1,001 MiB (5.29 bytes per extra float per fact) and the largest share of quota to 15.6%. The provider's width is unknown, so this shows sensitivity only; it does not predict the provider's index size. The canonical facts (`brain`) are 185 MiB in both.
- **Per-write history at scale is measured on 15 of 29 brains only.** The full fixture prepared with one commit per fact (the way production commits after each acknowledged remember) stopped at its 15th brain with `git commit: exit status 128: fatal: could not parse HEAD` (details below). The 15 brains that finished, 66,668 facts (all 10 Scale brains and 5 Builder brains), hold 66,683 commits and 195.2 MB of `.git`, against 150 commits and 105.9 MB for the same brains in the verified batched fixture: 89.3 MB more for 66,533 more commits, or 1,342 bytes per extra commit ([fixture/partial-per-write-history.json](fixture/partial-per-write-history.json), method in `fixture/partial_history_measure.py`). The 100-per-brain sample measured 1,677 bytes per extra commit ([fixture/prediction-per-write-git.txt](fixture/prediction-per-write-git.txt), written before the run, predicted 294 MB of `.git` for the whole fixture as a lower bound). The measured cost per commit is lower, so that prediction did not hold. If the 14 brains that did not finish cost the same per commit, a complete per-write fixture would hold about 264 MB of `.git` (143 MB plus 89,817 extra commits at 1,342 bytes); that is an extrapolation, not a measurement. This is one host's Git and not a production bound.
- **Still unmeasured:** the maximum production history. Production commits after every acknowledged remember and also carries forgets, edits, expiry, real timestamps and repacking that this fixture does not.
- At the 4,096-byte fact cap, fact text is at most 4.096% of each plan's storage quota. The envelope, Git objects and the derived index add to it.

### Client reach

The load client reads one credential per account, and the credential is bound to the account's default brain. Of the 90,000 facts, 25,002 sit on default brains and 64,998 on the 15 other brains, which no request in the current client can reach. They count toward the gateway's per-account memory total. See seam 3 in [integration-request-fixture-service-seams.md](integration-request-fixture-service-seams.md).

### Read path over a prepared brain

Two in-process tests (`readpath_test.go`) drive the real hosted pool over a prepared brain with no listener and no provider. The pool opens the brain under the fixture pin, and the production `recall` tool returns exactly the facts the plan regenerates. The pool refuses the same brain under two other pins with `embed.ErrPinMismatch` and leaves its config unchanged. This is an infrastructure check: the hash embedder carries no semantic quality.

### Resource limits and hazards observed

- **Disk.** The 64-wide full fixture is 654 MiB of file bytes in 181,044 files and 1.16 GiB allocated. Keep it off the internal disk. The retained fixtures together use about 4.5 GiB allocated.
- **Time and workers.** 12.9 minutes at 2 workers for the 64-wide batched full fixture; longer when other preparations overlap. `-workers` accepts 1 to 4.
- **Writer lock retained by a forked Git child.** A second full preparation failed at the index step of brain `builder-0/2` (the log shows brain 16 of 29 finishing beside it) with `another Serenity writer owns this brain` (`writer.ErrBrainOwned`). `writer.AcquireBrain` holds an `flock`; the lock belongs to the open file description, and a Git child that another worker forked keeps a duplicate descriptor until it executes, so an immediate reacquire can fail for a moment. The likely cause is that overlap; no targeted reproduction exists. The preparer now retries for up to 15 seconds at 50 ms and records the count in its manifest (2 retries in the 64-wide batched run, 0 in the wide run). T23.60 changed no runtime code. Whether the hosted runtime can meet the same window on a fast close and reopen of one brain is unverified.
- **Entitlement window.** The subscription rows run from one hour before to 30 days after preparation (`-entitlement-days`). After that the accounts fall back to Free, and `verify` fails `control_db.entitlements_match_plan_and_period`. Prepare again or start inside the window.
- **A lost Git commit object under per-write commits (unexplained).** The per-write full preparation failed on brain `builder-1/1` with 134 reflog entries. `HEAD` points at a commit whose object exists nowhere: the reflog records it, and `git fsck` reports `invalid sha1 pointer`. Its two blobs are still loose files, but its tree objects and the commit object are in no pack and no loose file, and every pack, the multi-pack-index, the ref and the index were last written within one second. The other 15 finished brains are intact, each with exactly one commit per fact plus the baseline. The host's Git (Apple Git-157, 2.54.0) repacks automatically after a few hundred per-write commits with no configuration, so overlapping automatic maintenance runs are the leading suspect. I did not establish that: six parallel scratch repositories with 18,000 per-write commits of the same shape and 0 failures ([fixture/git-race-probe.sh](fixture/git-race-probe.sh)) did not reproduce it, and the hosted runtime's Git version and configuration are unverified. `internal/` sets no `gc` or `maintenance` option. The failed repository and the failure record ([fixture/per-write-run-failure.txt](fixture/per-write-run-failure.txt)) are kept. T23.60 changed no runtime code. Whether production, which commits after every acknowledged remember, can meet the same failure on its host's Git is an open question for the runtime owner.
- **Interrupted run.** A preparation that stops leaves the marker at `status: preparing`. `verify` refuses it with exit 2. The tool never deletes it. Two real interrupted fixtures exist: the first full run (killed with its session) and the per-write run above.
- **Fixture removal.** The tool never deletes a fixture. Whoever prepared it removes it.

### Hashes

| Item | sha256 |
|---|---|
| Prepare binary, source `ba9512e` | `fda5750a9a0728c0e82c441324d7555a291b7e8bc0c2c9571eb6978eba0afba2` |
| Verify binary, source `c23d068` | `362737b932208e2b19aff9c3a53d40b40054b4df7d40454971242745bb13649f` |
| Full fixture content digest | `6470c220e508a760a706d513ce3c95a8e6a69c587edeaac75c052f89e805acd5` |
| Frozen `workload.json` | `2aa7c9b8e266036c9e28df583e99877fd6936416c3b88723b9c7d367a324d990` |

The prepare source is identical at `ba9512e` and `c23d068`; only `verify.go` and tests changed between them. Control database and marker hashes differ per preparation (they hold timestamps and generated ids). Each is in the verification report and in [fixture-receipt.json](fixture-receipt.json).

### Blocked

- **A service run over the fixture.** It needs the embedder seam and three more decisions in [integration-request-fixture-service-seams.md](integration-request-fixture-service-seams.md). Nothing was built or run for it.
- **A complete per-write full fixture.** Not achieved. A retry with default Git settings risks the same unexplained failure (1 of 16 brains failed: one observation, not a rate); a retry with Git's automatic maintenance turned off would measure loose objects, which is not what a production repository holds, so I did neither.
- **Every live-provider row.** `--live` stays unconditionally blocked (exit 2, `calls_used: 0`), and no cost, capacity or provider result is claimed.


## Not established

- Retrieval, provider or semantic quality. The hash embedder is a placeholder for infrastructure.
- Cold-open, recovery-time, load, latency, steady-state or storage-saturation behavior. No service ran.
- The maximum production history size and the storage quota boundary. The fixture commits in batches and fills the memory count only. A complete per-write full fixture does not exist: the attempt stopped at brain 15 of 29 (see the results).
- That the current load client can use the fixture end to end. Four seams are open: [integration-request-fixture-service-seams.md](integration-request-fixture-service-seams.md).

## Reproduce

```sh
python3 -m unittest evals/hosted-load/test_fixture_receipt.py evals/hosted-load/test_fixture_recount.py
go test ./evals/hosted-load/fixtureprep/
python3 evals/hosted-load/fixture_receipt.py --help
```

See `T23.60-offline-fixture-handoff.md` for the exact commands that produced the committed reports and receipt.

A full preparation with batched commits takes 13 to 20 minutes at 2 workers and 1.2 GiB of allocated disk at 64-wide vectors (measured in [fixture-receipt.json](fixture-receipt.json)); one commit per fact takes far longer. Build the fixture outside the repository and off the internal disk.
