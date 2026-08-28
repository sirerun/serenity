#!/usr/bin/env bash
# Re-vendors internal/dira/ from kazi-org/dira at a new pinned commit.
#
# Usage: scripts/update-dira.sh <commit-sha>
#
# What it does:
#   1. Fetches kazi-org/dira at <commit-sha>.
#   2. Overwrites every file internal/dira/README.md lists as vendored with
#      the fresh copy, rewriting the two documented import paths and
#      prepending the vendoring header (see internal/dira/verify-pin.sh for
#      the exact rewrite rules -- this script and that one must agree).
#   3. Applies every internal/dira/patches/*.patch, in filename order, on top
#      of the fresh copy -- this is the ONLY sanctioned way to carry a local
#      change; a vendored file is never hand-edited (see patches/README.md).
#   4. Writes the new sha to internal/dira/PIN.
#
# It does not run gofmt, go build, go test, or internal/dira/verify-pin.sh --
# run all four yourself before committing the bump.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 <commit-sha>" >&2
	exit 1
fi
PIN="$1"
UPSTREAM_URL="https://github.com/kazi-org/dira.git"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIRA="$REPO_ROOT/internal/dira"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "fetching kazi-org/dira @ $PIN ..." >&2
git init -q "$work/upstream"
git -C "$work/upstream" fetch -q --depth 1 "$UPSTREAM_URL" "$PIN"
mkdir -p "$work/tree"
git -C "$work/upstream" archive FETCH_HEAD -- \
	LICENSE NOTICE schema internal/ledger internal/frontmatter \
	| tar -x -C "$work/tree"

header() {
	cat <<EOF
// Vendored from github.com/kazi-org/dira @ ${PIN}.
// DO NOT EDIT DIRECTLY. A local change goes in internal/dira/patches/*.patch,
// applied by scripts/update-dira.sh, which also re-fetches this file. See
// internal/dira/PIN and internal/dira/README.md.
// vendor:pin=${PIN}

EOF
}

# vendor_go LOCAL UPSTREAM_REL -- copies one Go file, rewriting the two
# documented import-path lines and prepending the vendoring header.
vendor_go() {
	local local_path="$1" upstream_rel="$2"
	{
		header
		# Terminator-anchored: the import line ends the package path with a
		# closing quote, and the one comment referencing the schema package
		# ends it with a comma. Neither anchor matches schemaURL's
		# "https://github.com/kazi-org/dira/schema/entry.schema.json" --
		# that string is entry.schema.json's own $id and must stay
		# upstream-verbatim (see internal/dira/README.md).
		sed \
			-e 's#github\.com/kazi-org/dira/internal/frontmatter"#github.com/sirerun/serenity/internal/dira/frontmatter"#g' \
			-e 's#github\.com/kazi-org/dira/schema,#github.com/sirerun/serenity/internal/dira/schema,#g' \
			"$work/tree/$upstream_rel"
	} >"$DIRA/$local_path"
}

# vendor_verbatim LOCAL UPSTREAM_REL -- byte-identical copy, no rewriting.
vendor_verbatim() {
	cp "$work/tree/$2" "$DIRA/$1"
}

vendor_verbatim "LICENSE" "LICENSE"
vendor_verbatim "NOTICE" "NOTICE"
vendor_verbatim "schema/entry.schema.json" "schema/entry.schema.json"
vendor_verbatim "schema/check.schema.json" "schema/check.schema.json"

vendor_go "schema/schema.go" "schema/schema.go"
vendor_go "ledger/decode.go" "internal/ledger/decode.go"
vendor_go "ledger/draft.go" "internal/ledger/draft.go"
vendor_go "ledger/encode.go" "internal/ledger/encode.go"
vendor_go "ledger/entry.go" "internal/ledger/entry.go"
vendor_go "ledger/readonly.go" "internal/ledger/readonly.go"
vendor_go "ledger/store.go" "internal/ledger/store.go"
vendor_go "ledger/style.go" "internal/ledger/style.go"
vendor_go "ledger/timestamp.go" "internal/ledger/timestamp.go"
vendor_go "ledger/write.go" "internal/ledger/write.go"
vendor_go "frontmatter/frontmatter.go" "internal/frontmatter/frontmatter.go"

shopt -s nullglob
patches=("$DIRA"/patches/*.patch)
shopt -u nullglob
if [[ ${#patches[@]} -gt 0 ]]; then
	echo "applying ${#patches[@]} local patch(es) ..." >&2
	for p in "${patches[@]}"; do
		echo "  $p" >&2
		git -C "$REPO_ROOT" apply "$p"
	done
fi

printf '%s\n' "$PIN" >"$DIRA/PIN"

echo "vendored kazi-org/dira @ $PIN into internal/dira/." >&2
echo "now run: go build ./..., go test ./internal/dira/..., gofmt -l internal/dira, internal/dira/verify-pin.sh" >&2
