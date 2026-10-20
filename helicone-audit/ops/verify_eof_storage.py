import hashlib
import json
import subprocess
from pathlib import Path

root = Path('/opt/cc2cx-audit-validation')
credentials = json.loads((root / 'dashboard-credentials.json').read_text())
results = []
for filename in sorted(root.glob('eof-public-*.json')):
    for test in json.loads(filename.read_text()):
        identity = test.get('audit_id')
        if not identity:
            continue
        path = f"localminio/request-response-storage/organizations/{credentials['organization_id']}/requests/{identity}/raw_request_response_body"
        raw = subprocess.run(['docker', 'exec', 'cc2cx-helicone-audit', 'mc', 'cat', path], capture_output=True, check=True).stdout
        record = json.loads(raw)
        assert hashlib.sha256(record['response'].encode()).hexdigest() == test['sha256'], '审计响应正文与客户端不一致'
        assert b'EOF_OK' in record['request'].encode(), '审计请求缺少验证标记'
        sql = f"SELECT status,properties FROM request_response_rmt WHERE request_id='{identity}' LIMIT 1 FORMAT JSONEachRow"
        rows = subprocess.run(['docker', 'exec', 'cc2cx-helicone-audit', 'clickhouse-client', '--query', sql], capture_output=True, text=True, check=True).stdout.splitlines()
        assert rows, '缺少审计元数据'
        metadata = json.loads(rows[0])
        properties = metadata['properties']
        assert str(metadata['status']) == '200', '审计状态码不匹配'
        assert properties['helicone-property-sub2api-api-key-id'] == ('83' if test['group_id'] == 36 else '55'), '审计密钥归属不匹配'
        results.append({'group_id': test['group_id'], 'audit_id': identity, 'stream': test['stream'], 'path': test['path'], 'body_matches': True, 'identity_matches': True})
assert len(results) >= 8, '完整生产验证记录少于八条'
(root / 'eof-storage-verification.json').write_text(json.dumps(results, indent=2))
print(json.dumps({'verified_records': len(results), 'all_bodies_and_identities_match': True}))
