#!/usr/bin/env python3
"""Inert same-release maintenance owner; rendering is not operational authority.

Typed systemd custody adapted from the generic donor in provenance.json.
There is no release transition, SQLite access, cloud mutation or rollback here.
"""
import base64
import contextlib
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import pwd
import grp
import signal
import stat
import subprocess
import sys
import time
import urllib.request

CONFIG = None
WRITERS = ('serenity-hosted.service', 'serenity-broker.service')
TIMER = 'serenity-backup.timer'
BACKUP = 'serenity-backup.service'
UNITS = (*WRITERS, BACKUP, TIMER)
HOLD = '/var/lib/serenity-release/maintenance.hold.json'
STATE = '/var/lib/serenity-release/state.json'
BROKER = '/etc/serenity-authority/broker.json'
HOSTED = '/etc/serenity/hosted.json'
RUN = '/run/serenity-release-maintenance'
MARKER = '# serenity-release-managed-v1\n'
PURPOSE = 'serenity.fixture-maintenance/v1'
PROOF_FIELDS = set('version purpose operation nonce issued_at expires_at boot_id uid gid tool_source tool_path tool_sha256 wrapper_path wrapper_sha256 runtime_path runtime_sha256 service_binary_path service_binary_sha256 service_source config_path config_sha256 data_dir private_dir account_cap hold_sha256 maintenance_hold state_sha256 ready_sha256 authority_receipt_path authority_receipt_sha256 authority_generator_path authority_generator_sha256 lock_device lock_inode schema_sha256 authority_scope authority_session_id concurrency_target'.split())
HEX = re.compile(r'[0-9a-f]{64}\Z')


def require(value, message='maintenance custody refused'):
    if not value:
        raise RuntimeError(message)


def encoded(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), allow_nan=False).encode()


def digest(data):
    return hashlib.sha256(data).hexdigest()


def decoded(raw):
    def pairs(rows):
        result = {}
        for key, value in rows:
            require(key not in result)
            result[key] = value
        return result
    return json.loads(raw, object_pairs_hook=pairs, parse_constant=lambda _: require(False))


def exact(value, fields):
    require(isinstance(value, dict) and set(value) == set(fields.split()))
    return value


class SystemdBus:
    """One private sd-bus connection; no dependency on Python D-Bus packages.

    RefUnit atomically loads and pins the unit on this connection in systemd 252.
    Closing the connection also drops sender references if explicit Unref fails.
    No start/stop/job/property-write operation is exposed here.
    """
    DEST = b'org.freedesktop.systemd1'
    MANAGER = b'/org/freedesktop/systemd1'
    INTERFACE = b'org.freedesktop.systemd1.Manager'

    def __init__(self):
        import ctypes as C
        self.C = C
        self.lib = C.CDLL('libsystemd.so.0')
        P, S, I = C.c_void_p, C.c_char_p, C.c_int
        # Explicit ABI declarations, including fixed arguments of the variadic call.
        signatures = {
            'sd_bus_open_system': ([C.POINTER(P)], I),
            'sd_bus_set_method_call_timeout': ([P, C.c_uint64], I),
            'sd_bus_call_method': ([P, S, S, S, S, P, C.POINTER(P), S], I),
            'sd_bus_get_property': ([P, S, S, S, S, P, C.POINTER(P), S], I),
            'sd_bus_message_read_basic': ([P, C.c_char, P], I),
            'sd_bus_message_enter_container': ([P, C.c_char, S], I),
            'sd_bus_message_exit_container': ([P], I),
            'sd_bus_message_at_end': ([P, I], I),
            'sd_bus_message_unref': ([P], P),
            'sd_bus_flush_close_unref': ([P], P),
        }
        for name, (args, result) in signatures.items():
            fn = getattr(self.lib, name)
            fn.argtypes, fn.restype = args, result
        self.bus = P()
        try:
            self.checked(self.lib.sd_bus_open_system(C.byref(self.bus)), 'open system bus')
            self.checked(self.lib.sd_bus_set_method_call_timeout(self.bus, 15_000_000), 'set bus timeout')
        except BaseException:
            self.close()
            raise

    @staticmethod
    def checked(result, operation):
        if result < 0:
            raise RuntimeError('systemd D-Bus ' + operation + ': ' + os.strerror(-result))
        return result

    def close(self):
        if self.bus:
            self.lib.sd_bus_flush_close_unref(self.bus)
            self.bus = self.C.c_void_p()

    def basic(self, message, kind):
        value = self.C.c_char_p() if kind in (b's', b'o') else self.C.c_int()
        require(self.checked(self.lib.sd_bus_message_read_basic(message, kind, self.C.byref(value)),
                             'read typed value') > 0, 'missing typed D-Bus value')
        if kind in (b's', b'o'):
            require(value.value is not None, 'null D-Bus string')
            return value.value.decode('utf-8')
        require(kind != b'b' or value.value in (0, 1), 'invalid D-Bus boolean')
        return bool(value.value) if kind == b'b' else value.value

    def manager(self, method, unit):
        reply = self.C.c_void_p()
        try:
            self.checked(self.lib.sd_bus_call_method(self.bus, self.DEST, self.MANAGER,
                self.INTERFACE, method.encode('ascii'), None, self.C.byref(reply), b's',
                self.C.c_char_p(unit.encode('utf-8'))), method)
            if method == 'GetUnit':
                path = self.basic(reply, b'o')
                require(path.startswith('/org/freedesktop/systemd1/unit/'), 'unexpected unit D-Bus path')
                require(self.checked(self.lib.sd_bus_message_at_end(reply, 1), 'path end') > 0,
                        'extra GetUnit result')
                return path
        finally:
            self.lib.sd_bus_message_unref(reply)

    def conditions(self, path):
        reply = self.C.c_void_p()
        try:
            self.checked(self.lib.sd_bus_get_property(self.bus, self.DEST, path.encode('utf-8'),
                b'org.freedesktop.systemd1.Unit', b'Conditions', None, self.C.byref(reply),
                b'a(sbbsi)'), 'get typed Conditions')
            require(self.checked(self.lib.sd_bus_message_enter_container(reply, b'a', b'(sbbsi)'),
                                 'enter Conditions') > 0, 'missing Conditions array')
            rows = []
            while True:
                result = self.checked(self.lib.sd_bus_message_enter_container(reply, b'r', b'sbbsi'),
                                      'enter condition')
                if result == 0:
                    break
                require(len(rows) < 4096, 'too many Conditions')
                rows.append(tuple(self.basic(reply, kind) for kind in (b's', b'b', b'b', b's', b'i')))
                require(self.checked(self.lib.sd_bus_message_at_end(reply, 0), 'condition end') > 0,
                        'extra condition fields')
                self.checked(self.lib.sd_bus_message_exit_container(reply), 'exit condition')
            self.checked(self.lib.sd_bus_message_exit_container(reply), 'exit Conditions')
            # sd_bus_get_property leaves the cursor INSIDE its typed variant.
            self.checked(self.lib.sd_bus_message_exit_container(reply), 'exit property variant')
            require(self.checked(self.lib.sd_bus_message_at_end(reply, 1), 'Conditions end') > 0,
                    'extra Conditions data')
            return rows
        finally:
            self.lib.sd_bus_message_unref(reply)


def unit_conditions(unit, bus_factory=SystemdBus):
    """Read effective typed conditions even if an inactive unit was already GC'd.

    No LoadUnit/GetUnit race: RefUnit loads AND acquires the sender reference.
    Keep that same connection open through GetUnit/property/UnrefUnit. Ref failure,
    missing library, access denial or malformed property fail closed. The outer
    close runs even on signal/EXIT and releases references on process death too.
    """
    require(isinstance(unit, str) and unit and '\x00' not in unit, 'invalid unit name')
    bus = bus_factory()
    try:
        bus.manager('RefUnit', unit)
        try:
            return bus.conditions(bus.manager('GetUnit', unit))
        finally:
            bus.manager('UnrefUnit', unit)
    finally:
        bus.close()


def unit_exec_conditions(unit, bus_factory=SystemdBus):
    """Typed argv read under one sender reference; never parse display text."""
    require(unit in WRITERS, 'unexpected writer unit')
    bus = bus_factory()
    try:
        bus.manager('RefUnit', unit)
        try:
            path = bus.manager('GetUnit', unit)
            result = Runner().run(['/usr/bin/busctl', '--system', '--json=short',
                'get-property', 'org.freedesktop.systemd1', path,
                'org.freedesktop.systemd1.Service', 'ExecCondition'])
            require(result.returncode == 0 and len(result.stdout) <= 65536,
                    'typed ExecCondition unavailable')
            value = decoded(result.stdout)
            require(isinstance(value, dict) and set(value) == {'type', 'data'} and
                    value['type'] == 'a(sasbttttuii)', 'unexpected ExecCondition signature')
            return value['data']
        finally:
            bus.manager('UnrefUnit', unit)
    finally:
        bus.close()


def exact_startup_guard(rows, runtime, unit):
    if not isinstance(rows, list) or len(rows) != 1:
        return False
    row = rows[0]
    return (isinstance(row, list) and len(row) == 10 and
            row[0] == '/usr/bin/python3' and
            row[1] == ['/usr/bin/python3', runtime, 'guard', unit] and
            row[2] is False and all(type(value) is int for value in row[3:]))


def mandatory_negated_condition(rows, path):
    return any(row[:4] == ('ConditionPathExists', False, True, path) for row in rows)


