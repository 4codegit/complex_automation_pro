package store

import (
	"context"
	"database/sql"
	"fmt"
)

// migrations holds portable DDL. Each entry applies exactly once, tracked in
// schema_migrations. The SQL is intentionally SQLite/PostgreSQL compatible:
// TEXT ids (no autoincrement), INTEGER booleans, TEXT timestamps in UTC.
var migrations = []string{
	`CREATE TABLE IF NOT EXISTS assets (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		area TEXT NOT NULL,
		criticality TEXT NOT NULL DEFAULT 'medium',
		active INTEGER NOT NULL DEFAULT 1
	)`,
	`CREATE TABLE IF NOT EXISTS tags (
		id TEXT PRIMARY KEY,
		asset_id TEXT NOT NULL,
		name TEXT NOT NULL,
		unit TEXT NOT NULL,
		data_type TEXT NOT NULL DEFAULT 'number',
		engineering_min REAL,
		engineering_max REAL,
		sampling_interval_seconds REAL NOT NULL DEFAULT 1.0,
		criticality TEXT NOT NULL DEFAULT 'medium',
		active INTEGER NOT NULL DEFAULT 1
	)`,
	`CREATE TABLE IF NOT EXISTS telemetry_readings (
		id TEXT PRIMARY KEY,
		message_id TEXT NOT NULL,
		gateway_id TEXT NOT NULL,
		source_sequence INTEGER,
		observed_at TEXT NOT NULL,
		received_at TEXT NOT NULL,
		sent_at TEXT,
		asset_id TEXT NOT NULL,
		tag_id TEXT NOT NULL,
		value_number REAL,
		value_bool INTEGER,
		value_string TEXT,
		value_structured TEXT,
		unit TEXT NOT NULL,
		quality TEXT NOT NULL,
		profile_id TEXT
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS uq_gateway_message ON telemetry_readings (gateway_id, message_id)`,
	`CREATE INDEX IF NOT EXISTS idx_readings_tag_time ON telemetry_readings (tag_id, observed_at)`,
	`CREATE TABLE IF NOT EXISTS alerts (
		id TEXT PRIMARY KEY,
		stage TEXT NOT NULL,
		metric TEXT NOT NULL,
		value REAL NOT NULL,
		threshold REAL NOT NULL,
		message TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS profiles (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		ore_domain TEXT NOT NULL,
		version INTEGER NOT NULL,
		params TEXT NOT NULL,
		effective_from TEXT,
		effective_to TEXT,
		author TEXT NOT NULL,
		approved_by TEXT,
		approved_at TEXT,
		status TEXT NOT NULL DEFAULT 'draft',
		reason TEXT,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS meta (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS alarms (
		id TEXT PRIMARY KEY,
		tag_id TEXT NOT NULL,
		metric TEXT NOT NULL,
		state TEXT NOT NULL,
		severity TEXT NOT NULL,
		priority INTEGER NOT NULL,
		message TEXT NOT NULL,
		observed_at TEXT NOT NULL,
		ack_by TEXT,
		ack_at TEXT,
		ack_comment TEXT,
		shelved_until TEXT,
		cleared_at TEXT,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS gateways (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		area TEXT NOT NULL,
		protocol TEXT NOT NULL DEFAULT 'push',
		status TEXT NOT NULL DEFAULT 'offline',
		last_seen_at TEXT,
		buffer_size INTEGER NOT NULL DEFAULT 0,
		version TEXT,
		config TEXT,
		created_at TEXT NOT NULL
	)`,
	// Audit trail: one immutable row per platform-affecting action. The actor
	// is supplied by the caller (header-based role assumption for the demo;
	// OIDC/JWT attach the real subject in production). This is the minimum
	// that lets a plant audit "who changed what, when", before RBAC hardens
	// the accept side of those mutations. See ARCHITECTURE_DECISIONS.md ADR-002.
	`CREATE TABLE IF NOT EXISTS audit_events (
		id TEXT PRIMARY KEY,
		occurred_at TEXT NOT NULL,
		actor TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL DEFAULT '',
		detail TEXT NOT NULL DEFAULT '',
		src_ip TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_audit_occurred ON audit_events (occurred_at)`,
	`CREATE INDEX IF NOT EXISTS idx_audit_resource ON audit_events (resource_type, resource_id)`,
	`CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_events (actor)`,
	// Alarm rationalisation limits: plant-approved per-tag configuration that
	// replaces any hard-coded universal thresholds. Rationalised limits are the
	// authoritative source the alarm engine and simulator read from.
	`CREATE TABLE IF NOT EXISTS alarm_limits (
		tag_id TEXT PRIMARY KEY,
		enabled INTEGER NOT NULL DEFAULT 1,
		lo_lo REAL,
		lo REAL,
		hi REAL,
		hi_hi REAL,
		severity TEXT NOT NULL DEFAULT 'medium',
		rationalised_by TEXT NOT NULL DEFAULT '',
		rationalised_at TEXT,
		notes TEXT NOT NULL DEFAULT ''
	)`,
	// RBAC: roles carry a permission set; subjects get one or more roles via
	// role_assignments. The X-User header carries the subject id; the server
	// resolves its roles at request time. This is the minimum that lets
	// mid-size plants enforce separation of duties before OIDC/JWT lands.
	`CREATE TABLE IF NOT EXISTS roles (
		id TEXT PRIMARY KEY,
		label TEXT NOT NULL,
		permissions TEXT NOT NULL DEFAULT '',
		system INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS role_assignments (
		id TEXT PRIMARY KEY,
		subject TEXT NOT NULL,
		role_id TEXT NOT NULL,
		assigned_by TEXT NOT NULL DEFAULT '',
		assigned_at TEXT NOT NULL,
		UNIQUE (subject, role_id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_role_assignments_subject ON role_assignments (subject)`,
}

// Migrate applies pending migrations in a transaction.
func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	var current int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("read migration version: %w", err)
	}

	for i := current; i < len(migrations); i++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version) VALUES (?)`, i+1); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
