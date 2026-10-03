# Whole-module Git drift integration qualification

Tested source `78f0f6538025cce640617de04cd67edc94628c7f` on qualified main `8081632ac88bddf68e4e7f73cd670a8b5434bc5b`. Scanner bytes match independently cleared corrected author `48abce76e7c513df3014a0d5a638395977c076ae` exactly (SHA256 `aa4619da8b30c9ccefe14117d581c288c74bdf446ccd8380d1ce84203bc59347`). Initial04 scanner HOLD is preserved: function-wide declarations hid real calls and reported local function-literal parameters. Callsite identifier binding correction passes seven independently reproduced lexical controls.

Coordinator package race passed (17 passing test/subtest entries), parsing351 non-testGo files across the module including ignored command/platform files. Full `go vet ./...`, changed-package lint (zero issues), `gofmt -l ./internal ./cmd ./pkg` (empty) and diff-check passed. Actual shared build lease `0c58f2be7b5bb72d6eab7419ada881270dabbe9c` was WON, ownership verified before every stage and exactly released. Every stage began with fresh one-minute load <=10. Go caches/tmp on external SSD; runtime temp on verified private ownership fixture.

All productionGo and module files match qualified PR349 main byte-for-byte. Only new scanner tests and documentation change here; no new full-module race or Linux build is claimed. PR348 full qualification remains separately recorded.

Direct literal os/exec Git enforcement includes default/alias/dot imports and lexical shadows; computed executables, wrappers, shell strings and other process APIs are outside this parser check. Independent actual-source inventory is recorded in the adjacent review. Repository metadata and external dependency directories only are skipped; exact internal/gitrun subtree is allowed. No eval/hosted/pkg/cmd exclusion.

This is source-only enforcement, not T24.30 acceptance while its upstream hosted registry gates remain open. No provider/deployment/purge/spend or launch acceptance. Ajent tooling unavailable; actual rootfeed/board and fresh PR discussions required before normal ADR024 expected-head merge. External bundle `serenity-gitrun-module-drift-final-validation-20261003`.

- `package-race.log`: exit0; load6.26; SHA256 `ccdaf35eed73b48419e7610c06c792ece2d092961cd7e78c0189d9395b33f128`.
- `full-vet.log`: exit0; load5.89; SHA256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.
- `package-lint.log`: exit0; load5.82; SHA256 `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47`.
- `whole-source-gofmt.log`: exit0; load5.82; SHA256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.
- `diff-check.log`: exit0; load5.82; SHA256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.
