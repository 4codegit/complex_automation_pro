// Package api implements the HTTP API (push ingestion, pull queries, registry,
// health, WebSocket live fan-out) on top of the store and hub.
package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"cap/internal/config"
	"cap/internal/hub"
	"cap/internal/schema"
	"cap/internal/simulator"
	"cap/internal/store"
)

// Server wires handlers to shared dependencies.
type Server struct {
	db  *sql.DB
	hub *hub.Hub
	sim *simulator.Manager
	cfg *config.Settings
	now func() time.Time

	areasMu sync.RWMutex
	areas   map[string]string // asset_id -> area, lazily loaded for WS events
}

// New constructs a Server.
func New(db *sql.DB, h *hub.Hub, sim *simulator.Manager, cfg *config.Settings) *Server {
	return &Server{db: db, hub: h, sim: sim, cfg: cfg, now: time.Now}
}

// httpError distinguishes whole-request failures (4xx/5xx) from per-item
// ingest results. A nil *httpError means the item was processed normally.
type httpError struct {
	status  int
	code    string
	message string
}

func (e *httpError) Error() string { return e.message }

// gapNormalized returns the zero value representation for a tag when the
// telemetry quality marks a gap (offline/stale/substituted). The stored value
// carries no signal — only the timestamp and quality are meaningful.
func gapNormalized(dataType string) *schema.Normalized {
	switch dataType {
	case schema.KindBoolean:
		b := false
		return &schema.Normalized{Kind: schema.KindBoolean, Bool: &b}
	case schema.KindString:
		s := ""
		return &schema.Normalized{Kind: schema.KindString, Text: &s}
	case schema.KindStructured:
		s := "{}"
		return &schema.Normalized{Kind: schema.KindStructured, Structured: &s}
	default:
		f := 0.0
		return &schema.Normalized{Kind: schema.KindNumber, Number: &f}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeProblem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"code": code, "message": message})
}

func queryStr(r *http.Request, key string) string { return strings.TrimSpace(r.URL.Query().Get(key)) }

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

// HealthCheck reports dependency reachability. It is liveness for the API and
// readiness for its dependencies.
func (s *Server) HealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	dbStatus := "connected"
	overall := "healthy"
	if err := s.db.PingContext(ctx); err != nil {
		dbStatus = "disconnected"
		overall = "unhealthy"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    overall,
		"timestamp": s.now().UTC(),
		"database":  dbStatus,
	})
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

// ListRoles has moved to handlers_rbac.go: the permission matrix is now
// sourced from the roles table, not hard-coded here.

// ListAssets returns the equipment hierarchy.
func (s *Server) ListAssets(w http.ResponseWriter, r *http.Request) {
	assets, err := store.ListAssets(r.Context(), s.db)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, assets)
}

// ListTags returns registered signals, optionally filtered by asset.
func (s *Server) ListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := store.ListTags(r.Context(), s.db, queryStr(r, "asset_id"))
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

// ---------------------------------------------------------------------------
// Ingestion (push)
// ---------------------------------------------------------------------------

