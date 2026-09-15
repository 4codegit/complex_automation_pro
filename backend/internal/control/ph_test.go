package control

import (
	"testing"
)

func testPHConfig() Config {
	return Config{
		PVTag: "ph", OutTag: "doser",
		Kp: -0.5, Ki: -0.15, Kd: 0,
		Deadband: 0.1, Slew: 2,
		Interval: 1.0,
	}
}

func testPHDeadBands() PHDeadBands {
	return PHDeadBands{
		Warning:  0.2,
		Critical: 0.5,
	}
}

func testPHConfigSelfTune() PHConfig {
	return PHConfig{
		SelfTuningEnabled: true,
		TemperatureTag:    "ti301",
		FlowTag:           "fi301",
		KpTempFactor:      0.02,
		KiFlowFactor:      0.01,
	}
}

func TestPHStepWithinWarningBand(t *testing.T) {
	cfg := testPHConfig()
	bands := testPHDeadBands()
	prev := State{Output: 50, Status: StatusAuto, Integral: 10, HasPrev: true}

	// pv=8.95, sp=9.0 → absErr=0.05 < 0.2 (warning) → hold
	next, state := PHStep(cfg, 9.0, 8.95, prev, 1.0, bands)

	if state != PHStateOK {
		t.Errorf("state = %v, want ok", state)
	}
	if next.Output != 50 {
		t.Errorf("output = %v, want 50 (held)", next.Output)
	}
	if next.Integral != 10 {
		t.Errorf("integral = %v, want 10 (held)", next.Integral)
	}
}

func TestPHStepWarningZone(t *testing.T) {
	cfg := testPHConfig()
	bands := testPHDeadBands()
	prev := State{Output: 50, Status: StatusAuto, Integral: 10, HasPrev: true}

	// pv=8.7, sp=9.0 → absErr=0.3 > 0.2 (warning) but < 0.5 (critical)
	next, state := PHStep(cfg, 9.0, 8.7, prev, 1.0, bands)

	if state != PHStateWarning {
		t.Errorf("state = %v, want warning", state)
	}
	// PID should have moved output
	if next.Output == 50 {
		t.Errorf("output should change in warning zone, got 50")
	}
}

func TestPHStepCriticalZone(t *testing.T) {
	cfg := testPHConfig()
	bands := testPHDeadBands()
	prev := State{Output: 50, Status: StatusAuto, Integral: 10, HasPrev: true}

	// pv=8.4, sp=9.0 → absErr=0.6 > 0.5 (critical)
	next, state := PHStep(cfg, 9.0, 8.4, prev, 1.0, bands)

	if state != PHStateCritical {
		t.Errorf("state = %v, want critical", state)
	}
	// Output should be frozen
	if next.Output != 50 {
		t.Errorf("output = %v, want 50 (frozen)", next.Output)
	}
	if next.Integral != 10 {
		t.Errorf("integral = %v, want 10 (frozen)", next.Integral)
	}
}

func TestPHStepCriticalZoneAbove(t *testing.T) {
	cfg := testPHConfig()
	bands := testPHDeadBands()
	prev := State{Output: 50, Status: StatusAuto, Integral: 10, HasPrev: true}

	// pv=9.6, sp=9.0 → absErr=0.6 > 0.5 (critical)
	next, state := PHStep(cfg, 9.0, 9.6, prev, 1.0, bands)

	if state != PHStateCritical {
		t.Errorf("state = %v, want critical", state)
	}
	if next.Output != 50 {
		t.Errorf("output = %v, want 50 (frozen)", next.Output)
	}
}

func TestPHSelfTuneDisabled(t *testing.T) {
	cfg := testPHConfig()
	phCfg := PHConfig{SelfTuningEnabled: false}

	// Should return unchanged config
	result := PHSelfTune(cfg, 30.0, 150.0, phCfg)
	if result.Kp != cfg.Kp || result.Ki != cfg.Ki {
		t.Errorf("disabled: Kp=%v Ki=%v, want %v %v", result.Kp, result.Ki, cfg.Kp, cfg.Ki)
	}
}

func TestPHSelfTuneTemperature(t *testing.T) {
	cfg := testPHConfig()
	phCfg := testPHConfigSelfTune()

	// temp=30°C (5 above 25) → Kp = -0.5 * (1 + 0.02*5) = -0.5 * 1.1 = -0.55
	result := PHSelfTune(cfg, 30.0, 100.0, phCfg)
	expectedKp := -0.5 * 1.1
	if result.Kp < expectedKp-0.001 || result.Kp > expectedKp+0.001 {
		t.Errorf("temp 30: Kp = %v, want %v", result.Kp, expectedKp)
	}
	// Ki unchanged (flow at reference)
	if result.Ki != cfg.Ki {
		t.Errorf("temp 30: Ki = %v, want %v", result.Ki, cfg.Ki)
	}
}

func TestPHSelfTuneFlow(t *testing.T) {
	cfg := testPHConfig()
	phCfg := testPHConfigSelfTune()

	// flow=200 (100 above reference) → Ki = -0.15 * (1 + 0.01*1) = -0.15 * 1.01 = -0.1515
	result := PHSelfTune(cfg, 25.0, 200.0, phCfg)
	expectedKi := -0.15 * 1.01
	if result.Ki < expectedKi-0.001 || result.Ki > expectedKi+0.001 {
		t.Errorf("flow 200: Ki = %v, want %v", result.Ki, expectedKi)
	}
	// Kp unchanged (temp at reference)
	if result.Kp != cfg.Kp {
		t.Errorf("flow 200: Kp = %v, want %v", result.Kp, cfg.Kp)
	}
}

func TestPHSelfTuneBoth(t *testing.T) {
	cfg := testPHConfig()
	phCfg := testPHConfigSelfTune()

	// temp=30, flow=200
	result := PHSelfTune(cfg, 30.0, 200.0, phCfg)
	expectedKp := -0.5 * 1.1
	expectedKi := -0.15 * 1.01
	if result.Kp < expectedKp-0.001 || result.Kp > expectedKp+0.001 {
		t.Errorf("both: Kp = %v, want %v", result.Kp, expectedKp)
	}
	if result.Ki < expectedKi-0.001 || result.Ki > expectedKi+0.001 {
		t.Errorf("both: Ki = %v, want %v", result.Ki, expectedKi)
	}
}

func TestPHSelfTuneClampMin(t *testing.T) {
	cfg := testPHConfig()
	phCfg := PHConfig{
		SelfTuningEnabled: true,
		KpTempFactor:      -1.0, // extreme negative
	}

	// temp=0°C → Kp = -0.5 * (1 - 1.0*25) = -0.5 * -24 = 12 → clamped to 5×base = -2.5
	result := PHSelfTune(cfg, 0.0, 100.0, phCfg)
	if result.Kp > -0.05 || result.Kp < -2.51 {
		t.Errorf("clamp min: Kp = %v, want in [-2.5, -0.05]", result.Kp)
	}
}

func TestPHSelfTuneClampMax(t *testing.T) {
	cfg := testPHConfig()
	phCfg := PHConfig{
		SelfTuningEnabled: true,
		TemperatureTag:    "ti301", // required for temp compensation
		KpTempFactor:      1.0,     // extreme positive
	}

	// temp=50°C → Kp = -0.5 * (1 + 1.0*25) = -0.5 * 26 = -13 → clamped to 5×|base| = -2.5
	result := PHSelfTune(cfg, 50.0, 100.0, phCfg)
	if result.Kp < -2.51 || result.Kp > -2.49 {
		t.Errorf("clamp max: Kp = %v, want ≈ -2.5", result.Kp)
	}
}