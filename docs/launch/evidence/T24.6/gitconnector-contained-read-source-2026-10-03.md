# Git connector confined-read source receipt

Baseline main4b2fd8e. Existing pathname Lstat/resolve/containment checks are preserved for static diagnostics. The actual read now opens through an anchored os.Root and checks the opened descriptor is regular. Linux/Darwin add nonblocking and no-follow flags, refusing raced-in FIFO/leaf symlink; other platforms retain read-only flags and do not inherit that additional qualification. Go Root confinement cannot prohibit mounts or hard links and JS is not a race-resistant confinement target; no broader platform qualification is claimed. Root and file close errors are returned instead of success. No Git runner, config, module or caller routing changed.

Genuine pre-fix test evidence uses only owned temp repositories and an outside sentinel file. First single-run attempt passed (not a RED). Repeating unchanged-source test50 times produced10 actual outside-byte failures, including at reads1/2/3; this is the genuine RED. Source fix then passed race-enabled repeat50. Two close-check lint findings were corrected afterward; final entire connector package race/vet/lint pass. Independent review and combined qualification remain required before merge.

External logs under /Volumes/BuildOffload/serenity-gitconnector-contained-read-validation-20261002:

- `leaf-swap-original-red.log` SHA256 `11d395936bd27e2034ec7ab6005bd08b42db5dbb7f54e54c6f73109675c49ad1`
- `leaf-swap-original-repeat50.log` SHA256 `2cbc22865057289a2b8915d92f031fbc5e593b81064954c23d94e55dd5dc5c34`
- `leaf-swap-confined-race-repeat50.log` SHA256 `3df31b59896b5f23ee960cc4610765ceb3cd1199905bdc140265577122648ceb`
- `package-lint.log` SHA256 `af65e53636a2b2cacc8bdb12cb91705de9219eee7a1dab14b3c7474d44a32882`
- `final-package-race.log` SHA256 `b1ac0f9872da054bf91d85b29be21d6ad1a1971aeeec2794e6d637eda7a85a10`
- `final-package-vet.log` SHA256 `b9bdb22f29e89fe6d61fbfb5de5dee3b0a9f6ac6e04b69843e25196bf23a8b3f`
- `final-package-lint.log` SHA256 `aee9c9360835be9e0719e7f83cf58b8b6470edcbc99b15a773d2e0ef8a2a39b3`

Canonical source claim R-gitconnector-contained-read64adbe95ba87f9271de271fc4dcde17be50a81de remains held. No live host/provider/deploy/purge/spend action or full security/launch acceptance.
