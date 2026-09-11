package api

import (
	"context"
	"net"
	"net/http"
	"strings"

	"cap/internal/auth"
	"cap/internal/store"
)

// sessionCookie is the login cookie name (TZ §12).
const sessionCookie = "cap_session"

// ctxKeyUser carries the authenticated username through the request context.
type ctxKeyUser struct{}

// publicPaths skip session authentication. /api/v1/control/output is the
// machine-to-machine endpoint the edge bridge polls every second; in the MVP
// topology it is restricted to loopback clients.
var publicPaths = map[string]bool{
	"/api/v1/health":       true,
	"/api/v1/access/login": true,
}

// authenticate resolves the session cookie to a user and stores the subject
// in the request context. Requests without a valid session receive 401 unless
// the path is public, the caller presents the shared gateway device token
// (machine endpoints: ingest + heartbeat), or the bridge endpoint is polled
// from loopback.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The embedded SPA and its assets are public: the browser needs the
		// shell to render the login screen. Only /api/* is authenticated.
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if publicPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		// Machine path: edge gateways authenticate with the shared device
		// token, not a user session (TZ §12). Ingest is the only write path
		// they need; the heartbeat rides the same prefix.
		if strings.HasPrefix(r.URL.Path, "/api/v1/ingest/") &&
			s.cfg.GatewayToken != "" &&
			constantTimeEq(r.Header.Get("X-Gateway-Token"), s.cfg.GatewayToken) {
			next.ServeHTTP(w, r)
			return
		}
		// The actuator-output endpoint is polled by the edge bridge, which has
		// no user session. Loopback-only keeps it off the network boundary.
		if r.URL.Path == "/api/v1/control/output" && isLoopback(r) {
			next.ServeHTTP(w, r)
			return
		}

		c, err := r.Cookie(sessionCookie)
		if err == nil && c.Value != "" {
			subject, serr := store.GetSessionUser(r.Context(), s.db, auth.HashToken(c.Value), s.now().UTC())
			if serr == nil {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyUser{}, subject)))
				return
			}
		}
		w.Header().Set("WWW-Authenticate", `Session realm="cap"`)
		writeProblem(w, http.StatusUnauthorized, "unauthorized", "authentication required")
	})
}

// constantTimeEq compares two secrets without early exit.
func constantTimeEq(a, b string) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// subjectOf returns the authenticated username ("anonymous" only for tests
// running with auth explicitly bypassed).
func subjectOf(r *http.Request) string {
	if v, ok := r.Context().Value(ctxKeyUser{}).(string); ok && v != "" {
		return v
	}
	return "anonymous"
}

// permissionRequired maps a request method+path pattern to the permission a
// subject must hold to call it. Paths use the same Go 1.22 mux patterns as
// router.go. Unlisted endpoints require only a valid session.
var permissionRequired = map[string]string{
	"POST /api/v1/assets":                     "manage_tags",
	"PUT /api/v1/assets/{id}":                 "manage_tags",
	"DELETE /api/v1/assets/{id}":              "manage_tags",
	"POST /api/v1/tags":                       "manage_tags",
	"PUT /api/v1/tags/{id}":                   "manage_tags",
	"DELETE /api/v1/tags/{id}":                "manage_tags",
	"POST /api/v1/alarms/{id}/ack":            "acknowledge_alarms",
	"PUT /api/v1/alarms/limits/{tag_id}":      "rationalise_alarms",
	"DELETE /api/v1/alarms/limits/{tag_id}":   "rationalise_alarms",
	"POST /api/v1/profiles":                   "manage_profiles",
	"POST /api/v1/profiles/{id}/approve":      "approve_profiles",
	"POST /api/v1/profiles/{id}/activate":     "activate_profiles",
	"POST /api/v1/access/roles":               "manage_roles",
	"PUT /api/v1/access/roles/{id}":           "manage_roles",
	"DELETE /api/v1/access/roles/{id}":        "manage_roles",
	"POST /api/v1/access/assignments":         "manage_users",
	"DELETE /api/v1/access/assignments":       "manage_users",
	"PUT /api/v1/control/loops/{id}/setpoint": "control_process",
	"PUT /api/v1/control/loops/{id}/mode":     "control_process",
	"PUT /api/v1/control/loops/{id}/output":   "control_process",
	"PUT /api/v1/actuators/{tag_id}":          "control_process",
	"POST /api/v1/scenario":                   "scenario_run",
}

// permissionForRequest returns the permission a request requires, or "".
// Patterns with path placeholders are matched by segment count so the lookup
// does not need a real router.
func permissionForRequest(r *http.Request) string {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/api/v1/control/loops/") && strings.HasSuffix(p, "/setpoint"):
		return "control_process"
	case strings.HasPrefix(p, "/api/v1/control/loops/") && strings.HasSuffix(p, "/mode"):
		return "control_process"
	case strings.HasPrefix(p, "/api/v1/control/loops/") && strings.HasSuffix(p, "/output"):
		return "control_process"
	case strings.HasPrefix(p, "/api/v1/actuators/"):
		return "control_process"
	case p == "/api/v1/scenario":
		return "scenario_run"
	}
	return permissionRequired[r.Method+" "+normalisePath(p)]
}

// normalisePath collapses the concrete id in a protected URL into the
// placeholder used in permissionRequired.
func normalisePath(p string) string {
	switch {
	case strings.HasPrefix(p, "/api/v1/alarms/limits/"):
		return "/api/v1/alarms/limits/{tag_id}"
	case strings.HasPrefix(p, "/api/v1/alarms/") && strings.HasSuffix(p, "/ack"):
		return "/api/v1/alarms/{id}/ack"
	case strings.HasPrefix(p, "/api/v1/assets/") && strings.Count(p, "/") == 3:
		return "/api/v1/assets/{id}"
	case strings.HasPrefix(p, "/api/v1/tags/") && strings.Count(p, "/") == 3:
		return "/api/v1/tags/{id}"
	case strings.HasPrefix(p, "/api/v1/profiles/"):
		if strings.HasSuffix(p, "/approve") || strings.HasSuffix(p, "/activate") {
			tail := strings.TrimPrefix(p, "/api/v1/profiles/")
			verb := tail[strings.Index(tail, "/"):]
			return "/api/v1/profiles/{id}" + verb
		}
		return p
	}
	return p
}

// rbac is the authorization middleware. It runs after authentication; denied
// requests return 403 with the missing permission exposed for clarity.
func (s *Server) rbac(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		perm := permissionForRequest(r)
		if perm == "" {
			next.ServeHTTP(w, r)
			return
		}
		allowed, err := store.SubjectHasPermission(r.Context(), s.db, subjectOf(r), perm)
		if err != nil {
			writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if !allowed {
			writeProblem(w, http.StatusForbidden, "forbidden",
				"role assignment lacks permission: "+perm)
			return
		}
		next.ServeHTTP(w, r)
	})
}
