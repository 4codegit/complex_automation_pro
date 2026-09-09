# CAP Go platform

The production platform is written in Go (decision in `../ARCHITECTURE_DECISIONS.md`):
one language for the edge gateway and the server, one static binary per service,
no external services required for a demo (SQLite by default,
PostgreSQL for production, both via the same code).

## Layout

```
cmd/server/      all-in-one development bundle: every route section in one
                 process (dev convenience, same code as the services)
cmd/gateway-api/ split-mode entry point: reverse proxy routing /api/v1 and
                 the dashboard to the microservices (no DB of its own)
cmd/live/        live service: SPA, WebSocket fan-out, simulator, event intake
cmd/ingest/      ingest service: push ingestion (telemetry, gateway events)
cmd/historian/   historian service: history, latest, aggregates, CSV reports
cmd/alarms/      alarm service: active alarms, ack, rationalised limits
cmd/profiles/    profile service: ore profile change control
cmd/registry/    registry service: assets, tags, gateway registry
cmd/identity/    identity service: RBAC, assignments, audit trail
cmd/gateway/     edge collector: sensor polling, canonical normalization,
                 store-and-forward buffer (SQLite/WAL), batch delivery with
                 exponential backoff, heartbeat pulse, backfill on recovery
internal/
  config/        settings + .env loader (+ EVENT_SINKS)
  schema/        canonical telemetry contract + validation (shared by gateway)
  store/         database: open by DSN, portable migrations, models, repository
  hub/           in-process pub/sub for live dashboard events
  api/           HTTP handlers, sectioned router (standard library mux), WS
  service/       shared composition root for every deployable
  simulator/     deterministic demo telemetry generator
  gateway/       edge collector subsystem (buffer, sender, pulse, simulated sensor)
```

## Run (development)

```bash
cd backend
cp .env.example .env        # optional; defaults already match this file
go run ./cmd/server
```

The API is then available at `http://127.0.0.1:8000`. The same origin serves the
embedded dashboard (UI + API + WebSocket in one process), built from the
frontend into `internal/web/web` (see "Frontend" below).

### Endpoints (Go extensions beyond the Python MVP)

| Method | Path | Purpose |
| ------ | ---- | ------- |
| POST | `/api/v1/ingest/gateway_events` | edge gateway heartbeat (pulse/online/offline, buffer depth, latency) |
| GET | `/api/v1/gateways` | registered gateways with last known state |
| GET | `/api/v1/telemetry/aggregate` | downsample numerics into buckets (`resolution=30s\|1m\|5m\|1h\|1d`, `agg=avg\|min\|max\|sum\|count\|last`) |
| GET | `/api/v1/analytics/process` | derived metallurgical KPIs: percent solids from pulp density, specific reagent consumption, dry throughput, profile-corridor statuses (formulas exposed per KPI) |
| GET | `/api/v1/alarms/active` | current ISA-18.2-style alarm states |
| POST | `/api/v1/alarms/{id}/ack` | acknowledge an alarm (`{"ack_by": ..., "comment": ...}`) |
| GET | `/api/v1/profiles` | ore profiles, newest first |
| GET | `/api/v1/profiles/active` | currently active ore profile |
| POST | `/api/v1/profiles` | create a draft (`params` is a JSON object of thresholds/baselines) |
| POST | `/api/v1/profiles/{id}/approve` | approve a draft (change control) |
| POST | `/api/v1/profiles/{id}/activate` | switch the plant to an approved profile |

`/api/v1/telemetry/latest` additionally reports `stale: true` and
`age_seconds` when the newest sample is older than `STALENESS_SECONDS`.
The simulator (development only) reads the active ore profile: its thresholds
drive breach detection, and it maintains/clears alarm states automatically.

## Edge gateway (store-and-forward)

`go run ./cmd/gateway` runs the edge collector:

1. **Poll** local instruments (simulated sensor by default; `SOURCE_DRIVER`
   selects `opcua`, `modbus` or `sparkplug` — all read-only, per-tag address
   specs in `TAGS`) and normalize into the canonical contract. The sparkplug
   driver acts as a Sparkplug B Primary Host Application: it subscribes to
   `spBv1.0/#` and never publishes.
2. **Buffer** every message in a local SQLite/WAL file (`GATEWAY_BUFFER_DB`)
   — survives restarts, survives server outages.
3. **Deliver** as `:batch` batches; the queue is drained only on server
   confirmation: `accepted`/`duplicate` are removed, `rejected` goes to the
   resync register (`gateway-rejected.log`).
4. **Backoff** exponentially on transport failures (1s→10s), and the heartbeat
   wakes the sender the moment the server is reachable again — the queued data
   backfills immediately.
5. **Pulse** every `PULSE_INTERVAL` to `/api/v1/ingest/gateway_events` so the
   server tracks online/offline state and buffer depth per gateway.

Demo of the failure mode (see `demo.sh`): server up → gateway streams →
server killed → gateway keeps polling and buffering → server restarted →
gateway backfills the whole outage window, nothing is lost.

## Split mode (microservices)

The same codebase deploys as independent processes. `api.Routes` takes route
sections (`ingest`, `historian`, `alarms`, `profiles`, `registry`, `identity`,
`live`); each `cmd/<service>` binary mounts exactly one, `cmd/server` mounts
all of them. `cmd/gateway-api` fronts the services on a single address; the
ingest service forwards accepted readings to the live service over
`EVENT_SINKS` so the dashboard keeps its real-time feed.

```bash
cd backend
scripts/services.sh start   # live(:8001) → ingest/historian/alarms/profiles/
                            # registry/identity (:8002-8007) → gateway-api(:8000)
scripts/services.sh status  # running services
scripts/services.sh stop    # stop everything
```

Services share the database (`CAP_DB_URL`, default `sqlite://.run/cap.db` —
SQLite/WAL with busy_timeout is multi-process safe); the live service starts
first and owns migrations/seeds. `gateway-api` upstreams are overridable per
service (`INGEST_UPSTREAM`, `HISTORIAN_UPSTREAM`, ...).

## Test

```bash
go test ./...
```

Contract tests in `internal/api` re-express the canonical contract tests
(idempotency, batch per-item results, unknown/incompatible tags, schema rejects).

## Storage drivers

`DB_URL` selects the driver:

- `sqlite://./cap.db` — pure-Go SQLite (default, demo, tests). No CGO, no services.
- `postgres://user:pass@host:5432/dbname` — PostgreSQL (production). Same code path.

Migrations are portable DDL tracked in `schema_migrations`.

## Frontend (embedded SPA)

The dashboard is a React/Vite app in `../frontend`. Rebuild and embed after any
UI change:

```bash
cd ../frontend && npm run build
cp -r dist/* ../backend/internal/web/web/
cd ../backend && go build ./...
```

Restart the server and open `http://127.0.0.1:8000`: the single process serves
the HTML, hashed assets, the API and the WebSocket feed.

## Contract freeze

The API contract (v1 under `/api/v1`) is documented in `../TZ_DEVELOPMENT.md` and
exercised by the contract tests. Any API change requires updating that document
and the contract tests first.
