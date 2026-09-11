// Package api implements the HTTP API (push ingestion, pull queries, registry,
// control, alarms, metallurgy, health, WebSocket live fan-out) on top of the
// store, the alarm engine and the control manager.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"cap/internal/alarms"
	"cap/internal/config"
	"cap/internal/controlsvc"
	"cap/internal/hub"
	"cap/internal/schema"
	"cap/internal/store"
)

// Server wires handlers to shared dependencies.
type Server struct {
	db     *sql.DB
	hub    *hub.Hub
	cfg    *config.Settings
	alarms *alarms.Engine
	loops  *controlsvc.Manager
	now    func() time.Time
}

// New constructs a Server. alarms and loops may be nil in tests that do not
// exercise those paths.
func New(db *sql.DB, h *hub.Hub, cfg *config.Settings, eng *alarms.Engine, loops *controlsvc.Manager) *Server {
	return &Server{db: db, hub: h, cfg: cfg, alarms: eng, loops: loops, now: time.Now}
}

// SetLoops attaches the control manager after construction (it needs the
// server's ingest callback, so the wiring is two-phase in service.Run).
func (s *Server) SetLoops(m *controlsvc.Manager) { s.loops = m }

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
// Ingestion (push) — the single telemetry entry point of the platform
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

// IngestCanonical is the exported single telemetry entry point. The
// metallurgy calc service writes its virtual tags through this path so they
// are validated, stored and broadcast exactly like field measurements.
func (s *Server) IngestCanonical(ctx context.Context, t *schema.Telemetry) error {
	result, httpErr := s.ingestOne(ctx, t)
	if httpErr != nil {
		return fmt.Errorf("%s: %s", httpErr.code, httpErr.message)
	}
	if result.Status != schema.StatusAccepted && result.Status != schema.StatusDuplicate {
		return fmt.Errorf("ingest: %s", result.Status)
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

	// Fan out to dashboards, then to the alarm engine (TZ §11).
	s.hub.Broadcast(s.liveEvent(tag, reading))
	if s.alarms != nil {
		s.alarms.Evaluate(ctx, tag, reading)
	}
	return schema.IngestItemResult{MessageID: t.MessageID, Status: schema.StatusAccepted}, nil
}

// liveEvent exposes the dashboard telemetry event (TZ §13).
func (s *Server) liveEvent(tag *store.Tag, r *store.Reading) []byte {
	ev := struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		AssetID   string `json:"asset_id"`
		TagID     string `json:"tag_id"`
		Value     any    `json:"value"`
		Unit      string `json:"unit"`
		Quality   string `json:"quality"`
	}{
		Type: "telemetry", Timestamp: store.FormatUTC(r.ObservedAt),
		AssetID: r.AssetID, TagID: r.TagID,
		Value: r.Value(), Unit: r.Unit, Quality: r.Quality,
	}
	b, _ := json.Marshal(ev)
	return b
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

// ---------------------------------------------------------------------------
// WebSocket
// ---------------------------------------------------------------------------

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(*http.Request) bool { return true },
}

// WebSocket subscribes a dashboard to live events. Clients receive telemetry,
// alarm and loop_state events; there are no client→server commands (all
// control actions go through the audited REST API).
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
		if _, _, err := conn.ReadMessage(); err != nil {
			return
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
