#!/usr/bin/env bash
# AutoPro demo: edge gateway store-and-forward (TZ S3).
#
# Scenario: server up -> gateway streams -> server DOWN -> gateway buffers in
# local SQLite -> server back -> gateway backfills, queue drains.
#
# Usage: bash demo.sh   (builds both binaries, runs against port 18099)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
WORK="$(mktemp -d /tmp/opencode/autopro-demo.XXXXXX)"
PORT=18099
SERVER_BIN="$WORK/autopro-server"
GATEWAY_BIN="$WORK/autopro-gateway"
SERVER_LOG="$WORK/server.log"
GATEWAY_LOG="$WORK/gateway.log"

cleanup() {
  fuser -k "${PORT}/tcp" 2>/dev/null || true
  kill "${GW_PID:-0}" 2>/dev/null || true
}
trap cleanup EXIT

echo "==> Building binaries"
(cd "$ROOT" && go build -o "$SERVER_BIN" ./cmd/server && go build -o "$GATEWAY_BIN" ./cmd/gateway)

start_server() {
  DB_URL="sqlite://$WORK/demo.db" HTTP_ADDR="127.0.0.1:$PORT" SIMULATOR_ENABLED=false \
    STALENESS_SECONDS=60s "$SERVER_BIN" > "$SERVER_LOG" 2>&1 &
  sleep 1.5
}
start_gateway() {
  GATEWAY_ID="gw-crushing-01" SERVER_URL="http://127.0.0.1:$PORT" \
    GATEWAY_BUFFER_DB="$WORK/gw-buffer.db" POLL_INTERVAL=1s PUSH_INTERVAL=1s PULSE_INTERVAL=3s \
    "$GATEWAY_BIN" > "$GATEWAY_LOG" 2>&1 &
  GW_PID=$!
}

count() { curl -s "http://127.0.0.1:$PORT/api/v1/telemetry?limit=1000" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))"; }

echo "==> Phase 1: server + gateway up"
start_server
start_gateway
sleep 6
echo "    gateway row:   $(curl -s http://127.0.0.1:$PORT/api/v1/gateways | python3 -c "import sys,json; g=json.load(sys.stdin)[0]; print(g['id'], g['status'], 'buffer:', g['buffer_size'])")"
echo "    readings:      $(count)"

echo "==> Phase 2: server goes DOWN, gateway keeps collecting into the buffer"
fuser -k "${PORT}/tcp" >/dev/null 2>&1
sleep 8
echo "    gateway log (last 3):"
grep -E 'unreachable|delivered' "$GATEWAY_LOG" | tail -3 | sed 's/^/      /'

echo "==> Phase 3: server is back, gateway drains the queue (backfill)"
start_server
sleep 8
echo "    gateway row:   $(curl -s http://127.0.0.1:$PORT/api/v1/gateways | python3 -c "import sys,json; g=json.load(sys.stdin)[0]; print(g['id'], g['status'], 'buffer:', g['buffer_size'])")"
echo "    readings:      $(count)"
echo "    gateway log (backfill):"
grep -E 'delivered|unreachable' "$GATEWAY_LOG" | tail -4 | sed 's/^/      /'

echo "==> Phase 4: profile switch on the fly (alerts threshold change)"
ACTIVE=$(curl -s http://127.0.0.1:$PORT/api/v1/profiles/active | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
echo "    before: $ACTIVE"
curl -s -X POST "http://127.0.0.1:$PORT/api/v1/profiles/ore-high-silica-01/approve" -H 'Content-Type: application/json' -d '{"approved_by":"director"}' > /dev/null
curl -s -X POST "http://127.0.0.1:$PORT/api/v1/profiles/ore-high-silica-01/activate" -H 'Content-Type: application/json' -d '{}' > /dev/null
echo "    after:  $(curl -s http://127.0.0.1:$PORT/api/v1/profiles/active | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")"
echo "    readings still flowing: $(count)"

echo "==> Demo complete. Logs: $WORK"
