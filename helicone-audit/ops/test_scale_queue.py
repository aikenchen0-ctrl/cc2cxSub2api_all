import concurrent.futures
import hashlib
import http.client
import json
import os
import subprocess
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

ROOT = Path('/opt/cc2cx-audit-validation')
QUEUE = ROOT / 'queue-test'
COUNT = int(os.environ.get('CC2_TEST_CONCURRENCY', '300'))
BASELINE = '--baseline' in sys.argv
FAULT = '--fault' in sys.argv
BURST = '--burst' in sys.argv
credentials = json.loads((ROOT / 'dashboard-credentials.json').read_text())
payload = json.dumps({'model': 'gpt-4o-mini', 'stream': True, 'input': 'scale-queue-' + 'x' * 4096}).encode()
frame = b'data: ' + json.dumps({'text': 'x' * 16000}).encode() + b'\n\n'
tail = b'data: [DONE]\n\n'
expected = hashlib.sha256(frame * 16 + tail).hexdigest()
active = 0
peak = 0
lock = threading.Lock()
arrived = threading.Event()
stop = threading.Event()
memory = []


class Server(ThreadingHTTPServer):
    request_queue_size = 1024


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        global active, peak
        data = self.rfile.read(int(self.headers['Content-Length']))
        assert data == payload, '业务请求正文被改变'
        assert self.headers.get('Authorization') == 'Bearer synthetic-scale-user', '业务认证被改变'
        assert not self.headers.get('helicone-api-key'), '审计凭证泄露至上游'
        with lock:
            active += 1
            peak = max(peak, active)
            if active == COUNT:
                arrived.set()
        try:
            if not arrived.wait(20):
                self.send_error(503)
                return
            self.send_response(200)
            self.send_header('Content-Type', 'text/event-stream')
            self.end_headers()
            for _ in range(16):
                self.wfile.write(frame)
                self.wfile.flush()
                if not BURST:
                    time.sleep(0.08)
            self.wfile.write(tail)
        finally:
            with lock:
                active -= 1


def sample():
    while not stop.wait(0.5):
        result = subprocess.run(['docker', 'exec', 'cc2cx-audit-scale-test', 'cat', '/sys/fs/cgroup/memory.current'], capture_output=True, text=True)
        if result.returncode == 0:
            memory.append(int(result.stdout))


def request(index):
    connection = http.client.HTTPConnection('127.0.0.1', 18483 if BASELINE else 18482, timeout=40)
    begin = time.monotonic()
    headers = {
        'Authorization': 'Bearer synthetic-scale-user', 'Content-Type': 'application/json',
        'helicone-api-key': credentials['api_key'], 'helicone-property-sub2api-user-id': 'scale-validation',
        'helicone-property-audit-source': 'scale-validation'}
    if BASELINE:
        headers.pop('helicone-api-key')
    connection.request('POST', '/openai/v1/chat/completions', body=payload, headers=headers)
    response = connection.getresponse()
    first = response.read(1)
    first_time = time.monotonic() - begin
    digest = hashlib.sha256(first)
    while chunk := response.read(65536):
        digest.update(chunk)
    assert response.status == 200 and digest.hexdigest() == expected, '业务响应不完整或失败'
    result = {'request_id': response.getheader('helicone-id'), 'first_byte_seconds': first_time}
    if not BASELINE:
        assert result['request_id'], '正常并发下未分配审计采集'
    connection.close()
    return result


