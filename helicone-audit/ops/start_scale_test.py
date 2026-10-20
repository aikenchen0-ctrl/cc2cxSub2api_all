import json
import subprocess
import sys
from pathlib import Path

root = Path('/opt/cc2cx-audit-validation')
queue = root / 'queue-test'
queue.mkdir(mode=0o700, exist_ok=True)
credentials = json.loads((root / 'dashboard-credentials.json').read_text())
name = 'cc2cx-audit-scale-test'
subprocess.run(['docker', 'rm', '-f', name], capture_output=True)
subprocess.run(['docker', 'stop', 'cc2cx-audit-nonblocking-test'], capture_output=True)
command = ['docker', 'run', '-d', '--name', name, '--network', 'host',
           '--memory', '1g', '--memory-swap', '1g', '--cpus', '2', '--ulimit', 'nofile=65536:65536',
           '-v', str(root / 'config-memory-test.yaml') + ':/etc/ai-gateway/config.yaml:ro',
           '-v', str(queue) + ':/var/spool/cc2-audit' + (':ro' if '--read-only-spool' in sys.argv else ''),
           '-e', 'CC2_AUDIT_SPOOL=/var/spool/cc2-audit',
           '-e', 'OPENAI_API_KEY=synthetic-provider-key',
           '-e', 'HELICONE_CONTROL_PLANE_API_KEY=' + credentials['api_key'],
           'cc2cx/helicone-passthrough:20261009-eof-v3']
result = subprocess.run(command, capture_output=True, text=True)
if result.returncode:
    raise RuntimeError('数百并发测试容器启动失败')
print('数百并发测试容器已启动')
