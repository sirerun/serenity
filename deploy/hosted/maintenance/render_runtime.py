#!/usr/bin/env python3
"""Render an INERT same-release packet. Never imports or executes the template.

Root ownership, live authority and operational approval are deployment gates,
not claims supplied by this renderer. No private declaration is shipped here.
"""
import argparse
import ast
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat


class Refused(ValueError):
    def __init__(self):
        super().__init__('maintenance packet rendering refused; details withheld')


def require(ok):
    if not ok:
        raise Refused()


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def unique(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result)
        result[key] = value
    return result


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'),
                      ensure_ascii=True, allow_nan=False).encode()


def decode(raw):
    require(type(raw) is bytes and 0 < len(raw) <= 65536)
    try:
        value = json.loads(raw, object_pairs_hook=unique)
    except (ValueError, TypeError, UnicodeError):
        raise Refused() from None
    require(canonical(value) == raw)
    return value


def exact(value, keys):
    require(type(value) is dict and set(value) == set(keys.split()))


def text(value, maximum=256):
    require(type(value) is str and 0 < len(value) <= maximum
            and not re.search(r'[\x00-\x20\x7f]', value))


def sha(value):
    require(type(value) is str and re.fullmatch('[0-9a-f]{64}', value) is not None)


def source(value):
    require(type(value) is str and re.fullmatch('[0-9a-f]{40}', value) is not None)


def absolute(value):
    text(value, 512)
    require(value.startswith('/') and str(PurePosixPath(value)) == value
            and '..' not in PurePosixPath(value).parts)


def integer(value, minimum=1, maximum=(1 << 63) - 1):
    require(type(value) is int and minimum <= value <= maximum)


UNITS = ('serenity-hosted.service', 'serenity-broker.service',
         'serenity-backup.service', 'serenity-backup.timer')
EXPECTATIONS = ('cli_path cli_sha256 account principal region stack function '
                'version code_sha256 table namespace table_arn lineage '
                'witness_bucket metadata_sha256 tip_sha256 minimum_revision')


