# Current pin-owner focused qualification

Author source `a3526dc86d7f65054c04f044aa4c91be98423c88`, tree `7570c453b512bd9c5b019d0a8b24876a60902baa`, whole Go fingerprint `bc546e3408a058981655d03f7d63e107d41749a645576a16d00b95ddd0c5bbf0` remained clean and unchanged during each baseline command. All six owner source files match the current integration source; the author checkout does not qualify the newer combined producer source.

Package, focused race, vet and lint passed with exact build leases `07914ef1f2231d42dbdeec139c338b5e0f272425`, `267ae3690aade0c08e7398f62df9fbaaff299ce3`, `10f9815debb29a06f7c169e633c5bc88faa4ba62` and `12d43c2e5740e30eb0a1c871494a3d56f10e6f3f`. Each command and exact release exited 0.

Three intentional compiled mutants failed their targeted assertions:

- Absence-proof consumption bypass accepted the zero/unconsumed proof; lease `18d39e6f1081318d4ef17bc553f974fc982d2836`.
- Active superblock validation bypass returned the wrong refusal classification for future-version and changed-StoreID cases; lease `2b3bcac6cb737c74a250d782d4570f157fc108ae`.
- Bootstrap guard bypass recreated the missing bound lock; lease `b8e5db4d911911b8d82bcdc6ad49be6d3aefbd34`.

Each mutant command exited 1 at a test assertion, each exact release exited 0, and original source bytes were restored after each. The final combined restored targeted run passed with lease `9b115a821582f6e1d0288bf1ce1206f8ada7a76b`, command exit 0, release exit 0 and the original fingerprint. The coordinator inspected the stage records and assertion output directly.

An earlier wrapper setup failure had no command start and no retained exception output; it is excluded from test and negative-control evidence. Admission holds and lost claims are also excluded. Raw stage JSON, mutant diffs, command output and exact release logs remain in the external evidence archive. Combined producer runtime qualification, full-module checks, final independent exact-head review and PR #358 merge remain open.
