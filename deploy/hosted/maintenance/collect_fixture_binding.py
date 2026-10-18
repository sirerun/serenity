#!/usr/bin/env python3
"""Read-only v3 cold binding. Root reads pins; SQLite metadata runs as service UID.
Import-only filesystem/runner injection is synthetic qualification, not authority.
"""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import pwd
import stat
import subprocess
import time
import types

HOLD = '/var/lib/serenity-release/maintenance.hold.json'
STATE = '/var/lib/serenity-release/state.json'
HOSTED = '/etc/serenity/hosted.json'


def require(ok):
    if not ok: raise RuntimeError('fixture binding refused; details withheld')


def digest(raw): return hashlib.sha256(raw).hexdigest()


def unique(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result); result[key] = value
    return result


def decode(raw): return json.loads(raw, object_pairs_hook=unique)


def root_read(name, private=False, limit=262144, root=Path('/'), owner=0):
    require(type(name) is str and name.startswith('/') and str(Path(name)) == name and '..' not in Path(name).parts)
    path = Path(root) / name.lstrip('/')
    for parent in path.parents:
        s = parent.lstat(); require(stat.S_ISDIR(s.st_mode) and s.st_uid == owner and not s.st_mode & 0o022)
        if parent == Path(root): break
    before = path.lstat()
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd, 'rb') as stream:
        s = os.fstat(stream.fileno())
        require((s.st_dev, s.st_ino) == (before.st_dev, before.st_ino))
        require(stat.S_ISREG(s.st_mode) and s.st_uid == owner and s.st_nlink == 1 and not s.st_mode & (0o077 if private else 0o022) and s.st_size <= limit)
        raw = stream.read(limit + 1); require(len(raw) <= limit); return raw


def module(name, expected, root=Path('/'), owner=0):
    data = root_read(name, root=root, owner=owner)
    require(digest(data) == expected)
    m = types.ModuleType('fixture_pinned_' + Path(name).stem); m.__file__ = name
    exec(compile(data, name, 'exec', dont_inherit=True), m.__dict__)
    return m


METADATA_READER = """import contextlib,hashlib,json,sqlite3,sys,urllib.parse
uri='file:'+urllib.parse.quote(sys.argv[1],safe='/')+'?mode=ro&immutable=1'
with contextlib.closing(sqlite3.connect(uri,uri=True)) as db:
 db.execute('PRAGMA query_only=ON')
 if db.execute('SELECT count(*),min(version),max(version) FROM schema_migrations').fetchone()!=(10,1,10):raise SystemExit(1)
 h=hashlib.sha256()
 for row in db.execute("SELECT type,name,tbl_name,coalesce(sql,'') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY type,name"):
  for field in row:
   if chr(0) in field:raise SystemExit(1)
   h.update(field.encode()+bytes([0]))
 print(json.dumps({'schema_sha256':h.hexdigest()}))
"""


