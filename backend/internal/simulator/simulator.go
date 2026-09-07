// Package simulator generates deterministic, realistic telemetry for the seeded
// registry so development, tests and demos run without field equipment. It is
// development-only: production must disable it (SIMULATOR_ENABLED=false).
package simulator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	mathrand "math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"

	"autopro/internal/hub"
	"autopro/internal/schema"
	"autopro/internal/store"
)

// simMetric holds the demo operating point, noise and threshold envelope for a
// metric. Thresholds here are demo values and MUST move into ore profiles and
// the registry (see TZ_DEVELOPMENT.md S1) before production.
type simMetric struct {
	baseline float64
	noise    float64
	min      float64
	max      float64
}

// threshold is the breach envelope for one metric.
type threshold struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// profileParams is the machine-readable ore profile. Thresholds override the
// demo envelopes; baseline overrides the operating point for the metric.
type profileParams struct {
	Thresholds map[string]threshold `json:"thresholds"`
	Baseline   map[string]float64   `json:"baseline"`
}

// operatingPoint resolves the effective baseline and envelope for a metric,
// preferring the active ore profile and falling back to the demo defaults.
func operatingPoint(params *profileParams, metric string) (baseline, min, max float64) {
	def := simMetrics[metric]
	baseline, min, max = def.baseline, def.min, def.max
	if params == nil {
		return baseline, min, max
	}
	if b, ok := params.Baseline[metric]; ok {
		baseline = b
	}
	if th, ok := params.Thresholds[metric]; ok {
		min, max = th.Min, th.Max
	}
	return baseline, min, max
}

var simMetrics = map[string]simMetric{
	"particle_size":     {5.0, 0.5, 0.0, 12.0},
	"pulp_density":      {1.65, 0.05, 1.0, 2.5},
	"ph_level":          {9.5, 0.3, 7.0, 11.0},
	"reagent_dosage":    {55.0, 5.0, 10.0, 120.0},
	"cake_moisture":     {8.0, 0.5, 0.0, 12.0},
	"dryer_temperature": {150.0, 5.0, 60.0, 250.0},
	"tonnage_weight":    {250.0, 10.0, 0.0, 500.0},
	"final_moisture":    {5.0, 0.3, 0.0, 10.0},
}

const gatewayID = "gw-simulator-01"

// Manager owns the demo generator lifecycle and emergency state.
type Manager struct {
	db       *sql.DB
	hub      *hub.Hub
	interval time.Duration
	emergDur time.Duration

	mu            sync.Mutex
	running       bool
	emergencyMode bool
	count         int64
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// New creates a simulator manager.
func New(db *sql.DB, h *hub.Hub, interval, emergDur time.Duration) *Manager {
	return &Manager{db: db, hub: h, interval: interval, emergDur: emergDur}
}

// Start launches the background loop. It is idempotent.
func (m *Manager) Start(parent context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	m.running = true
	m.cancel = cancel
	m.wg.Add(1)
	go m.run(ctx)
}

// Stop halts the background loop and waits for it to finish.
func (m *Manager) Stop() {
	m.mu.Lock()
	cancel := m.cancel
	running := m.running
	m.mu.Unlock()
	if !running {
		return
	}
	cancel()
	m.wg.Wait()
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()
}

// IsRunning reports whether the generator loop is active.
func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Count returns the number of generated telemetry messages.
func (m *Manager) Count() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.count
}

// IsEmergency reports whether emergency mode is active.
func (m *Manager) IsEmergency() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.emergencyMode
}

// TriggerEmergency activates failure simulation for emergDur.
func (m *Manager) TriggerEmergency() {
	m.mu.Lock()
	if m.emergencyMode {
		m.mu.Unlock()
		return
	}
	m.emergencyMode = true
	m.mu.Unlock()

	m.broadcastEvent("emergency_start",
		"Emergency mode: Cake Moisture → 15%, Flotation pH → 5.0")

	go func() {
		time.Sleep(m.emergDur)
		m.EndEmergency()
	}()
}

