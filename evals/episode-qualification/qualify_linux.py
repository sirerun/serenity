#!/usr/bin/env python3
"""Synthetic child qualification only. Not an operational authority producer.

No production files are opened. Protocol-required /etc and /var/lib path strings
are inert proof labels; actual files, locks and data live in a fresh /tmp tree.
The real offline_operator.py/systemd/provider wrapper is deliberately not run.
"""
import argparse
import copy
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import selectors
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import time

SOURCE = None  # Explicit build identity supplied by the bounded runner.
SCHEMA = "9bd9153fd18f433eb1d0416c7faa38ca6eefdc29ebcd8585f3a021b8b63ed963"
UID = GID = 1800
SUCCESS = b"fixture operation completed; synthetic accounts/data retained; capture unchanged\n"
REFUSED = b"REFUSED: fixture custody or operation failed; retain hold for reconciliation\n"
ENV = {"PATH": "/usr/bin:/bin", "LANG": "C", "GIT_CONFIG_NOSYSTEM": "1",
       "GIT_CONFIG_GLOBAL": "/dev/null"}


class CheckFailed(Exception):
    pass


def require(value, label):
    if not value:
        raise CheckFailed(label)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def encoded(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":")).encode()


def write_private(path, data, owner=0):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "wb") as f:
        f.write(data)
        f.flush()
        os.fsync(f.fileno())
        os.fchown(f.fileno(), owner, owner)


def service_dir(path):
    path.mkdir(mode=0o700)
    os.chown(path, UID, GID)


def snapshot(*roots):
    """In-memory only: names, ownership, modes, inode, times and content hashes.

    atime is omitted because reads are allowed. No snapshot is emitted; it may
    include hashes of synthetic keys. SQLite sidecars are NOT excluded.
    """
    result = {}
    for root in roots:
        for p in [root] + sorted(root.rglob("*")):
            st = p.lstat()
            require(stat.S_ISREG(st.st_mode) or stat.S_ISDIR(st.st_mode), "unexpected file type")
            digest = sha(p.read_bytes()) if p.is_file() else None
            result[str(p)] = (st.st_mode, st.st_uid, st.st_gid, st.st_nlink,
                              st.st_ino, st.st_size, st.st_mtime_ns, st.st_ctime_ns, digest)
    return result


