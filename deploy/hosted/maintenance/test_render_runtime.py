import ast
import base64
import contextlib
import copy
import importlib.util
import io
import json
import os
from pathlib import Path
import tempfile
import unittest

MODULE_PATH = Path(__file__).with_name('render_runtime.py')
spec = importlib.util.spec_from_file_location('maintenance_render_test_subject', MODULE_PATH)
render_module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(render_module)


def declaration():
    op = 'a' * 64
    pins = []
    for unit in render_module.UNITS:
        for name in ('/etc/systemd/system/' + unit,
                     '/etc/systemd/system/' + unit + '.d/90-serenity-release-guard.conf'):
            pins.append(dict(path=name, sha256='b' * 64))
    expected = dict(cli_path='/usr/local/bin/aws', cli_sha256='c' * 64,
                    account='123456789012', principal='arn:aws:iam::123456789012:user/synthetic',
                    region='us-west-2', stack='synthetic-authority', function='synthetic-function',
                    version='1', code_sha256=base64.b64encode(bytes(32)).decode(),
                    table='synthetic-table', namespace='synthetic-namespace',
                    table_arn='arn:aws:dynamodb:us-west-2:123456789012:table/synthetic-table',
                    lineage='synthetic-lineage', witness_bucket='synthetic-witness',
                    metadata_sha256='d' * 64, tip_sha256='e' * 64, minimum_revision=5)
    return dict(version=1, purpose='serenity.fixture-maintenance/v1', operation_id=op,
                issued_at=1800000000, expires_at=1800000300,
                boot_id='01234567-89ab-cdef-0123-456789abcdef',
                stage='/var/lib/serenity-release/maintenance-' + op,
                supervisor='serenity-release-' + op + '.service',
                service=dict(source='1' * 40,
                             binary_path='/usr/local/lib/serenity-releases/synthetic/serenity',
                             binary_sha256='2' * 64, state_sha256='3' * 64,
                             broker_sha256='4' * 64, config_sha256='5' * 64,
                             installed_sequence=8, highest_sequence=8,
                             release_manifest_sha256='6' * 64,
                             startup_runtime_path='/var/lib/serenity-release/synthetic/activation-runtime.py',
                             startup_runtime_sha256='7' * 64, unit_files=pins),
                fixture=dict(tool_source='8' * 40, tool_path='/usr/local/libexec/synthetic-tool',
                             tool_sha256='9' * 64, wrapper_path='/usr/local/libexec/synthetic-wrapper.py',
                             wrapper_sha256='a' * 64, uid=12345, gid=12345,
                             data_dir='/var/lib/serenity/hosted',
                             private_dir='/var/lib/serenity-episode-fixture/' + 'b' * 32,
                             account_cap=20, schema_sha256='c' * 64,
                             unit='serenity-episode-fixture-' + op + '.service'),
                authority=dict(generator_path='/usr/local/libexec/synthetic-observer.py',
                               generator_sha256='f' * 64,
                               scope='123456789012:us-west-2:synthetic-table:synthetic-namespace',
                               session_id='closed-meta-' + 'd' * 64,
                               concurrency_target='synthetic-function', restore_concurrency=1,
                               expectations=expected))


