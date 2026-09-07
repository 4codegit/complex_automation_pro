package api_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
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

// TestSimulatorTelemetryReachesWebSocket starts the simulator inside the test
// process and verifies its tick broadcasts arrive at a WS client. This covers
// the path that manual ingests do not (the sim's own hub fan-out).
func TestSimulatorTelemetryReachesWebSocket(t *testing.T) {
	ctx := context.Background()

	db, err := store.Open(ctx, "sqlite://"+t.TempDir()+"/sim.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := store.SeedRegistry(ctx, db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := store.EnsureDefaultProfile(ctx, db); err != nil {
		t.Fatalf("profile seed: %v", err)
	}

	h := hub.New()
	sim := simulator.New(db, h, 10*time.Millisecond, 1*time.Second)
	sim.Start(ctx)
	defer sim.Stop()

	srv := api.New(db, h, sim, &config.Settings{Environment: "test", MaxBatchSize: 500, DefaultLimit: 100, MaxLimit: 1000})
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	var telemetryCount int
	for telemetryCount < 3 {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read ws: %v (received %d telemetry events)", err, telemetryCount)
		}
		var ev map[string]any
		if err := json.Unmarshal(data, &ev); err != nil {
			continue
		}
		if ev["type"] == "telemetry" {
			telemetryCount++
			if ev["stage"] == "" || ev["metric"] == "" {
				t.Fatalf("sim event missing stage/metric: %v", ev)
			}
		}
	}
}
