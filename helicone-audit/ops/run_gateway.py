import argparse
import json
import subprocess
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument('mode', choices=['passthrough', 'default', 'observe', 'sub2api'])
parser.add_argument('--name', default='cc2cx-helicone-passthrough')
args = parser.parse_args()
root = Path('/opt/cc2cx-audit-validation')
name = args.name
exists = subprocess.run(['docker', 'inspect', name], capture_output=True)
if exists.returncode == 0:
    subprocess.run(['docker', 'rm', '-f', name], check=True, stdout=subprocess.DEVNULL)
config = {'passthrough': 'config.yaml', 'default': 'config-default.yaml', 'observe': 'config-observe.yaml', 'sub2api': 'config-sub2api-eof-v3.yaml'}[args.mode]
command = ['docker','run','-d','--name',name,'--network','host','--memory','1g','--memory-swap','1g','--cpus','2','--ulimit','nofile=65536:65536','--restart','unless-stopped',
           '-v',f'{root / config}:/etc/ai-gateway/config.yaml:ro',
           '-e','OPENAI_API_KEY=synthetic-provider-key']
if args.mode in ('observe', 'sub2api'):
    credentials = json.loads((root / 'dashboard-credentials.json').read_text())
    command += ['-e', 'HELICONE_CONTROL_PLANE_API_KEY=' + credentials['api_key']]
if args.mode == 'sub2api':
    queue = root / 'queue'
    queue.mkdir(mode=0o700, exist_ok=True)
    command += ['-v', str(queue) + ':/var/spool/cc2-audit', '-e', 'CC2_AUDIT_SPOOL=/var/spool/cc2-audit']
command += ['cc2cx/helicone-passthrough:20261009-eof-v3']
result = subprocess.run(command, capture_output=True, text=True)
if result.returncode:
    raise RuntimeError('测试网关启动失败，请检查 Docker 状态。')
print('测试网关已启动，模式：' + args.mode)
