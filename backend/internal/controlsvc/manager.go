// Package controlsvc runs the supervisory PID loops server-side (TZ §9).
// Each tick it evaluates every auto-mode loop against the latest good PV,
// stores the mapped output and broadcasts loop_state events to dashboards.
// The edge bridge (internal/gateway) translates auto outputs into FC6 writes.
package controlsvc

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"cap/internal/alarms"
	"cap/internal/control"
	"cap/internal/hub"
	"cap/internal/store"
)

// Manager owns the loop tick.
type Manager struct {
	db     *sql.DB
	hub    *hub.Hub
	alarms *alarms.Engine
	stale  time.Duration // PV watchdog age
	now    func() time.Time
}

// New creates the manager. Run drives the ticks.
func New(db *sql.DB, h *hub.Hub, eng *alarms.Engine, stale time.Duration) *Manager {
	return &Manager{db: db, hub: h, alarms: eng, stale: stale, now: time.Now}
}

// SetNow overrides the clock (tests).
func (m *Manager) SetNow(f func() time.Time) { m.now = f }

func (m *Manager) broadcast(ev map[string]any) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	m.hub.Broadcast(b)
}

// Tick evaluates all loops once. Called from Run on a ticker; exported for
// tests.
func (m *Manager) Tick(ctx context.Context) {
	loops, err := store.ListLoops(ctx, m.db)
	if err != nil {
		log.Printf("[control] list loops: %v", err)
		return
	}
	now := m.now()
	for i := range loops {
		m.tickLoop(ctx, &loops[i], now)
	}
}

func (m *Manager) tickLoop(ctx context.Context, l *store.ControlLoop, now time.Time) {
	// Temperature loops (tic201): the high-temperature oil interlock must act
	// regardless of the operator mode, so they bypass the manual early-return.
	if l.LoopType == store.LoopTypeTemp {
		m.tickTempLoop(ctx, l, now)
		return
	}

	if l.Mode == store.LoopModeManual {
		// Manual: output stays where the operator (or the watchdog freeze)
		// put it; only the live state is republished.
		m.publish(*l)
		return
	}

	r, err := store.LatestGoodNumericReading(ctx, m.db, l.PVTag)
	if err != nil || m.now().Sub(r.ObservedAt) > m.stale {
		m.watchdog(ctx, l, now)
		return
	}

	// Fresh PV: leave the watchdog state behind.
	if l.State == store.LoopStateWatchdog {
		l.State = store.LoopStateOK
		if m.alarms != nil {
			m.alarms.ClearOperational(ctx, "loop:"+l.ID, now)
		}
	}

	// pH regulation (patent claim 5): dedicated logic with dual dead-bands
	// and self-tuning based on temperature and flow.
	if l.LoopType == "ph" {
		m.tickPHLoop(ctx, l, r, now)
		return
	}

	m.tickStandard(ctx, l, r, now)
}

// watchdog freezes the loop on stale PV (TZ §9): manual mode, output held,
// operational alarm raised.
func (m *Manager) watchdog(ctx context.Context, l *store.ControlLoop, now time.Time) {
	if l.State == store.LoopStateWatchdog {
		return
	}
	_ = store.SaveLoopRuntime(ctx, m.db, l.ID, l.Output, store.LoopModeManual, store.LoopStateWatchdog, l.Integral, deref(l.PrevError))
	if m.alarms != nil {
		m.alarms.RaiseOperational(ctx, "loop:"+l.ID, "control_fault", "high",
			"Петля "+l.ID+": потеря актуального PV, переход в ручной режим", now)
	}
	if updated, _ := store.GetLoop(ctx, m.db, l.ID); updated != nil {
		m.publish(*updated)
	}
}

