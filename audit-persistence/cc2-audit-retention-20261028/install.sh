#!/usr/bin/env bash
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "必须使用 root 执行"
  exit 1
fi

source_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
state_dir="/opt/cc2cx-audit-validation"
backup_dir="${state_dir}/backup-persistence-$(date -u +%Y%m%dT%H%M%SZ)"

mkdir -p "${backup_dir}"
chmod 0700 "${backup_dir}"

for path in \
  "${state_dir}/audit_retention.py" \
  "${state_dir}/audit_queue_worker.py" \
  "${state_dir}/retention-state.sqlite3" \
  /etc/systemd/system/cc2-audit-retention.service \
  /etc/systemd/system/cc2-audit-retention.timer \
  /etc/systemd/system/cc2-audit-upload.service
do
  if [[ -e "${path}" ]]; then
    cp -a "${path}" "${backup_dir}/"
  fi
done

systemctl stop cc2-audit-retention.timer
systemctl stop cc2-audit-upload.service

install -m 0755 "${source_dir}/audit_retention.py" "${state_dir}/audit_retention.py"
install -m 0644 "${source_dir}/audit_queue_worker.py" "${state_dir}/audit_queue_worker.py"
install -m 0644 "${source_dir}/cc2-audit-retention.service" /etc/systemd/system/cc2-audit-retention.service
install -m 0644 "${source_dir}/cc2-audit-retention.timer" /etc/systemd/system/cc2-audit-retention.timer
install -m 0644 "${source_dir}/cc2-audit-upload.service" /etc/systemd/system/cc2-audit-upload.service

systemctl daemon-reload
systemctl reset-failed cc2-audit-retention.service cc2-audit-upload.service
systemctl start cc2-audit-upload.service
systemctl enable --now cc2-audit-retention.timer

python3 "${state_dir}/audit_retention.py" --mode all

printf '安装完成，备份目录：%s\n' "${backup_dir}"
