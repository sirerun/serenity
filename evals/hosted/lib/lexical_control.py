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


class LexicalControlUnavailable(RuntimeError):
    """Raised when the serenity/seedbrain binaries are not available;
    callers must record this as a genuine skip, never a pass."""


@dataclass
class DisposableBrain:
    root: str
    serenity_bin: str

    def search(self, query: str, limit: int = 5, timeout_s: float = 30.0) -> list[str]:
        """Returns ranked chunk refs (e.g. "page:para-02a-fact"), most
        relevant first, from the real lexical-only (no embedder) search
        path. Empty list means "no results" (search.go prints that
        literally when nothing matches)."""
        proc = subprocess.run(
            [self.serenity_bin, "--root", self.root, "search", query, "--limit", str(limit)],
            capture_output=True,
            text=True,
            timeout=timeout_s,
            check=False,
        )
        if proc.returncode != 0:
            raise RuntimeError(f"serenity search failed (exit {proc.returncode}): {proc.stderr.strip()}")
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
    Raises RuntimeError with the failing command's stderr on any step
    failure -- never silently returns a partially-seeded brain.
    """
    root = tempfile.mkdtemp(prefix="serenity-t2343-", dir=workdir)

    init_proc = subprocess.run(
        [serenity_bin, "--root", root, "init"],
        capture_output=True, text=True, timeout=timeout_s, check=False,
    )
    if init_proc.returncode != 0:
        raise RuntimeError(f"serenity init failed: {init_proc.stderr.strip()}")

    seed_input = json.dumps(
        [{"slug": f["id"], "type": f["type"], "summary": f["text"]} for f in facts]
    )
    seed_proc = subprocess.run(
        [seedbrain_bin, "-root", root],
        input=seed_input, capture_output=True, text=True, timeout=timeout_s, check=False,
    )
    if seed_proc.returncode != 0:
        raise RuntimeError(f"seedbrain failed: {seed_proc.stderr.strip()}")

    sync_proc = subprocess.run(
        [serenity_bin, "--root", root, "sync"],
        capture_output=True, text=True, timeout=timeout_s, check=False,
    )
    if sync_proc.returncode != 0:
        raise RuntimeError(f"serenity sync failed: {sync_proc.stderr.strip()}")

    return DisposableBrain(root=root, serenity_bin=serenity_bin)
