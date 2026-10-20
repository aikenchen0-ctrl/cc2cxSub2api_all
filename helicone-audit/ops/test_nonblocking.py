import concurrent.futures
import hashlib
import http.client
import json
import subprocess
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

root = Path('/opt/cc2cx-audit-validation')
fault_mode = '--expect-no-audit' in sys.argv
credentials = json.loads((root / 'dashboard-credentials.json').read_text())
payload = json.dumps({'model': 'gpt-4o-mini', 'input': 'nonblocking-' + 'x' * 1024, 'stream': True}).encode()
frame = b'data: ' + json.dumps({'text': 'x' * 65000}).encode() + b'\n\n'
tail = b'data: [DONE]\n\n'
expected = hashlib.sha256(frame * 128 + tail).hexdigest()
lock = threading.Lock()
active = 0
peak = 0
arrived = threading.Event()


class Server(ThreadingHTTPServer):
    request_queue_size = 64


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        global active, peak
        data = self.rfile.read(int(self.headers['Content-Length']))
        assert data == payload, '上游请求正文被改变'
        assert self.headers.get('Authorization') == 'Bearer synthetic-nonblocking-user', '上游认证被改变'
        assert not self.headers.get('helicone-api-key'), '审计密钥不应发送至上游'
        with lock:
            active += 1
            peak = max(peak, active)
            if active >= 16:
                arrived.set()
        try:
            # 所有请求必须在任一响应结束前进入上游，直接检出业务八并发限制。
            if not arrived.wait(8):
                self.send_error(503)
                return
            self.send_response(200)
            self.send_header('Content-Type', 'text/event-stream')
            self.end_headers()
            for _ in range(128):
                self.wfile.write(frame)
                self.wfile.flush()
                time.sleep(0.02)
            self.wfile.write(tail)
        finally:
            with lock:
                active -= 1


server = Server(('127.0.0.1', 18483), Handler)
threading.Thread(target=server.serve_forever, daemon=True).start()


def request(index):
    connection = http.client.HTTPConnection('127.0.0.1', 18482, timeout=30)
    begin = time.monotonic()
    connection.request('POST', '/openai/v1/chat/completions', body=payload, headers={
        'Authorization': 'Bearer synthetic-nonblocking-user', 'Content-Type': 'application/json',
        'helicone-api-key': credentials['api_key'], 'helicone-property-sub2api-user-id': 'nonblocking-validation',
        'helicone-property-audit-source': 'nonblocking-validation'})
    response = connection.getresponse()
    first = response.read(1)
    first_time = time.monotonic() - begin
    digest = hashlib.sha256(first)
    while chunk := response.read(65536):
        digest.update(chunk)
    assert response.status == 200 and digest.hexdigest() == expected, '业务响应不完整或状态异常'
    result = {'first_byte_seconds': round(first_time, 3), 'request_id': response.getheader('helicone-id'), 'response_matches': True}
    connection.close()
    return result


try:
    with concurrent.futures.ThreadPoolExecutor(max_workers=16) as pool:
        results = list(pool.map(request, range(16)))
    assert peak == 16, '业务仍受审计并发名额限制'
    assert max(row['first_byte_seconds'] for row in results) < 5, '出现明显审计等待'
    captured = [row for row in results if row['request_id']]
    if not fault_mode:
        assert len(captured) <= 8, '旁路审计并发未受限'
    complete_records = 0
    for row in captured:
        path = f"localminio/request-response-storage/organizations/{credentials['organization_id']}/requests/{row['request_id']}/raw_request_response_body"
        for _ in range(1 if fault_mode else 15):
            raw = subprocess.run(['docker', 'exec', 'cc2cx-helicone-audit', 'mc', 'cat', path], capture_output=True)
            if raw.returncode == 0:
                record = json.loads(raw.stdout)
                assert record['request'].encode() == payload, '已保存提示词不一致'
                assert hashlib.sha256(record['response'].encode()).hexdigest() == expected, '不完整审计不应被保存为完整记录'
                complete_records += 1
                break
            time.sleep(1)
    if fault_mode:
        assert complete_records == 0, '只读文件系统故障注入没有生效'
    else:
        assert complete_records > 0, '正常情况下未留下任何完整审计'
    state = json.loads(subprocess.check_output(['docker', 'inspect', 'cc2cx-audit-nonblocking-test']))[0]
    assert state['RestartCount'] == 0 and not state['State']['OOMKilled'], '代理发生内存异常'
    report = {'requests': 16, 'peak_upstream_concurrency': peak, 'complete_audits': complete_records,
              'not_captured': 16 - complete_records, 'max_first_byte_seconds': max(row['first_byte_seconds'] for row in results),
              'restarts': state['RestartCount'], 'results': results}
    destination = 'nonblocking-fault-results.json' if fault_mode else 'nonblocking-test-results.json'
    (root / destination).write_text(json.dumps(report))
    print(json.dumps(report), flush=True)
finally:
    server.shutdown()
