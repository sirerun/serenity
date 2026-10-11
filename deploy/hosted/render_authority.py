#!/usr/bin/env python3
"""Render immutable Lambda-version and API deployment identities; no AWS calls."""
import argparse
import base64
import hashlib
import json
import re
from pathlib import Path

def render(template, archive, journal_incarnation=""):
    if not re.fullmatch(r"(?:recovery-[a-f0-9]{8})?", journal_incarnation):
        raise ValueError("invalid journal incarnation")
    digest = hashlib.sha256(archive).digest()
    identity = digest if not journal_incarnation else hashlib.sha256(b"serenity-journal-incarnation-v1\0" + digest + journal_incarnation.encode("ascii")).digest()
    suffix = identity.hex()[:24]
    names = {"GatewayVersion": "GatewayVersion" + suffix, "Deployment": "Deployment" + suffix}
    def visit(value):
        if isinstance(value, dict):
            return {names.get(k,k): visit(v) for k,v in value.items()}
        if isinstance(value, list):
            return [visit(v) for v in value]
        if isinstance(value,str):
            if value in names:
                return names[value]
            for old,new in names.items():
                value=value.replace("${"+old+"}", "${"+new+"}")
            return value
        return value
    result=visit(template)
    result["Parameters"]["ArtifactCodeSHA256"]["Default"]=base64.b64encode(digest).decode("ascii")
    result["Parameters"]["JournalIncarnation"]["Default"]=journal_incarnation
    return result

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archive",type=Path,required=True)
    parser.add_argument("--output",type=Path,required=True)
    parser.add_argument("--journal-incarnation",default="",help="reviewed recovery-[8 lowercase hex] table incarnation")
    args=parser.parse_args()
    template=json.loads(Path(__file__).with_name("authority-stack.json").read_text())
    output=render(template,args.archive.read_bytes(),args.journal_incarnation)
    # Never overwrite an existing reviewed activation candidate.
    with args.output.open("x") as stream:
        json.dump(output,stream,indent=2)
        stream.write("\n")
if __name__=="__main__":
    main()
