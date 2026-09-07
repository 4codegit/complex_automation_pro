package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// MetaKeyActiveProfile is the meta row holding the active ore profile id.
const MetaKeyActiveProfile = "active_profile_id"

// ListProfiles returns all ore profiles newest-first.
func ListProfiles(ctx context.Context, db *sql.DB) ([]Profile, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, name, ore_domain, version, params, effective_from, effective_to,
			author, approved_by, approved_at, status, reason, created_at FROM profiles
		 ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProfiles(rows)
}

// GetProfile returns one profile by id.
func GetProfile(ctx context.Context, db *sql.DB, id string) (*Profile, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, name, ore_domain, version, params, effective_from, effective_to,
			author, approved_by, approved_at, status, reason, created_at FROM profiles
		 WHERE id = ?`, id)
	p, err := scanProfileRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// CreateProfile inserts a draft profile and returns its stored form.
func CreateProfile(ctx context.Context, db *sql.DB, p *Profile) (*Profile, error) {
	now := time.Now().UTC()
	p.CreatedAt = now
	if p.Status == "" {
		p.Status = ProfileStatusDraft
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO profiles
		 (id, name, ore_domain, version, params, effective_from, effective_to,
		  author, approved_by, approved_at, status, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.OreDomain, p.Version, p.Params,
		nullTime(p.EffectiveFrom), nullTime(p.EffectiveTo),
		p.Author, p.ApprovedBy, nullTime(p.ApprovedAt),
		p.Status, p.Reason, FormatUTC(p.CreatedAt))
	if err != nil {
		return nil, err
	}
	return p, nil
}

// ApproveProfile marks a draft profile approved by the given role/user.
func ApproveProfile(ctx context.Context, db *sql.DB, id, approvedBy string) error {
	now := time.Now().UTC()
	res, err := db.ExecContext(ctx,
		`UPDATE profiles SET status = ?, approved_by = ?, approved_at = ?
		 WHERE id = ? AND status = ?`,
		ProfileStatusApproved, approvedBy, FormatUTC(now), id, ProfileStatusDraft)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("profile %q is not in draft state", id)
	}
	return nil
}

// SetActiveProfile activates a profile and records it in the meta table.
// The previous active profile becomes approved (superseded by the new one).
func SetActiveProfile(ctx context.Context, db *sql.DB, id string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx,
		`UPDATE profiles SET status = ?, approved_at = COALESCE(approved_at, ?), reason = ? WHERE id = ?`,
		ProfileStatusActive, FormatUTC(now), "activated", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		MetaKeyActiveProfile, id); err != nil {
		return err
	}
	return tx.Commit()
}

// GetActiveProfile returns the currently active profile, or ErrNotFound when none.
func GetActiveProfile(ctx context.Context, db *sql.DB) (*Profile, error) {
	var id string
	err := db.QueryRowContext(ctx,
		`SELECT value FROM meta WHERE key = ?`, MetaKeyActiveProfile).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return GetProfile(ctx, db, id)
}

// seedProfiles defines the demo ore profiles shipped for development. Thresholds
// mirror the simulator baselines; switching the active profile changes what the
// demo generator treats as a breach.
func seedProfiles() []Profile {
	return []Profile{
		{
			ID: "ore-baseline-01", Name: "Baseline ore", OreDomain: "sulfide_copper",
			Version: 1, Author: "system",
			Params: `{"thresholds":{"particle_size":{"min":0,"max":12},"pulp_density":{"min":1.0,"max":2.5},"ph_level":{"min":7,"max":11},"reagent_dosage":{"min":10,"max":120},"cake_moisture":{"min":0,"max":12},"dryer_temperature":{"min":60,"max":250},"tonnage_weight":{"min":0,"max":500},"final_moisture":{"min":0,"max":10}}}`,
		},
		{
			ID: "ore-high-silica-01", Name: "High silica ore", OreDomain: "sulfide_copper",
			Version: 1, Author: "system",
			Params: `{"thresholds":{"particle_size":{"min":0,"max":9},"pulp_density":{"min":1.2,"max":2.3},"ph_level":{"min":8,"max":10.5},"reagent_dosage":{"min":20,"max":140},"cake_moisture":{"min":0,"max":9},"dryer_temperature":{"min":80,"max":230},"tonnage_weight":{"min":0,"max":400},"final_moisture":{"min":0,"max":8}},"baseline":{"reagent_dosage":70}}`,
		},
		{
			ID: "ore-low-grade-01", Name: "Low grade ore", OreDomain: "sulfide_copper",
			Version: 1, Author: "system",
			Params: `{"thresholds":{"particle_size":{"min":0,"max":14},"pulp_density":{"min":0.9,"max":2.6},"ph_level":{"min":6.5,"max":11.5},"reagent_dosage":{"min":10,"max":110},"cake_moisture":{"min":0,"max":13},"dryer_temperature":{"min":50,"max":260},"tonnage_weight":{"min":0,"max":520},"final_moisture":{"min":0,"max":11}},"baseline":{"tonnage_weight":280}}`,
		},
	}
}

// EnsureDefaultProfile seeds the demo ore profiles once (when none exist) and
// activates the baseline so the active-profile machinery has a real target.
func EnsureDefaultProfile(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for i := range seedProfiles() {
		p := seedProfiles()[i]
		_, err := CreateProfile(ctx, db, &p)
		if err != nil {
			return err
		}
	}
	return SetActiveProfile(ctx, db, "ore-baseline-01")
}

func scanProfiles(rows *sql.Rows) ([]Profile, error) {
	out := make([]Profile, 0)
	for rows.Next() {
		p, err := scanProfileRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanProfileRow(row rowScanner) (*Profile, error) {
	var p Profile
	var effFrom, effTo, approvedAt sql.NullString
	var approvedBy sql.NullString
	var created string
	if err := row.Scan(&p.ID, &p.Name, &p.OreDomain, &p.Version, &p.Params,
		&effFrom, &effTo, &p.Author, &approvedBy, &approvedAt,
		&p.Status, &p.Reason, &created); err != nil {
		return nil, err
	}
	if effFrom.Valid {
		t, _ := time.Parse(time.RFC3339Nano, effFrom.String)
		p.EffectiveFrom = &t
	}
	if effTo.Valid {
		t, _ := time.Parse(time.RFC3339Nano, effTo.String)
		p.EffectiveTo = &t
	}
	if approvedBy.Valid {
		p.ApprovedBy = &approvedBy.String
	}
	if approvedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, approvedAt.String)
		p.ApprovedAt = &t
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return nil, fmt.Errorf("parse created_at %q: %w", created, err)
	}
	p.CreatedAt = t
	return &p, nil
}
