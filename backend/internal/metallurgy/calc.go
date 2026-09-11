// Package metallurgy implements the metallurgical balance calculations
// (TZ §10): two-product balance, Bond grinding energy, pulp conversions and
// node duties. Every result carries its formula string so the UI and the
// audit trail can show exactly how each number was produced.
package metallurgy

import (
	"fmt"
	"math"
)

// SolidsSG is the solids density of the treated ore, t/m3 (profile-
// overridable in principle; fixed for the MVP, TZ §10.3).
const SolidsSG = 2.7

// WaterSG is the water density, t/m3.
const WaterSG = 1.0

// PercentSolids converts pulp density (g/l) to percent solids by mass:
// S = ρs·(ρp−ρw) / ((ρs−ρw)·ρp) × 100.
func PercentSolids(pulpGL, solidsSG float64) (float64, string) {
	rhoP := pulpGL / 1000.0 // t/m3
	if rhoP <= WaterSG || solidsSG <= WaterSG {
		return 0, "%S = ρs(ρp−ρw)/((ρs−ρw)ρp)×100"
	}
	s := solidsSG * (rhoP - WaterSG) / ((solidsSG - WaterSG) * rhoP) * 100
	return s, fmt.Sprintf("%%S = %.1f·(%.3f−%.1f)/((%.1f−%.1f)·%.3f)×100",
		solidsSG, rhoP, WaterSG, solidsSG, WaterSG, rhoP)
}

// TwoProduct is the classic two-product metallurgical balance. Inputs are the
// Cu grades of feed, concentrate and tails in percent. It returns recovery
// (epsilon, %), concentrate yield (gamma, %) and the upgrade ratio.
func TwoProduct(alpha, beta, theta float64) (epsilon, gamma, upgrade float64, formula string) {
	const f = "ε = β(α−θ)/(α(β−θ))×100; γ = (α−θ)/(β−θ)×100; K = β/α"
	if alpha <= 0 || beta <= theta || alpha <= theta {
		return 0, 0, 0, f
	}
	epsilon = beta * (alpha - theta) / (alpha * (beta - theta)) * 100
	gamma = (alpha - theta) / (beta - theta) * 100
	upgrade = beta / alpha
	return epsilon, gamma, upgrade, f
}

// BondEnergy is Bond's third-theory specific grinding energy, kWh/t:
// W = 10·Wi·(1/√P80 − 1/√F80), sizes in microns.
func BondEnergy(wi, f80, p80 float64) (float64, string) {
	const f = "W = 10·Wi·(1/√P80 − 1/√F80)"
	if f80 <= 0 || p80 <= 0 || p80 >= f80 {
		return 0, f
	}
	w := 10 * wi * (1/math.Sqrt(p80) - 1/math.Sqrt(f80))
	return w, f
}

// CirculatingLoad computes the circulating load percent from the cyclone feed
// dry solids flow and the fresh mill feed: CL = (Qcyc_dry/Qfresh)×100 − 100.
func CirculatingLoad(qCycloneDryTPH, qFreshTPH float64) (float64, string) {
	const f = "CL = (Q_цикл/Q_свеж)×100 − 100"
	if qFreshTPH <= 0 {
		return 0, f
	}
	return qCycloneDryTPH/qFreshTPH*100 - 100, f
}

// SpecificCollector is the collector dose per tonne of ore, ml/t:
// q = Q_collector[ml/min]×60 / Q_feed[t/h].
func SpecificCollector(collectorMLMin, feedTPH float64) (float64, string) {
	const f = "q = Q_соб×60/Q_руда"
	if feedTPH <= 0 {
		return 0, f
	}
	return collectorMLMin * 60 / feedTPH, f
}

// ThickenerDuty is the thickener solids loading, t/m2·h.
func ThickenerDuty(solidsTPH, areaM2 float64) (float64, string) {
	const f = "q = W/A"
	if areaM2 <= 0 {
		return 0, f
	}
	return solidsTPH / areaM2, f
}

// Balance streams for the summary payload.
type Stream struct {
	Key      string  `json:"stream"`
	Label    string  `json:"label"`
	TPH      float64 `json:"tph"`
	GradePct float64 `json:"grade_pct"`
	MetalTPH float64 `json:"metal_tph"`
}

// Kpi is one computed indicator with its audit formula and sources.
type Kpi struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Value   float64  `json:"value"`
	Unit    string   `json:"unit"`
	Status  string   `json:"status"` // ok | warn | bad | no_data
	Formula string   `json:"formula"`
	Target  *Target  `json:"target,omitempty"`
	Src     []string `json:"src"`
	Note    string   `json:"note,omitempty"`
}

// Target is the acceptance corridor from the active ore profile.
type Target struct {
	Min *float64 `json:"min"`
	Max *float64 `json:"max"`
}

func statusFor(v float64, t *Target) string {
	if t == nil {
		return "ok"
	}
	if t.Min != nil && v < *t.Min {
		return "warn"
	}
	if t.Max != nil && v > *t.Max {
		return "warn"
	}
	return "ok"
}
