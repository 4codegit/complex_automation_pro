package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"cap/internal/control"
	"cap/internal/store"
)

// putSetpoint sends the operator's setpoint with an audit subject.
func putSetpoint(t *testing.T, env *testEnv, body string) (*http.Response, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, env.server.URL+"/api/v1/control/setpoints", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put setpoint: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

// TestControlSetpointClampsToProfileCorridor: the operator's value is stored
// clamped to the active profile corridor and the change is audited.
func TestControlSetpointClampsToProfileCorridor(t *testing.T) {
	env := newTestEnv(t)

	resp, out := putSetpoint(t, env, `{"value": 55, "updated_by": "operator-7"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %v", resp.StatusCode, out)
	}
	if out["clamped"] != true {
		t.Errorf("value 55 must be clamped to the profile corridor: %v", out)
	}

	sp, err := store.GetSetpoint(t.Context(), env.db, "plant-a.flotation.ph_level")
	if err != nil {
		t.Fatalf("stored setpoint: %v", err)
	}
	if sp.Value > 11 || sp.Value < 7 {
		t.Errorf("stored value %v outside corridor 7..11", sp.Value)
	}
	if sp.UpdatedBy != "operator-7" {
		t.Errorf("updated_by = %q", sp.UpdatedBy)
	}
}

// TestControlStatusReportsDisabledByDefault: without CONTROL_ENABLED the
// endpoints work but clearly report the loop as off.
func TestControlStatusReportsDisabledByDefault(t *testing.T) {
	env := newTestEnv(t)
	resp, body := getJSONMap(t, env, "/api/v1/control/status")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if body["enabled"] != false {
		t.Errorf("enabled = %v, want false (CONTROL_ENABLED unset)", body["enabled"])
	}
}

// TestControlOutputDefaultsToSafeZero: a gateway polling an unknown loop gets
// output 0 / status off — never a fabricated actuator value.
func TestControlOutputDefaultsToSafeZero(t *testing.T) {
	env := newTestEnv(t)
	resp, body := getJSONMap(t, env, "/api/v1/control/output?tag_id=plant-a.flotation.doser_speed")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if body["output"] != 0.0 || body["status"] != control.StatusOff {
		t.Errorf("output = %v status = %v, want 0/off", body["output"], body["status"])
	}
}

// TestControlSetpointRequiresPermission: a subject without control_process
// gets 403 (RBAC middleware), even though the loop itself is disabled.
func TestControlSetpointRequiresPermission(t *testing.T) {
	env := newTestEnv(t)
	req, _ := http.NewRequest(http.MethodPut, env.server.URL+"/api/v1/control/setpoints",
		strings.NewReader(`{"value": 9, "updated_by": "intruder"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User", "no-such-subject")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for subject without control_process", resp.StatusCode)
	}
}
