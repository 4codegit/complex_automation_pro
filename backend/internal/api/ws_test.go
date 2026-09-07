package api_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestWebSocketStreamsLiveTelemetry guards the Hijack path: the logging
// middleware must let WebSocket upgrades through, or the dashboard goes dark.
func TestWebSocketStreamsLiveTelemetry(t *testing.T) {
	env := newTestEnv(t)
	wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + "/api/v1/ws"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Simulator broadcasts emergency events; ingest pushes telemetry events.
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"trigger_emergency"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp, _ := postJSON(t, env, "/api/v1/ingest/telemetry", telemetryPayload(payloadOpts{messageID: messageIDOne}))
	if resp.StatusCode != 202 {
		t.Fatalf("ingest status = %d", resp.StatusCode)
	}

	var sawTelemetry, sawEmergency bool
	for !sawTelemetry || !sawEmergency {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read ws: %v (telemetry=%v emergency=%v)", err, sawTelemetry, sawEmergency)
		}
		var ev map[string]any
		if err := json.Unmarshal(data, &ev); err != nil {
			continue
		}
		switch ev["type"] {
		case "telemetry":
			sawTelemetry = true
			if ev["stage"] == "" {
				t.Fatalf("telemetry event missing stage: %v", ev)
			}
			if ev["metric"] != "particle_size" {
				t.Fatalf("unexpected metric %v", ev["metric"])
			}
		case "emergency_start":
			sawEmergency = true
		}
	}
}
