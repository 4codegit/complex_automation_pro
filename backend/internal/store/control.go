package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Supervisory control persistence (TZ §9). One control_loops row per loop;
// the PID runs server-side in percent of the MV range and the edge bridge
// maps the output to the actuator register. ErrNotFound (repo.go) is reused.

// Control loop modes and states.
const (
	LoopModeAuto   = "auto"
	LoopModeManual = "manual"

	LoopStateOK       = "ok"
	LoopStateWatchdog = "watchdog"
)

// ControlLoop is one supervisory loop: PV tag, MV (actuator) tag, setpoint
// window and tuning. Output is stored in engineering (MV) units.
type ControlLoop struct {
	ID        string     `json:"id"`
	Label     string     `json:"label"`
	PVTag     string     `json:"pv_tag"`
	MVTag     string     `json:"mv_tag"`
	SP        float64    `json:"sp"`
	SPMin     float64    `json:"sp_min"`
	SPMax     float64    `json:"sp_max"`
	OutMin    float64    `json:"out_min"`
	OutMax    float64    `json:"out_max"`
	Kp        float64    `json:"kp"`
	Ki        float64    `json:"ki"`
	Kd        float64    `json:"kd"`
	Deadband  float64    `json:"deadband"`
	Slew      float64    `json:"slew"`
	Mode      string     `json:"mode"`
	State     string     `json:"state"`
	Output    float64    `json:"output"`
	Integral  float64    `json:"-"`
	PrevError *float64   `json:"-"`
	UpdatedBy string     `json:"updated_by"`
	UpdatedAt *time.Time `json:"updated_at"`
}

const loopSelect = `SELECT id, label, pv_tag, mv_tag, sp, sp_min, sp_max, out_min, out_max,
	kp, ki, kd, deadband, slew, mode, state, output, integral, prev_error, updated_by, updated_at
	FROM control_loops`

func scanLoop(row rowScanner) (*ControlLoop, error) {
	var l ControlLoop
	var prev sql.NullFloat64
	var upd sql.NullString
	if err := row.Scan(&l.ID, &l.Label, &l.PVTag, &l.MVTag, &l.SP, &l.SPMin, &l.SPMax,
		&l.OutMin, &l.OutMax, &l.Kp, &l.Ki, &l.Kd, &l.Deadband, &l.Slew,
		&l.Mode, &l.State, &l.Output, &l.Integral, &prev, &l.UpdatedBy, &upd); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if prev.Valid {
		l.PrevError = &prev.Float64
	}
	if upd.Valid {
		t := mustParse(upd.String)
		l.UpdatedAt = &t
	}
	return &l, nil
}

