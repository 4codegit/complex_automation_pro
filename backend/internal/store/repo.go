package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound is returned when a registry row or latest value is absent.
var ErrNotFound = errors.New("not found")

// ErrDuplicate is returned when the (gateway_id, message_id) pair exists.
var ErrDuplicate = errors.New("duplicate message")

// SeedRegistry inserts demo assets/tags only when the tables are empty, so that
// development and tests have a predictable starting point. Live plants populate
// the registry through the change-controlled administrative workflow instead.
func SeedRegistry(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM assets`).Scan(&n); err != nil {
		return fmt.Errorf("count assets: %w", err)
	}
	if n > 0 {
		return nil
	}

	assets := []Asset{
		{ID: "plant-a.crushing", Name: "Crushing and grinding", Area: "crushing_grinding", Criticality: "high", Active: true},
		{ID: "plant-a.flotation", Name: "Flotation", Area: "flotation", Criticality: "high", Active: true},
		{ID: "plant-a.dewatering", Name: "Dewatering and drying", Area: "drying_dewatering", Criticality: "high", Active: true},
		{ID: "plant-a.concentrate", Name: "Final concentrate", Area: "final_concentrate", Criticality: "medium", Active: true},
	}
	tags := []Tag{
		{ID: "plant-a.crushing.particle_size", AssetID: "plant-a.crushing", Name: "Particle size", Unit: "mm", DataType: "number", SamplingIntervalSeconds: 1.0, Criticality: "high", Active: true},
		{ID: "plant-a.crushing.pulp_density", AssetID: "plant-a.crushing", Name: "Pulp density", Unit: "g/cm3", DataType: "number", SamplingIntervalSeconds: 2.0, Criticality: "high", Active: true},
		{ID: "plant-a.flotation.ph_level", AssetID: "plant-a.flotation", Name: "Pulp pH", Unit: "pH", DataType: "number", SamplingIntervalSeconds: 1.0, Criticality: "high", Active: true},
		{ID: "plant-a.flotation.reagent_dosage", AssetID: "plant-a.flotation", Name: "Reagent dosage", Unit: "mL/min", DataType: "number", SamplingIntervalSeconds: 1.0, Criticality: "medium", Active: true},
		{ID: "plant-a.dewatering.cake_moisture", AssetID: "plant-a.dewatering", Name: "Cake moisture", Unit: "%", DataType: "number", SamplingIntervalSeconds: 1.0, Criticality: "high", Active: true},
		{ID: "plant-a.dewatering.dryer_temperature", AssetID: "plant-a.dewatering", Name: "Dryer temperature", Unit: "C", DataType: "number", SamplingIntervalSeconds: 1.0, Criticality: "high", Active: true},
		{ID: "plant-a.concentrate.tonnage_weight", AssetID: "plant-a.concentrate", Name: "Concentrate throughput", Unit: "t/h", DataType: "number", SamplingIntervalSeconds: 1.0, Criticality: "medium", Active: true},
		{ID: "plant-a.concentrate.final_moisture", AssetID: "plant-a.concentrate", Name: "Final moisture", Unit: "%", DataType: "number", SamplingIntervalSeconds: 1.0, Criticality: "high", Active: true},
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, a := range assets {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO assets (id, name, area, criticality, active) VALUES (?, ?, ?, ?, ?)`,
			a.ID, a.Name, a.Area, a.Criticality, boolToInt(a.Active)); err != nil {
			return fmt.Errorf("seed asset %s: %w", a.ID, err)
		}
	}
	for _, t := range tags {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tags (id, asset_id, name, unit, data_type, sampling_interval_seconds, criticality, active)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID, t.AssetID, t.Name, t.Unit, t.DataType, t.SamplingIntervalSeconds, t.Criticality, boolToInt(t.Active)); err != nil {
			return fmt.Errorf("seed tag %s: %w", t.ID, err)
		}
	}
	return tx.Commit()
}

