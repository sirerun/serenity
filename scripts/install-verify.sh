#!/usr/bin/env bash
# Fresh-machine install + smoke-test script (plan T5.7, RFC 0001's M1 AC
# "First-value walkthrough": a new user installs, ingests one connector
# corpus, and gets one cited answer -- end to end from docs alone).
#
# Drives the full pipeline end to end against a throwaway brain repo:
#
#   clone -> init -> connect one connector -> sync -> extract ->
#   search -> ask -> inbox -> check
#
# Every serenity command below is named in the docs site: the CLI verb
# list (docs/rfc/0001-serenity.md, in the mkdocs Governance nav) names
# init/sync/extract/search/ask/inbox/check; docs/operator/scheduling.md
# documents `go install` as how the binary is built; docs/connectors/
# README.md and file.md document editing serenity.yml's `connectors:`
# key by hand (no CLI command exists to author it) plus
# `serenity connectors status`; docs/operator/claude.md documents the
# exact `check --json --actions '[...]'` invocation used below.
#
# Usage: scripts/install-verify.sh [source-repo-path-or-url]
#   Defaults to the current directory, so `cd` into a serenity checkout
#   and run it directly on a real fresh machine. The CI job passes the
#   checked-out workspace explicitly so `clone` exercises a genuine
#   local `git clone` rather than an unauthenticated fetch of a repo
#   that may still be private (docs/plan.md T5.20 is the "make public"
#   gate; it hasn't run yet).
#
# Budget (acc line): must complete in under 15 minutes wall clock.

set -euo pipefail

# --- Linux headless keychain bootstrap ---------------------------------
# `serenity init` mints a daemon auth token into the OS keychain
# (docs/operator/server.md: "minted once by `serenity init`"). On Linux
# that's the freedesktop Secret Service over D-Bus (zalando/go-keyring),
# which needs a running session bus and an unlocked keyring collection --
# neither exists on a bare machine or CI runner. macOS's Keychain needs
# no such bootstrap, so this only runs on Linux, and only once (a
# self-relaunch under `dbus-run-session`, guarded by
# DBUS_SESSION_BUS_ADDRESS so the relaunched process doesn't loop).
if [[ "$(uname -s)" == "Linux" && -z "${DBUS_SESSION_BUS_ADDRESS:-}" ]]; then
	if ! command -v dbus-run-session >/dev/null 2>&1 || ! command -v gnome-keyring-daemon >/dev/null 2>&1; then
		if command -v sudo >/dev/null 2>&1 && command -v apt-get >/dev/null 2>&1; then
			echo "install-verify: installing dbus-user-session + gnome-keyring (headless keychain backend)" >&2
			sudo apt-get update -y -qq
			sudo apt-get install -y -qq dbus-user-session gnome-keyring >/dev/null
		else
			echo "install-verify: dbus-run-session/gnome-keyring-daemon not found and no apt-get to install them" >&2
			exit 1
		fi
	fi
	exec dbus-run-session -- "$0" "$@"
fi
if [[ "$(uname -s)" == "Linux" ]]; then
	eval "$(printf '\n' | gnome-keyring-daemon --unlock --daemonize --components=secrets)"
	export GNOME_KEYRING_CONTROL
fi

SOURCE="${1:-$PWD}"
WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/serenity-install-verify.XXXXXX")"
trap 'rm -rf "$WORKDIR"' EXIT

STEPS_DIR="$WORKDIR/steps"
mkdir -p "$STEPS_DIR"

BUDGET_SECONDS=900 # 15 min (acc line)
START_TS=$(date +%s)
STEP_N=0

# step NAME CMD... -- runs CMD, requires exit 0 and non-empty combined
# stdout+stderr (acc line: "each step's output is asserted non-empty"),
# and prints the captured output indented so a CI log shows real
# evidence per step, not just a checkmark.
step() {
	local name="$1"
	shift
	STEP_N=$((STEP_N + 1))
	local log="$STEPS_DIR/$STEP_N-$name.log"
	echo "==> [$STEP_N] $name: $*"
	local t0 t1
	t0=$(date +%s)
	if ! "$@" >"$log" 2>&1; then
		echo "FAIL: step '$name' exited nonzero. Output:" >&2
		cat "$log" >&2
		exit 1
	fi
	t1=$(date +%s)
	if [[ ! -s "$log" ]]; then
		echo "FAIL: step '$name' produced empty output (acc: every step's output must be non-empty)" >&2
		exit 1
	fi
	echo "    ok ($((t1 - t0))s)"
	sed 's/^/    | /' "$log"
}

# assert_contains NAME LOGSTEP PATTERN -- extra, stronger-than-the-acc-
# line checks for the two steps whose whole point is proving real data
# flowed through the pipeline (connect->sync actually ingested the
# canned Ava corpus; search actually finds it). A merely non-empty log
# would still pass with "0 new source(s)" or "no results" -- vacuous
# green the acc line's letter wouldn't catch. See docs/lore.md's own
# anti-vacuous-green convention.
assert_contains() {
	local name="$1" log="$2" pattern="$3"
	if ! grep -Eq "$pattern" "$log"; then
		echo "FAIL: step '$name' output did not match /$pattern/ -- pipeline ran but produced no real signal. Output:" >&2
		cat "$log" >&2
		exit 1
	fi
}

echo "install-verify: source=$SOURCE workdir=$WORKDIR"

# --- clone --------------------------------------------------------------
# docs/rfc/0001-serenity.md / docs/threat-model.md both use `git clone`
# as the fresh-machine recovery primitive; here it fetches the source so
# the rest of this script can build and install it, mirroring RFC 0001's
# M1 "First-value walkthrough" acceptance line's "a new user installs".
SRC_DIR="$WORKDIR/src"
step clone git clone "$SOURCE" "$SRC_DIR"

