import json
import os
import pwd
import secrets
import subprocess
import urllib.request
from pathlib import Path

# 复用服务器已配置的管理员密钥，全程只在内存读取，不输出到终端。
items = json.loads(subprocess.check_output(["docker", "inspect", "agentapi"]))
environment = dict(item.split("=", 1) for item in items[0]["Config"]["Env"])
admin_key = environment.get("SUB2API_ADMIN_KEY", "")
# 失效的旧密钥不写入新服务；用户可在管理页验证并保存现行密钥。
if admin_key:
    try:
        request = urllib.request.Request(
            "http://127.0.0.1:28080/api/v1/admin/accounts?page=1&page_size=1&lite=true",
            headers={"x-api-key": admin_key},
        )
        urllib.request.urlopen(request, timeout=15).close()
    except Exception:
        admin_key = ""
account = pwd.getpwnam("svg-lab")
token_path = Path("/var/lib/upstream-svg-lab/access.token")
if token_path.exists():
    token = token_path.read_text().strip()
else:
    token = secrets.token_urlsafe(32)
    token_path.write_text(token + "\n")
os.chmod(token_path, 0o600)
os.chown(token_path, account.pw_uid, account.pw_gid)
values = {
    "SUB2API_BASE_URL": "http://127.0.0.1:28080",
    "SUB2API_ADMIN_API_KEY": admin_key,
    "LAB_PORT": "9876",
    "LAB_DATA_DIR": "/var/lib/upstream-svg-lab",
    "LAB_PUBLIC_ORIGIN": "https://cc2.cx",
    "LAB_BASE_PATH": "/ak47",
    "LAB_ACCESS_TOKEN": token,
    "LAB_TIMEOUT_SECONDS": "1200",
    "LAB_IDLE_TIMEOUT_SECONDS": "280",
    "LAB_REASONING_EFFORT": "low",
    "LAB_MAX_OUTPUT_TOKENS": "65536",
}
env_path = Path("/etc/upstream-svg-lab.env")
env_path.write_text("\n".join(f"{name}={value}" for name, value in values.items()) + "\n")
os.chmod(env_path, 0o600)
