#!/usr/bin/env python3
"""Render immutable Lambda-version and API deployment identities; no AWS calls."""
import argparse
import base64
import hashlib
import json
from pathlib import Path

def render(template, archive):
    digest = hashlib.sha256(archive).digest()
    suffix = digest.hex()[:24]
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
    return result

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archive",type=Path,required=True)
    parser.add_argument("--output",type=Path,required=True)
    args=parser.parse_args()
    template=json.loads(Path(__file__).with_name("authority-stack.json").read_text())
    output=render(template,args.archive.read_bytes())
    # Never overwrite an existing reviewed activation candidate.
    with args.output.open("x") as stream:
        json.dump(output,stream,indent=2)
        stream.write("\n")
if __name__=="__main__":
    main()
