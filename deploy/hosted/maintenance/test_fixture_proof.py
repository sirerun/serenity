"""Pure synthetic v3 producer/consumer shapes; no provider, UID drop or service."""
import copy
import datetime
import importlib.util
import json
from pathlib import Path
import unittest

HERE = Path(__file__).resolve().parent

def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path); module = importlib.util.module_from_spec(spec); spec.loader.exec_module(module); return module

producer = load('v3_producer', HERE / 'render_fixture_proof.py')
fixtures = load('v3_runtime_fixtures', HERE / 'test_render_runtime.py')
consumer = load('v3_consumer', HERE.parents[2] / 'evals/episode-qualification/offline_operator.py')


def cohort():
    c = fixtures.declaration(); c['manifest_sha256'] = producer.digest(producer.raw(c))
    s, f, a = c['service'], c['fixture'], c['authority']; now = c['issued_at'] + 10
    hold = dict(version=1, purpose=c['purpose'], operation_id=c['operation_id'], manifest_sha256=c['manifest_sha256'], service_source=s['source'], executable_sha256=s['binary_sha256'], state_sha256=s['state_sha256'], broker_sha256=s['broker_sha256'], config_sha256=s['config_sha256'], supervisor=c['supervisor'], stage=c['stage'], backup_timer_prior_state='active', phase='maintenance-ready', timestamp=c['issued_at'], expires_at=c['expires_at'])
    hold_hash = producer.digest(producer.raw(hold))
    ready = dict(version=2, purpose=c['purpose'], operation_id=c['operation_id'], manifest_sha256=c['manifest_sha256'], state_sha256=s['state_sha256'], hold_sha256=hold_hash, service_source=s['source'], executable_sha256=s['binary_sha256'], held=True, services_started=False)
    binding = dict(observed_at=now, boot_id=c['boot_id'], uid=f['uid'], gid=f['gid'], maintenance_hold=hold, hold_sha256=hold_hash, state_sha256=s['state_sha256'], ready_sha256=producer.digest(producer.raw(ready)), config_sha256=s['config_sha256'], data_dir=f['data_dir'], account_cap=f['account_cap'], schema_sha256=f['schema_sha256'], lock_device=1, lock_inode=2)
    expected = a['expectations']
    authority = dict(expected, observed_at=datetime.datetime.fromtimestamp(now-1, datetime.timezone.utc).isoformat(), completed_at=datetime.datetime.fromtimestamp(now, datetime.timezone.utc).isoformat(), collection_seconds=1, metadata_and_tip_rechecked=True, regional_endpoints_pinned=True, phase='CLOSED', lease_present=False, prepared_present=False, unfinished_witness_present=False, concurrency=0, independent_tip_matches=True, revision=expected['minimum_revision'], request_ids=['synthetic-request'])
    return dict(runtime_path=c['stage']+'/runtime.py', runtime_sha256='f'*64, config=c), binding, authority, now


class FixtureProofTests(unittest.TestCase):
    def test_exact_v3_all_operations_reach_python_consumer(self):
        for operation in ('prepare', 'reissue', 'revoke', 'adopt-source'):
            pins, binding, authority, now = cohort()
            adoption = dict(old_source='9'*40, old_manifest_sha256='a'*64) if operation == 'adopt-source' else None
            proof, receipt = producer.assemble(pins, binding, authority, operation, now=now, nonce='b'*64, source_adoption=adoption)
            adapter = consumer.Adapter(clock=lambda: now)
            self.assertEqual(adapter.validate_proof(producer.raw(proof)), proof)
            self.assertNotIn('hold', proof)
            self.assertEqual(receipt['scope'], pins['config']['authority']['scope'])
            self.assertIn('raise SystemExit(status if status in (0,2,3,5,6) else 6)', producer.script(proof, receipt))

    def test_strict_cross_version_null_duplicate_and_changed_hold(self):
        pins, binding, authority, now = cohort()
        proof, _ = producer.assemble(pins, binding, authority, 'prepare', now=now, nonce='b'*64)
        for mutation in ('legacy_hold', 'null_hold', 'null_adoption', 'previous_alias', 'expiry', 'service_source'):
            changed = copy.deepcopy(proof)
            if mutation == 'legacy_hold': changed['hold'] = {}
            elif mutation == 'null_hold': changed['maintenance_hold'] = None
            elif mutation == 'null_adoption': changed['source_adoption'] = None
            elif mutation == 'previous_alias': changed['maintenance_hold']['previous_state_sha256'] = changed['state_sha256']
            elif mutation == 'expiry': changed['maintenance_hold']['expires_at'] = now
            else: changed['maintenance_hold']['service_source'] = 'a'*40
            with self.subTest(mutation=mutation), self.assertRaises((RuntimeError, TypeError, ValueError)):
                consumer.Adapter(clock=lambda: now).validate_proof(producer.raw(changed))
        encoded = producer.raw(proof)
        with self.assertRaises(RuntimeError): consumer.Adapter(clock=lambda: now).validate_proof(encoded[:-1] + b',"version":3}')

    def test_ready_digest_and_emitted_script_are_checked(self):
        pins, binding, authority, now = cohort()
        proof, receipt = producer.assemble(pins, binding, authority, 'prepare', now=now, nonce='b'*64)
        script = producer.script(proof, receipt)
        body = script.split("<<'FIXTUREPY'\n", 1)[1].rsplit('\nFIXTUREPY', 1)[0]
        compile(body, '<inert fixture invocation>', 'exec')
        self.assertIn("cgroup.procs", body)
        self.assertIn("properties['MainPID']=='0'", body)
        binding['ready_sha256'] = '0'*64
        with self.assertRaises(ValueError): producer.assemble(pins, binding, authority, 'prepare', now=now)

    def test_parameterized_identity_and_changed_observations(self):
        for key in ('account', 'principal', 'region', 'function', 'table', 'metadata_sha256', 'tip_sha256'):
            pins, binding, authority, now = cohort(); authority[key] = 'different'
            with self.subTest(key=key), self.assertRaises(ValueError): producer.assemble(pins, binding, authority, 'prepare', now=now)
        pins, binding, authority, now = cohort(); pins['unexpected'] = True
        with self.assertRaises(ValueError): producer.assemble(pins, binding, authority, 'prepare', now=now)


if __name__ == '__main__': unittest.main()
