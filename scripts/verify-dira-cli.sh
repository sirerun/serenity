#!/usr/bin/env bash
# T3.14: proves the real kazi-org/dira CLI, installed unmodified at the pin in
# internal/dira/PIN, still behaves the way this repo depends on when run
# against testdata/brain-fixture/ (see that directory's README.md for what the
# fixture is and where it comes from).
#
# This is a conformance check on dira's OWN "check"/"why"/"brief" verbs, not
# on Serenity's (unbuilt) `serenity check` — see docs/plans/E3-m3-direction.md
# T3.14 and T3.7.
#
# Needs network access to install the pinned dira commit via `go install`. Run
# from the repo root:
#   scripts/verify-dira-cli.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PIN="$(tr -d '[:space:]' <"$ROOT/internal/dira/PIN")"
FIXTURE="$ROOT/testdata/brain-fixture"

if [[ -z "$PIN" ]]; then
	echo "internal/dira/PIN is empty" >&2
	exit 1
fi

work="$(mktemp -d)"
trap 'rm -rf "$work" "$FIXTURE/.dira/cache"' EXIT

echo "installing kazi-org/dira @ $PIN ..." >&2
GOBIN="$work/bin" go install "github.com/kazi-org/dira/cmd/dira@${PIN}"
DIRA="$work/bin/dira"

fail=0

# run NAME WANT_EXIT -- runs dira with the remaining args, captures stdout,
# and checks the exit code. Prints stdout on the way out via a nameref so the
# caller can grep it.
run() {
	local name="$1" want_exit="$2" cmd="$3"
	shift 3
	local got_exit=0
	# -C must come right after the subcommand: Go's flag package stops
	# parsing flags at the first positional argument, so "-C" placed after
	# the plan string would be swallowed as part of the plan instead.
	out="$("$DIRA" "$cmd" -C "$FIXTURE" "$@" 2>"$work/stderr")" || got_exit=$?
	if [[ "$got_exit" -ne "$want_exit" ]]; then
		echo "FAIL: $name: exit $got_exit, want $want_exit" >&2
		echo "--- stdout ---" >&2
		echo "$out" >&2
		echo "--- stderr ---" >&2
		cat "$work/stderr" >&2
		fail=1
		return 1
	fi
	return 0
}

# assert_contains NAME HAYSTACK NEEDLE
assert_contains() {
	local name="$1" haystack="$2" needle="$3"
	if [[ "$haystack" != *"$needle"* ]]; then
		echo "FAIL: $name: expected output to contain:" >&2
		echo "  $needle" >&2
		echo "--- got ---" >&2
		echo "$haystack" >&2
		fail=1
	fi
}

echo "=== dira check: compliant plan ===" >&2
if run "check (compliant)" 0 check "write the checkpoint file atomically"; then
	assert_contains "check (compliant)" "$out" "✓ no conflict with 6 enforced entries"
fi

echo "=== dira check: plan conflicting with dec-0060's rejected alternative ===" >&2
if run "check (conflict)" 2 check "add a background daemon to track run state"; then
	assert_contains "check (conflict)" "$out" '✗ conflicts with dec-0060 (accepted 2026-07-03)'
	assert_contains "check (conflict)" "$out" '    rejected alternative: "a daemon"'
	assert_contains "check (conflict)" "$out" '    why_not: violates the single-binary intent (int-0002)'
	assert_contains "check (conflict)" "$out" '    revisit_if: cold-start latency stops being the binding constraint'
	assert_contains "check (conflict)" "$out" '→ supersede dec-0060, or revise the plan'
fi

echo "=== dira why ===" >&2
if run "why" 0 why dec-0060; then
	assert_contains "why" "$out" "dec-0060"
	assert_contains "why" "$out" "Track run state with a checkpoint file, not a"
	assert_contains "why" "$out" "violates the single-binary intent (int-0002)"
fi

echo "=== dira brief ===" >&2
if run "brief" 0 brief; then
	assert_contains "brief" "$out" "brain-fixture"
	assert_contains "brief" "$out" "dec-0060"
fi

if [[ "$fail" -ne 0 ]]; then
	echo "dira CLI conformance failed -- see testdata/brain-fixture/README.md" >&2
	exit 1
fi

echo "dira CLI (check/why/brief) conforms at pin $PIN"
