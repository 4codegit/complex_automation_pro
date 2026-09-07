package api

import (
	"context"
	"net/http"
	"strings"

	"autopro/internal/store"
)

// permissionRequired maps a request method+path pattern to the permission a
// subject must hold to call it. Paths use the same Go 1.22 mux patterns as
// router.go. Unlisted endpoints are open — RBAC is opt-in per mutation.
var permissionRequired = map[string]string{
	"POST /api/v1/assets":                   "manage_tags",
	"PUT /api/v1/assets/{id}":               "manage_tags",
	"DELETE /api/v1/assets/{id}":            "manage_tags",
	"POST /api/v1/tags":                     "manage_tags",
	"PUT /api/v1/tags/{id}":                 "manage_tags",
	"DELETE /api/v1/tags/{id}":              "manage_tags",
	"POST /api/v1/alarms/{id}/ack":          "acknowledge_alarms",
	"PUT /api/v1/alarms/limits/{tag_id}":    "rationalise_alarms",
	"DELETE /api/v1/alarms/limits/{tag_id}": "rationalise_alarms",
	"POST /api/v1/profiles":                 "manage_profiles",
	"POST /api/v1/profiles/{id}/approve":    "approve_profiles",
	"POST /api/v1/profiles/{id}/activate":   "activate_profiles",
	"POST /api/v1/access/roles":             "manage_roles",
	"PUT /api/v1/access/roles/{id}":         "manage_roles",
	"DELETE /api/v1/access/roles/{id}":      "manage_roles",
	"POST /api/v1/access/assignments":       "manage_users",
	"DELETE /api/v1/access/assignments":     "manage_users",
}

// permissionForRequest returns the permission a request requires, or "".
// Patterns with path placeholders ({id}, {tag_id}) are matched by segments so
// the lookup does not need a real router.
func permissionForRequest(r *http.Request) string {
	key := strings.ToUpper(r.Method) + " " + normalisePath(r.URL.Path)
	if perm, ok := permissionRequired[key]; ok {
		return perm
	}
	return ""
}

// normalisePath collapses the concrete id in a protected URL into the
// placeholder used in permissionRequired, so the lookup matches. The set of
// protected patterns is small and known, so this is a deliberate, simple
// matcher instead of a generic path-templating engine.
func normalisePath(p string) string {
	switch {
	case strings.HasPrefix(p, "/api/v1/alarms/limits/"):
		// PUT/DELETE /api/v1/alarms/limits/{tag_id}
		return "/api/v1/alarms/limits/{tag_id}"
	case strings.HasPrefix(p, "/api/v1/alarms/") && strings.HasSuffix(p, "/ack"):
		// POST /api/v1/alarms/{id}/ack
		return "/api/v1/alarms/{id}/ack"
	case strings.HasPrefix(p, "/api/v1/assets/") && strings.Count(p, "/") == 3:
		return "/api/v1/assets/{id}"
	case strings.HasPrefix(p, "/api/v1/tags/") && strings.Count(p, "/") == 3:
		return "/api/v1/tags/{id}"
	case strings.HasPrefix(p, "/api/v1/profiles/"):
		// POST /api/v1/profiles/{id}/approve | /activate
		if strings.HasSuffix(p, "/approve") || strings.HasSuffix(p, "/activate") {
			// Cut the uuid segment, keep the verb suffix.
			tail := strings.TrimPrefix(p, "/api/v1/profiles/")
			verb := tail[strings.Index(tail, "/"):] // "/approve" or "/activate"
			return "/api/v1/profiles/{id}" + verb
		}
		return p
	}
	return p
}

// rbac is the authorization middleware. It runs after withLogging so audit is
// still populated for denied requests (the actor is what matters for "who
// tried to do what, when"). Denied requests return 403 with code
// "forbidden" — the permission name is exposed for clarity.
//
// The middleware is safe to compose with any handler: it resolves the required
// permission lazily, and only touches the DB when one is needed.
func (s *Server) rbac(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		perm := permissionForRequest(r)
		if perm == "" {
			next.ServeHTTP(w, r)
			return
		}
		actor := actorFromRequest(r)
		// In production the X-User header carries the authenticated subject
		// and the request is rejected without it. For the demo phase tests
		// and local scripts, the anonymous subject resolves to whatever roles
		// were assigned to it (the test harness assigns platform_admin to
		// "anonymous" once at startup). No special-casing here keeps the table
		// semantics honest: if "anonymous" has no role, the request is 403.
		subject := actor.Subject
		allowed, err := store.SubjectHasPermission(r.Context(), s.db, subject, perm)
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

// withContext attaches the subject id to the request context for downstream
// handlers. Kept here as a hook for future subject-scoped logic (e.g.
// "view_assigned_area").
func withContext(ctx context.Context, subject string) context.Context {
	return context.WithValue(ctx, ctxKeySubject{}, subject)
}

type ctxKeySubject struct{}

func subjectFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeySubject{}).(string); ok {
		return v
	}
	return ""
}
