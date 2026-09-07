package store

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Asset is an equipment node in the plant hierarchy.
type Asset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Area        string `json:"area"`
	Criticality string `json:"criticality"`
	Active      bool   `json:"active"`
}

// Tag is a registered signal (one sensor / derived value). This is the single
// abstraction for "any sensor": new devices are registry rows, not code.
type Tag struct {
	ID                      string   `json:"id"`
	AssetID                 string   `json:"asset_id"`
	Name                    string   `json:"name"`
	Unit                    string   `json:"unit"`
	DataType                string   `json:"data_type"` // number | boolean | string | structured
	EngineeringMin          *float64 `json:"engineering_min"`
	EngineeringMax          *float64 `json:"engineering_max"`
	SamplingIntervalSeconds float64  `json:"sampling_interval_seconds"`
	Criticality             string   `json:"criticality"`
	Active                  bool     `json:"active"`
}

// Reading is one stored canonical measurement.
type Reading struct {
	ID              string
	MessageID       string
	GatewayID       string
	SourceSequence  *int64
	ObservedAt      time.Time
	ReceivedAt      time.Time
	SentAt          *time.Time
	AssetID         string
	TagID           string
	ValueNumber     *float64
	ValueBool       *int64 // 0/1 for DB portability
	ValueString     *string
	ValueStructured *string
	Unit            string
	Quality         string
	ProfileID       *string
}

// Value exposes the typed value for API serialization.
func (r *Reading) Value() any {
	switch {
	case r.ValueNumber != nil:
		return *r.ValueNumber
	case r.ValueBool != nil:
		return *r.ValueBool == 1
	case r.ValueString != nil:
		return *r.ValueString
	case r.ValueStructured != nil:
		return *r.ValueStructured
	default:
		return nil
	}
}

// Alert is a persisted alarm/event row.
type Alert struct {
	ID        string    `json:"id"`
	Stage     string    `json:"stage"`
	Metric    string    `json:"metric"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// Alarm states (simplified ISA-18.2 lifecycle).
const (
	AlarmStateActiveUnack = "active_unacknowledged"
	AlarmStateActiveAck   = "active_acknowledged"
	AlarmStateNormal      = "returned_to_normal"
	AlarmStateShelved     = "shelved"
)

// Alarm is the current alarm state for one tag (one row per tag, upserted).
type Alarm struct {
	ID           string     `json:"id"`
	TagID        string     `json:"tag_id"`
	Metric       string     `json:"metric"`
	State        string     `json:"state"`
	Severity     string     `json:"severity"`
	Priority     int        `json:"priority"`
	Message      string     `json:"message"`
	ObservedAt   time.Time  `json:"observed_at"`
	AckBy        *string    `json:"ack_by"`
	AckAt        *time.Time `json:"ack_at"`
	AckComment   *string    `json:"ack_comment"`
	ShelvedUntil *time.Time `json:"shelved_until"`
	ClearedAt    *time.Time `json:"cleared_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Profile statuses.
const (
	ProfileStatusDraft      = "draft"
	ProfileStatusApproved   = "approved"
	ProfileStatusActive     = "active"
	ProfileStatusSuperseded = "superseded"
)

// Profile is a versioned set of process parameters for one ore type. Params is
// a JSON string; it overrides per-metric simulator/config operating points.
type Profile struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	OreDomain     string     `json:"ore_domain"`
	Version       int        `json:"version"`
	Params        string     `json:"params"`
	EffectiveFrom *time.Time `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
	Author        string     `json:"author"`
	ApprovedBy    *string    `json:"approved_by"`
	ApprovedAt    *time.Time `json:"approved_at"`
	Status        string     `json:"status"`
	Reason        string     `json:"reason"`
	CreatedAt     time.Time  `json:"created_at"`
}

// NewID returns a random 128-bit hex identifier usable as a portable TEXT PK.
func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
