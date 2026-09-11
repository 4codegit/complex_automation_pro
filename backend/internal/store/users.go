package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// User is a local login account (TZ §12). The password hash uses PBKDF2-HMAC-
// SHA256 in the form "pbkdf2-sha256$<iter>$<salt-b64>$<hash-b64>" (see
// internal/auth). Roles come from role_assignments, not from this table.
type User struct {
	Username     string    `json:"username"`
	Label        string    `json:"label"`
	PasswordHash string    `json:"-"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
}

// CreateUser inserts an account; the hash must already be encoded.
func CreateUser(ctx context.Context, db *sql.DB, u *User) error {
	if u.Username == "" {
		return errors.New("username is required")
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO users (username, label, password_hash, active, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		u.Username, u.Label, u.PasswordHash, boolToInt(u.Active), FormatUTC(time.Now().UTC()))
	return err
}

// CountUsers returns the number of accounts (seed guard).
func CountUsers(ctx context.Context, db *sql.DB) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// GetUserByName returns one account, or ErrNotFound.
func GetUserByName(ctx context.Context, db *sql.DB, username string) (*User, error) {
	row := db.QueryRowContext(ctx,
		`SELECT username, label, password_hash, active, created_at FROM users WHERE username = ?`, username)
	var u User
	var active int
	var created string
	if err := row.Scan(&u.Username, &u.Label, &u.PasswordHash, &active, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.Active = active == 1
	u.CreatedAt = mustParse(created)
	return &u, nil
}

// CreateSession stores a session keyed by the token hash. expiresAt is UTC.
func CreateSession(ctx context.Context, db *sql.DB, tokenHash, username string, ttl time.Duration, now time.Time) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, username, created_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		tokenHash, username, FormatUTC(now), FormatUTC(now.Add(ttl)))
	return err
}

// GetSessionUser resolves a live (non-expired) token hash to its username.
// Expired rows are treated as absent and pruned lazily.
func GetSessionUser(ctx context.Context, db *sql.DB, tokenHash string, now time.Time) (string, error) {
	var username, expires string
	err := db.QueryRowContext(ctx,
		`SELECT username, expires_at FROM sessions WHERE token_hash = ?`, tokenHash).
		Scan(&username, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if mustParse(expires).Before(now) {
		_, _ = db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
		return "", ErrNotFound
	}
	return username, nil
}

// DeleteSession removes one session (logout).
func DeleteSession(ctx context.Context, db *sql.DB, tokenHash string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}
