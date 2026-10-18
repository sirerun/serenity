"""Synthetic authenticated CLI transport responses; no AWS calls or credentials."""
import base64
import copy
import datetime
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import unittest

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('maintenance_observer_tested', HERE / 'observe_closed.py')
a = importlib.util.module_from_spec(spec)
spec.loader.exec_module(a)


def encoded(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':')).encode()


class Provider:
    def __init__(self, drift=None, capacity=0, phase='CLOSED'):
        self.drift, self.capacity, self.phase = drift, capacity, phase
        self.counts = {}
        self.calls = []
        tip = dict(table_arn='arn:aws:dynamodb:us-east-1:000000000000:table/synthetic', lineage='synthetic')
        state = dict(phase=phase)
        if phase == 'ACTIVE':
            state.update(epoch=1, position={'ActiveGeneration': 1}, measurement={'executable_sha256': 'b'*64})
            state['lease'] = dict(id='1'*64, reservation_id='2'*64, writer_id='3'*64, epoch=1,
                position=state['position'], build_sha='a'*40,
                expires_at=(datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(minutes=5)).isoformat())
        self.record = dict(revision=1, state=state, tip=tip)
        self.tip = encoded(tip)
        self.metadata = encoded(self.record)
        cli = Path('/usr/bin/true')
        self.e = dict(cli_path=str(cli), cli_sha256=hashlib.sha256(cli.read_bytes()).hexdigest(),
            account='000000000000', principal='arn:aws:iam::000000000000:user/synthetic', region='us-east-1',
            stack='synthetic', function='synthetic', version='1', code_sha256=base64.b64encode(b'x'*32).decode(),
            table='synthetic', namespace='synthetic', table_arn=tip['table_arn'], lineage='synthetic',
            witness_bucket='synthetic', metadata_sha256=hashlib.sha256(self.metadata).hexdigest(),
            tip_sha256=hashlib.sha256(self.tip).hexdigest(), minimum_revision=1)

    def run(self, argv, *, timeout, pass_fds=()):
        self.calls.append(argv)
        self.assert_bounds(timeout)
        args = argv[5:-4]  # --region/--endpoint then command; ending output/debug/pager
        key = tuple(args[:2])
        self.counts[key] = self.counts.get(key, 0) + 1
        e = self.e
        head = dict(ContentLength=len(self.tip), ETag='synthetic-etag', VersionId='synthetic-version')
        if key == ('sts', 'get-caller-identity'):
            out = dict(Account=e['account'], Arn=e['principal'])
            if self.drift == 'principal':
                out['Arn'] = 'foreign'
        elif key == ('cloudformation', 'describe-stacks'):
            out = dict(Stacks=[dict(StackStatus='UPDATE_COMPLETE', Outputs=[dict(OutputKey='GatewayVersionARN',
                OutputValue='arn:aws:lambda:us-east-1:000000000000:function:synthetic:1')])])
        elif key == ('lambda', 'get-function-configuration'):
            out = dict(CodeSha256=e['code_sha256'], State='Active', Version='1', Environment=dict(Variables={
                'AUTHORITY_TABLE': e['table'], 'AUTHORITY_NAMESPACE': e['namespace'], 'AUTHORITY_TABLE_ARN': e['table_arn'],
                'AUTHORITY_LINEAGE': e['lineage'], 'WITNESS_BUCKET': e['witness_bucket'],
                'APPROVED_BUILDS': json.dumps({'b'*64: 'a'*40})}))
            if self.drift == 'code' and self.counts[key] > 1:
                out['CodeSha256'] = 'foreign'
            if self.drift == 'namespace':
                out['Environment']['Variables']['AUTHORITY_NAMESPACE'] = 'foreign'
        elif key == ('lambda', 'get-function-concurrency'):
            out = dict(ReservedConcurrentExecutions=self.capacity + (1 if self.drift == 'capacity' and self.counts[key] > 1 else 0))
        elif key == ('dynamodb', 'get-item'):
            raw = self.metadata + (b' ' if self.drift == 'metadata' and self.counts[key] > 1 else b'')
            out = dict(Item={'payload': {'B': base64.b64encode(raw).decode()}})
        elif key == ('s3api', 'head-object'):
            out = head
            if self.drift == 'tip' and self.counts[key] > 1:
                out['VersionId'] = 'changed'
        elif key == ('s3api', 'get-object'):
            assert len(pass_fds) == 1
            os.write(pass_fds[0], self.tip)
            out = head
        else:
            raise AssertionError((argv, key))
        return subprocess.CompletedProcess(argv, 0, json.dumps(out),
            "DEBUG Response headers: {'x-amzn-requestid': 'synthetic-request-1'}\nAUTHORIZATION-SECRET-NOT-RETAINED")

    @staticmethod
    def assert_bounds(timeout):
        assert 0 < timeout <= 40


