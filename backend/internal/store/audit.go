package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// AuditEvent is one immutable row in the platform audit trail. Mutations of
// registry, profiles, alarms and alarm limits MUST record one. Reads are not
// audited — the trail exists to answer "who changed what, when".
type AuditEvent struct {
	ID           string    `json:"id"`
	OccurredAt   time.Time `json:"occurred_at"`
	Actor        string    `json:"actor"`
	Role         string    `json:"role"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	Detail       string    `json:"detail"`
	SrcIP        string    `json:"src_ip"`
}

// Audit emits an immutable audit event. It never returns an error that should
// fail the caller: the audit trail is best-effort and must not block the
// action it records. Callers therefore log the returned error at warn level.
func Audit(ctx context.Context, db *sql.DB, e *AuditEvent) error {
	if e.ID == "" {
		e.ID = NewID()
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO audit_events
		 (id, occurred_at, actor, role, action, resource_type, resource_id, detail, src_ip)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, FormatUTC(e.OccurredAt), e.Actor, e.Role, e.Action,
		e.ResourceType, e.ResourceID, e.Detail, e.SrcIP)
	return err
}

// ListAuditEvents returns audit events newest-first. Optional filters narrow
// by actor or resource_type; limit caps the page.
func ListAuditEvents(ctx context.Context, db *sql.DB, actor, resourceType string, limit int) ([]AuditEvent, error) {
	q := `SELECT id, occurred_at, actor, role, action, resource_type, resource_id, detail, src_ip
	      FROM audit_events`
	var args []any
	var where []string
	if actor != "" {
		where = append(where, `actor = ?`)
		args = append(args, actor)
	}
	if resourceType != "" {
		where = append(where, `resource_type = ?`)
		args = append(args, resourceType)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	if limit <= 0 {
		limit = 100
	}
	q += " ORDER BY occurred_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AuditEvent, 0)
	for rows.Next() {
		var e AuditEvent
		var occ string
		if err := rows.Scan(&e.ID, &occ, &e.Actor, &e.Role, &e.Action,
			&e.ResourceType, &e.ResourceID, &e.Detail, &e.SrcIP); err != nil {
			return nil, err
		}
		e.OccurredAt = mustParse(occ)
		out = append(out, e)
	}
	return out, rows.Err()
}

// joinStrings removed — strings.Join from stdlib is used.

// AlarmLimit is the plant-approved alarm rationalisation for one tag. A nil
// threshold disables that boundary; enabled=0 shelves the whole tag.
type AlarmLimit struct {
	TagID          string     `json:"tag_id"`
	Enabled        bool       `json:"enabled"`
	LoLo           *float64   `json:"lo_lo"`
	Lo             *float64   `json:"lo"`
	Hi             *float64   `json:"hi"`
	HiHi           *float64   `json:"hi_hi"`
	Severity       string     `json:"severity"`
	RationalisedBy string     `json:"rationalised_by"`
	RationalisedAt *time.Time `json:"rationalised_at"`
	Notes          string     `json:"notes"`
	Tag            *Tag       `json:"tag,omitempty"`
}

// UpsertAlarmLimit stores the rationalised limits for a tag. It overwrites the
// previous row, which is the rationalisation semantics: each change is a new
// approved setting and the old one is retired (recorded in audit_events).
func UpsertAlarmLimit(ctx context.Context, db *sql.DB, l *AlarmLimit, now time.Time) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO alarm_limits
		 (tag_id, enabled, lo_lo, lo, hi, hi_hi, severity, rationalised_by, rationalised_at, notes)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(tag_id) DO UPDATE SET
		   enabled = excluded.enabled,
		   lo_lo = excluded.lo_lo,
		   lo = excluded.lo,
		   hi = excluded.hi,
		   hi_hi = excluded.hi_hi,
		   severity = excluded.severity,
		   rationalised_by = excluded.rationalised_by,
		   rationalised_at = excluded.rationalised_at,
		   notes = excluded.notes`,
		l.TagID, boolToInt(l.Enabled), l.LoLo, l.Lo, l.Hi, l.HiHi, l.Severity,
		l.RationalisedBy, nullTime(&now), l.Notes)
	return err
}

// GetAlarmLimit returns the rationalised limits for one tag, or ErrNotFound.
func GetAlarmLimit(ctx context.Context, db *sql.DB, tagID string) (*AlarmLimit, error) {
	row := db.QueryRowContext(ctx,
		`SELECT tag_id, enabled, lo_lo, lo, hi, hi_hi, severity,
		        rationalised_by, rationalised_at, notes
		 FROM alarm_limits WHERE tag_id = ?`, tagID)
	l, err := scanAlarmLimit(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return l, err
}

// ListAlarmLimits returns all rationalised limits ordered by tag_id.
func ListAlarmLimits(ctx context.Context, db *sql.DB) ([]AlarmLimit, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT tag_id, enabled, lo_lo, lo, hi, hi_hi, severity,
		        rationalised_by, rationalised_at, notes
		 FROM alarm_limits ORDER BY tag_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AlarmLimit, 0)
	for rows.Next() {
		l, err := scanAlarmLimit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// DeleteAlarmLimit removes a tag's rationalised limits (e.g. when the tag is
// retired). Returns ErrNotFound if no row existed.
func DeleteAlarmLimit(ctx context.Context, db *sql.DB, tagID string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM alarm_limits WHERE tag_id = ?`, tagID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// rowScanner is shared with the rest of the store package (see profiles.go).

func scanAlarmLimit(row rowScanner) (*AlarmLimit, error) {
	var l AlarmLimit
	var enabled int
	var rat sql.NullString
	if err := row.Scan(&l.TagID, &enabled, &l.LoLo, &l.Lo, &l.Hi, &l.HiHi,
		&l.Severity, &l.RationalisedBy, &rat, &l.Notes); err != nil {
		return nil, err
	}
	l.Enabled = enabled == 1
	if rat.Valid {
		t := mustParse(rat.String)
		l.RationalisedAt = &t
	}
	return &l, nil
}
