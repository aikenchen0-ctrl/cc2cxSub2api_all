import json
import subprocess
from pathlib import Path

root = Path('/opt/cc2cx-audit-validation')
secret = json.loads((root / 'dashboard-credentials.json').read_text())['api_key']
command = [
    'docker', 'run', '-d', '--name', 'cc2cx-audit-eof-candidate', '--network', 'host',
    '--memory', '1g', '--memory-swap', '1g', '--cpus', '2',
    '--ulimit', 'nofile=65536:65536', '--restart', 'unless-stopped',
    '-v', str(root / 'config-sub2api-eof-v3.yaml') + ':/etc/ai-gateway/config.yaml:ro',
    '-v', str(root / 'queue') + ':/var/spool/cc2-audit',
    '-e', 'CC2_AUDIT_SPOOL=/var/spool/cc2-audit',
    '-e', 'OPENAI_API_KEY=synthetic-provider-key',
    '-e', 'HELICONE_CONTROL_PLANE_API_KEY=' + secret,
    'cc2cx/helicone-passthrough:20261009-eof-v3',
]
result = subprocess.run(command, capture_output=True, text=True)
if result.returncode:
    raise RuntimeError('候选代理启动失败，请检查容器状态。')
print('修复候选已启动，监听端口 18488。')