server = Server(('127.0.0.1', 18483), Handler)
threading.Thread(target=server.serve_forever, daemon=True).start()
threading.Thread(target=sample, daemon=True).start()
try:
    subprocess.run(['python3', str(ROOT / 'audit_queue_worker.py'), '--root', str(QUEUE), '--guard-only'], check=True, stdout=subprocess.DEVNULL)
    started = time.monotonic()
    with concurrent.futures.ThreadPoolExecutor(max_workers=COUNT) as pool:
        results = list(pool.map(request, range(COUNT)))
    business_seconds = time.monotonic() - started
    assert peak == COUNT, '业务并发仍受审计限制'
    if BASELINE or FAULT:
        times = sorted(row['first_byte_seconds'] for row in results)
        if FAULT:
            time.sleep(2)
            assert all(not (QUEUE / (row['request_id'] + '.ready')).exists() for row in results), '只读磁盘故障未生效'
        report = {'requests': COUNT, 'peak_upstream_concurrency': peak, 'business_seconds': round(business_seconds, 3),
                  'first_byte_p95_seconds': round(times[int(COUNT * .95) - 1], 3), 'first_byte_max_seconds': round(times[-1], 3),
                  'all_responses_match': True, 'mode': 'baseline' if BASELINE else 'read_only_spool'}
        filename = 'scale-baseline-results.json' if BASELINE else 'scale-fault-results.json'
        (ROOT / filename).write_text(json.dumps(report))
        print(json.dumps(report), flush=True)
        raise SystemExit(0)
    # 此时上传进程尚未启动，所有完整记录必须先进入持久队列。
    for _ in range(100):
        if all((QUEUE / (row['request_id'] + '.ready')).exists() for row in results):
            break
        time.sleep(0.1)
    for row in results:
        identity = row['request_id']
        assert (QUEUE / (identity + '.ready')).exists(), '正常并发下缺少完整审计元数据'
        record = json.loads((QUEUE / (identity + '.body')).read_bytes())
        assert record['request'].encode() == payload, '队列中的请求正文不一致'
        assert hashlib.sha256(record['response'].encode()).hexdigest() == expected, '队列中的响应正文不一致'
    queued_before_upload = len(list(QUEUE.glob('*.ready')))
    # 重启代理后再上传，证明持久队列不依赖原代理进程的内存。
    subprocess.run(['docker', 'restart', 'cc2cx-audit-scale-test'], check=True, stdout=subprocess.DEVNULL)
    assert all((QUEUE / (row['request_id'] + '.ready')).exists() for row in results), '代理重启导致队列丢失'
    uploaded = subprocess.run(['python3', str(ROOT / 'audit_queue_worker.py'), '--root', str(QUEUE), '--once'], capture_output=True, text=True, timeout=600)
    assert uploaded.returncode == 0, '独立上传进程失败'
    worker = json.loads(uploaded.stdout)
    assert worker['uploaded'] >= COUNT and worker['failed_attempts'] == 0, '未完成所有队列上传'
    for row in results:
        identity = row['request_id']
        path = f"localminio/request-response-storage/organizations/{credentials['organization_id']}/requests/{identity}/raw_request_response_body"
        raw = subprocess.run(['docker', 'exec', 'cc2cx-helicone-audit', 'mc', 'cat', path], capture_output=True, check=True)
        record = json.loads(raw.stdout)
        assert record['request'].encode() == payload, '对象存储请求正文不一致'
        assert hashlib.sha256(record['response'].encode()).hexdigest() == expected, '对象存储响应正文不一致'
    times = sorted(row['first_byte_seconds'] for row in results)
    report = {'requests': COUNT, 'peak_upstream_concurrency': peak, 'business_seconds': round(business_seconds, 3),
              'first_byte_p95_seconds': round(times[int(COUNT * .95) - 1], 3), 'first_byte_max_seconds': round(times[-1], 3),
              'peak_memory_mib': round(max(memory) / 1024**2, 2), 'queued_before_upload': queued_before_upload,
              'uploaded_and_verified': COUNT, 'worker_failed_attempts': worker['failed_attempts'], 'request_ids': [row['request_id'] for row in results]}
    (ROOT / ('scale-burst-results.json' if BURST else 'scale-queue-results.json')).write_text(json.dumps(report))
    print(json.dumps({key: value for key, value in report.items() if key != 'request_ids'}), flush=True)
finally:
    stop.set()
    server.shutdown()
