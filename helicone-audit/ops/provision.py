import hashlib
import json
import os
import secrets
import subprocess
import uuid
from pathlib import Path

os.umask(0o077)
path = Path('/opt/cc2cx-audit-validation/dashboard-credentials.json')
credentials = json.loads(path.read_text())
credentials.setdefault('api_key', 'sk-helicone-' + secrets.token_hex(32))
credentials.setdefault('organization_id', str(uuid.uuid4()))
path.write_text(json.dumps(credentials))
key_hash = hashlib.sha256(('Bearer ' + credentials['api_key']).encode()).hexdigest()
org = credentials['organization_id']
sql = f'''BEGIN;
UPDATE public."user" SET "emailVerified"=true WHERE email='audit-admin@cc2.cx';
INSERT INTO organization(id,name,owner,is_personal,has_onboarded)
SELECT '{org}','CC2CX Audit',auth_user_id,true,true FROM public."user" WHERE email='audit-admin@cc2.cx'
ON CONFLICT(id) DO NOTHING;
INSERT INTO organization_member(organization,"user",member,org_role)
SELECT '{org}',id,auth_user_id,'admin' FROM public."user" u WHERE email='audit-admin@cc2.cx'
AND NOT EXISTS(SELECT 1 FROM organization_member WHERE organization='{org}' AND "user"=u.id);
INSERT INTO helicone_api_keys(api_key_hash,api_key_name,user_id,organization_id,key_permissions)
SELECT '{key_hash}','Local audit gateway',auth_user_id,'{org}','rw' FROM public."user" WHERE email='audit-admin@cc2.cx'
AND NOT EXISTS(SELECT 1 FROM helicone_api_keys WHERE api_key_hash='{key_hash}');
COMMIT;'''
result = subprocess.run(['docker','exec','-i','-u','postgres','cc2cx-helicone-audit','psql','-v','ON_ERROR_STOP=1','-d','helicone_test'],input=sql,text=True,capture_output=True)
if result.returncode:
    raise RuntimeError(result.stderr.replace(key_hash, '[redacted]'))
print('审计账号、组织和独立密钥已初始化，凭证保存在服务器受限文件中。')
