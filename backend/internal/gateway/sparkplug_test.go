package gateway

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"cap/internal/schema"
)

// ---------------------------------------------------------------------------
// Minimal protobuf wire encoder: builds golden Sparkplug B payloads for tests.
// ---------------------------------------------------------------------------

func pbAppendVarint(b []byte, v uint64) []byte {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], v)
	return append(b, buf[:n]...)
}

func pbTag(b []byte, num uint32, wt uint8) []byte {
	return pbAppendVarint(b, uint64(num)<<3|uint64(wt))
}

func pbVarintField(b []byte, num uint32, v uint64) []byte {
	b = pbTag(b, num, wireVarint)
	return pbAppendVarint(b, v)
}

func pbStringField(b []byte, num uint32, s string) []byte {
	b = pbTag(b, num, wireBytes)
	b = pbAppendVarint(b, uint64(len(s)))
	return append(b, s...)
}

func pbSubField(b []byte, num uint32, body []byte) []byte {
	b = pbTag(b, num, wireBytes)
	b = pbAppendVarint(b, uint64(len(body)))
	return append(b, body...)
}

// spbEncodeMetric builds a Metric submessage; fields set per arguments.
func spbEncodeMetric(name string, alias uint64, tsMs uint64, isNull bool, value any) []byte {
	var m []byte
	if name != "" {
		m = pbStringField(m, metricFieldName, name)
	}
	if alias != 0 {
		m = pbVarintField(m, metricFieldAlias, alias)
	}
	if tsMs != 0 {
		m = pbVarintField(m, metricFieldTs, tsMs)
	}
	if isNull {
		m = pbVarintField(m, metricFieldIsNull, 1)
	}
	switch v := value.(type) {
	case nil:
	case bool:
		m = pbVarintField(m, metricFieldBool, boolToU64(v))
	case int32:
		m = pbVarintField(m, metricFieldInt, uint64(uint32(v)))
	case uint64:
		m = pbVarintField(m, metricFieldLong, v)
	case float32:
		m = pbTag(m, metricFieldFloat, wireFixed32)
		m = binary.LittleEndian.AppendUint32(m, math.Float32bits(v))
	case float64:
		m = pbTag(m, metricFieldDouble, wireFixed64)
		m = binary.LittleEndian.AppendUint64(m, math.Float64bits(v))
	case string:
		m = pbStringField(m, metricFieldString, v)
	}
	return m
}

func boolToU64(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}

// spbEncodePayload builds a Payload: timestamp, seq and metric submessages.
func spbEncodePayload(tsMs, seq uint64, seqSet bool, metrics ...[]byte) []byte {
	var p []byte
	if tsMs != 0 {
		p = pbVarintField(p, payloadFieldTimestamp, tsMs)
	}
	if seqSet {
		p = pbVarintField(p, payloadFieldSeq, seq)
	}
	for _, m := range metrics {
		p = pbSubField(p, payloadFieldMetrics, m)
	}
	return p
}

