package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"cap/internal/store"
)

// ProcessAnalytics derives metallurgical indicators from the latest raw
// readings and the active ore profile. CAP never invents raw values: every
// KPI here is a transparent computation over measured tags, with the formula
// exposed so a metallurgist can audit it.
//
// Assumptions are explicit constants (SG of solids, warn margins) — they are
// per-site configuration candidates, not universal truths.
//
//	GET /api/v1/analytics/process
type analyticsKPI struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Value   *float64 `json:"value"`
	Unit    string   `json:"unit"`
	Status  string   `json:"status"` // ok | warn | bad | nodata
	Detail  string   `json:"detail"`
	Formula string   `json:"formula"`
}

type analyticsResponse struct {
	GeneratedAt string         `json:"generated_at"`
	Profile     *profileRef    `json:"profile"`
	KPIs        []analyticsKPI `json:"kpis"`
}

type profileRef struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// SG of metal-sulfide solids, g/cm3 (site-configurable assumption).
const solidsSG = 2.7

type thresholdBand struct {
	Min *float64 `json:"min"`
	Max *float64 `json:"max"`
}

type profileParams struct {
	Thresholds map[string]thresholdBand `json:"thresholds"`
	Baseline   map[string]float64       `json:"baseline"`
}

// ProcessAnalytics handles GET /api/v1/analytics/process.
func (s *Server) ProcessAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	latest := func(tagID string) (float64, bool) {
		reading, err := store.LatestReading(ctx, s.db, tagID)
		if err != nil || reading == nil {
			return 0, false
		}
		v, ok := reading.Value().(float64)
		return v, ok
	}
	missing := func(ids ...string) bool {
		for _, id := range ids {
			if _, ok := latest(id); !ok {
				return true
			}
		}
		return false
	}

	var params profileParams
	var profile *profileRef
	if active, err := store.GetActiveProfile(ctx, s.db); err == nil && active != nil {
		profile = &profileRef{ID: active.ID, Name: active.Name, Version: active.Version}
		_ = json.Unmarshal([]byte(active.Params), &params)
	}

	kpis := make([]analyticsKPI, 0, 6)
	band := func(metric string) (min, max float64, ok bool) {
		t, has := params.Thresholds[metric]
		if !has || t.Min == nil || t.Max == nil {
			return math.NaN(), math.NaN(), false
		}
		return *t.Min, *t.Max, true
	}

	// 1. Percent solids in pulp from slurry density (mass balance on 1 m3:
	// solids volume x = (rho_p - rho_w)/(rho_s - rho_w), %S = rho_s*x/rho_p).
	if rho, ok := latest("plant-a.crushing.pulp_density"); ok && rho > 1.0001 {
		pct := solidsSG * (rho - 1) / ((solidsSG - 1) * rho) * 100
		status := "ok"
		switch {
		case pct < 25 || pct > 65:
			status = "bad"
		case pct < 30 || pct > 60:
			status = "warn"
		}
		kpis = append(kpis, analyticsKPI{
			ID: "solids_percent", Label: "Твёрдое в пульпе", Value: f64(pct), Unit: "%", Status: status,
			Detail:  rnz("по плотности пульпы %.2f г/см³ (SG твёрдого %.1f)", rho, solidsSG),
			Formula: "%S = ρт·(ρп−ρв) / ((ρт−ρв)·ρп) · 100",
		})
	} else {
		kpis = append(kpis, kpiNodata("solids_percent", "Твёрдое в пульпе", "нет плотности пульпы"))
	}

	// 2. Specific reagent consumption, mL per tonne of ore.
	dosage, hasDosage := latest("plant-a.flotation.reagent_dosage")
	tonnage, hasTonnage := latest("plant-a.concentrate.tonnage_weight")
	switch {
	case hasDosage && hasTonnage && tonnage > 0:
		specific := dosage * 60 / tonnage
		kpis = append(kpis, analyticsKPI{
			ID: "reagent_specific", Label: "Удельный расход реагента", Value: f64(specific), Unit: "мЛ/т", Status: "ok",
			Detail:  rnz("%.0f мЛ/мин на %.0f т/ч", dosage, tonnage),
			Formula: "q = Qр·60 / Qт",
		})
	default:
		kpis = append(kpis, kpiNodata("reagent_specific", "Удельный расход реагента", "нет дозировки или тоннажа"))
	}

	// 3. Dry solids throughput of the concentrate stream.
	if hasTonnage && tonnage > 0 {
		moisture, hasMoisture := latest("plant-a.concentrate.final_moisture")
		if !hasMoisture {
			moisture, hasMoisture = latest("plant-a.dewatering.cake_moisture")
		}
		if hasMoisture && moisture >= 0 && moisture < 100 {
			dry := tonnage * (1 - moisture/100)
			kpis = append(kpis, analyticsKPI{
				ID: "dry_throughput", Label: "Сухая производительность", Value: f64(dry), Unit: "т/ч", Status: "ok",
				Detail:  rnz("%.0f т/ч влажного при %.1f%% влаги", tonnage, moisture),
				Formula: "Qсух = Qт·(1 − W/100)",
			})
		}
	}

	// 4-6. Corridor checks against the active profile thresholds.
	corridor := func(id, label, tagID, unit, formula string) {
		v, has := latest(tagID)
		if !has {
			kpis = append(kpis, kpiNodata(id, label, "нет данных тега "+tagID))
			return
		}
		metric := tagID[strings.LastIndex(tagID, ".")+1:]
		min, max, hasBand := band(metric)
		status, detail := "ok", ""
		switch {
		case !hasBand:
			status, detail = "nodata", "в профиле нет порогов для "+tagID
		case v < min || v > max:
			status = "bad"
			if v < min {
				detail = rnz("ниже коридора %.1f–%.1f на %.2f", min, max, min-v)
			} else {
				detail = rnz("выше коридора %.1f–%.1f на %.2f", min, max, v-max)
			}
		default:
			margin := math.Min(v-min, max-v)
			edge := (max - min) * 0.1
			if margin <= edge {
				status = "warn"
				detail = rnz("у границы коридора %.1f–%.1f, запас %.2f", min, max, margin)
			} else {
				detail = rnz("в коридоре %.1f–%.1f, запас %.2f", min, max, margin)
			}
		}
		kpis = append(kpis, analyticsKPI{ID: id, Label: label, Value: f64(v), Unit: unit, Status: status, Detail: detail, Formula: formula})
	}
	corridor("ph_corridor", "pH: коридор профиля", "plant-a.flotation.ph_level", "pH",
		"статус по thresholds.ph_level активного профиля")
	corridor("p80_corridor", "Крупность P80: коридор", "plant-a.crushing.particle_size", "мм",
		"статус по thresholds.particle_size активного профиля")
	corridor("moisture_vs_target", "Влажность кека: к цели", "plant-a.dewatering.cake_moisture", "%",
		"статус по thresholds.cake_moisture активного профиля")

	// Sensor coverage: how many of the demo registry's key tags actually stream.
	coverage := struct {
		Missing []string `json:"missing,omitempty"`
	}{}
	for _, id := range []string{
		"plant-a.crushing.pulp_density", "plant-a.flotation.reagent_dosage",
		"plant-a.concentrate.tonnage_weight", "plant-a.flotation.ph_level",
		"plant-a.dewatering.cake_moisture", "plant-a.crushing.particle_size",
	} {
		if missing(id) {
			coverage.Missing = append(coverage.Missing, id)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at":       s.now().UTC(),
		"profile":            profile,
		"kpis":               kpis,
		"missing_raw_inputs": coverage.Missing,
	})
}

func f64(v float64) *float64 { return &v }

// kpiNodata builds an explicit "no data" KPI so gaps are visible, not silent.
func kpiNodata(id, label, reason string) analyticsKPI {
	return analyticsKPI{ID: id, Label: label, Status: "nodata", Detail: reason}
}

// rnz formats and trims trailing zeros: compact readouts without "1.50"-style noise.
func rnz(format string, args ...any) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf(format, args...), "0"), ".")
}
