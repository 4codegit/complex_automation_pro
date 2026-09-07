package api

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"cap/internal/store"
)

// actorFromRequest resolves the mutating user for the audit trail. Production
// deployments inject the subject from OIDC/JWT; the demo phase trusts two
// headers — X-User (subject) and X-Role (role). The "anonymous" fallback keeps
// the existing curl-driven scripts working without changes.
type actor struct {
	Subject string
	Role    string
	SrcIP   string
}

func actorFromRequest(r *http.Request) actor {
	subj := strings.TrimSpace(r.Header.Get("X-User"))
	if subj == "" {
		subj = "anonymous"
	}
	role := strings.TrimSpace(r.Header.Get("X-Role"))
	src := r.Header.Get("X-Forwarded-For")
	if src == "" {
		src = strings.Split(r.RemoteAddr, ":")[0]
	}
	return actor{Subject: subj, Role: role, SrcIP: src}
}

// audit records one immutable audit event for a mutation. It is best-effort:
// failures are logged at warn level but never fail the caller — the action has
// already been authorised and persisted.
func (s *Server) audit(r *http.Request, action, resourceType, resourceID, detail string) {
	a := actorFromRequest(r)
	if err := store.Audit(r.Context(), s.db, &store.AuditEvent{
		OccurredAt:   s.now().UTC(),
		Actor:        a.Subject,
		Role:         a.Role,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Detail:       detail,
		SrcIP:        a.SrcIP,
	}); err != nil {
		log.Printf("[audit] failed to record %s %s/%s by %s: %v",
			action, resourceType, resourceID, a.Subject, err)
	}
}

// auditNow records an audit event with an explicit timestamp (for cases that
// want the same `now` as the mutated row).
func (s *Server) auditNow(ctx context.Context, at time.Time, r *http.Request, action, resourceType, resourceID, detail string) {
	a := actorFromRequest(r)
	if err := store.Audit(ctx, s.db, &store.AuditEvent{
		OccurredAt:   at,
		Actor:        a.Subject,
		Role:         a.Role,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Detail:       detail,
		SrcIP:        a.SrcIP,
	}); err != nil {
		log.Printf("[audit] failed to record %s %s/%s by %s: %v",
			action, resourceType, resourceID, a.Subject, err)
	}
}