// IngestTelemetry handles a single canonical message.
func (s *Server) IngestTelemetry(w http.ResponseWriter, r *http.Request) {
	var t schema.Telemetry
	if err := decodeBody(w, r, &t); err != nil {
		writeProblem(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	result, httpErr := s.ingestOne(r.Context(), &t)
	if httpErr != nil {
		writeProblem(w, httpErr.status, httpErr.code, httpErr.message)
		return
	}
	s.writeIngestResponse(w, []schema.IngestItemResult{result})
}

// IngestTelemetryBatch handles 1..maxBatched canonical messages.
func (s *Server) IngestTelemetryBatch(w http.ResponseWriter, r *http.Request) {
	var items []schema.Telemetry
	if err := decodeBody(w, r, &items); err != nil {
		writeProblem(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	if len(items) == 0 || len(items) > s.cfg.MaxBatchSize {
		writeProblem(w, http.StatusBadRequest, "invalid_batch",
			fmt.Sprintf("Batch must contain 1 to %d readings", s.cfg.MaxBatchSize))
		return
	}
	results := make([]schema.IngestItemResult, 0, len(items))
	for i := range items {
		result, httpErr := s.ingestOne(r.Context(), &items[i])
		if httpErr != nil {
			// Per-item schema violations surface as rejected results so a
			// gateway can reconcile one message without losing the batch.
			results = append(results, schema.IngestItemResult{
				MessageID: items[i].MessageID,
				Status:    schema.StatusRejected,
				Code:      schema.Str(schema.CodeInvalidValue),
				Message:   schema.Str(httpErr.message),
			})
			continue
		}
		results = append(results, result)
	}
	s.writeIngestResponse(w, results)
}

func (s *Server) writeIngestResponse(w http.ResponseWriter, results []schema.IngestItemResult) {
	writeJSON(w, http.StatusAccepted, schema.IngestResponse{
		ReceivedAt: s.now().UTC(),
		Results:    results,
	})
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
	dec.UseNumber()
	if err := dec.Decode(dst); err != nil {
		return errors.New("malformed JSON body")
	}
	return nil
}

func (s *Server) ingestOne(ctx context.Context, t *schema.Telemetry) (schema.IngestItemResult, *httpError) {
	t.Quality = schema.NormalizeQuality(t.Quality)
	if err := t.Validate(); err != nil {
		return schema.IngestItemResult{}, &httpError{http.StatusUnprocessableEntity, "schema_error", err.Error()}
	}

	tag, err := store.GetTag(ctx, s.db, t.TagID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !tag.Active) {
		return reject(t.MessageID, schema.CodeUnknownTag, "Tag is not registered or inactive"), nil
	}
	if err != nil {
		return schema.IngestItemResult{}, &httpError{http.StatusInternalServerError, "internal", err.Error()}
	}
	if tag.AssetID != t.AssetID {
		return reject(t.MessageID, schema.CodeAssetTagMismatch, "Tag does not belong to asset"), nil
	}
	if tag.Unit != t.Unit {
		return reject(t.MessageID, schema.CodeUnitMismatch, fmt.Sprintf("Expected unit %s", tag.Unit)), nil
	}

	// Quality "offline" / "stale" mark a gap in the source, not a real
	// measurement: the value carries no signal and is not validated against
	// the tag data type. The historian records the gap; consumers flag the
	// point accordingly. A nil value is stored as the tag's zero value.
	quality := t.Quality
	isGap := quality == schema.QualityOffline || quality == schema.QualityStale || quality == schema.QualitySubstituted
	var normalized *schema.Normalized
	if isGap {
		normalized = gapNormalized(tag.DataType)
	} else {
		normalized, err = t.NormalizeValue(tag.DataType)
		if err != nil {
			return schema.IngestItemResult{}, &httpError{http.StatusUnprocessableEntity, schema.CodeInvalidValue, err.Error()}
		}
	}

	reading := &store.Reading{
		ID:              store.NewID(),
		MessageID:       t.MessageID,
		GatewayID:       t.GatewayID,
		SourceSequence:  t.SourceSequence,
		ObservedAt:      t.ObservedAt.UTC(),
		ReceivedAt:      s.now().UTC(),
		SentAt:          nil,
		AssetID:         t.AssetID,
		TagID:           t.TagID,
		ValueNumber:     normalized.Number,
		ValueBool:       boolPtrInt(normalized.Bool),
		ValueString:     normalized.Text,
		ValueStructured: normalized.Structured,
		Unit:            t.Unit,
		Quality:         t.Quality,
		ProfileID:       t.ProfileID,
	}
	if t.SentAt != nil {
		st := t.SentAt.UTC()
		reading.SentAt = &st
	}

	err = store.InsertReading(ctx, s.db, reading)
	if errors.Is(err, store.ErrDuplicate) {
		return schema.IngestItemResult{MessageID: t.MessageID, Status: schema.StatusDuplicate}, nil
	}
	if err != nil {
		return schema.IngestItemResult{}, &httpError{http.StatusInternalServerError, "internal", err.Error()}
	}

	s.publishLive(tag, reading)
	return schema.IngestItemResult{MessageID: t.MessageID, Status: schema.StatusAccepted}, nil
}

// publishLive fans an accepted reading out to dashboards: in-process hub
// subscribers first, then every configured cross-service event sink (the live
// service in split mode, see EVENT_SINKS).
func (s *Server) publishLive(tag *store.Tag, r *store.Reading) {
	ev := s.liveEvent(tag, r)
	s.hub.Broadcast(ev)
	s.forwardLiveEvent(ev)
}

var sinkClient = &http.Client{Timeout: 2 * time.Second}

// forwardLiveEvent delivers the event to every sink, best effort: a slow or
// down sink must never block or fail ingestion.
func (s *Server) forwardLiveEvent(body []byte) {
	for _, sink := range s.cfg.EventSinks {
		go func(u string) {
			resp, err := sinkClient.Post(u, "application/json", bytes.NewReader(body))
			if err != nil {
				log.Printf("event sink %s unreachable: %v", u, err)
				return
			}
			resp.Body.Close()
		}(sink)
	}
}

// InternalEvent accepts a forwarded live event (from the ingest service in
// split mode) and broadcasts it to this process's WebSocket subscribers.
func (s *Server) InternalEvent(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "malformed", "event body too large or unreadable")
		return
	}
	if len(bytes.TrimSpace(body)) == 0 {
		writeProblem(w, http.StatusBadRequest, "empty", "event body is required")
		return
	}
	s.hub.Broadcast(body)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "broadcast"})
}

