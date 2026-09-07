# CAP: product plan and technical specification

## 1. Purpose and success boundary

Build a vendor-neutral, read-only industrial telemetry and monitoring platform for a mineral-processing plant. The first release shall make process state, data quality, equipment health and alarms visible from ore receiving through concentrate dispatch.

It is not a replacement for PLC, DCS, SIS/ESD, motor protection or a certified SCADA system. It does not close control loops or bypass operator procedures.

## 2. Process map and initial telemetry catalogue

The actual flowsheet, ore mineralogy, equipment passports, P&IDs, control narratives and laboratory procedure are the source of truth. The list below is a configurable starting catalogue, not a universal setpoint list.

| Area | Equipment and process purpose | Initial signals to support |
| --- | --- | --- |
| Ore receiving and stockpile | feed continuity and material accounting | belt scale mass flow, belt speed, feeder current, bin/stockpile level, magnet/metal detector status, chute blockage, vibration, bearing temperature |
| Crushing and screening | size reduction and protection | crusher motor current/power, hydraulic pressure, lube oil pressure/temperature, vibration, bearing temperature, crusher gap where available, feed and product belt scales, screen vibration/current, screen availability |
| Grinding and classification | target liberation size and circulating load | mill power/current, mill speed, bearing/lube temperatures and pressures, feed water, cyclone feed pressure, cyclone density, slurry flow, sump level, particle-size analyser, pump current/vibration/seal condition |
| Flotation and reagent preparation | recovery and concentrate grade control | pulp pH, ORP where applicable, density, flow, froth depth/velocity/camera-derived features where validated, air flow/pressure, reagent tank level, reagent flow, agitator current, cell level, pump condition |
| Thickening | recover water and stabilise slurry | feed flow/density, bed level or interface level, overflow turbidity, underflow density/flow, rake torque/lift, flocculant flow, tank level, pump condition |
| Filtration | produce a stable filter cake | feed flow/density/pressure, vacuum or membrane pressure, filtrate turbidity/flow, cycle phase, cake thickness/moisture if measured, cloth wash pressure, drive current, vibration, bearing temperature |
| Drying and concentrate handling | meet moisture and safe handling requirements | feed rate, product moisture, dryer inlet/outlet temperature and pressure, gas/O2 analysis where applicable, burner/fuel state, fan current/vibration, dust collector differential pressure, bin level, belt scale, metal detector |
| Utilities and environment | availability, safety and compliance | process-water flow/pressure, compressed-air pressure/dew point, power quality, substation/motor states, ambient/dust/effluent instruments as required by site permits |

Every tag must retain unit, engineering range, sampling interval, source timestamp, quality code, calibration due date, asset ID, location, criticality and owner.

## 3. Configurable ore and flowsheet model

Do not encode one ore type into application code. A `process profile` shall version the properties below and reference laboratory testwork/plant approval:

- ore domain, mineralogy and deleterious components;
- feed grade, hardness/competency, moisture, clay content and size distribution;
- product specification, recovery and throughput targets;
- enabled unit operations and equipment topology;
- approved operating envelope, alarm limits and control narrative references;
- laboratory assay and reconciliation inputs.

Changing a profile or limit requires role-based approval, effective time, reason, author, reviewer and a full audit record.

## 4. Device connectivity

Support adapters, not direct device-specific code in the API. Priority adapters: OPC UA, Modbus TCP, MQTT Sparkplug B, and a file/API adapter for laboratory systems. Each adapter writes the same canonical message.

Gateways must push telemetry when connected and persist locally during a link loss. The central platform must also support pull for controlled backfill and reports. Pull is never used as a mechanism to command field equipment.

## 5. Canonical telemetry contract

All transports use the same versioned JSON payload. `value` can be numeric, boolean, string or structured; the tag registry defines its data type and unit.

```json
{
  "schema_version": "1.0",
  "message_id": "018f0e6b-7f1a-7e1d-a0bd-5b31d918c001",
  "gateway_id": "gw-grinding-01",
  "source_sequence": 482119,
  "observed_at": "2026-07-31T08:15:30.125Z",
  "sent_at": "2026-07-31T08:15:31.008Z",
  "asset_id": "plant-a.grinding.mill-01",
  "tag_id": "plant-a.grinding.mill-01.motor_power",
  "value": 1245.7,
  "unit": "kW",
  "quality": "good",
  "profile_id": "ore-profile-2026-07-a"
}
```

Allowed `quality` values: `good`, `uncertain`, `bad`, `stale`, `substituted`, `offline`. The server must not silently turn bad or stale data into a normal value.

