#!/usr/bin/env bash
# Proves internal/dira/ carries an unmodified copy of kazi-org/dira at the pin
# recorded in internal/dira/PIN -- the acceptance bar for T3.1
# (docs/plans/E3-m3-direction.md) and the "no fork-and-edit" rule
# (docs/adr/008-precepts-on-dira-applies-when-in-body.md).
#
# What it checks:
#   1. Every vendored .go file's header names the same commit as PIN.
#   2. LICENSE, NOTICE, and the two schema/*.json files are byte-identical to
#      the upstream file at that commit.
#   3. Every vendored .go file is byte-identical to its upstream counterpart,
#      once the vendoring header is stripped and the two documented
#      import-path rewrites (our module path standing in for dira's) are
#      reversed. No other content difference is tolerated.
#
# Needs network access to github.com. Run from the repo root:
#   internal/dira/verify-pin.sh
set -euo pipefail

DIRA="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PIN="$(tr -d '[:space:]' <"$DIRA/PIN")"
UPSTREAM_URL="https://github.com/kazi-org/dira.git"

if [[ -z "$PIN" ]]; then
	echo "internal/dira/PIN is empty" >&2
	exit 1
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "fetching kazi-org/dira @ $PIN ..." >&2
git init -q "$work/upstream"
git -C "$work/upstream" fetch -q --depth 1 "$UPSTREAM_URL" "$PIN"
mkdir -p "$work/upstream-tree"
git -C "$work/upstream" archive FETCH_HEAD -- \
	LICENSE NOTICE schema internal/ledger internal/frontmatter \
	| tar -x -C "$work/upstream-tree"
work_upstream="$work/upstream-tree"

fail=0

# check_header FILE -- the file's vendoring header must name the pinned sha.
check_header() {
	local f="$1"
	local got
	got="$(grep -m1 '^// vendor:pin=' "$f" | sed 's/^\/\/ vendor:pin=//')"
	if [[ "$got" != "$PIN" ]]; then
		echo "HEADER MISMATCH: $f: header pin '$got' != internal/dira/PIN '$PIN'" >&2
		fail=1
	fi
}

# diff_verbatim LOCAL UPSTREAM_REL -- byte-identical, no rewriting.
diff_verbatim() {
	local local_path="$1" upstream_rel="$2"
	if ! diff -q "$DIRA/$local_path" "$work_upstream/$upstream_rel" >/dev/null; then
		echo "DRIFT: $local_path differs from upstream $upstream_rel" >&2
		fail=1
	fi
}

# diff_vendored_go LOCAL UPSTREAM_REL -- strips the 6-line vendoring header,
# reverses the import-path rewrite, then requires byte-identical content.
diff_vendored_go() {
	local local_path="$1" upstream_rel="$2"
	check_header "$DIRA/$local_path"
	tail -n +7 "$DIRA/$local_path" \
		| sed \
			-e 's#github.com/sirerun/serenity/internal/dira/frontmatter#github.com/kazi-org/dira/internal/frontmatter#g' \
			-e 's#github.com/sirerun/serenity/internal/dira/schema#github.com/kazi-org/dira/schema#g' \
		>"$work/local-stripped"
	if ! diff -q "$work/local-stripped" "$work_upstream/$upstream_rel" >/dev/null; then
		echo "DRIFT: $local_path differs from upstream $upstream_rel (beyond the documented import-path rewrite)" >&2
		diff -u "$work_upstream/$upstream_rel" "$work/local-stripped" >&2 || true
		fail=1
	fi
}

diff_verbatim "LICENSE" "LICENSE"
diff_verbatim "NOTICE" "NOTICE"
diff_verbatim "schema/entry.schema.json" "schema/entry.schema.json"
diff_verbatim "schema/check.schema.json" "schema/check.schema.json"

diff_vendored_go "schema/schema.go" "schema/schema.go"
diff_vendored_go "ledger/decode.go" "internal/ledger/decode.go"
diff_vendored_go "ledger/draft.go" "internal/ledger/draft.go"
diff_vendored_go "ledger/encode.go" "internal/ledger/encode.go"
diff_vendored_go "ledger/entry.go" "internal/ledger/entry.go"
diff_vendored_go "ledger/readonly.go" "internal/ledger/readonly.go"
diff_vendored_go "ledger/store.go" "internal/ledger/store.go"
diff_vendored_go "ledger/style.go" "internal/ledger/style.go"
diff_vendored_go "ledger/timestamp.go" "internal/ledger/timestamp.go"
diff_vendored_go "ledger/write.go" "internal/ledger/write.go"
diff_vendored_go "frontmatter/frontmatter.go" "internal/frontmatter/frontmatter.go"

if [[ "$fail" -ne 0 ]]; then
	echo "internal/dira has drifted from the pin -- see internal/dira/README.md" >&2
	exit 1
fi

echo "internal/dira matches kazi-org/dira @ $PIN"
