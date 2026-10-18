#!/usr/bin/env python3
"""Bounded authenticated read-only authority observation; no saved diagnostics.

Adapted from the generic META/TIP recheck mechanism recorded in provenance.json.
Injected runners are an import-only synthetic test seam, never CLI authority.
"""
import ast
import base64
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import selectors
import stat
import subprocess
import time

FIELDS = frozenset('cli_path cli_sha256 account principal region stack function version code_sha256 table namespace table_arn lineage witness_bucket metadata_sha256 tip_sha256 minimum_revision'.split())
HEX = re.compile(r'[0-9a-f]{64}\Z')


def require(value):
    if not value:
        raise RuntimeError('authority observation refused')


def decode(raw):
    def pairs(rows):
        out = {}
        for key, value in rows:
            require(key not in out)
            out[key] = value
        return out
    return json.loads(raw, object_pairs_hook=pairs,
                      parse_constant=lambda _: require(False))


def expectations(value):
    require(isinstance(value, dict) and set(value) == FIELDS)
    out = dict(value)
    for key in FIELDS - {'minimum_revision'}:
        require(isinstance(out[key], str) and 0 < len(out[key]) <= 2048 and
                not any(ord(c) < 32 for c in out[key]))
    for key in ('cli_sha256', 'metadata_sha256', 'tip_sha256'):
        require(HEX.fullmatch(out[key]))
    require(re.fullmatch(r'[0-9]{12}', out['account']) and
            re.fullmatch(r'[a-z]{2}(?:-[a-z]+)+-[0-9]+', out['region']) and
            re.fullmatch(r'[1-9][0-9]*', out['version']))
    require(Path(out['cli_path']).is_absolute() and
            type(out['minimum_revision']) is int and out['minimum_revision'] >= 0)
    require(len(base64.b64decode(out['code_sha256'], validate=True)) == 32)
    require(out['table_arn'] == 'arn:aws:dynamodb:' + out['region'] + ':' +
            out['account'] + ':table/' + out['table'])
    return out


class Runner:
    """Pipe reads are capped before decoding; diagnostics never touch disk."""
    def run(self, argv, *, timeout, pass_fds=()):
        process = subprocess.Popen(argv, stdin=subprocess.DEVNULL,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                   pass_fds=pass_fds, env={**os.environ,
                                   'AWS_PAGER': '', 'AWS_CLI_AUTO_PROMPT': 'off'})
        deadline = time.monotonic() + timeout
        buffers = {process.stdout: bytearray(), process.stderr: bytearray()}
        try:
            with selectors.DefaultSelector() as selector:
                for stream in buffers:
                    os.set_blocking(stream.fileno(), False)
                    selector.register(stream, selectors.EVENT_READ)
                while selector.get_map():
                    remaining = deadline - time.monotonic()
                    require(remaining > 0)
                    for key, _ in selector.select(remaining):
                        data = os.read(key.fileobj.fileno(), 65536)
                        if not data:
                            selector.unregister(key.fileobj)
                        else:
                            buffers[key.fileobj].extend(data)
                            require(len(buffers[key.fileobj]) <= 1024 * 1024)
                code = process.wait(timeout=max(0.001, deadline - time.monotonic()))
            require(code == 0)
            return subprocess.CompletedProcess(argv, code,
                bytes(buffers[process.stdout]).decode('utf-8'),
                bytes(buffers[process.stderr]).decode('utf-8'))
        finally:
            if process.poll() is None:
                process.kill()
            process.wait()
            process.stdout.close()
            process.stderr.close()


