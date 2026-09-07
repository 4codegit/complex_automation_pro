package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	mathrand "math/rand/v2"
	"time"

	"autopro/internal/schema"
)

// Sensor polls local instruments and emits canonical messages. The simulated
// driver stands in for protocol adapters (Modbus TCP, OPC UA, MQTT Sparkplug B)
// so the demo works without field equipment.
type Sensor struct {
	gatewayID string
	tags      []TagSpec
	seq       int64
}

// NewSensor builds the demo driver from the configured tag specs.
func NewSensor(gatewayID string, tags []TagSpec) *Sensor {
	return &Sensor{gatewayID: gatewayID, tags: tags}
}

// Name implements Source.
func (s *Sensor) Name() string { return "simulated" }

// Close implements Source. The simulator holds no resources.
func (s *Sensor) Close() error { return nil }

// Poll reads every configured instrument once and returns canonical messages.
func (s *Sensor) Poll(ctx context.Context, now time.Time) ([]schema.Telemetry, error) {
	out := make([]schema.Telemetry, 0, len(s.tags))
	for _, spec := range s.tags {
		value := spec.Baseline + mathrand.NormFloat64()*spec.Noise
		value = math.Round(value*1000) / 1000

		t := schema.Telemetry{
			SchemaVersion:  schema.SchemaVersion,
			MessageID:      schema.NewUUID(),
			GatewayID:      s.gatewayID,
			SourceSequence: s.nextSeq(),
			ObservedAt:     now.UTC(),
			SentAt:         &now,
			AssetID:        spec.AssetID,
			TagID:          spec.TagID,
			Value:          value,
			Unit:           spec.Unit,
			Quality:        schema.QualityGood,
		}
		out = append(out, t)
	}
	return out, nil
}

func (s *Sensor) nextSeq() *int64 {
	s.seq++
	return &s.seq
}

// MarshalPayload renders one message as canonical JSON for the buffer.
func MarshalPayload(t *schema.Telemetry) (string, error) {
	raw, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("marshal telemetry: %w", err)
	}
	return string(raw), nil
}
