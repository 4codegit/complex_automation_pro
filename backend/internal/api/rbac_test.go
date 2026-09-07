package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cap/internal/config"
	"cap/internal/hub"
	"cap/internal/simulator"
	"cap/internal/store"
)

// newRBACTestServer builds an isolated server with seeded roles for the
// authorization tests.
func newRBACTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/rbac.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := store.SeedRegistry(context.Background(), db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := store.SeedRoles(context.Background(), db); err != nil {
		t.Fatalf("seed roles: %v", err)
	}
	// Two test users: metallurgist-anna gets metallurgist; ot-pavel gets
	// ot_engineer. anonymous intentionally has no assignment.
	if err := store.AssignRole(context.Background(), db, "metallurgist-anna", "metallurgist", "tests"); err != nil {
		t.Fatalf("assign metallurgist: %v", err)
	}
	if err := store.AssignRole(context.Background(), db, "ot-pavel", "ot_engineer", "tests"); err != nil {
		t.Fatalf("assign ot_engineer: %v", err)
	}
	cfg := &config.Settings{DefaultLimit: 50, MaxLimit: 1000}
	s := New(db, hub.New(), &simulator.Manager{}, cfg)
	s.now = func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) }
	return s
}

func callWhoAmI(t *testing.T, s *Server, user, role string) *http.Response {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/access/whoami", nil)
	if user != "" {
		req.Header.Set("X-User", user)
	}
	if role != "" {
		req.Header.Set("X-Role", role)
	}
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec.Result()
}

func callCreateTag(t *testing.T, s *Server, user, role, tagID string) *http.Response {
	body, _ := json.Marshal(map[string]any{
		"id": tagID, "asset_id": "plant-a.crushing", "name": "Demo", "unit": "x", "data_type": "number",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.Header.Set("X-User", user)
	}
	if role != "" {
		req.Header.Set("X-Role", role)
	}
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec.Result()
}

// TestRBACDenyAnonymousMutation: anonymous subject (no assignment) is denied
// manage_tags even though the endpoint exists.
func TestRBACDenyAnonymousMutation(t *testing.T) {
	s := newRBACTestServer(t)
	resp := callCreateTag(t, s, "anonymous", "", "tag-anon-1")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("anonymous create tag = %d, want 403", resp.StatusCode)
	}
}

// TestRBACAllowMetallurgistAlarmAck: metallurgist-anna has review_alarms
// permission by role, but acknowledge_alarms is the operator permission. So
// she should be 403 on ack. Operator permission is not in metallurgist set.
func TestRBACMetallurgistCannotAck(t *testing.T) {
	s := newRBACTestTestServer(t)
	// Seed an alarm via store directly to skip the alarm engine path.
	if err := store.UpsertAlarmActive(context.Background(), s.db, "plant-a.crushing.particle_size",
		"particle_size", "critical", "demo critical", time.Now().UTC()); err != nil {
		t.Fatalf("upsert alarm: %v", err)
	}
	alarms, _ := store.ListActiveAlarms(context.Background(), s.db, 10)
	if len(alarms) != 1 {
		t.Fatalf("seeded alarms = %d", len(alarms))
	}
	id := alarms[0].ID
	body, _ := json.Marshal(map[string]string{"ack_by": "metallurgist-anna", "comment": "ok"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alarms/"+id+"/ack", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User", "metallurgist-anna")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("metallurgist ack = %d, want 403 (no acknowledge_alarms)", rec.Code)
	}
}

// TestRBACAllowPlatformAdminEverything: platform_admin implicitly holds every
// permission, so a rationalise call goes through even though the subject has
// no specific role assignment.
func TestRBACAllowPlatformAdminEverything(t *testing.T) {
	s := newRBACTestServer(t)
	if err := store.AssignRole(context.Background(), s.db, "boss", "platform_admin", "tests"); err != nil {
		t.Fatalf("assign: %v", err)
	}
	body, _ := json.Marshal(map[string]any{"hi": 30.0, "severity": "high"})
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/alarms/limits/plant-a.crushing.particle_size", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User", "boss")
	req.SetPathValue("tag_id", "plant-a.crushing.particle_size")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("platform_admin rationalise = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// TestRBACRoleCRUDAndSystemRoleProtection: platform_admin can create, update
// and delete roles; system roles refuse deletion with 409.
func TestRBACRoleCRUDAndSystemRoleProtection(t *testing.T) {
	s := newRBACTestServer(t)
	if err := store.AssignRole(context.Background(), s.db, "boss", "platform_admin", "tests"); err != nil {
		t.Fatalf("assign: %v", err)
	}

	// Create custom role.
	body, _ := json.Marshal(map[string]any{"id": "qa", "label": "QA", "permissions": []string{"view_all"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/access/roles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User", "boss")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create role = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Delete system role -> 409.
	req2 := httptest.NewRequest(http.MethodDelete, "/api/v1/access/roles/director", nil)
	req2.SetPathValue("id", "director")
	req2.Header.Set("X-User", "boss")
	rec2 := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("delete system role = %d, want 409", rec2.Code)
	}

	// Delete custom role -> 204.
	req3 := httptest.NewRequest(http.MethodDelete, "/api/v1/access/roles/qa", nil)
	req3.SetPathValue("id", "qa")
	req3.Header.Set("X-User", "boss")
	rec3 := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusNoContent {
		t.Fatalf("delete custom role = %d, want 204", rec3.Code)
	}
}

// TestRBACOTEngineerCanCreateTag: ot-pavel has manage_tags, so POST /tags
// goes through.
func TestRBACOTEngineerCanCreateTag(t *testing.T) {
	s := newRBACTestServer(t)
	resp := callCreateTag(t, s, "ot-pavel", "ot_engineer", "tag-ot-1")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("ot_engineer create tag = %d, want 201", resp.StatusCode)
	}
}

// TestRBACMetallurgistCannotCreateTag: metallurgist-anna lacks manage_tags.
func TestRBACMetallurgistCannotCreateTag(t *testing.T) {
	s := newRBACTestServer(t)
	resp := callCreateTag(t, s, "metallurgist-anna", "metallurgist", "tag-met-1")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("metallurgist create tag = %d, want 403", resp.StatusCode)
	}
}

// helper used by the ack test: same server shape.
func newRBACTestTestServer(t *testing.T) *Server { return newRBACTestServer(t) }
