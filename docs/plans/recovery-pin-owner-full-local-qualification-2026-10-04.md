# Combined pin-owner full local qualification

Qualified clean source head `c916bcd7ed30592efb065c876455d1bb51509523`, tree `0b026e37226fd8295f2ca1a350a4f5072816a336`, Go fingerprint `5176070520a9496599cbf0aabe44cfcc2daae34fa26681c13b231f6c4d1c0425`. Source was unchanged during every command. Current tagged regressions and all five compiled controls are recorded separately in the current tagged matrix.

| Stage | Result | Exact released lease |
| --- | --- | --- |
| race | PASS; 3160 test/subtest events, 84 passing packages | `200ba2c545278ab9a6c36955f18d3dbcde974acb` |
| vet | PASS | `69ad696b4bc49b443ea2c8e3cbf4843df36ef346` |
| lint | PASS | `a8177d2a516feff36bb26945f78b1a60afab8d5b` |
| linux-arm64 | PASS | `d6c13a1126b421d92b31e732bd2e098815c6cc39` |

All four commands and exact compare-and-swap releases exited 0. Race duration was 360.74 seconds. Vet admission initially held because load exceeded 10; it ran only after fresh admissible load and a genuine lease win. That resource hold is not a test failure. Evidence archive: `serenity-recovery-pin-owner-final-validation-v2-20261003`; results record SHA-256 `7a5ab72977d7c2e549925fe0d380c03edec13064ae70cdbf6f850f76f15059c4`.

Nine actual test/subtest skips and four packages with no tests were disclosed; they are separate categories:

- No tests: `github.com/sirerun/serenity/cmd/serenity`.
- No tests: `github.com/sirerun/serenity/evals/gen/messages`.
- No tests: `github.com/sirerun/serenity/evals/hosted/fixtures/seedbrain`.
- Test/subtest: `github.com/sirerun/serenity/evals/importbudget / TestImportBudget10K`.
- Test/subtest: `github.com/sirerun/serenity/internal/cli / TestGbrainProtocolConformance`.
- Test/subtest: `github.com/sirerun/serenity/internal/conformance / TestBootDeliberatelyBrokenBuildFailsDisposeConformance`.
- No tests: `github.com/sirerun/serenity/internal/hosted/dashboard`.
- Test/subtest: `github.com/sirerun/serenity/internal/hosted/deletion / TestS3Qualification`.
- Test/subtest: `github.com/sirerun/serenity/internal/hosted/privatefs / TestOwnershipDisabledMountRejected`.
- Test/subtest: `github.com/sirerun/serenity/internal/hosted/recovery / TestVerifyFilesystemOwnershipRejectsDisabledMount`.
- Test/subtest: `github.com/sirerun/serenity/internal/import/gbrain / TestImportCrashHelper`.
- Test/subtest: `github.com/sirerun/serenity/pkg/serenity / TestCheckPlanDriftAgainstDirectionTranscripts/check_plan_with_neither_plan_text_nor_actions_returns_invalid_request`.
- Test/subtest: `github.com/sirerun/serenity/pkg/serenity / TestCheckPlanDriftAgainstDirectionTranscripts/check_plan_free_text_with_no_classifier_router_configured_returns_status_unverified,_never_a_silent_pass`.

The local APFS fixture is 8 GiB with ownership enabled; it is not 500 GB physical-storage qualification. Hosted S3 qualification, provider actions, deployment, production startup factory, full READY/epoch owner, approval authority and physical destination quota remain open. GitHub Actions did not execute due the account billing lock; founder-authorized ADR024 local checks substitute for this delivery only.

The historical first full snapshot at 1c9 failed the duplicated phase-list entry (3159 passing events, 83 passing packages, exact lease released). The fixed current snapshot above supersedes it; no historical compile/setup failure is negative-control evidence. Final independent exact-head review, normal merge and landed equality remain required.