class Observation:
    def __init__(self, expected, runner):
        self.e = expectations(expected)
        self.runner = runner or Runner()
        self.started = datetime.datetime.now(datetime.timezone.utc)
        self.start = time.monotonic()
        self.deadline = self.start + 45
        self.ids = []

    def cli_pin(self):
        path = Path(self.e['cli_path'])
        # The exact official executable may live behind an owned symlink, but its
        # resolved bytes and every parent must be non-writable by other users.
        for part in (path, *path.parents):
            st = part.lstat()
            require(st.st_uid == 0 and (stat.S_ISLNK(st.st_mode) or not st.st_mode & 0o022))
        resolved = path.resolve(strict=True)
        for part in (resolved, *resolved.parents):
            st = part.lstat()
            require(st.st_uid == 0 and not st.st_mode & 0o022)
        require(resolved.is_file() and resolved.stat().st_size <= 64 * 1024 * 1024)
        require(hashlib.sha256(resolved.read_bytes()).hexdigest() == self.e['cli_sha256'])

    def aws(self, *args, pass_fds=()):
        self.cli_pin()
        remaining = self.deadline - time.monotonic()
        require(remaining > 0)
        service = {'s3api': 's3'}.get(args[0], args[0])
        require(service in ('sts', 'cloudformation', 'lambda', 'dynamodb', 's3'))
        argv = [self.e['cli_path'], '--region', self.e['region'], '--endpoint-url',
                'https://' + service + '.' + self.e['region'] + '.amazonaws.com',
                *args, '--output', 'json', '--debug', '--no-cli-pager']
        result = self.runner.run(argv, timeout=min(40, remaining), pass_fds=pass_fds)
        require(result.returncode == 0 and isinstance(result.stdout, str) and
                isinstance(result.stderr, str) and len(result.stdout) <= 1024 * 1024 and
                len(result.stderr) <= 1024 * 1024)
        ids = []
        for line in result.stderr.splitlines():
            if 'Response headers: ' not in line:
                continue
            try:
                headers = ast.literal_eval(line.split('Response headers: ', 1)[1])
            except (ValueError, SyntaxError):
                continue
            if isinstance(headers, dict):
                for key, value in headers.items():
                    if isinstance(key, str) and key.lower() in (
                            'x-amzn-requestid', 'x-amz-request-id', 'x-amzn-request-id'):
                        require(isinstance(value, str) and
                                re.fullmatch(r'[A-Za-z0-9+/=_:.\-]{1,256}', value))
                        ids.append(value)
        require(ids)
        self.ids.append(ids[-1])
        return decode(result.stdout)

    def configuration(self):
        e = self.e
        identity = self.aws('sts', 'get-caller-identity')
        require(identity.get('Account') == e['account'] and identity.get('Arn') == e['principal'])
        stacks = self.aws('cloudformation', 'describe-stacks', '--stack-name', e['stack'])['Stacks']
        require(len(stacks) == 1 and stacks[0]['StackStatus'] in ('CREATE_COMPLETE', 'UPDATE_COMPLETE'))
        outputs = stacks[0]['Outputs']
        require(len({x['OutputKey'] for x in outputs}) == len(outputs))
        outputs = {x['OutputKey']: x['OutputValue'] for x in outputs}
        require(outputs['GatewayVersionARN'] == 'arn:aws:lambda:' + e['region'] + ':' +
                e['account'] + ':function:' + e['function'] + ':' + e['version'])
        config = self.aws('lambda', 'get-function-configuration', '--function-name',
                          e['function'], '--qualifier', e['version'])
        require(config['CodeSha256'] == e['code_sha256'] and config['State'] == 'Active' and
                config['Version'] == e['version'] and
                config.get('LastUpdateStatus', 'Successful') == 'Successful')
        env = config['Environment']['Variables']
        for key, name in [('AUTHORITY_TABLE', 'table'), ('AUTHORITY_NAMESPACE', 'namespace'),
                          ('AUTHORITY_TABLE_ARN', 'table_arn'), ('AUTHORITY_LINEAGE', 'lineage'),
                          ('WITNESS_BUCKET', 'witness_bucket')]:
            require(env[key] == e[name])
        return config

    def concurrency(self, expected):
        require(self.aws('lambda', 'get-function-concurrency', '--function-name',
                         self.e['function'])['ReservedConcurrentExecutions'] == expected)

    def metadata(self):
        key = json.dumps({'pk': {'S': self.e['namespace']}, 'sk': {'S': 'META'}}, separators=(',', ':'))
        value = self.aws('dynamodb', 'get-item', '--table-name', self.e['table'], '--key', key,
                         '--consistent-read', '--projection-expression', '#p',
                         '--expression-attribute-names', '{"#p":"payload"}')
        raw = base64.b64decode(value['Item']['payload']['B'], validate=True)
        require(0 < len(raw) <= 350 * 1024)
        return raw

    def tip(self):
        e = self.e
        head = self.aws('s3api', 'head-object', '--bucket', e['witness_bucket'], '--key', 'journal/TIP')
        require(type(head['ContentLength']) is int and 0 < head['ContentLength'] <= 4096 and
                not head.get('DeleteMarker', False) and isinstance(head['ETag'], str))
        # Anonymous RAM-backed Linux fd: no raw authority record or diagnostic is
        # ever persisted in a named temporary file. Non-Linux hosts fail closed.
        require(hasattr(os, 'memfd_create'))
        fd = os.memfd_create('serenity-authority-tip', os.MFD_CLOEXEC)
        try:
            got = self.aws('s3api', 'get-object', '--bucket', e['witness_bucket'], '--key',
                           'journal/TIP', '--if-match', head['ETag'], '/proc/self/fd/' + str(fd),
                           pass_fds=(fd,))
            require(all(got.get(k) == head.get(k) for k in ('ContentLength', 'ETag', 'VersionId')))
            require(os.fstat(fd).st_size == head['ContentLength'])
            raw = os.pread(fd, 4097, 0)
            return raw, head
        finally:
            os.close(fd)

    def collect(self, capacity, restored=False):
        require(type(capacity) is int and 0 <= capacity <= 1)
        config = self.configuration()
        self.concurrency(capacity)
        raw = self.metadata()
        record = decode(raw)
        state = record['state']
        require(type(record['revision']) is int and record['revision'] >= self.e['minimum_revision'])
        require(state['phase'] in (('CLOSED', 'ACTIVE') if restored else ('CLOSED',)) and
                state.get('prepared') is None and record.get('pending') is None)
        if not restored or state['phase'] == 'CLOSED':
            require(state.get('lease') is None)
        else:
            lease = state.get('lease')
            require(isinstance(lease, dict) and set(lease) == set(
                'id reservation_id writer_id epoch position expires_at build_sha'.split()))
            require(all(isinstance(lease[k], str) and HEX.fullmatch(lease[k]) for k in
                        ('id', 'reservation_id', 'writer_id')) and
                    isinstance(lease['build_sha'], str) and re.fullmatch(r'[0-9a-f]{40}', lease['build_sha']) and
                    type(lease['epoch']) is int and lease['epoch'] > 0 and lease['epoch'] == state.get('epoch') and
                    isinstance(lease['position'], dict) and bool(lease['position']) and
                    isinstance(state.get('position'), dict) and bool(state['position']))
            measurement = state.get('measurement')
            require(isinstance(measurement, dict) and isinstance(measurement.get('executable_sha256'), str) and
                    HEX.fullmatch(measurement['executable_sha256']))
            builds = decode(config['Environment']['Variables']['APPROVED_BUILDS'])
            require(isinstance(builds, dict) and builds.get(measurement['executable_sha256']) == lease['build_sha'])
            expires = datetime.datetime.fromisoformat(lease['expires_at'].replace('Z', '+00:00'))
            require(expires.tzinfo is not None and expires > datetime.datetime.now(datetime.timezone.utc))
        tip_raw, head = self.tip()
        tip = decode(tip_raw)
        require(tip == record['tip'] and tip['table_arn'] == self.e['table_arn'] and
                tip['lineage'] == self.e['lineage'])
        metadata_hash = hashlib.sha256(raw).hexdigest()
        tip_hash = hashlib.sha256(tip_raw).hexdigest()
        if not restored:
            require(metadata_hash == self.e['metadata_sha256'] and tip_hash == self.e['tip_sha256'])
        require(self.metadata() == raw)
        final = self.aws('s3api', 'head-object', '--bucket', self.e['witness_bucket'], '--key', 'journal/TIP')
        require(not final.get('DeleteMarker', False) and
                all(final.get(k) == head.get(k) for k in ('ContentLength', 'ETag', 'VersionId')))
        self.concurrency(capacity)
        require(self.configuration() == config)
        duration = time.monotonic() - self.start
        require(0 <= duration <= 45)
        return dict(observed_at=self.started.isoformat(),
                    completed_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),
                    collection_seconds=duration, cli_path=self.e['cli_path'],
                    cli_sha256=self.e['cli_sha256'], regional_endpoints_pinned=True,
                    **{k: self.e[k] for k in ('account', 'principal', 'region', 'function',
                                            'version', 'code_sha256', 'table', 'namespace')},
                    revision=record['revision'], phase=state['phase'],
                    lease_present=state.get('lease') is not None, prepared_present=False,
                    unfinished_witness_present=False, concurrency=capacity,
                    independent_tip_matches=True, tip_sha256=tip_hash, metadata_sha256=metadata_hash,
                    request_ids=self.ids[-8:], preflight_request_ids=self.ids[:-8],
                    metadata_and_tip_rechecked=True, diagnostic_output_retained=False,
                    raw_metadata_retained=False)


