package plantsim_test

import (
	"testing"
	"time"

	"cap/internal/plantsim"
)

// run advances the model n ticks of dt and returns the final sensor image.
func run(m *plantsim.Model, dt time.Duration, n int) {
	for i := 0; i < n; i++ {
		m.Tick(dt)
	}
}

// image converts the sensor registers to engineering values.
func eng(m *plantsim.Model, reg int) float64 {
	img := m.Input()
	return float64(img[reg]) * scaleOf(reg)
}

func scaleOf(reg int) float64 {
	scales := []float64{
		0.1, 0.1, 0.1, 0.1, 0.1, 1, 1, 0.1,
		1, 1, 0.01, 1, 1, 0.1, 0.001, 0.01,
		0.001, 0.01, 0.1, 0.01, 0.1, 0.1, 0.1, 0.1,
		0.01, 0.01, 0.1, 0.1,
	}
	return scales[reg]
}

// TestNominalSteadyState runs the model from its warm start for 300 s of
// simulated time and checks it stays at the nominal operating point of TZ §5
// with a closed mass balance.
func TestNominalSteadyState(t *testing.T) {
	m := plantsim.New(42, time.Now())
	run(m, 100*time.Millisecond, 3000) // 300 s simulated

	fi101 := eng(m, plantsim.RegFI101)
	wi301 := eng(m, plantsim.RegWI301)
	wi302 := eng(m, plantsim.RegWI302)

	if diff := fi101 - 100.0; diff > 3 || diff < -3 {
		t.Fatalf("fi101 = %.2f, want 100±3", fi101)
	}
	// Two-product recovery from the measured grades.
	alpha := eng(m, plantsim.RegAFI301)
	beta := eng(m, plantsim.RegAFC301)
	theta := eng(m, plantsim.RegAFT301)
	if beta <= theta || alpha <= theta {
		t.Fatalf("degenerate grades: a=%v b=%v t=%v", alpha, beta, theta)
	}
	eps := beta * (alpha - theta) / (alpha * (beta - theta)) * 100
	if eps < 88 || eps > 92 {
		t.Fatalf("recovery = %.2f%%, want 88..92 (a=%.3f b=%.2f t=%.3f)", eps, alpha, beta, theta)
	}
	if beta < 21 || beta > 23 {
		t.Fatalf("beta = %.2f, want 21..23", beta)
	}
	if theta < 0.06 || theta > 0.11 {
		t.Fatalf("theta = %.3f, want 0.06..0.11", theta)
	}

	// Mass balance closure: feed = concentrate + tails within ±0.5 %.
	errPct := (fi101 - wi301 - wi302) / fi101 * 100
	if errPct < -0.5 || errPct > 0.5 {
		t.Fatalf("mass balance error = %.2f%% (fi=%.2f conc=%.3f tails=%.2f)", errPct, fi101, wi301, wi302)
	}

	if li401 := eng(m, plantsim.RegLI401); li401 < 2.7 || li401 > 3.3 {
		t.Fatalf("li401 = %.2f, want 2.7..3.3", li401)
	}
	if di401 := eng(m, plantsim.RegDI401); di401 < 43 || di401 > 47 {
		t.Fatalf("di401 = %.2f, want 43..47", di401)
	}
	if ei401 := eng(m, plantsim.RegEI401); ei401 < 48 || ei401 > 62 {
		t.Fatalf("ei401 = %.2f, want 48..62", ei401)
	}
	if mi501 := eng(m, plantsim.RegMI501); mi501 < 9 || mi501 > 12 {
		t.Fatalf("mi501 = %.2f, want 9..12", mi501)
	}
	if level := eng(m, plantsim.RegLI301); level < 480 || level > 520 {
		t.Fatalf("li301 = %.0f, want 480..520", level)
	}
	if ph := eng(m, plantsim.RegAI301); ph < 10.0 || ph > 10.4 {
		t.Fatalf("ai301 = %.2f, want ~10.2", ph)
	}
	if mill := eng(m, plantsim.RegEI201); mill < 1150 || mill > 1350 {
		t.Fatalf("ei201 = %.0f, want ~1250", mill)
	}
}

// TestHarderOreLowersRecovery: ore hardness +20 % coarsens the grind and
// erodes recovery — the causal chain the demo §16.3 shows.
func TestHarderOreLowersRecovery(t *testing.T) {
	m := plantsim.New(42, time.Now())
	run(m, 100*time.Millisecond, 500)

	if m.Scenario(plantsim.ScenarioOreHard, 20) == "" {
		t.Fatal("scenario rejected")
	}
	run(m, 100*time.Millisecond, 2000) // 200 s: P80 (tau 30 s) settles

	p80 := eng(m, plantsim.RegXI201)
	if p80 < 165 || p80 > 195 {
		t.Fatalf("P80 = %.0f, want ~180 after +20%% hardness", p80)
	}
	alpha := eng(m, plantsim.RegAFI301)
	beta := eng(m, plantsim.RegAFC301)
	theta := eng(m, plantsim.RegAFT301)
	eps := beta * (alpha - theta) / (alpha * (beta - theta)) * 100
	if eps > 70 {
		t.Fatalf("recovery = %.2f%%, want well below nominal 90 after hard ore", eps)
	}
}

// TestOxideOreRecovery: switching to oxide ore cuts the achievable recovery.
func TestOxideOreRecovery(t *testing.T) {
	m := plantsim.New(42, time.Now())
	run(m, 100*time.Millisecond, 500)
	m.Scenario(plantsim.ScenarioOreType, 2)
	run(m, 100*time.Millisecond, 1500)

	alpha := eng(m, plantsim.RegAFI301)
	beta := eng(m, plantsim.RegAFC301)
	theta := eng(m, plantsim.RegAFT301)
	eps := beta * (alpha - theta) / (alpha * (beta - theta)) * 100
	if eps >= 80 {
		t.Fatalf("oxide ore recovery = %.2f%%, want < 80", eps)
	}
}

// TestUnderflowPumpFailure: scenario 6 stops the underflow pump; bed level
// and rake torque climb toward the alarm bands of TZ §6.
func TestUnderflowPumpFailure(t *testing.T) {
	m := plantsim.New(42, time.Now())
	run(m, 100*time.Millisecond, 500)

	m.Scenario(plantsim.ScenarioUFPumpFail, 240)
	run(m, 100*time.Millisecond, 2000) // 200 s of accumulation

	if li401 := eng(m, plantsim.RegLI401); li401 < 3.4 {
		t.Fatalf("li401 = %.2f, want > 3.4 after pump failure", li401)
	}
	if ei401 := eng(m, plantsim.RegEI401); ei401 < 70 {
		t.Fatalf("ei401 = %.2f, want > 70 (hi band) after pump failure", ei401)
	}
}

// TestScenarioResetRestoresNormal: scenario 0 drops every disturbance.
func TestScenarioResetRestoresNormal(t *testing.T) {
	m := plantsim.New(42, time.Now())
	m.Scenario(plantsim.ScenarioOreHard, 20)
	m.Scenario(plantsim.ScenarioP80Freeze, 1)
	m.Scenario(plantsim.ScenarioXRFFDrift, 1)
	m.Scenario(plantsim.ScenarioNormal, 0)
	st := m.State()
	if st.Hardness > 1.05 || st.OreFactor != 1.0 {
		t.Fatalf("state after reset = %+v", st)
	}
}
