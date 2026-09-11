#!/usr/bin/env bash
# Full demo procedure (TZ §16): builds and starts the trio, then walks the
# operator through every acceptance scenario. Requires curl + jq.
set -euo pipefail
cd "$(dirname "$0")/.."

say() { printf '\n\033[1;36m═══ %s ═══\033[0m\n' "$1"; }
pause() { echo "  (пауза ${1}с — показывайте UI)"; sleep "$1"; }

say "0/7 Запуск стенда"
bash backend/run.sh > /dev/null 2>&1 &
RUN_PID=$!
for _ in $(seq 1 60); do
  curl -sf http://127.0.0.1:8000/api/v1/health > /dev/null 2>&1 && break
  sleep 1
done
COOKIE=$(mktemp); trap 'rm -f "$COOKIE"' EXIT
curl -sf -c "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"username":"operator","password":"operator"}' \
  http://127.0.0.1:8000/api/v1/access/login > /dev/null
echo "  сервер: $(curl -sf http://127.0.0.1:8000/api/v1/health)"
echo "  UI:     http://127.0.0.1:8000  (operator/operator)"
pause 20

say "1/7 Нормальный режим: баланс сходится"
curl -sf -b "$COOKIE" http://127.0.0.1:8000/api/v1/metallurgy/summary | jq '{valid, epsilon: (.kpis[] | select(.id=="epsilon") | .value), balance_err: (.kpis[] | select(.id=="balance_err") | .value)}'
pause 30

say "2/7 Управление: lic301 SP 500 → 550 (перевод в АВТО)"
curl -sf -b "$COOKIE" -X PUT -H 'Content-Type: application/json' \
  -d '{"mode":"auto","confirm":true}' http://127.0.0.1:8000/api/v1/control/loops/lic301/mode > /dev/null
curl -sf -b "$COOKIE" -X PUT -H 'Content-Type: application/json' \
  -d '{"value":550,"confirm":true}' http://127.0.0.1:8000/api/v1/control/loops/lic301/setpoint | jq '{id, sp, out, mode}'
pause 90

say "3/7 Возмущение по руде: +20% крепости"
bash demo/scenario.sh 1 20
echo "  ожидание: P80 растёт → извлечение падает → тревога по хвостам"
pause 120
curl -sf -b "$COOKIE" http://127.0.0.1:8000/api/v1/metallurgy/summary | jq '{epsilon: (.kpis[] | select(.id=="epsilon") | .value)}'
echo "  тревоги:"; curl -sf -b "$COOKIE" http://127.0.0.1:8000/api/v1/alarms/active | jq -r '.[] | "\(.tag_id): \(.message)"' | head -5
echo "  → оператор квитирует в UI и поднимает дозу собирателя (fic301 SP 108→200)"
curl -sf -b "$COOKIE" -X PUT -H 'Content-Type: application/json' \
  -d '{"mode":"auto","confirm":true}' http://127.0.0.1:8000/api/v1/control/loops/fic301/mode > /dev/null
curl -sf -b "$COOKIE" -X PUT -H 'Content-Type: application/json' \
  -d '{"value":200,"confirm":true}' http://127.0.0.1:8000/api/v1/control/loops/fic301/setpoint > /dev/null
pause 150

say "4/7 Обрыв связи (сценарий 4) и восстановление"
bash demo/scenario.sh 4 1
pause 20
bash demo/scenario.sh 0 0
echo "  связь восстановлена; качество offline видно на тренде разрывами"
pause 20

say "5/7 Отказ насоса сгустителя (сценарий 6, 6 мин)"
bash demo/scenario.sh 6 360
echo "  ожидание: li401/ei401 растут → hi → hi_hi"
pause 180
echo "  тревоги:"; curl -sf -b "$COOKIE" http://127.0.0.1:8000/api/v1/alarms/active | jq -r '.[] | "\(.tag_id): \(.message)"' | head -6
echo "  → оператор берёт dic401 в РУЧНОЙ и поднимает насос (запись MV 80%)"
curl -sf -b "$COOKIE" -X PUT -H 'Content-Type: application/json' \
  -d '{"mode":"manual","confirm":true}' http://127.0.0.1:8000/api/v1/control/loops/dic401/mode > /dev/null
curl -sf -b "$COOKIE" -X PUT -H 'Content-Type: application/json' \
  -d '{"value":80,"confirm":true}' http://127.0.0.1:8000/api/v1/control/loops/dic401/output > /dev/null
pause 120

say "6/7 Дрейф XRF: детектор несогласованности данных"
bash demo/scenario.sh 2 1
pause 120
curl -sf -b "$COOKIE" http://127.0.0.1:8000/api/v1/metallurgy/summary | jq '{valid, note, metal_check: (.kpis[] | select(.id=="metal_check") | .status)}'
bash demo/scenario.sh 0 0

say "7/7 Отчёт: CSV выгрузки"
curl -sf -b "$COOKIE" "http://127.0.0.1:8000/api/v1/reports/readings/csv?limit=100" -o /tmp/cap-readings.csv
curl -sf -b "$COOKIE" "http://127.0.0.1:8000/api/v1/reports/alerts/csv?limit=100" -o /tmp/cap-alerts.csv
echo "  /tmp/cap-readings.csv ($(wc -l < /tmp/cap-readings.csv) строк), /tmp/cap-alerts.csv ($(wc -l < /tmp/cap-alerts.csv) строк)"
echo
echo "Демонстрация завершена. Сервер продолжает работать (логи в backend/.run/)."
wait $RUN_PID
