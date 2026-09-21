"""The real lexical-only control: shells out to the actual `serenity`
binary (built from this repo's own source, zero LLM calls) against a
disposable, throwaway brain repo -- never the hosted service, never a
public debug bypass. T23.43.md step 2: "separately runs lexical-only
control in a disposable brain."

`serenity search` degrades to full-text-only search whenever no embedding
model is pinned on the brain (internal/cli/search.go, runSearch's own
"no embedding model pinned; running full-text-only search" notice) -- a
freshly `serenity init`-ed repo has no model pinned, so this is the
production code path, not a special test mode.

Facts are written as fence entity pages via evals/hosted/fixtures/seedbrain
(a thin wrapper around store.NewEntityPage + writer.Fence, the exact
primitives internal/cli/search_test.go itself proves safe for a synchronous,
non-LLM disposable brain), then `serenity sync` builds the derived FTS
index, then `serenity search <query> --limit N` ranks it.

Both binaries must be pre-built by the caller (SERENITY_BIN / SEEDBRAIN_BIN
env vars, or explicit constructor args) -- this module never invokes `go
build` itself, so it never triggers a multi-package build outside the
mini's build-lease discipline (docs/launch/hosted-plan.md "Heavy-build
discipline"). Their absence is a genuine, disclosed skip, never a silent
pass.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import tempfile
from dataclasses import dataclass

_RESULT_LINE = re.compile(r"^\s*\d+\.\s+(\S+)\s+score=")


class LexicalControlError(RuntimeError):
    """Base for every way the local lexical control cannot produce a result.
    Messages are fixed text plus an exit code or a timeout: a control binary's
    stderr is never echoed, because it can carry paths or environment values."""


class LexicalControlUnavailable(LexicalControlError):
    """Raised when the serenity/seedbrain binaries are not available;
    callers must record this as a genuine skip, never a pass."""


class LexicalControlFailed(LexicalControlError):
    """The binaries exist but a step failed, hung or could not start. Callers
    record BLOCKED (exit 2) with a result file, never an uncaught error."""


def _run(argv: list[str], what: str, timeout_s: float, input_text: str | None = None) -> subprocess.CompletedProcess:
    try:
        proc = subprocess.run(
            argv, capture_output=True, text=True, timeout=timeout_s, check=False, input=input_text
        )
    except subprocess.TimeoutExpired as e:
        raise LexicalControlFailed(f"{what} timed out after {timeout_s:g}s") from e
    except (OSError, subprocess.SubprocessError, ValueError) as e:
        raise LexicalControlFailed(f"{what} could not run: {type(e).__name__}") from e
    if proc.returncode != 0:
        raise LexicalControlFailed(f"{what} failed (exit {proc.returncode})")
    return proc


@dataclass
class DisposableBrain:
    root: str
    serenity_bin: str

    def close(self) -> None:
        """Removes this brain's directory. It is a mkdtemp this module created."""
        shutil.rmtree(self.root, ignore_errors=True)

    def search(self, query: str, limit: int = 5, timeout_s: float = 30.0) -> list[str]:
        """Returns ranked chunk refs (e.g. "page:para-02a-fact"), most
        relevant first, from the real lexical-only (no embedder) search
        path. Empty list means "no results" (search.go prints that
        literally when nothing matches)."""
        proc = _run([self.serenity_bin, "--root", self.root, "search", query, "--limit", str(limit)], "serenity search", timeout_s)
        refs = []
        for line in proc.stdout.splitlines():
            m = _RESULT_LINE.match(line)
            if m:
                refs.append(m.group(1))
        return refs


def resolve_binaries(serenity_bin: str | None, seedbrain_bin: str | None) -> tuple[str, str]:
    serenity_bin = serenity_bin or os.environ.get("SERENITY_BIN", "")
    seedbrain_bin = seedbrain_bin or os.environ.get("SEEDBRAIN_BIN", "")

    def _exists(path: str) -> bool:
        return bool(path) and (os.path.isfile(path) or shutil.which(path) is not None)

    if not _exists(serenity_bin):
        raise LexicalControlUnavailable(
            "SERENITY_BIN not set to an existing binary; build ./cmd/serenity under the "
            "R-build-lease protocol and pass its path (never invoked automatically by this module)"
        )
    if not _exists(seedbrain_bin):
        raise LexicalControlUnavailable(
            "SEEDBRAIN_BIN not set to an existing binary; build ./evals/hosted/fixtures/seedbrain "
            "under the R-build-lease protocol and pass its path"
        )
    return serenity_bin, seedbrain_bin


def build_disposable_brain(
    serenity_bin: str,
    seedbrain_bin: str,
    facts: list[dict],
    workdir: str | None = None,
    timeout_s: float = 60.0,
) -> DisposableBrain:
    """Creates a fresh temp brain repo, seeds it with `facts`
    ({id, type, text} dicts, matching evals/hosted/facts.json's shape),
    and runs the non-LLM `serenity sync` to build its derived FTS index.
    Raises LexicalControlFailed (fixed text, no stderr) on any step failure or
    timeout, and removes the brain it created first -- it never silently
    returns a partially-seeded brain and never leaves one behind on failure.
    """
    root = tempfile.mkdtemp(prefix="serenity-t2343-", dir=workdir)
    try:
        _run([serenity_bin, "--root", root, "init"], "serenity init", timeout_s)
        seed_input = json.dumps([{"slug": f["id"], "type": f["type"], "summary": f["text"]} for f in facts])
        _run([seedbrain_bin, "-root", root], "seedbrain", timeout_s, input_text=seed_input)
        _run([serenity_bin, "--root", root, "sync"], "serenity sync", timeout_s)
    except BaseException:
        shutil.rmtree(root, ignore_errors=True)
        raise
    return DisposableBrain(root=root, serenity_bin=serenity_bin)
