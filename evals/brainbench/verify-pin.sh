#!/usr/bin/env bash
# Proves evals/brainbench/{fixtures,gold,schema,README.md,_ledger.json,LICENSE}
# carry an unmodified copy of dndungu/gbrain's BrainBench corpus at the
# commit recorded in evals/brainbench/PIN -- the acceptance bar for T1.21
# (docs/plans/E1-m1-ingest.md) and the "no fork-and-edit" rule the same
# T3.1/internal/dira/verify-pin.sh pattern established for vendored data.
#
# Unlike internal/dira/verify-pin.sh, nothing vendored here is Go source
# (no import paths to rewrite) -- every vendored file must be byte-identical
# to its upstream counterpart, full stop.
#
# Needs network access to github.com. Run from the repo root:
#   evals/brainbench/verify-pin.sh
set -euo pipefail

BB="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PIN="$(tr -d '[:space:]' <"$BB/PIN")"
UPSTREAM_URL="https://github.com/dndungu/gbrain.git"

if [[ -z "$PIN" ]]; then
	echo "evals/brainbench/PIN is empty" >&2
	exit 1
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "fetching dndungu/gbrain @ $PIN ..." >&2
git init -q "$work/upstream"
git -C "$work/upstream" fetch -q --depth 1 "$UPSTREAM_URL" "$PIN"
mkdir -p "$work/tree"
git -C "$work/upstream" archive FETCH_HEAD -- LICENSE evals/brainbench \
	| tar -x -C "$work/tree"
up="$work/tree"

fail=0

# diff_dir LOCAL_REL UPSTREAM_REL -- every file under LOCAL_REL must exist,
# byte-identical, under UPSTREAM_REL, and vice versa (catches both drift
# and an unpinned add/removal).
diff_dir() {
	local local_rel="$1" upstream_rel="$2"
	if ! diff -rq "$BB/$local_rel" "$up/$upstream_rel" >/dev/null 2>&1; then
		echo "DRIFT: evals/brainbench/$local_rel differs from upstream $upstream_rel" >&2
		diff -rq "$BB/$local_rel" "$up/$upstream_rel" >&2 || true
		fail=1
	fi
}

diff_file() {
	local local_rel="$1" upstream_rel="$2"
	if ! diff -q "$BB/$local_rel" "$up/$upstream_rel" >/dev/null; then
		echo "DRIFT: evals/brainbench/$local_rel differs from upstream $upstream_rel" >&2
		fail=1
	fi
}

diff_dir "fixtures" "evals/brainbench/fixtures"
diff_dir "gold" "evals/brainbench/gold"
diff_dir "schema" "evals/brainbench/schema"
diff_file "README.md" "evals/brainbench/README.md"
diff_file "_ledger.json" "evals/brainbench/_ledger.json"
diff_file "LICENSE" "LICENSE"

if [[ "$fail" -ne 0 ]]; then
	echo "evals/brainbench has drifted from the pin -- see evals/brainbench/README.md" >&2
	exit 1
fi

echo "evals/brainbench matches dndungu/gbrain @ $PIN"