class Runner:
    def run(self, argv, *, pass_fds=(), check=True, uid=None, gid=None):
        # systemctl/busctl outputs are small by design, but pipe reads still have
        # explicit caps. No shell, saved stderr or environment test switches.
        import selectors
        credentials = {} if uid is None else dict(user=uid, group=gid, extra_groups=[])
        process = subprocess.Popen(argv, pass_fds=pass_fds, stdin=subprocess.DEVNULL,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE, **credentials)
        buffers = {process.stdout: bytearray(), process.stderr: bytearray()}
        deadline = time.monotonic() + 15
        try:
            with selectors.DefaultSelector() as selector:
                for stream in buffers:
                    os.set_blocking(stream.fileno(), False)
                    selector.register(stream, selectors.EVENT_READ)
                while selector.get_map():
                    remaining = deadline - time.monotonic()
                    require(remaining > 0)
                    for key, _ in selector.select(remaining):
                        data = os.read(key.fileobj.fileno(), 16384)
                        if not data:
                            selector.unregister(key.fileobj)
                        else:
                            buffers[key.fileobj].extend(data)
                            require(len(buffers[key.fileobj]) <= 262144)
                result = process.wait(timeout=max(.001, deadline - time.monotonic()))
            require(not check or result == 0)
            return subprocess.CompletedProcess(argv, result,
                bytes(buffers[process.stdout]).decode(), bytes(buffers[process.stderr]).decode())
        finally:
            if process.poll() is None:
                process.kill()
            process.wait()
            process.stdout.close()
            process.stderr.close()


