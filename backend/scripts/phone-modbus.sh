#!/usr/bin/env bash
# ТЕЛЕФОН В РОЛИ ДАТЧИКОВ (Modbus TCP, порт 5020)
#
# Схема демо: телефон раздаёт Wi-Fi и запускает приложение Modbus-сервера
# (например, «Modbus Server» из Play Market). Ноутбук подключается к этому
# Wi-Fi. Шлюз edge опрашивает телефон каждую секунду — значения «датчиков»
# оживают на мнемосхеме http://127.0.0.1:8000. Команды оператора с
# мнемосхемы (контуры lic301/fic301/dic401, ручные записи актуаторов)
# записываются обратно в holding-регистры телефона (FC6).
#
# Запуск:
#   scripts/phone-modbus.sh                  # авто-поиск телефона в сети
#   scripts/phone-modbus.sh 192.168.43.1     # IP телефона явно
#   scripts/phone-modbus.sh 192.168.43.1:5020
#
# Карта регистров в приложении телефона (0-based; если приложение считает
# 1-based, IR 0 = регистр 30001, HR 0 = 40001):
#
#   ДАТЧИКИ — Input Registers (FC4), 28 шт., IR 0..27:
#     IR 0  питатель, t/h ×0.1        IR 14 Cu питания, %Cu ×0.001
#     IR 2  мощность мельницы, kW ×0.1 IR 15 Cu концентрата, %Cu ×0.01
#     IR 8  уровень пульпы, mm ×1     IR 16 Cu хвостов, %Cu ×0.001
#     IR 10 pH пульпы ×0.01           IR 24 влажность кека, % ×0.01
#     IR 11 собиратель, ml/min ×1     IR 27 уровень бункера, % ×0.1
#     (полная карта — как у демо-стенда plantsim: internal/plantsim/regs.go)
#
#   ИСПОЛНИТЕЛЬНЫЕ МЕХАНИЗМЫ — Holding Registers (FC3/FC6), HR 0..6:
#     HR 0 питатель HC101, t/h ×0.1   HR 4 хвостовой затвор LC301, % ×0.1
#     HR 1 вода мельницы FC201 ×0.1   HR 5 насос сгущения FC401, % ×0.1
#     HR 2 собиратель FC301, ml/min   HR 6 цикл фильтра HI501, s
#     HR 3 пенообразователь FC302
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RUN="$ROOT/.run"
PORT=5020
mkdir -p "$RUN"

say() { printf '\n\033[1;36m══ %s ══\033[0m\n' "$1"; }

# ── 1. Адрес телефона: аргумент или авто-поиск (порт 5020) ────────────
find_phone() {
    local gw found subnets
    # Кандидаты №1: шлюзы всех дефолтных маршрутов (Wi-Fi телефона / USB-модем)
    # плюс классический адрес Android в режиме USB-tethering.
    for gw in $(ip -o route show default 2>/dev/null | awk '{print $3}' | sort -u) 192.168.42.129; do
        if timeout 1 bash -c "</dev/tcp/$gw/$PORT" 2>/dev/null; then
            echo "$gw"; return 0
        fi
    done
    # Кандидаты №2: параллельное сканирование всех своих /24 подсетей.
    subnets="$(ip -4 -o addr show scope global 2>/dev/null \
        | awk '{split($4,a,"/"); split(a[1],b,"."); print b[1]"."b[2]"."b[3]}' | sort -u)"
    [[ -n "$subnets" ]] || return 1
    printf '  сканируем подсети: %s (порт %s) …\n' "$(echo $subnets | tr '\n' ' ')" "$PORT" >&2
    found="$(for sn in $subnets; do seq 1 254 | sed "s/^/$sn./"; done \
        | xargs -P 64 -I{} bash -c \
            "timeout 0.4 bash -c '</dev/tcp/{}/$PORT' 2>/dev/null && echo {}" \
        2>/dev/null | head -n1 || true)"
    [[ -n "$found" ]] && echo "$found"
}

PHONE="${1:-}"
if [[ -z "$PHONE" ]]; then
    say "Автоматический поиск телефона (Modbus, порт $PORT)"
    if ! PHONE="$(find_phone)"; then
        echo "❌ Телефон не найден."
        echo "   Включите на телефоне Modbus-сервер (порт $PORT) и Wi-Fi, затем запустите:"
        echo "   scripts/phone-modbus.sh <ip-телефона>"
        exit 1
    fi
    echo "  ✅ найден: $PHONE"
fi
case "$PHONE" in *:*) ;; *) PHONE="$PHONE:$PORT" ;; esac

# ── 2. Перезапуск и сборка ────────────────────────────────────────────
say "Подготовка"
pkill -f "$RUN/capd" 2>/dev/null || true
pkill -f "$RUN/edge" 2>/dev/null || true
pkill -f "$RUN/plantsim" 2>/dev/null || true
fuser -k 8000/tcp >/dev/null 2>&1 || true
sleep 0.5
(cd "$ROOT" && go build -o "$RUN/capd" ./cmd/capd && go build -o "$RUN/edge" ./cmd/edge)
rm -f "$RUN/phone.db" "$RUN/phone.db-wal" "$RUN/phone.db-shm" "$RUN/phone-edge-buffer.db"

# ── 3. Сервер capd (симулятор не нужен — источник реальный: телефон) ──
say "Старт сервера capd (:8000)"
setsid env HTTP_ADDR="0.0.0.0:8000" DB_URL="sqlite://$RUN/phone.db" \
    GATEWAY_TOKEN="cap-demo-token" CONTROL_ENABLED=1 \
    "$RUN/capd" > "$RUN/phone-capd.log" 2>&1 </dev/null &
