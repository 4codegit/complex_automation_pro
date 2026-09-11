package api

import (
	"bufio"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"cap/internal/web"
)

// Routes builds the HTTP handler tree of the monolith. It uses the standard
// library mux with Go 1.22 method patterns — no web framework dependency.
// Route domains remain grouped so a future split (or a read-only replica)
// can mount a subset, but capd always mounts everything.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Health: liveness for the API, readiness for its dependencies.
	mux.HandleFunc("GET /api/v1/health", s.HealthCheck)

	// SPA: the embedded dashboard owns all non-API GET paths (/, /assets/*,
	// client-side routes). API and WebSocket patterns take precedence.
	mux.Handle("GET /", web.Handler())

	// Ingest: the single telemetry entry point (edge gateways, calc service).
	mux.HandleFunc("POST /api/v1/ingest/telemetry", s.IngestTelemetry)
	mux.HandleFunc("POST /api/v1/ingest/telemetry:batch", s.IngestTelemetryBatch)
	mux.HandleFunc("POST /api/v1/ingest/gateway_events", s.IngestGatewayEvent)

	// Historian: pull queries, aggregates, CSV reports.
	mux.HandleFunc("GET /api/v1/telemetry", s.TelemetryHistory)
	mux.HandleFunc("GET /api/v1/telemetry/latest", s.TelemetryLatest)
	mux.HandleFunc("GET /api/v1/telemetry/aggregate", s.TelemetryAggregate)
	mux.HandleFunc("GET /api/v1/alerts", s.ListAlerts)
	mux.HandleFunc("GET /api/v1/alerts/latest", s.LatestAlert)
	mux.HandleFunc("GET /api/v1/reports/readings/csv", s.ExportReadingsCSV)
	mux.HandleFunc("GET /api/v1/reports/alerts/csv", s.ExportAlertsCSV)

	// Alarms: ISA-18.2 states, acknowledgement, rationalised limits, journal.
	mux.HandleFunc("GET /api/v1/alarms/active", s.ActiveAlarms)
	mux.HandleFunc("GET /api/v1/alarms/journal", s.AlarmJournal)
	mux.HandleFunc("POST /api/v1/alarms/{id}/ack", s.AckAlarm)
	mux.HandleFunc("GET /api/v1/alarms/limits", s.ListAlarmLimits)
	mux.HandleFunc("GET /api/v1/alarms/limits/{tag_id}", s.GetAlarmLimit)
	mux.HandleFunc("PUT /api/v1/alarms/limits/{tag_id}", s.RationaliseAlarmLimit)
	mux.HandleFunc("DELETE /api/v1/alarms/limits/{tag_id}", s.DeleteAlarmLimit)

	// Ore profiles: versioned change control.
	mux.HandleFunc("GET /api/v1/profiles", s.ListProfiles)
	mux.HandleFunc("GET /api/v1/profiles/active", s.ActiveProfile)
	mux.HandleFunc("POST /api/v1/profiles", s.CreateProfile)
	mux.HandleFunc("POST /api/v1/profiles/{id}/approve", s.ApproveProfile)
	mux.HandleFunc("POST /api/v1/profiles/{id}/activate", s.ActivateProfile)

	// Registry: assets, tags, gateways.
	mux.HandleFunc("GET /api/v1/assets", s.ListAssets)
	mux.HandleFunc("POST /api/v1/assets", s.CreateAsset)
	mux.HandleFunc("PUT /api/v1/assets/{id}", s.UpdateAsset)
	mux.HandleFunc("DELETE /api/v1/assets/{id}", s.DeleteAsset)
	mux.HandleFunc("GET /api/v1/tags", s.ListTags)
	mux.HandleFunc("POST /api/v1/tags", s.CreateTag)
	mux.HandleFunc("PUT /api/v1/tags/{id}", s.UpdateTag)
	mux.HandleFunc("DELETE /api/v1/tags/{id}", s.DeleteTag)
	mux.HandleFunc("GET /api/v1/gateways", s.ListGateways)

	// Identity: RBAC roles, assignments, audit trail, local login.
	mux.HandleFunc("POST /api/v1/access/login", s.Login)
	mux.HandleFunc("POST /api/v1/access/logout", s.Logout)
	mux.HandleFunc("GET /api/v1/access/whoami", s.WhoAmI)
	mux.HandleFunc("GET /api/v1/access/roles", s.ListRoles)
	mux.HandleFunc("POST /api/v1/access/roles", s.CreateRole)
	mux.HandleFunc("PUT /api/v1/access/roles/{id}", s.UpdateRole)
	mux.HandleFunc("DELETE /api/v1/access/roles/{id}", s.DeleteRole)
	mux.HandleFunc("GET /api/v1/access/assignments", s.ListAssignments)
	mux.HandleFunc("POST /api/v1/access/assignments", s.AssignRole)
	mux.HandleFunc("DELETE /api/v1/access/assignments", s.RevokeRole)
	mux.HandleFunc("GET /api/v1/access/audit", s.ListAudit)

	// Supervisory control (TZ §9): loops, actuator writes, bridge feed.
	mux.HandleFunc("GET /api/v1/control/loops", s.ControlStatus)
	mux.HandleFunc("PUT /api/v1/control/loops/{id}/setpoint", s.SetLoopSetpoint)
	mux.HandleFunc("PUT /api/v1/control/loops/{id}/mode", s.SetLoopMode)
	mux.HandleFunc("PUT /api/v1/control/loops/{id}/output", s.SetLoopOutput)
	mux.HandleFunc("GET /api/v1/control/output", s.ControlOutput)
	mux.HandleFunc("PUT /api/v1/actuators/{tag_id}", s.WriteActuator)

	// Metallurgical balance (TZ §10) and process stand scenarios (TZ §16).
	mux.HandleFunc("GET /api/v1/metallurgy/summary", s.MetallurgySummary)
	mux.HandleFunc("POST /api/v1/scenario", s.Scenario)

	// Live dashboard fan-out.
	mux.HandleFunc("GET /api/v1/ws", s.WebSocket)

	// Middleware order: logging → session authentication → RBAC.
	return withLogging(s.authenticate(s.rbac(mux)))
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack lets WebSocket upgrades pass through the wrappers: without it
// gorilla/websocket cannot upgrade and returns 500.
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijack not supported")
	}
	w.status = http.StatusSwitchingProtocols
	return h.Hijack()
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Microsecond))
	})
}