// EndEmergency deactivates failure simulation.
func (m *Manager) EndEmergency() {
	m.mu.Lock()
	if !m.emergencyMode {
		m.mu.Unlock()
		return
	}
	m.emergencyMode = false
	m.mu.Unlock()

	m.broadcastEvent("emergency_end",
		"Emergency mode ended — returning to normal")
}

func (m *Manager) run(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.tick(ctx); err != nil {
				// transient DB errors are logged and retried on the next tick
				continue
			}
		}
	}
}

func (m *Manager) tick(ctx context.Context) error {
	tags, err := store.ListTags(ctx, m.db, "")
	if err != nil {
		return err
	}
	areas, err := assetAreas(ctx, m.db)
	if err != nil {
		return err
	}
	activeProfile, err := store.GetActiveProfile(ctx, m.db)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	params := parseProfileParams(activeProfile)
	// Plant-approved alarm rationalisation overrides profile/demo envelopes. A
	// disabled limit means the tag's alarm is not armed: we still emit telemetry
	// but never fire an alarm. The cache is per-tick; rationalised settings are
	// rarely updated and harden if they do.
	limits, err := m.alarmLimitsCache(ctx)
	if err != nil {
		// Non-fatal: fall back to profile/demo envelopes only.
		limits = map[string]*store.AlarmLimit{}
	}
	now := time.Now().UTC()
	m.mu.Lock()
	emergency := m.emergencyMode
	m.mu.Unlock()

	for _, tag := range tags {
		metric := metricOf(tag.ID)
		def, ok := simMetrics[metric]
		if !ok || !tag.Active {
			continue
		}
		baseline, _, _ := operatingPoint(params, metric)
		value := baseline + mathrand.NormFloat64()*def.noise
		if emergency {
			switch metric {
			case "cake_moisture":
				value = 15.0 + mathrand.NormFloat64()*0.3
			case "ph_level":
				value = 5.0 + mathrand.NormFloat64()*0.15
			}
		}
		value = math.Round(value*1000) / 1000

		alert, severity, threshold := m.evalAlarm(limits, params, tag.ID, metric, value)

		reading := &store.Reading{
			ID:          store.NewID(),
			MessageID:   schema.NewUUID(),
			GatewayID:   gatewayID,
			ObservedAt:  now,
			ReceivedAt:  now,
			AssetID:     tag.AssetID,
			TagID:       tag.ID,
			ValueNumber: &value,
			Unit:        tag.Unit,
			Quality:     schema.QualityGood,
		}
		if activeProfile != nil {
			reading.ProfileID = &activeProfile.ID
		}
		if err := store.InsertReading(ctx, m.db, reading); err != nil {
			continue
		}
		m.mu.Lock()
		m.count++
		m.mu.Unlock()

		m.hub.Broadcast(liveEvent(areas, tag, reading, alert, emergency))

		if alert {
			msg := severityMessage(severity, metric, value, threshold)
			_ = store.InsertAlert(ctx, m.db, &store.Alert{
				ID:        store.NewID(),
				Stage:     areas[tag.AssetID],
				Metric:    metric,
				Value:     value,
				Threshold: threshold,
				Message:   msg,
				CreatedAt: now,
			})
			_ = store.UpsertAlarmActive(ctx, m.db, tag.ID, metric, severity, msg, now)
		} else {
			_ = store.ClearAlarm(ctx, m.db, tag.ID, now)
		}
	}
	return nil
}

// alarmLimitsCache loads all rationalised limits into a lookup keyed by tag_id.
// On any error it returns nil — the caller treats a missing entry as "use the
// profile/demo envelope", never as "fail the tick".
func (m *Manager) alarmLimitsCache(ctx context.Context) (map[string]*store.AlarmLimit, error) {
	limits, err := store.ListAlarmLimits(ctx, m.db)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*store.AlarmLimit, len(limits))
	for i := range limits {
		out[limits[i].TagID] = &limits[i]
	}
	return out, nil
}