def validate_declaration(value):
    exact(value, 'version purpose operation_id issued_at expires_at boot_id '
                'stage supervisor service fixture authority')
    require(type(value['version']) is int and value['version'] == 1
            and value['purpose'] == 'serenity.fixture-maintenance/v1')
    op = value['operation_id']
    sha(op)
    integer(value['issued_at'])
    integer(value['expires_at'])
    require(0 < value['expires_at'] - value['issued_at'] <= 3600)
    require(type(value['boot_id']) is str and re.fullmatch(
        '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}',
        value['boot_id']) is not None)
    require(value['stage'] == '/var/lib/serenity-release/maintenance-' + op)
    require(value['supervisor'] == 'serenity-release-' + op + '.service')
    service = value['service']
    exact(service, 'source binary_path binary_sha256 state_sha256 broker_sha256 '
                   'config_sha256 installed_sequence highest_sequence '
                   'release_manifest_sha256 startup_runtime_path '
                   'startup_runtime_sha256 unit_files')
    source(service['source'])
    for key in ('binary_path', 'startup_runtime_path'):
        absolute(service[key])
    require(service['binary_path'].startswith('/usr/local/lib/serenity-releases/')
            and service['binary_path'].endswith('/serenity'))
    for key in ('binary_sha256', 'state_sha256', 'broker_sha256', 'config_sha256',
                'release_manifest_sha256', 'startup_runtime_sha256'):
        sha(service[key])
    integer(service['installed_sequence'])
    integer(service['highest_sequence'])
    require(service['installed_sequence'] <= service['highest_sequence'])
    files = service['unit_files']
    require(type(files) is list and 8 <= len(files) <= 32)
    seen = set()
    for item in files:
        exact(item, 'path sha256')
        absolute(item['path'])
        sha(item['sha256'])
        name = item['path']
        require(name not in seen)
        seen.add(name)
        fragment = any(name == base + unit for base in
                       ('/etc/systemd/system/', '/usr/lib/systemd/system/',
                        '/lib/systemd/system/') for unit in UNITS)
        dropin = any(name.startswith('/etc/systemd/system/' + unit + '.d/')
                     and name.endswith('.conf') and
                     PurePosixPath(name).parent == PurePosixPath(
                         '/etc/systemd/system/' + unit + '.d') for unit in UNITS)
        require(fragment or dropin)
    for unit in UNITS:
        require('/etc/systemd/system/' + unit +
                '.d/90-serenity-release-guard.conf' in seen)
        require(sum(base + unit in seen for base in
                    ('/etc/systemd/system/', '/usr/lib/systemd/system/',
                     '/lib/systemd/system/')) == 1)
    fixture = value['fixture']
    exact(fixture, 'tool_source tool_path tool_sha256 wrapper_path wrapper_sha256 '
                   'uid gid data_dir private_dir account_cap schema_sha256 unit')
    source(fixture['tool_source'])
    for key in ('tool_path', 'wrapper_path', 'data_dir', 'private_dir'):
        absolute(fixture[key])
    for key in ('tool_sha256', 'wrapper_sha256', 'schema_sha256'):
        sha(fixture[key])
    for key in ('uid', 'gid', 'account_cap'):
        integer(fixture[key], maximum=(1 << 31) - 1)
    data, private = fixture['data_dir'], fixture['private_dir']
    require(private != data and private != data.rstrip('/') + '/brains'
            and not private.startswith(data.rstrip('/') + '/brains/'))
    require(fixture['unit'] == 'serenity-episode-fixture-' + op + '.service')
    authority = value['authority']
    exact(authority, 'generator_path generator_sha256 scope session_id '
                     'concurrency_target restore_concurrency expectations')
    absolute(authority['generator_path'])
    sha(authority['generator_sha256'])
    for key in ('scope', 'session_id', 'concurrency_target'):
        text(authority[key])
    # This source cohort is the existing capacity-one authority arrangement.
    require(type(authority['restore_concurrency']) is int
            and authority['restore_concurrency'] == 1)
    expected = authority['expectations']
    exact(expected, EXPECTATIONS)
    absolute(expected['cli_path'])
    for key in ('cli_sha256', 'metadata_sha256', 'tip_sha256'):
        sha(expected[key])
    for key in ('principal', 'region', 'stack', 'function', 'table', 'namespace',
                'table_arn', 'lineage', 'witness_bucket'):
        text(expected[key])
    require(type(expected['account']) is str and
            re.fullmatch('[0-9]{12}', expected['account']) is not None)
    require(expected['principal'].startswith('arn:aws:') and
            ':' + expected['account'] + ':' in expected['principal'])
    require(re.fullmatch('[a-z]{2}-[a-z]+-[1-9]', expected['region']) is not None)
    require(type(expected['version']) is str and
            re.fullmatch('[1-9][0-9]{0,9}', expected['version']) is not None)
    require(type(expected['code_sha256']) is str)
    import base64
    try:
        code = base64.b64decode(expected['code_sha256'], validate=True)
    except (ValueError, TypeError):
        raise Refused() from None
    require(len(code) == 32 and base64.b64encode(code).decode() == expected['code_sha256'])
    integer(expected['minimum_revision'], minimum=0)
    require(expected['table_arn'] == 'arn:aws:dynamodb:' + expected['region'] +
            ':' + expected['account'] + ':table/' + expected['table'])
    require(authority['scope'] == ':'.join(expected[key] for key in
                                         ('account', 'region', 'table', 'namespace')))
    require(authority['session_id'] == 'closed-meta-' + expected['metadata_sha256'])
    require(authority['concurrency_target'] == expected['function'])
    return value


