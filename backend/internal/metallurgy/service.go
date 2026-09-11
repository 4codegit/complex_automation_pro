package metallurgy

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"

	"cap/internal/schema"
	"cap/internal/store"
)

// Ingestor is the single telemetry entry point (the canonical ingest path).
// Virtual calc tags go through it so they are validated, stored, broadcast
// and journalised exactly like field measurements.
type Ingestor func(ctx context.Context, t *schema.Telemetry) error

// Snapshot is one computed metallurgical balance at a point in time.
type Snapshot struct {
	ComputedAt time.Time `json:"computed_at"`
	Valid      bool      `json:"valid"`
	Note       string    `json:"note,omitempty"`
	Streams    []Stream  `json:"balance,omitempty"`
	Kpis       []Kpi     `json:"kpis,omitempty"`
}

// Service periodically computes the balance and writes the virtual calc tags
// through the ingest path so they appear in the historian and on trends.
type Service struct {
	db     *sql.DB
	ingest Ingestor
	period time.Duration
}

// New creates the calc service.
func New(db *sql.DB, ingest Ingestor, period time.Duration) *Service {
	if period <= 0 {
		period = 5 * time.Second
	}
	return &Service{db: db, ingest: ingest, period: period}
}

// Run blocks until ctx is cancelled, computing on the configured cadence.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.period)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// latestGood fetches the newest good numeric value of a tag (empty if none).
func latestGood(ctx context.Context, db *sql.DB, tagID string) (float64, time.Time, bool) {
	r, err := store.LatestGoodNumericReading(ctx, db, tagID)
	if err != nil || r.ValueNumber == nil {
		return 0, time.Time{}, false
	}
	return *r.ValueNumber, r.ObservedAt, true
}

// SnapshotValues holds the tag values one computation is based on.
type SnapshotValues struct {
	Feed, Conc, Tails            float64 // fi101, wi301, wi302
	Alpha, Beta, Theta           float64 // afi301, afc301, aft301
	Collector                    float64 // qi301
	MillPower, P80               float64 // ei201, xi201
	CyclonePulp, CycloneDensity  float64 // fi202, di202
	ThickenerTorque, ThickBedLvl float64 // ei401, li401
	CakeTPH, CakeMoist           float64 // wi501, mi501
	OK                           map[string]bool
}

// Collect reads all source tags once.
func Collect(ctx context.Context, db *sql.DB) SnapshotValues {
	v := SnapshotValues{OK: map[string]bool{}}
	type src struct {
		tag string
		dst *float64
	}
	for _, s := range []src{
		{"plant.crushing.fi101", &v.Feed},
		{"plant.flotation.wi301", &v.Conc},
		{"plant.flotation.wi302", &v.Tails},
		{"plant.flotation.afi301", &v.Alpha},
		{"plant.flotation.afc301", &v.Beta},
		{"plant.flotation.aft301", &v.Theta},
		{"plant.flotation.qi301", &v.Collector},
		{"plant.grinding.ei201", &v.MillPower},
		{"plant.grinding.xi201", &v.P80},
		{"plant.grinding.fi202", &v.CyclonePulp},
		{"plant.grinding.di202", &v.CycloneDensity},
		{"plant.thickening.ei401", &v.ThickenerTorque},
		{"plant.thickening.li401", &v.ThickBedLvl},
		{"plant.filtration.wi501", &v.CakeTPH},
		{"plant.filtration.mi501", &v.CakeMoist},
	} {
		val, _, ok := latestGood(ctx, db, s.tag)
		v.OK[s.tag] = ok
		*s.dst = val
	}
	return v
}

// ProfileTargets extracts the acceptance corridor from the active ore profile.
type ProfileTargets struct {
	AlphaNominal     float64
	BetaTarget       float64
	EpsilonMin       *float64
	DoseRef          float64
	WiKwhT           float64
	F80Um            float64
	ThickenerMaxLoad *float64
}

