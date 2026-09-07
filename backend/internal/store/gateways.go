package store

import (
	"context"
	"database/sql"
	"time"
)

// Gateway is a registered edge collector. Only connection metadata is exposed
// through the API — never addresses, secrets or certificates.
type Gateway struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Area       string     `json:"area"`
	Protocol   string     `json:"protocol"`
	Status     string     `json:"status"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	BufferSize int64      `json:"buffer_size"`
	Version    string     `json:"version"`
	Config     *string    `json:"config"`
	CreatedAt  time.Time  `json:"created_at"`
}

// UpsertGateway registers or refreshes an edge gateway. The pulse carries the
// authoritative online/offline state and local buffer depth.
func UpsertGateway(ctx context.Context, db *sql.DB, g *Gateway) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO gateways (id, name, area, protocol, status, last_seen_at, buffer_size, version, config, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			last_seen_at = excluded.last_seen_at,
			buffer_size = excluded.buffer_size,
			version = COALESCE(NULLIF(excluded.version, ''), gateways.version)`,
		g.ID, g.Name, g.Area, g.Protocol, g.Status,
		nullTime(g.LastSeenAt), g.BufferSize, g.Version, g.Config,
		FormatUTC(g.CreatedAt))
	return err
}

// ListGateways returns the registered gateways with their last known state.
func ListGateways(ctx context.Context, db *sql.DB) ([]Gateway, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, name, area, protocol, status, last_seen_at, buffer_size, version, config, created_at
		 FROM gateways ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Gateway, 0)
	for rows.Next() {
		var g Gateway
		var lastSeen, created sql.NullString
		var config sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &g.Area, &g.Protocol, &g.Status,
			&lastSeen, &g.BufferSize, &g.Version, &config, &created); err != nil {
			return nil, err
		}
		if lastSeen.Valid {
			t := mustParse(lastSeen.String)
			g.LastSeenAt = &t
		}
		if config.Valid {
			g.Config = &config.String
		}
		g.CreatedAt = mustParse(created.String)
		out = append(out, g)
	}
	return out, rows.Err()
}