// liveEvent exposes a stable dashboard event without leaking gateway internals.
func (s *Server) liveEvent(tag *store.Tag, r *store.Reading) []byte {
	ev := struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		AssetID   string `json:"asset_id"`
		TagID     string `json:"tag_id"`
		Stage     string `json:"stage"`
		Metric    string `json:"metric"`
		Value     any    `json:"value"`
		Unit      string `json:"unit"`
		Quality   string `json:"quality"`
		Alert     bool   `json:"alert"`
		Emergency bool   `json:"emergency"`
	}{
		Type: "telemetry", Timestamp: store.FormatUTC(r.ObservedAt),
		AssetID: r.AssetID, TagID: r.TagID,
		Stage: s.stageOf(r.AssetID), Metric: metricOf(r.TagID),
		Value: r.Value(),
		Unit:  r.Unit, Quality: r.Quality,
	}
	b, _ := json.Marshal(ev)
	return b
}

// stageOf resolves the asset area, loading the registry once and caching it.
func (s *Server) stageOf(assetID string) string {
	s.areasMu.RLock()
	area, ok := s.areas[assetID]
	s.areasMu.RUnlock()
	if ok {
		return area
	}

	assets, err := store.ListAssets(context.Background(), s.db)
	if err != nil {
		return ""
	}
	s.areasMu.Lock()
	if s.areas == nil {
		s.areas = make(map[string]string, len(assets))
	}
	for _, a := range assets {
		s.areas[a.ID] = a.Area
	}
	area = s.areas[assetID]
	s.areasMu.Unlock()
	return area
}

func reject(messageID, code, message string) schema.IngestItemResult {
	return schema.IngestItemResult{
		MessageID: messageID,
		Status:    schema.StatusRejected,
		Code:      schema.Str(code),
		Message:   schema.Str(message),
	}
}

func boolPtrInt(b *bool) *int64 {
	if b == nil {
		return nil
	}
	if *b {
		v := int64(1)
		return &v
	}
	v := int64(0)
	return &v
}

// ---------------------------------------------------------------------------
// Pull telemetry
// ---------------------------------------------------------------------------

