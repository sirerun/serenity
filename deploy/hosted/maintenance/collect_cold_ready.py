#!/usr/bin/env python3
"""Read-only cold-ready aggregation through exact pinned source; no provider call."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import stat
import types


def require(ok):
    if not ok: raise RuntimeError('cold readiness refused; details withheld')


def unique(pairs):
    value = {}
    for k, v in pairs: require(k not in value); value[k] = v
    return value


def root_bytes(path, expected=None):
    p = Path(path); require(p.is_absolute())
    for parent in p.parents:
        s = parent.lstat(); require(stat.S_ISDIR(s.st_mode) and s.st_uid == 0 and not s.st_mode & 0o022)
    fd = os.open(p, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd, 'rb') as f:
        s = os.fstat(f.fileno()); require(stat.S_ISREG(s.st_mode) and s.st_uid == 0 and s.st_nlink == 1 and not s.st_mode & 0o022 and s.st_size <= 262144)
        raw = f.read(262145); require(len(raw) <= 262144)
    require(expected is None or hashlib.sha256(raw).hexdigest() == expected); return raw


def collect(spec):
    require(os.geteuid() == 0 and type(spec) is dict and set(spec) == {'fixture_pins', 'binding_path', 'binding_sha256'})
    data = root_bytes(spec['binding_path'], spec['binding_sha256'])
    binding = types.ModuleType('pinned_fixture_binding'); binding.__file__ = spec['binding_path']
    exec(compile(data, spec['binding_path'], 'exec', dont_inherit=True), binding.__dict__)
    observed = binding.collect(spec['fixture_pins'])
    return dict(version=2, purpose='serenity.fixture-maintenance/v1', observed_at=observed['observed_at'], phase='maintenance-ready', pending_present=False, writers_drained=True, binding=observed)


def main():
    p = argparse.ArgumentParser(); p.add_argument('--spec', required=True); args = p.parse_args()
    print(json.dumps(collect(json.loads(root_bytes(args.spec), object_pairs_hook=unique)), sort_keys=True, separators=(',', ':')))


if __name__ == '__main__':
    try: main()
    except Exception: raise SystemExit('cold readiness refused; details withheld')
