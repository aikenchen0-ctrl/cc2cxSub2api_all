#!/usr/bin/env bash
set -euo pipefail

# Run on the production host from the repository root, or set APP_ROOT to the
# checkout containing qrcode/docker-compose.yml. Secrets are read from the
# existing deployment environment and are never printed or passed as CLI args.
APP_ROOT="${APP_ROOT:-/opt/cc2cxSub2api_260920}"
if [ -n "${ENV_FILE:-}" ]; then
  ENV_FILE="$ENV_FILE"
elif [ -f "$APP_ROOT/sub2api/deploy/.env" ]; then
  ENV_FILE="$APP_ROOT/sub2api/deploy/.env"
else
  ENV_FILE="$APP_ROOT/.env"
fi
SAT_ENV_FILE="${SAT_ENV_FILE:-$APP_ROOT/sub2api/deploy/.env.satellites}"
COMPOSE_DIR="$APP_ROOT/qrcode"

test -f "$COMPOSE_DIR/docker-compose.yml" || { echo "qrcode compose not found: $COMPOSE_DIR" >&2; exit 1; }
test -f "$ENV_FILE" || { echo "deployment env not found: $ENV_FILE" >&2; exit 1; }

grep -Eq '^SUB2API_SSO_SECRET=.{32,}$' "$ENV_FILE" || {
  echo "SUB2API_SSO_SECRET is missing or shorter than 32 characters in $ENV_FILE" >&2
  exit 1
}
grep -Eq '^SUB2API_APP_CREDENTIAL=.+$' "$ENV_FILE" || {
  echo "SUB2API_APP_CREDENTIAL is missing in $ENV_FILE" >&2
  exit 1
}

backup="$COMPOSE_DIR/docker-compose.yml.bak.$(date +%Y%m%d%H%M%S)"
cp "$COMPOSE_DIR/docker-compose.yml" "$backup"

test -f "$SAT_ENV_FILE" || { echo "satellite env not found: $SAT_ENV_FILE" >&2; exit 1; }

# Compose loads files in order; give the primary SSO credentials explicitly
# from the main deployment env so satellite env defaults cannot shadow them.
secret="$(sed -n 's/^SUB2API_SSO_SECRET=//p' "$ENV_FILE" | tail -n 1)"
credential="$(sed -n 's/^SUB2API_APP_CREDENTIAL=//p' "$ENV_FILE" | tail -n 1)"
export SUB2API_SSO_SECRET="$secret"
export SUB2API_APP_CREDENTIAL="$credential"
export QR_API_BASE_URL="https://api.cc2.cx"
export QR_CORS_ORIGIN="https://qrcode.cc2.cx"
export QR_SECURE_COOKIE=1

env_args=(--env-file "$ENV_FILE" --env-file "$SAT_ENV_FILE")

docker compose "${env_args[@]}" --project-directory "$COMPOSE_DIR" \
  -f "$COMPOSE_DIR/docker-compose.yml" config --quiet
docker compose "${env_args[@]}" --project-directory "$COMPOSE_DIR" \
  -f "$COMPOSE_DIR/docker-compose.yml" up -d --build qrcode

for _ in $(seq 1 20); do
  if curl -fsS --max-time 5 http://127.0.0.1:5221/api/health >/dev/null; then
    echo "qrcode container is healthy; previous compose backed up at $backup"
    exit 0
  fi
  sleep 2
done

echo "qrcode health check failed; inspect: docker logs --tail 100 qrcode" >&2
exit 1