def run_child(argv, cwd, uid=UID, pass_fds=()):
    child = subprocess.Popen(argv, cwd=cwd, env=ENV, user=uid, group=GID,
                             extra_groups=[], pass_fds=tuple(pass_fds),
                             stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE, umask=0o077, start_new_session=True)
    buffers = [bytearray(), bytearray()]
    selector = selectors.DefaultSelector()
    deadline = time.monotonic() + 120
    try:
        for i, stream in enumerate((child.stdout, child.stderr)):
            os.set_blocking(stream.fileno(), False)
            selector.register(stream, selectors.EVENT_READ, i)
        while selector.get_map():
            remaining = deadline-time.monotonic()
            require(remaining>0, "child output deadline")
            for key, _ in selector.select(min(remaining, 1)):
                data=os.read(key.fileobj.fileno(), 4096)
                if not data:
                    selector.unregister(key.fileobj)
                    continue
                buffers[key.data].extend(data)
                require(len(buffers[key.data])<=65536, "bounded child output")
        remaining=deadline-time.monotonic()
        require(remaining>0, "child completion deadline")
        status=child.wait(timeout=remaining)
        return status, bytes(buffers[0]), bytes(buffers[1])
    except BaseException:
        # No poll()/wait() before output completion: an exited leader remains
        # unreaped and anchors its process-group identity while pipes stay open.
        try:
            os.killpg(child.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        child.stdout.close()
        child.stderr.close()
        child.wait(timeout=5)
        raise
    finally:
        selector.close()
        child.stdout.close()
        child.stderr.close()



class Case:
    def __init__(self, root, name, tool, helper, version=3):
        self.root = root / name
        self.root.mkdir(mode=0o755)
        self.root.chmod(0o755)  # Also work when the root launcher uses umask 077.
        self.data = self.root / "data"
        self.private = self.root / "private"
        self.custody = self.root / "root-custody"
        service_dir(self.data)
        service_dir(self.data / "brains")
        service_dir(self.private)
        self.custody.mkdir(mode=0o700)
        self.tool, self.helper = tool, helper
        self.version = version
        self.helper_call("init")
        now = int(time.time())
        state = encoded({"synthetic_only": True, "source": SOURCE})
        config = encoded({"data_dir": str(self.data), "account_cap": 2,
                          "episode_archive_enabled": False, "episode_synthetic_brains": []})
        dummy = sha(b"supplemental synthetic binding; not provider evidence")
        hold = dict(manifest_sha256=dummy, executable_sha256=dummy, source=SOURCE,
                    previous_state_sha256=sha(state), expected_state_sha256=dummy,
                    supervisor="serenity-release-supplemental-synthetic.service",
                    stage="/var/lib/serenity-release/supplemental-synthetic-never-live",
                    backup_timer_prior_state="active", phase="maintenance-ready", timestamp=now)
        hold_bytes = encoded(hold)
        for name, data in (("hold", hold_bytes), ("state", state), ("config", config), ("lock", b"")):
            write_private(self.custody / name, data)
        st = (self.custody / "lock").stat()
        self.proof = dict(version=1, purpose="serenity-episode-fixture-offline-v1",
                          operation="prepare", nonce=secrets.token_hex(32),
                          issued_at=now, expires_at=now + 120,
                          boot_id=Path("/proc/sys/kernel/random/boot_id").read_text().strip(),
                          uid=UID, gid=GID, tool_source=SOURCE, tool_path=str(tool),
                          tool_sha256=sha(tool.read_bytes()), wrapper_path=str(Path(__file__).resolve()),
                          wrapper_sha256=sha(Path(__file__).read_bytes()),
                          runtime_path=hold["stage"] + "/activation-runtime.py", runtime_sha256=dummy,
                          service_binary_path=str(tool), service_binary_sha256=sha(tool.read_bytes()),
                          service_source=SOURCE, config_path="/etc/serenity/hosted.json",
                          config_sha256=sha(config), data_dir=str(self.data), private_dir=str(self.private),
                          account_cap=2, hold_sha256=sha(hold_bytes), hold=hold,
                          state_sha256=sha(state), ready_sha256=dummy,
                          authority_receipt_path=str(self.custody / "synthetic-nonexistent-receipt"),
                          authority_receipt_sha256=dummy,
                          authority_generator_path=str(self.custody / "synthetic-nonexistent-generator"),
                          authority_generator_sha256=dummy, lock_device=st.st_dev, lock_inode=st.st_ino,
                          authority_scope="synthetic-only-no-provider", authority_session_id="synthetic-only",
                          concurrency_target="synthetic-only-no-provider", schema_sha256=SCHEMA)

    def v3_proof(self, p):
        # Use the actual pure producer, with synthetic authority observations.
        # These are test inputs and never provider evidence or host permission.
        here = Path(__file__).resolve().parents[2] / "deploy/hosted/maintenance"
        sys.path.insert(0, str(here))
        try:
            spec = importlib.util.spec_from_file_location("joined_fixture_shapes", here / "test_fixture_proof.py")
            shapes = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(shapes)
            pins, binding, authority, _ = shapes.cohort()
            c = pins['config']; service, fixture = c['service'], c['fixture']
            c.update(issued_at=p['issued_at']-1, expires_at=p['expires_at'], boot_id=p['boot_id'])
            service.update(source=p['service_source'], binary_path=p['service_binary_path'], binary_sha256=p['service_binary_sha256'], state_sha256=p['state_sha256'], config_sha256=p['config_sha256'])
            fixture.update(**{k:p[k] for k in ('tool_source','tool_path','tool_sha256','wrapper_path','wrapper_sha256','uid','gid','data_dir','private_dir','account_cap','schema_sha256')})
            c['manifest_sha256']=sha(encoded({k:v for k,v in c.items() if k!='manifest_sha256'}))
            h=dict(version=1,purpose=c['purpose'],operation_id=c['operation_id'],manifest_sha256=c['manifest_sha256'],service_source=service['source'],executable_sha256=service['binary_sha256'],state_sha256=service['state_sha256'],broker_sha256=service['broker_sha256'],config_sha256=service['config_sha256'],supervisor=c['supervisor'],stage=c['stage'],backup_timer_prior_state='active',phase='maintenance-ready',timestamp=c['issued_at'],expires_at=c['expires_at'])
            hh=sha(encoded(h)); ready=dict(version=2,purpose=c['purpose'],operation_id=c['operation_id'],manifest_sha256=c['manifest_sha256'],state_sha256=service['state_sha256'],hold_sha256=hh,service_source=service['source'],executable_sha256=service['binary_sha256'],held=True,services_started=False)
            binding.update(**{k:p[k] for k in ('boot_id','uid','gid','data_dir','account_cap','schema_sha256','state_sha256','config_sha256','lock_device','lock_inode')},observed_at=p['issued_at'],maintenance_hold=h,hold_sha256=hh,ready_sha256=sha(encoded(ready)))
            import datetime
            authority.update(observed_at=datetime.datetime.fromtimestamp(p['issued_at']-1,datetime.timezone.utc).isoformat(),completed_at=datetime.datetime.fromtimestamp(p['issued_at'],datetime.timezone.utc).isoformat())
            proof, _ = shapes.producer.assemble(pins,binding,authority,p['operation'],now=p['issued_at'],nonce=p['nonce'],source_adoption=p.get('source_adoption'))
            # Actual Python consumer strict shape reaches the actual Go child below.
            require(shapes.consumer.Adapter(clock=lambda:p['issued_at']).validate_proof(encoded(proof)) == proof, 'producer to Python consumer')
            hold_path=self.custody/'hold'
            with hold_path.open('wb') as f:
                f.write(encoded(h));f.flush();os.fsync(f.fileno())
            return proof
        finally:
            sys.path.pop(0)

    def helper_call(self, mode):
        rc, out, err = run_child([str(self.helper), mode, str(self.data), str(self.private)], self.private)
        require(rc == 0 and err == b"", "helper " + mode)
        require(json.loads(out) == {"uid": UID, "gid": GID, "supplementary_groups": [], "check": "passed"},
                "helper identity and result")

    def custody_snapshot(self):
        rc,out,err=run_child([str(self.tool),'--fixture-custody-snapshot',str(self.data),str(self.private),self.proof['config_sha256'],str(self.proof['account_cap'])],self.private)
        require(rc==0 and err==b'', 'service UID metadata snapshot')
        value=json.loads(out)
        require(type(value) is dict and set(value)==set('version private_inventory_sha256 fixture_scope_sha256 schema_sha256'.split()) and value['version']==1 and type(value['version']) is int, 'snapshot strict fields')
        require(encoded(value)==out and value['schema_sha256']==SCHEMA, 'snapshot canonical schema')
        for k in ('private_inventory_sha256','fixture_scope_sha256','schema_sha256'):
            require(type(value[k]) is str and len(value[k])==64 and all(c in '0123456789abcdef' for c in value[k]), 'snapshot hashes')
        return value

    def call(self, operation="prepare", mutation=None, held=True, uid=UID, adoption=None):
        p = copy.deepcopy(self.proof)
        p.update(operation=operation, nonce=secrets.token_hex(32), issued_at=int(time.time()))
        p["expires_at"] = p["issued_at"] + 120
        if adoption is not None:
            p["source_adoption"] = adoption
        if self.version == 3:
            p = self.v3_proof(p)
        if mutation:
            mutation(p)
        proof_path = self.custody / ("proof-" + secrets.token_hex(8))
        write_private(proof_path, encoded(p))
        fds = []
        try:
            for path in (proof_path, self.custody / "hold", self.custody / "state", self.custody / "config"):
                fds.append(os.open(path, os.O_RDONLY | os.O_NOFOLLOW))
            lock = os.open(self.custody / "lock", os.O_RDWR | os.O_NOFOLLOW)
            fds.append(lock)
            probe = os.open(self.custody / "lock", os.O_RDWR | os.O_NOFOLLOW)
            fds.append(probe)
            require(len(set(fds)) == 6 and all(fd >= 3 for fd in fds), "six distinct descriptors")
            if held:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            before = snapshot(self.data, self.private)
            rc, out, err = run_child([str(self.tool), "--custody-fds", ",".join(map(str, fds))],
                                    self.private if uid == UID else self.root, uid, fds)
            after = snapshot(self.data, self.private)
            # A fresh description must still conflict after the child exits:
            # this proves the parent retained its lock through child execution.
            if held:
                independent = os.open(self.custody / "lock", os.O_RDWR | os.O_NOFOLLOW)
                try:
                    try:
                        fcntl.flock(independent, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    except BlockingIOError:
                        pass
                    else:
                        raise CheckFailed("parent lost lock")
                finally:
                    os.close(independent)
            return rc, out, err, before == after
        finally:
            for fd in reversed(fds):
                os.close(fd)


def qualify(tool, helper, report, legacy_tool=None, legacy_source=None):
    require(sys.platform == "linux" and os.geteuid() == 0 and os.getegid() == 0, "Linux root required")
    require(shutil.which("git", path=ENV["PATH"]) is not None, "existing Git required")
    for path in (tool, helper, *([legacy_tool] if legacy_tool else [])):
        st = path.lstat()
        require(stat.S_ISREG(st.st_mode) and st.st_uid == 0 and st.st_mode & 0o022 == 0, "trusted binary")
    results = []
    scratch = Path(tempfile.mkdtemp(prefix="serenity-custody-source-", dir="/tmp"))
    # Root-owned searchable ancestor; data/private leaves are service-owned 0700.
    scratch.chmod(0o755)
    try:
        negatives = [
            ("wrong-source", lambda p: p.update(tool_source="0" * 40), {}, None),
            ("expired-proof", lambda p: p.update(expires_at=p["issued_at"] - 1), {}, None),
            ("unheld-lock", None, {"held": False}, None),
            ("wrong-uid", None, {"uid": 1801}, None),
            ("future-schema", None, {}, "future-schema"),
            ("unknown-schema-object", None, {}, "extra-schema-object"),
            ("wrong-executable-hash", lambda p: p.update(tool_sha256="0" * 64), {}, None),
            ("wrong-boot", lambda p: p.update(boot_id="00000000-0000-0000-0000-000000000000"), {}, None),
        ]
        for name, mutation, kwargs, schema_mode in negatives:
            case = Case(scratch, name, tool, helper)
            if schema_mode:
                case.helper_call(schema_mode)
            rc, out, err, unchanged = case.call(mutation=mutation, **kwargs)
            require(unchanged, name + ": rejected call changed data/private tree")
            require(rc == 1 and out == b"" and err == REFUSED, name + ": expected exact refusal")
            results.append({"case": name, "passed": True, "data_and_private_unchanged": True})

        case = Case(scratch, "positive", tool, helper)
        empty_snapshot=case.custody_snapshot()
        require(empty_snapshot==case.custody_snapshot(), "stable knownempty snapshot")
        active_snapshot=None
        for operation, label in (("prepare", "prepare"), ("prepare", "prepare-replay"),
                                 ("revoke", "revoke"), ("revoke", "revoke-replay")):
            before_manifest = ((case.private / "manifest.json").read_bytes()
                               if (case.private / "manifest.json").exists() else None)
            brains_before = snapshot(case.data / "brains") if before_manifest else None
            rc, out, err, _ = case.call(operation)
            require(rc == 0 and out == b"" and err == SUCCESS, label + ": expected success")
            if label == "prepare":
                write_private(case.data / "original-manifest.json", (case.private / "manifest.json").read_bytes(), UID)
            elif label.endswith("replay"):
                require(before_manifest == (case.private / "manifest.json").read_bytes(), label + ": manifest changed")
            if brains_before is not None:
                require(brains_before == snapshot(case.data / "brains"), label + ": brain data changed")
            case.helper_call("active" if operation == "prepare" else "revoked")
            observed=case.custody_snapshot()
            require(observed==case.custody_snapshot(), label+': stable custody snapshot')
            if label=='prepare':
                active_snapshot=observed
                require(observed!=empty_snapshot, 'prepare changed scoped snapshot')
            if label=='revoke':
                require(observed['fixture_scope_sha256']!=active_snapshot['fixture_scope_sha256'], 'revoke changed credential metadata digest')
            results.append({"case": label, "passed": True})
        if legacy_tool is not None:
            # Real legacy preparation, followed by current v3 adoption and rotation.
            case = Case(scratch, "legacy-adoption", legacy_tool, helper, version=1)
            case.proof['tool_source'] = legacy_source
            case.proof['service_source'] = legacy_source
            case.proof['hold']['source'] = legacy_source
            with (case.custody/'hold').open('wb') as f:
                f.write(encoded(case.proof['hold']));f.flush();os.fsync(f.fileno())
            case.proof['hold_sha256'] = sha((case.custody/'hold').read_bytes())
            rc,out,err,_=case.call()
            require(rc==0 and out==b'' and err==SUCCESS, 'legacy prepare')
            write_private(case.data / "original-manifest.json", (case.private / "manifest.json").read_bytes(), UID)
            before=(case.private/'manifest.json').read_bytes()
            case.tool=tool;case.version=3
            case.proof.update(tool_source=SOURCE,tool_path=str(tool),tool_sha256=sha(tool.read_bytes()))
            refused_adoption=dict(old_source=legacy_source,old_manifest_sha256=sha(before))
            rc,out,err,unchanged=case.call('adopt-source',adoption=refused_adoption)
            require(rc==2 and out==b'' and err==b'source adoption refused before publication; no adoption write; retain hold\n' and unchanged, 'active legacy adoption refused unchanged')
            results.append(dict(case='active-legacy-adoption-refused',passed=True,data_and_private_unchanged=True))
            # Existing adoption requires known externally revoked credentials
            # WITH retained old private custody. An active legacy key must refuse.
            case.helper_call('externally-revoke')
            require(before==(case.private/'manifest.json').read_bytes(), 'external revoke retained private custody')
            case.helper_call('retained-revoked')
            case.tool=tool;case.version=3
            case.proof.update(tool_source=SOURCE,tool_path=str(tool),tool_sha256=sha(tool.read_bytes()))
            for operation,mode in [('adopt-source','retained-revoked'),('reissue','reissued'),('revoke','revoked')]:
                adoption=dict(old_source=legacy_source,old_manifest_sha256=sha(before)) if operation=='adopt-source' else None
                brains=snapshot(case.data/'brains')
                rc,out,err,_=case.call(operation,adoption=adoption)
                require(rc==0 and out==b'' and err==SUCCESS, 'legacy '+operation)
                require(brains==snapshot(case.data/'brains'), 'legacy brain preservation')
                case.helper_call(mode)
                results.append(dict(case='legacy-'+operation,passed=True))
        receipt = {"status": "passed", "qualification": "real-linux-child-uid-fd-synthetic-persistence-only",
                   "source_revision": SOURCE, "schema_sha256": SCHEMA, "tool_sha256": sha(tool.read_bytes()),
                   "helper_sha256": sha(helper.read_bytes()), "harness_sha256": sha(Path(__file__).read_bytes()),
                   "uid": UID, "gid": GID, "supplementary_groups": [], "inherited_fds": 6,
                   "boot_id_checked_against_proc": True, "parent_lock_retained": True,
                   "producer_and_python_shape_qualified": True, "systemd_wrapper_qualified": False, "provider_qualified": False,
                   "live_use_authorized": False, "cases": results}
    finally:
        # Remove only this harness-created synthetic tree. Not secure erasure.
        shutil.rmtree(scratch)
    receipt["synthetic_scratch_removed"] = True
    write_private(report, encoded(receipt) + b"\n")
    print(json.dumps(receipt, sort_keys=True))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tool", required=True, type=Path)
    parser.add_argument("--helper", required=True, type=Path)
    parser.add_argument("--report", required=True, type=Path)
    parser.add_argument("--tool-source", required=True)
    parser.add_argument("--legacy-tool", type=Path)
    parser.add_argument("--legacy-source")
    args = parser.parse_args()
    global SOURCE
    SOURCE = args.tool_source
    try:
        require(len(SOURCE)==40 and all(c in "0123456789abcdef" for c in SOURCE), "explicit source identity")
        require(bool(args.legacy_tool)==bool(args.legacy_source), "paired legacy identity")
        qualify(args.tool.resolve(strict=True), args.helper.resolve(strict=True), args.report.absolute(), args.legacy_tool.resolve(strict=True) if args.legacy_tool else None, args.legacy_source)
    except CheckFailed as exc:
        print("SUPPLEMENTAL_LINUX_FAILED: " + str(exc), file=sys.stderr)
        return 1
    except BaseException:
        # Exceptions may carry child output or file content: keep receipts safe.
        print("SUPPLEMENTAL_LINUX_FAILED: harness prerequisite or execution error", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
