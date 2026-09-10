package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Supervisory control persistence (ADR-003): operator setpoints and the
// computed loop output. The setpoint row is the human decision; the state row
// is what the edge gateway translates into an actuator write. Neither table
// grants control by itself — the loop only runs when explicitly enabled.
// ErrNotFound (repo.go) is reused for missing rows.

type Setpoint struct {
	TagID     string
	Value     float64
	UpdatedBy string
	UpdatedAt time.Time
}

// GetSetpoint returns the stored setpoint for a tag.
func GetSetpoint(ctx context.Context, db *sql.DB, tagID string) (*Setpoint, error) {
	row := db.QueryRowContext(ctx,
		`SELECT value, updated_by, updated_at FROM control_setpoints WHERE tag_id = ?`, tagID)
	var sp Setpoint
	var updatedAt string
	if err := row.Scan(&sp.Value, &sp.UpdatedBy, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	sp.UpdatedAt = parseUTCOrZero(updatedAt)
	sp.TagID = tagID
	return &sp, nil
}

// SetSetpoint upserts the operator-approved setpoint.
func SetSetpoint(ctx context.Context, db *sql.DB, tagID string, value float64, updatedBy string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO control_setpoints (tag_id, value, updated_by, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(tag_id) DO UPDATE SET value = excluded.value,
		   updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		tagID, value, updatedBy, FormatUTC(time.Now().UTC()))
	return err
}

type LoopState struct {
	TagID     string
	Output    float64
	Status    string // off | auto | paused_watchdog
	UpdatedAt time.Time
}

// GetLoopState returns the last computed loop output for a tag.
func GetLoopState(ctx context.Context, db *sql.DB, tagID string) (*LoopState, error) {
	row := db.QueryRowContext(ctx,
		`SELECT output, status, updated_at FROM control_state WHERE tag_id = ?`, tagID)
	var st LoopState
	var updatedAt string
	if err := row.Scan(&st.Output, &st.Status, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	st.UpdatedAt = parseUTCOrZero(updatedAt)
	st.TagID = tagID
	return &st, nil
}

// SaveLoopState persists the computed output for the actuator bridge.
func SaveLoopState(ctx context.Context, db *sql.DB, tagID string, output float64, status string, integral, prevError float64) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO control_state (tag_id, output, status, integral, prev_error, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(tag_id) DO UPDATE SET output = excluded.output,
		   status = excluded.status, integral = excluded.integral,
		   prev_error = excluded.prev_error, updated_at = excluded.updated_at`,
		tagID, output, status, integral, prevError, FormatUTC(time.Now().UTC()))
	return err
}

// LatestGoodNumericReading returns the newest numeric reading of a tag whose
// quality is not a gap (offline/stale): control must never react to a
// placeholder zero recorded during a device outage.
func LatestGoodNumericReading(ctx context.Context, db *sql.DB, tagID string) (*Reading, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, message_id, gateway_id, source_sequence, observed_at, received_at, sent_at,
			asset_id, tag_id, value_number, value_bool, value_string, value_structured,
			unit, quality, profile_id FROM telemetry_readings
		 WHERE tag_id = ? AND quality NOT IN ('offline', 'stale') AND value_number IS NOT NULL
		 ORDER BY observed_at DESC, received_at DESC LIMIT 1`, tagID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rs, err := scanReadings(rows)
	if err != nil {
		return nil, err
	}
	if len(rs) == 0 {
		return nil, ErrNotFound
	}
	return &rs[0], nil
}

func parseUTCOrZero(s string) time.Time {
	t, err := time.Parse("2006-01-02T15:04:05.000000000Z", s)
	if err != nil {
		return time.Time{}
	}
	return t
}
