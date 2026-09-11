// Package alarms implements the ISA-18.2 alarm engine (TZ §11): rationalised
// band evaluation with on/off delays and hysteresis, an immutable journal of
// lifecycle events, WebSocket fan-out of state changes and a per-asset
// communication-loss watchdog. Evaluation runs on the ingest path — one value
// in, at most one state transition out.
package alarms

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"cap/internal/hub"
	"cap/internal/schema"
	"cap/internal/store"
)

// Evaluation timing (TZ §11): critical bands react fast, the rest with a
// deliberate delay; the hysteresis band is 1% of the engineering span.
const (
	onDelayCritical = 3 * time.Second
	onDelayDefault  = 10 * time.Second
	clearDelay      = 5 * time.Second
	commLossAfter   = 10 * time.Second
)

type bandState struct {
	active    bool   // alarm currently raised
	condBand  string // pending band ("hi", "lo", "lo_lo", "hi_hi") or "clear"
	condSince time.Time
}

type assetComm struct {
	lastGood time.Time
	raised   bool
}

// Engine evaluates telemetry against rationalised limits.
type Engine struct {
	db  *sql.DB
	hub *hub.Hub
	now func() time.Time

	mu          sync.Mutex
	limits      map[string]*store.AlarmLimit
	limitsUntil time.Time
	bands       map[string]*bandState // tag_id -> band state
	comm        map[string]*assetComm // asset_id -> comm state
	assets      []store.Asset
	assetsUntil time.Time
}

// New creates an Engine. Sweep runs periodically (2s cadence) to maintain the
// per-asset communication-loss watchdog.
func New(db *sql.DB, h *hub.Hub) *Engine {
	return &Engine{db: db, hub: h, now: time.Now,
		bands: map[string]*bandState{}, comm: map[string]*assetComm{}}
}

// SetNow overrides the clock (tests).
func (e *Engine) SetNow(f func() time.Time) { e.now = f }

func (e *Engine) broadcast(ev map[string]any) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	e.hub.Broadcast(b)
}

// limitsFor returns the rationalised limits cache, refreshed every 30s.
func (e *Engine) limitsFor(ctx context.Context) map[string]*store.AlarmLimit {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.limits != nil && e.now().Before(e.limitsUntil) {
		return e.limits
	}
	rows, err := store.ListAlarmLimits(ctx, e.db)
	if err != nil {
		log.Printf("[alarms] list limits: %v", err)
		if e.limits == nil {
			e.limits = map[string]*store.AlarmLimit{}
		}
		return e.limits
	}
	m := make(map[string]*store.AlarmLimit, len(rows))
	for i := range rows {
		m[rows[i].TagID] = &rows[i]
	}
	e.limits = m
	e.limitsUntil = e.now().Add(30 * time.Second)
	return m
}

type bandResult struct {
	band      string
	threshold float64
	limits    *store.AlarmLimit
}

// classify maps a value to an off-normal band, worst first.
func classify(l *store.AlarmLimit, v float64) (string, float64) {
	if l.HiHi != nil && v >= *l.HiHi {
		return "hi_hi", *l.HiHi
	}
	if l.LoLo != nil && v <= *l.LoLo {
		return "lo_lo", *l.LoLo
	}
	if l.Hi != nil && v >= *l.Hi {
		return "hi", *l.Hi
	}
	if l.Lo != nil && v <= *l.Lo {
		return "lo", *l.Lo
	}
	return "", 0
}

// hysteresis returns the re-arm margin: 1% of the engineering span, or 1% of
// the threshold magnitude when the tag has no configured span.
func hysteresis(tag *store.Tag, threshold float64) float64 {
	if tag.EngineeringMin != nil && tag.EngineeringMax != nil && *tag.EngineeringMax > *tag.EngineeringMin {
		return 0.01 * (*tag.EngineeringMax - *tag.EngineeringMin)
	}
	return 0.01 * math.Abs(threshold)
}

// Evaluate applies the band logic to one reading. Gap qualities skip band
// evaluation (they only feed the comm-loss watchdog via noteGood below).
func (e *Engine) Evaluate(ctx context.Context, tag *store.Tag, r *store.Reading) {
	if r.ValueNumber == nil {
		return
	}
	now := e.now()
	e.noteGood(tag.AssetID, now)

	var res *bandResult
	if r.Quality == schema.QualityGood {
		if lim := e.limitsFor(ctx)[r.TagID]; lim != nil && lim.Enabled {
			if band, thr := classify(lim, *r.ValueNumber); band != "" {
				res = &bandResult{band, thr, lim}
			}
		}
	}
	e.evaluateBand(ctx, tag, r, res, now)
}