def render(declaration, template, template_sha256):
    validate_declaration(declaration)
    sha(template_sha256)
    require(type(template) is bytes and 0 < len(template) <= 262144
            and digest(template) == template_sha256)
    try:
        code = template.decode('utf-8')
        tree = ast.parse(code)
    except (ValueError, SyntaxError, UnicodeError):
        raise Refused() from None
    assignments = [node for node in ast.walk(tree) if
                   isinstance(node, (ast.Assign, ast.AnnAssign, ast.NamedExpr, ast.AugAssign))
                   and any(isinstance(child, ast.Name) and child.id == 'CONFIG'
                           and isinstance(child.ctx, ast.Store)
                           for child in ast.walk(node))]
    require(len(assignments) == 1)
    assignment = assignments[0]
    require(assignment in tree.body and isinstance(assignment, ast.Assign)
            and len(assignment.targets) == 1 and
            isinstance(assignment.targets[0], ast.Name) and
            assignment.targets[0].id == 'CONFIG' and
            isinstance(assignment.value, ast.Constant) and assignment.value.value is None)
    lines = code.splitlines(keepends=True)
    require(assignment.lineno == assignment.end_lineno)
    original = lines[assignment.lineno - 1]
    require(re.fullmatch(r'CONFIG = None(?:[ \t]*#[^\r\n]*)?\r?\n?', original) is not None)
    declaration_bytes = canonical(declaration)
    config = dict(declaration, manifest_sha256=digest(declaration_bytes))
    lines[assignment.lineno - 1] = 'CONFIG = __import__("json").loads(' + repr(canonical(config).decode()) + ')\n'
    rendered = ''.join(lines).encode()
    require(len(rendered) <= 393216)
    return declaration_bytes, rendered


def read_owned(path, maximum, private):
    path = Path(path)
    require(path.is_absolute() and not path.is_symlink())
    before = path.lstat()
    require(stat.S_ISREG(before.st_mode) and before.st_uid == os.getuid()
            and before.st_nlink == 1 and not before.st_mode & (0o077 if private else 0o022)
            and 0 < before.st_size <= maximum)
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        opened = os.fstat(fd)
        require((opened.st_dev, opened.st_ino) == (before.st_dev, before.st_ino))
        chunks, size = [], 0
        while size <= maximum:
            block = os.read(fd, min(65536, maximum + 1 - size))
            if not block:
                break
            chunks.append(block)
            size += len(block)
        after = os.fstat(fd)
        require(size == before.st_size and size <= maximum and
                (after.st_dev, after.st_ino, after.st_mode, after.st_nlink,
                 after.st_size, after.st_mtime_ns, after.st_ctime_ns) ==
                (before.st_dev, before.st_ino, before.st_mode, before.st_nlink,
                 before.st_size, before.st_mtime_ns, before.st_ctime_ns))
        return b''.join(chunks)
    finally:
        os.close(fd)


def write_exclusive(directory, name, data):
    fd = os.open(directory / name, os.O_WRONLY | os.O_CREAT | os.O_EXCL |
                 os.O_NOFOLLOW | os.O_CLOEXEC, 0o600)
    try:
        os.fchmod(fd, 0o600)
        view = memoryview(data)
        while view:
            count = os.write(fd, view)
            require(count > 0)
            view = view[count:]
        os.fsync(fd)
    finally:
        os.close(fd)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--template', type=Path, required=True)
    parser.add_argument('--template-sha256', required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args(argv)
    declaration = decode(read_owned(args.manifest, 65536, True))
    template = read_owned(args.template, 262144, False)
    manifest_bytes, runtime_bytes = render(declaration, template, args.template_sha256)
    output = args.output
    require(output.is_absolute() and str(output) == str(output.resolve(strict=False))
            and not output.exists())
    parent = output.parent.stat()
    require(stat.S_ISDIR(parent.st_mode) and parent.st_uid == os.getuid()
            and stat.S_IMODE(parent.st_mode) == 0o700)
    os.mkdir(output, 0o700)
    os.chmod(output, 0o700)
    write_exclusive(output, 'manifest.json', manifest_bytes)
    write_exclusive(output, 'runtime.py', runtime_bytes)
    for directory in (output, output.parent):
        fd = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)
    print(json.dumps(dict(rendered=True, executed=False, residual_copies=True,
                          manifest_sha256=digest(manifest_bytes),
                          runtime_sha256=digest(runtime_bytes)), sort_keys=True))
    return 0


if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except (Refused, OSError, ValueError):
        raise SystemExit('maintenance packet rendering refused; retained outputs may require reconciliation') from None
