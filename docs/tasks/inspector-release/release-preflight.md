# Inspector and brand release preflight

Checked 2026-10-04. Production release remains blocked; no deployment, restart, resource provisioning or billing change was performed. Detailed provider observations remain in the isolated operator receipt; this public record contains source and release boundaries only.

## Serving and packaging contract

The existing hosted binary embeds `site/` and serves the public website, dashboard and login routes. GitHub Pages is a mirror; a Pages-only deployment cannot update the existing hosted application. Use the existing hosted packaging/deployment path after qualification, without a hosting migration.

## Artifact and deployment gates

`deploy/hosted/deploy.sh` requires archive checksum validation and a Sigstore bundle verified with Cosign against the exact tagged release workflow identity. The inspected candidate releases lack qualifying bundles, and recent release/website workflows failed before executing steps. The existing runtime verifier also requires qualification. No unsigned manual promotion, alternate trust identity or signature bypass is approved.

The installer retains a previous binary and attempts binary rollback on a failed readiness check. This does not establish a data/schema rollback or a completed live version-pair rehearsal. Exact artifact identity, runtime verifier, backup/rollback acceptance and dedicated non-customer canary checks remain release dependencies.

## Current-source boundary

The previously installed candidate source is `e98c382`; planning baseline `e2b5dd17` is42 commits ahead. The baseline carries intervening hosted/recovery/backup changes as well as this feature. The final-source Go/module tree matches the landed PR358 source `bc468943`, whose review and local qualification remain explicitly bounded: they do not establish production READY, startup factory/epoch authority, provider/runtime acceptance or physical capacity. Trusted recovery/startup authority assignments and exact runtime acceptance remain unresolved. Preserve those holds; do not cherry-pick this feature onto the older candidate to evade qualification.

The user's existing-host read-only feature release is explicitly authorized. Broad paid launch, billing cutover and new spend are separate scopes and are not inferred. No specific trusted hold against this narrow feature patch was found; that does not waive its runtime, signed-artifact or deployment gates. Ajent MCP tooling is unavailable; source coordination records were checked.

## Required performance evidence

The read projection has per-file/tree/node limits but allows approximately224 MiB of configured raw input across source bodies/metadata/claims/entities per read, with four concurrent read slots. This is not evidence of an OOM. Representative10K-record and high-water/concurrent-read peak heap and latency measurements are required before production qualification; tighten aggregate budget/admission where measurements require it. Rendering, loaded-count and response-size budgets remain frontend verification dependencies.

No source merge, build success, signed artifact or live inspector acceptance is established by this preflight.
