package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"cap/internal/control"
	"cap/internal/store"
)

// Supervisory control (ADR-003). The server runs the PID loop over stored
// telemetry and publishes the computed output; the edge gateway is the only
// component that touches the actuator (one Modbus write register). Every
// safety property is opt-in and explicit:
//
//	CONTROL_ENABLED=false (default) — no loop, no writes, endpoints report off
//	setpoints are clamped to the active profile corridor before storage
//	PV watchdog: stale PV pauses the loop and forces the output to 0
//	every setpoint change is audited (who, when, old -> new)

// StartControlLoop runs the PID tick loop until ctx is cancelled. It is
// started by the service composition only when CONTROL_ENABLED=true and the
// process owns the live section.
func (s *Server) StartControlLoop(ctx context.Context) {
	cfg, err := control.ParseLoopSpec(s.cfg.ControlLoop)
	if err != nil {
		log.Printf("[control] disabled: %v", err)
		return
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = time.Second
	}
	log.Printf("[control] loop armed: pv=%s out=%s kp=%g ki=%g kd=%g interval=%s",
		cfg.PVTag, cfg.OutTag, cfg.Kp, cfg.Ki, cfg.Kd, interval)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.controlTick(ctx, cfg, interval)
			}
		}
	}()
}

// controlTick executes one PID step: read SP and PV, run the watchdog, write
// the computed output into control_state for the gateway bridge.
func (s *Server) controlTick(ctx context.Context, cfg control.Config, interval time.Duration) {
	sp := 0.0
	if spRow, err := store.GetSetpoint(ctx, s.db, cfg.PVTag); err == nil {
		sp = spRow.Value
	} else {
		// No operator setpoint yet: hold at the profile corridor centre.
		min, max, ok := s.profileBand(cfg.PVTag)
		if !ok {
			return
		}
		sp = (min + max) / 2
	}

	var prev control.State
	if st, err := store.GetLoopState(ctx, s.db, cfg.OutTag); err == nil {
		prev = control.State{Integral: 0, Output: st.Output, Status: st.Status}
	}

	pv, age, hasPV := s.latestPV(cfg.PVTag)
	var next control.State
	if !hasPV || age > s.cfg.ControlStale {
		next = control.Watchdog(prev)
	} else {
		next = control.Step(cfg, sp, pv, prev, interval.Seconds())
	}

	if err := store.SaveLoopState(ctx, s.db, cfg.OutTag, next.Output, next.Status, next.Integral, next.PrevErr); err != nil {
		log.Printf("[control] save state: %v", err)
	}
}

func (s *Server) latestPV(tagID string) (float64, time.Duration, bool) {
	reading, err := store.LatestGoodNumericReading(context.Background(), s.db, tagID)
	if err != nil || reading == nil {
		return 0, 0, false
	}
	v, ok := reading.Value().(float64)
	if !ok {
		return 0, 0, false
	}
	age := s.now().UTC().Sub(reading.ObservedAt)
	if age < 0 {
		age = 0
	}
	return v, age, true
}

// profileBand returns the active profile's min/max corridor for a tag.
func (s *Server) profileBand(tagID string) (float64, float64, bool) {
	active, err := store.GetActiveProfile(context.Background(), s.db)
	if err != nil || active == nil {
		return 0, 0, false
	}
	var params struct {
		Thresholds map[string]struct {
			Min *float64 `json:"min"`
			Max *float64 `json:"max"`
		} `json:"thresholds"`
	}
	if json.Unmarshal([]byte(active.Params), &params) != nil {
		return 0, 0, false
	}
	metric := tagID[strings.LastIndex(tagID, ".")+1:]
	band, ok := params.Thresholds[metric]
	if !ok || band.Min == nil || band.Max == nil {
		return 0, 0, false
	}
	return *band.Min, *band.Max, true
}

