import json
import os
import secrets
import urllib.request
from pathlib import Path

os.umask(0o077)
path = Path('/opt/cc2cx-audit-validation/dashboard-credentials.json')
if path.exists():
    credentials = json.loads(path.read_text())
else:
    credentials = {'email': 'audit-admin@cc2.cx', 'password': secrets.token_urlsafe(32), 'name': 'Audit Administrator'}
    path.write_text(json.dumps(credentials))
request = urllib.request.Request(
    'http://127.0.0.1:18300/api/auth/sign-up/email',
    data=json.dumps(credentials).encode(),
    headers={'Content-Type': 'application/json', 'Origin': 'http://127.0.0.1:18300'},
)
try:
    with urllib.request.urlopen(request, timeout=30) as response:
        result = json.load(response)
        print(json.dumps({'status': response.status, 'user_id': result.get('user', {}).get('id')}))
except urllib.error.HTTPError as error:
    print(json.dumps({'status': error.code, 'error': error.read().decode()[:300]}))
