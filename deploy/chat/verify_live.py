#!/usr/bin/env python3
"""Bounded public-surface checks; no credentials or visitor content accepted."""
import argparse
import json
from pathlib import Path
import re
import urllib.error
import urllib.request

BASE = 'https://serenity.sire.run'


def request(url, method='GET', headers=None, data=None):
    req = urllib.request.Request(url, method=method, headers=headers or {}, data=data)
    try:
        response = urllib.request.urlopen(req, timeout=35)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        return response.status, response.headers, response.read()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--ask', action='store_true', help='Consume one public chat request to verify a cited answer')
    args = parser.parse_args()
    config = (Path(__file__).resolve().parents[2] / 'site/assets/chat-config.js').read_text()
    urls = re.findall(r'https://[a-z0-9]+\.lambda-url\.[a-z0-9-]+\.on\.aws/', config)
    if len(urls) != 1:
        raise SystemExit('Expected one public Lambda URL in chat-config.js')
    url = urls[0]
    status, _, body = request(url + 'healthz')
    health = json.loads(body)
    assert status == 200 and health.get('status') == 'ok' and health.get('documents', 0) > 0, 'Health check failed'
    print('PASS health: 200, nonempty public corpus')
    status, headers, _ = request(url, 'OPTIONS', {'Origin': BASE, 'Access-Control-Request-Method': 'POST', 'Access-Control-Request-Headers': 'content-type'})
    assert status in (200, 204) and headers.get('Access-Control-Allow-Origin') == BASE, 'Allowed-origin preflight failed'
    assert 'POST' in headers.get('Access-Control-Allow-Methods', ''), 'POST missing from preflight'
    print('PASS allowed-origin preflight')
    status, _, body = request(url, 'POST', {'Origin': 'https://untrusted.example', 'Content-Type': 'application/json'}, b'{"message":"How do I install Serenity?"}')
    assert status == 403 and json.loads(body).get('error') == 'Origin is not allowed.', 'Foreign origin was not rejected'
    print('PASS rejected origin: 403')
    if args.ask:
        status, headers, body = request(url, 'POST', {'Origin': BASE, 'Content-Type': 'application/json'}, b'{"message":"How do I install Serenity from source?","history":[]}')
        answer = json.loads(body)
        assert status == 200 and headers.get('Access-Control-Allow-Origin') == BASE, 'Allowed-origin POST failed'
        assert answer.get('mode') in ('answer', 'search') and answer.get('answer') and answer.get('citations'), 'Missing answer/citations'
        assert all(c['url'].startswith(BASE + '/') for c in answer['citations']), 'Unexpected citation origin'
        print('PASS public cited response: mode=' + answer['mode'])
    print('Executed ' + ('4' if args.ask else '3') + ' live checks; no sensitive payloads recorded')


if __name__ == '__main__':
    main()
