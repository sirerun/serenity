"""Synthetic filesystem/systemd tests; no installed or provider qualification."""
import base64
import copy
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('maintenance_runtime_tested', HERE / 'runtime.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


class Crash(BaseException):
    pass


class FakeRunner:
    def __init__(self):
        self.transaction = None
        self.states = {u: 'active' for u in m.UNITS}
        self.states[m.BACKUP] = 'inactive'
        self.calls = []
        self.start_failure = False

    def run(self, argv, *, check=True, pass_fds=(), uid=None, gid=None):
        self.calls.append(argv)
        if argv[0] == '/usr/bin/systemctl':
            if argv[1] == 'stop':
                for unit in argv[2:]:
                    self.states[unit] = 'inactive'
            elif argv[1] == 'start':
                unit = argv[2]
                if unit in m.WRITERS:
                    self.transaction.guard(unit)
                if self.start_failure:
                    raise RuntimeError('synthetic start uncertainty')
                self.states[unit] = 'active'
            elif argv[1] != 'daemon-reload':
                raise AssertionError(argv)
        elif len(argv) > 1 and argv[1] == '--fixture-custody-snapshot':
            assert uid == self.transaction.f['uid'] and gid == self.transaction.f['gid']
            return subprocess.CompletedProcess(argv, 0, m.encoded(dict(version=1, private_inventory_sha256='a'*64,
                fixture_scope_sha256='b'*64, schema_sha256=self.transaction.f['schema_sha256'])).decode(), '')
        else:
            raise AssertionError(argv)
        return subprocess.CompletedProcess(argv, 0, '', '')


class SyntheticTransaction(m.Transaction):
    def __init__(self, *args, crash=None, **kwargs):
        self.crash = crash
        self.stalled = set()
        super().__init__(*args, **kwargs)
        self.runner.transaction = self

    def checkpoint(self, name):
        if name == self.crash:
            self.crash = None
            raise Crash(name)

    def unit_state(self, unit):
        state = self.runner.states.get(unit, 'inactive')
        pid = os.getpid() if unit not in m.UNITS else (42 if state == 'active' else 0)
        transient = self.p('/etc/systemd/system/' + unit + '.d/91-serenity-release-maintenance.conf')
        bound = []
        if transient.exists():
            bound = [line.split('=', 1)[1] for line in transient.read_text().splitlines() if line.startswith('BindsTo=')]
        return dict(BindsTo=bound, After=bound, Restart='no' if bound else 'on-failure', LoadState='loaded', ActiveState=state,
                    SubState='running' if state == 'active' and unit.endswith('.service') else
                             ('waiting' if state == 'active' else 'dead'),
                    MainPID=pid, ControlPID=0, Job=[0, '/'], ControlGroup='/synthetic' if unit in self.stalled else '',
                    RuntimeMaxUSec=3600 * 1000000, User=str(self.f['uid']), Group=str(self.f['gid']),
                    FragmentPath='/etc/systemd/system/' + unit,
                    DropInPaths=sorted('/etc/systemd/system/' + unit + '.d/' + p.name
                        for p in self.p('/etc/systemd/system/' + unit + '.d').glob('*.conf')))

    def child_drained(self):
        if self.f['unit'] in self.stalled:
            raise RuntimeError('synthetic child remains')

    def empty_cgroup(self, group):
        raise RuntimeError('synthetic nonempty cgroup')

    def supervisor_remaining(self, unit):
        return 3600

    def authority(self, *args, **kwargs):
        # Exercise all packet/boot/deadline validation, but provide synthetic
        # actual systemd state for supervisor (not normal cold units).
        supervisor = self.c['supervisor']
        if args and args[0]:
            supervisor = m.decoded(self.read(args[0], private=True))['supervisor']
        self.runner.states[supervisor] = 'active'
        return super().authority(*args, **kwargs)

    def read_unit_exec_conditions(self, unit):
        path = '/etc/systemd/system/' + unit + '.d/90-serenity-release-guard.conf'
        raw = self.read(path).decode()
        runtime = self.runtime if self.runtime in raw else self.s['startup_runtime_path']
        return [['/usr/bin/python3', ['/usr/bin/python3', runtime, 'guard', unit], False, *([0] * 7)]]

    def read_unit_conditions(self, unit):
        return [('ConditionPathExists', False, True, m.HOLD, 0)]

    def readiness(self):
        self.pair()
        for unit in m.WRITERS:
            m.require(self.runner.states[unit] == 'active')


class Fixture:
    """Shared import-only integration fixture; source template remains inert."""
    def __init__(self, root):
        self.root = Path(root)
        self.now = 1000
        self.runner = FakeRunner()
        self.operation = 'a' * 64
        self.stage = '/var/lib/serenity-release/maintenance-' + self.operation
        self.uid, self.gid = os.getuid(), os.getgid()
        def put(path, raw, mode=0o600):
            target = self.root / path.lstrip('/')
            target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
            target.write_bytes(raw)
            target.chmod(mode)
            return m.digest(raw)
        self.put = put
        (self.root / 'run').mkdir(mode=0o755)
        boot = '00000000-0000-0000-0000-000000000001'
        put('/proc/sys/kernel/random/boot_id', boot.encode())
        source = 'b' * 40
        binary = put('/usr/local/lib/serenity-releases/current/serenity', b'unchanged binary')
        (self.root / 'usr/local/bin').mkdir(parents=True, exist_ok=True)
        (self.root / 'usr/local/bin/serenity').symlink_to('/usr/local/lib/serenity-releases/current/serenity')
        state = m.encoded(dict(version=1, installed_sequence=8, highest_sequence=8, manifest_sha256='c' * 64))
        state_sha = put(m.STATE, state)
        put(m.STATE + '.lock', b'')
        broker = put(m.BROKER, m.encoded(dict(build_sha=source, executable_sha256=binary)))
        config = put(m.HOSTED, m.encoded(dict(data_dir='/var/lib/serenity', episode_archive_enabled=False,
                     workos_enabled=False, billing_enabled=False, episode_synthetic_brains=[])))
        startup_path = '/usr/local/libexec/startup.py'
        startup_hash = put(startup_path, b'previous permanent runtime')
        pins = []
        for unit in m.UNITS:
            path = '/etc/systemd/system/' + unit
            sha = put(path, ('[Unit]\nDescription=' + unit + '\n').encode(), 0o644)
            pins.append(dict(path=path, sha256=sha))
            guard = '/etc/systemd/system/' + unit + '.d/90-serenity-release-guard.conf'
            content = ((m.MARKER + '[Service]\nExecCondition=+/usr/bin/python3 ' + startup_path + ' guard ' + unit + '\n')
                       if unit in m.WRITERS else m.MARKER + '[Unit]\nConditionPathExists=!' + m.HOLD + '\n')
            pins.append(dict(path=guard, sha256=put(guard, content.encode(), 0o644)))
        service = dict(source=source, binary_path='/usr/local/lib/serenity-releases/current/serenity',
            binary_sha256=binary, state_sha256=state_sha, broker_sha256=broker, config_sha256=config,
            installed_sequence=8, highest_sequence=8, release_manifest_sha256='c'*64,
            startup_runtime_path=startup_path, startup_runtime_sha256=startup_hash, unit_files=pins)
        fixture = dict(tool_source='d'*40, tool_path='/usr/local/bin/fixture-tool', tool_sha256=put('/usr/local/bin/fixture-tool', b'tool'),
            wrapper_path='/usr/local/bin/fixture-wrapper', wrapper_sha256=put('/usr/local/bin/fixture-wrapper', b'wrapper'),
            uid=1234, gid=1234, data_dir='/var/lib/serenity', private_dir='/var/lib/fixture', account_cap=2,
            schema_sha256='e'*64, unit='serenity-episode-fixture-' + self.operation + '.service')
        expected = dict(cli_path='/usr/bin/aws', cli_sha256='f'*64, account='000000000000', principal='synthetic',
            region='us-east-1', stack='synthetic', function='synthetic', version='1', code_sha256=base64.b64encode(b'x'*32).decode(),
            table='synthetic', namespace='synthetic', table_arn='arn:aws:dynamodb:us-east-1:000000000000:table/synthetic',
            lineage='synthetic', witness_bucket='synthetic', metadata_sha256='1'*64, tip_sha256='2'*64, minimum_revision=1)
        authority = dict(generator_path='/usr/local/bin/observer', generator_sha256=put('/usr/local/bin/observer', b'observer'),
            scope='synthetic', session_id='synthetic', concurrency_target='synthetic', restore_concurrency=1, expectations=expected)
        declaration = dict(version=1, purpose=m.PURPOSE, operation_id=self.operation, issued_at=999, expires_at=1500,
            boot_id=boot, stage=self.stage, supervisor='serenity-release-' + self.operation + '.service',
            service=service, fixture=fixture, authority=authority)
        self.config = {**declaration, 'manifest_sha256': m.digest(m.encoded(declaration))}
        put(self.stage + '/manifest.json', m.encoded(declaration))
        put(self.stage + '/runtime.py', b'pinned synthetic rendered runtime')

    def transaction(self, **kwargs):
        return SyntheticTransaction(self.config, root=self.root, runner=self.runner,
                                    uid=self.uid, gid=self.gid, clock=lambda: self.now, **kwargs)

    def final_revoke(self, t):
        nonce = '9' * 64
        proof = dict(version=3, purpose='serenity-episode-fixture-offline-v3', nonce=nonce,
            operation='revoke', hold_sha256=m.digest(t.matching_hold()), maintenance_hold=t.expected_hold(),
            service_source=t.s['source'], tool_source=t.f['tool_source'], tool_sha256=t.f['tool_sha256'], issued_at=self.now)
        proof.update(expires_at=self.now + 100, boot_id=t.c['boot_id'], uid=t.f['uid'], gid=t.f['gid'],
            tool_path=t.f['tool_path'], wrapper_path=t.f['wrapper_path'], wrapper_sha256=t.f['wrapper_sha256'],
            runtime_path=t.runtime, runtime_sha256=m.digest(t.read(t.runtime)),
            service_binary_path=t.s['binary_path'], service_binary_sha256=t.s['binary_sha256'],
            config_path=m.HOSTED, config_sha256=t.s['config_sha256'], data_dir=t.f['data_dir'],
            private_dir=t.f['private_dir'], account_cap=2, state_sha256=t.s['state_sha256'],
            ready_sha256=m.digest(t.read(t.stage + '/maintenance-ready.json')), authority_receipt_path='/synthetic/closed.json',
            authority_receipt_sha256='6'*64, authority_generator_path=t.a['generator_path'],
            authority_generator_sha256=t.a['generator_sha256'], schema_sha256=t.f['schema_sha256'],
            authority_scope=t.a['scope'], authority_session_id=t.a['session_id'], concurrency_target=t.a['concurrency_target'],
            lock_device=t.baseline()['lock_device'], lock_inode=t.baseline()['lock_inode'])
        proof_raw = m.encoded(proof)
        self.put(self.stage + '/fixture-proof-' + nonce + '.json', proof_raw)
        self.put(self.stage + '/.fixture-used-' + nonce, b'consumed\n')
        outcome = dict(version=1, purpose=proof['purpose'], operation_id=self.operation, nonce=nonce,
            operation='revoke', proof_sha256=m.digest(proof_raw), tool_source=t.f['tool_source'], tool_sha256=t.f['tool_sha256'],
            completed_at=self.now, sequence=1, result='success')
        self.put(self.stage + '/fixture-outcome-' + nonce + '.json', m.encoded(outcome))

    def authorization(self, t):
        auth = dict(version=1, purpose='serenity.fixture-closeout-authorization/v1', operation_id=self.operation,
            manifest_sha256=t.c['manifest_sha256'], hold_sha256=m.digest(m.encoded(t.expected_hold())),
            state_sha256=t.s['state_sha256'], executable_sha256=t.s['binary_sha256'],
            broker_sha256=t.s['broker_sha256'], config_sha256=t.s['config_sha256'], boot_id=t.c['boot_id'],
            runtime_sha256=m.digest(t.read(t.runtime)), entry_baseline_sha256=m.digest(t.read(t.stage + '/baseline.json')),
            supervisor='serenity-release-' + self.operation + '-closeout-' + '7'*64 + '.service',
            issued_at=self.now, expires_at=self.now + 100)
        name = self.stage + '/closeout-authorization.json'
        self.put(name, m.encoded(auth))
        return name

    def receipt(self, t):
        receipt = dict(version=1, purpose='serenity.fixture-authority-restored/v1', operation_id=self.operation,
            manifest_sha256=t.c['manifest_sha256'], hold_sha256=m.digest(m.encoded(t.expected_hold())), state_sha256=t.s['state_sha256'],
            boot_id=t.c['boot_id'], observed_at=self.now, scope=t.a['scope'], session_id=t.a['session_id'],
            concurrency_target=t.a['concurrency_target'], concurrency=1, phase='ACTIVE', revision=1, metadata_sha256='3'*64,
            tip_sha256='4'*64, request_ids=['synthetic-request'], generator_path=t.a['generator_path'], generator_sha256=t.a['generator_sha256'])
        self.put(self.stage + '/restore-receipt.json', m.encoded(receipt))
        return self.stage + '/restore-receipt.json'


class RuntimeTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.fixture = Fixture(self.temporary.name)

    def test_enter_closeout_preserves_actual_pair_and_baseline(self):
        f = self.fixture
        t = f.transaction()
        before = {p: t.read(p) for p in (m.STATE, m.BROKER, m.HOSTED, t.s['binary_path'])}
        ready = t.enter()
        self.assertEqual(ready['version'], 2)
        self.assertFalse(ready['services_started'])
        original_hold = t.matching_hold()
        f.final_revoke(t)
        result = t.closeout(f.receipt(t))
        self.assertTrue(result['services_started'])
        self.assertFalse(t.p(m.HOLD).exists())
        self.assertEqual(before, {p: t.read(p) for p in before})
        self.assertEqual(result['hold_sha256'], m.digest(original_hold))
        self.assertEqual(f.runner.states[m.TIMER], 'active')

    def test_unknown_outcome_and_derived_state_refuse(self):
        f = self.fixture
        t = f.transaction()
        t.enter()
        f.put(f.stage + '/.fixture-used-' + '8'*64, b'unknown')
        with self.assertRaises(RuntimeError):
            t.closeout(f.receipt(t))
        self.assertTrue(t.p(m.HOLD).exists())
        self.assertFalse(t.p(f.stage + '/closeout-pending.json').exists())

    def test_expired_packet_cannot_enter_or_release(self):
        f = self.fixture
        t = f.transaction()
        t.enter()
        f.final_revoke(t)
        f.now = 1600
        with self.assertRaises(RuntimeError):
            t.closeout(f.receipt(t))
        with self.assertRaises(RuntimeError):
            t.cold_ready()
        self.assertTrue(t.p(m.HOLD).exists())

    def test_crash_hold_publication_reenters_without_pair_change(self):
        f = self.fixture
        t = f.transaction(crash='hold-published')
        with self.assertRaises(Crash):
            t.enter()
        hold = t.read(m.HOLD)
        retry = f.transaction()
        retry.enter()
        self.assertEqual(retry.matching_hold(), hold)

    def test_atomic_sync_crash_reconciles_known_prefix_only(self):
        f = self.fixture
        t = f.transaction(crash='record-synced:baseline.json')
        with self.assertRaises(Crash):
            t.enter()
        f.transaction().enter()
        t = f.transaction()
        target = f.stage + '/extra.json'
        f.put(f.stage + '/.extra.json.maintenance-write', b'{wrong')
        with self.assertRaises(RuntimeError):
            t.atomic(target, b'{"known":1}')
        self.assertFalse(t.p(target).exists())

    def test_changed_baseline_or_pair_refuses_before_hold(self):
        f = self.fixture
        t = f.transaction()
        f.put(m.BROKER, b'changed')
        with self.assertRaises(RuntimeError):
            t.enter()
        self.assertFalse(t.p(m.HOLD).exists())

    def test_actual_cgroup_stall_refuses_cold(self):
        t = self.fixture.transaction()
        t.enter()
        t.stalled.add(m.WRITERS[0])
        with self.assertRaises(RuntimeError):
            t.cold_ready()

    def test_renewal_only_closes_and_keeps_original_hold_bytes(self):
        f = self.fixture
        t = f.transaction()
        t.enter()
        original = t.matching_hold()
        f.final_revoke(t)
        f.now = 1600
        auth = f.authorization(t)
        resumed = f.transaction(crash='closeout:writers-ready')
        with self.assertRaises(Crash):
            resumed.closeout(f.receipt(resumed), auth)
        self.assertEqual(resumed.matching_hold(), original)
        with self.assertRaises(RuntimeError):
            f.transaction().cold_ready()
        result = f.transaction().closeout(f.receipt(t), auth)
        self.assertTrue(result['services_started'])

    def test_forward_closeout_crashes_keep_gate_and_exact_pair(self):
        for boundary in ('closeout:intent', 'closeout:authority-restored', 'started:serenity-broker.service',
                         'closeout:writers-ready', 'closeout:guards-restored', 'closeout:hold-released',
                         'hold-released', 'closeout:timer-restored'):
            with self.subTest(boundary=boundary), tempfile.TemporaryDirectory() as directory:
                f = Fixture(directory)
                t = f.transaction()
                t.enter()
                f.final_revoke(t)
                interrupted = f.transaction(crash=boundary)
                # Intent publication's stable record checkpoint is named by
                # atomic(), while later logical stages have explicit hooks.
                if boundary == 'closeout:intent':
                    interrupted.crash = 'record-durable:closeout-pending.json'
                with self.assertRaises(Crash):
                    interrupted.closeout(f.receipt(t))
                self.assertTrue(interrupted.p(f.stage + '/closeout-pending.json').exists())
                if interrupted.p(m.HOLD).exists():
                    authorization = None
                else:
                    authorization = f.authorization(interrupted)
                closed = f.transaction().closeout(f.receipt(t), authorization)
                self.assertTrue(closed['services_started'])
                self.assertEqual(m.digest(interrupted.read(m.STATE)), t.s['state_sha256'])

    def test_sequence_bijection_and_highest_revoke(self):
        f = self.fixture
        t = f.transaction()
        t.enter()
        f.final_revoke(t)
        name = f.stage + '/fixture-outcome-' + '9'*64 + '.json'
        original = t.read(name)
        for invalid in (True, 0, 2, -1):
            with self.subTest(sequence=invalid):
                row = m.decoded(original)
                row['sequence'] = invalid
                f.put(name, m.encoded(row))
                with self.assertRaises(RuntimeError):
                    t.fixture_outcomes()
        f.put(name, original)
        self.assertEqual(len(t.fixture_outcomes()), 64)

    def test_prehold_expiry_recovery_is_closeout_only_and_snapshot_bound(self):
        f = self.fixture
        t = f.transaction(crash='record-durable:baseline.json')
        with self.assertRaises(Crash):
            t.enter()
        self.assertFalse(t.p(m.HOLD).exists())
        f.now = 1600
        auth = f.authorization(t)
        interrupted = f.transaction(crash='entry-abort-hold-published')
        with self.assertRaises(Crash):
            interrupted.closeout(f.receipt(t), auth)
        self.assertEqual(interrupted.matching_hold(), m.encoded(t.expected_hold()))
        self.assertTrue(interrupted.closeout_record()['entry_abort'])
        with self.assertRaises(RuntimeError):
            f.transaction().cold_ready()
        closed = f.transaction().closeout(f.receipt(t), auth)
        self.assertTrue(closed['services_started'])
        self.assertFalse(any('fixture-outcome-' in p.name for p in t.p(t.stage).iterdir()))

    def test_prehold_unknown_custody_or_child_history_parks(self):
        for unknown in ('snapshot', 'nonce'):
            with self.subTest(unknown=unknown), tempfile.TemporaryDirectory() as directory:
                f = Fixture(directory)
                t = f.transaction(crash='record-durable:baseline.json')
                with self.assertRaises(Crash):
                    t.enter()
                f.now = 1600
                auth = f.authorization(t)
                if unknown == 'nonce':
                    f.put(f.stage + '/.fixture-used-' + '8'*64, b'consumed\n')
                else:
                    # Replace only the injected service-UID observation, not the
                    # saved root baseline or canonical service files.
                    original_run = f.runner.run
                    def changed(argv, **kwargs):
                        result = original_run(argv, **kwargs)
                        if len(argv) > 1 and argv[1] == '--fixture-custody-snapshot':
                            data = m.decoded(result.stdout)
                            data['fixture_scope_sha256'] = 'c'*64
                            result.stdout = m.encoded(data).decode()
                        return result
                    f.runner.run = changed
                with self.assertRaises(RuntimeError):
                    f.transaction().closeout(f.receipt(t), auth)
                self.assertFalse(t.p(m.HOLD).exists())
                self.assertFalse(t.p(f.stage + '/closeout-pending.json').exists())

    def test_process_death_releases_stable_lock_but_preserves_hold(self):
        import sys
        f = self.fixture
        script = """import sys,os,json
from pathlib import Path
sys.path.insert(0,sys.argv[1])
from test_runtime import SyntheticTransaction,FakeRunner
with open(sys.argv[2]) as stream: config=json.load(stream)
t=SyntheticTransaction(config,root=Path(sys.argv[3]),runner=FakeRunner(),uid=os.getuid(),gid=os.getgid(),clock=lambda:1000)
t.enter()
with t.lock():
 print('locked',flush=True)
 sys.stdin.buffer.read(1)
"""
        config_path = f.root / 'test-config.json'
        config_path.write_bytes(m.encoded(f.config))
        child = subprocess.Popen([sys.executable, '-I', '-c', script, str(HERE), str(config_path), str(f.root)],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        try:
            import selectors
            with selectors.DefaultSelector() as selector:
                selector.register(child.stdout, selectors.EVENT_READ)
                self.assertTrue(selector.select(10), 'helper must reach actual retained EX lock')
            self.assertEqual(child.stdout.readline(), b'locked\n')
            t = f.transaction()
            hold = t.read(m.HOLD)
            lock_inode = t.p(m.STATE + '.lock').stat().st_ino
            with self.assertRaises(BlockingIOError):
                with t.lock():
                    self.fail('competing process acquired actual lock')
            child.kill()
            child.wait(timeout=10)
            self.assertEqual(t.read(m.HOLD), hold)
            with t.lock():
                self.assertEqual(t.p(m.STATE + '.lock').stat().st_ino, lock_inode)
            t.enter()
            self.assertEqual(t.matching_hold(), hold)
        finally:
            if child.poll() is None:
                child.kill()
            child.communicate(timeout=10)
            child.stdin.close()
            child.stdout.close()
            child.stderr.close()

    def test_unsafe_hardlink_pin_and_foreign_dropin_refuse(self):
        f = self.fixture
        t = f.transaction()
        name = '/etc/systemd/system/' + m.WRITERS[0] + '.d/foreign.conf'
        f.put(name, b'[Service]\nRestart=always\n', 0o644)
        with self.assertRaises(RuntimeError):
            t.enter()
        self.assertFalse(t.p(m.HOLD).exists())
        t.p(name).unlink()
        os.link(t.p(m.STATE), f.root / 'foreign-state-link')
        with self.assertRaises(RuntimeError):
            t.enter()
        self.assertFalse(t.p(m.HOLD).exists())

    def test_template_cli_is_inert(self):
        result = subprocess.run(['/usr/bin/python3', '-I', str(HERE / 'runtime.py'), 'enter'], capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn(b'Traceback', result.stderr)

class TypedCustodyTests(unittest.TestCase):
    def test_ref_unit_connection_retained_and_unref_on_failure(self):
        calls = []
        class Bus:
            def manager(self, method, unit):
                calls.append((method, unit))
                return '/org/freedesktop/systemd1/unit/synthetic'
            def conditions(self, path):
                calls.append(('conditions', path))
                raise RuntimeError('synthetic malformed typed condition')
            def close(self):
                calls.append(('close',))
        with self.assertRaises(RuntimeError):
            m.unit_conditions(m.BACKUP, bus_factory=Bus)
        self.assertEqual([x[0] for x in calls], ['RefUnit', 'GetUnit', 'conditions', 'UnrefUnit', 'close'])

    def test_negated_mandatory_guard_rejects_trigger_and_positive(self):
        self.assertFalse(m.mandatory_negated_condition([('ConditionPathExists', True, True, m.HOLD, 0)], m.HOLD))
        self.assertFalse(m.mandatory_negated_condition([('ConditionPathExists', False, False, m.HOLD, 0)], m.HOLD))
        self.assertTrue(m.mandatory_negated_condition([('ConditionPathExists', False, True, m.HOLD, 0)], m.HOLD))


class PropertyTransportTests(unittest.TestCase):
    def test_real_systemd_255_direct_property_shapes(self):
        t = object.__new__(m.Transaction)
        class PropertyRunner:
            def run(self, argv, *, check=True):
                return subprocess.CompletedProcess(argv, 0, json.dumps(self.value), '')
        runner = PropertyRunner()
        t.runner = runner
        for signature, data in [('s', 'inactive'), ('u', 0), ('t', 1000000),
                                ('as', []), ('as', ['synthetic.service']), ('(uo)', [0, '/'])]:
            with self.subTest(signature=signature, data=data):
                runner.value = dict(type=signature, data=data)
                self.assertEqual(t.property(None, '/synthetic', 'synthetic', 'synthetic', signature), data)
        for signature, data in [('s', ['inactive']), ('u', True), ('t', -1),
                                ('as', [['synthetic.service']]), ('(uo)', [[0, '/']]), ('(uo)', [False, '/'])]:
            with self.subTest(signature=signature, invalid=data), self.assertRaises(RuntimeError):
                runner.value = dict(type=signature, data=data)
                t.property(None, '/synthetic', 'synthetic', 'synthetic', signature)


if __name__ == '__main__':
    unittest.main()
