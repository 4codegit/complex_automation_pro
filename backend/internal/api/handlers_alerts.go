package api

import (
	"net/http"
	"strconv"

	"cap/internal/store"
)

// ListAlerts returns persisted alert history (legacy feed used by scripts).
func (s *Server) ListAlerts(w http.ResponseWriter, r *http.Request) {
	limit := s.cfg.DefaultLimit
	if v := queryStr(r, "limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= s.cfg.MaxLimit {
			limit = n
		}
	}
	alerts, err := store.ListAlerts(r.Context(), s.db, queryStr(r, "stage"), queryStr(r, "metric"), limit)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, alerts)
}

// LatestAlert returns the most recent alert or JSON null.
func (s *Server) LatestAlert(w http.ResponseWriter, r *http.Request) {
	alerts, err := store.ListAlerts(r.Context(), s.db, "", "", 1)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if len(alerts) == 0 {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, alerts[0])
}
