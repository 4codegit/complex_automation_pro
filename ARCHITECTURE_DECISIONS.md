# Architecture decisions

## Current boundary

CAP is currently a development monitoring system. It may collect, validate, display, archive and alert on telemetry. It must not issue direct commands to PLCs, DCSs, variable-speed drives, valves, pumps, crushers, filters, dryers, or safety systems.

The process safety layer, interlocks, emergency shutdown, and local control remain independent of CAP. A loss, defect, or compromise of CAP must not stop or destabilize production.

## Reference model

Use the ISA-95 logical separation:

- Levels 0-2: physical process, sensors, actuators, PLC/DCS and protection systems.
- Level 3: CAP edge collection, historian, monitoring, alarming, reporting and operator-facing applications.
- Level 4: ERP, laboratory systems, planning and business analytics.

CAP initially belongs at Level 3. It receives a read-only or brokered data flow from the OT network. Any future control capability requires a separately approved design, hazard study, vendor/plant engineering review, access control, audit trail, test environment and formal commissioning.

## Target components

1. Edge gateway per process area: protocol adapters, store-and-forward buffer, time synchronization and health status.
2. Ingestion service: versioned API, schema validation, deduplication, idempotency and authentication.
3. Time-series historian: immutable raw measurements plus aggregations and retention policies.
4. Asset and tag registry: an equipment hierarchy, sensor metadata, units, quality codes, calibration state and alarm limits.
5. Alarm and event service: separate alarms from notifications; acknowledgements, shelving, escalation and audit log.
6. Monitoring UI: current values, trend, equipment state, communication loss, alarm state and data-quality state.
7. Integration API: pull queries and push ingestion without exposing database internals.

## Technology direction

Decision (2026-07-31, director review): **the platform is Go.** Rust is excluded at platform level.

Rationale:

- CAP is Level 3 monitoring (no control loops, no deterministic-latency requirement; real-time is held by PLC/DCS). The strengths of Rust (predictable latency, no GC, max performance, memory safety without GC) provide no measurable benefit here, while its cost (development speed, talent availability, compile time, younger OPC UA ecosystem) is real for a small team shipping a sellable product.
- One language for the whole platform: the edge gateway and the ingestion server share a single Go module for the canonical telemetry schema, so the wire contract cannot drift.
- One static binary per service, no Python runtime/venv/dependencies on customer OT servers — a decisive deployment advantage for a product installed on multiple plants.
- Go headroom (~100k+ messages/s) far exceeds the target (~5k messages/s), so the hot path is not a future risk.
- Mature industrial-protocol ecosystem: gopcua/opcua, goburrow/modbus, eclipse/paho MQTT; embedded pure-Go SQLite (modernc.org/sqlite) for store-and-forward.

The existing Python/FastAPI MVP is being ported 1:1 to Go with the `backend/API.md` contract frozen and its contract tests re-expressed in Go. Rust remains justified only for a narrow, measured per-driver need after profiling; it is a module decision, not a platform one. Optional Python remains only as a thin analytics microservice (metallurgical balance/reconciliation), never as platform infrastructure.

## Non-negotiable engineering controls

- No Internet-to-PLC path and no shared credentials between IT and OT.
- Mutual authentication, least privilege, immutable audit events and key rotation for every gateway.
- UTC timestamps, unique message IDs, source sequence numbers and explicit data-quality codes.
- Store-and-forward at the edge; defined behaviour for disconnect, retry, duplicate and late telemetry.
- Backup/restore drills, monitoring of monitoring, capacity limits and change control.
- Alarm limits are plant-approved configuration, never hard-coded universal values.

## Standards to use as design references

- ISA-95 / IEC 62264 for Level 3/Level 4 boundaries and equipment hierarchy.
- ISA-18.2 / IEC 62682 for alarm lifecycle and rationalisation.
- IEC 62443 and NIST SP 800-82 for OT segmentation and security controls.

## ADR-001: Edge source abstraction (2026-08-04)

Status: accepted. Scope: `internal/gateway/`.

The edge gateway currently couples polling to a single concrete simulated
`Sensor` (`sensor.go`). To support real field protocols (OPC UA first, later
Modbus TCP and MQTT Sparkplug B) without forking the runner, abstraction is
introduced at the **source boundary**:

