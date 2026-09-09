package gateway

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"cap/internal/schema"
)

// sparkplugSource consumes an existing Sparkplug B deployment over MQTT in the
// Primary Host Application role: it subscribes to spBv1.0/# and never
// publishes, which keeps the read-only invariant (ADR-001) trivially true.
//
// Topic namespace (Sparkplug 3.0):
//
//	spBv1.0/{group_id}/{message_type}/{edge_node_id}[/{device_id}]
//
// NBIRTH/DBIRTH record the alias->name maps and may already carry values;
// NDATA/DDATA carry the deltas (name omitted, alias only). Messages for
// unmapped metrics are ignored; STATE/NDEATH/REBIRTH are not needed by a
// monitoring-only subscriber and are skipped.
//
// Tag spec syntax (TAGS entry suffix, after "|"):
//
//	sp={group_id}/{edge_node_id}/<metric_name>                  (edge metrics)
//	sp={group_id}/{edge_node_id}/{device_id}/<metric_name>      (device metrics)
//
// The payload decoder below implements the Sparkplug B protobuf wire format
// by hand (see sparkplug_b.proto: Payload/Metric field numbers are stable and
// frozen by the specification) so no protobuf codegen joins the build.
type sparkplugSource struct {
	cfg *Config

	mu       sync.Mutex
	closed   bool
	deltas   []schema.Telemetry // drained by Poll, appended by MQTT callbacks
	tagByKey map[string]int     // group/edge[/device]/metric -> cfg.Tags index
	aliases  map[string]map[uint64]string
	client   mqtt.Client
}

var _ Source = (*sparkplugSource)(nil)

// spbMetric is one decoded Sparkplug B metric.
type spbMetric struct {
	name    string
	alias   uint64
	hasName bool
	isNull  bool
	tsMs    uint64
	value   any
}

// ---------------------------------------------------------------------------
// Protobuf wire decoding (sparkplug_b.proto subset)
// ---------------------------------------------------------------------------

// Wire types (protowire enum).
const (
	wireVarint  = 0
	wireFixed64 = 1
	wireBytes   = 2
	wireFixed32 = 5
)

// Field numbers frozen by the Sparkplug B specification.
const (
	payloadFieldTimestamp = 1
	payloadFieldMetrics   = 2
	payloadFieldSeq       = 3

	metricFieldName   = 1
	metricFieldAlias  = 2
	metricFieldTs     = 3
	metricFieldIsNull = 7
	metricFieldInt    = 11
	metricFieldLong   = 12
	metricFieldFloat  = 13
	metricFieldDouble = 14
	metricFieldBool   = 15
	metricFieldString = 16
)

// pbReader walks protobuf wire data without reflection.
type pbReader struct {
	b   []byte
	err error
}

func (r *pbReader) done() bool { return r.err != nil || len(r.b) == 0 }

// next reads one tag: field number and wire type.
func (r *pbReader) next() (num uint32, wt uint8) {
	if r.err != nil {
		return 0, 0
	}
	key, err := r.varint()
	if err != nil {
		r.err = err
		return 0, 0
	}
	return uint32(key >> 3), uint8(key & 0x7)
}

func (r *pbReader) varint() (uint64, error) {
	if r.err != nil {
		return 0, r.err
	}
	v, n := binary.Uvarint(r.b)
	if n <= 0 {
		r.err = errors.New("truncated varint")
		return 0, r.err
	}
	r.b = r.b[n:]
	return v, nil
}

// skip consumes the payload of an uninteresting field.
func (r *pbReader) skip(wt uint8) {
	switch wt {
	case wireVarint:
		_, _ = r.varint()
	case wireFixed64:
		r.cut(8)
	case wireBytes:
		if n, err := r.varint(); err == nil {
			r.cut(int(n))
		}
	case wireFixed32:
		r.cut(4)
	default:
		if r.err == nil {
			r.err = fmt.Errorf("unsupported wire type %d", wt)
		}
	}
}

