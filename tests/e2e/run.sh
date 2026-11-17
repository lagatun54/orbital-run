#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.."

export BACKEND_IMAGE="${BACKEND_IMAGE:-orbital-backend:e2e}"
export FRONTEND_ORIGIN="${FRONTEND_ORIGIN:-https://frontend.example.com}"
export GAME_REPLICAS=1
export GAME_MAX_MATCHES=1
export DEMO_DOMAIN=http://
export POSTGRES_PASSWORD=orbital
export REDIS_URL=redis://redis:6379/0
project="orbital-e2e-$$-$RANDOM"
override=$(mktemp)
cat > "$override" <<'YAML'
services:
  caddy:
    ports: !override
      - "127.0.0.1::80"
YAML
export COMPOSE_FILE="$PWD/compose.yaml:$PWD/compose.public.yaml:$override"
compose=(docker compose -p "$project")
export E2E_COMPOSE_PROJECT="$project"

cleanup() {
  local status=$?
  if (( status != 0 )); then
    "${compose[@]}" logs --no-color >&2 || true
  fi
  "${compose[@]}" down --volumes || {
    if (( status == 0 )); then status=1; fi
  }
  rm -f "$override"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

docker build --target backend --tag "$BACKEND_IMAGE" .
"${compose[@]}" up --no-build --detach --wait --wait-timeout 90
"${compose[@]}" exec -T caddy caddy reload --config - --adapter caddyfile < Caddyfile
export E2E_BASE_URL="http://$("${compose[@]}" port caddy 80)"
go test -race -count=1 -tags=e2e -timeout=150s -v ./tests/e2e
