# Canonical provisioning correction — integration pending

Source: d6d37b5a48911cbe10eab24f6812c2429d04f212.

The concurrent allocation/recovery regression failed on main because a ready brain had no Git HEAD. Provisioning now takes brain ownership, initializes a minimal committed `.gitignore` baseline for allocating brains, and only then updates the database to ready. It does not recreate existing ready brains. Interrupted initialization preserves unrelated staged files; a symlinked `.git` is rejected without changing its target or marking ready.

`go test -race -count=1 -json ./internal/hosted/provision`: exit 0, 4 tests passed. `golangci-lint run ./internal/hosted/provision`: exit 0, zero issues. These are package fixtures, not complete identity or hosted recovery qualification.

## Required runtime integration before merge

`pool.open` currently writes its model-pinned config before testing for Git HEAD and commits config only when HEAD is absent. After this baseline exists, first open must still commit its newly created config under runtime ownership, using exact paths and preserving unrelated staged work. The initialization retry must also recover a config written before a failed commit. Do not broadly stage the directory. Preserve embedding-pin validation and avoid embedding calls during provisioning.

Runtime resource acquisition returned LOST with no visible holder. No runtime code was edited. Coordinate that ownership before applying the change. Verify first runtime open leaves canonical config tracked and the tree clean, then create/restore a version-2 backup of both untouched and first-open brains. Existing ready brains missing Git need explicit reconciliation; no legacy repair is authorized by this change.