```go
// Source emits canonical telemetry messages for a gateway.
// Implementations: simulated Sensor, opcua.Source, (future) modbus, mqtt.
// Sources MUST be safe for concurrent use by exactly one Polling loop.
type Source interface {
    // Name returns the driver id for logs/metrics, e.g. "opcua", "simulated".
    Name() string
    // Poll reads every configured tag once at t and returns canonical messages.
    // Implementations that maintain long-lived subscriptions MAY return a
    // delta-only slice; the runner treats an empty slice as "no new data".
    Poll(ctx context.Context, now time.Time) ([]schema.Telemetry, error)
    // Close releases the underlying session/connection. Must be idempotent.
    Close() error
}
```

Why an interface and not a plugin system:

- `Runner` already does everything past `Poll` (buffer, sender, pulse,
  rejected-log). Abstracting only the source keeps one binary, one config path
  and one store-and-forward pipeline for every protocol.
- `Poll` returns the same `[]schema.Telemetry` the simulator already produces,
  so no canonical-contract churn and no serializer change.

Read-only Level 3 invariant: `Source` exposes **read** operations only.
`Poll`/subscriptions MUST NOT provide a write path to the OT asset; control
remains outside CAP. A future write capability, if ever approved, would be
a separate, audited interface behind a different permission boundary.

### OPC UA driver (Block B1) — design

Driver: `gopcua/opcua` (pure Go, already surfaced in the technology direction).
Two collection modes, configurable per tag:

1. **Poll** — periodic `Read` on the configured `ns=...;s=...` NodeID at
   `POLL_INTERVAL`. Fits tags with low scan rates and instruments that lack
   subscriptions.
2. **Subscription** — `Subscribe`/`MonitoredItem` with a publishing interval;
   `Poll` returns the values accumulated since the previous call (delta-only,
   preserving the `ObservedAt` of the underlying `DataChangeNotification`).
   This keeps the response-time guarantees of OPC UA subscriptions while
   staying inside the existing poll-tick loop.

Quality mapping (OPC UA `StatusCode` → CAP canonical):

| OPC UA status                     | CAP quality   |
|-----------------------------------|-------------------|
| `Good` (0x00000000)               | `good`            |
| `Uncertain*` (0x40xxxxxx)         | `uncertain`       |
| `Bad*`        (0x80xxxxxx)        | `bad`             |
| no data within `OVERRIDE_STALE`   | `stale`           |
| session lost / reconnect buffer   | `offline`         |

Security: `opc.tcp://` with `SecurityMode SignAndEncrypt` + `Basic256Sha256`
where the server supports it; anonymous only for lab/demo endpoints. The
gateway does **not** cache credentials — `username/password` lives in the
gateway env / a local OS-keychain-backed secret and is re-read on session
reconnect.

Reconnect: exponential backoff on `BadSessionClosed`; local buffer absorbs
during outage; `QualityOffline` is emitted once per outage (not per tick) so the
historian records one gap, not a flood.

Subscription mode (B1.2, 2026-08-04): when `OPCUA_MODE=subscribe` the source
creates one OPC UA `Subscription` with `MonitoredItem` entries for all tags.
The gopcua `notifyCh` is drained in a background goroutine into a delta buffer,
which `Poll` returns at each tick. An empty slice means "no new data" — the
runner treats it as a non-event. `OPCUA_SUB_INTERVAL` (default 500ms) drives
the publishing interval; `LifetimeCount` and `MaxKeepAliveCount` are derived
multiplicatively from it. Default mode is `poll`, so existing envs keep working
unchanged.

Configuration (env, overrides the gateway defaults):

```
SOURCE_DRIVER=opcua                 # simulated | opcua
OPCUA_ENDPOINT=opc.tcp://host:4840  # required when SOURCE_DRIVER=opcua
OPCUA_MODE=poll                    # poll (default) | subscribe
OPCUA_SUB_INTERVAL=500ms           # publishing interval in subscribe mode
OPCUA_SECURITY=auto                 # auto | none | sign | signandencrypt
OPCUA_POLICY=Basic256Sha256         # auto | None | Aes128/Aes256Sha256RsaPss
OPCUA_AUTH=anonymous                # anonymous | username
OPCUA_USERNAME=...                  # when username
OPCUA_PASSWORD=...                  # when username (read once at start)
OPCUA_CERT_DIR=/var/lib/cap/certs # x509 client cert/key per gateway
TAGS=asset.tag.metric:unit|node=2;s=Tag.PV, ...
```

