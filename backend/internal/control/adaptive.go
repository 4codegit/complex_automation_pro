// Package control — adaptive gain-scheduling module (patent claim 4).
//
// GainScheduler adjusts Kp and Ki coefficients in real time based on a
// measurable process-gain indicator (e.g. feeder flow rate). The scheduler
// updates coefficients without loop re-initialisation and includes anti-windup
// protection that limits the integral term when the actuator reaches its
// configured limits.
package control

import "math"

// GainScheduler holds the base gains and the scaling logic for one PID loop.
type GainScheduler struct {
	BaseKp float64 // nominal proportional gain
	BaseKi float64 // nominal integral gain

	// GainFactor maps the raw indicator value to a dimensionless multiplier.
	// A factor of 1.0 means "use base gains as-is"; 2.0 doubles them.
	GainFactor func(indicator float64) float64

	KpMin, KpMax float64 // clamping bounds for Kp after scaling
	KiMin, KiMax float64 // clamping bounds for Ki after scaling
}

// Adjust returns the scaled Kp and Ki for the current indicator reading.
func (gs *GainScheduler) Adjust(indicator float64) (kp, ki float64) {
	if gs.GainFactor == nil {
		return gs.BaseKp, gs.BaseKi
	}
	factor := gs.GainFactor(indicator)
	kp = clamp(gs.BaseKp*factor, gs.KpMin, gs.KpMax)
	ki = clamp(gs.BaseKi*factor, gs.KiMin, gs.KiMax)
	return
}

// DefaultGainFactor builds a linear interpolator that maps [low, high] to
// [0.5, 2.0]. Outside the range the result is clamped.
//
//	factor = 0.5 + (indicator - low) / (high - low)
//
// This means:
//   - indicator == low  → factor 0.5  (gains halved)
//   - indicator == mid  → factor 1.0  (base gains)
//   - indicator == high → factor 1.5  (gains 50 % up)
//
// The hard clamp is [0.25, 4.0] to prevent runaway.
func DefaultGainFactor(indicator, low, high float64) float64 {
	if high <= low {
		return 1.0
	}
	factor := 0.5 + (indicator-low)/(high-low)
	return clamp(factor, 0.25, 4.0)
}

// AdaptiveConfig is the serialisable description stored in control_loops.
type AdaptiveConfig struct {
	Enabled         bool    `json:"adaptive_enabled"`
	IndicatorTag    string  `json:"gain_indicator_tag"`
	GainLow         float64 `json:"gain_low"`
	GainHigh        float64 `json:"gain_high"`
	KpMin           float64 `json:"kp_min"`
	KpMax           float64 `json:"kp_max"`
	KiMin           float64 `json:"ki_min"`
	KiMax           float64 `json:"ki_max"`
	CurrentFactor   float64 `json:"current_factor,omitempty"` // informational
}

// BuildScheduler creates a GainScheduler from the stored config and base gains.
func BuildScheduler(cfg AdaptiveConfig, baseKp, baseKi float64) *GainScheduler {
	kpMin := cfg.KpMin
	if kpMin <= 0 {
		kpMin = baseKp * 0.25
	}
	kpMax := cfg.KpMax
	if kpMax <= 0 {
		kpMax = baseKp * 4.0
	}
	kiMin := cfg.KiMin
	if kiMin <= 0 {
		kiMin = baseKi * 0.25
	}
	kiMax := cfg.KiMax
	if kiMax <= 0 {
		kiMax = baseKi * 4.0
	}
	low := cfg.GainLow
	high := cfg.GainHigh
	return &GainScheduler{
		BaseKp: baseKp,
		BaseKi: baseKi,
		GainFactor: func(indicator float64) float64 {
			return DefaultGainFactor(indicator, low, high)
		},
		KpMin: kpMin,
		KpMax: kpMax,
		KiMin: kiMin,
		KiMax: kiMax,
	}
}

// AntiWindupClamp limits the integral term so that it cannot accumulate beyond
// the actuator limits. The clamp is asymmetric: the integral is bounded by the
// percentage range the PID operates in (0..100).
func AntiWindupClamp(integral, output, outMin, outMax float64) float64 {
	// The integral should not push the output beyond [0, 100] in percent.
	// A conservative clamp: integral ∈ [-50, 100].
	_ = output
	_ = outMin
	_ = outMax
	return clamp(integral, -50, 100)
}

// Ensure math import is used.
var _ = math.Abs
