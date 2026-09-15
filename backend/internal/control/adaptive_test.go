package control

import (
	"testing"
)

func TestDefaultGainFactor(t *testing.T) {
	tests := []struct {
		name      string
		indicator float64
		low, high float64
		wantMin   float64
		wantMax   float64
	}{
		{"at_low", 50, 50, 200, 0.49, 0.51},
		{"at_high", 200, 50, 200, 1.49, 1.51},
		{"at_mid", 125, 50, 200, 0.99, 1.01},
		{"below_low", 0, 50, 200, 0.24, 0.26},    // clamped to 0.25
		{"above_high", 400, 50, 200, 2.82, 2.84},  // 0.5+350/150=2.83
		{"extreme_high", 10000, 50, 200, 3.99, 4.01}, // clamped to 4.0
		{"zero_range", 100, 50, 50, 0.99, 1.01},   // high == low → 1.0
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := DefaultGainFactor(tt.indicator, tt.low, tt.high)
			if f < tt.wantMin || f > tt.wantMax {
				t.Errorf("DefaultGainFactor(%v, %v, %v) = %v, want [%v, %v]",
					tt.indicator, tt.low, tt.high, f, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestGainSchedulerAdjust(t *testing.T) {
	gs := &GainScheduler{
		BaseKp: 2.0,
		BaseKi: 0.4,
		GainFactor: func(indicator float64) float64 {
			return DefaultGainFactor(indicator, 50, 200)
		},
		KpMin: 0.5,
		KpMax: 8.0,
		KiMin: 0.1,
		KiMax: 2.0,
	}

	// At mid range → factor ≈ 1.0 → Kp ≈ 2, Ki ≈ 0.4
	kp, ki := gs.Adjust(125)
	if kp < 1.9 || kp > 2.1 {
		t.Errorf("mid: Kp = %v, want ≈ 2.0", kp)
	}
	if ki < 0.39 || ki > 0.41 {
		t.Errorf("mid: Ki = %v, want ≈ 0.4", ki)
	}

	// At high → factor ≈ 1.5 → Kp ≈ 3, Ki ≈ 0.6
	kp, ki = gs.Adjust(200)
	if kp < 2.9 || kp > 3.1 {
		t.Errorf("high: Kp = %v, want ≈ 3.0", kp)
	}
	if ki < 0.59 || ki > 0.61 {
		t.Errorf("high: Ki = %v, want ≈ 0.6", ki)
	}

	// At low → factor ≈ 0.5 → Kp ≈ 1, Ki ≈ 0.2
	kp, ki = gs.Adjust(50)
	if kp < 0.9 || kp > 1.1 {
		t.Errorf("low: Kp = %v, want ≈ 1.0", kp)
	}
	if ki < 0.19 || ki > 0.21 {
		t.Errorf("low: Ki = %v, want ≈ 0.2", ki)
	}
}

func TestGainSchedulerClamp(t *testing.T) {
	gs := &GainScheduler{
		BaseKp: 2.0,
		BaseKi: 0.4,
		GainFactor: func(indicator float64) float64 {
			return DefaultGainFactor(indicator, 50, 200)
		},
		KpMin: 1.0,
		KpMax: 5.0,
		KiMin: 0.2,
		KiMax: 1.0,
	}

	// Very low indicator → factor clamped to 0.25 → Kp = 2*0.25 = 0.5 → clamped to 1.0
	kp, _ := gs.Adjust(0)
	if kp != 1.0 {
		t.Errorf("clamp min: Kp = %v, want 1.0", kp)
	}

	// Very high indicator → factor clamped to 4.0 → Kp = 2*4 = 8 → clamped to 5.0
	kp, _ = gs.Adjust(9999)
	if kp != 5.0 {
		t.Errorf("clamp max: Kp = %v, want 5.0", kp)
	}
}

func TestGainSchedulerNilFactor(t *testing.T) {
	gs := &GainScheduler{BaseKp: 3.0, BaseKi: 0.6}
	kp, ki := gs.Adjust(100) // nil GainFactor → return base
	if kp != 3.0 || ki != 0.6 {
		t.Errorf("nil factor: got Kp=%v Ki=%v, want 3.0, 0.6", kp, ki)
	}
}

func TestBuildScheduler(t *testing.T) {
	cfg := AdaptiveConfig{
		Enabled:      true,
		IndicatorTag: "plant.crushing.fi101",
		GainLow:      50,
		GainHigh:     200,
	}
	gs := BuildScheduler(cfg, 2.0, 0.4)

	// KpMin should default to 2.0 * 0.25 = 0.5
	if gs.KpMin != 0.5 {
		t.Errorf("KpMin = %v, want 0.5", gs.KpMin)
	}
	// KpMax should default to 2.0 * 4.0 = 8.0
	if gs.KpMax != 8.0 {
		t.Errorf("KpMax = %v, want 8.0", gs.KpMax)
	}

	kp, ki := gs.Adjust(125) // mid
	if kp < 1.9 || kp > 2.1 {
		t.Errorf("BuildScheduler Adjust mid: Kp = %v, want ≈ 2.0", kp)
	}
	if ki < 0.39 || ki > 0.41 {
		t.Errorf("BuildScheduler Adjust mid: Ki = %v, want ≈ 0.4", ki)
	}
}

func TestAntiWindupClamp(t *testing.T) {
	// Should not panic and should clamp to [-50, 100]
	v := AntiWindupClamp(200, 90, 0, 100)
	if v != 100 {
		t.Errorf("AntiWindupClamp(200) = %v, want 100", v)
	}
	v = AntiWindupClamp(-100, 10, 0, 100)
	if v != -50 {
		t.Errorf("AntiWindupClamp(-100) = %v, want -50", v)
	}
	v = AntiWindupClamp(50, 50, 0, 100)
	if v != 50 {
		t.Errorf("AntiWindupClamp(50) = %v, want 50", v)
	}
}
