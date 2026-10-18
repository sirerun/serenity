#!/usr/bin/env python3
"""Root custody adapter; no provider calls, service changes, or credential reads.

CLI accepts only an existing private root proof path. Test filesystem/runner
injection is available to imported tests, never via CLI or environment variables.
"""
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import pwd
import re
import signal
import stat
import subprocess
import sys
import time

HOLD = '/var/lib/serenity-release/maintenance.hold.json'
STATE = '/var/lib/serenity-release/state.json'
HOSTED = '/etc/serenity/hosted.json'
RUN = '/run/serenity-release-maintenance'
WRITERS = ('serenity-hosted.service', 'serenity-broker.service')
BACKUP = 'serenity-backup.service'
TIMER = 'serenity-backup.timer'
UNITS = (*WRITERS, BACKUP, TIMER)
MARKER = '# serenity-release-managed-v1\n'
PROOF_FIELDS = set('version purpose operation nonce issued_at expires_at boot_id uid gid tool_source tool_path tool_sha256 wrapper_path wrapper_sha256 runtime_path runtime_sha256 service_binary_path service_binary_sha256 service_source config_path config_sha256 data_dir private_dir account_cap hold_sha256 hold state_sha256 ready_sha256 authority_receipt_path authority_receipt_sha256 authority_generator_path authority_generator_sha256 lock_device lock_inode schema_sha256 authority_scope authority_session_id concurrency_target'.split())
ADOPTION_FIELDS = {'old_source', 'old_manifest_sha256'}
ADOPTION_OUTCOMES = {
    2: 'REFUSED: source adoption did not publish; retain hold',
    3: 'REFUSED: exact old manifest hash no longer matches; no adoption write; reconcile prior publication',
    5: 'INTERRUPTED: source adoption stopped before publication; old manifest retained; fresh custody required',
    6: 'UNCERTAIN: source adoption publication may have committed; retain hold and reconcile manifest; do not replay as success',
}
HOLD_FIELDS = set('manifest_sha256 executable_sha256 source previous_state_sha256 expected_state_sha256 supervisor stage backup_timer_prior_state phase timestamp'.split())
V3_HOLD_FIELDS = set('version purpose operation_id manifest_sha256 service_source executable_sha256 state_sha256 broker_sha256 config_sha256 supervisor stage backup_timer_prior_state phase timestamp expires_at'.split())
V3_PROOF_FIELDS = PROOF_FIELDS - {'hold'} | {'maintenance_hold'}
AUTHORITY_FIELDS = set('version observed_at status active_leases concurrency session_id scope boot_id hold_sha256 state_sha256 request_ids concurrency_target'.split())


def require(ok):
    if not ok:
        raise RuntimeError('fixture custody refused')


def digest(data):
    return hashlib.sha256(data).hexdigest()


def unique(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result)
        result[key] = value
    return result


def decode(data):
    return json.loads(data, object_pairs_hook=unique)


def snapshot(st):
    return (st.st_dev, st.st_ino, st.st_mode, st.st_uid, st.st_gid,
            st.st_nlink, st.st_size, st.st_mtime_ns, st.st_ctime_ns)


def absolute(path):
    return isinstance(path, str) and str(Path(path)) == path and path.startswith('/') and '..' not in Path(path).parts and not re.search(r'[\s\x00]', path)


EXEC_CONDITION_TYPE = 'a(sasbttttuii)'


def exact_guard(rows, runtime, unit):
    # systemd Service.ExecCondition: path, argv, ignore_errors, four timestamps,
    # PID, exit code, exit status. Never parse systemctl's display rendering.
    require(type(rows) is list and len(rows) == 1)
    row = rows[0]
    require(type(row) is list and len(row) == 10)
    require(row[0] == '/usr/bin/python3')
    require(type(row[1]) is list and row[1] == ['/usr/bin/python3', runtime, 'guard', unit])
    require(row[2] is False)
    for value, bits, signed in zip(row[3:], (64, 64, 64, 64, 32, 32, 32),
                                   (False, False, False, False, False, True, True)):
        require(type(value) is int)
        require(-(1 << (bits - 1)) <= value < (1 << (bits - 1)) if signed
                else 0 <= value < (1 << bits))


