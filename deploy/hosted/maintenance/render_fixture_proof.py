#!/usr/bin/env python3
"""Render INERT v3 fixture proofs from pinned config and fresh observations.
No network, credential access, operational approval or execution is performed.
"""
import argparse
import base64
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import time


def require(ok):
    if not ok:
        raise ValueError('fixture proof binding refused; details withheld')


def raw(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=True, allow_nan=False).encode()


def digest(data):
    return hashlib.sha256(data).hexdigest()


def unique(pairs):
    value = {}
    for key, item in pairs:
        require(key not in value); value[key] = item
    return value


def decode(data):
    return json.loads(data, object_pairs_hook=unique)


def validate_pins(pins):
    require(type(pins) is dict and set(pins) == {'runtime_path', 'runtime_sha256', 'config'})
    c = pins['config']
    require(type(c) is dict and set(c) == set('version purpose operation_id issued_at expires_at boot_id stage supervisor service fixture authority manifest_sha256'.split()))
    require(type(c['version']) is int and c['version'] == 1 and c['purpose'] == 'serenity.fixture-maintenance/v1')
    require(type(c['operation_id']) is str and re.fullmatch('[0-9a-f]{64}', c['operation_id']))
    require(c['stage'] == '/var/lib/serenity-release/maintenance-' + c['operation_id'])
    require(pins['runtime_path'] == c['stage'] + '/runtime.py')
    require(c['supervisor'] == 'serenity-release-' + c['operation_id'] + '.service')
    require(type(c['issued_at']) is int and type(c['expires_at']) is int and 0 < c['expires_at'] - c['issued_at'] <= 3600)
    require(c['manifest_sha256'] == digest(raw({k: v for k, v in c.items() if k != 'manifest_sha256'})))
    require(type(pins['runtime_sha256']) is str and re.fullmatch('[0-9a-f]{64}', pins['runtime_sha256']))
    require(type(c['service']) is dict and set(c['service']) == set('source binary_path binary_sha256 state_sha256 broker_sha256 config_sha256 installed_sequence highest_sequence release_manifest_sha256 startup_runtime_path startup_runtime_sha256 unit_files'.split()))
    require(type(c['fixture']) is dict and set(c['fixture']) == set('tool_source tool_path tool_sha256 wrapper_path wrapper_sha256 uid gid data_dir private_dir account_cap schema_sha256 unit'.split()))
    require(type(c['authority']) is dict and set(c['authority']) == set('generator_path generator_sha256 scope session_id concurrency_target restore_concurrency expectations'.split()))
    require(type(c['authority']['expectations']) is dict and set(c['authority']['expectations']) == set('cli_path cli_sha256 account principal region stack function version code_sha256 table namespace table_arn lineage witness_bucket metadata_sha256 tip_sha256 minimum_revision'.split()))
    return c


def timestamp(value):
    require(type(value) is str)
    parsed = datetime.datetime.fromisoformat(value)
    require(parsed.tzinfo is not None)
    return int(parsed.timestamp())


