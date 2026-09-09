package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"cap/internal/api"
	"cap/internal/config"
	"cap/internal/hub"
	"cap/internal/simulator"
	"cap/internal/store"
)

// newSectionEnv builds the seeded test database plus a server exposing only
// the given route sections — the shape split-mode services run with.
func newSectionEnv(t *testing.T, sections []string) (*httptest.Server, *sql.DB) {
	t.Helper()
	ctx := context.Background()

	db, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := store.SeedRegistry(ctx, db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := store.SeedRoles(ctx, db); err != nil {
		t.Fatalf("seed roles: %v", err)
	}
	if err := store.AssignRole(ctx, db, "anonymous", "platform_admin", "tests"); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}
	if err := store.EnsureDefaultProfile(ctx, db); err != nil {
		t.Fatalf("seed profiles: %v", err)
	}

	h := hub.New()
	sim := simulator.New(db, h, 10*time.Millisecond, time.Second)
	cfg := &config.Settings{MaxBatchSize: 500, DefaultLimit: 100, MaxLimit: 1000}
	srv := api.New(db, h, sim, cfg)
	ts := httptest.NewServer(srv.Routes(sections...))
	t.Cleanup(ts.Close)

	return ts, db
}

// TestSectionRoutingIsolatesDomains mounts each section alone and verifies
// that only its own endpoints answer while foreign domains yield 404 —
// the property split-mode services rely on.
func TestSectionRoutingIsolatesDomains(t *testing.T) {
	cases := []struct {
		section string
		allowed []string
		foreign []string
	}{
		{
			section: "ingest",
			allowed: []string{"POST /api/v1/ingest/gateway_events"},
			foreign: []string{"/api/v1/telemetry", "/api/v1/profiles", "/api/v1/assets", "/api/v1/access/roles"},
		},
		{
			section: "historian",
			allowed: []string{"/api/v1/telemetry", "/api/v1/analytics/process", "/api/v1/reports/readings/csv"},
			foreign: []string{"/api/v1/profiles", "/api/v1/tags"},
		},
		{
			section: "alarms",
			allowed: []string{"/api/v1/alarms/active", "/api/v1/alarms/limits"},
			foreign: []string{"/api/v1/telemetry", "/api/v1/tags"},
		},
		{
			section: "profiles",
			allowed: []string{"/api/v1/profiles"},
			foreign: []string{"/api/v1/telemetry", "/api/v1/assets"},
		},
		{
			section: "registry",
			allowed: []string{"/api/v1/assets", "/api/v1/tags", "/api/v1/gateways"},
			foreign: []string{"/api/v1/telemetry", "/api/v1/profiles"},
		},
		{
			section: "identity",
			allowed: []string{"/api/v1/access/roles"},
			foreign: []string{"/api/v1/telemetry", "/api/v1/assets"},
		},
		{
			section: "live",
			allowed: []string{"/api/v1/simulator/status"},
			foreign: []string{"/api/v1/telemetry", "/api/v1/ingest/telemetry"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.section, func(t *testing.T) {
			ts, _ := newSectionEnv(t, []string{tc.section})

			for _, entry := range tc.allowed {
				method, path := "GET", entry
				if i := strings.Index(entry, " "); i > 0 && !strings.HasPrefix(entry, "/") {
					method, path = entry[:i], entry[i+1:]
				}
				req, err := http.NewRequest(method, ts.URL+path, nil)
				if err != nil {
					t.Fatalf("%s %s: %v", method, path, err)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("%s %s: %v", method, path, err)
				}
				resp.Body.Close()
				if resp.StatusCode == http.StatusNotFound || resp.StatusCode >= 500 {
					t.Errorf("%s %s -> %d, want mounted (not 404/5xx)", method, path, resp.StatusCode)
				}
			}
			for _, path := range tc.foreign {
				resp, err := http.Get(ts.URL + path)
				if err != nil {
					t.Fatalf("get %s: %v", path, err)
				}
				resp.Body.Close()
				if resp.StatusCode != http.StatusNotFound {
					t.Errorf("GET %s -> %d, want 404 (not mounted in section %q)", path, resp.StatusCode, tc.section)
				}
			}
		})
	}
}

// TestInternalEventReachesWebSocketSubscribers verifies the split-mode live
// feed: an event POSTed to /internal/events (as the ingest service does via
// EVENT_SINKS) is broadcast to a connected WebSocket client.
func TestInternalEventReachesWebSocketSubscribers(t *testing.T) {
	ts, _ := newSectionEnv(t, []string{"live"})

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	event := map[string]any{
		"type": "telemetry", "timestamp": time.Now().UTC().Format(time.RFC3339),
		"asset_id": assetID, "tag_id": particleTagID, "stage": "Дробление",
		"metric": "particle_size", "value": 5.4, "unit": "mm",
		"quality": "good", "alert": false, "emergency": false,
	}
	raw, _ := json.Marshal(event)
	resp, err := http.Post(ts.URL+"/internal/events", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("post internal event: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("internal event -> %d, want 202", resp.StatusCode)
	}

	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ws message: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(msg, &got); err != nil {
		t.Fatalf("unmarshal ws payload: %v", err)
	}
	if got["type"] != "telemetry" || got["tag_id"] != particleTagID {
		t.Errorf("ws payload mismatch: %s", msg)
	}
}

// TestInternalEventRejectsEmptyBody guards the intake against silent no-ops.
func TestInternalEventRejectsEmptyBody(t *testing.T) {
	ts, _ := newSectionEnv(t, []string{"live"})

	resp, err := http.Post(ts.URL+"/internal/events", "application/json", strings.NewReader("   "))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty event -> %d, want 400", resp.StatusCode)
	}
}
