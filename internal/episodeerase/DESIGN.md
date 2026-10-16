# Isolated episode erasure candidate (T1.12)

Contract: Build accepts an exclusively held, clean, standalone episode-only Git
repository and an explicit complete current fence snapshot from the caller's
trusted authority. It constructs a fresh bare candidate; it never activates it,
rewrites the source, changes capture capabilities or emits physical completion.
The caller owns locking, authentication, freshness/completeness of external
fences and eventual integration. A local snapshot is not fence authority.

Qualification is deliberately narrow: SHA1 Git, one refs/heads/main branch,
linear bounded history, regular canonical episode source pairs only. Every
historical snapshot is validated by existing SourceStore/LoadEpisodes. Unknown
canonical files (including extracted facts, indexes or other derivatives),
foreign brains, symlinks, alternate object stores, shallow/partial repositories,
extra refs, dirty/untracked files and unreachable objects fail closed. Nothing
unknown is silently discarded. Existing commit messages/authors are replaced
with fixed metadata because they can contain deleted text.

Closure removes each fenced conversation's append/re-import/checkpoint sources
and checkpoints referencing its events, preserving unrelated sources byte for
byte. Inline attachments in surviving sources remain, even if independently
shared with deleted events: this is reference deletion, not content-wide purge.
Current content-free exclusion records replace historical fences in every
candidate revision, including snapshots taken before deletion. Missing or stale
fence snapshots fail closed. External journal completeness cannot be proven here.

Candidate trees contain only surviving canonical pairs and current fences; no
index/cache is copied. All Git objects are newly constructed; no clone, bundle,
alternates, old commit metadata, source reflogs or old object packs are copied.
Candidate verification enumerates every object, retained revision/ref and
reflog; removed source blobs must not survive unless independently retained as
identical canonical metadata. Arbitrary substrings also present in independent
surviving evidence are not promised absent.

Limits: 128 commits, 4096 files per snapshot, 64 MiB per Git response, 256 MiB
source filesystem, 65536 filesystem entries, existing episode archive limits.
Context cancellation stops Git and bounded loops. Build uses a fresh private
stage on every attempt; failure removes only that owned stage. Crash leftovers
are inert private stages, never resumed as trusted candidates. Retry identity
binds source HEAD and canonical current fences. Successful candidates are
synced and atomically renamed with a content-free manifest; caller must use a
fresh absent destination. No destructive cleanup of previous attempts occurs.

Unsupported: mixed ordinary brains, merges/multiple branches/tags, live mutation,
activation, external fence persistence/replay compatibility, backup/export/device
purge, queue reconciliation, extracting/rebuilding independent-support facts,
physical disk sanitization, arbitrary content-wide removal. Parent must qualify
checks on DGX and independently review before integration. T1.3 remains open.

Implementation detail: canonical record JSON and metadata must exactly match
PutEpisode encoding; unknown fields or edited metadata are not treated as safe
independent evidence. The small store helper validates this without changing
ordinary decoding. Aggregate Git stdout is capped at 256 MiB per build, in
addition to per-command and object-store bounds. Large repeated snapshots can
therefore fail below the commit-count limit. No network access is attempted.

Retry recovery restarts bounded construction from source plus the supplied
current fence snapshot; it does not resume halfway through a history rewrite.
Only final success has a durable synced manifest. A crash after rename but before
parent-directory fsync has an unknown publication outcome; callers inspect and
independently verify that inactive candidate or choose a fresh destination.
Existing destinations are always refused, never adopted based on a manifest.
Abandoned stages may contain source bytes and require parent-owned cleanup;
they are residual controlled copies, not a completed-purge claim.