def assemble(pins, binding, authority, operation, now=None, nonce=None, source_adoption=None):
    c = validate_pins(pins); service, fixture, auth = c['service'], c['fixture'], c['authority']
    now = int(time.time()) if now is None else now
    require(type(now) is int and c['issued_at'] <= now < c['expires_at'])
    require(operation in ('prepare', 'reissue', 'revoke', 'adopt-source'))
    require(type(binding) is dict and type(binding.get('maintenance_hold')) is dict)
    h = binding['maintenance_hold']
    require(set(h) == set('version purpose operation_id manifest_sha256 service_source executable_sha256 state_sha256 broker_sha256 config_sha256 supervisor stage backup_timer_prior_state phase timestamp expires_at'.split()))
    for key in ('version', 'purpose', 'operation_id', 'manifest_sha256', 'supervisor', 'stage', 'expires_at'):
        require(h[key] == c[key])
    require(h['phase'] == 'maintenance-ready' and h['backup_timer_prior_state'] == 'active')
    require(h['service_source'] == service['source'] and h['executable_sha256'] == service['binary_sha256'])
    require(h['state_sha256'] == service['state_sha256'] and h['config_sha256'] == service['config_sha256'] and h['broker_sha256'] == service['broker_sha256'])
    require(type(binding['observed_at']) is int and 0 <= now - binding['observed_at'] <= 120)
    require(binding['boot_id'] == c['boot_id'] and binding['data_dir'] == fixture['data_dir'])
    for key in ('uid', 'gid', 'account_cap', 'schema_sha256'):
        require(binding[key] == fixture[key])
    require(binding['hold_sha256'] == digest(raw(h)) and binding['state_sha256'] == service['state_sha256'] and binding['config_sha256'] == service['config_sha256'])
    ready = dict(version=2, purpose=c['purpose'], operation_id=c['operation_id'], manifest_sha256=c['manifest_sha256'], state_sha256=service['state_sha256'], hold_sha256=binding['hold_sha256'], service_source=service['source'], executable_sha256=service['binary_sha256'], held=True, services_started=False)
    require(binding['ready_sha256'] == digest(raw(ready)))
    observed, completed = timestamp(authority['observed_at']), timestamp(authority['completed_at'])
    require(0 <= now - observed <= 60 and observed <= completed <= now)
    require(0 <= authority['collection_seconds'] <= 45 and authority['metadata_and_tip_rechecked'] is True and authority['regional_endpoints_pinned'] is True)
    expected = auth['expectations']
    for key in ('cli_path', 'cli_sha256', 'account', 'principal', 'region', 'function', 'version', 'code_sha256', 'table', 'namespace', 'metadata_sha256', 'tip_sha256'):
        require(authority[key] == expected[key])
    require(authority['phase'] == 'CLOSED' and authority['lease_present'] is False and authority['prepared_present'] is False and authority['unfinished_witness_present'] is False and authority['concurrency'] == 0 and authority['independent_tip_matches'] is True)
    require(type(authority['revision']) is int and authority['revision'] >= expected['minimum_revision'])
    require(type(authority['request_ids']) is list and 1 <= len(authority['request_ids']) <= 8 and all(type(v) is str and 0 < len(v) <= 256 for v in authority['request_ids']))
    nonce = secrets.token_hex(32) if nonce is None else nonce
    require(type(nonce) is str and re.fullmatch('[0-9a-f]{64}', nonce))
    receipt = dict(version=1, observed_at=observed, status='CLOSED', active_leases=0, concurrency=0, scope=auth['scope'], session_id=auth['session_id'], concurrency_target=auth['concurrency_target'], request_ids=authority['request_ids'], **{k: binding[k] for k in ('boot_id', 'hold_sha256', 'state_sha256')})
    proof = dict(version=3, purpose='serenity-episode-fixture-offline-v3', operation=operation, nonce=nonce, issued_at=now, expires_at=min(now + 240, c['expires_at']), maintenance_hold=h, authority_scope=auth['scope'], authority_session_id=auth['session_id'], concurrency_target=auth['concurrency_target'], authority_receipt_path=c['stage'] + '/fixture-authority-' + nonce + '.json', authority_receipt_sha256=digest(raw(receipt)))
    for key in ('boot_id', 'uid', 'gid', 'hold_sha256', 'state_sha256', 'ready_sha256', 'config_sha256', 'data_dir', 'account_cap', 'schema_sha256', 'lock_device', 'lock_inode'):
        proof[key] = binding[key]
    for key in ('tool_source', 'tool_path', 'tool_sha256', 'wrapper_path', 'wrapper_sha256', 'private_dir'):
        proof[key] = fixture[key]
    proof.update(runtime_path=pins['runtime_path'], runtime_sha256=pins['runtime_sha256'], service_source=service['source'], service_binary_path=service['binary_path'], service_binary_sha256=service['binary_sha256'], config_path='/etc/serenity/hosted.json', authority_generator_path=auth['generator_path'], authority_generator_sha256=auth['generator_sha256'])
    if operation == 'adopt-source':
        require(type(source_adoption) is dict and set(source_adoption) == {'old_source', 'old_manifest_sha256'})
        require(type(source_adoption['old_source']) is str and re.fullmatch('[0-9a-f]{40}', source_adoption['old_source']) and source_adoption['old_source'] != fixture['tool_source'])
        require(type(source_adoption['old_manifest_sha256']) is str and re.fullmatch('[0-9a-f]{64}', source_adoption['old_manifest_sha256']))
        proof['source_adoption'] = source_adoption
    else:
        require(source_adoption is None)
    return proof, receipt


