#!/bin/bash
set -euo pipefail

# 独立用户和状态目录只属于测试画廊，不改变 Sub2API 容器及数据库。
id -u svg-lab >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin svg-lab
install -d -o svg-lab -g svg-lab -m 700 /var/lib/upstream-svg-lab
python3 /opt/upstream-svg-lab/deploy/prepare_env.py
install -m 644 /opt/upstream-svg-lab/deploy/upstream-svg-lab.service /etc/systemd/system/upstream-svg-lab.service
systemctl daemon-reload
systemctl enable --now upstream-svg-lab
systemctl restart upstream-svg-lab
for attempt in $(seq 1 10); do
  if curl -fsS http://127.0.0.1:9876/ak47/api/config >/dev/null; then break; fi
  sleep 1
done
curl -fsS http://127.0.0.1:9876/ak47/api/config >/dev/null

# 先保存原配置，只有语法检查通过才重载；失败立即恢复。
backup="/etc/nginx/conf.d/cc2-apps.conf.svg-backup-$(date +%Y%m%dT%H%M%S)"
cp /etc/nginx/conf.d/cc2-apps.conf "$backup"
install -m 644 /etc/nginx/cc2-apps-svg-staged.conf /etc/nginx/conf.d/cc2-apps.conf
if nginx -t; then
  systemctl reload nginx
else
  cp "$backup" /etc/nginx/conf.d/cc2-apps.conf
  exit 1
fi
printf '%s\n' '画廊服务已启动，原站点配置备份完成。'
