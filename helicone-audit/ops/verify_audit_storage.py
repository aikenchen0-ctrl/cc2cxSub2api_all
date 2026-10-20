import hashlib
import json
import subprocess
from pathlib import Path

root = Path('/opt/cc2cx-audit-validation')
credentials = json.loads((root / 'dashboard-credentials.json').read_text())
tests = json.loads((root / 'production-inference-results.json').read_text())
marker = tests[0]['marker']
sql = "SELECT request_id,status,target_url,properties FROM request_response_rmt WHERE properties['helicone-property-audit-source']='nginx-production' AND properties['helicone-property-sub2api-api-key-id']='27' AND status=200 ORDER BY request_created_at DESC LIMIT 20 FORMAT JSONEachRow"
result = subprocess.check_output(['docker', 'exec', 'cc2cx-helicone-audit', 'clickhouse-client', '--query', sql], text=True)
found = []
for line in result.splitlines():
    item = json.loads(line)
    if item['properties'].get('helicone-property-sub2api-user-id') != '11':
        raise RuntimeError('存储身份标记不符合预期')
    path = f"localminio/request-response-storage/organizations/{credentials['organization_id']}/requests/{item['request_id']}/raw_request_response_body"
    raw = subprocess.check_output(['docker', 'exec', 'cc2cx-helicone-audit', 'mc', 'cat', path], text=True)
    obj = json.loads(raw)
    if marker not in obj['request']:
        continue
    test = next(test for test in tests if item['target_url'].endswith(test['path']))
    assert obj['response'] == test['body'], '存储响应与客户端收到的字节正文不一致'
    assert credentials['api_key'] not in raw, '审计凭证出现在正文记录中'
    saved_request = json.loads(obj['request'])
    prompt = saved_request.get('input') or saved_request['messages'][0]['content']
    assert prompt == test['prompt'], '提示词正文发生变化'
    found.append({'request_id': item['request_id'], 'status': item['status'], 'path': test['path'], 'user_id': '11', 'api_key_id': '27', 'prompt_matches': True, 'response_matches': True, 'response_sha256': hashlib.sha256(obj['response'].encode()).hexdigest()})
assert len(found) == 2, '未找到两条完整的真实推理验证记录'
(root / 'production-storage-verification.json').write_text(json.dumps(found, ensure_ascii=False))
print(json.dumps(found, ensure_ascii=False))
