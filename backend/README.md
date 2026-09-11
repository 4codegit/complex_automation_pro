# CAP — АСУ ТП участка флотационного обогащения руды

SCADA-класс система оперативного диспетчерского контроля и управления: сбор
телеметрии по **реальному Modbus TCP**, металлургический баланс, супервизорные
ПИД-контуры, тревоги по ISA-18.2 и Web-HMI в духе ISA-101. Русский интерфейс.

## Архитектура (модульный монолит + два внешних процесса)

```
                    ┌──────────────────────────────────────────┐
                    │  capd — единственный сервер :8000        │
                    │  api · auth · ingest · historian ·       │
                    │  alarms · control · metallurgy ·         │
                    │  registry · hub · store · web (SPA)      │
                    └────────▲──────────────────┬──────────────┘
              HTTP batch     │                  │ GET /control/output
   ┌──────────┐  Modbus TCP   │      FC6-мост    │
   │ plantsim │◄──────────────┴──────────────────┤
   │ :1502/:15080                            │
   └──────────┘        edge (шлюз сбора) ────┘
   (демо-стенд)        store-and-forward буфер
```

- **capd** — модульный монолит: один бинарник, одна БД, один порт. Модули —
  пакеты `internal/*` с секциями маршрутов. SQL только в `internal/store`,
  формулы — в `internal/metallurgy` и `internal/plantsim`.
- **edge** — шлюз сбора: опрос Modbus (1 с), нормализация к контракту
  `internal/schema`, буферизация при недоступности сервера (SQLite), доставка
  батчами с подтверждением, пульс состояния. Запись актуаторов — FC6-мост
  (только режим АВТО контуров + разовые ручные команды; при потере сервера
  удерживает последнее значение).
- **plantsim** — демонстрационный стенд: матмодель обогащения (дробление →
  измельчение → флотация → сгуститель → фильтр) за настоящим Modbus TCP
  сервером (FC3/FC4/FC6, ручной MBAP) + HTTP для сценариев. В продакшене
  вместо него — контроллеры завода.

Единственный путь телеметрии: `plantsim/ПЛК → Modbus → edge → ingest
(идемпотентно) → capd (historian + WS + тревоги)`. Виртуальные расчётные теги
(`calc_*`) идут через тот же контракт.

## Сборка и запуск

```bash
# всё сразу (стенд + сервер + шлюз, свежая БД):
cd backend && ./run.sh
# открыть http://127.0.0.1:8000 — вход operator/operator или admin/admin

# фронтенд отдельно в dev-режиме (прокси на :8000):
cd frontend && npm run dev

# скриптованная демонстрация (7 сценариев, ~15 мин):
demo/run-demo.sh
```

`go build ./... && go vet ./... && go test ./...` — зелёные; фронтенд
`npm run build` без ошибок TypeScript.

## Переменные окружения (сервер)

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `HTTP_ADDR` | `127.0.0.1:8000` | адрес capd |
| `DB_URL` | `sqlite://./cap.db` | SQLite или `postgres://` |
| `GATEWAY_TOKEN` | — | общий ключ шлюзов для машинных endpoint'ов ingest |
| `PLANTSIM_URL` | — | HTTP стенда (включает `POST /api/v1/scenario`) |
| `CONTROL_ENABLED` | `false` | запуск супервизорных ПИД-контуров |
| `CONTROL_STALE` | `10s` | watchdog: возраст PV до заморозки контура |

Шлюз: `SOURCE_DRIVER=modbus`, `MODBUS_ADDR`, `MODBUS_UNIT_ID`, `POLL_INTERVAL`,
`TAGS` (карта `asset.tag:unit|reg=<fc>:<addr>:<тип>:<масштаб>`),
`GATEWAY_TOKEN` (тот же ключ), `CONTROL_ENABLED`, `CONTROL_POLL`.
Полный пример — `.env.example`.

## Модель данных и расчёты

- 43 тега: 28 датчиков + 7 актуаторов + 8 расчётных (`calc_epsilon`,
  `calc_gamma`, `calc_upgrade`, `calc_pull`, `calc_balance_err`,
  `calc_q_collector`, `calc_bond_kwt`, `calc_cl_pct`).
- Металлургия: двухпродуктовая формула (ε, γ, K), невязка баланса, кросс-проверка
  извлечений, энергия Бонда, циркулирующая нагрузка, удельные расходы. Каждый
  KPI несёт строку формулы и теги-источники (аудируемость расчёта).
- Тревоги: ISA-18.2 — рационализированные уставки, задержки 3/10 с, гистерезис
  1% шкалы, журнал `alarm_events`, comm_loss по активам, квитирование из сессии.
- Контур управления: 3 петли (уровень флотомашины, удельный собиратель,
  плотность сгущения), режимы АВТО/РУЧН с безбамперным переходом от позиции
  поля, watchdog PV, подтверждение записей, полный аудит.

## Безопасность (MVP)

Локальные учётки (PBKDF2-SHA256), серверные сессии в HttpOnly-cookie, RBAC по
ролям, аудит мутаций. Шлюзы аутентифицируются общим `GATEWAY_TOKEN`. Seed-пароли
демонстрационные — сменить перед реальной эксплуатацией. SameSite=Lax +
same-origin; OIDC/LDAP — вне рамок MVP.

## Структура

```
backend/
  cmd/capd        сервер-монолит       internal/store       SQL (SQLite/PG)
  cmd/edge        шлюз сбора           internal/api         HTTP + WS
  cmd/plantsim    стенд                internal/alarms      движок ISA-18.2
  internal/gateway  драйверы Modbus/OPC UA/Sparkplug, буфер, FC6-мост
  internal/plantsim модель процесса + Modbus TCP сервер + сценарии
  internal/metallurgy металлургический баланс
  internal/controlsvc супервизорные петли   internal/control  ядро ПИД
  internal/auth     PBKDF2 + сессии       internal/hub        шина событий
frontend/         React 18 + TS + Vite + Tailwind (встраивается в capd)
demo/             run-demo.sh, scenario.sh
```
