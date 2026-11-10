#!/usr/bin/env bash
set -euo pipefail

read_env() {
  grep -s "^$1=" .env | head -1 | cut -d= -f2-
}

export GIGJAR_TRONGRID_API_KEY="${GIGJAR_TRONGRID_API_KEY:-$(read_env GIGJAR_TRONGRID_API_KEY)}"
export GIGJAR_SIGNER_TOKEN="${GIGJAR_SIGNER_TOKEN:-$(read_env GIGJAR_SIGNER_TOKEN)}"
seed="${GIGJAR_SIGNER_SEED:-$(read_env GIGJAR_SIGNER_SEED)}"
port="${GIGJAR_SIGNER_PORT:-18081}"

for value in GIGJAR_TRONGRID_API_KEY GIGJAR_SIGNER_TOKEN; do
  [ -n "${!value}" ] || { echo "$value is missing (environment or .env)"; exit 1; }
done
[ -n "$seed" ] || { echo "GIGJAR_SIGNER_SEED is missing (environment or .env)"; exit 1; }

GIGJAR_CHAIN_NETWORK=tron-nile GIGJAR_SIGNER_SEED="$seed" GIGJAR_SIGNER_PORT="$port" node services/signer/src/server.js &
signer_pid=$!
trap 'kill "$signer_pid" 2>/dev/null || true' EXIT

for _ in $(seq 1 30); do
  curl -sf "http://localhost:$port/healthz" >/dev/null && break
  sleep 1
done

GIGJAR_SIGNER_URL="http://localhost:$port" \
GIGJAR_TEST_DATABASE_URL="${GIGJAR_TEST_DATABASE_URL:-postgres://gigjar:gigjar@localhost:55432/gigjar_test?sslmode=disable}" \
  go test -count=1 -p 1 -timeout 20m -tags "${TEST_TAGS:-integration trongrid}" -run "${TEST_RUN:-LiveNileSweep}" -v "${TEST_PKG:-./internal/settlement/}"
