#!/usr/bin/env python3
"""Create a reviewable CloudFormation change set; execution is an explicit flag."""
import argparse
import json
import pathlib
import subprocess
import time


def aws(*args):
    result = subprocess.run(["aws", "--region", "us-west-2", "cloudformation", *args, "--output", "json"], check=True, capture_output=True, text=True)
    return json.loads(result.stdout or "{}")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--vpc-id", required=True)
    parser.add_argument("--subnet-id", required=True)
    parser.add_argument("--availability-zone", required=True)
    parser.add_argument("--stack", default="serenity-hosted")
    parser.add_argument("--preview", action="store_true")
    parser.add_argument("--execute", action="store_true")
    args = parser.parse_args()
    if args.preview and args.execute:
        parser.error("choose preview or execute")
    template = pathlib.Path(__file__).with_name("stack.json").resolve()
    aws("validate-template", "--template-body", f"file://{template}")
    try:
        aws("describe-stacks", "--stack-name", args.stack)
        kind = "UPDATE"
    except subprocess.CalledProcessError as exc:
        if "does not exist" not in exc.stderr:
            raise
        kind = "CREATE"
    change = f"hosted-{int(time.time())}"
    params = [{"ParameterKey": k, "ParameterValue": v} for k, v in {"VpcId": args.vpc_id, "SubnetId": args.subnet_id, "AvailabilityZone": args.availability_zone}.items()]
    result = aws("create-change-set", "--stack-name", args.stack, "--change-set-name", change, "--change-set-type", kind,
                 "--template-body", f"file://{template}", "--parameters", json.dumps(params), "--capabilities", "CAPABILITY_IAM",
                 "--tags", "Key=Project,Value=serenity-hosted")
    aws("wait", "change-set-create-complete", "--stack-name", args.stack, "--change-set-name", change)
    details = aws("describe-change-set", "--stack-name", args.stack, "--change-set-name", change)
    print(json.dumps({"change_set": result.get("Id"), "status": details["Status"], "changes": details.get("Changes", [])}, indent=2))
    if args.execute:
        aws("execute-change-set", "--stack-name", args.stack, "--change-set-name", change)


if __name__ == "__main__":
    main()