// TelemetryHistory returns readings in descending observed_at order.
func (s *Server) TelemetryHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := store.ReadingFilter{
		TagID:   queryStr(r, "tag_id"),
		AssetID: queryStr(r, "asset_id"),
		Limit:   s.cfg.DefaultLimit,
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > s.cfg.MaxLimit {
			writeProblem(w, http.StatusBadRequest, "invalid_limit",
				fmt.Sprintf("limit must be in [1, %d]", s.cfg.MaxLimit))
			return
		}
		filter.Limit = n
	}
	if from, ok := parseTimeParam(q, "from_time", "from"); ok {
		filter.From = &from
	}
	if to, ok := parseTimeParam(q, "to_time", "to"); ok {
		filter.To = &to
	}

	readings, err := store.QueryReadings(r.Context(), s.db, filter)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toReadingResponses(readings))
}

// TelemetryLatest returns the newest reading for one tag, or JSON null. When
// the newest sample is older than STALENESS_SECONDS the payload is flagged as
// stale so consumers can render an alarm-like "no fresh data" state.
func (s *Server) TelemetryLatest(w http.ResponseWriter, r *http.Request) {
	tagID := queryStr(r, "tag_id")
	if tagID == "" {
		writeProblem(w, http.StatusBadRequest, "missing_param", "tag_id is required")
		return
	}
	reading, err := store.LatestReading(r.Context(), s.db, tagID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	resp := toReadingResponse(*reading)
	if s.cfg.StalenessSec > 0 {
		age := s.now().UTC().Sub(reading.ObservedAt)
		if age > s.cfg.StalenessSec {
			resp.Quality = schema.QualityStale
		}
		resp.Stale = age > s.cfg.StalenessSec
		resp.AgeSeconds = age.Seconds()
	}
	writeJSON(w, http.StatusOK, resp)
}

// TelemetryAggregate downsamples numeric readings into fixed time buckets.
// Supported resolutions are Go durations ("30s", "1m", "5m", "1h", "1d").
func (s *Server) TelemetryAggregate(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := store.AggregateFilter{
		TagID:    queryStr(r, "tag_id"),
		AssetID:  queryStr(r, "asset_id"),
		Function: queryStr(r, "agg"),
	}
	if filter.TagID == "" && filter.AssetID == "" {
		writeProblem(w, http.StatusBadRequest, "missing_param", "tag_id or asset_id is required")
		return
	}
	resolution, err := parseResolution(queryStr(r, "resolution"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_resolution",
			"resolution must be a duration like 30s, 1m, 5m, 1h or 1d")
		return
	}
	filter.Resolution = resolution
	if !store.ValidAggregate(filter.Function) {
		writeProblem(w, http.StatusBadRequest, "invalid_agg",
			"agg must be one of avg,min,max,sum,count,last")
		return
	}
	if from, ok := parseTimeParam(q, "from_time", "from"); ok {
		filter.From = &from
	}
	if to, ok := parseTimeParam(q, "to_time", "to"); ok {
		filter.To = &to
	}

	buckets, err := store.AggregateReadings(r.Context(), s.db, filter)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tag_id":     filter.TagID,
		"asset_id":   filter.AssetID,
		"resolution": resolution.String(),
		"function":   filter.Function,
		"buckets":    buckets,
	})
}

// ListAlerts returns persisted alert history.
func (s *Server) ListAlerts(w http.ResponseWriter, r *http.Request) {
	limit := s.cfg.DefaultLimit
	if v := queryStr(r, "limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= s.cfg.MaxLimit {
			limit = n
		}
	}
	alerts, err := store.ListAlerts(r.Context(), s.db, queryStr(r, "stage"), queryStr(r, "metric"), limit)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, alerts)
}

// LatestAlert returns the most recent alert or JSON null.
func (s *Server) LatestAlert(w http.ResponseWriter, r *http.Request) {
	alerts, err := store.ListAlerts(r.Context(), s.db, "", "", 1)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if len(alerts) == 0 {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, alerts[0])
}

// ---------------------------------------------------------------------------
// Simulator controls (development only)
// ---------------------------------------------------------------------------