// TargetsFromProfile parses the profile params JSON (missing keys keep defaults).
func TargetsFromProfile(params string) ProfileTargets {
	t := ProfileTargets{AlphaNominal: 0.80, BetaTarget: 22.0, DoseRef: 108, WiKwhT: 14.5, F80Um: 6500}
	if params == "" {
		return t
	}
	var raw struct {
		AlphaNominal     float64  `json:"alpha_nominal"`
		BetaTarget       float64  `json:"beta_target"`
		EpsilonTargetMin *float64 `json:"epsilon_target_min"`
		DoseRefMLPerT    float64  `json:"dose_ref_ml_per_t"`
		WiKwhT           float64  `json:"wi_kwh_t"`
		F80Um            float64  `json:"f80_um"`
		ThickenerLoadMax *float64 `json:"thickener_load_max"`
	}
	if err := json.Unmarshal([]byte(params), &raw); err != nil {
		return t
	}
	if raw.AlphaNominal > 0 {
		t.AlphaNominal = raw.AlphaNominal
	}
	if raw.BetaTarget > 0 {
		t.BetaTarget = raw.BetaTarget
	}
	if raw.DoseRefMLPerT > 0 {
		t.DoseRef = raw.DoseRefMLPerT
	}
	if raw.WiKwhT > 0 {
		t.WiKwhT = raw.WiKwhT
	}
	if raw.F80Um > 0 {
		t.F80Um = raw.F80Um
	}
	t.EpsilonMin = raw.EpsilonTargetMin
	t.ThickenerMaxLoad = raw.ThickenerLoadMax
	return t
}