// ListAssets returns the equipment tree ordered by area and name.
func ListAssets(ctx context.Context, db *sql.DB) ([]Asset, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, name, area, criticality, active FROM assets ORDER BY area, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Asset, 0)
	for rows.Next() {
		var a Asset
		var active int
		if err := rows.Scan(&a.ID, &a.Name, &a.Area, &a.Criticality, &active); err != nil {
			return nil, err
		}
		a.Active = active == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListTags returns registered tags, optionally filtered by asset.
func ListTags(ctx context.Context, db *sql.DB, assetID string) ([]Tag, error) {
	q := `SELECT id, asset_id, name, unit, data_type, engineering_min, engineering_max,
		sampling_interval_seconds, criticality, active FROM tags`
	var args []any
	if assetID != "" {
		q += ` WHERE asset_id = ?`
		args = append(args, assetID)
	}
	q += ` ORDER BY asset_id, id`

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Tag, 0)
	for rows.Next() {
		var t Tag
		var active int
		if err := rows.Scan(&t.ID, &t.AssetID, &t.Name, &t.Unit, &t.DataType,
			&t.EngineeringMin, &t.EngineeringMax, &t.SamplingIntervalSeconds,
			&t.Criticality, &active); err != nil {
			return nil, err
		}
		t.Active = active == 1
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTag returns a single tag by id, or ErrNotFound.
func GetTag(ctx context.Context, db *sql.DB, id string) (*Tag, error) {
	var t Tag
	var active int
	err := db.QueryRowContext(ctx,
		`SELECT id, asset_id, name, unit, data_type, engineering_min, engineering_max,
			sampling_interval_seconds, criticality, active FROM tags WHERE id = ?`, id).
		Scan(&t.ID, &t.AssetID, &t.Name, &t.Unit, &t.DataType,
			&t.EngineeringMin, &t.EngineeringMax, &t.SamplingIntervalSeconds,
			&t.Criticality, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.Active = active == 1
	return &t, nil
}

// InsertReading stores a reading if its (gateway_id, message_id) is new.
// It returns ErrDuplicate when the pair already exists.
func InsertReading(ctx context.Context, db *sql.DB, r *Reading) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var one int
	if err := tx.QueryRowContext(ctx,
		`SELECT 1 FROM telemetry_readings WHERE gateway_id = ? AND message_id = ?`,
		r.GatewayID, r.MessageID).Scan(&one); err == nil {
		return ErrDuplicate
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO telemetry_readings
		 (id, message_id, gateway_id, source_sequence, observed_at, received_at, sent_at,
		  asset_id, tag_id, value_number, value_bool, value_string, value_structured,
		  unit, quality, profile_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.MessageID, r.GatewayID, r.SourceSequence,
		FormatUTC(r.ObservedAt), FormatUTC(r.ReceivedAt), nullTime(r.SentAt),
		r.AssetID, r.TagID, r.ValueNumber, r.ValueBool, r.ValueString, r.ValueStructured,
		r.Unit, r.Quality, r.ProfileID); err != nil {
		return err
	}
	return tx.Commit()
}

// ReadingFilter narrows a history query.
type ReadingFilter struct {
	TagID   string
	AssetID string
	From    *time.Time
	To      *time.Time
	Limit   int
}

// QueryReadings returns readings in descending observed_at order.
func QueryReadings(ctx context.Context, db *sql.DB, f ReadingFilter) ([]Reading, error) {
	q := `SELECT id, message_id, gateway_id, source_sequence, observed_at, received_at, sent_at,
		asset_id, tag_id, value_number, value_bool, value_string, value_structured,
		unit, quality, profile_id FROM telemetry_readings`
	var conds []string
	var args []any
	if f.TagID != "" {
		conds = append(conds, "tag_id = ?")
		args = append(args, f.TagID)
	}
	if f.AssetID != "" {
		conds = append(conds, "asset_id = ?")
		args = append(args, f.AssetID)
	}
	if f.From != nil {
		conds = append(conds, "observed_at >= ?")
		args = append(args, FormatUTC(*f.From))
	}
	if f.To != nil {
		conds = append(conds, "observed_at <= ?")
		args = append(args, FormatUTC(*f.To))
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY observed_at DESC LIMIT ?"
	args = append(args, f.Limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReadings(rows)
}

// LatestReading returns the newest reading for a tag, or ErrNotFound.
func LatestReading(ctx context.Context, db *sql.DB, tagID string) (*Reading, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, message_id, gateway_id, source_sequence, observed_at, received_at, sent_at,
			asset_id, tag_id, value_number, value_bool, value_string, value_structured,
			unit, quality, profile_id FROM telemetry_readings
		 WHERE tag_id = ? ORDER BY observed_at DESC LIMIT 1`, tagID)
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

func scanReadings(rows *sql.Rows) ([]Reading, error) {
	var out []Reading
	for rows.Next() {
		var r Reading
		var obs, rec string
		var sent sql.NullString
		if err := rows.Scan(&r.ID, &r.MessageID, &r.GatewayID, &r.SourceSequence,
			&obs, &rec, &sent, &r.AssetID, &r.TagID,
			&r.ValueNumber, &r.ValueBool, &r.ValueString, &r.ValueStructured,
			&r.Unit, &r.Quality, &r.ProfileID); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, obs)
		if err != nil {
			return nil, fmt.Errorf("parse observed_at %q: %w", obs, err)
		}
		r.ObservedAt = t
		t, err = time.Parse(time.RFC3339Nano, rec)
		if err != nil {
			return nil, fmt.Errorf("parse received_at %q: %w", rec, err)
		}
		r.ReceivedAt = t
		if sent.Valid {
			st, err := time.Parse(time.RFC3339Nano, sent.String)
			if err != nil {
				return nil, fmt.Errorf("parse sent_at %q: %w", sent.String, err)
			}
			r.SentAt = &st
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// InsertAlert persists an alert row.
func InsertAlert(ctx context.Context, db *sql.DB, a *Alert) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO alerts (id, stage, metric, value, threshold, message, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Stage, a.Metric, a.Value, a.Threshold, a.Message, FormatUTC(a.CreatedAt))
	return err
}

// ListAlerts returns alerts newest-first with optional stage/metric filters.
func ListAlerts(ctx context.Context, db *sql.DB, stage, metric string, limit int) ([]Alert, error) {
	q := `SELECT id, stage, metric, value, threshold, message, created_at FROM alerts`
	var conds []string
	var args []any
	if stage != "" {
		conds = append(conds, "stage = ?")
		args = append(args, stage)
	}
	if metric != "" {
		conds = append(conds, "metric = ?")
		args = append(args, metric)
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Alert, 0)
	for rows.Next() {
		var a Alert
		var created string
		if err := rows.Scan(&a.ID, &a.Stage, &a.Metric, &a.Value, &a.Threshold, &a.Message, &created); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, fmt.Errorf("parse created_at %q: %w", created, err)
		}
		a.CreatedAt = t
		out = append(out, a)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return FormatUTC(*t)
}

// CreateAsset inserts a new asset. Returns an error on duplicate ID.
func CreateAsset(ctx context.Context, db *sql.DB, a *Asset) error {
	a.ID = strings.TrimSpace(a.ID)
	if a.ID == "" {
		a.ID = NewID()
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO assets (id, name, area, criticality, active) VALUES (?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Area, a.Criticality, boolToInt(a.Active)); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			return fmt.Errorf("asset %q already exists", a.ID)
		}
		return err
	}
	return nil
}

// UpdateAsset overwrites name, area, criticality and active for an existing asset.
func UpdateAsset(ctx context.Context, db *sql.DB, a *Asset) error {
	res, err := db.ExecContext(ctx,
		`UPDATE assets SET name=?, area=?, criticality=?, active=? WHERE id=?`,
		a.Name, a.Area, a.Criticality, boolToInt(a.Active), a.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAsset removes an asset if no tags reference it.
func DeleteAsset(ctx context.Context, db *sql.DB, id string) error {
	var cnt int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tags WHERE asset_id=?`, id).Scan(&cnt); err != nil {
		return err
	}
	if cnt > 0 {
		return fmt.Errorf("cannot delete asset %q: %d tag(s) still reference it", id, cnt)
	}
	res, err := db.ExecContext(ctx, `DELETE FROM assets WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateTag inserts a new tag. Returns an error on duplicate ID.
func CreateTag(ctx context.Context, db *sql.DB, t *Tag) error {
	t.ID = strings.TrimSpace(t.ID)
	if t.ID == "" {
		t.ID = NewID()
	}
	// Verify the referenced asset exists.
	var a string
	if err := db.QueryRowContext(ctx, `SELECT id FROM assets WHERE id=?`, t.AssetID).Scan(&a); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("asset %q does not exist", t.AssetID)
		}
		return err
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO tags (id, asset_id, name, unit, data_type, engineering_min, engineering_max,
		 sampling_interval_seconds, criticality, active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.AssetID, t.Name, t.Unit, t.DataType,
		t.EngineeringMin, t.EngineeringMax, t.SamplingIntervalSeconds,
		t.Criticality, boolToInt(t.Active)); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			return fmt.Errorf("tag %q already exists", t.ID)
		}
		return err
	}
	return nil
}

// UpdateTag overwrites mutable fields for an existing tag.
func UpdateTag(ctx context.Context, db *sql.DB, t *Tag) error {
	res, err := db.ExecContext(ctx,
		`UPDATE tags SET asset_id=?, name=?, unit=?, data_type=?,
		 engineering_min=?, engineering_max=?, sampling_interval_seconds=?,
		 criticality=?, active=? WHERE id=?`,
		t.AssetID, t.Name, t.Unit, t.DataType,
		t.EngineeringMin, t.EngineeringMax, t.SamplingIntervalSeconds,
		t.Criticality, boolToInt(t.Active), t.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTag removes a tag if no telemetry readings reference it.
func DeleteTag(ctx context.Context, db *sql.DB, id string) error {
	var cnt int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM telemetry_readings WHERE tag_id=?`, id).Scan(&cnt); err != nil {
		return err
	}
	if cnt > 0 {
		return fmt.Errorf("tag %q has %d historical readings; cannot delete", id, cnt)
	}
	res, err := db.ExecContext(ctx, `DELETE FROM tags WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
