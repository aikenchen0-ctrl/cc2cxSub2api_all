#!/usr/bin/env bash
set -euo pipefail

# Apply the public origins used by the production reverse proxies. This file
# intentionally changes only routing values; secrets and all other settings
# remain untouched.
APP_ROOT="${APP_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
if [ -n "${ENV_FILE:-}" ]; then
  ENV_FILE="$ENV_FILE"
elif [ -f "$APP_ROOT/sub2api/deploy/.env" ]; then
  ENV_FILE="$APP_ROOT/sub2api/deploy/.env"
else
  ENV_FILE="$APP_ROOT/.env"
fi
SAT_ENV_FILE="${SAT_ENV_FILE:-$APP_ROOT/sub2api/deploy/.env.satellites}"

test -f "$ENV_FILE" || { echo "deployment env not found: $ENV_FILE" >&2; exit 1; }
test -f "$SAT_ENV_FILE" || { echo "satellite env not found: $SAT_ENV_FILE" >&2; exit 1; }

secret="$(sed -n 's/^SUB2API_SSO_SECRET=//p' "$ENV_FILE" | tail -n 1)"
if [ "${#secret}" -lt 32 ]; then
  echo "SUB2API_SSO_SECRET must be at least 32 characters in $ENV_FILE" >&2
  exit 1
fi

python3 - "$ENV_FILE" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
values = {
    "qrcode_link": "https://qrcode.cc2.cx",
    "yibiao_link": "https://yibiao.cc2.cx",
    "QRCODE_SSO_CALLBACK_URL": "https://qrcode.cc2.cx/api/auth/sso/callback",
    "YIBIAO_SSO_CALLBACK_URL": "https://yibiao.cc2.cx/api/auth/sso/callback",
}
lines = path.read_text(encoding="utf-8").splitlines()
seen = set()
out = []
for line in lines:
    key = line.split("=", 1)[0].strip() if "=" in line and not line.lstrip().startswith("#") else ""
    if key in values:
        out.append(f"{key}={values[key]}")
        seen.add(key)
    else:
        out.append(line)
for key, value in values.items():
    if key not in seen:
        out.append(f"{key}={value}")
path.write_text("\n".join(out) + "\n", encoding="utf-8")
PY

cd "$APP_ROOT/sub2api/deploy"
docker compose --env-file "$ENV_FILE" up -d sub2api

# Both satellites must be recreated with the exact same shared secret. The
# explicit public gateway values also prevent the qrcode container from
# inheriting its local-development QR_API_BASE_URL.
export QR_API_BASE_URL="https://api.cc2.cx"
export QR_CORS_ORIGIN="https://qrcode.cc2.cx"
export QR_SECURE_COOKIE=1
docker compose --env-file "$ENV_FILE" --env-file "$SAT_ENV_FILE" \
  --project-directory "$APP_ROOT/qrcode" -f "$APP_ROOT/qrcode/docker-compose.yml" \
  up -d --build qrcode
docker compose --env-file "$ENV_FILE" --env-file "$SAT_ENV_FILE" \
  --project-directory "$APP_ROOT/yibiao" -f "$APP_ROOT/yibiao/docker-compose.yml" \
  up -d --build yibiao

for attempt in $(seq 1 20); do
  if curl -fsS --max-time 5 https://qrcode.cc2.cx/api/health >/dev/null &&
     curl -fsS --max-time 5 https://yibiao.cc2.cx/ >/dev/null; then
    echo "Online origins and satellite health checks passed."
    exit 0
  fi
  sleep 2
done

echo "Online health check failed; inspect docker logs for qrcode and yibiao." >&2
exit 1
