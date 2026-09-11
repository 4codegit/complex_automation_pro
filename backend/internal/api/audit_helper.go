package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"cap/internal/store"
)

// actor resolves the mutating user for the audit trail. The subject always
// comes from the authenticated session (TZ §12); the role label is resolved
// from the primary assignment when present.
type actor struct {
	Subject string
	Role    string
	SrcIP   string
}

func actorFromRequest(r *http.Request) actor {
	src := r.Header.Get("X-Forwarded-For")
	if src == "" {
		src = strings_SplitHost(r.RemoteAddr)
	}
	return actor{Subject: subjectOf(r), SrcIP: src}
}

func strings_SplitHost(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
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
