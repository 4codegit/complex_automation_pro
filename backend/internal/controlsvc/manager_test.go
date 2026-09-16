package controlsvc

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"cap/internal/hub"
	"cap/internal/store"
)

// TestTempInterlockForcesOilPour drives the tic201 loop through the
// high-temperature interlock: telemetry ≥ 75 °C must force auto mode with the
// output at the maximum (oil pour) even from manual, and the release needs a
// 5 °C hysteresis.
func TestTempInterlockForcesOilPour(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := store.SeedRegistry(ctx, db); err != nil {
		t.Fatalf("seed registry: %v", err)
	}
	if err := store.SeedControlLoops(ctx, db); err != nil {
		t.Fatalf("seed loops: %v", err)
	}
	if err := store.SeedAlarmLimits(ctx, db); err != nil {
		t.Fatalf("seed limits: %v", err)
	}

	m := New(db, hub.New(), nil, 10*time.Second)
	base := time.Now()

	putPV := func(at time.Time, celsius float64) {
		t.Helper()
		seq := at.Unix()
		r := &store.Reading{
			ID: store.NewID(), MessageID: store.NewID(), GatewayID: "test",
			SourceSequence: &seq, ObservedAt: at, ReceivedAt: at,
			AssetID: "plant.grinding", TagID: "plant.grinding.ti201",
			ValueNumber: &celsius, Unit: "C", Quality: "good",
		}
		if err := store.InsertReading(ctx, db, r); err != nil {
			t.Fatalf("insert pv %.1f: %v", celsius, err)
		}
	}

	loop, err := store.GetLoop(ctx, db, "tic201")
	if err != nil {
		t.Fatalf("get loop: %v", err)
	}
	if loop.TempInterlock != 75 {
		t.Fatalf("temp_interlock = %.0f, want 75", loop.TempInterlock)
	}
	// Operator left the loop in manual with the valve at 20 %.
	if err := store.SaveLoopRuntime(ctx, db, "tic201", 20.0, store.LoopModeManual, store.LoopStateOK, 0, 0); err != nil {
		t.Fatalf("seed runtime: %v", err)
	}

	putPV(base, 76.0)
	m.Tick(ctx)

	got, _ := store.GetLoop(ctx, db, "tic201")
	if got.Mode != store.LoopModeAuto || got.State != store.LoopStateInterlock {
		t.Fatalf("after trip: mode=%s state=%s, want auto/interlock", got.Mode, got.State)
	}
	if got.Output != got.OutMax {
		t.Fatalf("output = %.1f, want %.1f (oil pour)", got.Output, got.OutMax)
	}

	// Still interlocked at 74 °C (inside the 5 °C hysteresis band).
	putPV(base.Add(time.Second), 74.0)
	m.Tick(ctx)
	if got, _ = store.GetLoop(ctx, db, "tic201"); got.State != store.LoopStateInterlock {
		t.Fatalf("74 C must stay interlocked, state=%s", got.State)
	}

	// Below 70 °C: released back to manual, valve held.
	putPV(base.Add(2*time.Second), 69.0)
	m.Tick(ctx)
	if got, _ = store.GetLoop(ctx, db, "tic201"); got.State != store.LoopStateOK || got.Mode != store.LoopModeManual {
		t.Fatalf("after cooldown: mode=%s state=%s, want manual/ok", got.Mode, got.State)
	}
}

// openTestDB keeps this package standalone (no import of the api test env).
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
