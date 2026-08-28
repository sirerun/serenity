#!/usr/bin/env bash
# Re-vendors evals/brainbench/{fixtures,gold,schema,README.md,_ledger.json}
# and the root LICENSE from dndungu/gbrain at a new pinned commit.
#
# Usage: scripts/update-brainbench.sh <commit-sha>
#
# Unlike scripts/update-dira.sh, every vendored file here is data (JSON,
# Markdown) or a license text -- there are no import paths to rewrite, so
# this is a plain byte-identical copy. Run evals/brainbench/verify-pin.sh
# afterwards to confirm.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 <commit-sha>" >&2
	exit 1
fi
PIN="$1"
UPSTREAM_URL="https://github.com/dndungu/gbrain.git"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BB="$REPO_ROOT/evals/brainbench"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "fetching dndungu/gbrain @ $PIN ..." >&2
git init -q "$work/upstream"
git -C "$work/upstream" fetch -q --depth 1 "$UPSTREAM_URL" "$PIN"
mkdir -p "$work/tree"
git -C "$work/upstream" archive FETCH_HEAD -- LICENSE evals/brainbench \
	| tar -x -C "$work/tree"
up="$work/tree"

rm -rf "$BB/fixtures" "$BB/gold" "$BB/schema"
mkdir -p "$BB/fixtures" "$BB/gold" "$BB/schema"
cp "$up/evals/brainbench/fixtures/"*.fixture.json "$BB/fixtures/"
cp "$up/evals/brainbench/gold/"*.gold.json "$BB/gold/"
cp "$up/evals/brainbench/schema/"*.schema.json "$BB/schema/"
cp "$up/evals/brainbench/README.md" "$BB/README.md"
cp "$up/evals/brainbench/_ledger.json" "$BB/_ledger.json"
cp "$up/LICENSE" "$BB/LICENSE"
printf '%s\n' "$PIN" >"$BB/PIN"

echo "vendored dndungu/gbrain BrainBench corpus @ $PIN into evals/brainbench/." >&2
echo "now run: evals/brainbench/verify-pin.sh, go build ./..., go test ./internal/eval/brainbench/..." >&2
