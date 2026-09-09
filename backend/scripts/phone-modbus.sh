#!/usr/bin/env bash
# CAP phone mode: the dashboard is driven by a REAL protocol source — a Modbus
# TCP server app on an Android phone (e.g. "Modbus Server Simulator" from Play
# Market). The CAP gateway polls the phone; changing a register in the app
# changes the value on the dashboard. The demo simulator is switched OFF.
#
# Usage:  scripts/phone-modbus.sh <phone-ip>[:port]     (port default 502)
#
# Register map to configure in the phone app (holding registers, 0-based):
#
#   register  scale  tag                              unit   enter as
#   --------  -----  -------------------------------  -----  ---------
#   HR 0      0.01   plant-a.flotation.ph_level       pH     958  -> 9.58
#   HR 2      0.001  plant-a.crushing.pulp_density    g/cm3  1650 -> 1.650
#   HR 4      0.1    plant-a.dewatering.dryer_temperature  C 1413 -> 141.3
#   HR 6      0.01   plant-a.dewatering.cake_moisture %      806  -> 8.06
#
# If the app uses 1-based (4xxxx) addressing, HR 0 is register 40001 there.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RUN="${TMPDIR:-/tmp}/cap-run"
mkdir -p "$RUN"

PHONE="${1:?usage: phone-modbus.sh <phone-ip>[:port]}"
case "$PHONE" in *:*) ;; *) PHONE="$PHONE:502" ;; esac

echo "==> Stopping previous CAP processes"
fuser -k 8000/tcp >/dev/null 2>&1 || true
pkill -f "cap-run/gateway" 2>/dev/null || true
sleep 0.5

echo "==> Building binaries"
(cd "$ROOT" && go build -o "$RUN/server" ./cmd/server && go build -o "$RUN/gateway" ./cmd/gateway)

echo "==> Starting server (simulator OFF — only real sources now)"
rm -f "$RUN/app.db" "$RUN/app.db-wal" "$RUN/app.db-shm"
setsid nohup env DB_URL="sqlite://$RUN/app.db" HTTP_ADDR="127.0.0.1:8000" \
  SIMULATOR_ENABLED=false STALENESS_SECONDS=60s \
  "$RUN/server" > "$RUN/server.log" 2>&1 &
sleep 1.5

echo "==> Starting gateway polling your phone at $PHONE"
TAGS='plant-a.flotation.ph_level:pH|reg=hr:0:u16:0.01,plant-a.crushing.pulp_density:g/cm3|reg=hr:2:u16:0.001,plant-a.dewatering.dryer_temperature:C|reg=hr:4:u16:0.1,plant-a.dewatering.cake_moisture:%|reg=hr:6:u16:0.01'
setsid nohup env GATEWAY_ID="gw-phone-01" SERVER_URL="http://127.0.0.1:8000" \
  GATEWAY_BUFFER_DB="$RUN/phone-gateway-buffer.db" \
  POLL_INTERVAL=1s PUSH_INTERVAL=1s PULSE_INTERVAL=3s \
  SOURCE_DRIVER=modbus MODBUS_ADDR="$PHONE" MODBUS_UNIT_ID=1 MODBUS_TIMEOUT=2s \
  TAGS="$TAGS" "$RUN/gateway" > "$RUN/gateway.log" 2>&1 &
sleep 3

echo "==> Health"
curl -sf http://127.0.0.1:8000/api/v1/health >/dev/null && echo "    server: OK"
curl -sf http://127.0.0.1:8000/api/v1/gateways >/dev/null && echo "    gateways: OK"

echo
echo "Open http://127.0.0.1:8000  — change HR0..HR6 in the phone app and watch"
echo "the four values on the dashboard move within ~2 seconds."
echo "Logs: $RUN/server.log, $RUN/gateway.log"
