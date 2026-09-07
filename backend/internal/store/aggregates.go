package store

import (
	"context"
	"database/sql"
	"time"
)

// Aggregate functions supported by /telemetry/aggregate.
const (
	AggAvg   = "avg"
	AggMin   = "min"
	AggMax   = "max"
	AggSum   = "sum"
	AggCount = "count"
	AggLast  = "last"
)

// ValidAggregate reports whether f is a supported aggregation function.
func ValidAggregate(f string) bool {
	switch f {
	case AggAvg, AggMin, AggMax, AggSum, AggCount, AggLast:
		return true
	default:
		return false
	}
}

// Aggregate is one downsampled bucket over a time window.
type Aggregate struct {
	Bucket         string    `json:"bucket"`           // RFC3339Nano start of the window, UTC
	Count          int       `json:"count"`            // readings in the bucket
	Avg            *float64  `json:"avg,omitempty"`    // avg/min/max/sum/last are
	Min            *float64  `json:"min,omitempty"`    // populated only for the
	Max            *float64  `json:"max,omitempty"`    // requested aggregate
	Sum            *float64  `json:"sum,omitempty"`    //
	Last           *float64  `json:"last,omitempty"`   // most recent value in the bucket
	LastObservedAt time.Time `json:"last_observed_at"` // time of the Last value
}

// AggregateFilter narrows an aggregate query. Resolution is the UTC time bucket.
type AggregateFilter struct {
	TagID      string
	AssetID    string
	From       *time.Time
	To         *time.Time
	Resolution time.Duration
	Function   string
	MaxBuckets int
}

// AggregateReadings downsamples numeric readings into fixed UTC time buckets.
// Bucketing happens in Go so the query stays portable across SQLite and
// PostgreSQL; the MAX_BUCKETS cap bounds memory for huge ranges.
func AggregateReadings(ctx context.Context, db *sql.DB, f AggregateFilter) ([]Aggregate, error) {
	if f.Resolution <= 0 {
		f.Resolution = time.Minute
	}
	if !ValidAggregate(f.Function) {
		f.Function = AggAvg
	}
	if f.MaxBuckets <= 0 {
		f.MaxBuckets = 512
	}

	q := `SELECT id, message_id, gateway_id, source_sequence, observed_at, received_at, sent_at,
		asset_id, tag_id, value_number, value_bool, value_string, value_structured,
		unit, quality, profile_id FROM telemetry_readings
		WHERE value_number IS NOT NULL`
	var args []any
	if f.TagID != "" {
		q += ` AND tag_id = ?`
		args = append(args, f.TagID)
	}
	if f.AssetID != "" {
		q += ` AND asset_id = ?`
		args = append(args, f.AssetID)
	}
	if f.From != nil {
		q += ` AND observed_at >= ?`
		args = append(args, FormatUTC(*f.From))
	}
	if f.To != nil {
		q += ` AND observed_at <= ?`
		args = append(args, FormatUTC(*f.To))
	}
	q += ` ORDER BY observed_at ASC`

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	idx := make(map[int64]int) // bucket key -> index in out
	out := make([]Aggregate, 0)
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
		if r.ValueNumber == nil {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, obs)
		if err != nil {
			return nil, err
		}
		bucket := t.Truncate(f.Resolution).Unix()
		i, ok := idx[bucket]
		if !ok {
			if len(out) >= f.MaxBuckets {
				continue
			}
			i = len(out)
			idx[bucket] = i
			start := time.Unix(bucket, 0).UTC()
			out = append(out, Aggregate{Bucket: FormatUTC(start), LastObservedAt: start})
		}
		a := &out[i]
		v := *r.ValueNumber
		a.Count++
		if a.Min == nil || v < *a.Min {
			c := v
			a.Min = &c
		}
		if a.Max == nil || v > *a.Max {
			c := v
			a.Max = &c
		}
		if a.Sum == nil {
			c := v
			a.Sum = &c
		} else {
			c := *a.Sum + v
			a.Sum = &c
		}
		c := v
		a.Last = &c
		a.LastObservedAt = t
		if a.Avg == nil {
			avg := v
			a.Avg = &avg
		} else {
			avg := *a.Avg + (v-*a.Avg)/float64(a.Count)
			a.Avg = &avg
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Project only the requested function into a compact response.
	projected := make([]Aggregate, 0, len(out))
	for _, a := range out {
		switch f.Function {
		case AggMin:
			a.Avg, a.Max, a.Sum, a.Last = nil, nil, nil, nil
		case AggMax:
			a.Avg, a.Min, a.Sum, a.Last = nil, nil, nil, nil
		case AggSum:
			a.Avg, a.Min, a.Max, a.Last = nil, nil, nil, nil
		case AggLast:
			a.Avg, a.Min, a.Max, a.Sum = nil, nil, nil, nil
		case AggCount:
			a.Avg, a.Min, a.Max, a.Sum, a.Last = nil, nil, nil, nil, nil
		}
		projected = append(projected, a)
	}
	return projected, nil
}
