# OAuth method scope full local qualification

Exact qualified source: `9948c633a95d7af93e1451656e0cfc21e104241a`. Current Go/module bytes unchanged after documentation cleanup.

| Stage | Result | Exact released build lease |
|---|---|---|
| race | exit 0 | `e6420f998be8c96bc6fcb2d449d29ee8579040f9` |
| vet | exit 0 | `2a0c2dafea2e42d0ebe36cda06860a2e4324e02e` |
| lint | exit 0 | `1253b1f8b0d1f5ed7afa170df90b70c1e372bb41` |
| linux-arm64 | exit 0 | `81747b65b6002b9dc594063b1bee3923509bd243` |

Race passed 3,084 test/subtest results across 84 packages. Nine test/subtest skips and four no-test packages are disclosed below.

- `cmd/serenity (no test files)`.
- `evals/gen/messages (no test files)`.
- `evals/hosted/fixtures/seedbrain (no test files)`.
- `evals/importbudget / TestImportBudget10K`.
- `internal/cli / TestGbrainProtocolConformance`.
- `internal/conformance / TestBootDeliberatelyBrokenBuildFailsDisposeConformance`.
- `internal/hosted/dashboard (no test files)`.
- `internal/hosted/deletion / TestS3Qualification`.
- `internal/hosted/privatefs / TestOwnershipDisabledMountRejected`.
- `internal/hosted/recovery / TestVerifyFilesystemOwnershipRejectsDisabledMount`.
- `internal/import/gbrain / TestImportCrashHelper`.
- `pkg/serenity / TestCheckPlanDriftAgainstDirectionTranscripts/check_plan_with_neither_plan_text_nor_actions_returns_invalid_request`.
- `pkg/serenity / TestCheckPlanDriftAgainstDirectionTranscripts/check_plan_free_text_with_no_classifier_router_configured_returns_status_unverified,_never_a_silent_pass`.

All stages used the fresh-load <=10 shared lease gate and exact finally release, external build caches/artifacts, and private ownership-enabled8GiB APFS test fixture. The source correction changes only shared accounting method scope; it does not change5000/minute policy, cross-process aggregation, provider/live deployment, human risk acceptance or the outstanding17-prefix/refresh/consent hosted tests. GitHub billing-blocked checks are not CI success.
