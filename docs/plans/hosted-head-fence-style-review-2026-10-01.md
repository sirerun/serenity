# Supplemental review: style-only cleanup

Reviewed `78936a19f364c0272470a6c22c82d513b74f9961` against `d719347`. The two-file diff contains no behavioral change: the object-ID character predicate is De Morgan-equivalent (`!(digit || lowercase-hex)` becomes `not-digit && not-lowercase-hex`), and removal of the redundant `releaseFence` reassignment leaves the existing deferred release intact. The release closure returned by `enterExclusive` is guarded by `sync.Once`, so the explicit release followed by deferred release remains safe.

`git diff --check d719347..78936a19f364c0272470a6c22c82d513b74f9961` passed. No test/build was run for this style-only change; root's guarded validation was already in progress.
