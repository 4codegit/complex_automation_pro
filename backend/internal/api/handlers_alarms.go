package api

import (
	"errors"
	"net/http"
	"strconv"

	"cap/internal/store"
)

// ActiveAlarms returns alarms in an active lifecycle state, newest first.
func (s *Server) ActiveAlarms(w http.ResponseWriter, r *http.Request) {
	limit := s.cfg.DefaultLimit
	if v := queryStr(r, "limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= s.cfg.MaxLimit {
			limit = n
		}
	}
	alarms, err := store.ListActiveAlarms(r.Context(), s.db, limit)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, alarms)
}

type ackAlarmRequest struct {
	AckBy   string `json:"ack_by"`
	Comment string `json:"comment"`
}

// AckAlarm acknowledges an active_unacknowledged alarm and records an audit
// event. Acknowledgement is a human action and never mutates the telemetry.
func (s *Server) AckAlarm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	alarm, err := store.GetAlarm(r.Context(), s.db, id)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "alarm not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if alarm.State != store.AlarmStateActiveUnack {
		writeProblem(w, http.StatusConflict, "not_acknowledgeable",
			"only active_unacknowledged alarms can be acknowledged")
		return
	}

	var req ackAlarmRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	if req.AckBy == "" {
		writeProblem(w, http.StatusUnprocessableEntity, "missing_actor", "ack_by is required")
		return
	}

	now := s.now().UTC()
	if err := store.AckAlarm(r.Context(), s.db, id, req.AckBy, req.Comment, now); err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.auditNow(r.Context(), now, r, "alarm.ack", "alarm", id, "by="+req.AckBy+" metric="+alarm.Metric)

	_ = store.InsertAlert(r.Context(), s.db, &store.Alert{
		ID:        store.NewID(),
		Stage:     "alarm_management",
		Metric:    alarm.Metric,
		Value:     0,
		Threshold: 0,
		Message:   "ACK alarm " + alarm.ID + " (" + alarm.Metric + ") by " + req.AckBy,
		CreatedAt: now,
	})

	updated, err := store.GetAlarm(r.Context(), s.db, id)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