func (e *Engine) evaluateBand(ctx context.Context, tag *store.Tag, r *store.Reading, res *bandResult, now time.Time) {
	e.mu.Lock()
	st, ok := e.bands[r.TagID]
	if !ok {
		st = &bandState{}
		e.bands[r.TagID] = st
	}
	e.mu.Unlock()

	if res == nil {
		// Gap qualities (offline/stale/...) freeze the state: a lost signal
		// never fabricates a return to normal. Only good values advance the
		// clear timer.
		if r.Quality != schema.QualityGood {
			return
		}
		// Value inside limits: clear only after the value has stayed clear
		// (with hysteresis) for clearDelay.
		if st.active {
			lim := e.limitsFor(ctx)[r.TagID]
			if lim != nil {
				span := hysteresis(tag, worstThreshold(lim))
				v := *r.ValueNumber
				clear := true
				if lim.HiHi != nil && v >= *lim.HiHi-span {
					clear = false
				}
				if lim.Hi != nil && v >= *lim.Hi-span {
					clear = false
				}
				if lim.LoLo != nil && v <= *lim.LoLo+span {
					clear = false
				}
				if lim.Lo != nil && v <= *lim.Lo+span {
					clear = false
				}
				if !clear {
					st.condBand = ""
					st.condSince = time.Time{}
					return
				}
			}
		}
		if !st.active {
			st.condBand = ""
			st.condSince = time.Time{}
			return
		}
		if st.condBand != "clear" {
			st.condBand = "clear"
			st.condSince = now
			return
		}
		if now.Sub(st.condSince) < clearDelay {
			return
		}
		st.active = false
		st.condBand = ""
		st.condSince = time.Time{}
		e.clear(ctx, r.TagID, now)
		return
	}

	// Off-normal condition present.
	if st.active {
		st.condBand = ""
		st.condSince = time.Time{}
		return
	}
	if st.condBand != res.band {
		st.condBand = res.band
		st.condSince = now
		return
	}
	delay := onDelayDefault
	if res.band == "hi_hi" || res.band == "lo_lo" {
		delay = onDelayCritical
	}
	if now.Sub(st.condSince) < delay {
		return
	}
	st.active = true
	st.condBand = ""
	st.condSince = time.Time{}
	e.raise(ctx, r.TagID, res, *r.ValueNumber, now)
}

// worstThreshold returns the tightest configured boundary, for hysteresis.
func worstThreshold(l *store.AlarmLimit) float64 {
	first := 0.0
	for _, v := range []*float64{l.HiHi, l.Hi, l.Lo, l.LoLo} {
		if v != nil {
			first = *v
			break
		}
	}
	return first
}

func (e *Engine) raise(ctx context.Context, tagID string, res *bandResult, value float64, now time.Time) {
	severity := res.limits.Severity
	if severity == "" {
		severity = "medium"
	}
	if res.band == "hi" || res.band == "lo" {
		severity = "medium" // TZ §6: off-normal bands are operator notices
	}
	dir := "выше"
	if res.band == "lo" || res.band == "lo_lo" {
		dir = "ниже"
	}
	message := fmt.Sprintf("Значение %s предела %s: %.3g против %.3g %s",
		dir, res.band, value, res.threshold, unitOf(tagID))

	metric := metricOf(tagID)
	if err := store.UpsertAlarmActive(ctx, e.db, tagID, metric, severity, message, now); err != nil {
		log.Printf("[alarms] upsert %s: %v", tagID, err)
		return
	}
	a, err := store.GetAlarmByTag(ctx, e.db, tagID)
	if err != nil {
		log.Printf("[alarms] fetch %s: %v", tagID, err)
		return
	}
	_ = store.InsertAlarmEvent(ctx, e.db, &store.AlarmEvent{
		OccurredAt: now, TagID: tagID, Metric: metric, Event: store.AlarmEventRaised,
		Severity: a.Severity, Priority: a.Priority, Message: message,
	})
	e.broadcast(map[string]any{
		"type": "alarm_raised", "timestamp": formatUTC(now),
		"alarm_id": a.ID, "tag_id": tagID, "metric": metric,
		"severity": a.Severity, "priority": a.Priority, "message": message,
	})
}

func (e *Engine) clear(ctx context.Context, tagID string, now time.Time) {
	if err := store.ClearAlarm(ctx, e.db, tagID, now); err != nil {
		log.Printf("[alarms] clear %s: %v", tagID, err)
		return
	}
	a, err := store.GetAlarmByTag(ctx, e.db, tagID)
	if err != nil {
		return
	}
	_ = store.InsertAlarmEvent(ctx, e.db, &store.AlarmEvent{
		OccurredAt: now, TagID: tagID, Metric: a.Metric, Event: store.AlarmEventCleared,
		Severity: a.Severity, Priority: a.Priority, Message: "Возврат в норму",
	})
	e.broadcast(map[string]any{
		"type": "alarm_cleared", "timestamp": formatUTC(now),
		"alarm_id": a.ID, "tag_id": tagID, "metric": a.Metric,
	})
}

