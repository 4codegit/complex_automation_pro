#!/usr/bin/env bash
# Builds and starts the CAP demo trio: capd (server), edge (collector) and
# the plantsim stand. See demo/run-demo.sh for the scripted demo procedure.
set -euo pipefail
cd "$(dirname "$0")"

mkdir -p .run

# Idempotent restart: stop any previous demo processes first.
pkill -f "\.run/capd" 2>/dev/null || true
pkill -f "\.run/edge" 2>/dev/null || true
pkill -f "\.run/plantsim" 2>/dev/null || true
sleep 1

echo "── сборка"
go build -o .run/capd ./cmd/capd
go build -o .run/edge ./cmd/edge
go build -o .run/plantsim ./cmd/plantsim

# Fresh demo database every run.
rm -f ../.run/cap.db ../.run/cap.db-wal ../.run/cap.db-shm ../.run/gateway-buffer.db

echo "── старт plantsim (:1502 Modbus, :15080 http)"
PLANTSIM_MODBUS_ADDR="127.0.0.1:1502" PLANTSIM_HTTP_ADDR="127.0.0.1:15080" \
  .run/plantsim > .run/plantsim.log 2>&1 &
echo $! > .run/plantsim.pid

echo "── старт capd (:8000)"
HTTP_ADDR="127.0.0.1:8000" DB_URL="sqlite://../.run/cap.db" GATEWAY_TOKEN="cap-demo-token" \
  CONTROL_ENABLED=1 PLANTSIM_URL="http://127.0.0.1:15080" \
  .run/capd > .run/capd.log 2>&1 &
echo $! > .run/capd.pid

for _ in $(seq 1 30); do
  if curl -sf http://127.0.0.1:8000/api/v1/health > /dev/null 2>&1; then break; fi
  sleep 0.5
done

echo "── старт edge (Modbus-опрос стенда, 1 c)"
GATEWAY_ID="gw-plant-01" GATEWAY_TOKEN="cap-demo-token" \
SERVER_URL="http://127.0.0.1:8000" \
GATEWAY_BUFFER_DB="../.run/gateway-buffer.db" \
SOURCE_DRIVER="modbus" MODBUS_ADDR="127.0.0.1:1502" MODBUS_UNIT_ID=1 \
POLL_INTERVAL=1s PUSH_INTERVAL=1s PULSE_INTERVAL=5s \
CONTROL_ENABLED=1 CONTROL_POLL=1s \
TAGS='plant.crushing.fi101:t/h|reg=ir:0:u16:0.1,plant.crushing.ei101:kW|reg=ir:1:u16:0.1,plant.grinding.ei201:kW|reg=ir:2:u16:0.1,plant.grinding.fi201:m3/h|reg=ir:3:u16:0.1,plant.grinding.pi201:kPa|reg=ir:4:u16:0.1,plant.grinding.di202:g/l|reg=ir:5:u16:1,plant.grinding.xi201:um|reg=ir:6:u16:1,plant.grinding.fi202:m3/h|reg=ir:7:u16:0.1,plant.flotation.li301:mm|reg=ir:8:u16:1,plant.flotation.fi301:m3/h|reg=ir:9:u16:1,plant.flotation.ai301:pH|reg=ir:10:u16:0.01,plant.flotation.qi301:ml/min|reg=ir:11:u16:1,plant.flotation.qi302:ml/min|reg=ir:12:u16:1,plant.flotation.di301:%sol|reg=ir:13:u16:0.1,plant.flotation.afi301:%Cu|reg=ir:14:u16:0.001,plant.flotation.afc301:%Cu|reg=ir:15:u16:0.01,plant.flotation.aft301:%Cu|reg=ir:16:u16:0.001,plant.flotation.wi301:t/h|reg=ir:17:u16:0.01,plant.flotation.wi302:t/h|reg=ir:18:u16:0.1,plant.thickening.li401:m|reg=ir:19:u16:0.01,plant.thickening.di401:%sol|reg=ir:20:u16:0.1,plant.thickening.ei401:%|reg=ir:21:u16:0.1,plant.thickening.fi401:g/t|reg=ir:22:u16:0.1,plant.filtration.pi501:kPa|reg=ir:23:u16:0.1,plant.filtration.mi501:%|reg=ir:24:u16:0.01,plant.filtration.wi501:t/h|reg=ir:25:u16:0.01,plant.crushing.tit101:C|reg=ir:26:u16:0.1,plant.crushing.si101:%|reg=ir:27:u16:0.1,plant.crushing.hc101:t/h|reg=hr:0:u16:0.1,plant.grinding.fc201:%|reg=hr:1:u16:0.1,plant.flotation.fc301:ml/min|reg=hr:2:u16:1,plant.flotation.fc302:ml/min|reg=hr:3:u16:1,plant.flotation.lc301:%|reg=hr:4:u16:0.1,plant.thickening.fc401:%|reg=hr:5:u16:0.1,plant.filtration.hi501:s|reg=hr:6:u16:1' \
  .run/edge > .run/edge.log 2>&1 &
echo $! > .run/edge.pid

echo
echo "Готово: http://127.0.0.1:8000  (логин operator/operator или admin/admin)"
echo "Логи:   .run/{capd,edge,plantsim}.log"
echo "Стоп:   kill \$(cat .run/{capd,edge,plantsim}.pid)"
wait
