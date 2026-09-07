package api

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"cap/internal/store"
)

// ExportReadingsCSV handles GET /api/v1/reports/readings/csv
// Filters: ?tag_id=&from=&to=&quality=&limit=
func (s *Server) ExportReadingsCSV(w http.ResponseWriter, r *http.Request) {
	tagID := queryStr(r, "tag_id")
	from := queryStr(r, "from")
	to := queryStr(r, "to")
	quality := queryStr(r, "quality")
	limit := s.cfg.DefaultLimit
	if v := queryStr(r, "limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= s.cfg.MaxLimit {
			limit = n
		}
	}
	filename := fmt.Sprintf("readings-%s.csv", time.Now().UTC().Format("20060102T150405Z"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.WriteHeader(http.StatusOK)
	if err := store.WriteReadingsCSV(r.Context(), s.db, w, tagID, from, to, quality, limit); err != nil {
		log.Printf("[export] readings csv: %v", err)
	}
}

// ExportAlertsCSV handles GET /api/v1/reports/alerts/csv?stage=&limit=
func (s *Server) ExportAlertsCSV(w http.ResponseWriter, r *http.Request) {
	stage := queryStr(r, "stage")
	limit := s.cfg.DefaultLimit
	if v := queryStr(r, "limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= s.cfg.MaxLimit {
			limit = n
		}
	}
	filename := fmt.Sprintf("alerts-%s.csv", time.Now().UTC().Format("20060102T150405Z"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.WriteHeader(http.StatusOK)
	if err := store.WriteAlertsCSV(r.Context(), s.db, w, stage, limit); err != nil {
		log.Printf("[export] alerts: %v", err)
	}
}