class Transaction:
    def __init__(self, config, root=Path('/'), runner=None, uid=0, gid=0, clock=time.time):
        self.c = decoded(encoded(config))  # detach every caller-owned container
        self.root = Path(root)
        self.runner = runner or Runner()
        self.uid, self.gid, self.clock = uid, gid, clock
        self.fd = None
        self._gate = False
        self._authorization = None
        self._authorization_hold = None
        self.validate_config()
        self.stage = self.c['stage']
        self.runtime = self.stage + '/runtime.py'
        self.s = self.c['service']
        self.f = self.c['fixture']
        self.a = self.c['authority']

    def validate_config(self):
        c = self.c
        exact(c, 'version purpose operation_id issued_at expires_at boot_id stage supervisor service fixture authority manifest_sha256')
        require(type(c['version']) is int and c['version'] == 1 and c['purpose'] == PURPOSE and HEX.fullmatch(c['operation_id']))
        require(c['stage'] == '/var/lib/serenity-release/maintenance-' + c['operation_id'] and
                c['supervisor'] == 'serenity-release-' + c['operation_id'] + '.service')
        require(type(c['issued_at']) is int and type(c['expires_at']) is int and
                0 < c['expires_at'] - c['issued_at'] <= 3600 and isinstance(c['boot_id'], str) and
                re.fullmatch(r'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}', c['boot_id']))
        require(digest(encoded({k: v for k, v in c.items() if k != 'manifest_sha256'})) == c['manifest_sha256'])
        s = exact(c['service'], 'source binary_path binary_sha256 state_sha256 broker_sha256 config_sha256 installed_sequence highest_sequence release_manifest_sha256 startup_runtime_path startup_runtime_sha256 unit_files')
        f = exact(c['fixture'], 'tool_source tool_path tool_sha256 wrapper_path wrapper_sha256 uid gid data_dir private_dir account_cap schema_sha256 unit')
        a = exact(c['authority'], 'generator_path generator_sha256 scope session_id concurrency_target restore_concurrency expectations')
        for group in (s, f, a):
            for key, value in group.items():
                if key.endswith('_sha256'):
                    require(isinstance(value, str) and HEX.fullmatch(value))
                if key.endswith('_path') or key.endswith('_dir'):
                    require(isinstance(value, str) and value.startswith('/') and
                            '..' not in Path(value).parts and '\x00' not in value)
        require(re.fullmatch(r'[0-9a-f]{40}', s['source']) and
                re.fullmatch(r'[0-9a-f]{40}', f['tool_source']))
        require(type(s['installed_sequence']) is int and type(s['highest_sequence']) is int and
                0 < s['installed_sequence'] <= s['highest_sequence'])
        require(type(f['uid']) is int and type(f['gid']) is int and f['uid'] > 0 and f['gid'] > 0 and
                type(f['account_cap']) is int and f['account_cap'] == 2 and f['unit'] == 'serenity-episode-fixture-' + c['operation_id'] + '.service')
        require(not Path(f['private_dir']).is_relative_to(Path(f['data_dir']) / 'brains') and
                f['private_dir'] != f['data_dir'])
        require(type(a['restore_concurrency']) is int and a['restore_concurrency'] == 1)
        require(isinstance(a['scope'], str) and a['scope'] and isinstance(a['session_id'], str) and a['session_id'])
        exact(a['expectations'], 'cli_path cli_sha256 account principal region stack function version code_sha256 table namespace table_arn lineage witness_bucket metadata_sha256 tip_sha256 minimum_revision')
        require(isinstance(s['unit_files'], list) and 8 <= len(s['unit_files']) <= 32)
        names = set()
        for item in s['unit_files']:
            exact(item, 'path sha256')
            path = item['path']
            require(isinstance(path, str) and HEX.fullmatch(item['sha256']) and path not in names)
            require(any(path == base + '/' + unit or
                        (path.startswith(base + '/' + unit + '.d/') and
                         len(Path(path).parts) == len(Path(base).parts) + 2 and path.endswith('.conf'))
                        for base in ('/etc/systemd/system', '/usr/lib/systemd/system', '/lib/systemd/system')
                        for unit in UNITS))
            names.add(path)

    def p(self, name):
        require(isinstance(name, str) and name.startswith('/') and '..' not in Path(name).parts)
        return self.root / name.lstrip('/')

    def trusted(self, path, *, private=False, directory=False, owner=None):
        path = Path(path)
        current = path
        owner = self.uid if owner is None else owner
        while True:
            st = current.lstat()
            require(not stat.S_ISLNK(st.st_mode) and not st.st_mode & 0o022)
            require(st.st_uid == (owner if current == path else self.uid))
            if current == path:
                require(stat.S_ISDIR(st.st_mode) if directory else stat.S_ISREG(st.st_mode))
                require(directory or st.st_nlink == 1)
                require(not private or stat.S_IMODE(st.st_mode) == (0o700 if directory else 0o600))
            if current == self.root:
                break
            require(self.root in current.parents)
            current = current.parent
        return path

    def read(self, name, *, private=False, limit=1048576):
        path = self.trusted(self.p(name), private=private)
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
        try:
            before = os.fstat(fd)
            require(before.st_size <= limit and before.st_nlink == 1)
            raw = os.read(fd, limit + 1)
            after = os.fstat(fd)
            actual = path.lstat()
            require(len(raw) <= limit and (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns) ==
                    (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns) and
                    (actual.st_dev, actual.st_ino) == (after.st_dev, after.st_ino))
            return raw
        finally:
            os.close(fd)

    def sync_dir(self, path):
        fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)

    def checkpoint(self, name):
        """Import-only subclass seam for crash-boundary tests; no global hooks."""

    def before_mutation(self):
        if self._gate:
            self.authority(self._authorization, self._authorization_hold)
            self.pair()

    def write_record(self, fd, data, name):
        return os.write(fd, data)

    def atomic(self, name, data, mode=0o600, prior=None):
        self.before_mutation()
        path = self.p(name)
        self.trusted(path.parent, directory=True)
        if path.exists() or path.is_symlink():
            existing = self.read(name)
            if existing == data:
                self.sync_dir(path.parent)
                return
            require(prior is not None and existing == prior)
        staging_identity = '-' + digest(data) if path.name in ('closeout-pending.json', 'closed.json') else ''
        temporary = path.with_name('.' + path.name + staging_identity + '.maintenance-write')
        if temporary.exists() or temporary.is_symlink():
            prefix = self.read('/' + str(temporary.relative_to(self.root)), private=(mode == 0o600))
            require(data.startswith(prefix))
            fd = os.open(temporary, os.O_WRONLY | os.O_APPEND | os.O_NOFOLLOW)
        else:
            fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, mode)
            prefix = b''
        try:
            os.fchown(fd, self.uid, self.gid)
            os.fchmod(fd, mode)
            self.checkpoint('record-created:' + path.name)
            suffix = memoryview(data)[len(prefix):]
            while suffix:
                count = self.write_record(fd, suffix, path.name)
                require(count > 0)
                suffix = suffix[count:]
            os.fsync(fd)
            self.checkpoint('record-synced:' + path.name)
        finally:
            os.close(fd)
        os.replace(temporary, path)
        self.checkpoint('record-renamed:' + path.name)
        self.sync_dir(path.parent)
        self.checkpoint('record-durable:' + path.name)

    def mkdir(self, name):
        self.before_mutation()
        path = self.p(name)
        if path.exists() or path.is_symlink():
            self.trusted(path, directory=True)
            return
        self.trusted(path.parent, directory=True)
        path.mkdir(mode=0o700)
        os.chown(path, self.uid, self.gid)
        self.sync_dir(path)
        self.sync_dir(path.parent)

    def unlink_exact(self, name, raw):
        self.before_mutation()
        path = self.p(name)
        require(self.read(name) == raw)
        path.unlink()
        self.sync_dir(path.parent)
        self.checkpoint('unlinked:' + path.name)

    @contextlib.contextmanager
    def lock(self):
        path = self.trusted(self.p(STATE + '.lock'), private=True)
        fd = os.open(path, os.O_RDWR | os.O_NOFOLLOW)
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
            require(os.path.samestat(os.fstat(fd), path.lstat()))
            self.fd = fd
            yield
        finally:
            self.fd = None
            os.close(fd)

    def command(self, *argv, check=True):
        result = self.runner.run(list(argv), check=check)
        require(isinstance(result.stdout, str) and len(result.stdout) <= 262144)
        require(not check or result.returncode == 0)
        return result

    def systemctl(self, *args):
        self.before_mutation()
        return self.command('/usr/bin/systemctl', *args)

    def read_unit_conditions(self, unit):
        return unit_conditions(unit)

    def read_unit_exec_conditions(self, unit):
        return unit_exec_conditions(unit)

    def property(self, bus, path, interface, name, signature):
        result = self.command('/usr/bin/busctl', '--system', '--json=short', 'get-property',
                              'org.freedesktop.systemd1', path, interface, name)
        value = exact(decoded(result.stdout), 'type data')
        require(value['type'] == signature)
        data = value['data']
        if signature == 's':
            require(isinstance(data, str) and len(data) <= 65536 and '\x00' not in data)
        elif signature in ('u', 't'):
            require(type(data) is int and 0 <= data <= (1 << (32 if signature == 'u' else 64)) - 1)
        elif signature == 'as':
            require(isinstance(data, list) and len(data) <= 8192 and
                    all(isinstance(item, str) and len(item) <= 65536 and '\x00' not in item for item in data))
        elif signature == '(uo)':
            require(isinstance(data, list) and len(data) == 2 and type(data[0]) is int and
                    0 <= data[0] <= (1 << 32) - 1 and isinstance(data[1], str) and
                    re.fullmatch(r'/(?:[A-Za-z0-9_]+/)*[A-Za-z0-9_]*', data[1]))
        else:
            require(False)
        return data

    def unit_state(self, unit):
        bus = SystemdBus()
        try:
            bus.manager('RefUnit', unit)
            try:
                path = bus.manager('GetUnit', unit)
                result = {}
                for name, sig in [('LoadState', 's'), ('ActiveState', 's'), ('SubState', 's'),
                                  ('Job', '(uo)'), ('FragmentPath', 's'),
                                  ('DropInPaths', 'as'), ('BindsTo', 'as'), ('After', 'as')]:
                    result[name] = self.property(bus, path, 'org.freedesktop.systemd1.Unit', name, sig)
                if unit.endswith('.service'):
                    for name, sig in [('ControlGroup', 's'), ('MainPID', 'u'), ('ControlPID', 'u'), ('Result', 's'),
                                      ('RuntimeMaxUSec', 't'), ('User', 's'), ('Group', 's'), ('Restart', 's')]:
                        result[name] = self.property(bus, path, 'org.freedesktop.systemd1.Service', name, sig)
                else:
                    result['MainPID'] = result['ControlPID'] = 0
                    result['ControlGroup'] = ''
                return result
            finally:
                bus.manager('UnrefUnit', unit)
        finally:
            bus.close()

    def empty_cgroup(self, group):
        require(isinstance(group, str) and group.startswith('/') and '..' not in Path(group).parts)
        base = self.p('/sys/fs/cgroup' + group)
        require(base.is_dir() and not base.is_symlink())
        count = 0
        def visit(path):
            nonlocal count
            require(not path.is_symlink())
            with os.scandir(path) as entries:
                for entry in entries:
                    count += 1
                    require(count <= 4096 and not entry.is_symlink())
                    if entry.is_dir(follow_symlinks=False):
                        visit(Path(entry.path))
                    elif entry.name in ('cgroup.procs', 'cgroup.threads'):
                        with open(entry.path, 'rb') as stream:
                            raw = stream.read(4097)
                        require(len(raw) <= 4096 and raw.strip() == b'')
        visit(base)
        require((base / 'cgroup.procs').is_file())

    def drained(self, unit):
        state = self.unit_state(unit)
        require(state['LoadState'] == 'loaded' and state['ActiveState'] == 'inactive' and
                state['SubState'] == 'dead' and state['Job'][0] == 0 and
                state['MainPID'] == 0 and state['ControlPID'] == 0)
        group = state['ControlGroup']
        if group:
            self.empty_cgroup(group)
        else:
            # A GC'd empty control group is legitimate only with the unit pinned
            # loaded, dead, jobless and no PIDs on the same observation.
            require(state['Job'] == [0, '/'])
        require(self.unit_state(unit) == state)
        return state

    def authorization_identity(self, raw, hold_raw):
        auth = decoded(raw)
        exact(auth, 'version purpose operation_id manifest_sha256 hold_sha256 state_sha256 executable_sha256 broker_sha256 config_sha256 boot_id runtime_sha256 supervisor issued_at expires_at entry_baseline_sha256')
        require(type(auth['version']) is int and auth['version'] == 1 and auth['purpose'] == 'serenity.fixture-closeout-authorization/v1')
        require(auth['operation_id'] == self.c['operation_id'] and auth['manifest_sha256'] == self.c['manifest_sha256'] and
                auth['hold_sha256'] == digest(hold_raw) and auth['boot_id'] == self.c['boot_id'] and
                auth['runtime_sha256'] == digest(self.read(self.runtime)) and
                auth['entry_baseline_sha256'] == digest(self.read(self.stage + '/baseline.json', private=True)))
        for field, expected in [('state_sha256', self.s['state_sha256']), ('executable_sha256', self.s['binary_sha256']),
                                ('broker_sha256', self.s['broker_sha256']), ('config_sha256', self.s['config_sha256'])]:
            require(auth[field] == expected)
        require(type(auth['issued_at']) is int and type(auth['expires_at']) is int and
                0 < auth['expires_at'] - auth['issued_at'] <= 300)
        supervisor = auth['supervisor']
        require(supervisor == self.c['supervisor'] or re.fullmatch(
            'serenity-release-' + self.c['operation_id'] + r'-closeout-[0-9a-f]{64}\.service', supervisor))
        return auth

    def authority(self, authorization=None, hold_raw=None, main=False):
        now = int(self.clock())
        require(self.read('/proc/sys/kernel/random/boot_id').decode().strip() == self.c['boot_id'])
        supervisor, expiry = self.c['supervisor'], self.c['expires_at']
        if authorization is not None:
            auth = self.authorization_identity(self.read(authorization, private=True, limit=16384), hold_raw)
            require(auth['issued_at'] <= now)
            supervisor, expiry = auth['supervisor'], auth['expires_at']
        else:
            require(self.c['issued_at'] <= now)
        require(now < expiry)
        state = self.unit_state(supervisor)
        require(state['LoadState'] == 'loaded' and state['ActiveState'] == 'active' and
                type(state['MainPID']) is int and state['MainPID'] > 0 and
                (not main or state['MainPID'] == os.getpid()) and
                type(state['RuntimeMaxUSec']) is int and 0 < state['RuntimeMaxUSec'] <= 3600 * 1000000)
        # A finite maximum alone does not prove its remaining lifetime. Read the
        # monotonic start timestamp and compare against the same boot clock.
        remaining = self.supervisor_remaining(supervisor)
        require(remaining >= expiry - now)
        return supervisor, expiry

    def supervisor_remaining(self, unit):
        bus = SystemdBus()
        try:
            bus.manager('RefUnit', unit)
            try:
                path = bus.manager('GetUnit', unit)
                started = self.property(bus, path, 'org.freedesktop.systemd1.Unit', 'ActiveEnterTimestampMonotonic', 't')
                maximum = self.property(bus, path, 'org.freedesktop.systemd1.Service', 'RuntimeMaxUSec', 't')
                require(type(started) is int and type(maximum) is int)
                return (started + maximum) / 1000000 - time.clock_gettime(time.CLOCK_MONOTONIC)
            finally:
                bus.manager('UnrefUnit', unit)
        finally:
            bus.close()
    def service_identity(self):
        for unit in WRITERS:
            state = self.unit_state(unit)
            user, group = state['User'], state['Group']
            require(isinstance(user, str) and user and isinstance(group, str))
            account = None if user.isdecimal() else pwd.getpwnam(user)
            uid = int(user) if account is None else account.pw_uid
            if group:
                gid = int(group) if group.isdecimal() else grp.getgrnam(group).gr_gid
            else:
                gid = account.pw_gid if account is not None else pwd.getpwuid(uid).pw_gid
            require(uid == self.f['uid'] and gid == self.f['gid'])

    def pair(self):
        self.service_identity()
        link = self.p('/usr/local/bin/serenity')
        require(link.is_symlink() and link.lstat().st_uid == self.uid and
                os.readlink(link) == self.s['binary_path'])
        self.trusted(link.parent, directory=True)
        require(not self.p(STATE + '.pending').exists() and not self.p(STATE + '.pending').is_symlink())
        pins = [(STATE, self.s['state_sha256']), (BROKER, self.s['broker_sha256']),
                (HOSTED, self.s['config_sha256']), (self.s['binary_path'], self.s['binary_sha256']),
                (self.s['startup_runtime_path'], self.s['startup_runtime_sha256']),
                (self.f['tool_path'], self.f['tool_sha256']), (self.f['wrapper_path'], self.f['wrapper_sha256']),
                (self.a['generator_path'], self.a['generator_sha256'])]
        for path, expected in pins:
            require(digest(self.read(path, limit=512 * 1024 * 1024 if path == self.s['binary_path'] or
                                     path == self.f['tool_path'] else 1048576)) == expected)
        state = decoded(self.read(STATE))
        exact(state, 'version installed_sequence highest_sequence manifest_sha256')
        require(state == dict(version=1, installed_sequence=self.s['installed_sequence'],
                highest_sequence=self.s['highest_sequence'], manifest_sha256=self.s['release_manifest_sha256']))
        broker = decoded(self.read(BROKER, private=True))
        require(broker.get('build_sha') == self.s['source'] and
                broker.get('executable_sha256') == self.s['binary_sha256'])
        config = decoded(self.read(HOSTED))
        require(all(config.get(k, False) is False for k in
                    ('episode_archive_enabled', 'workos_enabled', 'billing_enabled')) and
                config.get('episode_synthetic_brains', []) == [] and config.get('data_dir') == self.f['data_dir'])
        raw = self.read(self.stage + '/manifest.json', private=True)
        require(digest(raw) == self.c['manifest_sha256'] and decoded(raw) ==
                {k: v for k, v in self.c.items() if k != 'manifest_sha256'})
        self.trusted(self.p(self.runtime), private=True)
        self.trusted(self.p(self.stage), private=True, directory=True)
        lock = self.trusted(self.p(STATE + '.lock'), private=True)
        if self.fd is not None:
            require(os.path.samestat(os.fstat(self.fd), lock.lstat()))

    def guard_names(self):
        result = {}
        for unit in WRITERS:
            base = '/etc/systemd/system/' + unit + '.d/'
            result[base + '90-serenity-release-guard.conf'] = (MARKER + '[Service]\nExecCondition=+/usr/bin/python3 ' + self.runtime + ' guard ' + unit + '\n').encode()
        for unit in (BACKUP, TIMER):
            result['/etc/systemd/system/' + unit + '.d/90-serenity-release-guard.conf'] = (
                MARKER + '[Unit]\nConditionPathExists=!' + HOLD + '\n').encode()
        return result

    def transient_names(self, supervisor):
        return {'/etc/systemd/system/' + unit + '.d/91-serenity-release-maintenance.conf':
                (MARKER + '[Unit]\nBindsTo=' + supervisor + '\nAfter=' + supervisor +
                 '\n[Service]\nRestart=no\n').encode() for unit in WRITERS}

    def unit_inventory(self, allow_pending=False):
        result = set()
        for unit in UNITS:
            state = self.unit_state(unit)
            require(state['LoadState'] == 'loaded' and isinstance(state['FragmentPath'], str) and
                    isinstance(state['DropInPaths'], list))
            result.add(state['FragmentPath'])
            result.update(state['DropInPaths'])
        # Also inspect every qualified drop-in directory: a file awaiting reload
        # must not escape the effective-property inventory.
        for base in ('/etc/systemd/system', '/usr/lib/systemd/system', '/lib/systemd/system'):
            for unit in UNITS:
                path = self.p(base + '/' + unit + '.d')
                if path.exists() or path.is_symlink():
                    self.trusted(path, directory=True)
                    with os.scandir(path) as entries:
                        count = 0
                        for entry in entries:
                            count += 1
                            require(count <= 128)
                            if allow_pending and entry.name in ('.90-serenity-release-guard.conf.maintenance-write',
                                                               '.91-serenity-release-maintenance.conf.maintenance-write'):
                                self.trusted(Path(entry.path))
                                continue
                            require(entry.name.endswith('.conf') and
                                    entry.is_file(follow_symlinks=False) and not entry.is_symlink())
                            result.add(base + '/' + unit + '.d/' + entry.name)
        require(len(result) <= 160)
        return result

    def baseline(self):
        value = decoded(self.read(self.stage + '/baseline.json', private=True))
        exact(value, 'version operation_id manifest_sha256 timestamp files lock_device lock_inode fixture_snapshot expected_hold')
        require(type(value['version']) is int and value['version'] == 1 and value['operation_id'] == self.c['operation_id'] and
                value['manifest_sha256'] == self.c['manifest_sha256'])
        require(type(value['timestamp']) is int and self.c['issued_at'] <= value['timestamp'] < self.c['expires_at'])
        require(isinstance(value['files'], dict) and set(value['files']) == {x['path'] for x in self.s['unit_files']})
        for item in self.s['unit_files']:
            row = exact(value['files'][item['path']], 'mode data')
            require(row['mode'] in (0o600, 0o644) and
                    digest(base64.b64decode(row['data'], validate=True)) == item['sha256'])
        st = self.p(STATE + '.lock').lstat()
        require((st.st_dev, st.st_ino) == (value['lock_device'], value['lock_inode']))
        require(value['expected_hold'] == self.hold_at(value['timestamp']))
        self.validate_snapshot(value['fixture_snapshot'])
        return value

    def validate_snapshot(self, value):
        exact(value, 'version private_inventory_sha256 fixture_scope_sha256 schema_sha256')
        require(type(value['version']) is int and value['version'] == 1)
        for key in ('private_inventory_sha256', 'fixture_scope_sha256', 'schema_sha256'):
            require(isinstance(value[key], str) and HEX.fullmatch(value[key]))
        require(value['schema_sha256'] == self.f['schema_sha256'])
        return value

    def fixture_snapshot(self):
        self.before_mutation()
        # Root measures the already-pinned executable, then drops identity before
        # any SQLite access. Only canonical metadata hashes return, never rows or
        # credential bytes. The existing service-UID tool owns WAL/schema checks.
        self.pair()
        result = self.runner.run([str(self.p(self.f['tool_path'])), '--fixture-custody-snapshot',
            str(self.p(self.f['data_dir'])), str(self.p(self.f['private_dir'])),
            self.s['config_sha256'], str(self.f['account_cap'])],
            uid=self.f['uid'], gid=self.f['gid'], check=True)
        require(result.returncode == 0 and isinstance(result.stdout, str) and len(result.stdout) <= 16384)
        raw = result.stdout.encode()
        value = decoded(raw)
        require(encoded(value) == raw)
        return self.validate_snapshot(value)

    def entry_nonexecution(self):
        require(not self.p(self.stage + '/maintenance-ready.json').exists() and
                not self.p(self.stage + '/maintenance-ready.json').is_symlink() and
                not self.p(self.stage + '/.maintenance-ready.json.maintenance-write').exists() and
                not self.p(self.stage + '/.maintenance-ready.json.maintenance-write').is_symlink())
        self.child_drained()
        with os.scandir(self.p(self.stage)) as entries:
            for count, entry in enumerate(entries, 1):
                require(count <= 4096 and not entry.name.startswith((
                    '.fixture-used-', 'fixture-proof-', 'fixture-outcome-', 'fixture-request-')))
        require(self.fixture_snapshot() == self.baseline()['fixture_snapshot'])
        original = self.baseline()['files']
        desired = {name: [raw] for name, raw in {**self.guard_names(), **self.transient_names(self.c['supervisor'])}.items()}
        if self.p(self.stage + '/closeout-pending.json').exists():
            record = self.closeout_record()
            require(record['entry_abort'] is True)
            for supervisor in (record['supervisor'], record['guard_supervisor']):
                for name, raw in self.transient_names(supervisor).items():
                    desired.setdefault(name, []).append(raw)
        for name, candidates in desired.items():
            temporary = str(Path(name).with_name('.' + Path(name).name + '.maintenance-write'))
            if self.p(temporary).exists() or self.p(temporary).is_symlink():
                prefix = self.read(temporary, limit=262144)
                require(any(candidate.startswith(prefix) for candidate in candidates))
        current = self.unit_inventory(allow_pending=True)
        require(set(original) <= current <= set(original) | set(desired))
        for name in current:
            raw = self.read(name)
            baseline = base64.b64decode(original[name]['data']) if name in original else None
            require(raw == baseline or raw in desired.get(name, []))

    def snapshot_baseline(self):
        name = self.stage + '/baseline.json'
        if self.p(name).exists() or self.p(name).is_symlink():
            return self.baseline()
        expected = {x['path'] for x in self.s['unit_files']}
        require(self.unit_inventory() == expected)
        files = {}
        for item in self.s['unit_files']:
            raw = self.read(item['path'], limit=262144)
            require(digest(raw) == item['sha256'])
            files[item['path']] = dict(mode=stat.S_IMODE(self.p(item['path']).stat().st_mode),
                                       data=base64.b64encode(raw).decode())
        for unit in WRITERS:
            require(exact_startup_guard(self.read_unit_exec_conditions(unit), self.s['startup_runtime_path'], unit))
            require('/etc/systemd/system/' + unit + '.d/91-serenity-release-maintenance.conf' not in expected)
        for unit in (BACKUP, TIMER):
            require(mandatory_negated_condition(self.read_unit_conditions(unit), HOLD))
        require(self.unit_state(TIMER)['ActiveState'] == 'active')
        for unit in WRITERS:
            require(self.unit_state(unit)['ActiveState'] == 'active')
        st = self.p(STATE + '.lock').lstat()
        value = dict(version=1, operation_id=self.c['operation_id'], manifest_sha256=self.c['manifest_sha256'],
                     timestamp=self.c['issued_at'], files=files, lock_device=st.st_dev, lock_inode=st.st_ino,
                     fixture_snapshot=self.fixture_snapshot(), expected_hold=self.hold_at(self.c['issued_at']))
        self.atomic(name, encoded(value))
        return self.baseline()

    def hold_at(self, timestamp):
        return dict(version=1, purpose=PURPOSE, operation_id=self.c['operation_id'],
                    manifest_sha256=self.c['manifest_sha256'], service_source=self.s['source'],
                    executable_sha256=self.s['binary_sha256'], state_sha256=self.s['state_sha256'],
                    broker_sha256=self.s['broker_sha256'], config_sha256=self.s['config_sha256'],
                    supervisor=self.c['supervisor'], stage=self.stage, backup_timer_prior_state='active',
                    phase='maintenance-ready', timestamp=timestamp, expires_at=self.c['expires_at'])

    def expected_hold(self):
        return self.baseline()['expected_hold']

    def matching_hold(self):
        raw = self.read(HOLD, private=True, limit=16384)
        require(decoded(raw) == self.expected_hold() and raw == encoded(self.expected_hold()))
        return raw

    def qualified_guards(self, supervisor, *, baseline=False):
        original = self.baseline()['files']
        desired = {} if baseline else {**self.guard_names(), **self.transient_names(supervisor)}
        expected = set(original) | set(desired)
        require(self.unit_inventory() == expected)
        for name in expected:
            wanted = desired.get(name)
            if wanted is None:
                wanted = base64.b64decode(original[name]['data'], validate=True)
            require(self.read(name) == wanted)
        for unit in WRITERS:
            require(exact_startup_guard(self.read_unit_exec_conditions(unit),
                    self.s['startup_runtime_path'] if baseline else self.runtime, unit))
        for unit in (BACKUP, TIMER):
            require(mandatory_negated_condition(self.read_unit_conditions(unit), HOLD))
        for unit in WRITERS:
            state = self.unit_state(unit)
            if not baseline:
                require(state['BindsTo'] == [supervisor] and supervisor in state['After'] and state['Restart'] == 'no')
            else:
                require(supervisor not in state['BindsTo'])

    def install_guards(self, supervisor, prior_supervisor=None):
        original = self.baseline()['files']
        desired = {**self.guard_names(), **self.transient_names(supervisor)}
        allowed = {**self.guard_names(), **self.transient_names(prior_supervisor or self.c['supervisor'])}
        require(self.unit_inventory(allow_pending=True) <= set(original) | set(desired))
        for name, data in desired.items():
            prior = None
            if self.p(name).exists():
                current = self.read(name)
                baseline = base64.b64decode(original[name]['data']) if name in original else None
                require(current in (data, baseline, allowed.get(name)))
                prior = current
            self.mkdir(str(Path(name).parent))
            self.atomic(name, data, 0o644, prior=prior)
        self.systemctl('daemon-reload')
        self.qualified_guards(supervisor)

    def permits_absent(self):
        path = self.p(RUN)
        if path.exists() or path.is_symlink():
            self.trusted(path, private=True, directory=True)
            with os.scandir(path) as entries:
                require(next(entries, None) is None)

    def revoke_permits(self):
        path = self.p(RUN)
        if not path.exists():
            return
        self.trusted(path, private=True, directory=True)
        with os.scandir(path) as entries:
            names = [entry.name for _, entry in zip(range(4), entries)]
        require(set(names) <= {unit + '.permit' for unit in WRITERS})
        for name in names:
            raw = self.read(RUN + '/' + name, private=True)
            value = decoded(raw)
            require(value.get('operation_id') == self.c['operation_id'] and
                    value.get('manifest_sha256') == self.c['manifest_sha256'])
            self.unlink_exact(RUN + '/' + name, raw)

    def stop_all(self):
        self.systemctl('stop', TIMER)
        deadline = time.monotonic() + 60
        while self.unit_state(BACKUP)['ActiveState'] in ('active', 'activating', 'deactivating'):
            require(time.monotonic() < deadline)
            time.sleep(.2)
        self.drained(BACKUP)
        self.systemctl('stop', *WRITERS)
        for unit in UNITS:
            self.drained(unit)

    def enter(self):
        with self.lock():
            self.authority(main=True)
            self.pair()
            self._gate = True
            require(not self.p(self.stage + '/closeout-pending.json').exists() and
                    not self.p(self.stage + '/closed.json').exists())
            if self.p(HOLD).exists() or self.p(HOLD).is_symlink():
                self.matching_hold()
            self.snapshot_baseline()
            self.permits_absent()
            self.install_guards(self.c['supervisor'])
            self.authority(main=True)
            self.pair()
            self.atomic(HOLD, encoded(self.expected_hold()))
            self.checkpoint('hold-published')
            self.stop_all()
            self.revoke_permits()
            self.authority(main=True)
            self.pair()
            ready = self.cold_ready()
            self.atomic(self.stage + '/maintenance-ready.json', encoded(ready))
            return ready

    def cold_ready(self):
        self.authority()
        self.pair()
        require(not self.p(self.stage + '/closeout-pending.json').exists() and
                not self.p(self.stage + '/closed.json').exists())
        raw = self.matching_hold()
        self.qualified_guards(self.c['supervisor'])
        self.permits_absent()
        for unit in UNITS:
            self.drained(unit)
        self.child_drained()
        self.authority()
        self.pair()
        require(self.matching_hold() == raw)
        return dict(version=2, purpose=PURPOSE, operation_id=self.c['operation_id'],
                    manifest_sha256=self.c['manifest_sha256'], state_sha256=self.s['state_sha256'],
                    hold_sha256=digest(raw), service_source=self.s['source'],
                    executable_sha256=self.s['binary_sha256'], held=True, services_started=False)

    def fixture_outcomes(self):
        self.child_drained()
        stage = self.p(self.stage)
        used, proofs, outcomes = set(), set(), set()
        with os.scandir(stage) as entries:
            count = 0
            for entry in entries:
                count += 1
                require(count <= 4096)
                name = entry.name
                for prefix, suffix, collection in [('.fixture-used-', '', used),
                    ('fixture-proof-', '.json', proofs), ('fixture-outcome-', '.json', outcomes)]:
                    if name.startswith(prefix):
                        nonce = name[len(prefix):len(name)-len(suffix) if suffix else None]
                        require(HEX.fullmatch(nonce) and (not suffix or name.endswith(suffix)))
                        collection.add(nonce)
        require(used and used == proofs == outcomes)
        ordered = []
        hold_hash = digest(self.matching_hold())
        for nonce in used:
            require(self.read(self.stage + '/.fixture-used-' + nonce, private=True, limit=65536) == b'consumed\n')
            proof_raw = self.read(self.stage + '/fixture-proof-' + nonce + '.json', private=True, limit=65536)
            proof = decoded(proof_raw)
            require(isinstance(proof, dict) and proof.get('operation') in ('prepare', 'reissue', 'revoke', 'adopt-source'))
            require(set(proof) == PROOF_FIELDS | ({'source_adoption'} if proof['operation'] == 'adopt-source' else set()))
            for key in PROOF_FIELDS:
                if key.endswith('_sha256'):
                    require(isinstance(proof[key], str) and HEX.fullmatch(proof[key]))
            require(type(proof['issued_at']) is int and type(proof['expires_at']) is int and
                    self.c['issued_at'] <= proof['issued_at'] < proof['expires_at'] <= self.c['expires_at'] and
                    proof['expires_at'] - proof['issued_at'] <= 300)
            bindings = dict(boot_id=self.c['boot_id'], uid=self.f['uid'], gid=self.f['gid'],
                tool_path=self.f['tool_path'], wrapper_path=self.f['wrapper_path'], wrapper_sha256=self.f['wrapper_sha256'],
                runtime_path=self.runtime, runtime_sha256=digest(self.read(self.runtime)),
                service_binary_path=self.s['binary_path'], service_binary_sha256=self.s['binary_sha256'],
                config_path=HOSTED, config_sha256=self.s['config_sha256'], data_dir=self.f['data_dir'],
                private_dir=self.f['private_dir'], account_cap=self.f['account_cap'], state_sha256=self.s['state_sha256'],
                schema_sha256=self.f['schema_sha256'], authority_generator_path=self.a['generator_path'],
                authority_generator_sha256=self.a['generator_sha256'], authority_scope=self.a['scope'],
                authority_session_id=self.a['session_id'], concurrency_target=self.a['concurrency_target'],
                lock_device=self.baseline()['lock_device'], lock_inode=self.baseline()['lock_inode'],
                ready_sha256=digest(self.read(self.stage + '/maintenance-ready.json', private=True)))
            require(all(proof[key] == value for key, value in bindings.items()))
            if proof['operation'] == 'adopt-source':
                adoption = exact(proof['source_adoption'], 'old_source old_manifest_sha256')
                require(isinstance(adoption['old_source'], str) and re.fullmatch(r'[0-9a-f]{40}', adoption['old_source']) and
                        isinstance(adoption['old_manifest_sha256'], str) and HEX.fullmatch(adoption['old_manifest_sha256']))
            outcome = decoded(self.read(self.stage + '/fixture-outcome-' + nonce + '.json', private=True, limit=16384))
            exact(outcome, 'version purpose operation_id nonce operation proof_sha256 tool_source tool_sha256 completed_at sequence result')
            require(type(outcome['version']) is int and outcome['version'] == 1 and outcome['purpose'] == 'serenity-episode-fixture-offline-v3' and
                    outcome['operation_id'] == self.c['operation_id'] and outcome['nonce'] == nonce and
                    outcome['result'] == 'success' and outcome['proof_sha256'] == digest(proof_raw) and
                    outcome['tool_source'] == self.f['tool_source'] and outcome['tool_sha256'] == self.f['tool_sha256'])
            require(type(proof.get('version')) is int and proof.get('version') == 3 and proof.get('purpose') == outcome['purpose'] and
                    proof.get('nonce') == nonce and proof.get('operation') == outcome['operation'] and
                    proof.get('hold_sha256') == hold_hash and proof.get('maintenance_hold') == self.expected_hold() and
                    proof.get('service_source') == self.s['source'] and proof.get('tool_source') == self.f['tool_source'] and
                    proof.get('tool_sha256') == self.f['tool_sha256'])
            require(type(outcome['completed_at']) is int and type(outcome['sequence']) is int and
                    outcome['sequence'] > 0 and proof['issued_at'] <= outcome['completed_at'] < proof['expires_at'] and
                    outcome['completed_at'] <= int(self.clock()))
            ordered.append((outcome['sequence'], nonce, outcome['operation']))
        ordered.sort()
        require([row[0] for row in ordered] == list(range(1, len(ordered) + 1)) and
                ordered[-1][2] == 'revoke')
        return digest(encoded(ordered))
    def child_drained(self):
        # Query actual manager inventories first; an absent transient unit is
        # accepted only if both lists and the actual cgroup agree. No broad
        # exception-to-not-found conversion can hide malformed unit properties.
        values = []
        for method, signature in [('ListUnits', 'a(ssssssouso)'), ('ListJobs', 'a(usssoo)')]:
            raw = self.command('/usr/bin/busctl', '--system', '--json=short', 'call',
                'org.freedesktop.systemd1', '/org/freedesktop/systemd1',
                'org.freedesktop.systemd1.Manager', method).stdout
            value = exact(decoded(raw), 'type data')
            require(value['type'] == signature and isinstance(value['data'], list) and len(value['data']) == 1 and
                    isinstance(value['data'][0], list) and len(value['data'][0]) <= 8192)
            values.append(value['data'][0])
        units, jobs = values
        require(all(isinstance(row, list) and len(row) == 10 for row in units) and
                all(isinstance(row, list) and len(row) == 6 for row in jobs))
        require(not any(row[1] == self.f['unit'] for row in jobs))
        if any(row[0] == self.f['unit'] for row in units):
            self.drained(self.f['unit'])
        else:
            path = self.p('/sys/fs/cgroup/system.slice/' + self.f['unit'])
            if path.exists() or path.is_symlink():
                self.empty_cgroup('/system.slice/' + self.f['unit'])

    def admission_bindings(self, value, identity):
        return dict(authorization_path=value['authorization_path'],
                    authorization_sha256=value['authorization_sha256'],
                    supervisor=value['supervisor'], expires_at=value['expires_at'], admission_sha256=identity)

    def admissions(self):
        result, incomplete = {}, []
        with os.scandir(self.p(self.stage)) as entries:
            for count, entry in enumerate(entries, 1):
                require(count <= 4096)
                match = re.fullmatch(r'closeout-admission-([0-9a-f]{64})\.json', entry.name)
                pending = re.fullmatch(r'\.closeout-admission-([0-9a-f]{64})\.json\.maintenance-write', entry.name)
                if pending:
                    self.read(self.stage + '/' + entry.name, private=True, limit=32768)
                    incomplete.append(pending.group(1))
                    continue
                if entry.name.startswith(('closeout-admission-', '.closeout-admission-')):
                    require(match is not None)
                if match is None:
                    continue
                raw = self.read(self.stage + '/' + entry.name, private=True, limit=32768)
                value = decoded(raw)
                require(encoded(value) == raw and digest(raw) == match.group(1))
                exact(value, 'version purpose operation_id manifest_sha256 entry_baseline_sha256 hold_sha256 runtime_sha256 authorization_path authorization_sha256 authorization_bytes supervisor expires_at')
                require(type(value['version']) is int and value['version'] == 1 and value['purpose'] == PURPOSE and
                        value['operation_id'] == self.c['operation_id'] and value['manifest_sha256'] == self.c['manifest_sha256'] and
                        value['entry_baseline_sha256'] == digest(self.read(self.stage + '/baseline.json', private=True)) and
                        value['hold_sha256'] == digest(encoded(self.expected_hold())) and
                        value['runtime_sha256'] == digest(self.read(self.runtime)))
                if value['authorization_path'] is None:
                    require(value['authorization_sha256'] is None and value['authorization_bytes'] is None and
                            value['supervisor'] == self.c['supervisor'] and value['expires_at'] == self.c['expires_at'])
                else:
                    require(isinstance(value['authorization_path'], str) and value['authorization_path'].startswith('/'))
                    auth_raw = base64.b64decode(value['authorization_bytes'], validate=True)
                    require(digest(auth_raw) == value['authorization_sha256'])
                    auth = self.authorization_identity(auth_raw, encoded(self.expected_hold()))
                    require(value['supervisor'] == auth['supervisor'] and value['expires_at'] == auth['expires_at'])
                result[match.group(1)] = value
        require(len(result) + len(incomplete) <= 32 and len(result) <= 16 and len(incomplete) <= 16)
        return result, incomplete

    def publish_admission(self, authorization, hold_raw):
        known, _ = self.admissions()  # Bound all retained complete and unfinished inputs.
        raw = self.read(authorization, private=True, limit=16384) if authorization else None
        supervisor, expiry = self.authority(authorization, hold_raw)
        value = dict(version=1, purpose=PURPOSE, operation_id=self.c['operation_id'],
            manifest_sha256=self.c['manifest_sha256'], entry_baseline_sha256=digest(self.read(self.stage + '/baseline.json', private=True)),
            hold_sha256=digest(hold_raw), runtime_sha256=digest(self.read(self.runtime)), authorization_path=authorization,
            authorization_sha256=digest(raw) if raw is not None else None,
            authorization_bytes=base64.b64encode(raw).decode() if raw is not None else None,
            supervisor=supervisor, expires_at=expiry)
        identity = digest(encoded(value))
        require(identity in known or len(known) < 16)
        self.atomic(self.stage + '/closeout-admission-' + identity + '.json', encoded(value))
        known, _ = self.admissions()
        require(identity in known)
        return self.admission_bindings(value, identity)

    def staged_records(self, name):
        require(name in ('closeout-pending.json', 'closed.json'))
        require(not self.p(self.stage + '/.' + name + '.maintenance-write').exists() and
                not self.p(self.stage + '/.' + name + '.maintenance-write').is_symlink())
        result = []
        with os.scandir(self.p(self.stage)) as entries:
            for count, entry in enumerate(entries, 1):
                require(count <= 4096)
                if not entry.name.startswith('.' + name + '-'):
                    continue
                match = re.fullmatch(re.escape('.' + name + '-') + r'([0-9a-f]{64})\.maintenance-write', entry.name)
                require(match is not None)
                raw = self.read(self.stage + '/' + entry.name, private=True, limit=65536)
                result.append((match.group(1), raw))
        require(len(result) <= (1 if name == 'closeout-pending.json' else 16))
        return result

    def initial_closeout_record(self, bindings, entry_abort, outcomes):
        return dict(version=1, purpose=PURPOSE, operation_id=self.c['operation_id'],
            manifest_sha256=self.c['manifest_sha256'], hold=self.expected_hold(),
            **bindings, guard_supervisor=self.c['supervisor'], outcomes_sha256=outcomes,
            entry_abort=entry_abort, rehold_from=None, phase='intent')

    def reconcile_pending_record(self, prior):
        staged = self.staged_records('closeout-pending.json')
        if not staged:
            return prior
        known, incomplete = self.admissions()
        target, prefix = staged[0]
        # A dependent progress record is eligible only from a fully published
        # admission. Incomplete admission copies never become authority.
        require(known)
        candidates = {}
        transitions = {'intent': ('authority-restored',), 'authority-restored': ('writers-ready',),
            'writers-ready': ('guards-restored', 'reholding'), 'guards-restored': ('hold-released', 'reholding'),
            'hold-released': ('timer-restored', 'reholding'), 'timer-restored': ('reholding',),
            'reholding': ('authority-restored',)}
        if prior is None:
            entry_abort = not self.p(self.stage + '/maintenance-ready.json').exists()
            if entry_abort:
                self.entry_nonexecution()
                outcomes = None
            else:
                outcomes = self.fixture_outcomes()
        for identity, admission in known.items():
            bindings = self.admission_bindings(admission, identity)
            if prior is None:
                options = [self.initial_closeout_record(bindings, entry_abort, outcomes)]
            else:
                base = {**prior, **bindings}
                options = [base, {**base, 'guard_supervisor': admission['supervisor']}]
                for phase in transitions[prior['phase']]:
                    candidate = {**base, 'phase': phase}
                    if phase == 'reholding':
                        candidate['rehold_from'] = prior['phase']
                    options.append(candidate)
                    options.append({**candidate, 'guard_supervisor': admission['supervisor']})
            for candidate in options:
                raw = encoded(candidate)
                if digest(raw) == target:
                    candidates[raw] = candidate
        require(len(candidates) == 1)
        raw = next(iter(candidates))
        require(raw.startswith(prefix))
        # Current fresh admission authorizes byte reconciliation. The recovered
        # historical admission is never used to execute any expired effect.
        self.atomic(self.stage + '/closeout-pending.json', raw, prior=encoded(prior) if prior else None)
        return self.closeout_record()

    def completion(self, record):
        return dict(version=1, purpose=PURPOSE, operation_id=self.c['operation_id'],
            manifest_sha256=self.c['manifest_sha256'], hold_sha256=digest(encoded(record['hold'])),
            state_sha256=self.s['state_sha256'], closeout_sha256=digest(encoded(record)),
            services_started=True, backup_timer='active')

    def validate_staged_completion(self, prior):
        staged = self.staged_records('closed.json')
        known = {}
        incomplete = 0
        with os.scandir(self.p(self.stage)) as entries:
            for count, entry in enumerate(entries, 1):
                require(count <= 4096)
                match = re.fullmatch(r'closeout-completion-([0-9a-f]{64})\.json', entry.name)
                pending = re.fullmatch(r'\.closeout-completion-([0-9a-f]{64})\.json\.maintenance-write', entry.name)
                if pending:
                    self.read(self.stage + '/' + entry.name, private=True, limit=65536)
                    incomplete += 1
                    continue
                if entry.name.startswith(('closeout-completion-', '.closeout-completion-')):
                    require(match is not None)
                if match is None:
                    continue
                raw = self.read(self.stage + '/' + entry.name, private=True, limit=65536)
                require(digest(raw) == match.group(1))
                value = self.validate_closeout_record(decoded(raw))
                require(encoded(value) == raw and value['phase'] == 'timer-restored')
                completion = encoded(self.completion(value))
                known[digest(completion)] = completion
        require(len(known) <= 16 and incomplete <= 16)
        for target, prefix in staged:
            require(target in known and known[target].startswith(prefix))
        # A prior complete/partial completion remains inactive. Actual readiness
        # and a new record are required to publish success after containment.
        return len(known), incomplete

    def closeout_record(self):
        return self.validate_closeout_record(decoded(self.read(self.stage + '/closeout-pending.json', private=True, limit=65536)))

    def validate_closeout_record(self, value):
        exact(value, 'version purpose operation_id manifest_sha256 hold authorization_path authorization_sha256 supervisor guard_supervisor expires_at outcomes_sha256 entry_abort rehold_from admission_sha256 phase')
        require(type(value['version']) is int and value['version'] == 1 and value['purpose'] == PURPOSE and
                value['operation_id'] == self.c['operation_id'] and value['manifest_sha256'] == self.c['manifest_sha256'] and
                value['hold'] == self.expected_hold() and
                value['phase'] in ('intent', 'reholding', 'authority-restored', 'writers-ready', 'guards-restored', 'hold-released', 'timer-restored'))
        require(isinstance(value['admission_sha256'], str) and HEX.fullmatch(value['admission_sha256']))
        known, _ = self.admissions()
        require(value['admission_sha256'] in known and all(value[key] == expected for key, expected in
                self.admission_bindings(known[value['admission_sha256']], value['admission_sha256']).items()))
        require(value['rehold_from'] is None or value['rehold_from'] in
                ('writers-ready', 'guards-restored', 'hold-released', 'timer-restored'))
        require(value['phase'] != 'reholding' or value['rehold_from'] is not None)
        for name in ('supervisor', 'guard_supervisor'):
            require(value[name] == self.c['supervisor'] or (isinstance(value[name], str) and re.fullmatch(
                'serenity-release-' + self.c['operation_id'] + r'-closeout-[0-9a-f]{64}\.service', value[name])))
        require(type(value['expires_at']) is int and type(value['entry_abort']) is bool and
                (value['outcomes_sha256'] is None if value['entry_abort'] else
                 isinstance(value['outcomes_sha256'], str) and HEX.fullmatch(value['outcomes_sha256'])))
        return value

    def closeout_authority(self, record, main=False):
        path = record['authorization_path']
        require(path is None or (isinstance(path, str) and path.startswith('/')))
        if path is not None:
            require(digest(self.read(path, private=True)) == record['authorization_sha256'])
        else:
            require(record['authorization_sha256'] is None)
        authority = self.authority(path, encoded(record['hold']), main=main)
        require(authority == (record['supervisor'], record['expires_at']))
        return authority

    def receipt(self, path, hold_raw):
        receipt = decoded(self.read(path, private=True, limit=32768))
        exact(receipt, 'version purpose operation_id manifest_sha256 hold_sha256 state_sha256 boot_id observed_at scope session_id concurrency_target concurrency phase revision metadata_sha256 tip_sha256 request_ids generator_path generator_sha256')
        require(type(receipt['version']) is int and receipt['version'] == 1 and receipt['purpose'] == 'serenity.fixture-authority-restored/v1' and
                receipt['operation_id'] == self.c['operation_id'] and receipt['manifest_sha256'] == self.c['manifest_sha256'] and
                receipt['hold_sha256'] == digest(hold_raw) and receipt['state_sha256'] == self.s['state_sha256'] and
                receipt['boot_id'] == self.c['boot_id'])
        for key in ('scope', 'session_id', 'concurrency_target', 'generator_path', 'generator_sha256'):
            require(receipt[key] == self.a[key])
        require(type(receipt['observed_at']) is int and 0 <= int(self.clock()) - receipt['observed_at'] <= 60 and
                type(receipt['concurrency']) is int and receipt['concurrency'] == self.a['restore_concurrency'] and
                receipt['phase'] in ('CLOSED', 'ACTIVE') and type(receipt['revision']) is int and
                receipt['revision'] >= self.a['expectations']['minimum_revision'])
        require(HEX.fullmatch(receipt['metadata_sha256']) and HEX.fullmatch(receipt['tip_sha256']) and
                isinstance(receipt['request_ids'], list) and 1 <= len(receipt['request_ids']) <= 64 and
                all(isinstance(x, str) and re.fullmatch(r'[A-Za-z0-9+/=_:.\-]{1,256}', x) for x in receipt['request_ids']))
        return receipt

    def advance(self, record, phase):
        self.closeout_authority(record)
        self.pair()
        prior = encoded(record)
        record = {**record, 'phase': phase}
        self.atomic(self.stage + '/closeout-pending.json', encoded(record), prior=prior)
        self.checkpoint('closeout:' + phase)
        return record

    def permit(self, unit, record):
        self.closeout_authority(record)
        self.pair()
        self.mkdir(RUN)
        self.atomic(RUN + '/' + unit + '.permit', encoded(dict(
            version=1, operation_id=self.c['operation_id'], manifest_sha256=self.c['manifest_sha256'],
            boot_id=self.c['boot_id'], hold_sha256=digest(encoded(record['hold'])),
            closeout_sha256=digest(encoded(record)), unit=unit)))

    def guard(self, unit):
        require(unit in WRITERS)
        self.pair()
        # Temporary guards never admit an absent hold. Permanent baseline guards
        # regain normal startup responsibility only after exact restoration.
        self.matching_hold()
        record = self.closeout_record()
        require(record['phase'] in ('authority-restored', 'writers-ready'))
        self.closeout_authority(record)
        self._authorization, self._authorization_hold, self._gate = record['authorization_path'], encoded(record['hold']), True
        name = RUN + '/' + unit + '.permit'
        raw = self.read(name, private=True, limit=16384)
        wanted = dict(version=1, operation_id=self.c['operation_id'], manifest_sha256=self.c['manifest_sha256'],
                      boot_id=self.c['boot_id'], hold_sha256=digest(encoded(record['hold'])),
                      closeout_sha256=digest(encoded(record)), unit=unit)
        require(decoded(raw) == wanted)
        self.unlink_exact(name, raw)

    def readiness(self):
        # Uses the unchanged application's real readiness route after authority
        # restoration; deliberately does not require CLOSED/no normal lease.
        with urllib.request.urlopen('http://127.0.0.1:8090/readyz', timeout=5) as response:
            require(response.status == 200)
        self.pair()
        for unit in WRITERS:
            state = self.unit_state(unit)
            require(state['LoadState'] == 'loaded' and state['ActiveState'] == 'active' and
                    state['SubState'] == 'running' and state['Job'][0] == 0 and
                    type(state['MainPID']) is int and state['MainPID'] > 0 and state['ControlPID'] == 0)
            pid = state['MainPID']
            executable = self.p('/proc/' + str(pid) + '/exe')
            # /proc/PID/exe is the kernel-owned identity link, not a trusted-file
            # pathname. Pin the opened running object, not the configurable link.
            with executable.open('rb') as stream:
                require(digest(stream.read(512 * 1024 * 1024 + 1)) == self.s['binary_sha256'])
            with self.p('/proc/' + str(pid) + '/status').open('rb') as stream:
                status_raw = stream.read(65537)
            require(len(status_raw) <= 65536)
            status = status_raw.decode()
            rows = dict(line.split(':', 1) for line in status.splitlines() if ':' in line)
            require([int(x) for x in rows['Uid'].split()] == [self.f['uid']] * 4 and
                    [int(x) for x in rows['Gid'].split()] == [self.f['gid']] * 4)
            require(self.unit_state(unit) == state)

    def restore_guards(self, record):
        original = self.baseline()['files']
        desired = {**self.guard_names(), **self.transient_names(record['supervisor'])}
        require(self.unit_inventory(allow_pending=True) <= set(original) | set(desired))
        for name, current in desired.items():
            self.closeout_authority(record)
            self.pair()
            if name in original:
                baseline = base64.b64decode(original[name]['data'])
                if self.read(name) != baseline:
                    self.atomic(name, baseline, original[name]['mode'], prior=current)
            elif self.p(name).exists():
                self.unlink_exact(name, current)
        self.systemctl('daemon-reload')
        self.qualified_guards(record['supervisor'], baseline=True)

    def closeout(self, restore_receipt_path, authorization_path=None):
        with self.lock():
            self.pair()
            pending = self.p(self.stage + '/closeout-pending.json')
            existing = self.closeout_record() if pending.exists() else None
            entry_abort = existing is not None and existing['entry_abort'] is True
            if self.p(HOLD).exists():
                hold_raw = self.matching_hold()
                if existing is None and not self.p(self.stage + '/maintenance-ready.json').exists():
                    # An entry that never published fixture readiness cannot
                    # manufacture a final revoke. Exact fresh closeout-only
                    # authority may reconcile verified nonexecution instead.
                    require(authorization_path is not None and
                            not self.p(self.stage + '/maintenance-ready.json').is_symlink())
                    entry_abort = True
            else:
                require(authorization_path is not None)
                if existing is None:
                    require(not self.p(self.stage + '/closed.json').exists() and int(self.clock()) >= self.c['expires_at'])
                    hold_raw = encoded(self.expected_hold())
                    entry_abort = True
                else:
                    require(existing['phase'] in ('hold-released', 'timer-restored') or
                            (existing['phase'] == 'reholding' and existing['rehold_from'] in ('hold-released', 'timer-restored')) or
                            (existing.get('entry_abort') is True and existing['phase'] == 'intent'))
                    hold_raw = encoded(existing['hold'])
                    entry_abort = existing.get('entry_abort') is True
            supervisor, expiry = self.authority(authorization_path, hold_raw, main=True)
            self._authorization, self._authorization_hold, self._gate = authorization_path, hold_raw, True
            self.validate_staged_completion(existing)
            existing = self.reconcile_pending_record(existing)
            entry_abort = existing['entry_abort'] if existing is not None else entry_abort
            if self.p(self.stage + '/closed.json').exists():
                require(existing is not None and existing['phase'] == 'timer-restored' and not self.p(HOLD).exists())
                closed = decoded(self.read(self.stage + '/closed.json', private=True))
                require(closed == dict(version=1, purpose=PURPOSE, operation_id=self.c['operation_id'],
                    manifest_sha256=self.c['manifest_sha256'], hold_sha256=digest(hold_raw),
                    state_sha256=self.s['state_sha256'], closeout_sha256=digest(encoded(existing)),
                    services_started=True, backup_timer='active'))
                self.readiness()
                self.qualified_guards(existing['supervisor'], baseline=True)
                require(self.unit_state(TIMER)['ActiveState'] == 'active')
                return closed
            if existing is None:
                if entry_abort:
                    self.entry_nonexecution()
                    outcomes = None
                else:
                    self.matching_hold()
                    self.permits_absent()
                    self.qualified_guards(self.c['supervisor'])
                    for unit in UNITS:
                        self.drained(unit)
                    outcomes = self.fixture_outcomes()
                bindings = self.publish_admission(authorization_path, hold_raw)
                record = self.initial_closeout_record(bindings, entry_abort, outcomes)
                self.atomic(self.stage + '/closeout-pending.json', encoded(record))
            else:
                record = existing
                if record['entry_abort']:
                    self.entry_nonexecution()
                    require(record['outcomes_sha256'] is None)
                elif self.p(HOLD).exists():
                    require(self.fixture_outcomes() == record['outcomes_sha256'])
                bindings = self.publish_admission(authorization_path, hold_raw)
                changed = {**record, **bindings}
                if changed != record:
                    self.atomic(self.stage + '/closeout-pending.json', encoded(changed), prior=encoded(record))
                    record = changed
            self.closeout_authority(record, main=True)
            if entry_abort and record['phase'] == 'intent':
                if not self.p(HOLD).exists():
                    self.atomic(HOLD, hold_raw)
                    self.checkpoint('entry-abort-hold-published')
                self.install_guards(supervisor, record['guard_supervisor'])
                self.stop_all()
            late = record['phase'] in ('writers-ready', 'guards-restored', 'hold-released', 'timer-restored')
            writers_running = all(self.unit_state(unit)['ActiveState'] == 'active' and
                                  self.unit_state(unit)['SubState'] == 'running' for unit in WRITERS)
            needs_rehold = late and (record['guard_supervisor'] != supervisor or not writers_running or
                (record['phase'] == 'timer-restored' and self.unit_state(TIMER)['ActiveState'] != 'active'))
            if needs_rehold:
                require(authorization_path is not None)
                self.receipt(restore_receipt_path, hold_raw)
                prior = encoded(record)
                record = {**record, 'rehold_from': record['phase'], 'phase': 'reholding'}
                self.atomic(self.stage + '/closeout-pending.json', encoded(record), prior=prior)
                self.checkpoint('closeout:reholding')
            if record['phase'] == 'reholding':
                require(authorization_path is not None)
                self.closeout_authority(record, main=True)
                self.receipt(restore_receipt_path, hold_raw)
                if not self.p(HOLD).exists():
                    self.atomic(HOLD, hold_raw)
                    self.checkpoint('reholding:hold-published')
                require(self.matching_hold() == hold_raw)
                if record['entry_abort']:
                    self.entry_nonexecution()
                else:
                    require(self.fixture_outcomes() == record['outcomes_sha256'])
                self.install_guards(supervisor, record['guard_supervisor'])
                prior = encoded(record)
                record = {**record, 'guard_supervisor': supervisor, 'phase': 'authority-restored'}
                self.atomic(self.stage + '/closeout-pending.json', encoded(record), prior=prior)
            if record['phase'] in ('intent', 'authority-restored'):
                self.receipt(restore_receipt_path, hold_raw)
                self.install_guards(supervisor, record['guard_supervisor'])
                if record['guard_supervisor'] != supervisor:
                    old_record = encoded(record)
                    record = {**record, 'guard_supervisor': supervisor}
                    self.atomic(self.stage + '/closeout-pending.json', encoded(record), prior=old_record)
                if record['phase'] == 'intent':
                    record = self.advance(record, 'authority-restored')
                self.revoke_permits()
                for unit in reversed(WRITERS):
                    self.closeout_authority(record)
                    self.receipt(restore_receipt_path, hold_raw)
                    state = self.unit_state(unit)
                    if state['ActiveState'] != 'active':
                        self.drained(unit)
                        self.permit(unit, record)
                        self.systemctl('start', unit)
                    self.checkpoint('started:' + unit)
                self.readiness()
                self.permits_absent()
                record = self.advance(record, 'writers-ready')
            if record['phase'] == 'writers-ready':
                self.receipt(restore_receipt_path, hold_raw)
                self.readiness()
                self.restore_guards(record)
                record = self.advance(record, 'guards-restored')
            if record['phase'] == 'guards-restored':
                self.receipt(restore_receipt_path, hold_raw)
                self.readiness()
                self.qualified_guards(supervisor, baseline=True)
                # Persist exact release intent BEFORE unlink. Recovery of a lost
                # hold requires this identity plus fresh closeout authorization.
                record = self.advance(record, 'hold-released')
            if record['phase'] == 'hold-released':
                self.closeout_authority(record)
                self.receipt(restore_receipt_path, hold_raw)
                self.readiness()
                self.qualified_guards(supervisor, baseline=True)
                if self.p(HOLD).exists():
                    self.unlink_exact(HOLD, hold_raw)
                self.checkpoint('hold-released')
                self.closeout_authority(record)
                self.systemctl('start', TIMER)
                self.checkpoint('timer-started')
                require(self.unit_state(TIMER)['ActiveState'] == 'active')
                record = self.advance(record, 'timer-restored')
            self.closeout_authority(record)
            self.readiness()
            self.qualified_guards(supervisor, baseline=True)
            require(not self.p(HOLD).exists() and self.unit_state(TIMER)['ActiveState'] == 'active')
            self.drained(BACKUP)
            completion_base = self.stage + '/closeout-completion-' + digest(encoded(record)) + '.json'
            complete_count, _ = self.validate_staged_completion(record)
            require(self.p(completion_base).exists() or complete_count < 16)
            self.atomic(completion_base, encoded(record))
            self.atomic(self.stage + '/closed.json', encoded(self.completion(record)))
            return decoded(self.read(self.stage + '/closed.json', private=True))

    def supervisor_exit(self):
        self._gate = False
        # Exit is containment only, including expired authority: never release a
        # hold, alter old data or restart anything on process death.
        if not self.p(HOLD).exists() and not self.p(self.stage + '/baseline.json').exists():
            return
        with self.lock():
            if self.p(HOLD).exists():
                self.matching_hold()
            else:
                if not self.p(self.stage + '/closeout-pending.json').exists():
                    self.baseline()
                    self.systemctl('stop', TIMER, *WRITERS)
                    return
                record = self.closeout_record()
                require(record['phase'] in ('hold-released', 'timer-restored') or
                        (record['phase'] == 'reholding' and record['rehold_from'] in ('hold-released', 'timer-restored')))
                if self.p(self.stage + '/closed.json').exists():
                    return
            self.revoke_permits()
            self.systemctl('stop', TIMER, *WRITERS)


def main():
    require(CONFIG is not None and os.geteuid() == 0 and sys.flags.isolated == 1)
    require(Path(__file__).resolve() == Path(CONFIG['stage']) / 'runtime.py')
    transaction = Transaction(CONFIG)
    action = sys.argv[1] if len(sys.argv) > 1 else ''
    require(action in ('enter', 'closeout', 'guard', 'supervisor-exit', 'cold-ready'))
    if action == 'guard':
        try:
            require(len(sys.argv) == 3)
            transaction.guard(sys.argv[2])
        except BaseException:
            return 1
        return 0
    if action == 'closeout':
        require(len(sys.argv) in (3, 4))
        transaction.closeout(sys.argv[2], sys.argv[3] if len(sys.argv) == 4 else None)
    elif action == 'cold-ready':
        require(len(sys.argv) == 2)
        print(encoded(transaction.cold_ready()).decode())
    else:
        require(len(sys.argv) == 2)
        getattr(transaction, action.replace('-', '_'))()
    return 0


if __name__ == '__main__':
    try:
        result = main()
    except BaseException:
        raise SystemExit('maintenance operation refused; retain custody for reconciliation') from None
    sys.exit(result)
