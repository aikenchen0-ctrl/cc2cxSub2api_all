import json
import subprocess
import sys
from pathlib import Path

root = Path('/opt/cc2cx-audit-validation')
credentials = json.loads((root / 'dashboard-credentials.json').read_text())
name = 'cc2cx-audit-nonblocking-test'
subprocess.run(['docker', 'rm', '-f', name], capture_output=True)
command = ['docker', 'run', '-d', '--name', name, '--network', 'host',
           '--memory', '512m', '--memory-swap', '512m', '--cpus', '1',
           '-v', str(root / 'config-memory-test.yaml') + ':/etc/ai-gateway/config.yaml:ro',
           '-e', 'OPENAI_API_KEY=synthetic-provider-key',
           '-e', 'HELICONE_CONTROL_PLANE_API_KEY=' + credentials['api_key']]
if '--read-only' in sys.argv:
    command += ['--read-only']
command += ['cc2cx/helicone-passthrough:20261007-nonblocking']
result = subprocess.run(command, capture_output=True, text=True)
if result.returncode:
    raise RuntimeError('非阻塞测试容器启动失败')
print('非阻塞测试容器已启动')
