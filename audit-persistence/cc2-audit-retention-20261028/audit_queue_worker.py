import argparse
import concurrent.futures
import fcntl
import http.client
import json
import os
import shutil
import time
import uuid
from datetime import datetime, timedelta, timezone
from pathlib import Path
from urllib.parse import urlsplit
from urllib.request import Request, urlopen

BASE = Path('/opt/cc2cx-audit-validation')
RETENTION_DAYS = max(1, int(os.environ.get('AUDIT_RETENTION_DAYS', '2')))
os.umask(0o077)


def atomic_json(path, value):
    temporary = path.with_suffix(path.suffix + '.new')
    temporary.write_text(json.dumps(value))
    temporary.replace(path)


def guard(root):
    bodies = list(root.glob('*.body'))
    size = 0
    for body in bodies:
        try:
            size += body.stat().st_size
        except FileNotFoundError:
            pass
    free = shutil.disk_usage(root).free
    allowed = free >= 8 * 1024**3 and size < 16 * 1024**3 and len(bodies) < 50000
    marker = root / 'capture-enabled'
    if allowed:
        marker.touch(mode=0o600)
    else:
        marker.unlink(missing_ok=True)
    result = {'time': time.time(), 'capture_enabled': allowed, 'queued_bodies': len(bodies), 'queued_bytes': size, 'free_bytes': free}
    atomic_json(root / 'guard-status.json', result)
    return result


def post(path, data, key):
    request = Request('http://127.0.0.1:18585' + path, data=json.dumps(data).encode(),
                      headers={'Content-Type': 'application/json', 'Authorization': 'Bearer ' + key})
    with urlopen(request, timeout=30) as response:
        raw = response.read(1024 * 1024)
    result = json.loads(raw) if raw else {}
    if result.get('error'):
        raise RuntimeError('审计后端拒绝请求')
    return result


def remove_job(root, identity):
    for suffix in ['ready', 'body', 'uploaded', 'retry']:
        (root / (identity + '.' + suffix)).unlink(missing_ok=True)


def deliver(root, path, key):
    identity = str(uuid.UUID(path.stem))
    if identity != path.stem:
        raise RuntimeError('队列编号格式异常')
    body = root / (identity + '.body')
    metadata = json.loads(path.read_text())
    request = metadata['log']['request']
    if str(request['id']) != identity:
        raise RuntimeError('队列正文与索引编号不匹配')
    created = datetime.fromisoformat(request['requestCreatedAt'].replace('Z', '+00:00'))
    permanent = request.get('properties', {}).get('helicone-property-sub2api-user-id') == '11'
    if not permanent and created < datetime.now(timezone.utc) - timedelta(days=RETENTION_DAYS):
        remove_job(root, identity)
        return 'expired'
    uploaded = root / (identity + '.uploaded')
    if not uploaded.exists():
        signed = post('/v1/router/control-plane/sign-s3-url', {'requestId': identity, 'payloadSize': body.stat().st_size}, key)
        target = urlsplit(signed['data']['url'])
        if target.scheme != 'http' or target.hostname not in ['127.0.0.1', 'localhost']:
            raise RuntimeError('对象上传地址不在本地允许范围')
        connection = http.client.HTTPConnection(target.hostname, target.port, timeout=120)
        connection.blocksize = 65536
        try:
            with body.open('rb') as stream:
                connection.request('PUT', target.path + ('?' + target.query if target.query else ''), body=stream,
                                   headers={'Content-Type': 'application/json', 'Content-Length': str(body.stat().st_size)})
                response = connection.getresponse()
                response.read(65536)
                if not 200 <= response.status < 300:
                    raise RuntimeError('对象存储上传失败')
        finally:
            connection.close()
        uploaded.touch(mode=0o600)
    metadata['authorization'] = key
    post('/v1/log/request', metadata, key)
    remove_job(root, identity)
    return 'uploaded'


def main():
    parser = argparse.ArgumentParser(description='独立上传持久审计队列，不参与业务转发')
    parser.add_argument('--root', default='/opt/cc2cx-audit-validation/queue')
    parser.add_argument('--guard-only', action='store_true')
    parser.add_argument('--once', action='store_true')
    args = parser.parse_args()
    root = Path(args.root)
    root.mkdir(mode=0o700, parents=True, exist_ok=True)
    if args.guard_only:
        print(json.dumps(guard(root)))
        return
    lock = (root / 'worker.lock').open('a')
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    key = json.loads((BASE / 'dashboard-credentials.json').read_text())['api_key']
    totals = {'uploaded': 0, 'expired': 0, 'failed_attempts': 0}
    pending = {}
    next_guard = 0
    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
        while True:
            now = time.time()
            for future in list(pending):
                if not future.done():
                    continue
                path = pending.pop(future)
                try:
                    totals[future.result()] += 1
                except Exception as error:
                    totals['failed_attempts'] += 1
                    retry_path = path.with_suffix('.retry')
                    old = json.loads(retry_path.read_text()) if retry_path.exists() else {}
                    attempts = old.get('attempts', 0) + 1
                    atomic_json(retry_path, {'attempts': attempts, 'next': now + min(300, 2 ** min(attempts, 8)), 'error_type': type(error).__name__})
            if now >= next_guard:
                guard(root)
                atomic_json(root / 'worker-status.json', {'time': now, 'active_uploads': len(pending), **totals})
                next_guard = now + 5
            active_paths = set(pending.values())
            for path in root.glob('*.ready'):
                if len(pending) >= 8:
                    break
                if path in active_paths:
                    continue
                retry = path.with_suffix('.retry')
                if retry.exists() and json.loads(retry.read_text()).get('next', 0) > now:
                    continue
                pending[pool.submit(deliver, root, path, key)] = path
            if args.once and not pending and not any(root.glob('*.ready')):
                atomic_json(root / 'worker-status.json', {'time': time.time(), 'active_uploads': 0, **totals})
                print(json.dumps(totals))
                return
            time.sleep(0.1)


if __name__ == '__main__':
    main()
