package metallurgy_test

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"cap/internal/metallurgy"
	"cap/internal/store"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "met.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := store.SeedRegistry(ctx, db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}

// ingestGood stores one good numeric reading through the store layer.
func ingestGood(t *testing.T, ctx context.Context, db *sql.DB, tagID string, v float64) {
	t.Helper()
	tag, err := store.GetTag(ctx, db, tagID)
	if err != nil {
		t.Fatalf("get tag %s: %v", tagID, err)
	}
	value := v
	seq := int64(1)
	r := &store.Reading{
		ID: store.NewID(), MessageID: fmt.Sprintf("m-%s-%d", tagID, time.Now().UnixNano()),
		GatewayID: "test-gw", SourceSequence: &seq,
		ObservedAt: time.Now().UTC(), ReceivedAt: time.Now().UTC(),
		AssetID: tag.AssetID, TagID: tag.ID,
		ValueNumber: &value, Unit: tag.Unit, Quality: "good",
	}
	if err := store.InsertReading(ctx, db, r); err != nil {
		t.Fatalf("insert reading %s: %v", tagID, err)
	}
}

// TestTwoProductReferencePoint validates the two-product balance against the
// design numbers of TZ §5: α=0.80 %, β=22 %, θ=0.08 % → ε≈90.3 %, γ≈3.29 %.
func TestTwoProductReferencePoint(t *testing.T) {
	eps, gamma, upgrade, _ := metallurgy.TwoProduct(0.80, 22.0, 0.08)
	if math.Abs(eps-90.28) > 0.1 {
		t.Fatalf("epsilon = %.4f, want ~90.28", eps)
	}
	if math.Abs(gamma-3.285) > 0.01 {
		t.Fatalf("gamma = %.4f, want ~3.285", gamma)
	}
	if math.Abs(upgrade-27.5) > 0.01 {
		t.Fatalf("upgrade = %.4f, want 27.5", upgrade)
	}
}

// TestTwoProductDegenerateCases guards the division-by-zero corners.
func TestTwoProductDegenerateCases(t *testing.T) {
	if eps, _, _, _ := metallurgy.TwoProduct(0, 22, 0.08); eps != 0 {
		t.Fatalf("zero alpha must yield 0, got %v", eps)
	}
	if eps, _, _, _ := metallurgy.TwoProduct(0.8, 0.08, 0.08); eps != 0 {
		t.Fatalf("beta == theta must yield 0, got %v", eps)
	}
}

// TestBondEnergyReferencePoint: Wi=14.5, F80=6500 um, P80=150 um → W≈10.04 kWh/t.
func TestBondEnergyReferencePoint(t *testing.T) {
	w, _ := metallurgy.BondEnergy(14.5, 6500, 150)
	if math.Abs(w-10.04) > 0.05 {
		t.Fatalf("bond energy = %.4f, want ~10.04", w)
	}
	if w2, _ := metallurgy.BondEnergy(14.5, 150, 150); w2 != 0 {
		t.Fatalf("p80 == f80 must yield 0, got %v", w2)
	}
}

// TestPercentSolids: pulp 1520 g/l with 2.7 solids → 54.3 % solids. (The 30 %
// flotation feed figure of TZ §5 is the separate di301 tag — cyclone overflow
// diluted with wash water; di202 is the cyclone feed density.)
func TestPercentSolids(t *testing.T) {
	s, _ := metallurgy.PercentSolids(1520, 2.7)
	if math.Abs(s-54.3) > 0.3 {
		t.Fatalf("percent solids = %.2f, want ~54.3", s)
	}
	// 30 % solids corresponds to ~1233 g/l.
	diluted, _ := metallurgy.PercentSolids(1233, 2.7)
	if math.Abs(diluted-30.0) > 0.5 {
		t.Fatalf("percent solids at 1233 g/l = %.2f, want ~30", diluted)
	}
}

