import json
import subprocess

# 保留测试实例的数据卷和认证配置，统一宿主机与容器内对象存储端口。
name = 'cc2cx-helicone-audit'
state = json.loads(subprocess.check_output(['docker', 'inspect', name]))[0]
command = ['docker','run','-d','--name',name,'--memory','3g','--cpus','2',
           '-p','127.0.0.1:18300:3000','-p','127.0.0.1:18585:8585','-p','127.0.0.1:9080:9080']
for value in state['Config']['Env']:
    if value.startswith('S3_ENDPOINT='):
        value = 'S3_ENDPOINT=http://127.0.0.1:9080'
    command += ['-e',value]
for mount in state['Mounts']:
    if mount['Type'] == 'volume':
        destination = mount['Destination']
        if mount['Name'] == 'cc2cx-audit-postgres':
            destination = '/var/lib/postgresql/17/main'
        command += ['-v', mount['Name'] + ':' + destination]
command += [state['Config']['Image']]
subprocess.run(['docker','stop',name],check=True,stdout=subprocess.DEVNULL)
subprocess.run(['docker','rm',name],check=True,stdout=subprocess.DEVNULL)
result = subprocess.run(command,capture_output=True,text=True)
if result.returncode:
    raise RuntimeError('审计测试实例重启失败，请检查 Docker 状态。')
print('审计测试实例已统一对象存储端口，数据卷保留。')
