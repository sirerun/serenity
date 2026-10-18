"""Filesystem/UID-dispatch binding tests; mocked units are not live cold authority."""
import copy
import json
import os
from pathlib import Path
import tempfile
import types
import unittest

from test_fixture_proof import consumer, cohort, load, producer, HERE

collector = load('v3_binding_collector', HERE / 'collect_fixture_binding.py')


class BindingCollectorTests(unittest.TestCase):
    def fixture(self):
        temp = tempfile.TemporaryDirectory(); self.addCleanup(temp.cleanup); root = Path(temp.name).resolve(); owner = os.geteuid()
        pins, _, _, now = cohort(); c = pins['config']; service, f, a = c['service'], c['fixture'], c['authority']
        def put(path, raw):
            p = root / path.lstrip('/'); p.parent.mkdir(mode=0o700, parents=True, exist_ok=True); p.write_bytes(raw); p.chmod(0o600)
        config = producer.raw(dict(data_dir=f['data_dir'], account_cap=f['account_cap'], episode_archive_enabled=False, episode_synthetic_brains=[], workos_enabled=False, billing_enabled=False))
        service['config_sha256'] = producer.digest(config); put(collector.HOSTED, config)
        put(collector.STATE, b'synthetic-state'); service['state_sha256'] = producer.digest(b'synthetic-state')
        broker = producer.raw(dict(build_sha=service['source'], executable_sha256=producer.digest(b'service')))
        put('/etc/serenity-authority/broker.json', broker); service['broker_sha256'] = producer.digest(broker)
        for path, raw, target, key in [(service['binary_path'], b'service', service, 'binary_sha256'), (f['tool_path'], b'tool', f, 'tool_sha256'), (f['wrapper_path'], (HERE.parents[2]/'evals/episode-qualification/offline_operator.py').read_bytes(), f, 'wrapper_sha256'), (a['generator_path'], b'generator', a, 'generator_sha256')]:
            put(path, raw); target[key] = producer.digest(raw)
        c['manifest_sha256'] = producer.digest(producer.raw({k:v for k,v in c.items() if k!='manifest_sha256'}))
        h = dict(version=1, purpose=c['purpose'], operation_id=c['operation_id'], manifest_sha256=c['manifest_sha256'], service_source=service['source'], executable_sha256=service['binary_sha256'], state_sha256=service['state_sha256'], broker_sha256=service['broker_sha256'], config_sha256=service['config_sha256'], supervisor=c['supervisor'], stage=c['stage'], backup_timer_prior_state='active', phase='maintenance-ready', timestamp=c['issued_at'], expires_at=c['expires_at'])
        hold_raw = producer.raw(h); put(collector.HOLD, hold_raw)
        ready = dict(version=2, purpose=c['purpose'], operation_id=c['operation_id'], manifest_sha256=c['manifest_sha256'], state_sha256=service['state_sha256'], hold_sha256=producer.digest(hold_raw), service_source=service['source'], executable_sha256=service['binary_sha256'], held=True, services_started=False)
        put(c['stage']+'/maintenance-ready.json', producer.raw(ready)); put(collector.STATE+'.lock', b'')
        runtime = ('CONFIG='+repr(c)+'\nWRITERS='+repr(consumer.WRITERS)+'\nHOLD='+repr(collector.HOLD)+'\ndef unit_conditions(unit):return []\ndef mandatory_negated_condition(rows,path):return True\nclass Transaction:\n def __init__(self,config,**kwargs):assert config==CONFIG\n def cold_ready(self):return '+repr(ready)+'\n').encode()
        put(pins['runtime_path'], runtime); pins['runtime_sha256'] = producer.digest(runtime)
        put('/proc/sys/kernel/random/boot_id', c['boot_id'].encode())
        for name in (f['data_dir'], f['data_dir']+'/brains', f['private_dir']): (root/name.lstrip('/')).mkdir(mode=0o700, parents=True, exist_ok=True)
        put(f['data_dir']+'/control.db', b'metadata-read-is-dispatched-not-opened-as-root')
        class Adapter(consumer.Adapter):
            def __init__(self, **kwargs):
                super().__init__(**kwargs, systemctl=self.units)
            def trusted(self, path, private=False, directory=False, owner=None): return super().trusted(path, private, directory, self.owner if owner==f['uid'] else owner)
            def validate_v3_runtime(self): pass  # synthetic runtime fixture, no live authority claim
            def units(self, unit): return dict(LoadState='loaded', ActiveState='inactive', SubState='dead', NeedDaemonReload='no', Job='0', MainPID='0', ControlPID='0', ControlGroup='', BindsTo=c['supervisor'], Restart='no', User='serenity', Group='serenity')
            def exec_conditions(self, unit): return [['/usr/bin/python3',['/usr/bin/python3',pins['runtime_path'],'guard',unit],False,0,0,0,0,0,0,0]]
        calls=[]
        def metadata(argv, **kwargs):
            calls.append((argv,kwargs)); self.assertEqual(kwargs['user'],f['uid']); self.assertEqual(kwargs['group'],f['gid']); self.assertEqual(kwargs['extra_groups'],[])
            self.assertIn('mode=ro&immutable=1',argv[3]); return types.SimpleNamespace(returncode=0,stdout=json.dumps(dict(schema_sha256=f['schema_sha256'])).encode())
        options=dict(root=root,owner=owner,clock=lambda:now,metadata_runner=metadata,identity=types.SimpleNamespace(pw_uid=f['uid'],pw_gid=f['gid']),adapter_factory=Adapter)
        return pins, root, put, options, calls

    def test_read_only_actual_files_and_metadata_service_uid_dispatch(self):
        pins,root,_,options,calls=self.fixture(); before={str(p.relative_to(root)):p.read_bytes() for p in root.rglob('*') if p.is_file()}
        binding=collector.collect(pins,**options)
        self.assertEqual(binding['maintenance_hold']['service_source'],pins['config']['service']['source']);self.assertEqual(len(calls),1)
        after={str(p.relative_to(root)):p.read_bytes() for p in root.rglob('*') if p.is_file()};self.assertEqual(before,after)

    def test_refuse_all_sidecars_and_closeout_intent(self):
        for suffix in ('-wal','-shm','-journal','closing'):
            pins,root,put,options,_=self.fixture()
            path=pins['config']['stage']+'/closeout-pending.json' if suffix=='closing' else pins['config']['fixture']['data_dir']+'/control.db'+suffix
            put(path,b'')
            with self.subTest(suffix=suffix),self.assertRaises(RuntimeError):collector.collect(pins,**options)


if __name__ == '__main__':unittest.main()
