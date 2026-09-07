package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"autopro/internal/store"
)

// handlers_audit.go — audit trail read endpoint and alarm rationalisation
// CRUD. The audit trail is append-only; rationalisation limits are versioned
// per rationalise call (the old row is overwritten and an audit event records
// the change). Both surface under /api/v1/access so the permission matrix is
// the single source of truth for who-may-what.

// ListAudit handles GET /api/v1/access/audit?actor=&resource_type=&limit=
func (s *Server) ListAudit(w http.ResponseWriter, r *http.Request) {
	actor := queryStr(r, "actor")
	rt := queryStr(r, "resource_type")
	limit := s.cfg.DefaultLimit
	if v := queryStr(r, "limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= s.cfg.MaxLimit {
			limit = n
		}
	}
	events, err := store.ListAuditEvents(r.Context(), s.db, actor, rt, limit)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// ListAlarmLimits handles GET /api/v1/alarms/limits
func (s *Server) ListAlarmLimits(w http.ResponseWriter, r *http.Request) {
	limits, err := store.ListAlarmLimits(r.Context(), s.db)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, limits)
}

// GetAlarmLimit handles GET /api/v1/alarms/limits/{tag_id}
func (s *Server) GetAlarmLimit(w http.ResponseWriter, r *http.Request) {
	tagID := r.PathValue("tag_id")
	l, err := store.GetAlarmLimit(r.Context(), s.db, tagID)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "no rationalised limits for this tag")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, l)
}

// rationaliseAlarmLimitRequest is the body of PUT
// /api/v1/alarms/limits/{tag_id}. Threshold values may be null to disable
// that boundary; enabled=false shelves the whole tag.
type rationaliseAlarmLimitRequest struct {
	Enabled  *bool    `json:"enabled"`
	LoLo     *float64 `json:"lo_lo"`
	Lo       *float64 `json:"lo"`
	Hi       *float64 `json:"hi"`
	HiHi     *float64 `json:"hi_hi"`
	Severity string   `json:"severity"`
	Notes    string   `json:"notes"`
}

// RationaliseAlarmLimit handles PUT /api/v1/alarms/limits/{tag_id} and is the
// authoritative rationalisation action: plant-approved thresholds for one tag.
// The required X-User header (or body `rationalised_by` once we add auth)
// records who approved the change for the audit trail.
func (s *Server) RationaliseAlarmLimit(w http.ResponseWriter, r *http.Request) {
	tagID := r.PathValue("tag_id")

	if _, err := store.GetTag(r.Context(), s.db, tagID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeProblem(w, http.StatusNotFound, "not_found", "tag not registered")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	var req rationaliseAlarmLimitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "body is not valid JSON")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.Severity == "" {
		req.Severity = "medium"
	}
	now := s.now().UTC()
	limit := &store.AlarmLimit{
		TagID:          tagID,
		Enabled:        enabled,
		LoLo:           req.LoLo,
		Lo:             req.Lo,
		Hi:             req.Hi,
		HiHi:           req.HiHi,
		Severity:       req.Severity,
		RationalisedBy: actorFromRequest(r).Subject,
		Notes:          req.Notes,
	}
	if err := store.UpsertAlarmLimit(r.Context(), s.db, limit, now); err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	limit.RationalisedAt = &now

	detail := serializeLimitDetails(limit)
	s.auditNow(r.Context(), now, r, "alarm_limit.rationalise", "alarm_limit", tagID, detail)

	writeJSON(w, http.StatusOK, limit)
}

// DeleteAlarmLimit handles DELETE /api/v1/alarms/limits/{tag_id} and retires
// the rationalised limits for a tag (used when a tag is decommissioned or a
// rationalisation is reverted).
func (s *Server) DeleteAlarmLimit(w http.ResponseWriter, r *http.Request) {
	tagID := r.PathValue("tag_id")
	if err := store.DeleteAlarmLimit(r.Context(), s.db, tagID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeProblem(w, http.StatusNotFound, "not_found", "no rationalised limits for this tag")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "alarm_limit.delete", "alarm_limit", tagID, "")
	writeJSON(w, http.StatusNoContent, nil)
}

func serializeLimitDetails(l *store.AlarmLimit) string {
	b, _ := json.Marshal(map[string]any{
		"enabled":         l.Enabled,
		"severity":        l.Severity,
		"rationalised_by": l.RationalisedBy,
		"lo_lo":           l.LoLo,
		"lo":              l.Lo,
		"hi":              l.Hi,
		"hi_hi":           l.HiHi,
		"notes":           l.Notes,
		"rationalised_at": timeOrEmpty(l.RationalisedAt),
	})
	return string(b)
}

func timeOrEmpty(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
