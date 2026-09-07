package store

import (
	"context"
	"database/sql"
	"encoding/csv"
	"io"
	"strconv"
)

// ReadingsCSVHeader is the canonical column order for readings CSV export.
var ReadingsCSVHeader = []string{"observed_at", "tag_id", "asset_id", "value", "unit", "quality"}

// AlertsCSVHeader is the canonical column order for alerts CSV export.
var AlertsCSVHeader = []string{"created_at", "stage", "metric", "value", "threshold", "message"}

// WriteReadingsCSV streams readings matching the filter to w in RFC-4180 CSV.
func WriteReadingsCSV(ctx context.Context, db *sql.DB, w io.Writer, tagID, from, to, quality string, limit int) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(ReadingsCSVHeader); err != nil {
		return err
	}
	q := `SELECT observed_at, tag_id, asset_id, value_number, unit, quality
	      FROM telemetry_readings WHERE value_number IS NOT NULL`
	var args []any
	if tagID != "" {
		q += ` AND tag_id = ?`
		args = append(args, tagID)
	}
	if from != "" {
		q += ` AND observed_at >= ?`
		args = append(args, from)
	}
	if to != "" {
		q += ` AND observed_at <= ?`
		args = append(args, to)
	}
	if quality != "" {
		q += ` AND quality = ?`
		args = append(args, quality)
	}
	q += ` ORDER BY observed_at DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var obs, tid, aid, unit, qual string
		var val sql.NullFloat64
		if err := rows.Scan(&obs, &tid, &aid, &val, &unit, &qual); err != nil {
			return err
		}
		v := ""
		if val.Valid {
			v = strconv.FormatFloat(val.Float64, 'f', -1, 64)
		}
		if err := cw.Write([]string{obs, tid, aid, v, unit, qual}); err != nil {
			return err
		}
	}
	defer cw.Flush()
	return rows.Err()
}

// WriteAlertsCSV streams alerts filtered to csv.
func WriteAlertsCSV(ctx context.Context, db *sql.DB, w io.Writer, stage string, limit int) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(AlertsCSVHeader); err != nil {
		return err
	}
	q := `SELECT created_at, stage, metric, value, threshold, message FROM alerts`
	var args []any
	if stage != "" {
		q += ` WHERE stage = ?`
		args = append(args, stage)
	}
	q += ` ORDER BY created_at DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var created, stage, metric, msg string
		var val, th float64
		if err := rows.Scan(&created, &stage, &metric, &val, &th, &msg); err != nil {
			return err
		}
		if err := cw.Write([]string{
			created, stage, metric,
			strconv.FormatFloat(val, 'f', -1, 64),
			strconv.FormatFloat(th, 'f', -1, 64),
			msg,
		}); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}