// ListLoops returns all control loops ordered by id.
func ListLoops(ctx context.Context, db *sql.DB) ([]ControlLoop, error) {
	rows, err := db.QueryContext(ctx, loopSelect+` ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ControlLoop, 0)
	for rows.Next() {
		l, err := scanLoop(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// GetLoop returns one control loop by id, or ErrNotFound.
func GetLoop(ctx context.Context, db *sql.DB, id string) (*ControlLoop, error) {
	return scanLoop(db.QueryRowContext(ctx, loopSelect+` WHERE id = ?`, id))
}

// UpdateLoopSetpoint stores the operator-approved setpoint.
func UpdateLoopSetpoint(ctx context.Context, db *sql.DB, id string, sp float64, by string) error {
	res, err := db.ExecContext(ctx,
		`UPDATE control_loops SET sp = ?, updated_by = ?, updated_at = ? WHERE id = ?`,
		sp, by, FormatUTC(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateLoopMode switches the loop mode. Moving into auto performs the
// bumpless transfer: the integral is seeded so the PID resumes from the
// current output instead of stepping the actuator.
func UpdateLoopMode(ctx context.Context, db *sql.DB, id, mode string, by string) error {
	seed, err := bumplessIntegralSeed(ctx, db, id, mode)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx,
		`UPDATE control_loops SET mode = ?, integral = ?, prev_error = NULL, state = ?,
		 updated_by = ?, updated_at = ? WHERE id = ?`,
		mode, seed, LoopStateOK, by, FormatUTC(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// bumplessIntegralSeed computes the integral term that makes the PID output
// start from the current actuator position. The field position (the MV tag's
// own telemetry) is authoritative; the stored output is the fallback when no
// feedback has arrived yet.
func bumplessIntegralSeed(ctx context.Context, db *sql.DB, id, mode string) (float64, error) {
	if mode != LoopModeAuto {
		return 0, nil
	}
	l, err := GetLoop(ctx, db, id)
	if err != nil {
		return 0, err
	}
	span := l.OutMax - l.OutMin
	if span <= 0 {
		return 0, nil
	}
	pct := (l.Output - l.OutMin) / span * 100
	if r, err := LatestGoodNumericReading(ctx, db, l.MVTag); err == nil && r.ValueNumber != nil {
		pct = (*r.ValueNumber - l.OutMin) / span * 100
	}
	if pct < -50 {
		pct = -50
	}
	if pct > 100 {
		pct = 100
	}
	return pct, nil
}

// UpdateLoopOutput stores a manual output (engineering units, clamped by the
// caller to the loop range). Only meaningful in manual mode.
func UpdateLoopOutput(ctx context.Context, db *sql.DB, id string, out float64, by string) error {
	res, err := db.ExecContext(ctx,
		`UPDATE control_loops SET output = ?, state = ?, updated_by = ?, updated_at = ? WHERE id = ?`,
		out, LoopStateOK, by, FormatUTC(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SaveLoopRuntime persists the per-tick loop computation: output in
// engineering units plus the PID memory. mode/state describe the loop
// condition the edge bridge acts on ("auto" only drives the actuator).
func SaveLoopRuntime(ctx context.Context, db *sql.DB, id string, output float64, mode, state string, integral float64, prevError float64) error {
	_, err := db.ExecContext(ctx,
		`UPDATE control_loops SET output = ?, mode = ?, state = ?, integral = ?, prev_error = ?,
		 updated_at = ? WHERE id = ?`,
		output, mode, state, integral, prevError, FormatUTC(time.Now().UTC()), id)
	return err
}

// SeedControlLoops inserts the three flotation plant loops (TZ §9) when the
// table is empty. Tuning values are the starting point confirmed against the
// process stand (acceptance: hold SP within the §17 bands).
func SeedControlLoops(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM control_loops`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	loops := []ControlLoop{
		{ID: "lic301", Label: "Уровень пульпы флотомашины", PVTag: "plant.flotation.li301", MVTag: "plant.flotation.lc301",
			SP: 500, SPMin: 450, SPMax: 650, OutMin: 0, OutMax: 100,
			Kp: -0.06, Ki: -0.02, Kd: 0, Deadband: 2, Slew: 5, Mode: LoopModeManual, State: LoopStateOK},
		{ID: "fic301", Label: "Удельный расход собирателя", PVTag: "plant.flotation.qi301", MVTag: "plant.flotation.fc301",
			SP: 108, SPMin: 30, SPMax: 300, OutMin: 0, OutMax: 500,
			Kp: -1.2, Ki: -0.4, Kd: 0, Deadband: 3, Slew: 5, Mode: LoopModeManual, State: LoopStateOK},
		{ID: "dic401", Label: "Плотность сгущённого продукта", PVTag: "plant.thickening.di401", MVTag: "plant.thickening.fc401",
			SP: 45, SPMin: 40, SPMax: 55, OutMin: 10, OutMax: 90,
			Kp: -2.5, Ki: -0.8, Kd: 0, Deadband: 0.3, Slew: 5, Mode: LoopModeManual, State: LoopStateOK},
	}
	now := time.Now().UTC()
	for _, l := range loops {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO control_loops
			 (id, label, pv_tag, mv_tag, sp, sp_min, sp_max, out_min, out_max,
			  kp, ki, kd, deadband, slew, mode, state, output, updated_by, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			l.ID, l.Label, l.PVTag, l.MVTag, l.SP, l.SPMin, l.SPMax, l.OutMin, l.OutMax,
			l.Kp, l.Ki, l.Kd, l.Deadband, l.Slew, l.Mode, l.State, l.Output, "seed", FormatUTC(now)); err != nil {
			return err
		}
	}
	return nil
}

// ActuatorWrite is a one-shot manual actuator command (TZ §9). seq increases
// with every accepted write; the edge bridge writes each new seq once.
type ActuatorWrite struct {
	TagID     string    `json:"tag_id"`
	Value     float64   `json:"value"`
	Seq       int64     `json:"seq"`
	UpdatedBy string    `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpsertActuatorWrite records a manual write request, bumping the sequence.
func UpsertActuatorWrite(ctx context.Context, db *sql.DB, tagID string, value float64, by string) (*ActuatorWrite, error) {
	var seq int64
	err := db.QueryRowContext(ctx,
		`INSERT INTO actuator_writes (tag_id, value, seq, updated_by, updated_at)
		 VALUES (?, ?, 1, ?, ?)
		 ON CONFLICT(tag_id) DO UPDATE SET value = excluded.value,
		   seq = actuator_writes.seq + 1, updated_by = excluded.updated_by,
		   updated_at = excluded.updated_at
		 RETURNING seq`,
		tagID, value, by, FormatUTC(time.Now().UTC())).Scan(&seq)
	if err != nil {
		// Postgres-compatible path: RETURNING is supported by SQLite >= 3.35
		// and PostgreSQL; any failure here is a real storage error.
		return nil, err
	}
	return &ActuatorWrite{TagID: tagID, Value: value, Seq: seq, UpdatedBy: by, UpdatedAt: time.Now().UTC()}, nil
}

// ListActuatorWrites returns all recorded manual writes.
func ListActuatorWrites(ctx context.Context, db *sql.DB) ([]ActuatorWrite, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT tag_id, value, seq, updated_by, updated_at FROM actuator_writes ORDER BY tag_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ActuatorWrite, 0)
	for rows.Next() {
		var w ActuatorWrite
		var upd string
		if err := rows.Scan(&w.TagID, &w.Value, &w.Seq, &w.UpdatedBy, &upd); err != nil {
			return nil, err
		}
		w.UpdatedAt = mustParse(upd)
		out = append(out, w)
	}
	return out, rows.Err()
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