func TestDecodeSparkplugPayload(t *testing.T) {
	raw := spbEncodePayload(1725000000123, 42, true,
		spbEncodeMetric("temp", 0, 1725000000100, false, float32(5.4)),
		spbEncodeMetric("", 7, 0, false, int32(-5)),
		spbEncodeMetric("count", 0, 0, false, uint64(1234567890123)),
		spbEncodeMetric("label", 0, 0, false, "OK"),
		spbEncodeMetric("hole", 0, 0, true, nil),
	)
	tsMs, seq, seqSet, metrics, err := decodePayload(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if tsMs != 1725000000123 || seq != 42 || !seqSet {
		t.Fatalf("header: ts=%d seq=%d seqSet=%v", tsMs, seq, seqSet)
	}
	if len(metrics) != 5 {
		t.Fatalf("metrics: got %d, want 5", len(metrics))
	}
	if metrics[0].name != "temp" || !metrics[0].hasName || metrics[0].tsMs != 1725000000100 {
		t.Errorf("temp header: %+v", metrics[0])
	}
	if metrics[0].value != float64(float32(5.4)) {
		t.Errorf("temp value = %v (%T)", metrics[0].value, metrics[0].value)
	}
	if metrics[1].value != float64(-5) {
		t.Errorf("int_value -5 -> %v, want -5 (two's complement)", metrics[1].value)
	}
	if metrics[2].value != float64(1234567890123) {
		t.Errorf("long_value -> %v", metrics[2].value)
	}
	if metrics[3].value != "OK" {
		t.Errorf("string_value -> %v", metrics[3].value)
	}
	if !metrics[4].isNull || metrics[4].value != nil {
		t.Errorf("null metric: %+v", metrics[4])
	}
}

func sparkplugTestSource(t *testing.T, tags ...TagSpec) *sparkplugSource {
	t.Helper()
	cfg := &Config{
		ID:     "gw-sparkplug-test",
		Driver: DriverSparkplug,
		Tags:   tags,
	}
	s := &sparkplugSource{cfg: cfg, tagByKey: map[string]int{}, aliases: map[string]map[uint64]string{}}
	if err := s.buildTagIndex(); err != nil {
		t.Fatalf("buildTagIndex: %v", err)
	}
	return s
}

func TestSparkplugMappingBirthDataAndAliases(t *testing.T) {
	s := sparkplugTestSource(t,
		TagSpec{TagID: "plant-a.grinding.mill_power", AssetID: "plant-a.grinding", Unit: "kW", Sparkplug: "plant-a/grinding-01/mill_power"},
		TagSpec{TagID: "plant-a.grinding.feeder.temp", AssetID: "plant-a.grinding.feeder", Unit: "C", Sparkplug: "plant-a/grinding-01/feeder-01/winding_temp"},
		TagSpec{TagID: "plant-a.grinding.unmapped", AssetID: "plant-a.grinding", Unit: "x", Sparkplug: "plant-a/grinding-01/other"},
	)

	// NBIRTH: names + aliases; mill_power value already present.
	s.handleTopic("spBv1.0/plant-a/NBIRTH/grinding-01", spbEncodePayload(1725000000000, 1, true,
		spbEncodeMetric("mill_power", 7, 1725000000050, false, float64(1245.7)),
		spbEncodeMetric("not_mapped", 8, 0, false, float64(1)),
	))
	// NDATA: alias-only delta (Sparkplug compact form).
	s.handleTopic("spBv1.0/plant-a/NDATA/grinding-01", spbEncodePayload(1725000001000, 2, true,
		spbEncodeMetric("", 7, 1725000001050, false, float64(1250.1)),
		spbEncodeMetric("", 99, 0, false, float64(9)), // unknown alias: ignored
	))
	// DBIRTH + DDATA on the feeder device.
	s.handleTopic("spBv1.0/plant-a/DBIRTH/grinding-01/feeder-01", spbEncodePayload(1725000002000, 3, true,
		spbEncodeMetric("winding_temp", 11, 0, false, float64(65.5)),
	))
	s.handleTopic("spBv1.0/plant-a/DDATA/grinding-01/feeder-01", spbEncodePayload(1725000003000, 4, true,
		spbEncodeMetric("", 11, 0, false, float64(66.0)),
	))
	// Unrelated traffic must be ignored silently.
	s.handleTopic("spBv1.0/other/NDATA/edge-01", spbEncodePayload(1725000004000, 5, true, spbEncodeMetric("mill_power", 7, 0, false, float64(0))))

	deltas, err := s.Poll(t.Context(), time.Now())
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	type row struct {
		tagID string
		value any
		seq   *int64
		obsMs int64
	}
	want := []row{
		{"plant-a.grinding.mill_power", 1245.7, int64ptr(1), 1725000000050},
		{"plant-a.grinding.mill_power", 1250.1, int64ptr(2), 1725000001050},
		{"plant-a.grinding.feeder.temp", 65.5, int64ptr(3), 1725000002000},
		{"plant-a.grinding.feeder.temp", 66.0, int64ptr(4), 1725000003000},
	}
	if len(deltas) != len(want) {
		t.Fatalf("poll returned %d deltas, want %d: %+v", len(deltas), len(want), deltas)
	}
	for i, d := range deltas {
		w := want[i]
		if d.TagID != w.tagID || d.Value != w.value {
			t.Errorf("delta[%d] = %s %v, want %s %v", i, d.TagID, d.Value, w.tagID, w.value)
		}
		if d.Quality != schema.QualityGood {
			t.Errorf("delta[%d] quality = %s", i, d.Quality)
		}
		if w.seq != nil && (d.SourceSequence == nil || *d.SourceSequence != *w.seq) {
			t.Errorf("delta[%d] seq = %v, want %d", i, d.SourceSequence, *w.seq)
		}
		if got := d.ObservedAt.UnixMilli(); got != w.obsMs {
			t.Errorf("delta[%d] observed_at = %s (%d ms), want %d ms", i, d.ObservedAt, got, w.obsMs)
		}
	}
	// Second Poll must be empty (deltas drained).
	again, err := s.Poll(t.Context(), time.Now())
	if err != nil || len(again) != 0 {
		t.Errorf("second poll: %d deltas, err %v; want empty", len(again), err)
	}
}

func int64ptr(v int64) *int64 { return &v }

func TestSparkplugSpecValidation(t *testing.T) {
	if _, err := canonicalSparkplugKey("a/b"); err == nil {
		t.Error("group/edge-only spec must fail")
	}
	if _, err := canonicalSparkplugKey("a//name"); err == nil {
		t.Error("empty segment must fail")
	}
	if _, err := canonicalSparkplugKey("a/e/dev/name"); err != nil {
		t.Errorf("device-level spec must pass: %v", err)
	}
	cfg := &Config{ID: "gw", Driver: DriverSparkplug, Tags: []TagSpec{
		{TagID: "x", AssetID: "x", Sparkplug: "a/e/name"},
		{TagID: "y", AssetID: "x", Sparkplug: "a/e/name"},
	}}
	s := &sparkplugSource{cfg: cfg, tagByKey: map[string]int{}, aliases: map[string]map[uint64]string{}}
	if err := s.buildTagIndex(); err == nil {
		t.Error("duplicate specs must fail")
	}
}

func TestSparkplugSourceRequiresBroker(t *testing.T) {
	if _, err := NewSource(&Config{Driver: DriverSparkplug, Tags: []TagSpec{
		{TagID: "x", AssetID: "x", Sparkplug: "a/e/n"},
	}}); err == nil {
		t.Error("missing MQTT_BROKER must fail at construction")
	}
}
