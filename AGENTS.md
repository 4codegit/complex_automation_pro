# CAP — AGENTS.md

Проект: платформа мониторинга обогатительной фабрики (Go backend + TS frontend + edge gateway).

## Обязательные проверки после изменений Go-кода

Рабочая директория бэкенда: `backend/`. Всегда запускай из неё:

```bash
cd backend
gofmt -l .                # пусто = ок
go vet ./...
go build ./...
go test ./... -timeout 100s
```

Не коммить и не считай задачу закрытой, пока все четыре не зелёные.

## Фронтенд

```bash
cd frontend
npm run build             # сборка; финальный артефакт копируется в backend/internal/web/web
```

## Живой прогон (демо)

- Сервер: `backend/cmd/server` (env: `DB_URL=sqlite:///path.db`, `HTTP_ADDR=127.0.0.1:8000`, `SIMULATOR_ENABLED=true`)
- Gateway: `backend/cmd/gateway` (env: `GATEWAY_ID`, `SERVER_URL`, `GATEWAY_BUFFER_DB`)
- Полный сценарий: `backend/demo.sh`
- Единый запуск: `backend/run.sh` (собирает фронт + бинари, стартует server+gateway, health-check)
- Health-check: `backend/scripts/health.sh` (проверка API+дашборд+симулятор, без запуска)
- Быстрый старт: `backend/scripts/start.sh` (только сервер, без шлюза и пересборки фронта)
- Проверочные WS-скрипты: `/tmp/opencode/ws-check*.mjs` (Node 22, встроенный WebSocket)

Запуск фоновых процессов: используй `setsid nohup ... &` — НЕ запускай сервер фоновой командой bash-тула с коротким `timeout`: SIGTERM по таймауту убивает группу и оставляет «полуживой» сервер.

## Известные ловушки

- `simulator.Manager`: при добавлении горутин всегда держи пару `wg.Add`/`defer wg.Done` — без этого `Stop()` виснет навсегда.
- SQLite-файлы (`*.db`, `-wal`, `-shm`) переиспользуются между перезапусками — при странных симптомах удаляй базу и начинай с чистой.
- Playwright: браузеры в `~/.cache/ms-playwright` (chromium-1234); chrome не установлен, используй chromium.
