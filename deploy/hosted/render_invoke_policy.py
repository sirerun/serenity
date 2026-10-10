#!/usr/bin/env python3
"""Render separate base and published-version exclusive Lambda invoke policies. Never applies it."""
import argparse
import json
import re
from pathlib import Path

def policies(function_arn, version, api_id, account, region):
    if not re.fullmatch(r"arn:aws:lambda:"+re.escape(region)+":"+re.escape(account)+r":function:[A-Za-z0-9-_]+",function_arn):
        raise ValueError("function ARN must bind the exact account and region")
    if not re.fullmatch(r"[1-9][0-9]*",version) or not re.fullmatch(r"[a-z0-9]+",api_id) or not re.fullmatch(r"[0-9]{12}",account):
        raise ValueError("invalid exact policy identity")
    resources=[function_arn,function_arn+":*"]
    routes=[f"arn:aws:execute-api:{region}:{account}:{api_id}/prod/POST/{route}" for route in ("runtime","owner")]
    base = {"Version":"2012-10-17","Statement":[
        {"Sid":"DenyNonGatewayInvocation","Effect":"Deny","Principal":"*","Action":"lambda:InvokeFunction","Resource":resources,
         "Condition":{"ArnNotEquals":{"aws:SourceArn":routes}}},
        {"Sid":"DenyOtherSourceAccount","Effect":"Deny","Principal":"*","Action":"lambda:InvokeFunction","Resource":resources,
         "Condition":{"StringNotEquals":{"aws:SourceAccount":account}}}
    ]}
    qualified = json.loads(json.dumps(base))
    for statement in qualified["Statement"]:
        statement["Resource"] = function_arn+":"+version
    qualified["Statement"].append(
        {"Sid":"AllowExactGatewayVersion","Effect":"Allow","Principal":{"Service":"apigateway.amazonaws.com"},"Action":"lambda:InvokeFunction",
         "Resource":function_arn+":"+version,"Condition":{"ArnEquals":{"aws:SourceArn":routes},"StringEquals":{"aws:SourceAccount":account}}}
    )
    return base, qualified

def main():
    p=argparse.ArgumentParser(description=__doc__)
    for field in ("function-arn","version","api-id","account","region"):
        p.add_argument("--"+field,required=True)
    p.add_argument("--output",required=True,type=Path)
    p.add_argument("--version-output",required=True,type=Path)
    a=p.parse_args()
    if a.output.resolve() == a.version_output.resolve():
        p.error("base and version output paths must differ")
    base, qualified=policies(a.function_arn,a.version,a.api_id,a.account,a.region)
    for path, data in ((a.output, base), (a.version_output, qualified)):
        with path.open("x") as stream:
            json.dump(data,stream,indent=2)
            stream.write("\n")
if __name__=="__main__":
    main()
