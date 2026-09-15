// Package control — automatic pH regulation (patent claim 5).
//
// A dedicated PID loop controls the pH of the flotation pulp by manipulating
// the dosage of acid or alkali reagents. The loop operates fully automatically
// and includes:
//   (a) a dead-band of ±0.2 pH units around the setpoint that generates a
//       warning alarm, and a wider dead-band of ±0.5 pH units that generates
//       a critical alarm and forces the loop into manual mode with output frozen;
//   (b) self-tuning of Kp/Ki based on pulp temperature and flow rate;
//   (c) bumpless transfer from manual to automatic mode by initializing the
//       integral term from the current actuator position (already implemented
//       in store.bumplessIntegralSeed).
package control

import "math"

// PHDeadBands defines the warning and critical zones around the pH setpoint.
type PHDeadBands struct {
	Warning  float64 // ±0.2 pH units → warning alarm
	Critical float64 // ±0.5 pH units → critical alarm + force manual
}

// PHConfig holds the self-tuning parameters for the pH loop.
type PHConfig struct {
	SelfTuningEnabled bool
	TemperatureTag    string  // tag for pulp temperature
	FlowTag           string  // tag for pulp flow rate
	KpTempFactor      float64 // Kp adjustment per °C above 25°C
	KiFlowFactor      float64 // Ki adjustment per 100 units above 100 flow
}

// PHState is the alarm state returned by PHStep.
type PHState string

const (
	PHStateOK       PHState = "ok"
	PHStateWarning  PHState = "warning"
	PHStateCritical PHState = "critical"
)

// PHStep advances the pH PID with dual dead-bands.
//
// Returns the new PID state and the alarm state (ok/warning/critical).
// On critical: output is frozen (returns prev state), critical alarm raised.
// On warning: normal PID step, warning alarm raised.
// On ok: normal PID step, alarms cleared.
func PHStep(cfg Config, sp, pv float64, prev State, dt float64, bands PHDeadBands) (State, PHState) {
	err := sp - pv
	absErr := math.Abs(err)

	switch {
	case absErr > bands.Critical:
		// Critical deviation: freeze output, force manual
		return State{
			Integral: prev.Integral,
			PrevErr:  err,
			HasPrev:  true,
			Output:   prev.Output, // frozen
			Status:   StatusAuto,  // manager will override to manual
		}, PHStateCritical

	case absErr > bands.Warning:
		// Warning zone: normal PID but raise warning
		next := Step(cfg, sp, pv, prev, dt)
		return next, PHStateWarning

	default:
		// Within warning dead-band: hold output steady
		return State{
			Integral: prev.Integral,
			PrevErr:  err,
			HasPrev:  true,
			Output:   prev.Output,
			Status:   StatusAuto,
		}, PHStateOK
	}
}

// PHSelfTune adjusts Kp and Ki based on temperature and flow rate.
//
// Kp increases with temperature (higher temp → faster reaction → more gain needed).
// Ki increases with flow rate (higher flow → more reagent needed → more integral).
//
// Base values are the nominal Kp/Ki from the loop config.
// The factors are small (e.g., 0.02 per °C, 0.01 per 100 flow units).
// Result is clamped to [0.1×|base|, 5.0×|base|] with correct sign to prevent runaway.
func PHSelfTune(cfg Config, temp, flow float64, phCfg PHConfig) Config {
	if !phCfg.SelfTuningEnabled {
		return cfg
	}

	kp := cfg.Kp
	ki := cfg.Ki

	// Temperature compensation: Kp scales with temperature
	if phCfg.KpTempFactor != 0 && phCfg.TemperatureTag != "" {
		// Reference temp = 25°C
		tempDelta := temp - 25.0
		kp = kp * (1.0 + phCfg.KpTempFactor*tempDelta)
	}

	// Flow compensation: Ki scales with flow rate
	if phCfg.KiFlowFactor != 0 && phCfg.FlowTag != "" {
		// Reference flow = 100 (arbitrary units)
		flowDelta := (flow - 100.0) / 100.0
		ki = ki * (1.0 + phCfg.KiFlowFactor*flowDelta)
	}

	// Clamp to reasonable bounds: [0.1×|base|, 5.0×|base|] preserving sign
	kp = clampSigned(kp, cfg.Kp)
	ki = clampSigned(ki, cfg.Ki)

	return Config{
		PVTag:    cfg.PVTag,
		OutTag:   cfg.OutTag,
		Unit:     cfg.Unit,
		Kp:       kp,
		Ki:       ki,
		Kd:       cfg.Kd,
		Deadband: cfg.Deadband,
		Slew:     cfg.Slew,
		Interval: cfg.Interval,
	}
}

// clampSigned clamps value to [0.1×|base|, 5.0×|base|] preserving the sign of base.
func clampSigned(v, base float64) float64 {
	if base == 0 {
		return v
	}
	sign := 1.0
	if base < 0 {
		sign = -1.0
	}
	absBase := math.Abs(base)
	lo := sign * absBase * 0.1
	hi := sign * absBase * 5.0
	// Ensure lo <= hi for clamp function
	if lo > hi {
		lo, hi = hi, lo
	}
	return clamp(v, lo, hi)
}

// PHConfigFromLoop extracts pH-specific config from a ControlLoop.
func PHConfigFromLoop(l storeControlLoop) PHConfig {
	return PHConfig{
		SelfTuningEnabled: l.GetSelfTuningEnabled(),
		TemperatureTag:    l.GetTemperatureTag(),
		FlowTag:           l.GetFlowTag(),
		KpTempFactor:      l.GetKpTempFactor(),
		KiFlowFactor:      l.GetKiFlowFactor(),
	}
}

// PHDeadBandsFromLoop extracts dead-band config from a ControlLoop.
func PHDeadBandsFromLoop(l storeControlLoop) PHDeadBands {
	return PHDeadBands{
		Warning:  l.GetPHDeadbandWarning(),
		Critical: l.GetPHDeadbandCritical(),
	}
}

// storeControlLoop is a minimal interface for the fields we need from store.ControlLoop.
// This avoids a circular import (control → store).
type storeControlLoop interface {
	GetSelfTuningEnabled() bool
	GetTemperatureTag() string
	GetFlowTag() string
	GetKpTempFactor() float64
	GetKiFlowFactor() float64
	GetPHDeadbandWarning() float64
	GetPHDeadbandCritical() float64
}