// Compute evaluates the full balance snapshot (TZ §10). Inputs come from the
// latest good readings; missing sources degrade gracefully into no_data KPIs.
func Compute(ctx context.Context, db *sql.DB) Snapshot {
	v := Collect(ctx, db)
	targets := ProfileTargets{AlphaNominal: 0.80, BetaTarget: 22, DoseRef: 108, WiKwhT: 14.5, F80Um: 6500}
	if p, err := store.GetActiveProfile(ctx, db); err == nil {
		targets = TargetsFromProfile(p.Params)
	}

	snap := Snapshot{ComputedAt: time.Now().UTC(), Valid: true}
	coreOK := v.OK["plant.crushing.fi101"] && v.OK["plant.flotation.afi301"] &&
		v.OK["plant.flotation.afc301"] && v.OK["plant.flotation.aft301"]

	kpis := make([]Kpi, 0, 8)

	// --- Two-product balance ---
	if coreOK {
		eps, gamma, upgrade, formula := TwoProduct(v.Alpha, v.Beta, v.Theta)
		balanceErr := 0.0
		if v.Feed > 0 {
			balanceErr = (v.Feed - v.Conc - v.Tails) / v.Feed * 100
		}
		massOK := math.Abs(balanceErr) <= 1.0
		gradeOK := math.Abs(balanceErr) <= 2.0 && v.Beta > 0 && v.Beta < 40 && v.Theta < v.Alpha

		snap.Streams = []Stream{
			{Key: "feed", Label: "Питание", TPH: v.Feed, GradePct: v.Alpha, MetalTPH: v.Feed * v.Alpha / 100},
			{Key: "conc", Label: "Концентрат", TPH: v.Conc, GradePct: v.Beta, MetalTPH: v.Conc * v.Beta / 100},
			{Key: "tails", Label: "Хвосты", TPH: v.Tails, GradePct: v.Theta, MetalTPH: v.Tails * v.Theta / 100},
		}

		kpis = append(kpis,
			Kpi{ID: "epsilon", Label: "Извлечение Cu", Value: round(eps, 2), Unit: "%",
				Status: statusFor(eps, &Target{Min: targets.EpsilonMin}), Formula: formula,
				Src: []string{"afi301", "afc301", "aft301"}},
			Kpi{ID: "gamma", Label: "Выход концентрата", Value: round(gamma, 3), Unit: "%",
				Status: "ok", Formula: formula, Src: []string{"afi301", "afc301", "aft301"}},
			Kpi{ID: "upgrade", Label: "Коэффициент обогащения", Value: round(upgrade, 2), Unit: "x",
				Status: "ok", Formula: "K = β/α", Src: []string{"afi301", "afc301"}},
		)

		// Mass balance closure: (feed − conc − tails)/feed × 100.
		kpis = append(kpis, Kpi{ID: "balance_err", Label: "Невязка массового баланса",
			Value: round(balanceErr, 2), Unit: "%",
			Status:  map[bool]string{true: "ok", false: "warn"}[massOK],
			Formula: "err = (Q_пит − Q_конц − Q_хв)/Q_пит×100",
			Src:     []string{"fi101", "wi301", "wi302"},
			Note:    map[bool]string{true: "", false: "данные несогласованы: проверь XRF/весы"}[massOK]})
		if !gradeOK {
			snap.Valid = false
			snap.Note = "баланс не сходится: проверь анализаторы XRF и весы"
		}

		// Cross-check: recovery from mass flow vs two-product recovery.
		if v.Alpha > 0 && eps > 0 {
			metalIn := v.Feed * v.Alpha / 100
			epsMass := 0.0
			if metalIn > 0 {
				epsMass = v.Conc * v.Beta / 100 / metalIn * 100
			}
			delta := 0.0
			if eps > 0 {
				delta = (eps - epsMass) / eps * 100
			}
			checkOK := math.Abs(delta) <= 5.0 // XRF sample&hold noise ~1.5% rel
			kpis = append(kpis, Kpi{ID: "metal_check", Label: "Сходимость извлечений",
				Value: round(delta, 2), Unit: "%",
				Status:  map[bool]string{true: "ok", false: "warn"}[checkOK],
				Formula: "δ = (ε_двухпрод − ε_масс)/ε_двухпрод×100",
				Src:     []string{"fi101", "wi301", "afi301", "afc301"},
				Note:    map[bool]string{true: "", false: "расхождение потоков и анализаторов"}[checkOK]})
			if !checkOK {
				snap.Valid = false
			}
		}
	} else {
		snap.Valid = false
		snap.Note = "нет полных данных по качеству (XRF) или расходам"
		kpis = append(kpis, Kpi{ID: "epsilon", Label: "Извлечение Cu", Unit: "%",
			Status: "no_data", Formula: "ε = β(α−θ)/(α(β−θ))×100",
			Src: []string{"afi301", "afc301", "aft301"}, Note: "нет достоверных данных"})
	}

	// --- Grinding energy (Bond) ---
	if v.OK["plant.grinding.xi201"] && v.OK["plant.crushing.fi101"] {
		w, formula := BondEnergy(targets.WiKwhT, targets.F80Um, v.P80)
		kpis = append(kpis, Kpi{ID: "bond", Label: "Удельная энергия измельчения",
			Value: round(w, 2), Unit: "kWh/t", Status: "ok", Formula: formula,
			Src: []string{"xi201"}})
		if v.OK["plant.grinding.ei201"] {
			theoretical := w * v.Feed
			eff := 0.0
			if v.MillPower > 0 {
				eff = theoretical / v.MillPower * 100
			}
			kpis = append(kpis, Kpi{ID: "mill_eff", Label: "Энергетический КПД мельницы",
				Value: round(eff, 1), Unit: "%", Status: "ok",
				Formula: "η = W·Q_руда/EI_мельн×100", Src: []string{"xi201", "ei201", "fi101"}})
		}
	}

	// --- Circulating load ---
	if v.OK["plant.grinding.fi202"] && v.OK["plant.grinding.di202"] && v.Feed > 0 {
		solids, sf := PercentSolids(v.CycloneDensity, SolidsSG)
		// Q_pulp [m3/h] × S [%] × ρp [t/m3] = dry solids t/h
		qDry := v.CyclonePulp * (solids / 100) * (v.CycloneDensity / 1000)
		cl, cf := CirculatingLoad(qDry, v.Feed)
		kpis = append(kpis, Kpi{ID: "cl", Label: "Циркулирующая нагрузка",
			Value: round(cl, 1), Unit: "%", Status: "ok", Formula: cf + " (S: " + sf + ")",
			Src: []string{"fi202", "di202", "fi101"}})
	}

	// --- Reagent dose ---
	if v.OK["plant.flotation.qi301"] && v.Feed > 0 {
		q, formula := SpecificCollector(v.Collector, v.Feed)
		kpis = append(kpis, Kpi{ID: "q_collector", Label: "Удельный расход собирателя",
			Value: round(q, 1), Unit: "ml/t", Status: "ok", Formula: formula,
			Src: []string{"qi301", "fi101"}})
	}

	// --- Thickening / filtration duties ---
	if v.OK["plant.flotation.wi301"] {
		duty, formula := ThickenerDuty(v.Conc, 28.3)
		kpis = append(kpis, Kpi{ID: "thickener_duty", Label: "Нагрузка сгустителя",
			Value: round(duty, 3), Unit: "t/m2·h", Status: "ok", Formula: formula,
			Src: []string{"wi301"}})
	}
	if v.OK["plant.filtration.wi501"] && v.OK["plant.filtration.mi501"] && v.CakeMoist < 100 {
		water := v.CakeTPH * v.CakeMoist / (100 - v.CakeMoist)
		kpis = append(kpis, Kpi{ID: "cake_water", Label: "Вода с кеком",
			Value: round(water, 2), Unit: "t/h", Status: "ok",
			Formula: "W_вода = Q_кек×w/(100−w)", Src: []string{"wi501", "mi501"}})
	}

	snap.Kpis = kpis
	return snap
}

