import argparse
import hashlib
import gzip
import http.client
import json
import subprocess
import time
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument('--target', choices=['old', 'candidate', 'direct', 'public'], required=True)
parser.add_argument('--stream', action='store_true')
parser.add_argument('--responses', action='store_true')
parser.add_argument('--gzip', action='store_true')
parser.add_argument('--model', default='gpt-6-astra')
args = parser.parse_args()
root = Path('/opt/cc2cx-audit-validation')
secret = json.loads((root / 'dashboard-credentials.json').read_text())['api_key']
sql = "SELECT id,user_id,group_id,key FROM api_keys WHERE id IN (55,83) AND status='active' AND deleted_at IS NULL ORDER BY group_id;"
rows = subprocess.run(['docker', 'exec', 'sub2api-postgres', 'psql', '-U', 'sub2api', '-d', 'sub2api', '-Atc', sql], capture_output=True, text=True, check=True).stdout.splitlines()
records = []
for row in rows:
    key_id, user_id, group_id, key = row.split('|', 3)
    endpoint = '/v1/responses' if args.responses else '/v1/chat/completions'
    payload = {'model': args.model, 'stream': args.stream}
    prompt = 'Reply only with EOF_OK.'
    if args.responses:
        payload.update(input=prompt, max_output_tokens=128, store=False)
    else:
        payload.update(messages=[{'role': 'user', 'content': prompt}], max_completion_tokens=64)
    headers = {'Authorization': 'Bearer ' + key, 'Content-Type': 'application/json', 'User-Agent': 'cc2-eof-regression/1.0'}
    if args.gzip:
        headers['Accept-Encoding'] = 'gzip'
    if args.target in ('old', 'candidate'):
        port = 18486 if args.target == 'old' else 18488
        connection = http.client.HTTPConnection('127.0.0.1', port, timeout=90)
        endpoint = '/openai' + endpoint
        headers.update({'helicone-api-key': secret, 'helicone-property-sub2api-user-id': user_id, 'helicone-property-sub2api-api-key-id': key_id})
    elif args.target == 'direct':
        connection = http.client.HTTPConnection('127.0.0.1', 8080, timeout=90)
    else:
        connection = http.client.HTTPSConnection('api.cc2.cx', timeout=90)
    start = time.monotonic()
    result = {'target': args.target, 'group_id': int(group_id), 'model': args.model, 'stream': args.stream, 'gzip': args.gzip, 'path': endpoint}
    try:
        connection.request('POST', endpoint, json.dumps(payload), headers)
        response = connection.getresponse()
        result.update(status=response.status, audit_id=response.getheader('helicone-id'))
        body = response.read()
        result['content_encoding'] = response.getheader('Content-Encoding')
        if result['content_encoding'] == 'gzip':
            body = gzip.decompress(body)
        if args.stream:
            complete = b'[DONE]' in body or b'response.completed' in body
            fragments = []
            for line in body.decode().splitlines():
                if not line.startswith('data: ') or line == 'data: [DONE]':
                    continue
                try:
                    event = json.loads(line[6:])
                except json.JSONDecodeError:
                    continue
                if event.get('type') == 'response.output_text.delta':
                    fragments.append(event.get('delta', ''))
                for choice in event.get('choices', []):
                    fragments.append(choice.get('delta', {}).get('content') or '')
            marker_present = 'EOF_OK' in ''.join(fragments)
        else:
            parsed = json.loads(body)
            complete = bool(parsed.get('choices') or parsed.get('output'))
            marker_present = b'EOF_OK' in body
        result.update(complete=complete, marker_present=marker_present, body_bytes=len(body), sha256=hashlib.sha256(body).hexdigest())
    except Exception as error:
        result.update(error_type=type(error).__name__)
    finally:
        connection.close()
    result['seconds'] = round(time.monotonic() - start, 3)
    records.append(result)
    print(json.dumps(result), flush=True)
output = root / ('eof-' + args.target + '-' + ('responses' if args.responses else 'chat') + '-' + ('stream' if args.stream else 'json') + '-' + ('gzip' if args.gzip else 'identity') + '-' + args.model + '.json')
output.write_text(json.dumps(records, indent=2))
