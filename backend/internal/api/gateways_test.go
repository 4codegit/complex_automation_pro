package api_test

import (
	"net/http"
	"testing"
)

func TestGatewayEventsRegisterAndRefreshGateway(t *testing.T) {
	env := newTestEnv(t)

	// First beat: gateway comes online with an empty buffer.
	resp, out := postJSON(t, env, "/api/v1/ingest/gateway_events", map[string]any{
		"gateway_id":  "gw-crushing-01",
		"event":       "online",
		"status":      "online",
		"buffer_size": 0,
		"latency_ms":  12,
		"version":     "edge-0.1.0",
		"timestamp":   "2026-08-01T12:00:00.000Z",
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	if out["status"] != "accepted" {
		t.Fatalf("out = %v", out)
	}

	// Second beat: buffered 120 messages, still online.
	resp2, _ := postJSON(t, env, "/api/v1/ingest/gateway_events", map[string]any{
		"gateway_id":  "gw-crushing-01",
		"event":       "pulse",
		"status":      "online",
		"buffer_size": 120,
		"latency_ms":  8,
		"timestamp":   "2026-08-01T12:00:05.000Z",
	})
	if resp2.StatusCode != http.StatusAccepted {
		t.Fatalf("status2 = %d", resp2.StatusCode)
	}

	gresp, gateways := getJSON(t, env, "/api/v1/gateways")
	if gresp.StatusCode != http.StatusOK || len(gateways) != 1 {
		t.Fatalf("gateways = %d (status %d)", len(gateways), gresp.StatusCode)
	}
	gw := gateways[0]
	if gw["id"] != "gw-crushing-01" || gw["buffer_size"] != float64(120) || gw["status"] != "online" {
		t.Fatalf("gateway row = %v", gw)
	}
	if gw["version"] != "edge-0.1.0" {
		t.Fatalf("version = %v", gw["version"])
	}
}

func TestGatewayEventValidation(t *testing.T) {
	env := newTestEnv(t)
	resp, _ := postJSON(t, env, "/api/v1/ingest/gateway_events", map[string]any{
		"gateway_id": "gw-x",
		"event":      "explode",
		"status":     "online",
		"timestamp":  "2026-08-01T12:00:00.000Z",
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
}

func TestIngestKeepsWorkingAcrossGatewayEvents(t *testing.T) {
	env := newTestEnv(t)
	// Telemetry ingestion must be unaffected by gateway heartbeat traffic.
	_, _ = postJSON(t, env, "/api/v1/ingest/gateway_events", map[string]any{
		"gateway_id":  "gw-crushing-01",
		"event":       "pulse",
		"status":      "online",
		"buffer_size": 7,
		"timestamp":   "2026-08-01T12:00:00.000Z",
	})
	resp, _ := postJSON(t, env, "/api/v1/ingest/telemetry", telemetryPayload(payloadOpts{messageID: messageIDOne}))
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("ingest status = %d", resp.StatusCode)
	}
}
