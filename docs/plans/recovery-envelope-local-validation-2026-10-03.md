# Pure envelope local qualification

Exact qualified source `96bd06e5033929a32a9586e7ecf43b1cf28f846f`, author context correction `5ad0e94475634b67cd1f600ad8898d983141841a`. Previous eb10 allocation and42ab cancellation findings remain historical HOLD evidence, not final qualification.

Current full local checks, under fresh load gates and exact shared build leases:

- race: exit 0; real WON lease `4f64816490089e6c1419740c161a2004ca8bd3c8`, exactly released.
- vet: exit 0; real WON lease `50a1c9b7bdd64f0add4e398085cd55349195bf93`, exactly released.
- lint: exit 0; real WON lease `13d6c0343bd9e10a3e04ba18b40087b87cfa7458`, exactly released.
- linux-arm64: exit 0; real WON lease `5ebc2c01cec50479a0191c93d219fb286f801bdc`, exactly released.

Race: 3,082 test/subtest passes across84 packages. Nine test/subtest skips and four no-test packages:

- `cmd/serenity :: no test files`.
- `evals/gen/messages :: no test files`.
- `evals/hosted/fixtures/seedbrain :: no test files`.
- `evals/importbudget :: TestImportBudget10K`.
- `internal/cli :: TestGbrainProtocolConformance`.
- `internal/conformance :: TestBootDeliberatelyBrokenBuildFailsDisposeConformance`.
- `internal/hosted/dashboard :: no test files`.
- `internal/hosted/deletion :: TestS3Qualification`.
- `internal/hosted/privatefs :: TestOwnershipDisabledMountRejected`.
- `internal/hosted/recovery :: TestVerifyFilesystemOwnershipRejectsDisabledMount`.
- `internal/import/gbrain :: TestImportCrashHelper`.
- `pkg/serenity :: TestCheckPlanDriftAgainstDirectionTranscripts/check_plan_with_neither_plan_text_nor_actions_returns_invalid_request`.
- `pkg/serenity :: TestCheckPlanDriftAgainstDirectionTranscripts/check_plan_free_text_with_no_classifier_router_configured_returns_status_unverified,_never_a_silent_pass`.

The owned8GiB APFS fixture verifies UID/mode and ownership enforcement for these local checks only. The early bare single-package authoring test had no runner/lease evidence and is not qualification. All final checks above are runner-backed. Canonical codec and hashes operate on inert caller-supplied projections; they establish no pin, journal ancestry, approval, READY, provider truth or runtime admission authority. Physical quota, capacity, hosted startup and live acceptance remain open. GitHub billing-locked checks do not become CI success.
