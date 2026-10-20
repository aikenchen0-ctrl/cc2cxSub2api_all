import json
import subprocess
from pathlib import Path

root = Path('/opt/cc2cx-audit-validation')
total = 0
for batch in range(60):
    result = subprocess.run(['/usr/bin/python3', str(root / 'audit_retention.py'), '--apply', '--remove-default-ttl'], capture_output=True, text=True, timeout=880)
    if result.returncode:
        raise RuntimeError('保留期清理失败，保留待处理编号供下次重试：' + result.stderr[-500:])
    state = json.loads(result.stdout)
    total += state['deleted']
    print(json.dumps({'batch': batch + 1, 'total_deleted': total, **state}), flush=True)
    # 一旦积压不足一批，就交回小时定时器，避免追逐不断跨过截止时间的新记录。
    if state['candidates'] < 2000 and not state['pending']:
        break
