package api

import (
	"errors"
	"fmt"
	"math"
	"net/http"

	"cap/internal/store"
)

// ControlStatus returns the loop registry with the latest PV values attached.
func (s *Server) ControlStatus(w http.ResponseWriter, r *http.Request) {
	loops, err := store.ListLoops(r.Context(), s.db)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	out := make([]map[string]any, 0, len(loops))
	for _, l := range loops {
		item := map[string]any{
			"id": l.ID, "label": l.Label, "pv_tag": l.PVTag, "mv_tag": l.MVTag,
			"sp": l.SP, "sp_min": l.SPMin, "sp_max": l.SPMax,
			"out_min": l.OutMin, "out_max": l.OutMax,
			"kp": l.Kp, "ki": l.Ki, "kd": l.Kd,
			"mode": l.Mode, "state": l.State, "out": l.Output,
		}
		if pv, err := store.LatestGoodNumericReading(r.Context(), s.db, l.PVTag); err == nil && pv.ValueNumber != nil {
			item["pv"] = *pv.ValueNumber
			item["pv_at"] = pv.ObservedAt
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

type loopWriteRequest struct {
	Value   float64 `json:"value"`
	Confirm bool    `json:"confirm"`
}

var errConfirm = errors.New("write requires confirm:true")

// SetLoopSetpoint: PUT /api/v1/control/loops/{id}/setpoint (TZ §13).
func (s *Server) SetLoopSetpoint(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	loop, err := store.GetLoop(r.Context(), s.db, id)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "loop not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	var req loopWriteRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	if !req.Confirm {
		writeProblem(w, http.StatusUnprocessableEntity, "confirm_required", errConfirm.Error())
		return
	}
	if math.IsNaN(req.Value) || req.Value < loop.SPMin || req.Value > loop.SPMax {
		writeProblem(w, http.StatusUnprocessableEntity, "out_of_range",
			fmt.Sprintf("setpoint must be in [%.3g, %.3g]", loop.SPMin, loop.SPMax))
		return
	}
	by := subjectOf(r)
	if err := store.UpdateLoopSetpoint(r.Context(), s.db, id, req.Value, by); err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "control.setpoint", "loop", id, fmt.Sprintf("sp=%.4g (was %.4g)", req.Value, loop.SP))
	s.writeLoop(w, r, id)
}

// SetLoopMode: PUT /api/v1/control/loops/{id}/mode (TZ §13).
func (s *Server) SetLoopMode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	loop, err := store.GetLoop(r.Context(), s.db, id)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "loop not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	var req struct {
		Mode    string `json:"mode"`
		Confirm bool   `json:"confirm"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	if !req.Confirm {
		writeProblem(w, http.StatusUnprocessableEntity, "confirm_required", errConfirm.Error())
		return
	}
	if req.Mode != store.LoopModeAuto && req.Mode != store.LoopModeManual {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_mode", "mode must be auto or manual")
		return
	}
	if req.Mode == loop.Mode {
		s.writeLoop(w, r, id)
		return
	}
	by := subjectOf(r)
	if err := store.UpdateLoopMode(r.Context(), s.db, id, req.Mode, by); err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "control.mode", "loop", id, fmt.Sprintf("mode=%s (was %s)", req.Mode, loop.Mode))
	s.writeLoop(w, r, id)
}

// SetLoopOutput: PUT /api/v1/control/loops/{id}/output — manual only (TZ §9).
func (s *Server) SetLoopOutput(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	loop, err := store.GetLoop(r.Context(), s.db, id)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "loop not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	var req loopWriteRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	if !req.Confirm {
		writeProblem(w, http.StatusUnprocessableEntity, "confirm_required", errConfirm.Error())
		return
	}
	if loop.Mode != store.LoopModeManual {
		writeProblem(w, http.StatusConflict, "mode_conflict", "output can be written in manual mode only")
		return
	}
	if math.IsNaN(req.Value) || req.Value < loop.OutMin || req.Value > loop.OutMax {
		writeProblem(w, http.StatusUnprocessableEntity, "out_of_range",
			fmt.Sprintf("output must be in [%.3g, %.3g]", loop.OutMin, loop.OutMax))
		return
	}
	by := subjectOf(r)
	if err := store.UpdateLoopOutput(r.Context(), s.db, id, req.Value, by); err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "control.output", "loop", id, fmt.Sprintf("out=%.4g (was %.4g)", req.Value, loop.Output))
	s.writeLoop(w, r, id)
}

func (s *Server) writeLoop(w http.ResponseWriter, r *http.Request, id string) {
	loop, err := store.GetLoop(r.Context(), s.db, id)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	item := map[string]any{
		"id": loop.ID, "label": loop.Label, "pv_tag": loop.PVTag, "mv_tag": loop.MVTag,
		"sp": loop.SP, "sp_min": loop.SPMin, "sp_max": loop.SPMax,
		"out_min": loop.OutMin, "out_max": loop.OutMax,
		"mode": loop.Mode, "state": loop.State, "out": loop.Output,
	}
	if pv, err := store.LatestGoodNumericReading(r.Context(), s.db, loop.PVTag); err == nil && pv.ValueNumber != nil {
		item["pv"] = *pv.ValueNumber
	}
	writeJSON(w, http.StatusOK, item)
}

// actuatorOutput is one element of the edge-bridge contract (TZ §13): value is
// in engineering units; seq is bumped on every accepted manual write so the
// bridge performs exactly one FC6 write per operator action.
type actuatorOutput struct {
	TagID  string  `json:"tag_id"`
	Value  float64 `json:"value"`
	Seq    int64   `json:"seq"`
	Status string  `json:"status"` // auto | manual | hold
}

// ControlOutput is the bridge feed: every output-direction tag with its last
// commanded value. Loop MVs carry the live PID output in auto (written every
// poll) or the held value in manual; non-loop actuators carry one-shot manual
// writes (hold after delivery). Loopback-only, no session required.
func (s *Server) ControlOutput(w http.ResponseWriter, r *http.Request) {
	loops, err := store.ListLoops(r.Context(), s.db)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	out := make([]actuatorOutput, 0, len(loops)+4)
	for _, l := range loops {
		status := "manual"
		if l.Mode == store.LoopModeAuto && l.State == store.LoopStateOK {
			status = "auto"
		}
		out = append(out, actuatorOutput{TagID: l.MVTag, Value: l.Output, Seq: 0, Status: status})
	}
	writes, err := store.ListActuatorWrites(r.Context(), s.db)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	for _, wr := range writes {
		out = append(out, actuatorOutput{TagID: wr.TagID, Value: wr.Value, Seq: wr.Seq, Status: "hold"})
	}
	writeJSON(w, http.StatusOK, out)
}

// WriteActuator: PUT /api/v1/actuators/{tag_id} — one-shot manual write to a
// non-loop actuator (TZ §9). The edge bridge delivers it once (FC6) and
// returns to hold.
func (s *Server) WriteActuator(w http.ResponseWriter, r *http.Request) {
	tagID := r.PathValue("tag_id")
	tag, err := store.GetTag(r.Context(), s.db, tagID)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "tag not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if tag.Direction != store.DirectionOutput || !tag.Active {
		writeProblem(w, http.StatusUnprocessableEntity, "not_actuator", "tag is not an active output tag")
		return
	}

	var req loopWriteRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	if !req.Confirm {
		writeProblem(w, http.StatusUnprocessableEntity, "confirm_required", errConfirm.Error())
		return
	}
	if (tag.EngineeringMin != nil && req.Value < *tag.EngineeringMin) ||
		(tag.EngineeringMax != nil && req.Value > *tag.EngineeringMax) {
		writeProblem(w, http.StatusUnprocessableEntity, "out_of_range", "value outside the tag engineering range")
		return
	}
	loops, err := store.ListLoops(r.Context(), s.db)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	for _, l := range loops {
		if l.MVTag == tagID {
			writeProblem(w, http.StatusConflict, "loop_driven",
				"actuator is driven by control loop "+l.ID+"; use the loop endpoints")
			return
		}
	}

	by := subjectOf(r)
	wr, err := store.UpsertActuatorWrite(r.Context(), s.db, tagID, req.Value, by)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "actuator.write", "tag", tagID, fmt.Sprintf("value=%.4g seq=%d", req.Value, wr.Seq))
	writeJSON(w, http.StatusOK, map[string]any{"tag_id": tagID, "value": req.Value, "seq": wr.Seq})
}
