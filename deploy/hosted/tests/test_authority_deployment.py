import importlib.util
import json
from pathlib import Path
import unittest

ROOT=Path(__file__).resolve().parents[3]
DEPLOY=ROOT/"deploy/hosted"
spec=importlib.util.spec_from_file_location("render_authority",DEPLOY/"render_authority.py")
render_module=importlib.util.module_from_spec(spec)
spec.loader.exec_module(render_module)

class AuthorityDeploymentTests(unittest.TestCase):
    def test_changed_archive_replaces_version_and_api_deployment(self):
        original=json.loads((DEPLOY/"authority-stack.json").read_text())
        a=render_module.render(original,b"reviewed artifact A")
        b=render_module.render(original,b"reviewed artifact B")
        version_a=next(k for k,v in a["Resources"].items() if v["Type"]=="AWS::Lambda::Version")
        version_b=next(k for k,v in b["Resources"].items() if v["Type"]=="AWS::Lambda::Version")
        self.assertNotEqual(version_a,version_b)
        self.assertNotEqual(a["Resources"]["Stage"]["Properties"]["DeploymentId"],b["Resources"]["Stage"]["Properties"]["DeploymentId"])
        uri=a["Resources"]["RuntimeMethod"]["Properties"]["Integration"]["Uri"]["Fn::Sub"]
        self.assertIn("${"+version_a+"}",uri)
        self.assertNotIn("${GatewayVersion}",json.dumps(a))
        self.assertIn("GatewayVersion",original["Resources"])

    def test_incarnation_replaces_version_and_retargets_every_reference(self):
        template=json.loads((DEPLOY/"authority-stack.json").read_text())
        rendered=[render_module.render(template,b"same archive",i) for i in ("","recovery-0123abcd","recovery-89abcdef")]
        versions=[]
        deployments=[]
        for incarnation,t in zip(("","recovery-0123abcd","recovery-89abcdef"),rendered):
            version=next(k for k,v in t["Resources"].items() if v["Type"]=="AWS::Lambda::Version")
            deployment=next(k for k,v in t["Resources"].items() if v["Type"]=="AWS::ApiGateway::Deployment")
            versions.append(version); deployments.append(deployment)
            self.assertEqual(t["Parameters"]["JournalIncarnation"]["Default"],incarnation)
            self.assertEqual(t["Resources"]["Stage"]["Properties"]["DeploymentId"],{"Ref":deployment})
            for route in ("RuntimeMethod","OwnerMethod"):
                self.assertIn("${"+version+"}",t["Resources"][route]["Properties"]["Integration"]["Uri"]["Fn::Sub"])
            for v in t["Resources"].values():
                if v["Type"]=="AWS::Lambda::Permission":
                    self.assertEqual(v["Properties"]["FunctionName"],{"Ref":version})
            self.assertNotIn('"Ref": "GatewayVersion"',json.dumps(t))
            self.assertNotIn('"Ref": "Deployment"',json.dumps(t))
            self.assertEqual(t["Outputs"]["GatewayVersionARN"]["Value"],{"Ref":version})
        self.assertEqual(len(set(versions)),3)
        self.assertEqual(len(set(deployments)),3)
        import hashlib
        self.assertEqual(versions[0],"GatewayVersion"+hashlib.sha256(b"same archive").hexdigest()[:24])
        self.assertEqual(rendered[0]["Parameters"]["ArtifactCodeSHA256"],rendered[1]["Parameters"]["ArtifactCodeSHA256"])

    def test_incarnation_validation_and_retained_journal(self):
        template=json.loads((DEPLOY/"authority-stack.json").read_text())
        for invalid in ("recovery-ABCDEF12","recovery-1234567","arbitrary","recovery-12345678\n"):
            with self.assertRaises(ValueError):
                render_module.render(template,b"archive",invalid)
        journal=template["Resources"]["Journal"]
        self.assertEqual(journal["DeletionPolicy"],"Retain")
        self.assertEqual(journal["UpdateReplacePolicy"],"Retain")
        self.assertTrue(journal["Properties"]["DeletionProtectionEnabled"])
        self.assertEqual(journal["Properties"]["TableName"]["Fn::If"][2],{"Ref":"AWS::NoValue"})
        self.assertEqual(template["Conditions"]["HasJournalIncarnation"],{"Fn::Not":[{"Fn::Equals":[{"Ref":"JournalIncarnation"},""]}]})

    def test_generated_routes_require_iam_and_pin_version(self):
        template=json.loads((DEPLOY/"authority-stack.json").read_text())
        resources=template["Resources"]
        self.assertEqual(resources["Gateway"]["Properties"]["ReservedConcurrentExecutions"],0)
        permissions=[v["Properties"] for v in resources.values() if v["Type"]=="AWS::Lambda::Permission"]
        self.assertEqual(len(permissions),2)
        for permission in permissions:
            self.assertEqual(permission["FunctionName"],{"Ref":"GatewayVersion"})
            self.assertEqual(permission["Principal"],"apigateway.amazonaws.com")
            self.assertEqual(permission["SourceAccount"],{"Ref":"AWS::AccountId"})
            self.assertNotIn("*",permission["SourceArn"]["Fn::Sub"])
        for v in resources.values():
            if v["Type"]=="AWS::ApiGateway::Method":
                self.assertEqual(v["Properties"]["AuthorizationType"],"AWS_IAM")

    def test_pending_install_brackets_selection_and_commit(self):
        script=(DEPLOY/"deploy.sh").read_text()
        prepare=script.index('"$release_verifier" prepare')
        install=script.index('install -m 0755 "$work/serenity"')
        select=script.index('ln -sfn "$binary_target"')
        ready=script.index('if ! serenity_wait_ready')
        commit=script.index('"$release_verifier" commit')
        self.assertLess(prepare,install)
        self.assertLess(install,select)
        self.assertLess(select,ready)
        self.assertLess(ready,commit)
        self.assertIn('pending_install=1',script[prepare:install])
        self.assertIn('systemctl stop serenity-hosted',script[script.index('cleanup_deploy()'):script.index('trap cleanup_deploy EXIT')])
        self.assertLess(script.index('flock -n 9'),script.index('"$release_verifier" verify'))
if __name__=="__main__":
    unittest.main()
