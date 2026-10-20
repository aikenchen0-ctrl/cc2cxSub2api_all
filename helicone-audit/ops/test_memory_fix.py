import concurrent.futures
import hashlib
import http.client
import json
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

root = Path('/opt/cc2cx-audit-validation')
credentials = json.loads((root / 'dashboard-credentials.json').read_text())
name = 'cc2cx-audit-memory-test'
payload = json.dumps({'model': 'gpt-4o-mini', 'stream': True, 'input': 'memory-validation-' + 'x' * (1024 * 1024)}, ensure_ascii=False).encode()
frame = ('data: ' + json.dumps({'choices': [{'delta': {'content': 'text-"-\\-\n-\u4e2d\u6587-' + 'x' * 65000}}]}, ensure_ascii=False) + '\n\n').encode()
count = 128
tail = b'data: [DONE]\n\n'
expected = hashlib.sha256(frame * count + tail).hexdigest()
active = 0
peak = 0
lock = threading.Lock()


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        global active, peak
        body = self.rfile.read(int(self.headers['Content-Length']))
        assert body == payload, '请求正文被修改'
        assert self.headers.get('Authorization') == 'Bearer synthetic-memory-user', '认证头被修改'
        assert not self.headers.get('helicone-api-key'), '审计密钥泄露至上游'
        with lock:
            active += 1
            peak = max(peak, active)
        try:
            self.send_response(200)
            self.send_header('Content-Type', 'text/event-stream')
            self.end_headers()
            for _ in range(count):
                self.wfile.write(frame)
                self.wfile.flush()
                time.sleep(0.015)
            self.wfile.write(tail)
        finally:
            with lock:
                active -= 1


server = ThreadingHTTPServer(('127.0.0.1', 18483), Handler)
threading.Thread(target=server.serve_forever, daemon=True).start()
samples = []
anonymous_samples = []
stop = threading.Event()


def sample_memory():
    while not stop.wait(1):
        result = subprocess.run(['docker', 'exec', name, 'cat', '/sys/fs/cgroup/memory.current', '/sys/fs/cgroup/memory.stat'], capture_output=True, text=True)
        if result.returncode == 0:
            lines = result.stdout.splitlines()
            samples.append(int(lines[0]))
            counters = dict(line.split() for line in lines[1:] if len(line.split()) == 2)
            anonymous_samples.append(int(counters['anon']))


def request(index):
    conn = http.client.HTTPConnection('127.0.0.1', 18482, timeout=120)
    start = time.monotonic()
    conn.request('POST', '/openai/v1/chat/completions', body=payload, headers={
        'Content-Type': 'application/json', 'Authorization': 'Bearer synthetic-memory-user',
        'helicone-api-key': credentials['api_key'], 'helicone-property-sub2api-user-id': 'memory-validation',
        'helicone-property-audit-source': 'memory-validation'})
    response = conn.getresponse()
    first = response.read(1)
    first_time = time.monotonic() - start
    digest = hashlib.sha256(first)
    while chunk := response.read(65536):
        digest.update(chunk)
    assert response.status == 200 and digest.hexdigest() == expected, '流式响应不完整'
    request_id = response.getheader('helicone-id')
    conn.close()
    return {'request_id': request_id, 'first_byte_seconds': round(first_time, 3), 'response_matches': True}


threading.Thread(target=sample_memory, daemon=True).start()
try:
    with concurrent.futures.ThreadPoolExecutor(max_workers=16) as pool:
        results = list(pool.map(request, range(16)))
    for result in results:
        path = f"localminio/request-response-storage/organizations/{credentials['organization_id']}/requests/{result['request_id']}/raw_request_response_body"
        for _ in range(60):
            raw = subprocess.run(['docker', 'exec', 'cc2cx-helicone-audit', 'mc', 'cat', path], capture_output=True)
            if raw.returncode == 0:
                break
            time.sleep(1)
        assert raw.returncode == 0, '审计正文未成功上传'
        record = json.loads(raw.stdout)
        assert record['request'].encode() == payload, '存储提示词不完整'
        assert hashlib.sha256(record['response'].encode()).hexdigest() == expected, '存储流式正文不完整'
        result['stored_body_matches'] = True
    assert peak <= 8, '并发许可没有生效'
    status = json.loads(subprocess.check_output(['docker', 'inspect', name]))[0]
    assert status['RestartCount'] == 0 and not status['State']['OOMKilled'], '测试发生内存异常'
    report = {'requests': len(results), 'response_bytes_each': len(frame) * count + len(tail),
              'peak_upstream_concurrency': peak, 'peak_cgroup_memory_mib': round(max(samples) / 1024**2, 2),
              'peak_anonymous_memory_mib': round(max(anonymous_samples) / 1024**2, 2),
              'restarts': status['RestartCount'], 'results': results}
    (root / 'memory-test-results.json').write_text(json.dumps(report))
    print(json.dumps(report), flush=True)
finally:
    stop.set()
    server.shutdown()
