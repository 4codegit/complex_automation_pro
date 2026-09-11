package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// Alarm event kinds recorded in the immutable ISA-18.2 journal.
const (
	AlarmEventRaised  = "raised"
	AlarmEventCleared = "cleared"
	AlarmEventAcked   = "acked"
	AlarmEventShelved = "shelved"
)

// AlarmEvent is one immutable journal row. The alarms table holds the current
// state per tag; the journal answers "what happened, when, who acted".
type AlarmEvent struct {
	ID         string    `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
	TagID      string    `json:"tag_id"`
	Metric     string    `json:"metric"`
	Event      string    `json:"event"`
	Severity   string    `json:"severity"`
	Priority   int       `json:"priority"`
	Message    string    `json:"message"`
	Actor      string    `json:"actor"`
	Comment    string    `json:"comment"`
}

// InsertAlarmEvent appends one journal row.
func InsertAlarmEvent(ctx context.Context, db *sql.DB, e *AlarmEvent) error {
	if e.ID == "" {
		e.ID = NewID()
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO alarm_events
		 (id, occurred_at, tag_id, metric, event, severity, priority, message, actor, comment)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, FormatUTC(e.OccurredAt), e.TagID, e.Metric, e.Event,
		e.Severity, e.Priority, e.Message, e.Actor, e.Comment)
	return err
}

// AlarmEventFilter narrows a journal query.
type AlarmEventFilter struct {
	TagID       string
	MinPriority int
	From        *time.Time
	To          *time.Time
	Limit       int
}

// QueryAlarmEvents returns journal rows newest-first with optional filters.
func QueryAlarmEvents(ctx context.Context, db *sql.DB, f AlarmEventFilter) ([]AlarmEvent, error) {
	q := `SELECT id, occurred_at, tag_id, metric, event, severity, priority, message, actor, comment
	      FROM alarm_events`
	var conds []string
	var args []any
	if f.TagID != "" {
		conds = append(conds, "tag_id = ?")
		args = append(args, f.TagID)
	}
	if f.MinPriority > 0 {
		conds = append(conds, "priority <= ?")
		args = append(args, f.MinPriority)
	}
	if f.From != nil {
		conds = append(conds, "occurred_at >= ?")
		args = append(args, FormatUTC(*f.From))
	}
	if f.To != nil {
		conds = append(conds, "occurred_at <= ?")
		args = append(args, FormatUTC(*f.To))
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	if f.Limit <= 0 {
		f.Limit = 500
	}
	q += " ORDER BY occurred_at DESC LIMIT ?"
	args = append(args, f.Limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]AlarmEvent, 0)
	for rows.Next() {
		var e AlarmEvent
		var occ string
		if err := rows.Scan(&e.ID, &occ, &e.TagID, &e.Metric, &e.Event,
			&e.Severity, &e.Priority, &e.Message, &e.Actor, &e.Comment); err != nil {
			return nil, err
		}
		e.OccurredAt = mustParse(occ)
		out = append(out, e)
	}
	return out, rows.Err()
}

// seedLimitSpec is one rationalised alarm boundary set (TZ §6). Nil entries
// leave that band unset; severity applies to the off-normal extreme bands.
type seedLimitSpec struct {
	tag                string
	loLo, lo, hi, hiHi *float64
	sev                string
	notes              string
}

func f64(v float64) *float64 { return &v }

// SeedAlarmLimits inserts the rationalised limits of TZ §6 when the table is
// empty. They are the authoritative source the alarm engine evaluates.
func SeedAlarmLimits(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alarm_limits`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	// fullID resolves the short metric name to the registry tag id.
	fullID := func(metric string) string {
		for _, spec := range seedTags {
			if spec.metric == metric {
				return spec.asset + "." + spec.metric
			}
		}
		return metric
	}

	limits := []seedLimitSpec{
		{tag: "fi101", loLo: f64(40), lo: f64(70), hi: f64(130), hiHi: f64(160), sev: "critical", notes: "Потеря производства"},
		{tag: "ei201", loLo: f64(200), lo: f64(600), hi: f64(1800), hiHi: f64(2100), sev: "critical", notes: "Защита привода мельницы"},
		{tag: "pi201", loLo: f64(30), lo: f64(60), hi: f64(220), hiHi: f64(260), sev: "high", notes: "Классификация"},
		{tag: "xi201", lo: f64(120), hi: f64(190), hiHi: f64(220), sev: "critical", notes: "Грубый слив → потеря извлечения"},
		{tag: "li301", loLo: f64(300), lo: f64(380), hi: f64(700), hiHi: f64(760), sev: "critical", notes: "Перелив флотомашины"},
		{tag: "ai301", loLo: f64(9.0), lo: f64(9.6), hi: f64(10.8), hiHi: f64(11.5), sev: "high", notes: "Окно извлечения по pH"},
		{tag: "di301", loLo: f64(20), lo: f64(24), hi: f64(36), hiHi: f64(40), sev: "high", notes: "Плотность питания флотации"},
		{tag: "afi301", loLo: f64(0.45), lo: f64(0.60), sev: "high", notes: "Резкое изменение качества руды"},
		{tag: "afc301", lo: f64(18.0), sev: "high", notes: "Качество концентрата"},
		{tag: "aft301", hi: f64(0.12), hiHi: f64(0.18), sev: "critical", notes: "Потери металла в хвостах"},
		{tag: "wi301", loLo: f64(1.5), lo: f64(2.2), sev: "high", notes: "Пропуск концентрата"},
		{tag: "li401", loLo: f64(0.8), lo: f64(1.2), hi: f64(5.5), hiHi: f64(6.5), sev: "critical", notes: "Переполнение постели"},
		{tag: "di401", loLo: f64(32), lo: f64(36), hi: f64(52), hiHi: f64(58), sev: "high", notes: "Сгущённый продукт"},
		{tag: "ei401", hi: f64(70), hiHi: f64(85), sev: "critical", notes: "Защита механизма сгустителя"},
		{tag: "mi501", hi: f64(13.0), hiHi: f64(16.0), sev: "high", notes: "Влажность кека"},
		{tag: "pi501", loLo: f64(25), lo: f64(35), hi: f64(70), hiHi: f64(78), sev: "high", notes: "Вакуум фильтра"},
		{tag: "si101", lo: f64(15), hi: f64(95), sev: "high", notes: "Пересорт/пустой бункер"},
		{tag: "tit101", loLo: f64(8), lo: f64(12), hi: f64(35), hiHi: f64(40), sev: "medium", notes: "Температура пульпы"},
	}

	now := time.Now().UTC()
	for _, l := range limits {
		if err := UpsertAlarmLimit(ctx, db, &AlarmLimit{
			TagID:          fullID(l.tag),
			Enabled:        true,
			LoLo:           l.loLo,
			Lo:             l.lo,
			Hi:             l.hi,
			HiHi:           l.hiHi,
			Severity:       l.sev,
			RationalisedBy: "seed",
			Notes:          l.notes,
		}, now); err != nil {
			return err
		}
	}
	return nil
}