class RenderRuntimeTests(unittest.TestCase):
    def test_render_never_executes_template_and_binds_exact_declaration(self):
        template = b'CONFIG = None\nraise RuntimeError("must not execute")\n'
        value = declaration()
        before = copy.deepcopy(value)
        manifest, runtime = render_module.render(value, template, render_module.digest(template))
        self.assertEqual(value, before)
        self.assertEqual(manifest, render_module.canonical(value))
        tree = ast.parse(runtime)
        # Read the literal passed to json.loads; never execute even synthetic code.
        config = json.loads(tree.body[0].value.args[0].value)
        self.assertEqual(config, dict(value, manifest_sha256=render_module.digest(manifest)))
        self.assertIn(b'raise RuntimeError', runtime)

    def test_refuse_template_mismatch_or_noninert_config(self):
        templates = [b'CONFIG = {}\n', b'CONFIG = None\nCONFIG = {}\n',
                     b'def f():\n CONFIG = None\n', b'CONFIG=None\n',
                     b'CONFIG = None; print("surprise")\n',
                     b'CONFIG = None\nCONFIG |= {}\n']
        for template in templates:
            with self.subTest(template=template), self.assertRaises(render_module.Refused):
                render_module.render(declaration(), template, render_module.digest(template))
        with self.assertRaises(render_module.Refused):
            render_module.render(declaration(), b'CONFIG = None\n', '0' * 64)

    def test_strict_declaration_and_cross_binding(self):
        changes = {
            'bool-version': lambda v: v.update(version=True),
            'long-window': lambda v: v.update(expires_at=v['issued_at'] + 3601),
            'foreign-stage': lambda v: v.update(stage='/var/lib/serenity-release/foreign'),
            'root-service-uid': lambda v: v['fixture'].update(uid=0),
            'canonical-private-dir': lambda v: v['fixture'].update(private_dir=v['fixture']['data_dir'] + '/brains/one'),
            'extra-authority-field': lambda v: v['authority'].update(approved=True),
            'wrong-scope': lambda v: v['authority'].update(scope='foreign'),
            'wrong-metadata-session': lambda v: v['authority'].update(session_id='closed-meta-' + '0' * 64),
            'new-capacity': lambda v: v['authority'].update(restore_concurrency=2),
            'private-account-mismatch': lambda v: v['authority']['expectations'].update(account='999999999999'),
            'missing-unit-guard': lambda v: v['service']['unit_files'].pop(),
            'duplicate-unit-pin': lambda v: v['service']['unit_files'].append(v['service']['unit_files'][0]),
            'control-field-null': lambda v: v.update(service=None),
        }
        for name, change in changes.items():
            value = declaration()
            change(value)
            with self.subTest(name=name), self.assertRaises(render_module.Refused):
                render_module.validate_declaration(value)

    def test_canonical_decoder_refuses_duplicate_unknown_encoding(self):
        for data in (b'{"version":1,"version":1}', b'{"a": 1}', b'{"a":1}\n', b'null', b'{} {}'):
            with self.subTest(data=data):
                if data == b'null':
                    with self.assertRaises(render_module.Refused):
                        render_module.validate_declaration(render_module.decode(data))
                else:
                    with self.assertRaises(render_module.Refused):
                        render_module.decode(data)

    def test_cli_exclusive_private_output_has_no_execution_or_route_receipt(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve()
            root.chmod(0o700)
            manifest = root / 'input.json'
            manifest.write_bytes(render_module.canonical(declaration()))
            manifest.chmod(0o600)
            template = root / 'template.py'
            template.write_bytes(b'CONFIG = None\nraise RuntimeError("not executed")\n')
            template.chmod(0o600)
            output = root / 'packet'
            args = ['--manifest', str(manifest), '--template', str(template),
                    '--template-sha256', render_module.digest(template.read_bytes()),
                    '--output', str(output)]
            captured = io.StringIO()
            with contextlib.redirect_stdout(captured):
                self.assertEqual(render_module.main(args), 0)
            receipt = json.loads(captured.getvalue())
            self.assertFalse(receipt['executed'])
            self.assertNotIn(str(root), captured.getvalue())
            self.assertEqual(sorted(p.name for p in output.iterdir()), ['manifest.json', 'runtime.py'])
            self.assertEqual(output.stat().st_mode & 0o777, 0o700)
            for item in output.iterdir():
                self.assertEqual(item.stat().st_mode & 0o777, 0o600)
            with self.assertRaises(render_module.Refused):
                render_module.main(args)
            self.assertEqual((output / 'manifest.json').read_bytes(), manifest.read_bytes())


if __name__ == '__main__':
    unittest.main()