// calcTags maps computed KPIs onto the virtual registry tags (TZ §10).
func calcTags(snap Snapshot) map[string]float64 {
	out := map[string]float64{}
	for _, k := range snap.Kpis {
		switch k.ID {
		case "epsilon":
			out["plant.metallurgy.calc_epsilon"] = k.Value
		case "gamma":
			out["plant.metallurgy.calc_gamma"] = k.Value
		case "upgrade":
			out["plant.metallurgy.calc_upgrade"] = k.Value
		case "balance_err":
			out["plant.metallurgy.calc_balance_err"] = k.Value
		case "q_collector":
			out["plant.metallurgy.calc_q_collector"] = k.Value
		case "bond":
			out["plant.metallurgy.calc_bond_kwt"] = k.Value
		case "cl":
			out["plant.metallurgy.calc_cl_pct"] = k.Value
		}
	}
	return out
}

// tick computes and writes virtual tags through the canonical ingest path.
func (s *Service) tick(ctx context.Context) {
	snap := Compute(ctx, s.db)
	if !snap.Valid {
		return
	}
	bucket := snap.ComputedAt.Unix() / int64(s.period.Seconds())
	for tagID, value := range calcTags(snap) {
		t := &schema.Telemetry{
			SchemaVersion: schema.SchemaVersion,
			MessageID:     fmt.Sprintf("calc-%s-%d", tagID, bucket),
			GatewayID:     "calc",
			ObservedAt:    snap.ComputedAt,
			AssetID:       "plant.metallurgy",
			TagID:         tagID,
			Value:         value,
			Unit:          calcUnit(tagID),
			Quality:       schema.QualityGood,
		}
		if err := s.ingest(ctx, t); err != nil {
			log.Printf("[metallurgy] ingest %s: %v", tagID, err)
		}
	}
}

func calcUnit(tagID string) string {
	switch tagID {
	case "plant.metallurgy.calc_epsilon", "plant.metallurgy.calc_gamma",
		"plant.metallurgy.calc_pull", "plant.metallurgy.calc_balance_err",
		"plant.metallurgy.calc_cl_pct":
		return "%"
	case "plant.metallurgy.calc_upgrade":
		return "x"
	case "plant.metallurgy.calc_q_collector":
		return "ml/t"
	case "plant.metallurgy.calc_bond_kwt":
		return "kWh/t"
	}
	return ""
}

// Summary is the API-facing snapshot (fresh computation each call).
func Summary(ctx context.Context, db *sql.DB) Snapshot {
	return Compute(ctx, db)
}

func round(v float64, digits int) float64 {
	p := 1.0
	for i := 0; i < digits; i++ {
		p *= 10
	}
	return math.Round(v*p) / p
}
