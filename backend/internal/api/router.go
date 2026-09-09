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

// Routes builds the HTTP handler tree. It uses the standard library mux with
// Go 1.22 method patterns — no web framework dependency.
//
// Sections select the route domains this process owns, one per microservice:
//
//	ingest    push ingestion (gateways -> platform)
//	historian pull queries, aggregates, CSV reports
//	alarms    alarm states, acknowledgement, rationalised limits
//	profiles  ore profiles and change control
//	registry  assets, tags, gateway registry
//	identity  RBAC roles, assignments, audit trail
//	live      WebSocket fan-out, simulator controls, event intake, SPA
//
// Routes() with no sections mounts everything: the all-in-one development
// bundle (cmd/server). Split deployments pass their own subset (cmd/ingest,
// cmd/historian, ...) and gateway-api fronts them on a single address.
func (s *Server) Routes(sections ...string) http.Handler {
	mux := http.NewServeMux()

	all := len(sections) == 0
	has := func(name string) bool {
		if all {
			return true
		}
		for _, sec := range sections {
			if sec == name {
				return true
			}
		}
		return false
	}

	// Health is always mounted: every service must answer its own readiness.
	mux.HandleFunc("GET /api/v1/health", s.HealthCheck)

	if has("live") {
		// SPA: the embedded dashboard owns all non-API GET paths (/, /assets/*,
		// client-side routes). API and WebSocket patterns take precedence.
		mux.Handle("GET /", web.Handler())
	}

	if has("ingest") {
		mux.HandleFunc("POST /api/v1/ingest/telemetry", s.IngestTelemetry)
		mux.HandleFunc("POST /api/v1/ingest/telemetry:batch", s.IngestTelemetryBatch)
		mux.HandleFunc("POST /api/v1/ingest/gateway_events", s.IngestGatewayEvent)
	}

	if has("historian") {
		mux.HandleFunc("GET /api/v1/telemetry", s.TelemetryHistory)
		mux.HandleFunc("GET /api/v1/telemetry/latest", s.TelemetryLatest)
		mux.HandleFunc("GET /api/v1/telemetry/aggregate", s.TelemetryAggregate)
		mux.HandleFunc("GET /api/v1/alerts", s.ListAlerts)
		mux.HandleFunc("GET /api/v1/alerts/latest", s.LatestAlert)
		mux.HandleFunc("GET /api/v1/reports/readings/csv", s.ExportReadingsCSV)
		mux.HandleFunc("GET /api/v1/reports/alerts/csv", s.ExportAlertsCSV)
	}

	if has("alarms") {
		mux.HandleFunc("GET /api/v1/alarms/active", s.ActiveAlarms)
		mux.HandleFunc("POST /api/v1/alarms/{id}/ack", s.AckAlarm)
		// Alarm rationalisation (ADR-002 / ISA-18.2): plant-approved limits.
		mux.HandleFunc("GET /api/v1/alarms/limits", s.ListAlarmLimits)
		mux.HandleFunc("GET /api/v1/alarms/limits/{tag_id}", s.GetAlarmLimit)
		mux.HandleFunc("PUT /api/v1/alarms/limits/{tag_id}", s.RationaliseAlarmLimit)
		mux.HandleFunc("DELETE /api/v1/alarms/limits/{tag_id}", s.DeleteAlarmLimit)
	}

	if has("profiles") {
		mux.HandleFunc("GET /api/v1/profiles", s.ListProfiles)
		mux.HandleFunc("GET /api/v1/profiles/active", s.ActiveProfile)
		mux.HandleFunc("POST /api/v1/profiles", s.CreateProfile)
		mux.HandleFunc("POST /api/v1/profiles/{id}/approve", s.ApproveProfile)
		mux.HandleFunc("POST /api/v1/profiles/{id}/activate", s.ActivateProfile)
	}

	if has("registry") {
		mux.HandleFunc("GET /api/v1/assets", s.ListAssets)
		mux.HandleFunc("POST /api/v1/assets", s.CreateAsset)
		mux.HandleFunc("PUT /api/v1/assets/{id}", s.UpdateAsset)
		mux.HandleFunc("DELETE /api/v1/assets/{id}", s.DeleteAsset)
		mux.HandleFunc("GET /api/v1/tags", s.ListTags)
		mux.HandleFunc("POST /api/v1/tags", s.CreateTag)
		mux.HandleFunc("PUT /api/v1/tags/{id}", s.UpdateTag)
		mux.HandleFunc("DELETE /api/v1/tags/{id}", s.DeleteTag)
		mux.HandleFunc("GET /api/v1/gateways", s.ListGateways)
	}

	if has("identity") {
		mux.HandleFunc("GET /api/v1/access/roles", s.ListRoles)
		mux.HandleFunc("POST /api/v1/access/roles", s.CreateRole)
		mux.HandleFunc("PUT /api/v1/access/roles/{id}", s.UpdateRole)
		mux.HandleFunc("DELETE /api/v1/access/roles/{id}", s.DeleteRole)
		mux.HandleFunc("GET /api/v1/access/assignments", s.ListAssignments)
		mux.HandleFunc("POST /api/v1/access/assignments", s.AssignRole)
		mux.HandleFunc("DELETE /api/v1/access/assignments", s.RevokeRole)
		mux.HandleFunc("GET /api/v1/access/whoami", s.WhoAmI)
		mux.HandleFunc("GET /api/v1/access/audit", s.ListAudit)
	}

	if has("live") {
		// Live dashboard
		mux.HandleFunc("GET /api/v1/ws", s.WebSocket)
		// Cross-service event intake: the ingest service forwards accepted
		// readings here so WebSocket subscribers keep their live feed in
		// split mode (EVENT_SINKS on the ingest side).
		mux.HandleFunc("POST /internal/events", s.InternalEvent)
		// Development simulator (must be disabled in production)
		mux.HandleFunc("GET /api/v1/simulator/status", s.SimulatorStatus)
		mux.HandleFunc("POST /api/v1/simulator/start", s.SimulatorStart)
		mux.HandleFunc("POST /api/v1/simulator/stop", s.SimulatorStop)
		mux.HandleFunc("POST /api/v1/simulator/emergency", s.SimulatorEmergency)
		mux.HandleFunc("POST /api/v1/simulator/emergency/stop", s.SimulatorEmergencyStop)
	}

	return withLogging(s.rbac(mux))
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

// Hijack lets WebSocket upgrades pass through the logging wrapper: without it
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
