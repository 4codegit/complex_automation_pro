package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // postgres driver (database/sql)
	_ "modernc.org/sqlite"             // pure-Go sqlite driver, no CGO
)

// Open opens a database using the driver selected by the DSN prefix.
//   - sqlite://./cap.db  -> pure-Go SQLite (development, demo, tests)
//   - postgres://...         -> PostgreSQL (production)
//
// It configures sane connection limits and verifies connectivity.
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	driver, source, err := parseDSN(dsn)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open(driver, source)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", driver, err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping %s: %w", driver, err)
	}
	return db, nil
}

func parseDSN(dsn string) (driver, source string, err error) {
	switch {
	case strings.HasPrefix(dsn, "sqlite://"):
		src := strings.TrimPrefix(dsn, "sqlite://")
		if src == "" || src == ":memory:" {
			src = ":memory:"
		} else {
			sep := "?"
			if strings.Contains(src, "?") {
				sep = "&"
			}
			src += sep + "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
		}
		return "sqlite", src, nil
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		return "pgx", dsn, nil
	default:
		return "", "", fmt.Errorf("unsupported DB_URL %q (use sqlite:// or postgres://)", dsn)
	}
}

// FormatUTC renders a time as fixed-width UTC for lexicographically ordered
// storage, portable across SQLite and PostgreSQL TEXT columns.
func FormatUTC(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000000Z")
}
