#!/usr/bin/env bash
# CAP one-command demo: rebuild embedded dashboard, build binaries,
# start server + gateway, open health checks.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
RUN="${TMPDIR:-/tmp}/cap-run"
mkdir -p "$RUN"

echo "==> Building embedded dashboard"
(cd "$ROOT/../frontend" && npm run build >/dev/null && cp -r dist/* "$ROOT/internal/web/web/")

echo "==> Building binaries"
go build -o "$RUN/server" ./cmd/server
go build -o "$RUN/gateway" ./cmd/gateway

echo "==> Starting server on http://127.0.0.1:8000"
fuser -k 8000/tcp >/dev/null 2>&1 || true
sleep 0.5
rm -f "$RUN/app.db" "$RUN/app.db-wal" "$RUN/app.db-shm" "$RUN/gateway-buffer.db"*
setsid nohup env DB_URL="sqlite://$RUN/app.db" HTTP_ADDR="127.0.0.1:8000" \
  SIMULATOR_ENABLED=true SIM_INTERVAL=1s EMERGENCY_SECONDS=5s \
  STALENESS_SECONDS=60s "$RUN/server" > "$RUN/server.log" 2>&1 &
sleep 1.5

echo "==> Starting gateway (gw-demo-01)"
setsid nohup env GATEWAY_ID="gw-demo-01" SERVER_URL="http://127.0.0.1:8000" \
  GATEWAY_BUFFER_DB="$RUN/gateway-buffer.db" POLL_INTERVAL=1s PUSH_INTERVAL=1s \
  PULSE_INTERVAL=3s "$RUN/gateway" > "$RUN/gateway.log" 2>&1 &
sleep 2

echo "==> Health"
curl -sf http://127.0.0.1:8000/api/v1/health >/dev/null && echo "    server: OK"
curl -sf http://127.0.0.1:8000/ >/dev/null && echo "    dashboard: OK"
curl -sf http://127.0.0.1:8000/api/v1/gateways >/dev/null && echo "    gateways: OK"
curl -sf http://127.0.0.1:8000/api/v1/simulator/status >/dev/null && echo "    simulator: OK"

echo
echo "Open http://127.0.0.1:8000  (stop all: fuser -k 8000/tcp)"
echo "Logs: $RUN/server.log, $RUN/gateway.log"