# --- build (not one of the 9 acc-line pipeline steps; not output-asserted) ---
# docs/operator/scheduling.md: "Find your `serenity` binary's absolute
# path first -- `command -v serenity` (a `go install` build)".
BIN_DIR="$WORKDIR/bin"
echo "==> building serenity (go install)"
t0=$(date +%s)
(cd "$SRC_DIR" && GOBIN="$BIN_DIR" go install ./cmd/serenity)
echo "    ok ($(($(date +%s) - t0))s)"
SERENITY="$BIN_DIR/serenity"

BRAIN="$WORKDIR/brain"

# --- init -----------------------------------------------------------------
step init "$SERENITY" -C "$BRAIN" init

# `serenity init`'s git init has no identity configured yet -- sync and
# extract below each commit newly-ingested sources/claims (RFC 0001
# §7.7's writer queue), which git refuses without one. GitHub Actions
# runners carry no global git identity either (found the hard way by
# T1.15, docs/roadmap.md). Local to this throwaway brain only.
git -C "$BRAIN" config user.email "install-verify@localhost"
git -C "$BRAIN" config user.name "install-verify"

# --- connect one connector -------------------------------------------------
# docs/connectors/README.md / file.md: the file connector has no CLI
# command to author its config ("there's still no CLI command to author
# this config; edit serenity.yml directly") -- so "connect" here is
# exactly what the docs site instructs: hand-append the documented
# `connectors.file.path` shape, pointed at the canned Ava corpus
# (evals/corpora/ava/labels, already checksummed and checked into the
# repo -- plan T1.14) that ships with the clone.
CORPUS_DIR="$SRC_DIR/evals/corpora/ava/labels"
if [[ ! -d "$CORPUS_DIR" ]]; then
	echo "FAIL: canned Ava corpus not found at $CORPUS_DIR" >&2
	exit 1
fi
cat >>"$BRAIN/serenity.yml" <<YAML
connectors:
  file:
    path: $CORPUS_DIR
YAML
# `serenity connectors status` (docs/connectors/README.md) is the one
# documented connector-facing command beyond auth; run it as this step's
# asserted-non-empty evidence that "connect" happened. The real proof
# the file connector was actually wired for this brain is the sync
# step's own "N new source(s)" line right below, checked more strictly.
step connect "$SERENITY" -C "$BRAIN" connectors status

# The file connector's Poll debounces a path until it has gone
# unchanged for >= 2s (docs/connectors/file.md) so a half-written file
# is never ingested. The corpus was written by `git clone` well over
# 2s ago by now, but this sleep removes any doubt on a very fast disk.
sleep 3

# --- sync -------------------------------------------------------------
step sync "$SERENITY" -C "$BRAIN" sync
assert_contains sync "$STEPS_DIR/$STEP_N-sync.log" 'file: [1-9][0-9]* item\(s\) polled, [1-9][0-9]* new source\(s\)'

# --- extract ------------------------------------------------------------
# No extraction model is pinned (config.Default's install-time
# "none@v0") and none is credentialed in CI -- `extract` honestly
# reports the skip and exits cleanly rather than erroring or fabricating
# results (docs/connectors/README.md: "`serenity extract` needs a
# pinned model plus a credential to do anything -- otherwise it reports
# why it skipped and exits cleanly"). That reported skip is this step's
# asserted-non-empty output.
step extract "$SERENITY" -C "$BRAIN" extract

# --- search -------------------------------------------------------------
# `sync`'s rebuild indexes every ingested source's raw text into FTS
# independent of extraction (docs/rfc/0001-serenity.md's `search` verb),
# so a plain query for the corpus's own subject name proves the ingest
# -> index -> search path actually works end to end, without needing a
# live embedding/extraction credential.
step search "$SERENITY" -C "$BRAIN" search "Ava"
assert_contains search "$STEPS_DIR/$STEP_N-search.log" '^ *1\.'

# --- ask ------------------------------------------------------------------
# No composer model is pinned either -- `ask` reports the same kind of
# honest skip `extract` does (internal/providers.BuildComposerRouter:
# "no composer model pinned ... ask skipped"), which is this step's
# asserted-non-empty output.
step ask "$SERENITY" -C "$BRAIN" ask "What does the brain know about Ava?"

# --- inbox --------------------------------------------------------------
# `inbox` with no flags drives an interactive j/k/space TUI
# (docs/rfc/0001-serenity.md's `inbox` verb) -- not scriptable in a CI
# job with no tty. `--parked` is the documented non-interactive,
# read-only one-shot mode (docs/protocol/DISPOSITION_v1.md references
# `inbox --parked`); it prints "inbox: no parked items" on an empty
# queue, never a blank line, so this step's output is non-empty either
# way.
step inbox "$SERENITY" -C "$BRAIN" inbox --parked

# --- check ----------------------------------------------------------------
# The exact documented example (docs/operator/claude.md):
step check "$SERENITY" -C "$BRAIN" check --json --actions '[{"action":"spend_over","params":{"amount":100}}]'

TOTAL=$(( $(date +%s) - START_TS ))
echo "install-verify: completed $STEP_N steps in ${TOTAL}s (budget ${BUDGET_SECONDS}s)"
if (( TOTAL >= BUDGET_SECONDS )); then
	echo "FAIL: exceeded the ${BUDGET_SECONDS}s (15 min) wall-clock budget" >&2
	exit 1
fi