echo $! > "$RUN/phone-capd.pid"
for _ in $(seq 1 30); do
    curl -sf http://127.0.0.1:8000/api/v1/health >/dev/null 2>&1 && break
    sleep 0.5
done
curl -sf http://127.0.0.1:8000/api/v1/health >/dev/null || { echo "❌ capd не поднялся: $(tail -5 "$RUN/phone-capd.log")"; exit 1; }
echo "  сервер: OK"

# ── 4. Шлюз edge: опрос телефона 1 раз в секунду + запись команд (FC6) ─
say "Подключение к телефону $PHONE"
TAGS='plant.crushing.fi101:t/h|reg=ir:0:u16:0.1,plant.crushing.ei101:kW|reg=ir:1:u16:0.1,plant.grinding.ei201:kW|reg=ir:2:u16:0.1,plant.grinding.fi201:m3/h|reg=ir:3:u16:0.1,plant.grinding.pi201:kPa|reg=ir:4:u16:0.1,plant.grinding.di202:g/l|reg=ir:5:u16:1,plant.grinding.xi201:um|reg=ir:6:u16:1,plant.grinding.fi202:m3/h|reg=ir:7:u16:0.1,plant.flotation.li301:mm|reg=ir:8:u16:1,plant.flotation.fi301:m3/h|reg=ir:9:u16:1,plant.flotation.ai301:pH|reg=ir:10:u16:0.01,plant.flotation.qi301:ml/min|reg=ir:11:u16:1,plant.flotation.qi302:ml/min|reg=ir:12:u16:1,plant.flotation.di301:%sol|reg=ir:13:u16:0.1,plant.flotation.afi301:%Cu|reg=ir:14:u16:0.001,plant.flotation.afc301:%Cu|reg=ir:15:u16:0.01,plant.flotation.aft301:%Cu|reg=ir:16:u16:0.001,plant.flotation.wi301:t/h|reg=ir:17:u16:0.01,plant.flotation.wi302:t/h|reg=ir:18:u16:0.1,plant.thickening.li401:m|reg=ir:19:u16:0.01,plant.thickening.di401:%sol|reg=ir:20:u16:0.1,plant.thickening.ei401:%|reg=ir:21:u16:0.1,plant.thickening.fi401:g/t|reg=ir:22:u16:0.1,plant.filtration.pi501:kPa|reg=ir:23:u16:0.1,plant.filtration.mi501:%|reg=ir:24:u16:0.01,plant.filtration.wi501:t/h|reg=ir:25:u16:0.01,plant.crushing.tit101:C|reg=ir:26:u16:0.1,plant.crushing.si101:%|reg=ir:27:u16:0.1,plant.crushing.hc101:t/h|reg=hr:0:u16:0.1,plant.grinding.fc201:%|reg=hr:1:u16:0.1,plant.flotation.fc301:ml/min|reg=hr:2:u16:1,plant.flotation.fc302:ml/min|reg=hr:3:u16:1,plant.flotation.lc301:%|reg=hr:4:u16:0.1,plant.thickening.fc401:%|reg=hr:5:u16:0.1,plant.filtration.hi501:s|reg=hr:6:u16:1'
setsid env GATEWAY_ID="gw-phone-01" GATEWAY_TOKEN="cap-demo-token" \
    SERVER_URL="http://127.0.0.1:8000" GATEWAY_BUFFER_DB="$RUN/phone-edge-buffer.db" \
    SOURCE_DRIVER="modbus" MODBUS_ADDR="$PHONE" MODBUS_UNIT_ID=1 MODBUS_TIMEOUT=2s \
    POLL_INTERVAL=1s PUSH_INTERVAL=1s PULSE_INTERVAL=3s \
    CONTROL_ENABLED=1 CONTROL_POLL=1s \
    TAGS="$TAGS" "$RUN/edge" > "$RUN/phone-edge.log" 2>&1 </dev/null &
echo $! > "$RUN/phone-edge.pid"

# ── 5. Самопроверка: данные реально идут с телефона? ──────────────────
sleep 4
COOKIE="$(mktemp)"
curl -sf -c "$COOKIE" -H 'Content-Type: application/json' \
    -d '{"username":"operator","password":"operator"}' \
    http://127.0.0.1:8000/api/v1/access/login >/dev/null
for tag in plant.flotation.ai301 plant.crushing.fi101; do
    v="$(curl -sf -b "$COOKIE" "http://127.0.0.1:8000/api/v1/telemetry/latest?tag_id=$tag" | jq -r '.value // "нет данных"')"
    echo "  $tag = $v"
done
rm -f "$COOKIE"
grep -q "WRITE BRIDGE ACTIVE" "$RUN/phone-edge.log" && echo "  запись команд в телефон: включена"

# ── 6. Что делать дальше ──────────────────────────────────────────────
MYIP="$(ip -4 addr show scope global | awk '/inet /{split($2,a,"/"); print a[1]; exit}')"
say "Готово: телефон $PHONE подключён"
echo "  Мнемосхема:   http://127.0.0.1:8000   (логин operator/operator)"
[[ -n "$MYIP" ]] && echo "  С телефона:    http://$MYIP:8000"
echo
echo "  Рецепт демонстрации:"
echo "   1. Поменяйте в приложении IR 10 (pH, ×0.01): 950 → 9.50 на схеме за ~2 с."
echo "   2. IR 14 (Cu питания ×0.001) 1400 → 1.400 %Cu — пересчитается извлечение."
echo "   3. Схема → контур LIC301 → режим АВТО, SP 550 — платформа запишет"
echo "      выход в HR 4 телефона (значение в приложении изменится само)."
echo
echo "  Логи:  tail -f $RUN/phone-{capd,edge}.log"
echo "  Стоп:  kill \$(cat $RUN/phone-{capd,edge}.pid)"
