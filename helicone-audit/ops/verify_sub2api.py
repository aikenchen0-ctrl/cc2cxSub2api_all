import json
import subprocess
import urllib.error
import urllib.request
from pathlib import Path

sql = "SELECT key FROM api_keys WHERE user_id=11 AND deleted_at IS NULL AND status='active' AND (expires_at IS NULL OR expires_at>now()) ORDER BY id DESC LIMIT 1;"
command = ['docker','exec','sub2api-postgres','sh','-lc','psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "$1"','sh',sql]
key = subprocess.check_output(command,text=True).strip()
if not key or '\n' in key:
    raise RuntimeError('未找到可用于只读验证的单个有效密钥。')
audit_key = json.loads(Path('/opt/cc2cx-audit-validation/dashboard-credentials.json').read_text())['api_key']


def read_models(base, token, audit=False):
    headers = {}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    if audit:
        headers['helicone-api-key'] = audit_key
    request = urllib.request.Request(base + '/v1/models',headers=headers)
    try:
        with urllib.request.urlopen(request,timeout=30) as response:
            return response.status,json.load(response)
    except urllib.error.HTTPError as error:
        return error.code,None


for label,token in [('existing',key),('invalid','synthetic-invalid-key'),('missing',None)]:
    direct = read_models('http://127.0.0.1:8080',token)
    proxied = read_models('http://127.0.0.1:18480/openai',token,True)
    print(json.dumps({'case':label,'direct_status':direct[0],'proxy_status':proxied[0],'same_body':direct[1]==proxied[1]}))
    assert direct == proxied, '代理前后的只读接口结果不一致'
    if label == 'existing':
        assert direct[0] == 200, '现有密钥未通过只读认证验证'
    else:
        assert direct[0] == 401, '缺失或错误的密钥没有被拒绝'