func (r *pbReader) cut(n int) {
	if r.err != nil {
		return
	}
	if n < 0 || n > len(r.b) {
		r.err = errors.New("truncated field")
		return
	}
	r.b = r.b[n:]
}

// sub returns a reader over a length-delimited field's payload.
func (r *pbReader) sub() pbReader {
	n, err := r.varint()
	if err != nil {
		r.err = err
		return pbReader{err: err}
	}
	size := int(n)
	if size > len(r.b) {
		r.err = errors.New("truncated submessage")
		return pbReader{err: r.err}
	}
	s := pbReader{b: r.b[:size]}
	r.b = r.b[size:]
	return s
}

// decodePayload decodes a Sparkplug B Payload message.
func decodePayload(raw []byte) (tsMs, seq uint64, seqSet bool, metrics []spbMetric, err error) {
	r := pbReader{b: raw}
	for !r.done() {
		num, wt := r.next()
		switch num {
		case payloadFieldTimestamp:
			tsMs, _ = r.varint()
		case payloadFieldSeq:
			seq, _ = r.varint()
			seqSet = true
		case payloadFieldMetrics:
			s := r.sub()
			m, derr := decodeMetric(&s)
			if derr != nil {
				return 0, 0, false, nil, derr
			}
			metrics = append(metrics, m)
		default:
			r.skip(wt)
		}
	}
	return tsMs, seq, seqSet, metrics, r.err
}

// decodeMetric decodes one Metric submessage. Numeric int_value fields carry
// Int8/Int16/Int32 (two's complement in 32 bits); long_value carries the
// unsigned 64-bit family per the Sparkplug type convention.
func decodeMetric(r *pbReader) (spbMetric, error) {
	var m spbMetric
	for !r.done() {
		num, wt := r.next()
		switch num {
		case metricFieldName:
			s := r.sub()
			m.name, m.hasName = string(s.b), true
		case metricFieldAlias:
			m.alias, _ = r.varint()
		case metricFieldTs:
			m.tsMs, _ = r.varint()
		case metricFieldIsNull:
			v, _ := r.varint()
			m.isNull = v != 0
		case metricFieldInt:
			v, _ := r.varint()
			m.value = float64(int64(int32(uint32(v))))
		case metricFieldLong:
			v, _ := r.varint()
			m.value = float64(v)
		case metricFieldFloat:
			if len(r.b) < 4 {
				r.err = errors.New("truncated float_value")
				break
			}
			m.value = float64(math.Float32frombits(binary.LittleEndian.Uint32(r.b[:4])))
			r.b = r.b[4:]
		case metricFieldDouble:
			if len(r.b) < 8 {
				r.err = errors.New("truncated double_value")
				break
			}
			m.value = math.Float64frombits(binary.LittleEndian.Uint64(r.b[:8]))
			r.b = r.b[8:]
		case metricFieldBool:
			v, _ := r.varint()
			m.value = v != 0
		case metricFieldString:
			s := r.sub()
			m.value = string(s.b)
		default:
			r.skip(wt)
		}
	}
	return m, r.err
}

// decodeFixed reads a little-endian fixed field and remembers it.
func decodeFixed32(b []byte) uint32 { return binary.LittleEndian.Uint32(b) }

// ---------------------------------------------------------------------------