def script(proof, receipt):
    """Produce an inert invocation; it preserves nonzero/unknown outcomes."""
    payload = base64.b64encode(raw(dict(proof=proof, receipt=receipt))).decode()
    return '''#!/bin/bash
set -euo pipefail
/usr/bin/python3 -I - <<'FIXTUREPY'
import base64,hashlib,json,os,pathlib,stat,subprocess,time
value=json.loads(base64.b64decode(''' + repr(payload) + '''))
p=value['proof']; stage=pathlib.Path(p['maintenance_hold']['stage'])
def require(ok):
 if not ok:raise RuntimeError('fixture invocation refused; details withheld')
require(os.geteuid()==0 and p['issued_at']<=int(time.time())<p['expires_at'])
for item in [stage,*stage.parents]:
 s=item.lstat();require(stat.S_ISDIR(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022)
def publish(path,data):
 require(path.parent==stage)
 fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(data);f.flush();os.fsync(f.fileno())
 fd=os.open(stage,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
 try:os.fsync(fd)
 finally:os.close(fd)
publish(pathlib.Path(p['authority_receipt_path']),json.dumps(value['receipt'],sort_keys=True,separators=(',',':')).encode())
request=stage/('fixture-request-'+p['nonce']+'.json');publish(request,json.dumps(p,sort_keys=True,separators=(',',':')).encode())
wrapper=pathlib.Path(p['wrapper_path'])
for item in [wrapper,*wrapper.parents]:
 s=item.lstat();require(s.st_uid==0 and not s.st_mode&0o022 and not stat.S_ISLNK(s.st_mode))
require(hashlib.sha256(wrapper.read_bytes()).hexdigest()==p['wrapper_sha256'])
unit='serenity-episode-fixture-'+p['maintenance_hold']['operation_id']+'.service'
command=['/usr/bin/systemd-run','--quiet','--wait','--collect','--unit='+unit,'--property=Type=exec','--property=Restart=no','--property=RuntimeMaxSec=150','--property=TimeoutStartSec=20','--property=TimeoutStopSec=10','--property=KillMode=control-group','--property=SendSIGKILL=yes','--property=StandardOutput=null','--property=StandardError=null','/usr/bin/python3','-I',str(wrapper),str(request)]
try: status=subprocess.run(command,stdin=subprocess.DEVNULL,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=185,env={'PATH':'/usr/bin:/bin','LANG':'C'}).returncode
except subprocess.TimeoutExpired:
 subprocess.run(['/usr/bin/systemctl','stop',unit],stdin=subprocess.DEVNULL,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=30,check=True)
 raise SystemExit(6)
drain=subprocess.run(['/usr/bin/systemctl','show',unit,'--property=ActiveState','--property=SubState','--property=ControlGroup','--property=MainPID','--property=ControlPID'],stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,timeout=15,env={'PATH':'/usr/bin:/bin','LANG':'C'})
require(drain.returncode==0 and len(drain.stdout)<=8192)
properties={}
for line in drain.stdout.decode('ascii').splitlines():
 key,sep,value=line.partition('=');require(sep and key not in properties);properties[key]=value
require(set(properties)=={'ActiveState','SubState','ControlGroup','MainPID','ControlPID'})
require(properties['ActiveState'] in ('inactive','failed') and properties['SubState'] in ('dead','failed') and properties['MainPID']=='0' and properties['ControlPID']=='0')
group=properties['ControlGroup']
if group:
 require(group.startswith('/') and '..' not in group.split('/') and group!='/')
 control=pathlib.Path('/sys/fs/cgroup')/group.lstrip('/')
 if control.exists():
  for count,(directory,dirs,files) in enumerate(os.walk(control,followlinks=False),1):
   require(count<=256)
   require(not pathlib.Path(directory).is_symlink() and len(dirs)<=256 and len(files)<=256)
   require(not (pathlib.Path(directory)/'cgroup.procs').read_text().strip())
raise SystemExit(status if status in (0,2,3,5,6) else 6)
FIXTUREPY
'''


def main():
    p = argparse.ArgumentParser()
    for key in ('pins', 'binding', 'authority', 'output'):
        p.add_argument('--' + key, type=Path, required=True)
    p.add_argument('--operation', choices=('prepare', 'reissue', 'revoke', 'adopt-source'), required=True)
    p.add_argument('--adoption', type=Path)
    a = p.parse_args()
    proof, receipt = assemble(decode(a.pins.read_bytes()), decode(a.binding.read_bytes()), decode(a.authority.read_bytes()), a.operation, source_adoption=decode(a.adoption.read_bytes()) if a.adoption else None)
    a.output.mkdir(mode=0o700)
    for name, data in [('proof.json', raw(proof)), ('authority-receipt.json', raw(receipt)), ('invoke.sh', script(proof, receipt).encode())]:
        fd = os.open(a.output / name, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, 'wb') as f: f.write(data); f.flush(); os.fsync(f.fileno())
    print('v3 packet rendered; inert and unapproved')


if __name__ == '__main__':
    main()
