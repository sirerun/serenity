#!/usr/bin/env python3
"""Require immutable, version-annotated references for workflow actions."""

from pathlib import Path
import re
import sys


USES = re.compile(r"^\s*-\s*uses:\s*([^\s#]+)\s*(?:#\s*(.*))?$")
PIN = re.compile(r"^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@([0-9a-f]{40})$")
VERSION = re.compile(r"^v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?$")


def main() -> int:
    failures = []
    for path in sorted(Path(".github/workflows").glob("*.yml")):
        for number, line in enumerate(path.read_text().splitlines(), 1):
            match = USES.match(line)
            if not match:
                continue
            action, comment = match.groups()
            if action.startswith("./"):
                continue
            if not PIN.fullmatch(action) or not comment or not VERSION.fullmatch(comment.strip()):
                failures.append(f"{path}:{number}: {line.strip()}")
    if failures:
        print("workflow actions must use owner/repo@<40-hex-sha> # vX.Y.Z:", file=sys.stderr)
        print("\n".join(failures), file=sys.stderr)
        return 1
    print("all workflow actions use immutable, version-annotated pins")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