// newSparkplugSource validates the config, builds the tag index and connects
// to the broker. MQTT delivery starts asynchronously: a broker outage at
// startup is not fatal, the driver simply yields no deltas until it connects.
func newSparkplugSource(cfg *Config) (Source, error) {
	if cfg.MQTTBroker == "" {
		return nil, errors.New("MQTT_BROKER is required when SOURCE_DRIVER=sparkplug")
	}
	s := &sparkplugSource{cfg: cfg}
	if err := s.buildTagIndex(); err != nil {
		return nil, err
	}
	s.aliases = make(map[string]map[uint64]string)

	clientID := cfg.MQTTClientID
	if clientID == "" {
		clientID = "cap-gw-" + cfg.ID
	}
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.MQTTBroker).
		SetClientID(clientID).
		SetCleanSession(false). // broker queues QoS1 data while we are offline
		SetAutoReconnect(true).
		SetResumeSubs(true).
		SetOrderMatters(false)
	if cfg.MQTTUsername != "" {
		opts.SetUsername(cfg.MQTTUsername)
		opts.SetPassword(cfg.MQTTPassword)
	}
	opts.OnConnectionLost = func(_ mqtt.Client, err error) {
		log.Printf("[gateway/sparkplug] connection lost: %v (auto-reconnect)", err)
	}
	opts.OnConnect = func(c mqtt.Client) {
		if tok := c.Subscribe("spBv1.0/#", 1, s.onMessage); tok.Wait() && tok.Error() != nil {
			log.Printf("[gateway/sparkplug] subscribe failed: %v", tok.Error())
			return
		}
		log.Printf("[gateway/sparkplug] subscribed to spBv1.0/# at %s", cfg.MQTTBroker)
	}
	s.client = mqtt.NewClient(opts)
	if tok := s.client.Connect(); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		log.Printf("[gateway/sparkplug] initial connect to %s pending/failed: %v (will retry)",
			cfg.MQTTBroker, tok.Error())
	}
	return s, nil
}

// Name implements Source.
func (s *sparkplugSource) Name() string { return DriverSparkplug }

// buildTagIndex parses every tag's Sparkplug spec and maps the canonical key
// (group/edge[/device]/metric) to its TagSpec index.
func (s *sparkplugSource) buildTagIndex() error {
	s.tagByKey = make(map[string]int, len(s.cfg.Tags))
	for i, t := range s.cfg.Tags {
		if t.Sparkplug == "" {
			return fmt.Errorf("tag %q is missing a sparkplug spec (expected ...:unit|sp=group/edge/metric)", t.TagID)
		}
		key, err := canonicalSparkplugKey(t.Sparkplug)
		if err != nil {
			return fmt.Errorf("tag %q: %w", t.TagID, err)
		}
		if _, dup := s.tagByKey[key]; dup {
			return fmt.Errorf("duplicate sparkplug spec %q", t.Sparkplug)
		}
		s.tagByKey[key] = i
	}
	return nil
}

// canonicalSparkplugKey validates "group/edge[/device]/metric" and returns it
// normalized to group/edge[/device]/metric.
func canonicalSparkplugKey(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	parts := strings.Split(spec, "/")
	if len(parts) != 3 && len(parts) != 4 {
		return "", fmt.Errorf("sparkplug spec %q must be group/edge/metric or group/edge/device/metric", spec)
	}
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			return "", fmt.Errorf("sparkplug spec %q has an empty segment", spec)
		}
	}
	return spec, nil
}

// Poll drains the delta buffer accumulated by MQTT callbacks.
func (s *sparkplugSource) Poll(ctx context.Context, now time.Time) ([]schema.Telemetry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("sparkplug source closed")
	}
	out := s.deltas
	s.deltas = nil
	return out, nil
}

// onMessage is the MQTT callback: it delegates to handleTopic. It runs on the
// paho goroutine and must never block, so decode work is kept minimal and
// unbounded growth of the delta buffer is prevented by a hard cap.
func (s *sparkplugSource) onMessage(_ mqtt.Client, msg mqtt.Message) {
	s.handleTopic(msg.Topic(), msg.Payload())
}

// handleTopic parses a Sparkplug topic, decodes the payload and buffers
// canonical deltas for the next Poll.
func (s *sparkplugSource) handleTopic(rawTopic string, payload []byte) {
	seg := strings.Split(rawTopic, "/")
	// spBv1.0 / group / type / edge [/ device]
	if len(seg) < 4 || seg[0] != "spBv1.0" {
		return
	}
	group, msgType, edge := seg[1], seg[2], seg[3]
	device := ""
	if len(seg) >= 5 {
		device = seg[4]
	}
	switch msgType {
	case "NBIRTH", "DBIRTH":
		s.handleBirth(group, edge, device, payload)
	case "NDATA", "DDATA":
		s.handleData(group, edge, device, payload)
	}
}

