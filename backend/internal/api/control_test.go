package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestControlLoopSetpointAndMode exercises the supervisory loop API (TZ §13):
// setpoint clamping to the loop window, mode switching and the confirmation
// requirement on every write.
func TestControlLoopSetpointAndMode(t *testing.T) {
	env := newTestEnv(t)

	_, loops := getJSON(t, env, "/api/v1/control/loops")
	if len(loops) != 3 {
		t.Fatalf("seeded loops = %d, want 3", len(loops))
	}

	// Writes require confirm:true.
	resp, _ := putJSON(t, env, "/api/v1/control/loops/lic301/setpoint",
		map[string]any{"value": 550.0})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("setpoint without confirm = %d, want 422", resp.StatusCode)
	}

	// Out of the loop window -> 422.
	resp2, _ := putJSON(t, env, "/api/v1/control/loops/lic301/setpoint",
		map[string]any{"value": 900.0, "confirm": true})
	if resp2.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("setpoint out of range = %d, want 422", resp2.StatusCode)
	}

	// Valid write.
	resp3, sp := putJSON(t, env, "/api/v1/control/loops/lic301/setpoint",
		map[string]any{"value": 550.0, "confirm": true})
	if resp3.StatusCode != http.StatusOK || sp["sp"] != 550.0 {
		t.Fatalf("setpoint write = %d sp=%v", resp3.StatusCode, sp["sp"])
	}

	// Manual output write works; auto output write is rejected.
	resp4, _ := putJSON(t, env, "/api/v1/control/loops/lic301/output",
		map[string]any{"value": 42.0, "confirm": true})
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("manual output write = %d", resp4.StatusCode)
	}
	resp5, _ := putJSON(t, env, "/api/v1/control/loops/lic301/mode",
		map[string]any{"mode": "auto", "confirm": true})
	if resp5.StatusCode != http.StatusOK {
		t.Fatalf("mode auto = %d", resp5.StatusCode)
	}
	resp6, _ := putJSON(t, env, "/api/v1/control/loops/lic301/output",
		map[string]any{"value": 10.0, "confirm": true})
	if resp6.StatusCode != http.StatusConflict {
		t.Fatalf("auto output write = %d, want 409", resp6.StatusCode)
	}
}

// TestControlOutputBridgeFeedShape verifies the edge-bridge contract: loop MVs
// appear with their status and manual actuator writes carry a bumping seq.
func TestControlOutputBridgeFeedShape(t *testing.T) {
	env := newTestEnv(t)

	// Manual one-shot write to a non-loop actuator.
	resp, wr := putJSON(t, env, "/api/v1/actuators/plant.grinding.fc201",
		map[string]any{"value": 48.0, "confirm": true})
	if resp.StatusCode != http.StatusOK || wr["seq"] != float64(1) {
		t.Fatalf("actuator write = %d %v", resp.StatusCode, wr)
	}

	// Loop-driven actuators reject manual writes.
	resp2, _ := putJSON(t, env, "/api/v1/actuators/plant.flotation.lc301",
		map[string]any{"value": 50.0, "confirm": true})
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("loop-driven actuator write = %d, want 409", resp2.StatusCode)
	}

	// Out of engineering range.
	resp3, _ := putJSON(t, env, "/api/v1/actuators/plant.grinding.fc201",
		map[string]any{"value": 480.0, "confirm": true})
	if resp3.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("out-of-range actuator write = %d, want 422", resp3.StatusCode)
	}

	// The feed is reachable without a session from loopback (the bridge).
	client := &http.Client{Timeout: 2 * time.Second}
	httpReq, _ := http.NewRequestWithContext(context.Background(), "GET", env.server.URL+"/api/v1/control/output", nil)
	httpResp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("bridge feed: %v", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		t.Fatalf("bridge feed = %d, want 200 (loopback)", httpResp.StatusCode)
	}
	var items []map[string]any
	_ = json.NewDecoder(httpResp.Body).Decode(&items)
	if len(items) < 4 { // 3 loop MVs + fc201
		t.Fatalf("bridge feed items = %d", len(items))
	}
	byTag := map[string]map[string]any{}
	for _, it := range items {
		byTag[it["tag_id"].(string)] = it
	}
	if byTag["plant.grinding.fc201"]["status"] != "hold" || byTag["plant.grinding.fc201"]["seq"] != float64(1) {
		t.Fatalf("fc201 item = %v", byTag["plant.grinding.fc201"])
	}
	if byTag["plant.flotation.lc301"]["status"] != "manual" {
		t.Fatalf("lc301 item = %v", byTag["plant.flotation.lc301"])
	}
}
