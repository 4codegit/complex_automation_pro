package alarms_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"cap/internal/alarms"
	"cap/internal/hub"
	"cap/internal/store"
)

// newHarness seeds the registry + rationalised limits and returns the engine,
// the database and a controllable clock.
func newHarness(t *testing.T) (*alarms.Engine, *sql.DB, *time.Time) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "alarms.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := store.SeedRegistry(ctx, db); err != nil {
		t.Fatalf("seed registry: %v", err)
	}
	if err := store.SeedAlarmLimits(ctx, db); err != nil {
		t.Fatalf("seed limits: %v", err)
	}

	h := hub.New()
	eng := alarms.New(db, h)
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	eng.SetNow(func() time.Time { return now })
	return eng, db, &now
}

// evaluate feeds one tails-grade reading through the engine.
func evaluate(t *testing.T, eng *alarms.Engine, db *sql.DB, value float64, quality string) {
	t.Helper()
	ctx := context.Background()
	tag, err := store.GetTag(ctx, db, "plant.flotation.aft301")
	if err != nil {
		t.Fatalf("get tag: %v", err)
	}
	v := value
	seq := int64(1)
	r := &store.Reading{
		ID: "r1", MessageID: "m1", GatewayID: "gw",
		ObservedAt: time.Now().UTC(), ReceivedAt: time.Now().UTC(),
		AssetID: tag.AssetID, TagID: tag.ID,
		ValueNumber: &v, Unit: tag.Unit, Quality: quality,
		SourceSequence: &seq,
	}
	eng.Evaluate(ctx, tag, r)
}

func activeAlarms(t *testing.T, db *sql.DB) []store.Alarm {
	t.Helper()
	rows, err := store.ListActiveAlarms(context.Background(), db, 10)
	if err != nil {
		t.Fatalf("list alarms: %v", err)
	}
	return rows
}

// TestCriticalBandDelay: a hi_hi excursion shorter than the 3 s critical
// on-delay raises nothing; a sustained one raises a critical alarm.
func TestCriticalBandDelay(t *testing.T) {
	eng, db, now := newHarness(t)

	// Brief excursion (2 s < 3 s delay).
	evaluate(t, eng, db, 0.19, "good")
	*now = now.Add(2 * time.Second)
	evaluate(t, eng, db, 0.19, "good")
	if n := len(activeAlarms(t, db)); n != 0 {
		t.Fatalf("alarm raised after 2s, want none (rows=%+v)", activeAlarms(t, db))
	}

	// Sustained excursion (4 s total).
	*now = now.Add(2 * time.Second)
	evaluate(t, eng, db, 0.19, "good")
	rows := activeAlarms(t, db)
	if len(rows) != 1 {
		t.Fatalf("active alarms = %d, want 1", len(rows))
	}
	if rows[0].Severity != "critical" || rows[0].TagID != "plant.flotation.aft301" {
		t.Fatalf("alarm = %+v", rows[0])
	}

	// The journal carries the raised event.
	events, err := store.QueryAlarmEvents(context.Background(), db, store.AlarmEventFilter{
		TagID: "plant.flotation.aft301",
	})
	if err != nil {
		t.Fatalf("journal: %v", err)
	}
	if len(events) == 0 || events[0].Event != store.AlarmEventRaised {
		t.Fatalf("journal events = %+v", events)
	}
}

// TestClearRequiresHysteresisAndDelay: a value back inside limits clears the
// alarm only after the clear delay.
func TestClearRequiresHysteresisAndDelay(t *testing.T) {
	eng, db, now := newHarness(t)

	evaluate(t, eng, db, 0.19, "good")
	*now = now.Add(4 * time.Second)
	evaluate(t, eng, db, 0.19, "good")
	if len(activeAlarms(t, db)) != 1 {
		t.Fatal("alarm not raised in setup")
	}

	// Back to 0.10 (< hi 0.12 − hysteresis 0.0049): clear pending, 5 s delay.
	evaluate(t, eng, db, 0.10, "good")
	*now = now.Add(4 * time.Second)
	evaluate(t, eng, db, 0.10, "good")
	if len(activeAlarms(t, db)) != 1 {
		t.Fatal("alarm cleared before the clear delay")
	}
	*now = now.Add(2 * time.Second)
	evaluate(t, eng, db, 0.10, "good")
	if n := len(activeAlarms(t, db)); n != 0 {
		t.Fatalf("alarm still active after clear delay: %+v", activeAlarms(t, db))
	}
}

// TestGapQualityNeverClears: an offline reading freezes the alarm state — a
// lost signal must not fabricate a return to normal.
func TestGapQualityNeverClears(t *testing.T) {
	eng, db, now := newHarness(t)

	evaluate(t, eng, db, 0.19, "good")
	*now = now.Add(4 * time.Second)
	evaluate(t, eng, db, 0.19, "good")
	if len(activeAlarms(t, db)) != 1 {
		t.Fatal("alarm not raised in setup")
	}

	for i := 1; i <= 5; i++ {
		*now = now.Add(time.Duration(i) * time.Minute)
		evaluate(t, eng, db, 0, "offline")
	}
	if len(activeAlarms(t, db)) != 1 {
		t.Fatal("gap quality cleared the alarm, want frozen state")
	}
}

// TestCommLossWatchdog: an asset with no good data for >10 s raises the
// comm_loss alarm on Sweep; fresh data clears it.
func TestCommLossWatchdog(t *testing.T) {
	eng, db, now := newHarness(t)

	evaluate(t, eng, db, 0.10, "good")

	// Jump 11 s ahead without data.
	*now = now.Add(11 * time.Second)
	eng.Sweep(context.Background())

	comm := map[string]bool{}
	for _, a := range activeAlarms(t, db) {
		comm[a.TagID] = true
	}
	if !comm["comm:plant.flotation"] {
		t.Fatalf("comm_loss not raised: %+v", activeAlarms(t, db))
	}

	// Data returns → the next Sweep clears it.
	evaluate(t, eng, db, 0.10, "good")
	*now = now.Add(3 * time.Second)
	eng.Sweep(context.Background())
	for _, a := range activeAlarms(t, db) {
		if a.TagID == "comm:plant.flotation" {
			t.Fatal("comm_loss not cleared after data returned")
		}
	}
}
