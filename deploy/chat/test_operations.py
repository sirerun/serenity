"""Operational behavior against the actual packaged handler and AWS request seam."""
import contextlib
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import sys
from types import SimpleNamespace
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('operations_handler', Path(__file__).with_name('handler.py'))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
m.CORPUS = json.loads((ROOT / 'site/content.json').read_text())


class OperationsTests(unittest.TestCase):
    def event(self, origin=m.BASE, method='POST', path='/', message='How do I install?'):
        return {'headers': {'origin': origin}, 'rawPath': path,
                'requestContext': {'http': {'method': method, 'sourceIp': '192.0.2.44'}},
                'body': json.dumps({'message': message, 'history': []})}

    def invoke(self, event):
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            response = m.handler(event, None)
        records = [json.loads(line) for line in output.getvalue().splitlines()]
        return response, records

    def test_health_is_read_only_and_does_not_consume_chat_budget(self):
        with patch.object(m, 'limit') as limit, patch.object(m, 'compose') as compose:
            response, records = self.invoke(self.event(method='GET', path='/healthz'))
        self.assertEqual(response['statusCode'], 200)
        self.assertEqual(json.loads(response['body']), {'status': 'ok', 'documents': len(m.CORPUS)})
        self.assertGreater(len(m.CORPUS), 0)
        self.assertEqual(records, [])
        limit.assert_not_called()
        compose.assert_not_called()

    def test_allowed_origin_answer_emits_fixed_metrics(self):
        with patch.object(m, 'limit') as limit, patch.object(m, 'compose', return_value=('Cited answer', 'answer')):
            response, records = self.invoke(self.event())
        self.assertEqual(json.loads(response['body'])['answer'], 'Cited answer')
        limit.assert_called_once_with('192.0.2.44')
        self.assertEqual(len(records), 1)
        record = records[0]
        self.assertEqual(set(record), {'_aws', 'Service', 'ChatRequests', 'ChatErrors', 'ChatFallbacks', 'ChatRateLimited', 'ChatLatencyMs'})
        self.assertEqual([record[x] for x in ('ChatRequests', 'ChatErrors', 'ChatFallbacks', 'ChatRateLimited')], [1, 0, 0, 0])
        self.assertGreaterEqual(record['ChatLatencyMs'], 0)
        directive = record['_aws']['CloudWatchMetrics'][0]
        self.assertEqual(directive['Namespace'], 'Serenity/AdoptionChat')
        self.assertEqual(directive['Dimensions'], [['Service']])
        self.assertEqual({x['Name'] for x in directive['Metrics']}, set(record) - {'_aws', 'Service'})

    def test_rejected_origin_never_calls_provider(self):
        with patch.object(m, 'limit') as limit, patch.object(m, 'compose') as compose:
            response, records = self.invoke(self.event(origin='https://untrusted.example'))
        self.assertEqual(response['statusCode'], 403)
        self.assertEqual(records[0]['ChatErrors'], 0)
        limit.assert_not_called()
        compose.assert_not_called()

    def test_provider_outage_returns_cited_fallback_and_signal(self):
        sentinel = 'PROMPT-SECRET-DO-NOT-LOG'
        with patch.object(m, 'limit'), patch.object(m, 'compose', side_effect=RuntimeError(sentinel)):
            response, records = self.invoke(self.event(message='install ' + sentinel))
        body = json.loads(response['body'])
        self.assertEqual(response['statusCode'], 200)
        self.assertEqual(body['mode'], 'search')
        self.assertTrue(body['citations'])
        self.assertIn('search result', body['answer'])
        self.assertEqual(records[0]['ChatFallbacks'], 1)
        self.assertNotIn(sentinel, json.dumps(records))
        self.assertNotIn(sentinel, response['body'])

    def test_rate_limit_is_429_and_never_calls_provider(self):
        error = RuntimeError('sensitive SDK diagnostic')
        error.response = {'Error': {'Code': 'TransactionCanceledException'}}
        with patch.object(m, 'limit', side_effect=error), patch.object(m, 'compose') as compose:
            response, records = self.invoke(self.event())
        self.assertEqual(response['statusCode'], 429)
        self.assertEqual(records[0]['ChatRateLimited'], 1)
        self.assertEqual(records[0]['ChatErrors'], 0)
        compose.assert_not_called()

    def test_storage_outage_is_503_with_no_exception_or_request_logging(self):
        secret = 'sk-test-PRIVATE-EXCEPTION-CONTENT'
        event = self.event(message='install ' + secret)
        event['headers']['authorization'] = secret
        with patch.object(m, 'limit', side_effect=RuntimeError(secret)), patch.object(m, 'compose') as compose:
            response, records = self.invoke(event)
        self.assertEqual(response['statusCode'], 503)
        self.assertNotIn(secret, json.dumps(records))
        self.assertNotIn('192.0.2.44', json.dumps(records))
        self.assertEqual(records[0]['ChatErrors'], 1)
        self.assertNotIn(secret, response['body'])
        compose.assert_not_called()

    def test_unexpected_retrieval_error_is_sanitized_and_measured(self):
        secret = 'private-provider-or-prompt-detail'
        with patch.object(m, 'limit'), patch.object(m, 'retrieve', side_effect=RuntimeError(secret)):
            response, records = self.invoke(self.event())
        self.assertEqual(response['statusCode'], 503)
        self.assertEqual(records[0]['ChatErrors'], 1)
        self.assertNotIn(secret, json.dumps(records) + response['body'])

    def aws(self, salt='invented-unit-test-salt-0123456789abcdef'):
        calls, reads = [], []
        dynamodb = SimpleNamespace(transact_write_items=lambda **kw: calls.append(kw))
        def get_secret_value(SecretId):
            reads.append(SecretId)
            return {'SecretString': salt}
        secrets = SimpleNamespace(get_secret_value=get_secret_value)
        clients = {'dynamodb': dynamodb, 'secretsmanager': secrets}
        return SimpleNamespace(client=lambda service: clients.get(service) or self.fail(service)), calls, reads

    def test_real_limiter_builds_one_atomic_private_transaction(self):
        sdk, calls, reads = self.aws()
        env = {'RATE_TABLE': 'test-table', 'RATE_SALT_SECRET_ARN': 'arn:aws:secretsmanager:us-west-2:000000000000:secret:test-salt'}
        with patch.dict(sys.modules, {'boto3': sdk}), patch.dict(os.environ, env), patch.object(m, 'SALT', None, create=True), patch.object(m.time, 'time', return_value=360000):
            os.environ.pop('RATE_SALT', None)
            m.limit('192.0.2.44')
            m.limit('192.0.2.44')
        self.assertEqual(reads, [env['RATE_SALT_SECRET_ARN']])  # read once, then cached
        self.assertEqual(len(calls), 2)
        updates = [x['Update'] for x in calls[0]['TransactItems']]
        self.assertEqual(len(updates), 3)
        self.assertEqual([u['ExpressionAttributeValues'][':cap']['N'] for u in updates], ['20', '50', '200'])
        anonymous = hashlib.sha256(('invented-unit-test-salt-0123456789abcdef' + '192.0.2.44').encode()).hexdigest()
        self.assertEqual(updates[0]['Key']['pk']['S'], 'ip:' + anonymous + ':100')
        self.assertEqual(updates[1]['Key']['pk']['S'], 'ipday:' + anonymous + ':4')
        self.assertEqual(updates[2]['Key']['pk']['S'], 'global:4')
        for update in updates:
            self.assertEqual(update['ConditionExpression'], 'attribute_not_exists(requests) OR requests < :cap')
            self.assertEqual(update['ExpressionAttributeValues'][':ttl']['N'], '450000')
        self.assertNotIn('192.0.2.44', json.dumps(calls))
        self.assertNotIn('invented-unit-test-salt', json.dumps(calls))

    def test_single_ip_is_capped_per_day_below_global_cap(self):
        # Emulate the table's conditional writes: one IP spreading requests over
        # every hour of a day stays under 20/hour but must stop at 50/day,
        # leaving the rest of the 200/day service budget for other visitors.
        counters = {}
        class Cancelled(Exception):
            response = {'Error': {'Code': 'TransactionCanceledException'}}
        def transact_write_items(TransactItems):
            updates = [x['Update'] for x in TransactItems]
            keys = [u['Key']['pk']['S'] for u in updates]
            if any(counters.get(k, 0) >= int(u['ExpressionAttributeValues'][':cap']['N']) for k, u in zip(keys, updates)):
                raise Cancelled()
            for k in keys:
                counters[k] = counters.get(k, 0) + 1
        sdk, _, _ = self.aws()
        sdk.client('dynamodb').transact_write_items = transact_write_items
        allowed = 0
        env = {'RATE_TABLE': 'test-table', 'RATE_SALT_SECRET_ARN': 'arn:aws:secretsmanager:us-west-2:000000000000:secret:test-salt'}
        with patch.dict(sys.modules, {'boto3': sdk}), patch.dict(os.environ, env), patch.object(m, 'SALT', None, create=True):
            os.environ.pop('RATE_SALT', None)
            for hour in range(24):
                for _ in range(5):
                    with patch.object(m.time, 'time', return_value=86400 * 10 + 3600 * hour):
                        try:
                            m.limit('192.0.2.44')
                            allowed += 1
                        except Cancelled:
                            pass
            with patch.object(m.time, 'time', return_value=86400 * 10 + 3600 * 23):
                m.limit('192.0.2.45')  # another visitor still has budget
        self.assertEqual(allowed, 50)

    def test_plain_env_salt_is_refused(self):
        sdk, calls, reads = self.aws()
        for env in ({'RATE_TABLE': 'test-table', 'RATE_SALT': 'plaintext-salt-in-lambda-environment'},
                    {'RATE_TABLE': 'test-table', 'RATE_SALT': 'plaintext-salt-in-lambda-environment',
                     'RATE_SALT_SECRET_ARN': 'arn:aws:secretsmanager:us-west-2:000000000000:secret:test-salt'},
                    {'RATE_TABLE': 'test-table'}):
            with self.subTest(env=sorted(env)), patch.dict(sys.modules, {'boto3': sdk}), patch.dict(os.environ, env, clear=True), patch.object(m, 'SALT', None, create=True):
                with self.assertRaises(RuntimeError):
                    m.limit('192.0.2.44')
        self.assertEqual(calls, [])

    def test_short_secret_salt_is_refused(self):
        sdk, calls, _ = self.aws(salt='short')
        env = {'RATE_TABLE': 'test-table', 'RATE_SALT_SECRET_ARN': 'arn:aws:secretsmanager:us-west-2:000000000000:secret:test-salt'}
        with patch.dict(sys.modules, {'boto3': sdk}), patch.dict(os.environ, env, clear=True), patch.object(m, 'SALT', None, create=True):
            with self.assertRaises(RuntimeError):
                m.limit('192.0.2.44')
        self.assertEqual(calls, [])

    def test_template_reads_salt_from_generated_secret_not_env(self):
        template = json.loads(Path(__file__).with_name('stack.json').read_text())
        resources = template['Resources']
        self.assertNotIn('RateSalt', template.get('Parameters', {}))
        variables = resources['Function']['Properties']['Environment']['Variables']
        self.assertNotIn('RATE_SALT', variables)
        self.assertEqual(variables['RATE_SALT_SECRET_ARN'], {'Ref': 'RateSaltSecret'})
        secret = resources['RateSaltSecret']
        self.assertEqual(secret['Type'], 'AWS::SecretsManager::Secret')
        self.assertNotIn('SecretString', secret['Properties'])
        self.assertGreaterEqual(secret['Properties']['GenerateSecretString']['PasswordLength'], 32)
        statements = resources['Role']['Properties']['Policies'][0]['PolicyDocument']['Statement']
        reads = [s for s in statements if isinstance(s, dict) and s.get('Action') == ['secretsmanager:GetSecretValue']]
        self.assertEqual([s['Resource'] for s in reads], [{'Ref': 'RateSaltSecret'}])

    def test_packaged_handler_has_same_health_and_monitoring_behavior(self):
        template = json.loads(Path(__file__).with_name('stack.json').read_text())
        namespace = {}
        exec(compile(template['Resources']['Function']['Properties']['Code']['ZipFile'], 'index.py', 'exec'), namespace)
        namespace['limit'] = lambda ip: None
        namespace['compose'] = lambda *args: ('Public answer', 'answer')
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            health = namespace['handler'](self.event(method='GET', path='/healthz'), None)
            answer = namespace['handler'](self.event(), None)
        self.assertEqual(health['statusCode'], 200)
        self.assertEqual(json.loads(answer['body'])['answer'], 'Public answer')
        self.assertEqual(json.loads(output.getvalue())['ChatRequests'], 1)
        resources = template['Resources']
        self.assertEqual(resources['LogGroup']['Properties']['RetentionInDays'], 14)
        statements = resources['Role']['Properties']['Policies'][0]['PolicyDocument']['Statement']
        logging = [s for s in statements if 'logs:PutLogEvents' in s.get('Action', [])]
        self.assertEqual(len(logging), 1)
        self.assertEqual(logging[0]['Resource'], {'Fn::GetAtt': ['LogGroup', 'Arn']})
        alarms = [x['Properties'] for x in resources.values() if x['Type'] == 'AWS::CloudWatch::Alarm']
        self.assertEqual(len(alarms), 5)
        self.assertEqual({x['MetricName'] for x in alarms}, {'ChatErrors', 'ChatFallbacks', 'ChatRateLimited', 'Url5xxCount', 'UrlRequestLatency'})
        for alarm in alarms:
            self.assertIn('README.md#operations', alarm['AlarmDescription'])
            self.assertEqual(alarm['TreatMissingData'], 'notBreaching')


if __name__ == '__main__':
    unittest.main()