## 6. API contract, v1

All endpoints are under `/api/v1`. Gateway endpoints require a machine identity; user endpoints require role-based access. APIs return RFC 7807-style error objects with a stable `code`, `message`, `trace_id` and field errors when relevant.

| Method and path | Purpose | Required behaviour |
| --- | --- | --- |
| `POST /ingest/telemetry` | Push one or many readings from a gateway | Validate schema/tag/unit/quality, deduplicate by gateway plus message ID, accept retry safely, return accepted/rejected items and server receipt time |
| `POST /ingest/telemetry:batch` | Push buffered readings | Transactional per item result, bounded batch size, preserves source order per tag |
| `GET /telemetry` | Pull historical readings | Filters: tag, asset, from, to, resolution, cursor; UTC only; paginated and rate-limited |
| `GET /telemetry/latest` | Pull latest value per tag/asset | Returns value, observed time, quality and staleness calculation |
| `GET /assets` and `GET /tags` | Discover equipment and registered signals | Includes metadata, units and access scope; no secret or PLC address exposure |
| `GET /alarms` | Query alarm/event history | Filters by state, priority, time, asset, tag and profile |
| `POST /alarms/{id}/acknowledgements` | Acknowledge an alarm | Requires authenticated human, comment and immutable audit event |
| `GET /health` | Health of application dependencies | Separate liveness/readiness; must include stale gateway count without leaking internals externally |

Response semantics: `202 Accepted` for asynchronously persisted ingestion, `200 OK` for a completed query, `400` for invalid payload, `401/403` for identity/permission, `409` only for non-idempotent version conflicts, `429` for rate limit and `503` for an unavailable dependency. A duplicate message returns a successful idempotent result, not a second measurement.

## 7. Alarms and events

Alarm design is a plant engineering task. Each alarm must document: purpose, consequence, operator action, priority, response time, limit/deadband/delay, suppression rules, owner and review date. "Value changed" is an event; it is not automatically an alarm.

Initial states: `normal`, `active_unacknowledged`, `active_acknowledged`, `returned_to_normal`, `shelved`, `suppressed_by_design`. Alarm acknowledgement never changes the physical process and never suppresses the source event history.

## 8. Delivery phases and gates

1. Discovery and safety boundary: plant walkdown, P&ID/tag register, network zones, owner matrix, process profile template, alarm philosophy. Gate: approved scope and read-only boundary.
2. Foundation: local developer setup, API contract, asset/tag registry, simulator, automated tests, observability and threat model. Gate: contract tests and failure-mode tests pass.
3. Edge pilot: one non-critical area, read-only adapter, buffering, quality codes, historian and dashboard. Gate: reconciled data and network-loss recovery are accepted by operations.
4. Monitoring pilot: add trends, alarms, laboratory correlation and role-based access. Gate: alarm rationalisation and operator acceptance.
5. Scale-out: remaining areas, redundancy, backup/restore exercise, performance and security testing. Gate: production readiness review.
6. Production release: only after factory/site acceptance tests, operational runbook, incident response, rollback plan and director-approved go/no-go.

## 9. Roles and decisions

| Role | Accountable output |
| --- | --- |
| Director / asset owner | scope, budget, production acceptance and risk ownership |
| Chief metallurgist | flowsheet, ore profiles, KPIs and laboratory reconciliation |
| Process engineer | operating envelopes, control narratives and alarm rationalisation input |
| Automation/OT engineer | P&IDs, PLC/DCS interface, instrument list, interlocks and commissioning |
| Maintenance/reliability | asset hierarchy, condition-monitoring signals and maintenance workflow |
| OT cybersecurity | zones/conduits, identities, remote access and security acceptance |
| Product/technical lead | architecture, API governance, delivery plan and engineering quality |
| Development team | tested implementation, documentation, traceability and support tooling |
| Operators | HMI validation, alarm usability, procedures and acceptance feedback |

## 10. First sprint backlog

1. Replace fixed telemetry constants with a database-backed tag registry and process-profile configuration. Status: initial registry implemented; administrative change workflow remains.
2. Add canonical ingestion schemas, message ID validation, quality codes and batch receipt response. Status: implemented for the development API.
3. Separate simulator endpoints from production ingestion; simulator must be disabled by default outside development.
4. Add automated contract tests for valid, duplicate, late, invalid and offline telemetry.
5. Add an equipment tree and data-quality/staleness indicator to the monitoring UI.
6. Produce the plant discovery questionnaire and obtain the first approved instrument/tag list before connecting a live device.
