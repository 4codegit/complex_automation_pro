package api

import (
	"net/http"

	"cap/internal/metallurgy"
)

// MetallurgySummary: GET /api/v1/metallurgy/summary — the two-product balance
// and derived KPIs (TZ §10). Each KPI carries its formula and source tags.
func (s *Server) MetallurgySummary(w http.ResponseWriter, r *http.Request) {
	snap := metallurgy.Summary(r.Context(), s.db)
	writeJSON(w, http.StatusOK, snap)
}
