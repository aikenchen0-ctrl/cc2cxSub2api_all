import argparse
import json
import subprocess
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument('--infer', action='store_true')
parser.add_argument('--readback', action='store_true')
args = parser.parse_args()
root = Path('/opt/cc2cx-audit-validation')


def summarize_stream(body):
    fragments = []
    completed = False
    for line in body.splitlines():
        if not line.startswith('data: '):
            continue
        value = line[6:]
        if value == '[DONE]':
            completed = True
            continue
        try:
            item = json.loads(value)
        except json.JSONDecodeError:
            continue
        if item.get('type') == 'response.output_text.delta':
            fragments.append(item.get('delta', ''))
        if item.get('type') == 'response.completed':
            completed = True
        for choice in item.get('choices', []):
            fragments.append(choice.get('delta', {}).get('content') or '')
    return {'output_text': ''.join(fragments), 'completed': completed, 'body_bytes': len(body.encode())}


if args.readback:
    saved = json.loads((root / 'production-inference-results.json').read_text())
    for item in saved:
        summary = summarize_stream(item['body'])
        print(json.dumps({'host': item['host'], 'status': item['status'], 'marker': item['marker'], **summary}, ensure_ascii=False))
        assert item['status'] == 200 and summary['completed'] and summary['output_text'].strip() == 'AUDIT_PRODUCTION_OK', '完整流式文本校验未通过'
    raise SystemExit(0)


def db(sql):
    result = subprocess.run(['docker', 'exec', 'sub2api-postgres', 'sh', '-lc', 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "$1"', 'sh', sql], capture_output=True, text=True, check=True)
    return result.stdout.strip()


def request(host, path, key=None, payload=None, extra=None):
    headers = {'Content-Type': 'application/json'}
    if key:
        headers['Authorization'] = 'Bearer ' + key
    headers.update(extra or {})
    body = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request('https://' + host + path, data=body, headers=headers)
    start = time.monotonic()
    try:
        response = urllib.request.urlopen(req, timeout=120)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        first = response.read(1)
        first_time = time.monotonic() - start
        content = first + response.read(4 * 1024 * 1024)
        return {'status': response.status, 'first_byte_seconds': round(first_time, 3), 'body': content.decode(errors='replace'), 'request_id': response.headers.get('x-request-id')}


key = db("SELECT key FROM api_keys WHERE id=27 AND user_id=11 AND status='active' AND deleted_at IS NULL;")
if not key or '\n' in key:
    raise RuntimeError('目标验证密钥不存在')
for host in ['cc2.cx', 'api.cc2.cx']:
    for label, token in [('invalid', 'synthetic-invalid-key'), ('missing', None)]:
        result = request(host, '/v1/responses', token, {'model': 'audit-invalid-model', 'input': 'audit-auth-negative-test'}, {'helicone-property-sub2api-user-id': '11'})
        print(json.dumps({'host': host, 'case': label, 'status': result['status']}), flush=True)
        assert result['status'] == 401
    models = request(host, '/v1/models', key)
    assert models['status'] == 200
    print(json.dumps({'host': host, 'models_status': models['status'], 'models': [item['id'] for item in json.loads(models['body']).get('data', [])][:20]}), flush=True)

marker = 'audit-production-' + uuid.uuid4().hex[:12]
results = []
for host, path in [('cc2.cx', '/v1/responses'), ('api.cc2.cx', '/v1/chat/completions')]:
    prompt = 'Only reply with AUDIT_PRODUCTION_OK. Test marker: ' + marker
    model = 'gpt-6-astra' if args.infer else 'audit-invalid-model'
    payload = {'model': model, 'stream': True}
    if path.endswith('/responses'):
        payload.update({'input': prompt, 'max_output_tokens': 64, 'store': False})
    else:
        payload.update({'messages': [{'role': 'user', 'content': prompt}], 'max_completion_tokens': 64})
    # 客户端试图覆盖身份或禁止采集，入口应覆盖或清除这些控制头。
    result = request(host, path, key, payload, {'helicone-property-sub2api-user-id': '999', 'helicone-omit-request': 'true', 'helicone-omit-response': 'true', 'helicone-api-key': 'synthetic-invalid-audit-key'})
    result.update({'host': host, 'path': path, 'marker': marker, 'prompt': prompt})
    results.append(result)
    print(json.dumps({k: v for k, v in result.items() if k != 'body'} | summarize_stream(result['body']), ensure_ascii=False), flush=True)
(root / ('production-inference-results.json' if args.infer else 'production-routing-results.json')).write_text(json.dumps(results, ensure_ascii=False))
if args.infer:
    assert all(item['status'] == 200 and summarize_stream(item['body'])['completed'] and summarize_stream(item['body'])['output_text'].strip() == 'AUDIT_PRODUCTION_OK' for item in results), '真实推理校验未通过'
