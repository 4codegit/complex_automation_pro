# Local development

The platform runs directly on the workstation in Go. Docker is deliberately
not used at this stage. The demo needs no external services: the server uses a
SQLite file and embeds the dashboard.

## Prerequisites

- Go 1.22 or later
- Node.js 18 or later (only to rebuild the frontend; not needed to run)

## Run (one process)

```bash
cd backend
go run ./cmd/server        # http://127.0.0.1:8000
```

`http://127.0.0.1:8000` serves everything from a single process:

- the dashboard (embedded SPA) at `/`;
- the JSON API under `/api/v1`;
- the WebSocket live feed at `/api/v1/ws`;
- the development simulator (off unless `SIMULATOR_ENABLED=true`).

Open `http://127.0.0.1:8000` in a browser.

## Edge gateway (optional)

```bash
cd backend
go run ./cmd/gateway       # streams telemetry + heartbeats into the server
```

See `demo.sh` for the store-and-forward outage scenario.

## Configuration

Environment variables (defaults in `.env.example`):

| Variable | Description |
| -------- | ----------- |
| `DB_URL` | `sqlite://path.db` (default) or `postgres://...` |
| `HTTP_ADDR` | listen address, default `127.0.0.1:8000` |
| `SIMULATOR_ENABLED` | demo generator on/off; keep off when attaching real sources |
| `STALENESS_SECONDS` | mark telemetry as stale after this age |

## Frontend (only when changing the UI)

```bash
cd frontend
npm ci
npm run dev          # dev server with proxy to :8000
```

To publish a UI change into the single-binary demo:

```bash
npm run build
cp -r dist/* ../backend/internal/web/web/
```

## Verification

```bash
curl http://127.0.0.1:8000/api/v1/health
curl http://127.0.0.1:8000/api/v1/simulator/status
curl http://127.0.0.1:8000/api/v1/assets
```

## Quality gates (Go)

```bash
cd backend
gofmt -l .            # empty = ok
go vet ./...
go build ./...
go test ./... -timeout 100s
```