def collect(pins, root=Path('/'), owner=0, clock=time.time, metadata_runner=None, identity=None, adapter_factory=None, runtime_runner=None):
    require(os.geteuid() == owner and type(pins) is dict and set(pins) == {'runtime_path', 'runtime_sha256', 'config'})
    c = pins['config']; f, service, auth = c['fixture'], c['service'], c['authority']
    runtime = module(pins['runtime_path'], pins['runtime_sha256'], root, owner)
    require(runtime.CONFIG == c)
    # Constructor performs the current strict declaration validation. It does
    # not enter maintenance, acquire another lock, or collect provider data.
    transaction = runtime.Transaction(c, root=Path(root), uid=owner, gid=os.getegid(), clock=clock, runner=runtime_runner)
    wrapper = module(f['wrapper_path'], f['wrapper_sha256'], root, owner)
    adapter = (adapter_factory or wrapper.Adapter)(root=Path(root), owner=owner, clock=clock)
    lock = None
    try:
        lock_path = adapter.trusted(STATE + '.lock', private=True)
        lock = os.open(lock_path, os.O_RDONLY | os.O_NOFOLLOW)
        ls = os.fstat(lock); require(ls.st_dev == lock_path.lstat().st_dev and ls.st_ino == lock_path.lstat().st_ino)
        fcntl.flock(lock, fcntl.LOCK_SH | fcntl.LOCK_NB)
        hold_raw = root_read(HOLD, True, root=root, owner=owner); hold = decode(hold_raw)
        expected_hold = dict(version=1, purpose=c['purpose'], operation_id=c['operation_id'], manifest_sha256=c['manifest_sha256'], service_source=service['source'], executable_sha256=service['binary_sha256'], state_sha256=service['state_sha256'], broker_sha256=service['broker_sha256'], config_sha256=service['config_sha256'], supervisor=c['supervisor'], stage=c['stage'], backup_timer_prior_state='active', phase='maintenance-ready', timestamp=hold.get('timestamp'), expires_at=c['expires_at'])
        require(hold == expected_hold and type(hold['timestamp']) is int and c['issued_at'] <= hold['timestamp'] <= int(clock()) < c['expires_at'])
        state_raw = root_read(STATE, True, root=root, owner=owner); require(digest(state_raw) == service['state_sha256'])
        config_raw = root_read(HOSTED, root=root, owner=owner); config = decode(config_raw)
        require(digest(config_raw) == service['config_sha256'] and config['data_dir'] == f['data_dir'] and type(config['account_cap']) is int and config['account_cap'] == f['account_cap'])
        require(all(config.get(k, False) is False for k in ('episode_archive_enabled', 'workos_enabled', 'billing_enabled')) and config.get('episode_synthetic_brains', []) == [])
        require(digest(root_read('/etc/serenity-authority/broker.json', True, root=root, owner=owner)) == service['broker_sha256'])
        for path, expected, maximum in [(f['tool_path'], f['tool_sha256'], 268435456), (f['wrapper_path'], f['wrapper_sha256'], 262144), (service['binary_path'], service['binary_sha256'], 268435456), (auth['generator_path'], auth['generator_sha256'], 262144)]:
            require(digest(root_read(path, limit=maximum, root=root, owner=owner)) == expected)
        adapter.absent(STATE + '.pending'); adapter.absent(c['stage'] + '/closeout-pending.json')
        ready_raw = root_read(c['stage'] + '/maintenance-ready.json', True, root=root, owner=owner)
        require(decode(ready_raw) == dict(version=2, purpose=c['purpose'], operation_id=c['operation_id'], manifest_sha256=c['manifest_sha256'], state_sha256=service['state_sha256'], hold_sha256=digest(hold_raw), service_source=service['source'], executable_sha256=service['binary_sha256'], held=True, services_started=False))
        actual_identity = identity or pwd.getpwnam('serenity')
        require(actual_identity.pw_uid == f['uid'] > 0 and actual_identity.pw_gid == f['gid'] > 0)
        for path in (f['data_dir'], f['data_dir'] + '/brains', f['private_dir']):
            adapter.trusted(path, private=True, directory=True, owner=f['uid'])
        dbpath = f['data_dir'] + '/control.db'
        adapter.trusted(dbpath, private=True, owner=f['uid'])
        for suffix in ('-wal', '-shm', '-journal'): adapter.absent(dbpath + suffix)
        run = metadata_runner or subprocess.run
        checked = run(['/usr/bin/python3', '-I', '-c', METADATA_READER, str(adapter.path(dbpath))], user=f['uid'], group=f['gid'], extra_groups=[], stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=15, env={'PATH': '/usr/bin:/bin', 'LANG': 'C'})
        require(checked.returncode == 0 and type(checked.stdout) is bytes and len(checked.stdout) <= 128 and decode(checked.stdout) == {'schema_sha256': f['schema_sha256']})
        require(transaction.cold_ready() == decode(ready_raw))
        adapter.module = runtime; adapter.proof = dict(version=3, maintenance_hold=hold, runtime_path=pins['runtime_path'])
        adapter.check_units()
        for unit in wrapper.WRITERS: adapter.absent(wrapper.RUN + '/' + unit + '.permit')
        require(adapter.path('/proc/sys/kernel/random/boot_id').read_text().strip() == c['boot_id'])
        require(root_read(HOLD, True, root=root, owner=owner) == hold_raw and root_read(STATE, True, root=root, owner=owner) == state_raw and root_read(HOSTED, root=root, owner=owner) == config_raw)
        adapter.absent(c['stage'] + '/closeout-pending.json')
        return dict(observed_at=int(clock()), boot_id=c['boot_id'], uid=f['uid'], gid=f['gid'], maintenance_hold=hold, hold_sha256=digest(hold_raw), state_sha256=digest(state_raw), ready_sha256=digest(ready_raw), config_sha256=digest(config_raw), data_dir=f['data_dir'], account_cap=f['account_cap'], schema_sha256=f['schema_sha256'], lock_device=ls.st_dev, lock_inode=ls.st_ino)
    finally:
        adapter.close()
        if lock is not None: os.close(lock)


def main():
    p = argparse.ArgumentParser(); p.add_argument('--pins', required=True); args = p.parse_args()
    print(json.dumps(collect(decode(root_read(args.pins, True))), sort_keys=True, separators=(',', ':')))


if __name__ == '__main__':
    try: main()
    except Exception: raise SystemExit('fixture binding refused; details withheld')