// SimulatorStatus reports generator state.
func (s *Server) SimulatorStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"running":         s.sim.IsRunning(),
		"interval":        s.cfg.SimInterval.Seconds(),
		"telemetry_count": s.sim.Count(),
		"emergency":       s.sim.IsEmergency(),
	})
}

// SimulatorStart launches the generator.
func (s *Server) SimulatorStart(w http.ResponseWriter, r *http.Request) {
	s.sim.Start(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"status": "running"})
}

// SimulatorStop halts the generator.
func (s *Server) SimulatorStop(w http.ResponseWriter, r *http.Request) {
	s.sim.Stop()
	writeJSON(w, http.StatusOK, map[string]any{"status": "stopped"})
}

// SimulatorEmergency activates failure simulation.
func (s *Server) SimulatorEmergency(w http.ResponseWriter, r *http.Request) {
	s.sim.TriggerEmergency()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "emergency_active",
		"duration_seconds": s.cfg.EmergencySec.Seconds(),
	})
}

// SimulatorEmergencyStop deactivates failure simulation.
func (s *Server) SimulatorEmergencyStop(w http.ResponseWriter, r *http.Request) {
	s.sim.EndEmergency()
	writeJSON(w, http.StatusOK, map[string]any{"status": "emergency_stopped"})
}

// ---------------------------------------------------------------------------
// WebSocket
// ---------------------------------------------------------------------------

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(*http.Request) bool { return true },
}

// WebSocket subscribes a dashboard to live events and accepts emergency commands.
func (s *Server) WebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := s.hub.Register()
	defer client.Close()

	go func() {
		defer conn.Close()
		for msg := range client.Messages() {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var cmd map[string]any
		if json.Unmarshal(data, &cmd) != nil {
			continue
		}
		switch cmd["type"] {
		case "trigger_emergency":
			s.sim.TriggerEmergency()
		case "stop_emergency":
			s.sim.EndEmergency()
		}
	}
}

// ---------------------------------------------------------------------------
// Serialization helpers
// ---------------------------------------------------------------------------

type readingResponse struct {
	MessageID      string     `json:"message_id"`
	GatewayID      string     `json:"gateway_id"`
	SourceSequence *int64     `json:"source_sequence"`
	ObservedAt     time.Time  `json:"observed_at"`
	ReceivedAt     time.Time  `json:"received_at"`
	SentAt         *time.Time `json:"sent_at"`
	AssetID        string     `json:"asset_id"`
	TagID          string     `json:"tag_id"`
	Value          any        `json:"value"`
	Unit           string     `json:"unit"`
	Quality        string     `json:"quality"`
	ProfileID      *string    `json:"profile_id"`
	Stale          bool       `json:"stale,omitempty"`
	AgeSeconds     float64    `json:"age_seconds,omitempty"`
}

func toReadingResponse(r store.Reading) readingResponse {
	return readingResponse{
		MessageID:      r.MessageID,
		GatewayID:      r.GatewayID,
		SourceSequence: r.SourceSequence,
		ObservedAt:     r.ObservedAt,
		ReceivedAt:     r.ReceivedAt,
		SentAt:         r.SentAt,
		AssetID:        r.AssetID,
		TagID:          r.TagID,
		Value:          r.Value(),
		Unit:           r.Unit,
		Quality:        r.Quality,
		ProfileID:      r.ProfileID,
	}
}

func toReadingResponses(rs []store.Reading) []readingResponse {
	out := make([]readingResponse, 0, len(rs))
	for _, r := range rs {
		out = append(out, toReadingResponse(r))
	}
	return out
}

func parseTimeParam(q url.Values, keys ...string) (time.Time, bool) {
	for _, k := range keys {
		if v := q.Get(k); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

// parseResolution accepts Go durations plus the "1d" calendar day alias.
func parseResolution(s string) (time.Duration, error) {
	switch s {
	case "", "1d", "24h":
		return 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func metricOf(tagID string) string {
	seg := strings.Split(tagID, ".")
	return seg[len(seg)-1]
}
