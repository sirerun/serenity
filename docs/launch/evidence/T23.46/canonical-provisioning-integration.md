# Canonical provisioning correction — integration pending

Source: ed42c1cbce8d6ad3c352659d07a176d50476acbe.

The concurrent allocation/recovery regression failed on main because a ready brain had no Git HEAD. Provisioning now takes brain ownership, initializes a minimal committed `.gitignore` baseline for allocating brains, and only then updates the database to ready. It does not recreate existing ready brains. Interrupted initialization preserves unrelated staged files; a symlinked `.git` is rejected without changing its target or marking ready.

`go test -race -count=1 -json ./internal/hosted/provision`: exit 0, 5 tests passed. `golangci-lint run ./internal/hosted/provision`: exit 0, zero issues. These are package fixtures, not complete identity or hosted recovery qualification.

## Required runtime integration before merge

`pool.open` currently writes its model-pinned config before testing for Git HEAD and commits config only when HEAD is absent. After this baseline exists, first open must still commit its newly created config under runtime ownership, using exact paths and preserving unrelated staged work. The initialization retry must also recover a config written before a failed commit. Do not broadly stage the directory. Preserve embedding-pin validation and avoid embedding calls during provisioning.

Runtime resource acquisition returned LOST with no visible holder. No runtime code was edited. Coordinate that ownership before applying the change. Verify first runtime open leaves canonical config tracked and the tree clean, then create/restore a version-2 backup of both untouched and first-open brains. Existing ready brains missing Git need explicit reconciliation; no legacy repair is authorized by this change.

Additional retry regression: an allocating brain with an unrelated existing commit but no baseline was incorrectly marked ready. The new test failed before the correction and passes afterward. Existing HEAD now requires the exact baseline in both committed content and its regular working file; no silent repair occurs.
