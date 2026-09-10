package control

import (
	"math"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		PVTag: "pv", OutTag: "out",
		Kp: 2, Ki: 0.4, Kd: 0,
		Deadband: 0.1, Slew: 5,
		Interval: time.Second,
	}
}

func TestParseLoopSpec(t *testing.T) {
	cfg, err := ParseLoopSpec("pv=plant-a.flotation.ph_level,out=plant-a.flotation.doser_speed,kp=2,ki=0.4,kd=0,deadband=0.1,slew=5,interval=500ms")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.PVTag != "plant-a.flotation.ph_level" || cfg.OutTag != "plant-a.flotation.doser_speed" {
		t.Errorf("tags: %+v", cfg)
	}
	if cfg.Kp != 2 || cfg.Ki != 0.4 || cfg.Kd != 0 || cfg.Deadband != 0.1 || cfg.Slew != 5 {
		t.Errorf("gains: %+v", cfg)
	}
	if cfg.Interval != 500*time.Millisecond {
		t.Errorf("interval = %s", cfg.Interval)
	}
	if _, err := ParseLoopSpec("pv=x"); err == nil {
		t.Error("missing out= must fail")
	}
	if _, err := ParseLoopSpec("pv=x,out=y,foo=1"); err == nil {
		t.Error("unknown key must fail")
	}
}

// TestStepRaisesOutputWhenPVBelowSetpoint: pH ниже уставки (кислота) — выход
// (дозатор) должен расти от нуля.
func TestStepRaisesOutputWhenPVBelowSetpoint(t *testing.T) {
	cfg := testConfig()
	st := State{}
	for i := 0; i < 10; i++ {
		st = Step(cfg, 9.0, 7.0, st, 1)
	}
	if st.Output <= 0 || st.Output > 100 {
		t.Fatalf("output = %v, want (0, 100]", st.Output)
	}
	if st.Status != StatusAuto {
		t.Errorf("status = %s, want auto", st.Status)
	}
}

// TestStepLowersOutputWhenPVAboveSetpoint: pH выше уставки — выход к нулю.
func TestStepLowersOutputWhenPVAboveSetpoint(t *testing.T) {
	cfg := testConfig()
	cfg.Slew = 0 // isolate PID semantics from the slew limiter
	st := State{Output: 80, Status: StatusAuto, HasPrev: true}
	for i := 0; i < 10; i++ {
		st = Step(cfg, 9.0, 10.5, st, 1)
	}
	if st.Output > 20 {
		t.Errorf("output = %v, want near zero for over-target PV", st.Output)
	}
}

func TestStepDeadbandHoldsOutput(t *testing.T) {
	cfg := testConfig()
	prev := State{Output: 42, Status: StatusAuto, Integral: 10, HasPrev: true}
	next := Step(cfg, 9.0, 8.95, prev, 1) // |e| = 0.05 < deadband 0.1
	if next.Output != 42 {
		t.Errorf("deadband must hold output: got %v, want 42", next.Output)
	}
	if next.Integral != 10 {
		t.Errorf("deadband must hold integral: got %v", next.Integral)
	}
}

func TestStepClampsOutputToPercent(t *testing.T) {
	cfg := testConfig()
	cfg.Kp = 50
	st := State{}
	for i := 0; i < 5; i++ {
		st = Step(cfg, 9.0, 3.0, st, 1)
	}
	if st.Output != 100 {
		t.Errorf("output = %v, want clamped 100", st.Output)
	}
}

func TestSlewLimitsActuatorMovement(t *testing.T) {
	cfg := testConfig() // slew 5%/tick
	prev := State{Output: 0, Status: StatusAuto, HasPrev: true}
	next := Step(cfg, 9.0, 3.0, prev, 1) // huge error wants ~100
	if math.Abs(next.Output-5) > 1e-9 {
		t.Errorf("slew limit: output = %v, want 5", next.Output)
	}
}

func TestWatchdogForcesSafeState(t *testing.T) {
	prev := State{Output: 77, Status: StatusAuto, Integral: 12, HasPrev: true}
	next := Watchdog(prev)
	if next.Output != 0 || next.Status != StatusWatchdog {
		t.Errorf("watchdog = %+v, want output 0 / paused_watchdog", next)
	}
}
