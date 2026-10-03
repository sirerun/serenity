# Combined Git confinement and eval caller local validation

Tested source0b8e848edbe73fb2e275c87af687ab8fc515f519 on main4b2fd8e. Integrated eval files are byte-identical independently cleared6f6b388; connector files are byte-identical independently cleareda501ed56. All earlier held candidates, genuine REDs, non-failing attempts and corrections are preserved.

Full Go race:84 passing packages and2980 passing tests/subtests; four no-test packages and seven existing helper/gated skips. Vet, lint and Linux ARM64 CGO-disabled CLI compilation pass. Actual shared leaseada58351d0a19be08fe8da894158832b7081ebf0 was WON, reverified before each stage and exactly released in finally. Earlier attempt lost to a verified active other-project build and launched no full Go command. Every actual stage checked load<=10. Caches/temp/artifacts are on SSD; the reattached owner-enabled8GiB APFS tmp is currentUID0700. That fixture proves ownership mechanics, not physicalquota/capacity.

All three ignored generating commands were separately compiled/tested by author and reviewer, with four genuine baseline-caller mutation REDs total; root explicit-file vet for each passed. Their main functions were not invoked and no generated report/trend/fixture was regenerated. The ordinary full suite excludes those ignored files, so its count does not imply otherwise.

Existing skipped tests: importbudget10K, gbrainprotocolconformance, deliberately broken build helper, liveS3 qualification, importcrash helper, and two unconfigured classifier-router cases. No new source skip introduced.

- `eval-git-confined-full-race.jsonl` SHA256 `06cee54be8f20dac8ec1ef31b6454d0ff742e898ab29ab24eb1dc65f9557137c`
- `eval-git-confined-full-vet.log` SHA256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`
- `eval-git-confined-full-lint.log` SHA256 `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47`
- `eval-git-confined-full-linux-arm64-build.log` SHA256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`
- `calibration-explicit-vet.log` SHA256 `e3daa9608106786cf9b5a13c7e316ea18fbb7bf0d9a4c660349a11903317cbbe`
- `gen-trend-explicit-vet.log` SHA256 `e3daa9608106786cf9b5a13c7e316ea18fbb7bf0d9a4c660349a11903317cbbe`
- `publish-trend-explicit-vet.log` SHA256 `fe6d77ac544f3b60fdd87497a7626bb624302d923dded65d7ebe4e23d00385d9`

Full T24.30 whole-module scanner acceptance, current hosted registry task acceptance, provider/live qualification, recovery factory, physical storage and launch remain open. No deployment/provider/purge/spend action. Ajent MCP unavailable; current root feed and coordinator board checked.
