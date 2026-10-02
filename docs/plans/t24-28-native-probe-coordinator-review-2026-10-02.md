# T24.28 native keychain probe coordinator review

Scope: scratch runtime feasibility only, not production ACL remediation or task acceptance. Coordinator independently read the complete controlled log, checked the binary/log/ledger hashes below, and verified the exact fixture directory was absent after the child completed. Source baseline for the intended production lane remains c3d491b03980310d7d59688d092b8119640e5250.

The earlier scratch Add omitted the disposable-keychain selector; all earlier Add/read/update results with unproven provenance remain invalid. No generic service-name cleanup is authorized. A corrected intermediate probe proved fixture Add provenance but read returned item-not-found and no Update occurred; this was not success.

The final controlled probe explicitly selected the unique owned file-based keychain on Add, independently obtained its item reference and actual keychain path, and restricted matching/update queries to that keychain. Process-wide interaction was disabled and verified before native operations. The fixture/account ledger was created exclusively and fsynced before any Security operation. Results: Add, item provenance lookup, initial read, data-only Update, and post-update read each returned status zero; initial value matched, and after Update the old value did not match while the new value did. The owned child exited zero within the 15-second bound and removed its exact created fixture. No existing credential was intentionally inspected or changed by this controlled probe.

This proves the combined explicit-selector and process-wide no-UI scratch path works for its newly created item. It does not isolate one cause of prior failures. The Apple selector distinction is documented in [TN3137](https://developer.apple.com/documentation/Technotes/tn3137-on-mac-keychains). Runtime evidence is specific to this fixture.

Checked artifacts in external validation storage:

- t24-28-native-update-no-ui-searchlist.log: SHA-256 `f56678aebff0f34e0b2e9e391ba9e0878a30a553b6ca827b0dc67913cb7ee96e`.
- t24-28-owned-fixture-ledger-20261001-searchlist.log: SHA-256 `f03a008b35395fce2def46986b0169f55aeb23dbde764fa44c523f094ffcd634`.
- t24-28-native-probe-searchlist-no-ui: SHA-256 `85b0f85d2a2746053aae39df38355552618bab29cf0e32458cd35025fee8d074`.

Scratch source hash at that run: 4ebfb0c30e1e186c728d97fe61eb9d285cf68025f19d51b17a6ff7140ab9358d. Exact owned fixture basename: t24-28-keychain-697722971. Logs record generated fixture identity and boolean matches, not existing credential contents.

Remaining before production implementation: actual CLI-created legacy-item compatibility without UI or resets; ACL inspection proving the intended canonical executable; denial of a separately compiled reader with distinct code identity; no-CGO shipping compatibility; a genuine regression RED on unchanged production code; corrected threat model; independent source review and required Go gates. Preserve existing daemon/profile APIs, Linux behavior and all existing user credentials. No production source, deployment, token migration or task acceptance follows from this scratch review.
