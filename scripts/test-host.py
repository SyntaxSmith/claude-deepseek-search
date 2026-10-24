#!/usr/bin/env python3
"""Exercise a real CPA process with a local fake Search API; no real credentials."""
import argparse
import copy
import http.server
import json
import pathlib
import socket
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request


class SearchAPI(http.server.BaseHTTPRequestHandler):
    calls = []

    def log_message(self, *_):
        pass

    def do_POST(self):
        request = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        assert self.path == '/v1/tools/search'
        assert self.headers['Authorization'] == 'Bearer test-key'
        self.calls.append(request)
        status = 429 if request['query'] == 'rate-limit-test' else 200
        body = {'results': [{'title': 'Search fixture', 'url': 'https://example.com/test',
                             'summary': 'Verified local search content.'}]}
        if status == 429:
            body = {'error': {'message': 'upstream echoed test-key'}}
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def unused_port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


def run(cpa, plugins):
    api = http.server.ThreadingHTTPServer(('127.0.0.1', 0), SearchAPI)
    threading.Thread(target=api.serve_forever, daemon=True).start()
    try:
        with tempfile.TemporaryDirectory(prefix='claude-search-host-test-') as tmp:
            root = pathlib.Path(tmp)
            (root / 'auth').mkdir()
            port = unused_port()
            config = {
                'config-version': 8,
                'server': {'host': '127.0.0.1', 'port': port},
                'management': {'disable-control-panel': True},
                'access': {'api-keys': ['test-frontend-key']},
                'oauth': {'auth-dir': str(root / 'auth')},
                'plugins': {'enabled': True, 'dir': str(plugins), 'configs': {
                    'claude-deepseek-search': {
                        'enabled': True, 'priority': 20, 'api_key': 'test-key',
                        'models': ['GLM-*'], 'count': 5, 'timeout_seconds': 5,
                        'base_url': f'http://127.0.0.1:{api.server_port}',
                    }
                }},
            }
            config_path = root / 'config.yaml'
            config_path.write_text(json.dumps(config))
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

            def request(payload=None):
                path = '/v1/models' if payload is None else '/v1/messages'
                data = None if payload is None else json.dumps(payload).encode()
                req = urllib.request.Request(f'http://127.0.0.1:{port}{path}', data=data,
                    headers={'Authorization': 'Bearer test-frontend-key',
                             'Content-Type': 'application/json', 'anthropic-version': '2023-06-01'})
                try:
                    with opener.open(req, timeout=12) as response:
                        return response.status, response.read().decode()
                except urllib.error.HTTPError as error:
                    return error.code, error.read().decode()

            with (root / 'host.log').open('w') as log:
                proc = subprocess.Popen([str(cpa), '-config', str(config_path)],
                                        cwd=root, stdout=log, stderr=log)
                try:
                    for _ in range(150):
                        if proc.poll() is not None:
                            raise RuntimeError((root / 'host.log').read_text())
                        try:
                            if request()[0] == 200:
                                break
                        except (OSError, urllib.error.URLError):
                            pass
                        time.sleep(0.1)
                    else:
                        raise TimeoutError('CPA did not become ready')

                    base = {'model': 'GLM-test', 'max_tokens': 1024,
                            'tools': [{'type': 'web_search_20250305', 'name': 'web_search', 'max_uses': 1}],
                            'tool_choice': {'type': 'tool', 'name': 'web_search'},
                            'messages': [{'role': 'user', 'content': 'Perform a web search for the query: smoke test'}]}
                    status, body = request(base)
                    assert status == 200, (status, body)
                    message = json.loads(body)
                    assert message['stop_reason'] == 'end_turn'
                    assert any(block['type'] == 'web_search_tool_result' for block in message['content'])
                    assert 'Verified local search content.' in body
                    assert SearchAPI.calls[-1]['count'] == 5
                    print('PASS real CPA: non-streaming search and result count')

                    streamed = copy.deepcopy(base)
                    streamed['stream'] = True
                    status, body = request(streamed)
                    assert status == 200 and 'event: message_stop' in body, (status, body)
                    assert 'Verified local search content.' in body
                    print('PASS real CPA: Claude SSE search')

                    for stream in (False, True):
                        failure = copy.deepcopy(base)
                        failure['stream'] = stream
                        failure['messages'][0]['content'] = 'Perform a web search for the query: rate-limit-test'
                        status, body = request(failure)
                        assert status == 429 and 'test-key' not in body, (status, body)
                    print('PASS real CPA: JSON/SSE HTTP 429 and credential redaction')

                    before = len(SearchAPI.calls)
                    for model in ('gpt-test', 'claude-test', 'deepseek-test', 'Qwen-test'):
                        excluded = copy.deepcopy(base)
                        excluded['model'] = model
                        request(excluded)
                    assert len(SearchAPI.calls) == before
                    print('PASS real CPA: native models and non-allowlisted models bypass Search API')
                finally:
                    proc.terminate()
                    try:
                        proc.wait(timeout=8)
                    except subprocess.TimeoutExpired:
                        proc.kill()
                        proc.wait()
    finally:
        api.shutdown()
        api.server_close()


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--cpa', type=pathlib.Path, required=True)
    parser.add_argument('--plugins', type=pathlib.Path, default=pathlib.Path('build'))
    args = parser.parse_args()
    run(args.cpa.resolve(), args.plugins.resolve())