// ControlStatus handles GET /api/v1/control/status.
func (s *Server) ControlStatus(w http.ResponseWriter, r *http.Request) {
	cfg, parseErr := control.ParseLoopSpec(s.cfg.ControlLoop)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	resp := map[string]any{
		"enabled":   s.cfg.ControlEnabled && parseErr == nil,
		"stale_sec": s.cfg.ControlStale.Seconds(),
	}
	if parseErr != nil {
		resp["error"] = parseErr.Error()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp["loop"] = map[string]any{
		"pv_tag": cfg.PVTag, "out_tag": cfg.OutTag, "unit": "%",
		"kp": cfg.Kp, "ki": cfg.Ki, "kd": cfg.Kd,
		"deadband": cfg.Deadband, "slew": cfg.Slew, "interval_s": cfg.Interval.Seconds(),
	}
	if min, max, ok := s.profileBand(cfg.PVTag); ok {
		resp["sp_min"], resp["sp_max"] = min, max
	}
	if sp, err := store.GetSetpoint(ctx, s.db, cfg.PVTag); err == nil {
		resp["setpoint"] = map[string]any{
			"value": sp.Value, "updated_by": sp.UpdatedBy, "updated_at": sp.UpdatedAt,
		}
	}
	if pv, age, ok := s.latestPV(cfg.PVTag); ok {
		resp["pv"] = map[string]any{"value": pv, "age_seconds": age.Seconds(), "stale": age > s.cfg.ControlStale}
	}
	if st, err := store.GetLoopState(ctx, s.db, cfg.OutTag); err == nil {
		resp["output"] = st.Output
		resp["status"] = st.Status
		resp["updated_at"] = st.UpdatedAt
	} else {
		resp["output"] = 0.0
		resp["status"] = control.StatusOff
	}
	writeJSON(w, http.StatusOK, resp)
}

// ControlSetSetpoint handles PUT /api/v1/control/setpoints.
func (s *Server) ControlSetSetpoint(w http.ResponseWriter, r *http.Request) {
	cfg, parseErr := control.ParseLoopSpec(s.cfg.ControlLoop)
	if parseErr != nil {
		writeProblem(w, http.StatusServiceUnavailable, "control_unavailable", parseErr.Error())
		return
	}
	var req struct {
		Value     float64 `json:"value"`
		UpdatedBy string  `json:"updated_by"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	actor := actorFromRequest(r)
	subject := req.UpdatedBy
	if subject == "" {
		subject = actor.Subject
	}

	min, max, ok := s.profileBand(cfg.PVTag)
	if !ok {
		writeProblem(w, http.StatusServiceUnavailable, "no_corridor",
			"активный профиль не задаёт коридор для "+cfg.PVTag)
		return
	}
	original := req.Value
	clamped := control.Clamp(req.Value, min, max)
	if err := store.SetSetpoint(r.Context(), s.db, cfg.PVTag, clamped, subject); err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	detail := fmt.Sprintf("sp=%.4g", original)
	if clamped != original {
		detail += fmt.Sprintf(" (clamped to corridor %.4g..%.4g)", min, max)
	}
	s.audit(r, "control.setpoint", "control_loop", cfg.PVTag, detail)
	writeJSON(w, http.StatusOK, map[string]any{
		"tag_id": cfg.PVTag, "value": clamped, "clamped": clamped != original, "updated_by": subject,
	})
}

// ControlOutput handles GET /api/v1/control/output?tag_id=... — the machine
// endpoint the edge gateway polls before every actuator write.
func (s *Server) ControlOutput(w http.ResponseWriter, r *http.Request) {
	tagID := queryStr(r, "tag_id")
	if tagID == "" {
		writeProblem(w, http.StatusBadRequest, "missing_param", "tag_id is required")
		return
	}
	st, err := store.GetLoopState(r.Context(), s.db, tagID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"tag_id": tagID, "output": 0.0, "status": control.StatusOff})
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tag_id": st.TagID, "output": st.Output, "status": st.Status, "updated_at": st.UpdatedAt,
	})
}
