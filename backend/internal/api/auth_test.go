package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"cap/internal/auth"
	"cap/internal/store"
)

// TestLoginRequiresValidCredentials guards the session boundary: wrong
// password is a uniform 401 (no account enumeration), and unauthenticated
// API calls are rejected before any handler runs.
func TestLoginRequiresValidCredentials(t *testing.T) {
	env := newTestEnvNoLogin(t)

	// Unknown user and wrong password produce the identical 401 body.
	resp1, body1 := env.do(t, "POST", "/api/v1/access/login",
		map[string]string{"username": "ghost", "password": "x"})
	resp2, body2 := env.do(t, "POST", "/api/v1/access/login",
		map[string]string{"username": "admin", "password": "wrong"})
	if resp1.StatusCode != http.StatusUnauthorized || resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login statuses = %d/%d, want 401/401", resp1.StatusCode, resp2.StatusCode)
	}
	if body1 != body2 {
		t.Fatalf("credential errors differ: %q vs %q", body1, body2)
	}

	// Health and login are public; everything else is 401 without a session.
	resp3, _ := env.do(t, "GET", "/api/v1/health", nil)
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("health = %d, want 200 (public)", resp3.StatusCode)
	}
	resp4, _ := env.do(t, "GET", "/api/v1/assets", nil)
	if resp4.StatusCode != http.StatusUnauthorized {
		t.Fatalf("assets without session = %d, want 401", resp4.StatusCode)
	}
}

// TestWhoAmIResolvesSessionPermissions verifies the whoami payload used by
// the UI to render available actions.
func TestWhoAmIResolvesSessionPermissions(t *testing.T) {
	env := newTestEnv(t)

	resp, raw := env.do(t, "GET", "/api/v1/access/whoami", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("whoami = %d", resp.StatusCode)
	}
	out := decodeMap(t, raw)
	if out["subject"] != "admin" {
		t.Fatalf("subject = %v", out["subject"])
	}
	perms, ok := out["permissions"].([]any)
	if !ok || len(perms) == 0 {
		t.Fatalf("permissions missing: %v", out)
	}
	roles := out["roles"].([]any)
	if len(roles) != 1 || roles[0] != "platform_admin" {
		t.Fatalf("roles = %v", roles)
	}
}

// TestRBACOperatorBoundaries: the operator session may acknowledge alarms and
// drive control, but registry management stays forbidden (TZ §12 matrix).
func TestRBACOperatorBoundaries(t *testing.T) {
	env := newTestEnvNoLogin(t)
	ctx := context.Background()

	if err := store.CreateUser(ctx, env.db, mustUser(t, "operator", "operator")); err != nil {
		t.Fatalf("create operator: %v", err)
	}
	if err := store.AssignRole(ctx, env.db, "operator", "operator", "tests"); err != nil {
		t.Fatalf("assign operator: %v", err)
	}
	env.login(t, "operator", "operator")

	// Denied: registry management.
	resp, _ := postJSON(t, env, "/api/v1/tags", map[string]any{
		"id": "plant.crushing.extra", "asset_id": "plant.crushing",
		"name": "Extra", "unit": "x", "data_type": "number",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("operator create tag = %d, want 403", resp.StatusCode)
	}

	// Denied: profile approval (technologist's job).
	resp2, _ := postJSON(t, env, "/api/v1/profiles", map[string]any{
		"name": "x", "ore_domain": "x", "author": "op", "params": "{}",
	})
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("operator create profile = %d, want 403", resp2.StatusCode)
	}

	// Allowed: alarm acknowledgement, actor from the session.
	if err := store.UpsertAlarmActive(ctx, env.db, feedTagID, "fi101",
		"critical", "demo critical", time.Now().UTC()); err != nil {
		t.Fatalf("upsert alarm: %v", err)
	}
	_, active := getJSON(t, env, "/api/v1/alarms/active")
	if len(active) != 1 {
		t.Fatalf("active alarms = %d", len(active))
	}
	id := active[0]["id"].(string)
	ackResp, acked := postJSON(t, env, "/api/v1/alarms/"+id+"/ack", map[string]any{"comment": "ok"})
	if ackResp.StatusCode != http.StatusOK {
		t.Fatalf("operator ack = %d, want 200; body=%v", ackResp.StatusCode, acked)
	}
	if acked["state"] != store.AlarmStateActiveAck || acked["ack_by"] != "operator" {
		t.Fatalf("acked = %v", acked)
	}

	// The journal carries the ack event with the session actor.
	_, journal := getJSON(t, env, "/api/v1/alarms/journal?limit=50")
	var sawAck bool
	for _, ev := range journal {
		if ev["event"] == "acked" && ev["actor"] == "operator" {
			sawAck = true
		}
	}
	if !sawAck {
		t.Fatalf("journal lacks acked event: %v", journal)
	}
}

// TestRBACPlatformAdminEverything: platform_admin implicitly holds every
// permission, including rationalise_alarms.
func TestRBACPlatformAdminEverything(t *testing.T) {
	env := newTestEnvNoLogin(t)
	env.login(t, "admin", "admin")

	resp, _ := putJSON(t, env, "/api/v1/alarms/limits/"+pHTagID,
		map[string]any{"hi": 11.0, "severity": "high"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin rationalise = %d", resp.StatusCode)
	}
}

func mustUser(t *testing.T, username, password string) *store.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return &store.User{Username: username, PasswordHash: hash, Active: true}
}
