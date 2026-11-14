#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.."

export BACKEND_IMAGE="${BACKEND_IMAGE:-orbital-backend:e2e}"
export FRONTEND_ORIGIN="${FRONTEND_ORIGIN:-https://frontend.example.com}"
export GAME_PORT=0
export POSTGRES_PASSWORD=orbital
project="orbital-e2e-$$-$RANDOM"
compose=(docker compose -p "$project" -f compose.yaml)
export E2E_COMPOSE_PROJECT="$project"

docker build --target backend --tag "$BACKEND_IMAGE" .
cleanup() {
  local status=$?
  if (( status != 0 )); then
    "${compose[@]}" logs --no-color >&2 || true
  fi
  "${compose[@]}" down --volumes || {
    if (( status == 0 )); then status=1; fi
  }
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

"${compose[@]}" up --no-build --detach --wait --wait-timeout 90
export E2E_BASE_URL="http://$("${compose[@]}" port game 8080)"
go test -race -count=1 -tags=e2e -timeout=150s -v ./tests/e2e
