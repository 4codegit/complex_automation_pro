package simulator

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"autopro/internal/hub"
	"autopro/internal/store"
)

// TestTickBroadcastsDirectly checks the sim's hub fan-out with a direct hub
// client, isolating the WS transport from the generator.
func TestTickBroadcastsDirectly(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedRegistry(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureDefaultProfile(ctx, db); err != nil {
		t.Fatal(err)
	}

	h := hub.New()
	m := New(db, h, 10*time.Millisecond, time.Second)
	m.Start(ctx)

	client := h.Register()
	defer client.Close()

	deadline := time.After(3 * time.Second)
	telemetry := 0
	for telemetry < 2 {
		select {
		case <-deadline:
			t.Fatalf("received %d telemetry events, want >= 2", telemetry)
		case msg := <-client.Messages():
			var ev map[string]any
			if err := json.Unmarshal(msg, &ev); err != nil {
				continue
			}
			if ev["type"] == "telemetry" {
				telemetry++
				t.Logf("telemetry event: metric=%v stage=%v", ev["metric"], ev["stage"])
			}
		}
	}
}
