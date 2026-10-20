import json
import subprocess
import uuid
from datetime import datetime, timedelta, timezone

import audit_retention as retention

org = str(uuid.UUID(json.loads((retention.ROOT / 'dashboard-credentials.json').read_text())['organization_id']))
cases = [
    ('expired_other', 11, '12', '12', False),
    ('permanent_ch', 120, '11', '', True),
    ('permanent_pg', 120, '', '11', True),
    ('recent_other', 1, '12', '12', True),
]
fixtures = []
try:
    for name, age, ch_user, pg_user, keep in cases:
        request_id = str(uuid.uuid4())
        response_id = str(uuid.uuid4())
        fixtures.append({'name': name, 'id': request_id, 'keep': keep})
        stamp = datetime.now(timezone.utc) - timedelta(days=age)
        ch_properties = {retention.USER_FIELD: ch_user, 'helicone-property-retention-fixture': 'true'}
        pg_properties = {retention.USER_FIELD: pg_user, 'helicone-property-retention-fixture': 'true'}
        retention.pg(f"INSERT INTO request(id,created_at,path,auth_hash,properties,helicone_org_id) VALUES('{request_id}','{stamp.isoformat()}','/retention-test','synthetic','{json.dumps(pg_properties)}','{org}'); INSERT INTO response(id,created_at,request,helicone_org_id,status) VALUES('{response_id}','{stamp.isoformat()}','{request_id}','{org}',200);")
        record = {'request_id': request_id, 'organization_id': org, 'request_created_at': stamp.strftime('%Y-%m-%d %H:%M:%S'),
                  'response_created_at': stamp.strftime('%Y-%m-%d %H:%M:%S'), 'response_id': response_id,
                  'properties': ch_properties, 'user_id': 'synthetic', 'model': 'cc2-retention-fixture',
                  'provider': 'OPENAI', 'status': 200, 'request_body': 'synthetic retention prompt', 'response_body': 'synthetic retention response'}
        retention.run(['docker', 'exec', '-i', retention.CONTAINER, 'clickhouse-client', '--query', 'INSERT INTO request_response_rmt FORMAT JSONEachRow'], json.dumps(record) + '\n')
        prefix = f'localminio/request-response-storage/organizations/{org}/requests/{request_id}/'
        for suffix in ['raw_request_response_body', 'request_response_body']:
            retention.run(['docker', 'exec', '-i', retention.CONTAINER, 'mc', 'pipe', prefix + suffix], json.dumps({'request': 'synthetic retention prompt', 'response': 'synthetic retention response'}))
    result = subprocess.run(['python3', str(retention.ROOT / 'audit_retention.py'), '--apply'], capture_output=True, text=True)
    if result.returncode:
        print(result.stderr[-1000:])
        raise RuntimeError('保留策略集成测试执行失败')
    checks = []
    for fixture in fixtures:
        request_id = fixture['id']
        pg_exists = int(retention.pg(f"SELECT count(*) FROM request WHERE id='{request_id}';")) == 1
        ch_exists = int(retention.ch(f"SELECT count(*) FROM request_response_rmt WHERE request_id='{request_id}'")) == 1
        path = f'localminio/request-response-storage/organizations/{org}/requests/{request_id}/raw_request_response_body'
        s3_exists = subprocess.run(['docker', 'exec', retention.CONTAINER, 'mc', 'stat', path], capture_output=True).returncode == 0
        assert pg_exists == ch_exists == s3_exists == fixture['keep'], '保留判定或关联清理未通过'
        checks.append({'case': fixture['name'], 'postgres_exists': pg_exists, 'clickhouse_exists': ch_exists, 'body_exists': s3_exists, 'passed': True})
    (retention.ROOT / 'retention-test-results.json').write_text(json.dumps(checks))
    print(json.dumps(checks, ensure_ascii=False))
finally:
    # 只撤销本测试刚创建的随机编号记录，不触及任何既有用户数据。
    if fixtures:
        retention.delete_metadata(org, {fixture['id'] for fixture in fixtures})
        for fixture in fixtures:
            prefix = f"localminio/request-response-storage/organizations/{org}/requests/{fixture['id']}/"
            retention.run(['docker', 'exec', retention.CONTAINER, 'mc', 'rm', '--recursive', '--force', prefix])
