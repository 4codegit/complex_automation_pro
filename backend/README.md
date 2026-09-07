# CAP Go platform

The production platform is written in Go (decision in `../ARCHITECTURE_DECISIONS.md`):
one language for the edge gateway and the server, one static binary per service,
no external services required for a demo (SQLite by default,
PostgreSQL for production, both via the same code).

## Layout

```
cmd/server/      HTTP API: push ingestion, pull queries, registry, alarms,
                 WebSocket live fan-out, development simulator
cmd/gateway/     edge collector: sensor polling, canonical normalization,
                 store-and-forward buffer (SQLite/WAL), batch delivery with
                 exponential backoff, heartbeat pulse, backfill on recovery
internal/
  config/        settings + .env loader
  schema/        canonical telemetry contract + validation (shared by gateway)
  store/         database: open by DSN, portable migrations, models, repository
  hub/           in-process pub/sub for live dashboard events
  api/           HTTP handlers, router (standard library mux), WebSocket
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

1. **Poll** local instruments (simulated sensor by default; the adapter
   interface is where Modbus TCP / OPC UA / MQTT Sparkplug B plug in) and
   normalize into the canonical contract.
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
