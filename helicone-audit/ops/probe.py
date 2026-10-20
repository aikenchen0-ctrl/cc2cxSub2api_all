import http.client
import json
import threading
import time
import argparse
from pathlib import Path
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

observed = []
parser = argparse.ArgumentParser()
parser.add_argument('--mode', choices=['passthrough', 'default'], default='passthrough')
parser.add_argument('--audit', action='store_true')
args = parser.parse_args()
audit_key = json.loads(Path('/opt/cc2cx-audit-validation/dashboard-credentials.json').read_text())['api_key'] if args.audit else 'synthetic-audit-key'


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        body = self.rfile.read(int(self.headers.get('Content-Length', '0')))
        observed.append({'path': self.path, 'authorization': self.headers.get('Authorization'), 'audit_header': self.headers.get('helicone-api-key'), 'body': body.decode()})
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.end_headers()
        for part in [b'data: {"type":"response.created"}\n\n', b'data: {"type":"response.completed"}\n\n']:
            self.wfile.write(part)
            self.wfile.flush()
            time.sleep(1)


server = ThreadingHTTPServer(('127.0.0.1', 18481), Handler)
threading.Thread(target=server.serve_forever, daemon=True).start()
for token in ['synthetic-user-key-a', 'synthetic-user-key-b', None]:
    path = '/openai/v1/responses'
    observed.clear()
    conn = http.client.HTTPConnection('127.0.0.1', 18480, timeout=10)
    payload = json.dumps({'model': 'gpt-4o-mini', 'input': 'audit-probe-synthetic', 'stream': True})
    start = time.monotonic()
    try:
        headers = {'Content-Type': 'application/json', 'helicone-api-key': audit_key}
        if token:
            headers['Authorization'] = 'Bearer ' + token
        conn.request('POST', path, body=payload, headers=headers)
        response = conn.getresponse()
        first = response.read(1)
        first_time = time.monotonic() - start
        body = first + response.read()
        print(json.dumps({'path': path, 'status': response.status, 'first_byte_seconds': round(first_time, 3), 'total_seconds': round(time.monotonic()-start, 3), 'response': body.decode(errors='replace')[:600], 'upstream': observed}, ensure_ascii=False), flush=True)
        expected_auth = ('Bearer ' + token if token else None) if args.mode == 'passthrough' else 'Bearer synthetic-provider-key'
        assert response.status == 200
        assert len(observed) == 1
        assert observed[0]['authorization'] == expected_auth, '上游认证头不符合预期'
        assert observed[0]['body'] == payload, '请求正文发生变化'
        assert observed[0]['audit_header'] is None, '审计认证头泄露到业务上游'
        assert first_time < 0.8, '流式首字节被缓冲'
        assert b'response.completed' in body, '流式正文不完整'
    except Exception as exc:
        print(json.dumps({'path': path, 'error': str(exc), 'upstream': observed}), flush=True)
        raise
    finally:
        conn.close()
server.shutdown()
