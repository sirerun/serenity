#!/usr/bin/env python3
"""Unpack a bounded backup archive as an unprivileged user; does not restore/admit."""
import argparse
import ctypes
import os
from pathlib import Path
import re
import shutil
import tarfile
import tempfile

MAX_BYTES=2*1024*1024*1024
MAX_FILES=10003

def extract(archive,destination):
    archive=Path(archive)
    destination=Path(destination)
    if os.geteuid()==0:
        raise ValueError("unpack snapshots as an unprivileged operator")
    if archive.stat().st_size>MAX_BYTES:
        raise ValueError("archive exceeds transport ceiling")
    parent=destination.parent.resolve(strict=True)
    if parent.stat().st_uid!=os.geteuid() or parent.stat().st_mode&0o077:
        raise ValueError("destination parent must be private and owned")
    if not destination.name or destination.name in (".",".."):
        raise ValueError("invalid destination")
    target=parent/destination.name
    staging=Path(tempfile.mkdtemp(prefix=".serenity-snapshot-",dir=parent))
    try:
        seen=set()
        total=0
        with tarfile.open(archive,"r:") as tf:
            for member in tf:
                name=member.name
                if name.startswith("./"):name=name[2:]
                if member.isdir() and name in ("","brains"):
                    continue
                if not member.isfile() or name in seen:
                    raise ValueError("archive contains a link, special file or duplicate")
                if name not in ("manifest.json","authority-cut.json","control.db") and not re.fullmatch(r"brains/[A-Za-z0-9]{16,64}\.bundle",name):
                    raise ValueError("unexpected snapshot path")
                seen.add(name)
                total+=member.size
                if len(seen)>MAX_FILES or member.size<0 or total>MAX_BYTES:
                    raise ValueError("archive inventory exceeds transport bounds")
                output=staging/name
                output.parent.mkdir(mode=0o700,exist_ok=True)
                stream=tf.extractfile(member)
                if stream is None:raise ValueError("missing regular file payload")
                with stream,output.open("xb") as dest:
                    os.chmod(output,0o600)
                    remaining=member.size
                    while remaining:
                        chunk=stream.read(min(remaining,1024*1024))
                        if not chunk:raise ValueError("truncated archive member")
                        dest.write(chunk);remaining-=len(chunk)
                    dest.flush();os.fsync(dest.fileno())
        if not {"manifest.json","authority-cut.json","control.db"}<=seen:
            raise ValueError("snapshot authority/manifest/control file missing")
        # Linux deployment tool: atomically publish without replacing a raced path.
        libc=ctypes.CDLL(None,use_errno=True)
        rename=libc.renameat2
        rename.argtypes=[ctypes.c_int,ctypes.c_char_p,ctypes.c_int,ctypes.c_char_p,ctypes.c_uint]
        if rename(-100,os.fsencode(staging),-100,os.fsencode(target),1)!=0:
            raise OSError(ctypes.get_errno(),"snapshot publication failed")
        staging=None
    finally:
        if staging is not None:shutil.rmtree(staging)

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument("archive",type=Path)
    p.add_argument("destination",type=Path)
    a=p.parse_args()
    extract(a.archive,a.destination)
if __name__=="__main__":
    main()
