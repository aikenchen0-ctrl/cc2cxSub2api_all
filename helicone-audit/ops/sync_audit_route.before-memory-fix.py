import fcntl
import json
import os
import re
import subprocess
import urllib.request
from pathlib import Path

ROOT = Path('/opt/cc2cx-audit-validation')
TARGET = Path('/etc/nginx/conf.d/00-cc2-audit-map.conf')
os.umask(0o077)


def command(args):
    result = subprocess.run(args, capture_output=True, text=True, timeout=15)
    if result.returncode:
        raise RuntimeError('本地检查未通过')
    return result.stdout


def healthy(url, headers=None):
    with urllib.request.urlopen(urllib.request.Request(url, headers=headers or {}), timeout=4) as response:
        if response.status != 200:
            raise RuntimeError('审计服务未就绪')
        response.read(65536)


def main():
    lock = (ROOT / 'route-sync.lock').open('w')
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    enabled = (ROOT / 'production-enabled').exists()
    keys = []
    reason = '手动关闭'
    secret = ''
    if enabled:
        try:
            credentials = json.loads((ROOT / 'dashboard-credentials.json').read_text())
            secret = credentials['api_key']
            if not re.fullmatch(r'[A-Za-z0-9_-]+', secret):
                raise RuntimeError('凭证格式不受支持')
            sql = "SELECT k.id,k.user_id,k.key FROM api_keys k JOIN users u ON u.id=k.user_id WHERE u.status='active' AND k.deleted_at IS NULL AND k.status='active' AND (k.expires_at IS NULL OR k.expires_at>now()) ORDER BY (k.user_id=11) DESC,k.id DESC;"
            rows = command(['docker', 'exec', 'sub2api-postgres', 'sh', '-lc', 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "$1"', 'sh', sql])
            for row in rows.splitlines():
                key_id, user_id, key = row.split('|', 2)
                if not key_id.isdecimal() or not user_id.isdecimal() or not re.fullmatch(r'[A-Za-z0-9_-]+', key):
                    raise RuntimeError('用户密钥格式不受支持')
                keys.append((key_id, user_id, key))
            if not keys:
                raise RuntimeError('目标用户没有有效密钥')
            healthy('http://127.0.0.1:18480/health')
            healthy('http://127.0.0.1:18585/healthcheck')
            healthy('http://127.0.0.1:9080/minio/health/live')
            # 业务鉴权检查直接访问后端，避免每半分钟制造一条无正文的审计记录。
            healthy('http://127.0.0.1:8080/v1/models', {'Authorization': 'Bearer ' + keys[0][2]})
            command(['docker', 'exec', 'cc2cx-helicone-audit', 'clickhouse-client', '--query', 'SELECT count(*) FROM request_response_rmt LIMIT 1'])
            if os.statvfs(ROOT).f_bavail * os.statvfs(ROOT).f_frsize < 5 * 1024**3:
                raise RuntimeError('磁盘余量不足')
            reason = '健康检查通过'
        except Exception:
            enabled = False
            reason = '依赖未就绪，后续新请求直连'
    # 仅由服务器数据库生成身份映射，不接受客户端声明的审计身份。
    lines = ['map $http_authorization $cc2_audit_key_id {', '    default "";']
    if enabled:
        for key_id, user_id, key in keys:
            lines.append(f'    "~^(?i:Bearer) +{key}$" "{key_id}";')
    lines += ['}', 'map $cc2_audit_key_id $cc2_audit_user {', '    default "";']
    if enabled:
        for key_id, user_id, key in keys:
            lines.append(f'    "{key_id}" "{user_id}";')
    lines += ['}',
              'map "$cc2_audit_user:$request_method:$http_upgrade" $cc2_audit_backend {',
              '    default "http://127.0.0.1:8080";',
              '    "~^[0-9]+:POST:$" "http://127.0.0.1:18480/openai";', '}',
              'map $cc2_audit_backend $cc2_audit_secret {', '    default "";']
    if enabled:
        lines.append(f'    "http://127.0.0.1:18480/openai" "{secret}";')
    lines += ['}', '']
    content = '\n'.join(lines)
    old = TARGET.read_text() if TARGET.exists() else None
    changed = old != content
    if changed:
        temp = TARGET.with_suffix('.pending')
        temp.write_text(content)
        os.chmod(temp, 0o600)
        os.replace(temp, TARGET)
        try:
            command(['nginx', '-t'])
            command(['systemctl', 'reload', 'nginx'])
        except Exception:
            if old is None:
                TARGET.unlink(missing_ok=True)
            else:
                temp.write_text(old)
                os.replace(temp, TARGET)
            raise RuntimeError('入口配置验证失败，已恢复原文件') from None
    state = {'enabled': enabled, 'mapped_keys': len(keys) if enabled else 0, 'mapped_users': len({item[1] for item in keys}) if enabled else 0, 'changed': changed, 'reason': reason}
    (ROOT / 'production-route-status.json').write_text(json.dumps(state, ensure_ascii=False))
    print(json.dumps(state, ensure_ascii=False))


if __name__ == '__main__':
    main()
