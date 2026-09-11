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
	Comment string `json:"comment"`
}

// AckAlarm acknowledges an active_unacknowledged alarm and records the action
// in the audit trail and the alarm journal. The actor is the authenticated
// session user — acknowledgement is never anonymous (TZ §11).
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

	actor := subjectOf(r)
	now := s.now().UTC()
	if err := store.AckAlarm(r.Context(), s.db, id, actor, req.Comment, now); err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.auditNow(r.Context(), now, r, "alarm.ack", "alarm", id, "metric="+alarm.Metric)

	if s.alarms != nil {
		s.alarms.Acked(r.Context(), alarm, actor, req.Comment, now)
	}

	updated, err := store.GetAlarm(r.Context(), s.db, id)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// AlarmJournal returns the immutable ISA-18.2 journal (TZ §11/§13).
func (s *Server) AlarmJournal(w http.ResponseWriter, r *http.Request) {
	filter := store.AlarmEventFilter{
		TagID: queryStr(r, "tag_id"),
		Limit: s.cfg.DefaultLimit,
	}
	if v := queryStr(r, "min_priority"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.MinPriority = n
		}
	}
	if from, ok := parseTimeParam(r.URL.Query(), "from", "from_time"); ok {
		filter.From = &from
	}
	if to, ok := parseTimeParam(r.URL.Query(), "to", "to_time"); ok {
		filter.To = &to
	}
	if v := queryStr(r, "limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= s.cfg.MaxLimit {
			filter.Limit = n
		}
	}
	events, err := store.QueryAlarmEvents(r.Context(), s.db, filter)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, events)
}
