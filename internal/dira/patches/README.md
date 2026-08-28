# Local patches

No fork-and-edit: a vendored file under `internal/dira/` is never hand-edited
in place. If a local change to dira's schema, ledger codec, or frontmatter
splitter is ever needed, it lands as a `.patch` file here (produced by `git
diff` against the unpatched vendored tree) and `scripts/update-dira.sh`
applies every patch in this directory, in filename order, after re-fetching
the pinned files.

This directory is empty as of `internal/dira/PIN` -- no local patch exists.
See `internal/dira/README.md` and docs/adr/008-precepts-on-dira-applies-when-in-body.md.
