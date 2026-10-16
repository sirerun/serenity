"""Synthetic filesystem/systemd custody model; never invokes a live unit/provider."""
import copy
import contextlib
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import tempfile
import types
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('fixture_operator', HERE / 'offline_operator.py')
a = importlib.util.module_from_spec(spec)
spec.loader.exec_module(a)


def blob(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':')).encode()


class Model(a.Adapter):
    def __init__(self, root):
        self.now = 1000
        self.states = {}
        super().__init__(root=root, owner=os.geteuid(), clock=lambda: self.now,
                         systemctl=lambda unit: dict(self.states[unit]))

    def exec_conditions(self, unit):
        return self.states[unit]['ExecCondition']

    def trusted(self, path, private=False, directory=False, owner=None):
        # Model service UID 1200 on unprivileged or root test workers. No real
        # ownership changes, live systemctl, provider call or subprocess launch.
        if owner == 1200:
            owner = self.owner
        return super().trusted(path, private, directory, owner)


class CustodyTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.model = Model(self.root)
        self.addCleanup(self.model.close)
        self.hold = dict(manifest_sha256='a'*64, executable_sha256='b'*64,
                         source='c'*40, previous_state_sha256=a.digest(b'previous-state'),
                         expected_state_sha256='d'*64, supervisor='serenity-release-fixture.service',
                         stage='/var/lib/serenity-release/fixture', backup_timer_prior_state='active',
                         phase='maintenance-ready', timestamp=990)
        self.proof = dict(version=1, purpose='serenity-episode-fixture-offline-v1', operation='prepare',
            nonce='1'*64, issued_at=1000, expires_at=1120, boot_id='00000000-0000-0000-0000-000000000001',
            uid=1200, gid=1200, tool_source='e'*40, tool_path='/opt/fixture/tool', tool_sha256=a.digest(b'tool'),
            wrapper_path='/opt/fixture/offline_operator.py', wrapper_sha256=a.digest(b'wrapper'),
            runtime_path=self.hold['stage']+'/activation-runtime.py', runtime_sha256='',
            service_binary_path='/opt/service/serenity', service_binary_sha256=a.digest(b'service'), service_source='f'*40,
            config_path=a.HOSTED, config_sha256='', data_dir='/data', private_dir='/fixture-private', account_cap=10,
            hold_sha256=a.digest(blob(self.hold)), hold=self.hold, state_sha256=self.hold['previous_state_sha256'], ready_sha256='',
            authority_receipt_path='/proof/authority.json', authority_receipt_sha256='',
            authority_generator_path='/proof/generator.py', authority_generator_sha256=a.digest(b'generator'),
            lock_device=0, lock_inode=0, schema_sha256='2'*64,authority_scope='synthetic-scope',authority_session_id='synthetic-session',concurrency_target='synthetic-function')
        self.put(a.STATE+'.lock', b'')
        st=self.model.path(a.STATE+'.lock').stat()
        self.proof.update(lock_device=st.st_dev, lock_inode=st.st_ino)
        self.put(a.STATE, b'previous-state')
        self.put(a.HOLD, blob(self.hold))
        config = blob(dict(data_dir='/data', account_cap=10, episode_archive_enabled=False, episode_synthetic_brains=[]))
        self.proof['config_sha256']=a.digest(config)
        self.put(a.HOSTED, config)
        c={key:self.hold[key] for key in ('manifest_sha256','executable_sha256','previous_state_sha256','expected_state_sha256','supervisor','stage')}
        c.update(commit=self.hold['source'], previous_commit=self.proof['service_source'], previous_executable_sha256=self.proof['service_binary_sha256'], hosted_config_sha256=self.proof['config_sha256'])
        runtime = ('CONFIG='+repr(c)+'\n'
            'def unit_conditions(unit): return [("ConditionPathExists", False, True, '+repr(a.HOLD)+', 0)]\n'
            'def mandatory_negated_condition(rows,path): return any(row[:4]==("ConditionPathExists",False,True,path) for row in rows)\n').encode()
        self.put(self.proof['runtime_path'],runtime)
        self.proof['runtime_sha256']=a.digest(runtime)
        ready=blob(dict(manifest_sha256=self.hold['manifest_sha256'],state_sha256=self.proof['state_sha256'],installed_pair='previous-unmodified',held=True,services_started=False))
        self.put(self.hold['stage']+'/maintenance-ready.json', ready)
        self.proof['ready_sha256']=a.digest(ready)
        self.authority=dict(version=1,observed_at=999,status='CLOSED',active_leases=0,concurrency=0,
            session_id='synthetic-session',scope='synthetic-scope',boot_id=self.proof['boot_id'],
            hold_sha256=self.proof['hold_sha256'],state_sha256=self.proof['state_sha256'],request_ids=['synthetic-request'],concurrency_target='synthetic-function')
        self.write_authority()
        for path,data in [(self.proof['tool_path'],b'tool'),(self.proof['wrapper_path'],b'wrapper'),(self.proof['service_binary_path'],b'service'),(self.proof['authority_generator_path'],b'generator')]:
            self.put(path,data)
        self.put('/proc/sys/kernel/random/boot_id', self.proof['boot_id'].encode())
        self.model.path('/usr/local/bin').mkdir(parents=True,mode=0o700)
        self.model.path('/usr/local/bin/serenity').symlink_to(self.proof['service_binary_path'])
        for path in ('/data','/data/brains','/fixture-private'):
            self.model.path(path).mkdir(parents=True,mode=0o700)
        self.put('/data/control.db',b'synthetic-db-placeholder')  # wrapper never opens SQLite
        for unit in a.UNITS:
            base='/etc/systemd/system/'+unit+'.d/'
            if unit in a.WRITERS:
                content=a.MARKER+'[Service]\nExecCondition=+/usr/bin/python3 '+self.proof['runtime_path']+' guard '+unit+'\n'
                self.put(base+'91-serenity-release-maintenance.conf',(a.MARKER+'[Unit]\nBindsTo='+self.hold['supervisor']+'\nAfter='+self.hold['supervisor']+'\n[Service]\nRestart=no\n').encode(),0o644)
            else:
                content=a.MARKER+'[Unit]\nConditionPathExists=!'+a.HOLD+'\n'
            self.put(base+'90-serenity-release-guard.conf',content.encode(),0o644)
            self.model.states[unit]=dict(LoadState='loaded',ActiveState='inactive',SubState='dead',MainPID='0',ControlPID='0',Job='',NeedDaemonReload='no',ExecCondition=[['/usr/bin/python3', ['/usr/bin/python3', self.proof['runtime_path'], 'guard', unit], False, 0, 0, 0, 0, 0, 0, 0]],BindsTo=self.hold['supervisor'],Restart='no',User='serenity',Group='serenity',ControlGroup='')
        self.write_proof()

    def test_other_executable_with_guard_text_refused(self):
        self.model.prepare('/proof/proof.json')
        for unit in a.WRITERS:
            row = self.model.states[unit]['ExecCondition'][0]
            row[0] = row[1][0] = '/usr/bin/true'
        with self.assertRaises(RuntimeError):
            self.model.refresh()
        self.model.close()
        with self.assertRaises(RuntimeError):
            self.model.prepare('/proof/proof.json')

    def test_python_non_guard_invocation_refused(self):
        self.model.prepare('/proof/proof.json')
        for unit in a.WRITERS:
            self.model.states[unit]['ExecCondition'][0][1] = [
                '/usr/bin/python3', '-c', 'pass', self.proof['runtime_path'], 'guard', unit]
        with self.assertRaises(RuntimeError):
            self.model.refresh()
        self.model.close()
        with self.assertRaises(RuntimeError):
            self.model.prepare('/proof/proof.json')

    def test_ambiguous_or_ignored_exec_conditions_refused(self):
        self.model.prepare('/proof/proof.json')
        unit = a.WRITERS[0]
        good = copy.deepcopy(self.model.states[unit]['ExecCondition'])
        ignored = copy.deepcopy(good)
        ignored[0][2] = True
        malformed_bool = copy.deepcopy(good)
        malformed_bool[0][2] = 0
        malformed_pid = copy.deepcopy(good)
        malformed_pid[0][7] = True
        for rows in ([], good + good, good + ignored, ignored, malformed_bool,
                     malformed_pid, 'path=/usr/bin/python3 ; ignore_errors=no'):
            with self.subTest(rows=rows):
                self.model.states[unit]['ExecCondition'] = rows
                with self.assertRaises(RuntimeError):
                    self.model.refresh()
        self.model.states[unit]['ExecCondition'] = good
        self.model.refresh()

    def test_typed_exec_condition_transport_and_reference_cleanup(self):
        # A synthetic instance of busctl's typed property representation. The
        # parent's real read-only receipt remains a separate integration gate.
        unit = a.WRITERS[0]
        rows = self.model.states[unit]['ExecCondition']
        payload = blob(dict(type=a.EXEC_CONDITION_TYPE, data=rows))
        bus = mock.Mock()
        bus.manager.return_value = '/org/freedesktop/systemd1/unit/serenity_2dhosted_2eservice'
        self.model.module = types.SimpleNamespace(SystemdBus=lambda: bus)
        with mock.patch.object(a.subprocess, 'run', return_value=types.SimpleNamespace(stdout=payload)) as run:
            result = a.Adapter.exec_conditions(self.model, unit)
            a.exact_guard(result, self.proof['runtime_path'], unit)
            self.assertEqual(run.call_args.args[0], [
                '/usr/bin/busctl', '--system', '--json=short', 'get-property',
                'org.freedesktop.systemd1', bus.manager.return_value,
                'org.freedesktop.systemd1.Service', 'ExecCondition'])
        self.assertEqual(bus.manager.call_args_list, [mock.call('RefUnit', unit), mock.call('GetUnit', unit), mock.call('UnrefUnit', unit)])
        bus.close.assert_called_once()
        for bad in (b'{}', b'{"type":"s","data":[]}', blob(dict(type=a.EXEC_CONDITION_TYPE, data=[rows])),
                    b'{"type":"a(sasbttttuii)","data":[],"data":[]}'):
            bus.reset_mock()
            with mock.patch.object(a.subprocess, 'run', return_value=types.SimpleNamespace(stdout=bad)):
                with self.assertRaises(RuntimeError):
                    result = a.Adapter.exec_conditions(self.model, unit)
                    a.exact_guard(result, self.proof['runtime_path'], unit)
            bus.manager.assert_any_call('UnrefUnit', unit)
            bus.close.assert_called_once()

    def test_observed_get_property_representation(self):
        # Shape captured from both running writer units with busctl get-property;
        # runtime path is normalized to this isolated model. No provider claim.
        for unit in a.WRITERS:
            row = ['/usr/bin/python3', ['/usr/bin/python3', self.proof['runtime_path'],
                   'guard', unit], False, 0, 0, 0, 0, 0, 0, 0]
            payload = blob({'type': 'a(sasbttttuii)', 'data': [row]})
            a.exact_guard(a.decode_exec_conditions(payload), self.proof['runtime_path'], unit)
            with self.assertRaises(RuntimeError):
                a.exact_guard(a.decode_exec_conditions(blob({'type': 'a(sasbttttuii)',
                    'data': [[row]]})), self.proof['runtime_path'], unit)

    def test_exec_condition_query_failure_releases_reference(self):
        unit = a.WRITERS[0]
        bus = mock.Mock()
        self.model.module = types.SimpleNamespace(SystemdBus=lambda: bus)
        for failure in (FileNotFoundError(), a.subprocess.TimeoutExpired('busctl', 5),
                        a.subprocess.CalledProcessError(1, 'busctl')):
            bus.reset_mock()
            with self.subTest(failure=type(failure).__name__):
                with mock.patch.object(a.subprocess, 'run', side_effect=failure):
                    with self.assertRaises(type(failure)):
                        a.Adapter.exec_conditions(self.model, unit)
                bus.manager.assert_any_call('UnrefUnit', unit)
                bus.close.assert_called_once()

    def put(self,path,data,mode=0o600):
        p=self.model.path(path)
        p.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
        p.write_bytes(data);p.chmod(mode)
        return p

    def write_authority(self):
        data=blob(self.authority)
        self.put(self.proof['authority_receipt_path'],data)
        self.proof['authority_receipt_sha256']=a.digest(data)

    def write_proof(self):
        self.put('/proof/proof.json',blob(self.proof))

    def test_valid_model_and_one_use_proof(self):
        fds=self.model.prepare('/proof/proof.json')
        self.assertEqual(len(fds),6)
        self.model.refresh()
        self.model.consume('/proof/proof.json')
        with self.assertRaises(FileExistsError):self.model.consume('/proof/proof.json')
        self.assertEqual(self.model.path(a.HOLD).read_bytes(),blob(self.hold))

    def test_proof_fields_refused(self):
        for key,value in [('expires_at',999),('issued_at',1001),('expires_at',1400),('uid',0),('operation','serve'),('tool_source','unknown'),('config_path','/etc/other.json'),('lock_inode',1),('tool_sha256','0'*64)]:
            with self.subTest(key=key,value=value):
                p=copy.deepcopy(self.proof);p[key]=value
                self.put('/proof/changed.json',blob(p))
                m=Model(self.root);m.states=self.model.states
                try:
                    with self.assertRaises((RuntimeError,FileNotFoundError)):m.prepare('/proof/changed.json')
                finally:m.close()

    def test_bad_authority_refused_even_with_matching_receipt_hash(self):
        for key,value in [('status','OPEN'),('active_leases',1),('concurrency',1),('observed_at',900),('boot_id','wrong'),('request_ids',[]),('scope','foreign-scope'),('concurrency_target','foreign-function')]:
            with self.subTest(key=key):
                saved=self.authority.copy();self.authority[key]=value;self.write_authority();self.write_proof()
                m=Model(self.root);m.states=self.model.states
                try:
                    with self.assertRaises(RuntimeError):m.prepare('/proof/proof.json')
                finally:m.close()
                self.authority=saved

    def test_missing_and_unsafe_proof(self):
        self.model.path('/proof/proof.json').chmod(0o644)
        with self.assertRaises(RuntimeError):self.model.prepare('/proof/proof.json')

    def test_duplicate_json_refused(self):
        with self.assertRaises(RuntimeError):a.decode(b'{"version":1,"version":1}')

    def test_state_lock_contention(self):
        self.model.prepare('/proof/proof.json')
        other=Model(self.root);self.addCleanup(other.close);other.states=self.model.states
        with self.assertRaises(BlockingIOError):other.prepare('/proof/proof.json')

    def test_refresh_detects_file_replacement_races(self):
        for path in (a.HOLD,a.STATE,'/proof/proof.json',self.proof['runtime_path'],self.proof['tool_path'],a.HOSTED):
            with self.subTest(path=path):
                m=Model(self.root);m.states=self.model.states
                try:
                    m.prepare('/proof/proof.json')
                    p=m.path(path);backup=p.with_suffix(p.suffix+'.saved');p.rename(backup)
                    p.write_bytes(backup.read_bytes());p.chmod(backup.stat().st_mode & 0o777)
                    try:
                        with self.assertRaises(RuntimeError):m.refresh()
                    finally:
                        p.unlink();backup.rename(p)
                finally:m.close()

    def test_permit_and_service_races(self):
        self.model.prepare('/proof/proof.json')
        permit=self.put(a.RUN+'/'+a.WRITERS[0]+'.permit',b'not-authority')
        with self.assertRaises(RuntimeError):self.model.refresh()
        permit.unlink()
        for prop,value in [('ActiveState','active'),('MainPID','42'),('ControlPID','42'),('Job','5'),('NeedDaemonReload','yes'),('LoadState','not-found'),('Restart','always'),('ExecCondition','ignored guard')]:
            original=self.model.states[a.WRITERS[0]][prop]
            self.model.states[a.WRITERS[0]][prop]=value
            with self.subTest(prop=prop):
                with self.assertRaises(RuntimeError):self.model.refresh()
            self.model.states[a.WRITERS[0]][prop]=original
        self.model.now=1120
        with self.assertRaises(RuntimeError):self.model.refresh()

    def test_positive_or_trigger_backup_condition_refused(self):
        self.model.prepare('/proof/proof.json')
        for trigger,negate in [(False,False),(True,True)]:
            self.model.module.unit_conditions=lambda unit:[('ConditionPathExists',trigger,negate,a.HOLD,0)]
            with self.assertRaises(RuntimeError):self.model.refresh()

    def test_hold_wrong_phase_refused(self):
        self.hold['phase']='failed'
        self.proof['hold_sha256']=a.digest(blob(self.hold));self.put(a.HOLD,blob(self.hold));self.write_proof()
        with self.assertRaises(RuntimeError):self.model.prepare('/proof/proof.json')

    def test_startup_guard_writable_refused(self):
        p=self.model.path('/etc/systemd/system/'+a.WRITERS[0]+'.d/90-serenity-release-guard.conf')
        p.chmod(0o666)
        with self.assertRaises(RuntimeError):self.model.prepare('/proof/proof.json')

    def test_root_proof_symlink_refused(self):
        p=self.model.path('/proof/proof.json');p.rename(p.with_name('real.json'));p.symlink_to('real.json')
        with self.assertRaises(RuntimeError):self.model.prepare('/proof/proof.json')

    def adoption_proof(self):
        p = copy.deepcopy(self.proof)
        p.update(version=2, purpose='serenity-episode-fixture-offline-v2', operation='adopt-source',
                 source_adoption=dict(old_source='3'*40, old_manifest_sha256='4'*64))
        return p

    def test_adoption_packet_preserved_in_inherited_fd_and_one_use(self):
        self.proof = self.adoption_proof()
        self.write_proof()
        fds = self.model.prepare('/proof/proof.json')
        self.assertEqual(len(fds), 6)
        self.assertEqual(os.pread(fds[0], 65536, 0), blob(self.proof))
        self.model.consume('/proof/proof.json')
        self.model.refresh()
        with self.assertRaises(FileExistsError):
            self.model.consume('/proof/proof.json')
        # The root adapter never reads the private manifest/keys or rewrites
        # source authority. The child hashes/decodes under its fixture lock.
        self.assertEqual(list(self.model.path('/fixture-private').iterdir()), [])
        self.assertEqual(self.model.path('/data/control.db').read_bytes(), b'synthetic-db-placeholder')

    def test_adoption_proof_exact_extension(self):
        original = self.adoption_proof()
        for key, value in [('version', 1), ('version', 3), ('version', True),
                           ('purpose', 'serenity-episode-fixture-offline-v1'),
                           ('operation', 'prepare'), ('operation', 'reissue'), ('operation', 'revoke'),
                           ('source_adoption', None), ('source_adoption', {}),
                           ('source_adoption', dict(old_source='3'*40)),
                           ('source_adoption', dict(old_source='3'*40, old_manifest_sha256='4'*64, extra=True)),
                           ('source_adoption', dict(old_source=original['tool_source'], old_manifest_sha256='4'*64)),
                           ('source_adoption', dict(old_source='bad', old_manifest_sha256='4'*64)),
                           ('source_adoption', dict(old_source='3'*40, old_manifest_sha256='bad'))]:
            with self.subTest(key=key, value=value):
                p = copy.deepcopy(original)
                p[key] = value
                with self.assertRaises(RuntimeError):
                    self.model.validate_proof(blob(p))
        p = copy.deepcopy(original)
        del p['source_adoption']
        with self.assertRaises(RuntimeError):
            self.model.validate_proof(blob(p))
        for mode in ('prepare', 'reissue', 'revoke'):
            p = copy.deepcopy(self.proof)
            p['operation'] = mode
            self.model.validate_proof(blob(p))
            for extra in (None, original['source_adoption']):
                p['source_adoption'] = extra
                with self.assertRaises(RuntimeError):
                    self.model.validate_proof(blob(p))

    def test_adoption_custody_refresh_still_checks_all_pins_and_units(self):
        self.proof = self.adoption_proof()
        self.write_proof()
        self.model.prepare('/proof/proof.json')
        self.proof['source_adoption']['old_manifest_sha256'] = '5'*64
        self.write_proof()
        with self.assertRaises(RuntimeError):
            self.model.refresh()

    def test_adoption_keeps_cold_state_and_authority_requirements(self):
        self.proof = self.adoption_proof()
        for key, value in [('state_sha256', '9'*64), ('schema_sha256', ''),
                           ('lock_inode', 1), ('uid', 0), ('expires_at', 999),
                           ('hold_sha256', '9'*64), ('config_sha256', '9'*64),
                           ('authority_receipt_sha256', '9'*64), ('tool_sha256', '9'*64)]:
            with self.subTest(key=key):
                p = copy.deepcopy(self.proof)
                p[key] = value
                self.put('/proof/changed.json', blob(p))
                model = Model(self.root)
                model.states = self.model.states
                try:
                    with self.assertRaises(RuntimeError):
                        model.prepare('/proof/changed.json')
                finally:
                    model.close()
        self.write_proof()
        self.model.states[a.WRITERS[0]]['ActiveState'] = 'active'
        with self.assertRaises(RuntimeError):
            self.model.prepare('/proof/proof.json')

    def test_adoption_child_outcome_is_not_success_or_generic_denial(self):
        # Execute orchestration model only: no real UID transition, process,
        # systemd call, root filesystem access or production adoption.
        adapter = a.Adapter()
        adapter.proof = self.adoption_proof()
        adapter.proof['wrapper_path'] = str(Path(a.__file__).absolute())
        child = mock.Mock()
        child.poll.return_value = 3
        child.returncode = 3
        fds = (30, 31, 32, 33, 34, 35)
        with mock.patch.object(a.os, 'geteuid', return_value=0), \
             mock.patch.object(a.sys, 'flags', types.SimpleNamespace(isolated=1)), \
             mock.patch.object(a.pwd, 'getpwnam', return_value=types.SimpleNamespace(pw_uid=1200, pw_gid=1200)), \
             mock.patch.object(adapter, 'prepare', return_value=fds), \
             mock.patch.object(adapter, 'consume') as consume, \
             mock.patch.object(adapter, 'refresh'), \
             mock.patch.object(a.subprocess, 'Popen', return_value=child) as launch:
            with self.assertRaises(a.AdoptionOutcome) as outcome:
                adapter.execute('/proof/proof.json')
            self.assertEqual(outcome.exception.code, 3)
            consume.assert_called_once_with('/proof/proof.json')
            self.assertEqual(launch.call_args.kwargs['pass_fds'], fds)
            self.assertEqual(launch.call_args.kwargs['user'], 1200)
            self.assertEqual(launch.call_args.kwargs['extra_groups'], [])
            self.assertEqual(launch.call_args.args[0][1:], ['--custody-fds', '30,31,32,33,34,35'])
            child.wait.assert_called_once_with(timeout=5)

    def test_adoption_main_preserves_publication_outcomes(self):
        for code in (2, 3, 5, 6):
            with self.subTest(code=code):
                self.model.proof = self.adoption_proof()
                output = io.StringIO()
                with mock.patch.object(a, 'Adapter', return_value=self.model), \
                     mock.patch.object(self.model, 'execute', side_effect=a.AdoptionOutcome(code)), \
                     mock.patch.object(a.signal, 'signal'), mock.patch.object(a.signal, 'alarm'), \
                     contextlib.redirect_stderr(output):
                    self.assertEqual(a.main(['operator', '/proof/proof.json']), code)
                self.assertIn(a.ADOPTION_OUTCOMES[code], output.getvalue())
                self.assertIn('proof consumed', output.getvalue())
        # A killed child, wrapper interruption or failed final custody refresh
        # cannot establish whether atomic publication happened.
        self.model.proof = self.adoption_proof()
        output = io.StringIO()
        with mock.patch.object(a, 'Adapter', return_value=self.model), \
             mock.patch.object(self.model, 'execute', side_effect=RuntimeError('synthetic interruption')), \
             mock.patch.object(a.signal, 'signal'), mock.patch.object(a.signal, 'alarm'), \
             contextlib.redirect_stderr(output):
            self.assertEqual(a.main(['operator', '/proof/proof.json']), 6)
        self.assertIn('publication not established', output.getvalue())
        self.assertNotIn('REFUSED:', output.getvalue())


if __name__=='__main__':
    unittest.main()
