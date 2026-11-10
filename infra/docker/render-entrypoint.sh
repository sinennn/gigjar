#!/bin/sh
set -eu

signer_pid=""
if [ "${GIGJAR_CHAIN_NETWORK:-simulated}" != "simulated" ]; then
  GIGJAR_SIGNER_HOST=127.0.0.1 GIGJAR_SIGNER_PORT=8081 node /app/signer/src/server.js &
  signer_pid=$!
  export GIGJAR_SIGNER_URL=http://127.0.0.1:8081
fi

env -u GIGJAR_SIGNER_SEED /app/gigjar &
app_pid=$!

shutdown() {
  kill -TERM "$app_pid" 2>/dev/null || true
  [ -n "$signer_pid" ] && kill -TERM "$signer_pid" 2>/dev/null || true
  wait "$app_pid" 2>/dev/null || true
  exit 0
}
trap shutdown TERM INT

while kill -0 "$app_pid" 2>/dev/null; do
  if [ -n "$signer_pid" ] && ! kill -0 "$signer_pid" 2>/dev/null; then
    echo '{"level":"ERROR","service":"entrypoint","msg":"signer exited; stopping"}'
    kill -TERM "$app_pid" 2>/dev/null || true
    wait "$app_pid" 2>/dev/null || true
    exit 1
  fi
  sleep 2
done

[ -n "$signer_pid" ] && kill -TERM "$signer_pid" 2>/dev/null || true
wait "$app_pid"