// handleBirth resets the node's alias map, re-ties every named metric to its
// alias and emits the birth-time values.
func (s *sparkplugSource) handleBirth(group, edge, device string, payload []byte) {
	nodeKey := strings.Join(nonEmpty(group, edge, device), "/")
	tsMs, seq, seqSet, metrics, err := decodePayload(payload)
	if err != nil {
		log.Printf("[gateway/sparkplug] %s birth decode: %v", nodeKey, err)
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	aliases := make(map[uint64]string, len(metrics))
	for i := range metrics {
		if metrics[i].hasName && metrics[i].alias != 0 {
			aliases[metrics[i].alias] = metrics[i].name
		}
	}
	s.aliases[nodeKey] = aliases
	s.mu.Unlock()
	s.handlePayload(group, edge, device, tsMs, seq, seqSet, metrics)
}

// handleData buffers deltas for a DATA message, resolving aliases from the
// most recent birth.
func (s *sparkplugSource) handleData(group, edge, device string, payload []byte) {
	tsMs, seq, seqSet, metrics, err := decodePayload(payload)
	if err != nil {
		log.Printf("[gateway/sparkplug] %s data decode: %v", strings.Join(nonEmpty(group, edge, device), "/"), err)
		return
	}
	s.handlePayload(group, edge, device, tsMs, seq, seqSet, metrics)
}

// handlePayload converts decoded metrics into canonical telemetry and buffers
// them. Shared by birth and data paths.
func (s *sparkplugSource) handlePayload(group, edge, device string, tsMs, seq uint64, seqSet bool, metrics []spbMetric) {
	nodeKey := strings.Join(nonEmpty(group, edge, device), "/")
	prefix := group + "/" + edge
	if device != "" {
		prefix += "/" + device
	}
	publishAt := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	for i := range metrics {
		m := &metrics[i]
		name := m.name
		if !m.hasName {
			if alias := s.aliases[nodeKey]; alias != nil {
				name = alias[m.alias]
			}
			if name == "" {
				continue // unknown alias: metric was never birthed to us
			}
		}
		idx, ok := s.tagByKey[prefix+"/"+name]
		if !ok {
			continue // not a monitored tag
		}
		if m.isNull || m.value == nil {
			continue // null metric: recorded as a gap, not a fabricated value
		}
		t := s.basicTelemetry(idx, publishAt)
		if m.tsMs > 0 {
			t.ObservedAt = time.UnixMilli(int64(m.tsMs)).UTC()
		} else if tsMs > 0 {
			t.ObservedAt = time.UnixMilli(int64(tsMs)).UTC()
		}
		if seqSet {
			sq := int64(seq)
			t.SourceSequence = &sq
		}
		t.Value = m.value
		s.deltas = append(s.deltas, t)
	}
	// Hard cap: a flooding publisher must not grow this buffer without bound.
	const maxBuffered = 8192
	if len(s.deltas) > maxBuffered {
		log.Printf("[gateway/sparkplug] delta buffer overflow, dropping %d oldest",
			len(s.deltas)-maxBuffered)
		s.deltas = append([]schema.Telemetry(nil), s.deltas[len(s.deltas)-maxBuffered:]...)
	}
}

func nonEmpty(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// basicTelemetry fills the invariant fields shared by all paths.
func (s *sparkplugSource) basicTelemetry(i int, now time.Time) schema.Telemetry {
	spec := s.cfg.Tags[i]
	return schema.Telemetry{
		SchemaVersion:  schema.SchemaVersion,
		MessageID:      schema.NewUUID(),
		GatewayID:      s.cfg.ID,
		ObservedAt:     now.UTC(),
		SentAt:         &now,
		AssetID:        spec.AssetID,
		TagID:          spec.TagID,
		Unit:           spec.Unit,
		Quality:        schema.QualityGood,
		SourceSequence: nil,
	}
}

// Close implements Source; idempotent.
func (s *sparkplugSource) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.client != nil && s.client.IsConnectionOpen() {
		s.client.Disconnect(250)
	}
	return nil
}