// evalAlarm resolves whether a value fires an alarm for tagID. Precedence:
// rationalised limit (if armed) > active ore-profile threshold > demo envelope.
// Returns (alert, severity||"", threshold||0). params is the same profile
// params object the tick already decoded, so the fallback can honour the
// active profile when no rationalised limit exists.
func (m *Manager) evalAlarm(limits map[string]*store.AlarmLimit, params *profileParams, tagID, metric string, value float64) (bool, string, float64) {
	if lim, ok := limits[tagID]; ok {
		if !lim.Enabled {
			return false, "", 0
		}
		return evalAgainstLimit(value, lim)
	}
	_, min, max := operatingPoint(params, metric)
	if value >= min && value <= max {
		return false, "", 0
	}
	// No rationalised severity: keep "critical" for the legacy envelope so
	// previously stored alerts keep the same wording (regression-safe).
	th := max
	if value < min {
		th = min
	}
	return true, "critical", th
}

func evalAgainstLimit(value float64, lim *store.AlarmLimit) (bool, string, float64) {
	// Per ISA-18.2 multi-band: hi_hi/lo_lo are critical, hi/lo are warning
	// (medium severity unless the limit row overrides). Boundaries are checked
	// low-to-high; the most severe breach wins.
	severity := lim.Severity
	if severity == "" {
		severity = "high"
	}
	switch {
	case lim.HiHi != nil && value > *lim.HiHi:
		return true, severity, *lim.HiHi
	case lim.LoLo != nil && value < *lim.LoLo:
		return true, severity, *lim.LoLo
	case lim.Hi != nil && value > *lim.Hi:
		return true, severityOf(lim, "medium"), *lim.Hi
	case lim.Lo != nil && value < *lim.Lo:
		return true, severityOf(lim, "medium"), *lim.Lo
	default:
		return false, "", 0
	}
}

func severityOf(lim *store.AlarmLimit, def string) string {
	if lim.Severity != "" {
		return lim.Severity
	}
	return def
}

func severityMessage(severity, metric string, value, threshold float64) string {
	tag := strings.ToUpper(severity)
	if tag == "" {
		tag = "ALARM"
	}
	return tag + ": " + metric + " = " + trimFloat(value) + " (threshold " + trimFloat(threshold) + ")"
}

// parseProfileParams decodes the active profile params JSON, ignoring any
// malformed field so the demo never dies on a bad profile edit.
func parseProfileParams(p *store.Profile) *profileParams {
	if p == nil || p.Params == "" {
		return nil
	}
	var params profileParams
	if err := json.Unmarshal([]byte(p.Params), &params); err != nil {
		return nil
	}
	return &params
}

func (m *Manager) broadcastEvent(typ, msg string) {
	now := store.FormatUTC(time.Now().UTC())
	payload := fmt.Sprintf(`{"type":%s,"timestamp":%s,"message":%s}`,
		strconv.Quote(typ), strconv.Quote(now), strconv.Quote(msg))
	m.hub.Broadcast([]byte(payload))
}

// liveEvent mirrors the dashboard wire format: metric is the last tag segment,
// stage is the asset area.
func liveEvent(areas map[string]string, tag store.Tag, r *store.Reading, alert, emergency bool) []byte {
	ev := struct {
		Type      string  `json:"type"`
		Timestamp string  `json:"timestamp"`
		AssetID   string  `json:"asset_id"`
		TagID     string  `json:"tag_id"`
		Stage     string  `json:"stage"`
		Metric    string  `json:"metric"`
		Value     float64 `json:"value"`
		Unit      string  `json:"unit"`
		Quality   string  `json:"quality"`
		Alert     bool    `json:"alert"`
		Emergency bool    `json:"emergency"`
	}{
		Type: "telemetry", Timestamp: store.FormatUTC(r.ObservedAt),
		AssetID: r.AssetID, TagID: r.TagID,
		Stage: areas[tag.AssetID], Metric: metricOf(tag.ID),
		Value: *r.ValueNumber, Unit: r.Unit, Quality: r.Quality,
		Alert: alert, Emergency: emergency,
	}
	b, _ := json.Marshal(ev)
	return b
}

func assetAreas(ctx context.Context, db *sql.DB) (map[string]string, error) {
	assets, err := store.ListAssets(ctx, db)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(assets))
	for _, a := range assets {
		m[a.ID] = a.Area
	}
	return m, nil
}

func metricOf(tagID string) string {
	seg := strings.Split(tagID, ".")
	return seg[len(seg)-1]
}

func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
