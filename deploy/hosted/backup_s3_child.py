"""Private single-threaded launcher for a file-size-limited CLI child."""

from __future__ import annotations

import os
import resource
import sys


def _arguments(argv: list[str]) -> tuple[int, str, list[str]]:
    if len(argv) < 4 or argv[0] != "--file-size-limit" or argv[2] != "--":
        raise ValueError("invalid private child invocation")
    try:
        limit = int(argv[1], 10)
    except ValueError as error:
        raise ValueError("invalid private child file-size limit") from error
    executable = argv[3]
    arguments = argv[3:]
    if limit <= 0 or not os.path.isabs(executable) or not os.path.isfile(executable):
        raise ValueError("invalid private child target")
    return limit, executable, arguments


def _set_file_size_limit(limit: int) -> None:
    _soft, hard = resource.getrlimit(resource.RLIMIT_FSIZE)
    if hard != resource.RLIM_INFINITY and limit > hard:
        raise OSError("requested file-size limit exceeds the inherited hard limit")
    resource.setrlimit(resource.RLIMIT_FSIZE, (limit, limit))
    actual = resource.getrlimit(resource.RLIMIT_FSIZE)
    if actual != (limit, limit):
        raise OSError("operating system did not enforce the requested file-size limit")


def main(argv: list[str] | None = None) -> int:
    """Set a kernel file-size bound and exec the target without changing PID."""
    if not hasattr(resource, "RLIMIT_FSIZE"):
        return 125
    try:
        limit, executable, arguments = _arguments(
            list(sys.argv[1:] if argv is None else argv)
        )
        _set_file_size_limit(limit)
        os.execve(executable, arguments, os.environ.copy())
    except (OSError, ValueError):
        return 125
    return 125


if __name__ == "__main__":
    raise SystemExit(main())
