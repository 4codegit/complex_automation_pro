// Package schema defines the canonical telemetry contract shared by the server
// and every edge gateway, plus the response payloads of the HTTP API.
package schema

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// SchemaVersion is the canonical message contract version.
const SchemaVersion = "1.0"

// Quality values follow the contract in TECHNICAL_SPECIFICATION.md.
const (
	QualityGood        = "good"
	QualityUncertain   = "uncertain"
	QualityBad         = "bad"
	QualityStale       = "stale"
	QualitySubstituted = "substituted"
	QualityOffline     = "offline"
)

var validQualities = map[string]bool{
	QualityGood: true, QualityUncertain: true, QualityBad: true,
	QualityStale: true, QualitySubstituted: true, QualityOffline: true,
}

// Value kinds a tag can carry.
const (
	KindNumber     = "number"
	KindBoolean    = "boolean"
	KindString     = "string"
	KindStructured = "structured"
)

// Telemetry is the canonical push payload. Value is a JSON number, boolean,
// string, or nested object/array (structured); the tag registry defines which
// kind a tag expects.
type Telemetry struct {
	SchemaVersion  string     `json:"schema_version"`
	MessageID      string     `json:"message_id"`
	GatewayID      string     `json:"gateway_id"`
	SourceSequence *int64     `json:"source_sequence,omitempty"`
	ObservedAt     time.Time  `json:"observed_at"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
	AssetID        string     `json:"asset_id"`
	TagID          string     `json:"tag_id"`
	Value          any        `json:"value"`
	Unit           string     `json:"unit"`
	Quality        string     `json:"quality"`
	ProfileID      *string    `json:"profile_id,omitempty"`
}

// Normalized holds the typed view of a Telemetry value for storage.
type Normalized struct {
	Kind       string
	Number     *float64
	Bool       *bool
	Text       *string
	Structured *string // compact JSON for structured values
}

// NormalizeValue classifies and validates the telemetry value against the tag
// data type. dataType is one of KindNumber|KindBoolean|KindString|KindStructured.
func (t *Telemetry) NormalizeValue(dataType string) (*Normalized, error) {
	kind := strings.ToLower(dataType)
	switch v := t.Value.(type) {
	case float64:
		if kind != "" && kind != KindNumber {
			return nil, fmt.Errorf("value is a number but tag expects %q", kind)
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("value must be finite")
		}
		return &Normalized{Kind: KindNumber, Number: &v}, nil
	case json.Number:
		f, err := v.Float64()
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("value must be a finite number")
		}
		if kind != "" && kind != KindNumber {
			return nil, fmt.Errorf("value is a number but tag expects %q", kind)
		}
		return &Normalized{Kind: KindNumber, Number: &f}, nil
	case bool:
		if kind != "" && kind != KindBoolean {
			return nil, fmt.Errorf("value is a boolean but tag expects %q", kind)
		}
		return &Normalized{Kind: KindBoolean, Bool: &v}, nil
	case string:
		if kind != "" && kind != KindString {
			return nil, fmt.Errorf("value is a string but tag expects %q", kind)
		}
		return &Normalized{Kind: KindString, Text: &v}, nil
	default:
		if kind != "" && kind != KindStructured {
			return nil, fmt.Errorf("value is structured but tag expects %q", kind)
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("structured value is not JSON serializable: %w", err)
		}
		s := string(raw)
		return &Normalized{Kind: KindStructured, Structured: &s}, nil
	}
}

// Validate checks the canonical message shape. Registry-level consistency
// (asset/tag/unit) is enforced by the ingest layer.
func (t *Telemetry) Validate() error {
	if t.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version %q", t.SchemaVersion)
	}
	if t.MessageID == "" {
		return fmt.Errorf("message_id is required")
	}
	if len(t.MessageID) > 64 {
		return fmt.Errorf("message_id too long")
	}
	if t.GatewayID == "" || len(t.GatewayID) > 120 {
		return fmt.Errorf("gateway_id must be 1..120 chars")
	}
	if t.AssetID == "" || len(t.AssetID) > 160 {
		return fmt.Errorf("asset_id must be 1..160 chars")
	}
	if t.TagID == "" || len(t.TagID) > 200 {
		return fmt.Errorf("tag_id must be 1..200 chars")
	}
	if t.Unit == "" || len(t.Unit) > 32 {
		return fmt.Errorf("unit must be 1..32 chars")
	}
	if !validQualities[t.Quality] {
		return fmt.Errorf("unsupported quality %q", t.Quality)
	}
	if t.ObservedAt.IsZero() {
		return fmt.Errorf("observed_at is required")
	}
	return nil
}

// Ingest statuses and rejection codes for the push contract.
const (
	StatusAccepted  = "accepted"
	StatusDuplicate = "duplicate"
	StatusRejected  = "rejected"

	CodeUnknownTag       = "unknown_tag"
	CodeAssetTagMismatch = "asset_tag_mismatch"
	CodeUnitMismatch     = "unit_mismatch"
	CodeInvalidValue     = "invalid_value"
)

// IngestItemResult mirrors the Python contract result for one message.
type IngestItemResult struct {
	MessageID string  `json:"message_id"`
	Status    string  `json:"status"`
	Code      *string `json:"code"`
	Message   *string `json:"message"`
}

// IngestResponse is returned for single and batch ingestion.
type IngestResponse struct {
	ReceivedAt time.Time          `json:"received_at"`
	Results    []IngestItemResult `json:"results"`
}

// Str returns a pointer to s (helper for optional JSON fields).
func Str(s string) *string { return &s }

// Gateway event kinds.
const (
	GatewayEventPulse   = "pulse"
	GatewayEventOnline  = "online"
	GatewayEventOffline = "offline"
)

// GatewayEvent is the heartbeat contract between an edge gateway and the
// server. It carries connection state, local buffer depth and delivery latency
// so the platform can render per-gateway health without polling the field.
type GatewayEvent struct {
	GatewayID  string    `json:"gateway_id"`
	Event      string    `json:"event"`
	Status     string    `json:"status"`
	BufferSize int64     `json:"buffer_size"`
	LatencyMs  int64     `json:"latency_ms"`
	Version    string    `json:"version"`
	Timestamp  time.Time `json:"timestamp"`
}

// Validate checks the heartbeat contract shape.
func (e *GatewayEvent) Validate() error {
	if e.GatewayID == "" || len(e.GatewayID) > 120 {
		return fmt.Errorf("gateway_id must be 1..120 chars")
	}
	if e.Event == "" {
		return fmt.Errorf("event is required")
	}
	if e.Event != GatewayEventPulse && e.Event != GatewayEventOnline && e.Event != GatewayEventOffline {
		return fmt.Errorf("unsupported event %q", e.Event)
	}
	if e.Status == "" {
		return fmt.Errorf("status is required")
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	return nil
}

// NormalizeQuality trims and lower-cases a quality code from a client.
func NormalizeQuality(q string) string { return strings.ToLower(strings.TrimSpace(q)) }

// NewUUID returns a random RFC 4122 v4 UUID string.
func NewUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
