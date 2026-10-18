# Pinned mixed-source erasure preparation — T5.48–T5.52

Base: `499891236b5541092a61213febcb7550805a4230`.
Implementation snapshot: `fbaa1e626f285c13142232fcc8e061c6ac4405e7`.
Source archive SHA256: `e4be980a6c2a5331d56bef9d005ce1eef1a50162d6a5c1792191616fa4a3cd3c`.

## Delivered boundary

`episodeerase.BuildMixed` adds inactive preparation for explicitly pinned
independent raw file/email/voice pairs across the entire qualified linear Git
history. Pins bind data and metadata and assert caller-trusted independent
retention; they do not authenticate provenance or fence freshness. Unknown
fields/dependencies, unsupported source kinds, unlisted/unused/conflicting pins
and changed metadata refuse. Each surviving pair retains its exact historical
bytes. Existing episode/checkpoint exclusion closure and object verification are
shared. The legacy episode-only entry point and manifest stay compatible.

The mixed manifest has a separate schema and binds sorted retained identities
into retry identity. No active pointer, production caller, captured history,
managed-copy cleanup or physical-purge receipt is added. T5.2, T5.3 and T1.3
remain open. Hosted config/derivatives and external history authority are still
unsupported in this preparer. Failure stages remain controlled sensitive copies
until their owning cleanup; this is not a terminal erasure promise.

## Execution and evidence

Current attended engineering executor; one isolated implementation member under
the default batch ceiling3, one inherited implementer and a separate independent
reviewer. Model/reasoning/account selection is inherited; no alternate provider
or paid capacity is introduced. Token/cost telemetry is unavailable; no savings
or throughput comparison is claimed. Source work began after the full144-task
Wazi reader/schema/semantic gate passed. Primary dirty work and foreign shared
memory/hosted deletion/backup claims are preserved.

Checks use one existing Linux ARM64 runner, cached Go1.26.9, GOMAXPROCS2,
serial package work and explicit process/test deadlines. The transferred archive
hash matches. Temporary storage is owned mode0700 under umask077. This qualifies
source behavior only; it is neither native-host acceptance nor production data.

Verification, review and landing results are recorded in the PR and appended
below when actually observed. GitHub Actions remains separately evaluated under
ADR024; local fallback never means hosted CI passed. Ajent MCP is unavailable;
repository coordination and GitHub are checked without inferring a hold lift.

Observed source checks at fbaa1e626f285c13142232fcc8e061c6ac4405e7:

- `go test -race -count=1 -p=1 -timeout=240s ./internal/episodeerase ./internal/store ./internal/gitrun`: pass (6.914s,13.187s,2.087s package times).
- `go test -race -count=1 -p=1 -timeout=240s ./internal/hosted/gateway -run '^TestEpisode(ErasureChain|PortableRestore)'`: pass (13.725s).
- `go vet -p=1 ./internal/episodeerase ./internal/store ./internal/gitrun ./internal/hosted/gateway`: pass.
- `golangci-lint run --concurrency 2 ./internal/episodeerase`: zero issues.
- `govulncheck ./internal/episodeerase`: no vulnerabilities found.
- Formatting and `git diff --check`: pass.

The runner has300second race-command,180second vet/lint and120second vulnerability
deadlines. No full repository suite was run; aggregate evidence covers changed
code, direct dependencies and the existing integration consumers. All139 prior
plan rows were compared byte-for-byte before the two new completion updates.