// tickTempLoop drives the equipment-temperature loop (oil station). The
// interlock (PV ≥ TempInterlock) forces the oil valve to 100 % in auto mode —
// even from manual — because bearing protection outranks operator intent.
// Release hysteresis: 5 °C below the trip point.
func (m *Manager) tickTempLoop(ctx context.Context, l *store.ControlLoop, now time.Time) {
	const releaseHyst = 5.0

	r, err := store.LatestGoodNumericReading(ctx, m.db, l.PVTag)
	if err != nil || m.now().Sub(r.ObservedAt) > m.stale {
		if l.State != store.LoopStateInterlock {
			m.watchdog(ctx, l, now)
		}
		return
	}
	pv := *r.ValueNumber

	interlocked := l.State == store.LoopStateInterlock
	if !interlocked && l.TempInterlock > 0 && pv >= l.TempInterlock {
		interlocked = true
		if m.alarms != nil {
			m.alarms.RaiseOperational(ctx, "loop:"+l.ID, "temp_interlock", "critical",
				"Перегрев "+l.ID+": температура "+formatC(pv)+" ≥ "+formatC(l.TempInterlock)+
					" — аварийная подача масла (клапан 100 %)", now)
		}
	}

	if interlocked {
		if pv < l.TempInterlock-releaseHyst {
			// Cooled down: release, hand the loop back to the operator.
			if m.alarms != nil {
				m.alarms.ClearOperational(ctx, "loop:"+l.ID, now)
			}
			_ = store.SaveLoopRuntime(ctx, m.db, l.ID, l.Output, store.LoopModeManual, store.LoopStateOK, l.Integral, deref(l.PrevError))
			if updated, _ := store.GetLoop(ctx, m.db, l.ID); updated != nil {
				m.publish(*updated)
			}
			return
		}
		_ = store.SaveLoopRuntime(ctx, m.db, l.ID, l.OutMax, store.LoopModeAuto, store.LoopStateInterlock, l.Integral, deref(l.PrevError))
		if updated, _ := store.GetLoop(ctx, m.db, l.ID); updated != nil {
			m.publishWithPv(*updated, r.ValueNumber)
		}
		return
	}

	// No interlock: a manual loop belongs to the operator, an auto loop runs
	// the ordinary PID (which also brings it out of the watchdog state).
	if l.Mode == store.LoopModeManual {
		if l.State != store.LoopStateOK {
			l.State = store.LoopStateOK
			_ = store.SaveLoopRuntime(ctx, m.db, l.ID, l.Output, l.Mode, l.State, l.Integral, deref(l.PrevError))
			if m.alarms != nil {
				m.alarms.ClearOperational(ctx, "loop:"+l.ID, now)
			}
		}
		m.publish(*l)
		return
	}
	if l.State == store.LoopStateWatchdog {
		l.State = store.LoopStateOK
		if m.alarms != nil {
			m.alarms.ClearOperational(ctx, "loop:"+l.ID, now)
		}
	}
	m.tickStandard(ctx, l, r, now)
}

// tickStandard is the plain PID evaluation shared by standard and temp loops.
func (m *Manager) tickStandard(ctx context.Context, l *store.ControlLoop, r *store.Reading, now time.Time) {
	cfg := control.Config{
		PVTag: l.PVTag, OutTag: l.MVTag,
		Kp: l.Kp, Ki: l.Ki, Kd: l.Kd,
		Deadband: l.Deadband, Slew: l.Slew,
		Interval: time.Second,
	}

	// Adaptive gain scheduling (patent claim 4): read the indicator tag,
	// compute the gain factor, and scale Kp/Ki without loop re-init.
	var currentFactor float64
	if l.AdaptiveEnabled && l.GainIndicatorTag != "" {
		if indicator, err := store.LatestGoodNumericReading(ctx, m.db, l.GainIndicatorTag); err == nil && indicator.ValueNumber != nil {
			gs := control.BuildScheduler(control.AdaptiveConfig{
				Enabled:   true,
				GainLow:   l.GainLow,
				GainHigh:  l.GainHigh,
			}, l.Kp, l.Ki)
			cfg.Kp, cfg.Ki = gs.Adjust(*indicator.ValueNumber)
			currentFactor = control.DefaultGainFactor(*indicator.ValueNumber, l.GainLow, l.GainHigh)
		}
	}

	prev := control.State{Integral: l.Integral, Output: percentOf(*l, l.Output), Status: control.StatusAuto, HasPrev: l.PrevError != nil}
	if l.PrevError != nil {
		prev.PrevErr = *l.PrevError
	}
	next := control.Step(cfg, l.SP, *r.ValueNumber, prev, 1.0)
	output := engineeringOf(*l, next.Output)

	if err := store.SaveLoopRuntime(ctx, m.db, l.ID, output, store.LoopModeAuto, store.LoopStateOK, next.Integral, next.PrevErr); err != nil {
		log.Printf("[control] save %s: %v", l.ID, err)
		return
	}
	l.Output = output
	l.State = store.LoopStateOK
	l.CurrentFactor = currentFactor
	m.publishWithPv(*l, r.ValueNumber)
}

// formatC renders a temperature for alarm messages.
func formatC(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64) + " °C"
}