def observe(expected, runner=None):
    try:
        return Observation(expected, runner).collect(0)
    except Exception:
        raise RuntimeError('authority observation refused') from None


def observe_restored(expected, binding, restore_concurrency, runner=None):
    try:
        require(isinstance(binding, dict) and set(binding) == set(
            'operation_id manifest_sha256 hold_sha256 state_sha256 boot_id scope session_id concurrency_target generator_path generator_sha256'.split()))
        require(type(restore_concurrency) is int and restore_concurrency == 1)
        require(isinstance(binding['boot_id'], str) and re.fullmatch(
            r'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}', binding['boot_id']))
        for key in ('scope', 'session_id', 'concurrency_target'):
            require(isinstance(binding[key], str) and 0 < len(binding[key]) <= 256 and
                    not any(ord(c) < 33 for c in binding[key]))
        require(isinstance(binding['generator_path'], str) and Path(binding['generator_path']).is_absolute())
        if runner is None:
            generator = Path(binding['generator_path']).resolve(strict=True)
            require(generator == Path(__file__).resolve() and
                    hashlib.sha256(generator.read_bytes()).hexdigest() == binding['generator_sha256'])
        for key in ('operation_id', 'manifest_sha256', 'hold_sha256', 'state_sha256', 'generator_sha256'):
            require(isinstance(binding[key], str) and HEX.fullmatch(binding[key]))
        result = Observation(expected, runner).collect(restore_concurrency, restored=True)
        return dict(version=1, purpose='serenity.fixture-authority-restored/v1', **binding,
                    observed_at=int(datetime.datetime.fromisoformat(result['observed_at']).timestamp()),
                    concurrency=result['concurrency'], phase=result['phase'], revision=result['revision'],
                    metadata_sha256=result['metadata_sha256'], tip_sha256=result['tip_sha256'],
                    request_ids=result['preflight_request_ids'] + result['request_ids'])
    except Exception:
        raise RuntimeError('authority observation refused') from None
