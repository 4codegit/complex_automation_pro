package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// UpsertAlarmActive ensures an active alarm exists for a tag. If one is already
// active it refreshes it; otherwise it inserts a new active_unacknowledged row.
func UpsertAlarmActive(ctx context.Context, db *sql.DB, tagID, metric, severity, message string, now time.Time) error {
	existing, err := GetAlarmByTag(ctx, db, tagID)
	if err == nil {
		if isActiveState(existing.State) {
			_, err := db.ExecContext(ctx,
				`UPDATE alarms SET updated_at = ?, message = ? WHERE id = ?`,
				FormatUTC(now), message, existing.ID)
			return err
		}
		// A cleared alarm re-fires: reuse the row but reset state.
		_, err := db.ExecContext(ctx,
			`UPDATE alarms SET state = ?, severity = ?, message = ?, observed_at = ?,
				ack_by = NULL, ack_at = NULL, ack_comment = NULL, cleared_at = NULL, updated_at = ?
			 WHERE id = ?`,
			AlarmStateActiveUnack, severity, message, FormatUTC(now), FormatUTC(now), existing.ID)
		return err
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}

	_, err = db.ExecContext(ctx,
		`INSERT INTO alarms
		 (id, tag_id, metric, state, severity, priority, message, observed_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		NewID(), tagID, metric, AlarmStateActiveUnack, severity, severityPriority(severity),
		message, FormatUTC(now), FormatUTC(now))
	return err
}

// ClearAlarm marks an active alarm returned_to_normal.
func ClearAlarm(ctx context.Context, db *sql.DB, tagID string, now time.Time) error {
	existing, err := GetAlarmByTag(ctx, db, tagID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !isActiveState(existing.State) {
		return nil
	}
	_, err = db.ExecContext(ctx,
		`UPDATE alarms SET state = ?, cleared_at = ?, updated_at = ? WHERE id = ?`,
		AlarmStateNormal, FormatUTC(now), FormatUTC(now), existing.ID)
	return err
}

// AckAlarm acknowledges an unacknowledged active alarm. It is a human action and
// is recorded immutably on the row plus as an alert event by the caller.
func AckAlarm(ctx context.Context, db *sql.DB, id, by, comment string, now time.Time) error {
	res, err := db.ExecContext(ctx,
		`UPDATE alarms SET state = ?, ack_by = ?, ack_comment = ?, ack_at = ?, updated_at = ?
		 WHERE id = ? AND state = ?`,
		AlarmStateActiveAck, by, comment, FormatUTC(now), FormatUTC(now),
		id, AlarmStateActiveUnack)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("alarm %q is not active_unacknowledged", id)
	}
	return nil
}

// GetAlarm returns one alarm by id.
func GetAlarm(ctx context.Context, db *sql.DB, id string) (*Alarm, error) {
	a, err := scanAlarm(db.QueryRowContext(ctx, alarmSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// ListAlarms returns alarms newest-first, optionally filtered by state.
func ListAlarms(ctx context.Context, db *sql.DB, state string, limit int) ([]Alarm, error) {
	q := alarmSelect
	var args []any
	if state != "" {
		q += ` WHERE state = ?`
		args = append(args, state)
	}
	q += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Alarm, 0)
	for rows.Next() {
		a, err := scanAlarm(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// ListActiveAlarms returns alarms in an active lifecycle state (unacknowledged,
// acknowledged or shelved), newest-first.
func ListActiveAlarms(ctx context.Context, db *sql.DB, limit int) ([]Alarm, error) {
	rows, err := db.QueryContext(ctx,
		alarmSelect+` WHERE state IN (?, ?, ?) ORDER BY updated_at DESC LIMIT ?`,
		AlarmStateActiveUnack, AlarmStateActiveAck, AlarmStateShelved, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Alarm, 0)
	for rows.Next() {
		a, err := scanAlarm(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

const alarmSelect = `SELECT id, tag_id, metric, state, severity, priority, message,
	observed_at, ack_by, ack_at, ack_comment, shelved_until, cleared_at, updated_at FROM alarms`

// GetAlarmByTag returns the current alarm row for a tag, or ErrNotFound.
func GetAlarmByTag(ctx context.Context, db *sql.DB, tagID string) (*Alarm, error) {
	a, err := scanAlarm(db.QueryRowContext(ctx, alarmSelect+` WHERE tag_id = ?`, tagID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func scanAlarm(row rowScanner) (*Alarm, error) {
	var a Alarm
	var obs, upd string
	var ackBy, ackComment sql.NullString
	var ackAt, shelved, cleared sql.NullString
	if err := row.Scan(&a.ID, &a.TagID, &a.Metric, &a.State, &a.Severity, &a.Priority,
		&a.Message, &obs, &ackBy, &ackAt, &ackComment, &shelved, &cleared, &upd); err != nil {
		return nil, err
	}
	a.ObservedAt = mustParse(obs)
	a.UpdatedAt = mustParse(upd)
	if ackBy.Valid {
		a.AckBy = &ackBy.String
	}
	if ackComment.Valid {
		a.AckComment = &ackComment.String
	}
	if ackAt.Valid {
		t := mustParse(ackAt.String)
		a.AckAt = &t
	}
	if shelved.Valid {
		t := mustParse(shelved.String)
		a.ShelvedUntil = &t
	}
	if cleared.Valid {
		t := mustParse(cleared.String)
		a.ClearedAt = &t
	}
	return &a, nil
}

func mustParse(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func isActiveState(state string) bool {
	return state == AlarmStateActiveUnack || state == AlarmStateActiveAck || state == AlarmStateShelved
}

func severityPriority(sev string) int {
	switch strings.ToLower(sev) {
	case "critical":
		return 1
	case "high":
		return 2
	case "medium":
		return 3
	default:
		return 4
	}
}
