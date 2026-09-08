# Complex Automation Pro (CAP)

CAP is a vendor-neutral monitoring platform for mineral-processing operations,
covering the full flowsheet from ore receiving through crushing, grinding,
flotation, thickening, filtration, drying and concentrate dispatch.

The platform is written in **Go** (single static binary, SQLite for demo /
PostgreSQL for production). One process serves the dashboard, the JSON API and
the live WebSocket feed on the same origin.

## Local start

```bash
cd backend
go run ./cmd/server       # http://127.0.0.1:8000  (UI + API + WS)
```

Open `http://127.0.0.1:8000`. See [backend/README.md](backend/README.md) for the edge
gateway, the demo scenario (`backend/demo.sh`) and [LOCAL_DEVELOPMENT.md](LOCAL_DEVELOPMENT.md)
for the details.

## Product direction

- [Master plan (director's order) — full factory automation roadmap](MASTER_PLAN.md)
- [Sensor and signal catalogue by process area](SENSOR_CATALOGUE.md)
- [Development task specification (ТЗ)](TZ_DEVELOPMENT.md)
- [Technical specification and delivery plan](TECHNICAL_SPECIFICATION.md)
- [Architecture decisions and OT safety boundary](ARCHITECTURE_DECISIONS.md)
- [Program description for patent registration (RU)](PATENT_DESCRIPTION.md)

Licensed under the [MIT License](LICENSE).

The simulator and monitoring dashboard are not authorised to control industrial
equipment. Production deployment will be designed and approved as a dedicated
later phase.