class AuthorityTests(unittest.TestCase):
    def test_unknown_fields_and_duplicate_json_refuse(self):
        p = Provider()
        with self.assertRaises(RuntimeError):
            a.observe({**p.e, 'permit': True}, p)
        with self.assertRaises(RuntimeError):
            a.decode('{"a":1,"a":2}')

    @unittest.skipUnless(hasattr(os, 'memfd_create'), 'Linux anonymous provider output fd')
    def test_closed_observes_real_transport_and_rechecks(self):
        p = Provider()
        out = a.observe(p.e, p)
        self.assertEqual(p.counts[('dynamodb', 'get-item')], 2)
        self.assertEqual(p.counts[('s3api', 'head-object')], 2)
        self.assertEqual(p.counts[('lambda', 'get-function-configuration')], 2)
        self.assertTrue(out['metadata_and_tip_rechecked'])
        self.assertNotIn('SECRET', json.dumps(out))
        self.assertFalse(out['raw_metadata_retained'])
        self.assertTrue(all('--endpoint-url' in argv for argv in p.calls))

    @unittest.skipUnless(hasattr(os, 'memfd_create'), 'Linux anonymous provider output fd')
    def test_identity_and_final_boundary_drifts_refuse(self):
        for drift in ('principal', 'namespace', 'metadata', 'tip', 'capacity', 'code'):
            with self.subTest(drift=drift), self.assertRaises(RuntimeError):
                p = Provider(drift=drift)
                a.observe(p.e, p)

    @unittest.skipUnless(hasattr(os, 'memfd_create'), 'Linux anonymous provider output fd')
    def test_restored_active_allows_new_metadata_and_normal_lease(self):
        p = Provider(capacity=1, phase='ACTIVE')
        p.e['metadata_sha256'] = 'a' * 64
        p.e['tip_sha256'] = 'b' * 64
        binding = dict(operation_id='1'*64, manifest_sha256='2'*64, hold_sha256='3'*64,
            state_sha256='4'*64, boot_id='00000000-0000-0000-0000-000000000001', scope='synthetic',
            session_id='synthetic', concurrency_target='synthetic', generator_path='/usr/bin/observer', generator_sha256='5'*64)
        out = a.observe_restored(p.e, binding, 1, p)
        self.assertEqual(out['phase'], 'ACTIVE')
        self.assertEqual(out['concurrency'], 1)
        self.assertNotEqual(out['metadata_sha256'], p.e['metadata_sha256'])
        self.assertEqual(out['hold_sha256'], binding['hold_sha256'])

    def test_bounded_runner_refuses_excess_and_does_not_persist_diagnostics(self):
        runner = a.Runner()
        with self.assertRaises(RuntimeError):
            runner.run([sys.executable, '-I', '-c', "import sys;sys.stdout.write('x'*1100000)"], timeout=2)
        with self.assertRaises(RuntimeError):
            runner.run([sys.executable, '-I', '-c', 'import time;time.sleep(2)'], timeout=.05)


if __name__ == '__main__':
    unittest.main()
