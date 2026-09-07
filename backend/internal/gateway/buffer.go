package gateway

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite, no CGO
)

// Pending is one buffered canonical message awaiting delivery.
type Pending struct {
	MessageID string
	Payload   string // canonical JSON
	QueuedAt  time.Time
}

// Buffer is the store-and-forward queue on the edge: a local SQLite file in
// WAL mode. It survives gateway restarts and holds messages until the server
// confirms them as accepted or duplicate.
type Buffer struct {
	db *sql.DB
}

// OpenBuffer opens (creating if needed) the local queue database.
func OpenBuffer(path string) (*Buffer, error) {
	if path == "" || path == ":memory:" {
		return openSQLite(":memory:")
	}
	return openSQLite(path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
}

func openSQLite(dsn string) (*Buffer, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open buffer: %w", err)
	}
	db.SetMaxOpenConns(4)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS pending (
			message_id TEXT PRIMARY KEY,
			payload TEXT NOT NULL,
			queued_at TEXT NOT NULL
		)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("init buffer: %w", err)
	}
	return &Buffer{db: db}, nil
}

// Enqueue stores one message. Duplicate message_ids are ignored (at-least-once
// semantics on the edge too).
func (b *Buffer) Enqueue(ctx context.Context, messageID, payload string) error {
	_, err := b.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO pending (message_id, payload, queued_at) VALUES (?, ?, ?)`,
		messageID, payload, time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z"))
	return err
}

// Snapshot returns up to limit messages oldest-first for the next delivery
// attempt. It does not remove them — removal is confirmation-driven.
func (b *Buffer) Snapshot(ctx context.Context, limit int) ([]Pending, error) {
	rows, err := b.db.QueryContext(ctx,
		`SELECT message_id, payload, queued_at FROM pending ORDER BY queued_at ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Pending, 0, limit)
	for rows.Next() {
		var p Pending
		var queued string
		if err := rows.Scan(&p.MessageID, &p.Payload, &queued); err != nil {
			return nil, err
		}
		p.QueuedAt, _ = time.Parse("2006-01-02T15:04:05.000000000Z", queued)
		out = append(out, p)
	}
	return out, rows.Err()
}

// Delete removes confirmed messages (accepted or duplicate) from the queue.
func (b *Buffer) Delete(ctx context.Context, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return nil
	}
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `DELETE FROM pending WHERE message_id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, id := range messageIDs {
		if _, err := stmt.ExecContext(ctx, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Len reports the number of undelivered messages.
func (b *Buffer) Len(ctx context.Context) (int64, error) {
	var n int64
	err := b.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pending`).Scan(&n)
	return n, err
}

// Close releases the queue database.
func (b *Buffer) Close() error { return b.db.Close() }
