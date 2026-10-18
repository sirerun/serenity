# Hosted canonical erasure preparation — T5.58–T5.62

Base7fe9b46375f24bfbacb74b9aaf72de6f9382e6a0; tested source
3813cdbe5cc602b3ba8242be5a60f1ee04fc72b2, archive SHA256
0673c2a5287f7abf844267354b61933b6ad77f5e563cadb6de8d3eb53ef23082
matched after transfer.

BuildHosted is additive inactive preparation. Exact data/metadata raw pins and
canonical path/blob pins assert caller-trusted independent retention. Baseline
.gitignore is exact; generated serenity.yml is preserved, and HEAD must contain
config. Unknown paths/sources or changed/unused/incomplete pins refuse.
Entities/claims/.dira files are preserved only under explicit qualified pins,
not automatically deemed independent derivatives. Reserved episode paths cannot
be relabeled. Legacy Build/BuildMixed remain strict.

Only bounded safe ignored root .serenity material is allowed and never copied.
All candidate objects are freshly constructed with fixed metadata, shared episode
exclusion/checkpoint closure and explicit inactive/unsupported-purge status.
No active pointer, authenticated fence or managed-copy cleanup is added.

Initial tests exposed ignored-file status records versus directory shorthand;
strict NUL-delimited parsing corrected that mismatch. Initial lint found import
grouping, two Boolean simplifications and an unused private wrapper; corrected.
Independent HOST-Q1 required actual runtime initialization instead of synthetic
config. The revised fixture exercises Provisioner, real pool initialization and
close, pins generated config, and verifies original runtime files/stable lock
remain while absent from candidate trees.

On existing Linux ARM64 Go1.26.9, GOMAXPROCS2 and serial bounded commands:

- `go test -race -count=1 -p=1 -timeout=240s ./internal/episodeerase`: pass8.739s.
- `go vet -p=1 ./internal/episodeerase`: pass.
- `golangci-lint run --concurrency 2 ./internal/episodeerase`: zero issues.
- `govulncheck ./internal/episodeerase`: none found.
- Formatting and diff checks pass.

Private owned temporary storage and immutable source archive; no secrets or
provider data transferred. Full repo/native/live/physical-power-loss evidence
is not claimed. Current attended inherited selection, one implementation member
under ceiling3, independent reviewer separate; token/cost telemetry unavailable.
Full parent T5.2/T5.3/T1.3 remain open. Exact review/merge/landing results go in
the PR. Ajent MCP unavailable; owner-authorized source scope preserves other
claims and production gates. Actions fallback follows ADR024 if billing blocks.
