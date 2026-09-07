#!/usr/bin/env python3
"""Deploy the reviewed CloudFormation template; never print secret material."""
import argparse,json,os,secrets,subprocess,tempfile,time
from pathlib import Path
parser=argparse.ArgumentParser();parser.add_argument('--region',default='us-west-2');parser.add_argument('--execute',action='store_true');a=parser.parse_args()
root=Path(__file__).resolve().parents[2]
def aws(*args):return subprocess.check_output(['aws',*args,'--region',a.region],text=True)
try:
 stack=json.loads(aws('cloudformation','describe-stacks','--stack-name','serenity-adoption-chat'))['Stacks'][0]
 update=True
except subprocess.CalledProcessError:update=False
if os.environ.get('OPENROUTER_API_KEY'):
 parameters=[{'ParameterKey':'ModelApiKey','ParameterValue':os.environ['OPENROUTER_API_KEY']}]
elif update and any(p['ParameterKey']=='ModelApiKey' for p in stack.get('Parameters',[])):
 parameters=[{'ParameterKey':'ModelApiKey','UsePreviousValue':True}]
else:
 parameters=[{'ParameterKey':'ModelApiKey','ParameterValue':''}]
parameters.append({'ParameterKey':'RateSalt','UsePreviousValue':True} if update else {'ParameterKey':'RateSalt','ParameterValue':secrets.token_hex(32)})
with tempfile.TemporaryDirectory(prefix='serenity-chat-deploy-') as temp:
 path=Path(temp)/'parameters.json';path.write_text(json.dumps(parameters));path.chmod(0o600)
 command=['cloudformation','create-change-set','--stack-name','serenity-adoption-chat','--change-set-name','site-'+str(int(time.time())),'--change-set-type','UPDATE' if update else 'CREATE','--template-body','file://'+str(root/'deploy/chat/stack.json'),'--parameters','file://'+str(path),'--capabilities','CAPABILITY_IAM']
 response=json.loads(aws(*command));change=response['Id']
 aws('cloudformation','wait','change-set-create-complete','--change-set-name',change)
 changes=json.loads(aws('cloudformation','describe-change-set','--change-set-name',change))
 print(json.dumps([{'action':c['ResourceChange']['Action'],'resource':c['ResourceChange']['LogicalResourceId'],'type':c['ResourceChange']['ResourceType']} for c in changes['Changes']],indent=2))
 if a.execute:
  aws('cloudformation','execute-change-set','--change-set-name',change)
  print('CloudFormation deployment started: serenity-adoption-chat')
 else:print('Review complete. Change set:',change)