// tickPHLoop handles the pH regulation loop (patent claim 5).
func (m *Manager) tickPHLoop(ctx context.Context, l *store.ControlLoop, r *store.Reading, now time.Time) {
	cfg := control.Config{
		PVTag: l.PVTag, OutTag: l.MVTag,
		Kp: l.Kp, Ki: l.Ki, Kd: l.Kd,
		Deadband: l.Deadband, Slew: l.Slew,
		Interval: time.Second,
	}

	// Self-tuning: adjust Kp/Ki based on temperature and flow
	if l.SelfTuningEnabled {
		var temp, flow float64
		if l.TemperatureTag != "" {
			if tr, err := store.LatestGoodNumericReading(ctx, m.db, l.TemperatureTag); err == nil && tr.ValueNumber != nil {
				temp = *tr.ValueNumber
			}
		}
		if l.FlowTag != "" {
			if fr, err := store.LatestGoodNumericReading(ctx, m.db, l.FlowTag); err == nil && fr.ValueNumber != nil {
				flow = *fr.ValueNumber
			}
		}
		phCfg := control.PHConfig{
			SelfTuningEnabled: l.SelfTuningEnabled,
			TemperatureTag:    l.TemperatureTag,
			FlowTag:           l.FlowTag,
			KpTempFactor:      l.KpTempFactor,
			KiFlowFactor:      l.KiFlowFactor,
		}
		cfg = control.PHSelfTune(cfg, temp, flow, phCfg)
	}

	prev := control.State{Integral: l.Integral, Output: percentOf(*l, l.Output), Status: control.StatusAuto, HasPrev: l.PrevError != nil}
	if l.PrevError != nil {
		prev.PrevErr = *l.PrevError
	}

	bands := control.PHDeadBands{
		Warning:  l.PHDeadbandWarning,
		Critical: l.PHDeadbandCritical,
	}
	next, phState := control.PHStep(cfg, l.SP, *r.ValueNumber, prev, 1.0, bands)

	// Handle pH alarm states
	switch phState {
	case control.PHStateCritical:
		// Force manual, output frozen, raise critical alarm
		_ = store.SaveLoopRuntime(ctx, m.db, l.ID, l.Output, store.LoopModeManual, store.LoopStateWatchdog, next.Integral, next.PrevErr)
		if m.alarms != nil {
			m.alarms.RaiseOperational(ctx, "loop:"+l.ID, "ph_critical", "critical",
				"pH критическое отклонение (>0.5), принудительный ручной режим", now)
		}
		updated, _ := store.GetLoop(ctx, m.db, l.ID)
		if updated != nil {
			m.publishWithPv(*updated, r.ValueNumber)
		}
		return

	case control.PHStateWarning:
		// Raise warning alarm, continue auto
		if m.alarms != nil {
			m.alarms.RaiseOperational(ctx, "loop:"+l.ID, "ph_warning", "high",
				"pH отклонение >0.2 от уставки", now)
		}
		// Fall through to normal save

	case control.PHStateOK:
		// Clear previous pH alarms
		if m.alarms != nil {
			m.alarms.ClearOperational(ctx, "loop:"+l.ID, now)
		}
	}

	output := engineeringOf(*l, next.Output)

	if err := store.SaveLoopRuntime(ctx, m.db, l.ID, output, store.LoopModeAuto, store.LoopStateOK, next.Integral, next.PrevErr); err != nil {
		log.Printf("[control] save %s: %v", l.ID, err)
		return
	}
	l.Output = output
	l.State = store.LoopStateOK
	m.publishWithPv(*l, r.ValueNumber)
}

// publish broadcasts one loop_state event (TZ §13).
func (m *Manager) publish(l store.ControlLoop) {
	m.publishWithPv(l, nil)
}

func (m *Manager) publishWithPv(l store.ControlLoop, pv *float64) {
	ev := map[string]any{
		"type": "loop_state", "timestamp": m.now().UTC().Format(time.RFC3339Nano),
		"loop_id": l.ID, "label": l.Label,
		"pv_tag": l.PVTag, "mv_tag": l.MVTag,
		"mode": l.Mode, "state": l.State,
		"sp": l.SP, "out": l.Output,
		"adaptive_enabled": l.AdaptiveEnabled,
		"loop_type": l.LoopType,
	}
	if pv != nil {
		ev["pv"] = *pv
	}
	if l.AdaptiveEnabled {
		ev["gain_factor"] = l.CurrentFactor
		ev["gain_indicator_tag"] = l.GainIndicatorTag
	}
	if l.LoopType == "ph" {
		ev["ph_deadband_warning"] = l.PHDeadbandWarning
		ev["ph_deadband_critical"] = l.PHDeadbandCritical
		ev["self_tuning_enabled"] = l.SelfTuningEnabled
		ev["temperature_tag"] = l.TemperatureTag
		ev["flow_tag"] = l.FlowTag
	}
	m.broadcast(ev)
}

// percentOf maps an engineering output into the 0..100 percent range the PID
// core works in.
func percentOf(l store.ControlLoop, engineering float64) float64 {
	span := l.OutMax - l.OutMin
	if span <= 0 {
		return 0
	}
	return (engineering - l.OutMin) / span * 100
}

// engineeringOf maps the percent output back into MV engineering units.
func engineeringOf(l store.ControlLoop, pct float64) float64 {
	return l.OutMin + pct/100*(l.OutMax-l.OutMin)
}

func deref(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

// Run blocks until ctx is cancelled, ticking once per second.
func (m *Manager) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.Tick(ctx)
		}
	}
}
