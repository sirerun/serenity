# T23.52 manifest-v2 final-source review — bounded clearance with publication hold

Reviewed exact source `0027e2290ebe2802b27ea62476366175ac6944fc` in isolated external-SSD full clone `/Volumes/BuildOffload/worktrees/serenity-manifest-v2-review-0027e2-20261001` (created with `--no-hardlinks`, repacked reachable objects, and removed inherited alternates). The source was clean before temporary mutation controls.

## Result

The exact-key parser follow-up closes the case-folding gap found at `0c8e7816d3f420d9c820edee47b6c0542c134510`. Its schema-aware token walk whitelists exact field names at each manifest object level. The new parser regression tests cover aliases alone and alongside canonical names for root version, source build SHA, and watermark hash; unknown fields, literal duplicate keys, trailing JSON values, oversized manifests, and excessive nesting are also rejected.

The latest commit still has a publication no-overwrite limitation under concurrent destination creation. Create and Restore perform `os.Lstat(destination)` immediately before `os.Rename(staging, destination)`. The outer check rejects destinations already present, and a temporary interleaving injected immediately before the `os.Rename` call observed an `EEXIST` refusal on this macOS host. That does not prove atomic no-replace: cached Go 1.26.2 Unix `os.rename` source (`src/os/file_unix.go:26-48`) itself does `Lstat(newname)` and then calls `syscall.Rename`; a competing process can create an empty directory after Go's internal Lstat and before the syscall. Linux `rename(2)` may replace an empty destination directory in that interval. I found Go 1.26.2 source in the shared module cache, but no Go 1.26.5 source; current Go 1.27.1 has the same wrapper pattern. Treat concurrent destination creation as a remaining no-overwrite gap unless exclusive parent ownership is guaranteed or publication uses an atomic no-replace primitive with a fail-closed fallback.

## Validation

- `go test -race -p 1 ./internal/hosted/backup` — PASS, 19.379s; before the run load averages were 2.72/4.29/4.97. External Go caches and temp paths were used.
- Focused parser, size/depth, tampered control checksum, and existing-destination tests — PASS, 0.777s after all temporary source mutations were restored.
- Case-alias removal control — RED as intended: temporarily disabling exact-key enforcement caused parser alias subtests for root version, source build SHA, and watermark hash to fail. The original condition was restored.
- Checksum-guard removal control — RED as intended: tampered-control test returned nil when digest enforcement was disabled. Guard restored.
- Both Restore destination checks removal control — RED as intended: the existing-destination test observed publication over an existing empty destination. Checks restored.
- Temporary interleaving hook immediately before `os.Rename` created an empty destination after the package's final `Lstat`; `os.Rename` returned `file exists` on this macOS host. Hook and test were removed. This did not exercise Go's internal Lstat-to-syscall window and is not evidence of atomic no-replace.
- `gofmt -d` on the production and test files returned no differences. Temporary changes were removed; `git diff --check` and source status were clean before this receipt.
- No cloud/provider calls were made.

## Scope limits

The package parser and tested barriers pass at this pinned source, but this is not full T23.52 acceptance. The concurrent destination no-overwrite guarantee remains unresolved across supported platforms. No production adapters, IAM/S3, scheduler/uploader wiring, or production restore acceptance were evaluated.
