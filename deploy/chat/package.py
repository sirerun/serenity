#!/usr/bin/env python3
"""Produce a reviewable CloudFormation template with immutable inline source."""
import json, base64, zlib
from pathlib import Path
root=Path(__file__).resolve().parents[2]
source=(root/'deploy/chat/handler.py').read_text().replace('CORPUS = []  # Injected from the committed public site by package.py.', 'import zlib\nCORPUS = json.loads(zlib.decompress(base64.b64decode('+repr(base64.b64encode(zlib.compress((root/'site/content.json').read_bytes())).decode())+')))')
T={'AWSTemplateFormatVersion':'2010-09-09','Description':'Serenity public documentation chat: Lambda URL, rate limits, least-privilege read of an existing model secret.', 'Parameters':{'ModelApiKey':{'Type':'String','Default':'','NoEcho':True},'RateSalt':{'Type':'String','NoEcho':True,'MinLength':32}},'Conditions':{'HasSecret':{'Fn::Not':[{'Fn::Equals':[{'Ref':'ModelApiKey'},'']}]}},'Resources':{},'Outputs':{}}
r=T['Resources']
r['ModelSecret']={'Type':'AWS::SecretsManager::Secret','Condition':'HasSecret','Properties':{'Name':'serenity/adoption/openrouter','Description':'Model credential for Serenity public documentation chat','SecretString':{'Ref':'ModelApiKey'}}}
r['RateTable']={'Type':'AWS::DynamoDB::Table','Properties':{'BillingMode':'PAY_PER_REQUEST','AttributeDefinitions':[{'AttributeName':'pk','AttributeType':'S'}],'KeySchema':[{'AttributeName':'pk','KeyType':'HASH'}],'TimeToLiveSpecification':{'AttributeName':'expires','Enabled':True},'SSESpecification':{'SSEEnabled':True}}}
r['Role']={'Type':'AWS::IAM::Role','Properties':{'AssumeRolePolicyDocument':{'Version':'2012-10-17','Statement':[{'Effect':'Allow','Principal':{'Service':'lambda.amazonaws.com'},'Action':'sts:AssumeRole'}]},'Policies':[{'PolicyName':'SerenityChatResources','PolicyDocument':{'Version':'2012-10-17','Statement':[{'Effect':'Allow','Action':['dynamodb:UpdateItem'],'Resource':{'Fn::GetAtt':['RateTable','Arn']}},{'Fn::If':['HasSecret',{'Effect':'Allow','Action':['secretsmanager:GetSecretValue'],'Resource':{'Ref':'ModelSecret'}},{'Ref':'AWS::NoValue'}]}]}}]}}
r['Function']={'Type':'AWS::Lambda::Function','Properties':{'FunctionName':'serenity-adoption-chat','Runtime':'python3.13','Handler':'index.handler','Role':{'Fn::GetAtt':['Role','Arn']},'Timeout':28,'MemorySize':256,'Code':{'ZipFile':source},'Environment':{'Variables':{'MODEL_SECRET_ARN':{'Fn::If':['HasSecret',{'Ref':'ModelSecret'},'']},'RATE_TABLE':{'Ref':'RateTable'},'RATE_SALT':{'Ref':'RateSalt'},'ASK_MODEL':'openai/gpt-4.1-mini'}},'Tags':[{'Key':'Project','Value':'serenity'}]}}
r['URL']={'Type':'AWS::Lambda::Url','Properties':{'TargetFunctionArn':{'Ref':'Function'},'AuthType':'NONE','Cors':{'AllowOrigins':['https://serenity.sire.run','http://127.0.0.1:8937'],'AllowMethods':['POST','GET'],'AllowHeaders':['content-type'],'MaxAge':3600}}}
r['URLPermission']={'Type':'AWS::Lambda::Permission','Properties':{'Action':'lambda:InvokeFunctionUrl','FunctionName':{'Ref':'Function'},'Principal':'*','FunctionUrlAuthType':'NONE'}}
r['InvokePermission']={'Type':'AWS::Lambda::Permission','Properties':{'Action':'lambda:InvokeFunction','FunctionName':{'Ref':'Function'},'Principal':'*','InvokedViaFunctionUrl':True}}
T['Outputs']['ChatURL']={'Value':{'Fn::GetAtt':['URL','FunctionUrl']}}
(root/'deploy/chat/stack.json').write_text(json.dumps(T,indent=2)+'\n')
print('Packaged public documentation into CloudFormation template')