// Acked records an operator acknowledgement: journal row + WS notification.
// The state row was already updated by the handler; the actor always comes
// from the authenticated session.
func (e *Engine) Acked(ctx context.Context, alarm *store.Alarm, actor, comment string, now time.Time) {
	_ = store.InsertAlarmEvent(ctx, e.db, &store.AlarmEvent{
		OccurredAt: now, TagID: alarm.TagID, Metric: alarm.Metric,
		Event: store.AlarmEventAcked, Severity: alarm.Severity, Priority: alarm.Priority,
		Message: "Квитировано оператором", Actor: actor, Comment: comment,
	})
	e.broadcast(map[string]any{
		"type": "alarm_acked", "timestamp": formatUTC(now),
		"alarm_id": alarm.ID, "tag_id": alarm.TagID, "actor": actor,
	})
}

// RaiseOperational records a system alarm not driven by a tag band (control
// watchdog, communication loss). tagID may be a synthetic "comm:<asset>" id.
func (e *Engine) RaiseOperational(ctx context.Context, tagID, metric, severity, message string, now time.Time) {
	if err := store.UpsertAlarmActive(ctx, e.db, tagID, metric, severity, message, now); err != nil {
		return
	}
	a, err := store.GetAlarmByTag(ctx, e.db, tagID)
	if err != nil {
		return
	}
	_ = store.InsertAlarmEvent(ctx, e.db, &store.AlarmEvent{
		OccurredAt: now, TagID: tagID, Metric: metric, Event: store.AlarmEventRaised,
		Severity: a.Severity, Priority: a.Priority, Message: message,
	})
	e.broadcast(map[string]any{
		"type": "alarm_raised", "timestamp": formatUTC(now),
		"alarm_id": a.ID, "tag_id": tagID, "metric": metric,
		"severity": a.Severity, "priority": a.Priority, "message": message,
	})
}

// ClearOperational removes a system alarm (recovery path).
func (e *Engine) ClearOperational(ctx context.Context, tagID string, now time.Time) {
	a, err := store.GetAlarmByTag(ctx, e.db, tagID)
	if err != nil {
		return
	}
	if err := store.ClearAlarm(ctx, e.db, tagID, now); err != nil {
		return
	}
	_ = store.InsertAlarmEvent(ctx, e.db, &store.AlarmEvent{
		OccurredAt: now, TagID: tagID, Metric: a.Metric, Event: store.AlarmEventCleared,
		Severity: a.Severity, Priority: a.Priority, Message: "Возврат в норму",
	})
	e.broadcast(map[string]any{
		"type": "alarm_cleared", "timestamp": formatUTC(now),
		"alarm_id": a.ID, "tag_id": tagID, "metric": a.Metric,
	})
}

func (e *Engine) noteGood(assetID string, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if c, ok := e.comm[assetID]; ok {
		c.lastGood = now
		return
	}
	e.comm[assetID] = &assetComm{lastGood: now}
}

// Sweep maintains the per-asset communication-loss alarm: every asset must
// deliver at least one good reading per interval, or a medium "comm_loss"
// alarm is raised once (cleared on recovery).
func (e *Engine) Sweep(ctx context.Context) {
	now := e.now()

	var toRaise, toClear []string
	e.mu.Lock()
	for _, a := range e.assetListLocked(ctx) {
		c, ok := e.comm[a.ID]
		fresh := ok && now.Sub(c.lastGood) < commLossAfter
		switch {
		case !fresh && (!ok || !c.raised):
			if !ok {
				e.comm[a.ID] = &assetComm{lastGood: time.Time{}}
			}
			e.comm[a.ID].raised = true
			toRaise = append(toRaise, a.ID)
		case fresh && ok && c.raised:
			c.raised = false
			toClear = append(toClear, a.ID)
		}
	}
	e.mu.Unlock()

	for _, id := range toRaise {
		e.RaiseOperational(ctx, "comm:"+id, "comm_loss", "medium",
			"Нет достоверных данных по активу более 10 с", now)
	}
	for _, id := range toClear {
		e.ClearOperational(ctx, "comm:"+id, now)
	}
}

// assetListLocked reads the asset cache; the caller holds e.mu. The cache
// itself is refreshed without re-locking (single-threaded Sweep ticker plus
// Evaluate both end here, so a plain re-entrancy guard is enough).
func (e *Engine) assetListLocked(ctx context.Context) []store.Asset {
	if e.assets != nil && e.now().Before(e.assetsUntil) {
		return e.assets
	}
	e.mu.Unlock()
	rows, err := store.ListAssets(ctx, e.db)
	e.mu.Lock()
	if err != nil {
		return e.assets
	}
	e.assets = rows
	e.assetsUntil = e.now().Add(time.Minute)
	return rows
}

func metricOf(tagID string) string {
	if i := strings.LastIndexByte(tagID, '.'); i >= 0 {
		return tagID[i+1:]
	}
	return tagID
}

func unitOf(tagID string) string { return "" }

func formatUTC(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