The `:` separator for unit already exists; the appended `|node=<NodeID>`
node spec is parsed by the opcua driver and ignored by `simulated`, so
existing demo envs keep working unchanged. The pipe `|` (not `=`) separates
the unit from the node spec because OPC UA NodeIDs contain `=` (e.g.
`ns=2;s=Sim.PV`) and `:` (e.g. opaque byte-string NodeIDs).

Phasing: B1.1 ships the `Source` interface + simulated refactor + a
`gopcua/opcua` poll-only driver behind `SOURCE_DRIVER=opcua`. Subscriptions
land as B1.2 once the poll path is green in a lab. Modbus/MQTT remain future
work referenced from this ADR.

## ADR-002: Audit trail before auth (2026-08-04)

Status: accepted. Scope: `internal/store/audit.go`, `internal/api/audit_helper.go`,
`internal/api/handlers_audit.go`.

CAP Level 3 boundary requires an immutable audit trail "who changed what,
when" for every platform mutation. Full OIDC/JWT auth is a downstream ADR; this
one ships the **record side** first because it is valuable alone and because
retrofitting audit later requires either replaying history or accepting a gap
in the trail.

Tables: `audit_events(id, occurred_at, actor, role, action, resource_type,
resource_id, detail, src_ip)` with indexes on `occurred_at`, `(resource_type,
resource_id)` and `actor`. Rows are append-only: no UPDATE/DELETE is permitted
from the API. The simulator and OT-side reads are **not** audited — only human
mutations of registry, profiles, alarm rationalisation and acknowledgements.

Mutations covered in this iteration (the list grows as new writes land):

| Action                     | Endpoint                                              |
|----------------------------|-------------------------------------------------------|
| `asset.create`            | `POST /api/v1/assets`                                 |
| `asset.update`             | `PUT /api/v1/assets/{id}`                             |
| `asset.delete`             | `DELETE /api/v1/assets/{id}`                          |
| `tag.create`               | `POST /api/v1/tags`                                   |
| `tag.update`               | `PUT /api/v1/tags/{id}`                               |
| `tag.delete`               | `DELETE /api/v1/tags/{id}`                            |
| `profile.create`           | `POST /api/v1/profiles`                               |
| `profile.approve`          | `POST /api/v1/profiles/{id}/approve`                  |
| `profile.activate`         | `POST /api/v1/profiles/{id}/activate`                 |
| `alarm.ack`                | `POST /api/v1/alarms/{id}/ack`                        |
| `alarm_limit.rationalise`  | `PUT /api/v1/alarms/limits/{tag_id}`                  |
| `alarm_limit.delete`       | `DELETE /api/v1/alarms/limits/{tag_id}`               |

Actor resolution today reads the `X-User` and `X-Role` headers (unknown →
`anonymous`). A future OIDC middleware replaces this extraction, not the
`audit()` call sites — so adding SSO does not duplicate work.

Audit is best-effort: the `Server.audit` helper logs a write failure at warn
level but does not return an error to the caller. Losing one audit row is
preferable to blocking the action it records, and the audit trail surviving
a partial outage is exactly why it is append-only.

## ADR-003: Alarm rationalisation before live OT (2026-08-04)

Status: accepted. Scope: `internal/store/audit.go` (AlarmLimit struct +
UpsertAlarmLimit/etc), `internal/api/handlers_audit.go`.

Alarm limits are plant-approved per-tag configuration. **No universal
hard-coded thresholds** is a non-negotiable engineering control in this
document, and rationalisation is the mechanism that operationalises it.

Table: `alarm_limits(tag_id PRIMARY KEY, enabled, lo_lo, lo, hi, hi_hi,
severity, rationalised_by, rationalised_at, notes)`. One row per tag — a new
rationalisation overwrites the previous one, with the old setting preserved as
an audit event (action `alarm_limit.rationalise`, full JSON of the new limits
in `detail`). This is the ISA-18.2 "rationalise" lifecycle step.

`enabled=0` is the shelf: background alarms the plant has decided to suppress.
null loever/upper bounds disable that boundary; the alarm engine treats a
null limit as "not armed" rather than 0.

Rationalisation requires the tag to already be registered
(`GET /api/v1/alarms/limits/{tag_id}` 404 if not). A rationalisation for a
retired tag is removed via `DELETE`; retirement of the tag itself does not
cascade (a retired tag still has its audit trail).

The simulator/ingest alarm engine is not yet wired to read from
`alarm_limits` — that wiring lands together with the simulator refactor that
moves alarms out of the per-tick path. Storing the configuration now means the
rationalisation workflow can be exercised and approved before the engine
consumes it.
