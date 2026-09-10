package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Role groups a set of permissions. System roles (system=1) are seeded at
// startup and cannot be deleted through the API; their permissions can be
// edited by a platform_admin only.
type Role struct {
	ID          string    `json:"id"`
	Label       string    `json:"label"`
	Permissions []string  `json:"permissions"`
	System      bool      `json:"system"`
	CreatedAt   time.Time `json:"created_at"`
}

// RoleAssignment binds a subject (user id) to a role.
type RoleAssignment struct {
	ID         string    `json:"id"`
	Subject    string    `json:"subject"`
	RoleID     string    `json:"role_id"`
	AssignedBy string    `json:"assigned_by"`
	AssignedAt time.Time `json:"assigned_at"`
}

// System roles seeded in every install. The seeds mirror the matrix that
// ListRoles previously returned, so existing scripts and UI keep working when
// the table becomes the source of truth.
var systemRoles = []Role{
	{ID: "director", Label: "Director", Permissions: []string{"view_all", "view_reports", "approve_profiles"}, System: true},
	{ID: "metallurgist", Label: "Chief metallurgist", Permissions: []string{"view_all", "manage_profiles", "review_alarms", "rationalise_alarms"}, System: true},
	{ID: "operator", Label: "Operator", Permissions: []string{"view_assigned_area", "acknowledge_alarms", "control_process"}, System: true},
	{ID: "ot_engineer", Label: "OT engineer", Permissions: []string{"view_all", "manage_tags", "manage_gateways"}, System: true},
	{ID: "maintenance", Label: "Maintenance", Permissions: []string{"view_assets", "view_condition_signals"}, System: true},
	{ID: "platform_admin", Label: "Platform administrator", Permissions: []string{"manage_users", "manage_roles", "manage_platform", "view_all", "control_process"}, System: true},
}

// SeedRoles inserts the system roles when the table is empty, and bootstraps
// the initial platform_admin assignment to the subject "admin" so the first
// plant deployment can manage users and roles before OIDC/JWT attaches the
// real subject. The "admin" subject is rotated to a real plant identity via
// the access/assignments API after the first SSO bootstrap.
func SeedRoles(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM roles`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	for _, r := range systemRoles {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO roles (id, label, permissions, system, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			r.ID, r.Label, strings.Join(r.Permissions, ","), boolToInt(r.System),
			FormatUTC(now)); err != nil {
			return err
		}
	}
	// Bootstrap: admin -> platform_admin so the first deployment can grant
	// other subjects via /api/v1/access/assignments (manage_users).
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO role_assignments (id, subject, role_id, assigned_by, assigned_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(subject, role_id) DO NOTHING`,
		NewID(), "admin", "platform_admin", "seed", FormatUTC(now)); err != nil {
		return err
	}
	return tx.Commit()
}

// ListRoles returns all defined roles.
func ListRoles(ctx context.Context, db *sql.DB) ([]Role, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, label, permissions, system, created_at FROM roles ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Role, 0)
	for rows.Next() {
		var r Role
		var perms string
		var sys int
		var created string
		if err := rows.Scan(&r.ID, &r.Label, &perms, &sys, &created); err != nil {
			return nil, err
		}
		if perms != "" {
			r.Permissions = strings.Split(perms, ",")
		}
		r.System = sys == 1
		r.CreatedAt = mustParse(created)
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRole returns one role by id.
func GetRole(ctx context.Context, db *sql.DB, id string) (*Role, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, label, permissions, system, created_at FROM roles WHERE id = ?`, id)
	var r Role
	var perms string
	var sys int
	var created string
	if err := row.Scan(&r.ID, &r.Label, &perms, &sys, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if perms != "" {
		r.Permissions = strings.Split(perms, ",")
	}
	r.System = sys == 1
	r.CreatedAt = mustParse(created)
	return &r, nil
}

// CreateRole stores a custom (non-system) role. System roles cannot be created
// through this path — seed only.
func CreateRole(ctx context.Context, db *sql.DB, r *Role) error {
	if r.ID == "" || r.Label == "" {
		return errors.New("role id and label are required")
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO roles (id, label, permissions, system, created_at)
		 VALUES (?, ?, ?, 0, ?)`,
		r.ID, r.Label, strings.Join(r.Permissions, ","), FormatUTC(time.Now().UTC()))
	return err
}

// UpdateRolePermissions replaces the permission set of a role. System roles
// are editable only by a platform_admin (enforced at the handler layer).
func UpdateRolePermissions(ctx context.Context, db *sql.DB, id string, permissions []string) error {
	res, err := db.ExecContext(ctx,
		`UPDATE roles SET permissions = ? WHERE id = ?`, strings.Join(permissions, ","), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteRole removes a non-system role. System roles are protected: returns
// ErrConflict (the handler maps this to 409).
var ErrConflict = errors.New("conflict")

func DeleteRole(ctx context.Context, db *sql.DB, id string) error {
	var sys int
	if err := db.QueryRowContext(ctx, `SELECT system FROM roles WHERE id = ?`, id).Scan(&sys); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if sys == 1 {
		return ErrConflict
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM role_assignments WHERE role_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM roles WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// AssignRole grants role to subject; idempotent thanks to the UNIQUE
// constraint on (subject, role_id).
func AssignRole(ctx context.Context, db *sql.DB, subject, roleID, assignedBy string) error {
	if subject == "" || roleID == "" {
		return errors.New("subject and role_id are required")
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO role_assignments (id, subject, role_id, assigned_by, assigned_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(subject, role_id) DO NOTHING`,
		NewID(), subject, roleID, assignedBy, FormatUTC(time.Now().UTC()))
	return err
}

// RevokeRole removes a role from a subject.
func RevokeRole(ctx context.Context, db *sql.DB, subject, roleID string) error {
	res, err := db.ExecContext(ctx,
		`DELETE FROM role_assignments WHERE subject = ? AND role_id = ?`, subject, roleID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListAssignments returns role assignments optionally filtered by subject.
func ListAssignments(ctx context.Context, db *sql.DB, subject string) ([]RoleAssignment, error) {
	q := `SELECT id, subject, role_id, assigned_by, assigned_at FROM role_assignments`
	var args []any
	if subject != "" {
		q += ` WHERE subject = ?`
		args = append(args, subject)
	}
	q += ` ORDER BY assigned_at DESC`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RoleAssignment, 0)
	for rows.Next() {
		var a RoleAssignment
		var at string
		if err := rows.Scan(&a.ID, &a.Subject, &a.RoleID, &a.AssignedBy, &at); err != nil {
			return nil, err
		}
		a.AssignedAt = mustParse(at)
		out = append(out, a)
	}
	return out, rows.Err()
}

// SubjectHasPermission resolves whether a subject has a given permission
// through any of its assigned roles. Unknown subject or role → false. The
// "platform_admin" role implicitly carries every permission, so it short-
// circuits the lookup.
func SubjectHasPermission(ctx context.Context, db *sql.DB, subject, permission string) (bool, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT r.permissions, r.id
		 FROM role_assignments a JOIN roles r ON r.id = a.role_id
		 WHERE a.subject = ?`, subject)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var perms, id string
		if err := rows.Scan(&perms, &id); err != nil {
			return false, err
		}
		if id == "platform_admin" {
			return true, nil
		}
		if perms == "" {
			continue
		}
		for _, p := range strings.Split(perms, ",") {
			if strings.TrimSpace(p) == permission {
				return true, nil
			}
		}
	}
	return false, rows.Err()
}