// TestComputeEndToEnd feeds a consistent nominal dataset through the store
// and verifies the computed balance is valid and near the design point.
func TestComputeEndToEnd(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	// Nominal operating point (TZ §5).
	values := map[string]float64{
		"plant.crushing.fi101":   100.0,
		"plant.flotation.wi301":  3.29,
		"plant.flotation.wi302":  96.71,
		"plant.flotation.afi301": 0.80,
		"plant.flotation.afc301": 22.0,
		"plant.flotation.aft301": 0.08,
		"plant.flotation.qi301":  180,
		"plant.grinding.ei201":   1250,
		"plant.grinding.xi201":   150,
		"plant.grinding.fi202":   410,
		"plant.grinding.di202":   1520,
		"plant.thickening.ei401": 55,
		"plant.thickening.li401": 3.0,
		"plant.filtration.wi501": 2.95,
		"plant.filtration.mi501": 10.5,
	}
	for tagID, v := range values {
		ingestGood(t, ctx, db, tagID, v)
	}

	snap := metallurgy.Compute(ctx, db)
	if !snap.Valid {
		t.Fatalf("snapshot invalid: %s", snap.Note)
	}
	byID := map[string]metallurgy.Kpi{}
	for _, k := range snap.Kpis {
		byID[k.ID] = k
	}
	if eps := byID["epsilon"]; math.Abs(eps.Value-90.28) > 0.2 {
		t.Fatalf("epsilon = %v, want ~90.28", eps.Value)
	}
	if balErr := byID["balance_err"]; math.Abs(balErr.Value) > 0.01 {
		t.Fatalf("balance_err = %v, want ~0", balErr.Value)
	}
	if byID["epsilon"].Formula == "" {
		t.Fatal("KPI formula missing (audit requirement)")
	}
	if byID["epsilon"].Status != "ok" {
		t.Fatalf("epsilon status = %v", byID["epsilon"].Status)
	}
	if len(snap.Streams) != 3 {
		t.Fatalf("streams = %d, want 3", len(snap.Streams))
	}
	if math.Abs(snap.Streams[0].MetalTPH-0.8) > 0.01 {
		t.Fatalf("feed metal = %v, want 0.8", snap.Streams[0].MetalTPH)
	}
}

// TestComputeDriftDetected: a drifting XRF feed grade breaks the mass-flow
// cross-check — the snapshot must flag it (detector of TZ §10.1).
func TestComputeDriftDetected(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	values := map[string]float64{
		"plant.crushing.fi101":   100.0,
		"plant.flotation.wi301":  3.29,
		"plant.flotation.wi302":  96.71,
		"plant.flotation.afi301": 1.10, // drifted up from 0.80
		"plant.flotation.afc301": 22.0,
		"plant.flotation.aft301": 0.08,
		"plant.flotation.qi301":  180,
		"plant.grinding.xi201":   150,
	}
	for tagID, v := range values {
		ingestGood(t, ctx, db, tagID, v)
	}
	snap := metallurgy.Compute(ctx, db)
	if snap.Valid {
		t.Fatal("drifted XRF must invalidate the snapshot")
	}
}

// TestTargetsFromProfile checks the acceptance-corridor parser.
func TestTargetsFromProfile(t *testing.T) {
	targets := metallurgy.TargetsFromProfile(`{"alpha_nominal":0.7,"beta_target":20,"epsilon_target_min":85}`)
	if targets.AlphaNominal != 0.7 || targets.BetaTarget != 20 || targets.EpsilonMin == nil || *targets.EpsilonMin != 85 {
		t.Fatalf("targets = %+v", targets)
	}
	// Missing keys keep design defaults.
	fallback := metallurgy.TargetsFromProfile(`{}`)
	if fallback.AlphaNominal != 0.80 || fallback.WiKwhT != 14.5 {
		t.Fatalf("fallback targets = %+v", fallback)
	}
}
