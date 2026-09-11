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
	if l.Mode == store.LoopModeManual {
		// Manual: output stays where the operator (or the watchdog freeze)
		// put it; only the live state is republished.
		m.publish(*l)
		return
	}

	r, err := store.LatestGoodNumericReading(ctx, m.db, l.PVTag)
	if err != nil || m.now().Sub(r.ObservedAt) > m.stale {
		// TZ §9 watchdog: stale PV → manual, output frozen (not zeroed — the
		// actuator holds its last safe position), operational alarm raised.
		if l.State != store.LoopStateWatchdog {
			_ = store.SaveLoopRuntime(ctx, m.db, l.ID, l.Output, store.LoopModeManual, store.LoopStateWatchdog, l.Integral, deref(l.PrevError))
			if m.alarms != nil {
				m.alarms.RaiseOperational(ctx, "loop:"+l.ID, "control_fault", "high",
					"Петля "+l.ID+": потеря актуального PV, переход в ручной режим", now)
			}
			updated, _ := store.GetLoop(ctx, m.db, l.ID)
			if updated != nil {
				m.publish(*updated)
			}
		}
		return
	}

	// Fresh PV: leave the watchdog state behind.
	if l.State == store.LoopStateWatchdog {
		l.State = store.LoopStateOK
		if m.alarms != nil {
			m.alarms.ClearOperational(ctx, "loop:"+l.ID, now)
		}
	}

	cfg := control.Config{
		PVTag: l.PVTag, OutTag: l.MVTag,
		Kp: l.Kp, Ki: l.Ki, Kd: l.Kd,
		Deadband: l.Deadband, Slew: l.Slew,
		Interval: time.Second,
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
	}
	if pv != nil {
		ev["pv"] = *pv
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
