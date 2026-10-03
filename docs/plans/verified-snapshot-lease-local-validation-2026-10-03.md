# Verified snapshot producer local validation receipt

Exact qualified source: `f4de27e029d88fa1e7bdd61b5e140524ce7a6154`. Author implementation `c48b09bf6614f917cb16d3659c241f788f6446de`, cleanup correction `454af2028889a567e0a05c38b62765a6cdfc5846`. Historical `e67b5fb8f90281928627d98993dee980143681d7` passed checks but independent HOLD retained verification repositories; it is not the final qualification head.

All current full checks ran under fresh load gates and real shared lease wins, with exact compare-and-swap releases:

- race: exit 0; lease `a8ac5e0ded2e071d2236ce91f4bc3d2a171669d2`.
- vet: exit 0; lease `19f65efa9700746b3193047b280ed32eefc411d5`.
- lint: exit 0; lease `2ae725eca9212872aac8dca9f416dd7ead7fca72`.
- linux-arm64: exit 0; lease `4b8f5bc44de522c27c9fa68612e5a6e006744946`.

Full race: **3,047 test/subtest passes across 84 packages**. Nine test/subtest skips and four packages without tests:

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

The explicit owned APFS fixture establishes filesystem permissions for local tests only. Raw lease artifact/metadata/copy accounting is cooperative and bounded by the implemented options; transient Git expansion, destination staging/output, index/vector/database/WAL growth, physical quota and 500 GB acceptance remain open. No provider, approval, READY store, runtime startup/admission, CI or hosted acceptance is claimed.

The first cleanup mutation failed to compile and is not behavioral RED. A second author mutation also failed to compile and is not RED; one LOST claim ran no command. Corrected author cleanup mutation ran under lease `31f7c14ac302437e1d897553c427918156732672` and failed the retained-directory assertion, followed by restored PASS under `a98dce5438577403a63ce10d72598171d18da6e8`. Independent compiled RED and restored PASS are banked in the exact-source CLEAR review. All these executed controls have exact releases.
