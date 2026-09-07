import importlib.util
import json
from pathlib import Path
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('chat_handler',Path(__file__).with_name('handler.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
m.CORPUS=json.loads((Path(__file__).resolve().parents[2]/'site/content.json').read_text())

class ChatTests(unittest.TestCase):
    def event(self,message='How do I install Serenity?',**changes):
        event={'headers':{'origin':m.BASE},'requestContext':{'http':{'method':'POST','sourceIp':'192.0.2.1'}},'body':json.dumps({'message':message})}
        event.update(changes);return event
    def test_install_retrieves_install_guide(self):
        self.assertTrue(any('/get-started/' in x['url'] for x in m.retrieve('install Serenity using Go')))
    def test_privacy_retrieves_ownership(self):
        self.assertTrue(any('/ownership/' in x['url'] or '/privacy/' in x['url'] for x in m.retrieve('data privacy model provider credentials backup')))
    def test_unknown_topic_does_not_invent(self):
        answer=m.fallback(m.retrieve('xylophonic quasar narwhalz'))
        self.assertIn('couldn’t find',answer)
    def test_foreign_origin_rejected_before_rate_or_provider(self):
        with patch.object(m,'limit') as limit:
            self.assertEqual(m.handler(self.event(headers={'origin':'https://example.com'}),None)['statusCode'],403)
            limit.assert_not_called()
    def test_invalid_input(self):
        for body in ['[]','null','{','{"message":22}','{"message":""}',json.dumps({'message':'a'*1001}),json.dumps({'message':'valid','history':'bad'})]:
            self.assertEqual(m.handler(self.event(body=body),None)['statusCode'],400)
    def test_oversized_input(self):
        self.assertEqual(m.handler(self.event(body='x'*18001),None)['statusCode'],413)
    def test_rate_failure_never_calls_provider(self):
        with patch.object(m,'limit',side_effect=RuntimeError()),patch.object(m,'compose') as compose:
            self.assertEqual(m.handler(self.event(),None)['statusCode'],503);compose.assert_not_called()
    def test_model_outage_yields_cited_search_result(self):
        with patch.object(m,'limit'),patch.object(m,'compose',side_effect=RuntimeError()):
            data=json.loads(m.handler(self.event(),None)['body']);self.assertEqual(data['mode'],'search');self.assertIn(m.BASE,data['answer']);self.assertTrue(data['citations'])
    def test_no_key_is_honest_search(self):
        with patch.object(m,'get_key',return_value=None):
            answer,mode=m.compose('install',m.retrieve('install'),[]);self.assertEqual(mode,'search');self.assertIn('search result',answer)
    def test_generated_template_compiles_and_corpus_matches(self):
        template=json.loads(Path(__file__).with_name('stack.json').read_text())
        ns={};exec(compile(template['Resources']['Function']['Properties']['Code']['ZipFile'],'index.py','exec'),ns)
        self.assertEqual(ns['CORPUS'],m.CORPUS)

if __name__=='__main__':unittest.main()
