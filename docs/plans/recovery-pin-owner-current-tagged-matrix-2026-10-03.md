# Current combined tagged crash qualification

Clean producer author `c75452a45858b39193655c6191924b8cc3ad33df` passed `go test -race -tags=hostedtest -count=1` for the backup and recovery packages. The Go fingerprint `bfd24dc7218d34ae19e9468b5a528efc4cdf3d2741141cc9f6343d11b3862600` exactly matches integration `e0508c13276f6d56bd0731ebfc4ddba5518456ad`; the coordinator verified that equality and inspected the durable stage record and package results. The actual lease `f449a5442db074b1bc3c26928a8bd1a05eaa1ed5` was released with exit 0; command exit was 0 and source was unchanged throughout the command.

The current test source contains 40 real phase cases plus five actual canonical partial-temp fixtures (45 total), along with the captured partial-artifact release restart and real owner/producer pair regressions. Captured directory, temp and store identities are checked in their correct scopes. Retained-tree comparisons include device/inode, mode, inventory and bytes. A prepublication pending owner with a STAGED lease is explicitly an unchanged incomplete boundary, not successful pin completion. Ordinary production capture remains a no-op; these tests do not prove power loss or every unlink order inside the unchanged standard-library RemoveAll call.

Earlier compile, digest, identity and pipe-lifetime fixture failures are retained in the evidence archive and excluded from passing or negative-control evidence. Current producer controls and tagged vet/lint passed as recorded below; full-module checks and final exact-head independent source review remain pending. PR #358 remains on coordinator merge hold.

Current tagged vet and lint also passed with exact released leases f4bd550587b07cf031664da601ccddc1b04b3fdb and 8bef454f90c0d70e49ff27bd8af61b8073360ff4. Earlier f7 tagged passes are historical after mechanical Go lint corrections; these current-byte stages supersede them.

Four current-source compiled mutants exited 1 at their intended assertions; each exact release exited 0, source remained unchanged during each command, and each mutation was restored:

- Bypassing fresh release authorization let PinKeep authorize partial deletion. Lease `96c7c825b7de5acd232862e146bb0055a64cad49`.
- Checking only inode after acknowledgement accepted mutated receipt content. Lease `0b2192b1393fc5e2c2dc8266883cbfd22e9abe9d`.
- Removing the terminal receipt caused a real owner/producer reopened reconciliation conflict. Lease `aa659f5b9e9f4b3a990c2264ac8840d1ae4781a2`.
- Bypassing the pre-owner capacity reservation let release return success where the valid budget-gap fixture required refusal. Lease `a558a6b4e8ec90e4f7d875394b45720616904d7c`.

After all four mutations were restored, current-tagged-race-17 repeated the full tagged backup/recovery race run successfully on the original fingerprint. The coordinator inspected the stage records and assertion output. No compile/setup failure or lost claim is counted as a negative control.

Currentness notice: the c754 focused checks above are now historical after the reviewer-required tombstone absence assertion and duplicate phase-inventory test correction. Author f1d9/root ad3d129 require fresh qualification; the first root full-module race failed on the duplicate phase-list entry. No full-module or final review acceptance is claimed.
