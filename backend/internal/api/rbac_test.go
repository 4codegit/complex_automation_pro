package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"cap/internal/store"
)

// callWithCookie performs a request against the full route tree with the
// session cookie of the given subject's pre-created session.
func callWithCookie(t *testing.T, env *testEnv, subject, method, path string, body any) *http.Response {
	t.Helper()
	token, err := sessionToken(env, subject)
	if err != nil {
		t.Fatalf("session for %s: %v", subject, err)
	}
	var rd *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, env.server.URL+path, rd)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "cap_session", Value: token})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	resp.Body.Close()
	return resp
}

func TestRBACDenyUnassignedSubject(t *testing.T) {
	env := newTestEnvNoLogin(t)
	seedUserPassword(t, env, "ghost", "pw-ghost")
	env.login(t, "ghost", "pw-ghost")
	resp, _ := postJSON(t, env, "/api/v1/tags", map[string]any{
		"id": "plant.crushing.extra", "asset_id": "plant.crushing",
		"name": "Demo", "unit": "x", "data_type": "number",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unassigned create tag = %d, want 403", resp.StatusCode)
	}
}

func TestRBACMetallurgistCannotAck(t *testing.T) {
	env := newTestEnvNoLogin(t)
	seedUserWithRole(t, env, "anna", "metallurgist")
	env.login(t, "anna", "pw-anna")

	// Seed an alarm via store directly to skip the alarm engine path.
	if err := store.UpsertAlarmActive(context.Background(), env.db, feedTagID,
		"fi101", "critical", "demo critical", time.Now().UTC()); err != nil {
		t.Fatalf("upsert alarm: %v", err)
	}
	_, alarmList := getJSON(t, env, "/api/v1/alarms/active")
	if len(alarmList) != 1 {
		t.Fatalf("seeded alarms = %d", len(alarmList))
	}
	id := alarmList[0]["id"].(string)
	resp, _ := postJSON(t, env, "/api/v1/alarms/"+id+"/ack", map[string]string{"comment": "ok"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("metallurgist ack = %d, want 403 (no acknowledge_alarms)", resp.StatusCode)
	}
}

func TestRBACRoleCRUDAndSystemRoleProtection(t *testing.T) {
	env := newTestEnv(t) // admin client

	// Create custom role.
	resp, _ := postJSON(t, env, "/api/v1/access/roles",
		map[string]any{"id": "qa", "label": "QA", "permissions": []string{"view_all"}})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create role = %d", resp.StatusCode)
	}

	// Delete system role -> 409.
	resp2, _ := env.do(t, "DELETE", "/api/v1/access/roles/director", nil)
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("delete system role = %d, want 409", resp2.StatusCode)
	}

	// Delete custom role -> 204.
	resp3, _ := env.do(t, "DELETE", "/api/v1/access/roles/qa", nil)
	if resp3.StatusCode != http.StatusNoContent {
		t.Fatalf("delete custom role = %d, want 204", resp3.StatusCode)
	}
}

func TestRBACOTEngineerCanCreateTag(t *testing.T) {
	env := newTestEnvNoLogin(t)
	seedUserWithRole(t, env, "pavel", "ot_engineer")
	env.login(t, "pavel", "pw-pavel")
	resp, _ := postJSON(t, env, "/api/v1/tags", map[string]any{
		"id": "plant.crushing.extra", "asset_id": "plant.crushing",
		"name": "Demo", "unit": "x", "data_type": "number",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("ot_engineer create tag = %d, want 201", resp.StatusCode)
	}
}
