// Package control implements the supervisory PID loop (ADR-003). The loop
// runs server-side on stored telemetry; the edge gateway translates the
// computed output into an actuator write. Every safety property is explicit:
// output clamp, integral clamp, deadband, slew limit and a PV watchdog.
package control

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Config describes one control loop. It is parsed from CONTROL_LOOP:
//
//	CONTROL_LOOP=pv=plant-a.flotation.ph_level,out=plant-a.flotation.doser_speed,kp=2,ki=0.4,kd=0,deadband=0.1,slew=5
type Config struct {
	PVTag    string
	OutTag   string
	Unit     string // actuator unit, display only (%)
	Kp, Ki   float64
	Kd       float64
	Deadband float64
	Slew     float64 // max output change per tick, percent points
	Interval time.Duration
}

// ParseLoopSpec parses the CONTROL_LOOP key=value spec.
func ParseLoopSpec(spec string) (Config, error) {
	cfg := Config{Interval: time.Second}
	for _, part := range strings.Split(spec, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			return cfg, fmt.Errorf("CONTROL_LOOP: expected key=value, got %q", part)
		}
		key, val := strings.ToLower(strings.TrimSpace(kv[0])), strings.TrimSpace(kv[1])
		switch key {
		case "pv":
			cfg.PVTag = val
		case "out":
			cfg.OutTag = val
		case "kp", "ki", "kd", "deadband", "slew":
			v, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return cfg, fmt.Errorf("CONTROL_LOOP: %s=%q is not a number", key, val)
			}
			switch key {
			case "kp":
				cfg.Kp = v
			case "ki":
				cfg.Ki = v
			case "kd":
				cfg.Kd = v
			case "deadband":
				cfg.Deadband = v
			case "slew":
				cfg.Slew = v
			}
		case "interval":
			d, err := time.ParseDuration(val)
			if err != nil {
				return cfg, fmt.Errorf("CONTROL_LOOP: interval %q: %w", val, err)
			}
			cfg.Interval = d
		default:
			return cfg, fmt.Errorf("CONTROL_LOOP: unknown key %q", key)
		}
	}
	if cfg.PVTag == "" || cfg.OutTag == "" {
		return cfg, fmt.Errorf("CONTROL_LOOP: pv= and out= are required")
	}
	return cfg, nil
}

// Status values written to control_state (the gateway acts on "auto" only).
const (
	StatusOff      = "off"
	StatusAuto     = "auto"
	StatusWatchdog = "paused_watchdog" // stale PV: output forced to safe 0
)

// State carries the loop memory between ticks (persisted in control_state).
type State struct {
	Integral float64
	PrevErr  float64
	HasPrev  bool
	Output   float64
	Status   string
}

// Step advances the PID one tick. Output is clamped to [0, 100] percent,
// the integral is clamped against windup, a deadband holds the output near
// the setpoint and the slew limit bounds per-tick actuator movement.
func Step(cfg Config, sp, pv float64, prev State, dt float64) State {
	errTerm := sp - pv

	// Watchdog-free zone: within the deadband, hold the output steady.
	if math.Abs(errTerm) <= cfg.Deadband {
		return State{Integral: prev.Integral, PrevErr: errTerm, HasPrev: true, Output: prev.Output, Status: StatusAuto}
	}

	p := cfg.Kp * errTerm
	integral := clamp(prev.Integral+cfg.Ki*errTerm*dt, -50, 100)
	d := 0.0
	if cfg.Kd != 0 && prev.HasPrev {
		d = cfg.Kd * (errTerm - prev.PrevErr) / dt
	}

	out := clamp(p+integral+d, 0, 100)
	if cfg.Slew > 0 && prev.Status == StatusAuto {
		out = clamp(out, prev.Output-cfg.Slew, prev.Output+cfg.Slew)
	}
	return State{Integral: integral, PrevErr: errTerm, HasPrev: true, Output: out, Status: StatusAuto}
}

// Watchdog returns the safe state for a stale PV: output to zero, loop paused.
func Watchdog(prev State) State {
	return State{Integral: 0, PrevErr: prev.PrevErr, HasPrev: prev.HasPrev, Output: 0, Status: StatusWatchdog}
}

// clamp keeps v inside [lo, hi].
func clamp(v, lo, hi float64) float64 { return math.Min(math.Max(v, lo), hi) }

// Clamp is the exported bounds helper shared by control consumers.
func Clamp(v, lo, hi float64) float64 { return clamp(v, lo, hi) }