def decode_exec_conditions(data):
    value = decode(data)
    require(type(value) is dict and set(value) == {'type', 'data'})
    require(value['type'] == EXEC_CONDITION_TYPE and type(value['data']) is list)
    # get-property emits the property array directly (unlike method replies).
    # Exact command cardinality and field types are validated by exact_guard.
    return value['data']


class Adapter:
    def __init__(self, root=Path('/'), owner=0, clock=time.time, systemctl=None):
        self.root, self.owner, self.clock = Path(root), owner, clock
        self.systemctl = systemctl or self.real_systemctl
        self.fds = []
        self.pins = {}
        self.module = None
        self.proof = None

    def path(self, path):
        require(absolute(path))
        return self.root / path.lstrip('/')

    def trusted(self, path, private=False, directory=False, owner=None):
        owner = self.owner if owner is None else owner
        p = self.path(path)
        for current in (p, *p.parents):
            st = current.lstat()
            require(not stat.S_ISLNK(st.st_mode))
            if current == p:
                require(st.st_uid == owner and not st.st_mode & 0o022)
                require(stat.S_ISDIR(st.st_mode) if directory else stat.S_ISREG(st.st_mode))
                require(directory or st.st_nlink == 1)
                require(not private or not st.st_mode & 0o077)
            else:
                require(stat.S_ISDIR(st.st_mode) and st.st_uid in (self.owner, owner) and not st.st_mode & 0o022)
            if current == self.root:
                break
            require(self.root in current.parents)
        return p

    def open_pin(self, path, expected=None, private=True, limit=65536):
        p = self.trusted(path, private=private)
        before = p.lstat()
        fd = os.open(p, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC)
        self.fds.append(fd)
        opened = os.fstat(fd)
        require(snapshot(before) == snapshot(opened) and opened.st_size <= limit)
        if limit > 262144:
            hasher, offset = hashlib.sha256(), 0
            while offset < opened.st_size:
                block = os.pread(fd, min(1048576, opened.st_size - offset), offset)
                require(block)
                hasher.update(block)
                offset += len(block)
            data, sha = b'', hasher.hexdigest()
        else:
            data = os.pread(fd, limit + 1, 0)
            require(len(data) <= limit)
            sha = digest(data)
        require(snapshot(opened) == snapshot(os.fstat(fd)))
        require(expected is None or sha == expected)
        self.pins[path] = (fd, snapshot(opened), sha, limit)
        return fd, data

    def close(self):
        for fd in reversed(self.fds):
            os.close(fd)
        self.fds = []

    def real_systemctl(self, unit):
        props = 'LoadState,ActiveState,SubState,MainPID,ControlPID,Job,NeedDaemonReload,BindsTo,Restart,User,Group,ControlGroup'
        completed = subprocess.run(['/usr/bin/systemctl', 'show', unit, '--no-pager', '--property=' + props],
                                   check=True, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                                   text=True, timeout=5, env={'PATH': '/usr/bin:/bin', 'LANG': 'C'})
        require(len(completed.stdout) < 32768)
        return unique(line.split('=', 1) for line in completed.stdout.splitlines())

    def exec_conditions(self, unit):
        # Reuse the pinned runtime's read-only bus custody: RefUnit prevents an
        # inactive unit being garbage-collected between GetUnit and the query.
        # busctl emits typed JSON; missing support or malformed data refuses.
        require(unit in WRITERS)
        bus = self.module.SystemdBus()
        try:
            bus.manager('RefUnit', unit)
            try:
                path = bus.manager('GetUnit', unit)
                completed = subprocess.run([
                    '/usr/bin/busctl', '--system', '--json=short', 'get-property',
                    'org.freedesktop.systemd1', path,
                    'org.freedesktop.systemd1.Service', 'ExecCondition'],
                    check=True, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                    timeout=5, env={'PATH': '/usr/bin:/bin', 'LANG': 'C'})
                require(len(completed.stdout) < 32768)
                return decode_exec_conditions(completed.stdout)
            finally:
                bus.manager('UnrefUnit', unit)
        finally:
            bus.close()

    def hold(self):
        return self.proof['maintenance_hold' if self.proof['version'] == 3 else 'hold']

    def check_time(self):
        p, now = self.proof, int(self.clock())
        require(type(p['issued_at']) is int and type(p['expires_at']) is int)
        require(0 <= now - p['issued_at'] <= 300 and 0 < p['expires_at'] - now <= 300)
        require(0 < p['expires_at'] - p['issued_at'] <= 300)
        if p['version'] == 3:
            require(now < self.hold()['expires_at'] and p['expires_at'] <= self.hold()['expires_at'])

    def validate_proof(self, data):
        p = decode(data)
        require(isinstance(p, dict) and type(p.get('version')) is int)
        if p['version'] == 1:
            require(set(p) == PROOF_FIELDS)
            require(p['purpose'] == 'serenity-episode-fixture-offline-v1')
            require(p['operation'] in ('prepare', 'reissue', 'revoke'))
        elif p['version'] == 2:
            require(set(p) == PROOF_FIELDS | {'source_adoption'})
            require(p['purpose'] == 'serenity-episode-fixture-offline-v2' and p['operation'] == 'adopt-source')
            adoption = p['source_adoption']
            require(type(adoption) is dict and set(adoption) == ADOPTION_FIELDS)
            require(isinstance(adoption['old_source'], str) and re.fullmatch('[0-9a-f]{40}', adoption['old_source']))
            require(isinstance(adoption['old_manifest_sha256'], str) and re.fullmatch('[0-9a-f]{64}', adoption['old_manifest_sha256']))
            require(adoption['old_source'] != p['tool_source'])
        else:
            adoption = p.get('source_adoption')
            require(p['version'] == 3 and p['purpose'] == 'serenity-episode-fixture-offline-v3')
            require(p['operation'] in ('prepare', 'reissue', 'revoke', 'adopt-source'))
            require(set(p) == (V3_PROOF_FIELDS | {'source_adoption'} if p['operation'] == 'adopt-source' else V3_PROOF_FIELDS))
            if p['operation'] == 'adopt-source':
                require(type(adoption) is dict and set(adoption) == ADOPTION_FIELDS)
                require(isinstance(adoption['old_source'], str) and re.fullmatch('[0-9a-f]{40}', adoption['old_source']) and adoption['old_source'] != p['tool_source'])
                require(isinstance(adoption['old_manifest_sha256'], str) and re.fullmatch('[0-9a-f]{64}', adoption['old_manifest_sha256']))
        for key in PROOF_FIELDS:
            if key.endswith('_sha256') or key == 'nonce':
                require(isinstance(p[key], str) and re.fullmatch('[0-9a-f]{64}', p[key]))
            if key.endswith('_path') or key in ('data_dir', 'private_dir'):
                require(absolute(p[key]))
        for key in ('authority_scope', 'authority_session_id', 'concurrency_target'):
            require(isinstance(p[key], str) and 0 < len(p[key]) <= 256)
        for key in ('tool_source', 'service_source'):
            require(isinstance(p[key], str) and re.fullmatch('[0-9a-f]{40}', p[key]))
        require(re.fullmatch('[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}', p['boot_id']))
        for key in ('uid', 'gid', 'account_cap', 'lock_device', 'lock_inode'):
            require(type(p[key]) is int and p[key] > 0)
        require(p['config_path'] == HOSTED and p['private_dir'] != p['data_dir'])
        require(not p['private_dir'].startswith(p['data_dir'].rstrip('/') + '/brains/'))
        h = p['maintenance_hold' if p['version'] == 3 else 'hold']
        if p['version'] == 3:
            require(type(h) is dict and set(h) == V3_HOLD_FIELDS)
            require(type(h['version']) is int and h['version'] == 1 and h['purpose'] == 'serenity.fixture-maintenance/v1')
            require(isinstance(h['operation_id'], str) and re.fullmatch('[0-9a-f]{64}', h['operation_id']))
            for key in V3_HOLD_FIELDS:
                if key.endswith('_sha256'):
                    require(isinstance(h[key], str) and re.fullmatch('[0-9a-f]{64}', h[key]))
            require(h['stage'] == '/var/lib/serenity-release/maintenance-' + h['operation_id'])
            require(h['supervisor'] == 'serenity-release-' + h['operation_id'] + '.service')
            require(h['phase'] == 'maintenance-ready' and h['backup_timer_prior_state'] == 'active')
            require(type(h['timestamp']) is int and 0 < h['timestamp'] <= p['issued_at'])
            require(type(h['expires_at']) is int and 0 < h['expires_at'] - h['timestamp'] <= 3600 and p['expires_at'] <= h['expires_at'])
            require(h['service_source'] == p['service_source'] and h['executable_sha256'] == p['service_binary_sha256'])
            require(h['state_sha256'] == p['state_sha256'] and h['config_sha256'] == p['config_sha256'])
            require(p['runtime_path'] == h['stage'] + '/runtime.py')
        else:
            require(isinstance(h, dict) and set(h) == HOLD_FIELDS)
            require(h['phase'] == 'maintenance-ready' and h['backup_timer_prior_state'] == 'active')
            require(type(h['timestamp']) is int and 0 < h['timestamp'] <= p['issued_at'])
            for key in HOLD_FIELDS:
                if key.endswith('_sha256'):
                    require(isinstance(h[key], str) and re.fullmatch('[0-9a-f]{64}', h[key]))
            require(re.fullmatch('[0-9a-f]{40}', h['source']))
            require(absolute(h['stage']) and h['stage'].startswith('/var/lib/serenity-release/'))
            require(re.fullmatch('serenity-release-[a-zA-Z0-9_.-]+\\.service', h['supervisor']))
            require(p['runtime_path'] == h['stage'] + '/activation-runtime.py')
            require(p['state_sha256'] == h['previous_state_sha256'])
        self.proof = p
        self.check_time()
        return p

    def load_runtime(self):
        p = self.proof
        # Compile only the already pinned bytes, not a cached .pyc or a second
        # pathname read. This is existing reviewed root code, not proof content.
        _, data = self.open_pin(p['runtime_path'], p['runtime_sha256'], private=False, limit=262144)
        spec = importlib.util.spec_from_loader('fixture_pinned_activation', loader=None)
        module = importlib.util.module_from_spec(spec)
        exec(compile(data, p['runtime_path'], 'exec'), module.__dict__)
        self.module = module
        c, h = module.CONFIG, self.hold()
        if p['version'] == 3:
            require(type(c) is dict and c['manifest_sha256'] == h['manifest_sha256'])
            require(c['operation_id'] == h['operation_id'] and c['stage'] == h['stage'] and c['supervisor'] == h['supervisor'])
            require(c['purpose'] == h['purpose'] and c['version'] == h['version'] and c['expires_at'] == h['expires_at'])
            service, fixture = c['service'], c['fixture']
            require(service['source'] == p['service_source'] == h['service_source'])
            require(service['binary_path'] == p['service_binary_path'] and service['binary_sha256'] == p['service_binary_sha256'] == h['executable_sha256'])
            require(service['state_sha256'] == p['state_sha256'] and service['config_sha256'] == p['config_sha256'] and service['broker_sha256'] == h['broker_sha256'])
            for key in ('tool_source', 'tool_path', 'tool_sha256', 'wrapper_path', 'wrapper_sha256', 'uid', 'gid', 'data_dir', 'private_dir', 'account_cap', 'schema_sha256'):
                require(fixture[key] == p[key])
            require(c['authority']['generator_path'] == p['authority_generator_path'] and c['authority']['generator_sha256'] == p['authority_generator_sha256'])
            require(c['boot_id'] == p['boot_id'] and c['issued_at'] <= p['issued_at'])
            self.open_pin('/etc/serenity-authority/broker.json', h['broker_sha256'])
        else:
            for key in ('manifest_sha256', 'executable_sha256', 'previous_state_sha256', 'expected_state_sha256', 'supervisor', 'stage'):
                require(c[key] == h[key])
            require(c['commit'] == h['source'] and c['previous_commit'] == p['service_source'])
            require(c['previous_executable_sha256'] == p['service_binary_sha256'])
            require(c['hosted_config_sha256'] == p['config_sha256'])

    def validate_v3_runtime(self):
        transaction = self.module.Transaction(self.module.CONFIG, root=self.root, uid=self.owner, gid=os.getegid(), clock=self.clock)
        transaction.authority()
        transaction.pair()
        require(decode(transaction.matching_hold()) == self.hold())
        transaction.qualified_guards(self.hold()['supervisor'])

    def check_units(self):
        p, h = self.proof, self.hold()
        if p['version'] == 3:
            self.validate_v3_runtime()
        for unit in UNITS:
            v = self.systemctl(unit)
            require(v['LoadState'] == 'loaded' and v['ActiveState'] == 'inactive' and v['SubState'] == 'dead')
            require(v['NeedDaemonReload'] == 'no' and v['Job'] in ('', '0'))
            if unit != TIMER:
                require(v['MainPID'] == '0' and v['ControlPID'] == '0')
                group = v['ControlGroup']
                if group:
                    require(group.startswith('/') and '..' not in Path(group).parts)
                    require(not self.path('/sys/fs/cgroup' + group + '/cgroup.procs').read_text().strip())
            if unit in WRITERS:
                exact_guard(self.exec_conditions(unit), p['runtime_path'], unit)
                require(h['supervisor'] in v['BindsTo'].split() and v['Restart'] == 'no')
            else:
                require(self.module.mandatory_negated_condition(self.module.unit_conditions(unit), HOLD))
        hosted = self.systemctl(WRITERS[0])
        require(hosted['User'] == 'serenity' and hosted['Group'] == 'serenity')

    def absent(self, path):
        try:
            self.path(path).lstat()
        except FileNotFoundError:
            return
        require(False)

    def refresh(self):
        self.check_time()
        for path, (fd, saved, sha, limit) in self.pins.items():
            require(snapshot(self.path(path).lstat()) == saved == snapshot(os.fstat(fd)))
            if limit <= 262144:
                require(digest(os.pread(fd, limit + 1, 0)) == sha)
        require(self.path('/proc/sys/kernel/random/boot_id').read_text().strip() == self.proof['boot_id'])
        if self.path(RUN).exists():
            self.trusted(RUN, private=True, directory=True)
        else:
            self.absent(RUN)
        for unit in WRITERS:
            self.absent(RUN + '/' + unit + '.permit')
        self.absent(STATE + '.pending')
        self.absent(self.hold()['stage'] + '/closeout-pending.json')
        link = self.path('/usr/local/bin/serenity')
        if self.proof['service_binary_path'] != '/usr/local/bin/serenity':
            require(link.is_symlink() and link.lstat().st_uid == self.owner and os.readlink(link) == self.proof['service_binary_path'])
        self.check_units()

    def prepare(self, proof_path):
        # Same stable lock as the established release transaction, retained until
        # the child has exited AND all final checks have completed.
        lock_path = self.trusted(STATE + '.lock', private=True)
        lock = os.open(lock_path, os.O_RDWR | os.O_NOFOLLOW | os.O_CLOEXEC)
        self.fds.append(lock)
        require(snapshot(lock_path.lstat()) == snapshot(os.fstat(lock)))
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        # Separate open description proves an inherited lock already exists.
        probe = os.open(lock_path, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
        self.fds.append(probe)
        require(snapshot(os.fstat(lock)) == snapshot(os.fstat(probe)))
        proof_fd, data = self.open_pin(proof_path)
        p = self.validate_proof(data)
        require(os.fstat(lock).st_dev == p['lock_device'] and os.fstat(lock).st_ino == p['lock_inode'])
        self.pins[STATE + '.lock'] = (lock, snapshot(os.fstat(lock)), digest(os.pread(lock, 65537, 0)), 65536)
        self.trusted(str(Path(proof_path).parent), private=True, directory=True)
        hold_fd, hold_bytes = self.open_pin(HOLD, p['hold_sha256'])
        require(decode(hold_bytes) == self.hold())
        state_fd, _ = self.open_pin(STATE, p['state_sha256'])
        config_fd, config_bytes = self.open_pin(HOSTED, p['config_sha256'], private=False)
        config = decode(config_bytes)
        require(config['data_dir'] == p['data_dir'] and type(config['account_cap']) is int and config['account_cap'] == p['account_cap'])
        require(config.get('episode_archive_enabled', False) is False and config.get('episode_synthetic_brains', []) == [])
        if p['version'] == 3:
            require(config.get('workos_enabled', False) is False and config.get('billing_enabled', False) is False)
        self.open_pin(p['wrapper_path'], p['wrapper_sha256'], private=False, limit=262144)
        self.open_pin(p['tool_path'], p['tool_sha256'], private=False, limit=268435456)
        self.open_pin(p['service_binary_path'], p['service_binary_sha256'], private=False, limit=268435456)
        link = self.path('/usr/local/bin/serenity')
        if link.is_symlink():
            st = link.lstat()
            require(st.st_uid == self.owner and os.readlink(link) == p['service_binary_path'])
            self.trusted('/usr/local/bin', directory=True)
        else:
            require(p['service_binary_path'] == '/usr/local/bin/serenity')
        self.trusted(self.hold()['stage'], private=True, directory=True)
        self.load_runtime()
        _, ready_bytes = self.open_pin(self.hold()['stage'] + '/maintenance-ready.json', p['ready_sha256'])
        if p['version'] == 3:
            expected_ready = dict(version=2, purpose='serenity.fixture-maintenance/v1', operation_id=self.hold()['operation_id'], manifest_sha256=self.hold()['manifest_sha256'], state_sha256=p['state_sha256'], hold_sha256=p['hold_sha256'], service_source=p['service_source'], executable_sha256=p['service_binary_sha256'], held=True, services_started=False)
        else:
            expected_ready = dict(manifest_sha256=p['hold']['manifest_sha256'], state_sha256=p['state_sha256'], installed_pair='previous-unmodified', held=True, services_started=False)
        require(decode(ready_bytes) == expected_ready)
        self.open_pin(p['authority_generator_path'], p['authority_generator_sha256'], private=False, limit=262144)
        _, receipt_bytes = self.open_pin(p['authority_receipt_path'], p['authority_receipt_sha256'])
        a = decode(receipt_bytes)
        require(isinstance(a, dict) and set(a) == AUTHORITY_FIELDS and type(a['version']) is int and a['version'] == 1)
        require(a['status'] == 'CLOSED' and type(a['active_leases']) is int and a['active_leases'] == 0 and type(a['concurrency']) is int and a['concurrency'] == 0)
        require(type(a['observed_at']) is int and p['issued_at'] - 60 <= a['observed_at'] <= p['issued_at'])
        for key in ('boot_id', 'hold_sha256', 'state_sha256'):
            require(a[key] == p[key])
        require(a['scope'] == p['authority_scope'] and a['session_id'] == p['authority_session_id'] and a['concurrency_target'] == p['concurrency_target'])
        require(isinstance(a['request_ids'], list) and 1 <= len(a['request_ids']) <= 8 and all(isinstance(v, str) and 0 < len(v) <= 256 for v in a['request_ids']))
        for unit in UNITS:
            base = '/etc/systemd/system/' + unit + '.d/'
            if unit in WRITERS:
                expected = MARKER + '[Service]\nExecCondition=+/usr/bin/python3 ' + p['runtime_path'] + ' guard ' + unit + '\n'
            else:
                expected = MARKER + '[Unit]\nConditionPathExists=!' + HOLD + '\n'
            _, data = self.open_pin(base + '90-serenity-release-guard.conf', private=False)
            require(data == expected.encode())
            if unit in WRITERS:
                _, data = self.open_pin(base + '91-serenity-release-maintenance.conf', private=False)
                require(data == (MARKER + '[Unit]\nBindsTo=' + self.hold()['supervisor'] + '\nAfter=' + self.hold()['supervisor'] + '\n[Service]\nRestart=no\n').encode())
        for path in (p['data_dir'], p['data_dir'] + '/brains', p['private_dir']):
            self.trusted(path, private=True, directory=True, owner=p['uid'])
        for suffix in ('', '-wal', '-shm'):
            path = p['data_dir'] + '/control.db' + suffix
            if suffix and not self.path(path).exists():
                self.absent(path)  # dangling symlinks are not absence
                continue
            self.trusted(path, private=True, owner=p['uid'])
        self.refresh()
        return (proof_fd, hold_fd, state_fd, config_fd, lock, probe)

    def consume(self, proof_path):
        # Durable one-use packet. Even a failed child needs a fresh proof/receipt.
        path = str(Path(self.hold()['stage']) / ('.fixture-used-' + self.proof['nonce']))
        fd = os.open(self.path(path), os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        try:
            os.write(fd, b'consumed\n')
            os.fsync(fd)
        finally:
            os.close(fd)
        directory = os.open(self.path(str(Path(path).parent)), os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)

    def save_v3_proof(self, proof_path):
        if self.proof['version'] != 3:
            return
        fd, _, expected, limit = self.pins[proof_path]
        raw = os.pread(fd, limit + 1, 0)
        require(len(raw) <= limit and digest(raw) == expected)
        self.publish_private_record('fixture-proof-' + self.proof['nonce'] + '.json', raw)

    def publish_private_record(self, name, raw):
        final = self.path(self.hold()['stage'] + '/' + name)
        pending = Path(str(final) + '.pending')
        require(not os.path.lexists(final))
        fd = os.open(pending, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        try:
            require(os.write(fd, raw) == len(raw)); os.fsync(fd)
        finally:
            os.close(fd)
        os.link(pending, final, follow_symlinks=False)
        os.unlink(pending)
        directory = os.open(final.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try: os.fsync(directory)
        finally: os.close(directory)

    def outcome_sequence(self, current_nonce=None):
        """Validate the complete prior bijection while the actual EX lock is held.
        Historical proof expiry is not renewed; its recorded completion was inside
        its own window. Unknown/refused/uncertain history blocks the next child.
        """
        p, h = self.proof, self.hold()
        used, proofs, outcomes = set(), set(), set()
        with os.scandir(self.path(h['stage'])) as entries:
            for count, entry in enumerate(entries, 1):
                require(count <= 4096)
                for prefix, suffix, target in (('.fixture-used-', '', used), ('fixture-proof-', '.json', proofs), ('fixture-outcome-', '.json', outcomes)):
                    if entry.name.startswith(prefix):
                        nonce = entry.name[len(prefix):len(entry.name)-len(suffix) if suffix else None]
                        require((not suffix or entry.name.endswith(suffix)) and re.fullmatch('[0-9a-f]{64}', nonce))
                        target.add(nonce)
        if current_nonce is not None:
            require(current_nonce in used and current_nonce in proofs and current_nonce not in outcomes)
            used.remove(current_nonce); proofs.remove(current_nonce)
        require(used == proofs == outcomes)
        sequences = set()
        for nonce in used:
            _, consumed = self.open_pin(h['stage'] + '/.fixture-used-' + nonce)
            require(consumed == b'consumed\n')
            _, proof_raw = self.open_pin(h['stage'] + '/fixture-proof-' + nonce + '.json')
            previous = decode(proof_raw)
            validator = Adapter(root=self.root, owner=self.owner, clock=lambda: previous['issued_at'])
            validator.validate_proof(proof_raw)
            require(previous['version'] == 3 and previous['maintenance_hold'] == h and previous['hold_sha256'] == p['hold_sha256'])
            require(previous['nonce'] == nonce and previous['tool_source'] == p['tool_source'] and previous['tool_sha256'] == p['tool_sha256'])
            _, raw = self.open_pin(h['stage'] + '/fixture-outcome-' + nonce + '.json')
            outcome = decode(raw)
            require(type(outcome) is dict and set(outcome) == set('version purpose operation_id nonce operation proof_sha256 tool_source tool_sha256 completed_at result sequence'.split()))
            require(type(outcome['version']) is int and outcome['version'] == 1 and outcome['purpose'] == p['purpose'] and outcome['operation_id'] == h['operation_id'])
            require(outcome['nonce'] == nonce and outcome['operation'] == previous['operation'] and outcome['proof_sha256'] == digest(proof_raw))
            require(outcome['tool_source'] == p['tool_source'] and outcome['tool_sha256'] == p['tool_sha256'] and outcome['result'] == 'success')
            require(type(outcome['completed_at']) is int and previous['issued_at'] <= outcome['completed_at'] < previous['expires_at'] and outcome['completed_at'] <= int(self.clock()))
            require(type(outcome['sequence']) is int and outcome['sequence'] > 0 and outcome['sequence'] not in sequences)
            sequences.add(outcome['sequence'])
        require(sequences == set(range(1, len(used) + 1)))
        return len(used) + 1

    def publish_outcome(self, result):
        p = self.proof
        if p['version'] != 3:
            return
        require(result in ('success', 'refused', 'uncertain'))
        self.refresh()
        value = dict(version=1, purpose=p['purpose'], operation_id=self.hold()['operation_id'], nonce=p['nonce'], operation=p['operation'], proof_sha256=self.pins[self.proof_path][2], tool_source=p['tool_source'], tool_sha256=p['tool_sha256'], completed_at=int(self.clock()), result=result, sequence=self.outcome_sequence(p['nonce']))
        raw = json.dumps(value, sort_keys=True, separators=(',', ':')).encode()
        self.publish_private_record('fixture-outcome-' + p['nonce'] + '.json', raw)

    def execute(self, proof_path):
        require(os.geteuid() == 0 and sys.flags.isolated == 1 and self.root == Path('/') and self.owner == 0)
        self.proof_path = proof_path
        fds = self.prepare(proof_path)
        p = self.proof
        account = pwd.getpwnam('serenity')
        require(account.pw_uid == p['uid'] and account.pw_gid == p['gid'] and account.pw_uid != 0)
        require(str(Path(__file__).absolute()) == p['wrapper_path'])
        if p['version'] == 3:
            self.outcome_sequence()
        self.save_v3_proof(proof_path)
        self.consume(proof_path)
        self.refresh()
        child = None
        try:
            child = subprocess.Popen([p['tool_path'], '--custody-fds', ','.join(str(fd) for fd in fds)],
                user=p['uid'], group=p['gid'], extra_groups=[], pass_fds=fds,
                stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                env={'PATH': '/usr/bin:/bin', 'LANG': 'C', 'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': '/dev/null'},
                cwd=p['private_dir'], umask=0o077, start_new_session=True)
            deadline = time.monotonic() + 120
            while child.poll() is None:
                require(time.monotonic() < deadline)
                self.refresh()
                time.sleep(0.2)
            if p['operation'] == 'adopt-source':
                self.refresh()
                if child.returncode in ADOPTION_OUTCOMES:
                    self.publish_outcome('uncertain' if child.returncode == 6 else 'refused')
                    raise AdoptionOutcome(child.returncode)
            if child.returncode != 0:
                self.publish_outcome('uncertain')
            require(child.returncode == 0)
            self.refresh()
            self.publish_outcome('success')
        finally:
            if child is not None:
                # A still-running/unreaped child anchors its process-group ID;
                # never kill a group after reaping and possible PID reuse.
                if child.poll() is None:
                    try:
                        os.killpg(child.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                child.wait(timeout=5)


class AdoptionOutcome(RuntimeError):
    """Recognized child outcome; no retry or replacement proof means success."""
    def __init__(self, code):
        self.code = code
        super().__init__(ADOPTION_OUTCOMES[code])


def main(argv):
    adapter = Adapter()
    try:
        require(len(argv) == 2 and absolute(argv[1]))
        for sig in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP, signal.SIGALRM):
            signal.signal(sig, lambda *_: (_ for _ in ()).throw(RuntimeError('interrupted')))
        signal.alarm(180)
        adapter.execute(argv[1])
        print('fixture operation completed; synthetic accounts/data retained; capture unchanged')
        return 0
    except AdoptionOutcome as outcome:
        print(str(outcome) + '; proof consumed', file=sys.stderr)
        return outcome.code
    except BaseException:
        if adapter.proof and adapter.proof['operation'] == 'adopt-source':
            print('UNCERTAIN: source adoption did not complete custody checks; publication not established; retain hold and reconcile before fresh proof', file=sys.stderr)
            return 6
        print('REFUSED: fixture custody or operation failed; retain hold for reconciliation', file=sys.stderr)
        return 1
    finally:
        signal.alarm(0)
        adapter.close()


if __name__ == '__main__':
    sys.exit(main(sys.argv))
