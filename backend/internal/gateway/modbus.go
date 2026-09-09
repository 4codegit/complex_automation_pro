package gateway

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	modbus "github.com/goburrow/modbus"

	"cap/internal/schema"
)

// modbusSource is a read-only Modbus TCP poll driver (de-facto protocol of
// PLCs, energy meters and simple field instruments). On every Poll call it
// issues one read request per configured register spec and decodes the raw
// bytes into canonical values.
//
// Read-only invariant (see ARCHITECTURE_DECISIONS.md ADR-001): this driver
// performs function codes 1-4 (read) only. It exposes no write path (5, 6,
// 15, 16) to the OT asset.
//
// Register spec syntax (TAGS entry suffix, after "|"):
//
//	reg=<fc>:<address>:<type>[:<scale>]
//	  fc    hr (holding register, FC3) | ir (input register, FC4)
//	        c  (coil, FC1)             | di (discrete input, FC2)
//	  type  bool | u16 | i16 | u32 | i32 | f32   ("sw" suffix = 32-bit
//	        word swap, CDAB order used by some PLCs)
//	  scale optional multiplier applied to numeric values (e.g. 0.01)
//
// Examples:
//
//	plant-a.grinding.mill_power:kW|reg=hr:100:f32
//	plant-a.flotation.ph_level:pH|reg=ir:30:u16:0.01
//	plant-a.receiving.metal_detected:state|reg=di:20:bool
type modbusSource struct {
	cfg     *Config
	handler *modbus.TCPClientHandler
	client  modbus.Client
	regs    []modbusReg

	mu     sync.Mutex
	closed bool
}

// modbusReg is one decoded register spec bound to its tag index.
type modbusReg struct {
	tagIndex int
	fc       byte // 1..4
	addr     uint16
	qty      uint16 // registers (or coil count, always 1 for bool)
	kind     modbusKind
	swap     bool    // 32-bit word swap (CDAB)
	scale    float64 // multiplier for numeric kinds
}

type modbusKind uint8

const (
	mbBool modbusKind = iota
	mbU16
	mbI16
	mbU32
	mbI32
	mbF32
)

var _ Source = (*modbusSource)(nil)

// newModbusSource validates the config and constructs the source. The TCP
// session is established lazily on the first Poll so that a momentarily
// unreachable device does not abort gateway startup: the local buffer absorbs
// the outage, and the first poll emits QualityOffline for each tag.
func newModbusSource(cfg *Config) (Source, error) {
	if cfg.ModbusAddr == "" {
		return nil, errors.New("MODBUS_ADDR is required when SOURCE_DRIVER=modbus")
	}
	if len(cfg.Tags) == 0 {
		return nil, errors.New("TAGS must contain at least one tag spec for the modbus driver")
	}
	s := &modbusSource{cfg: cfg, regs: make([]modbusReg, 0, len(cfg.Tags))}
	for i, t := range cfg.Tags {
		if t.Modbus == "" {
			return nil, fmt.Errorf("tag %q is missing a register spec (expected ...:unit|reg=hr:100:f32)", t.TagID)
		}
		r, err := parseModbusSpec(t.Modbus)
		if err != nil {
			return nil, fmt.Errorf("tag %q: %w", t.TagID, err)
		}
		r.tagIndex = i
		s.regs = append(s.regs, r)
	}
	return s, nil
}

// Name implements Source.
func (s *modbusSource) Name() string { return DriverModbus }

// Poll reads every configured register once and returns canonical messages.
// A transport error aborts the tick: the connection is reset for the next
// poll and the tags not yet read simply do not emit (a gap in the historian,
// not a fabricated value). A fully unreachable device emits QualityOffline
// per tag so consumers see the outage instead of silence.
func (s *modbusSource) Poll(ctx context.Context, now time.Time) ([]schema.Telemetry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("modbus source closed")
	}
	if s.client == nil {
		if err := s.connect(); err != nil {
			return s.offlineAll(now, err), nil
		}
	}
	out := make([]schema.Telemetry, 0, len(s.regs))
	for _, r := range s.regs {
		raw, err := s.read(r)
		if err != nil {
			s.resetConn()
			log.Printf("[gateway/modbus] %s read fc%d@%d: %v (reconnect next poll)",
				s.cfg.Tags[r.tagIndex].TagID, r.fc, r.addr, err)
			break
		}
		t := s.basicTelemetry(r.tagIndex, now)
		t.Value = decodeModbus(r, raw)
		out = append(out, t)
	}
	return out, nil
}

// connect opens the TCP session. The handler reconnects lazily, but an
// explicit reset (resetConn) after any error keeps state predictable.
func (s *modbusSource) connect() error {
	h := modbus.NewTCPClientHandler(s.cfg.ModbusAddr)
	h.SlaveId = s.cfg.ModbusUnitID
	h.Timeout = s.cfg.ModbusTimeout
	if err := h.Connect(); err != nil {
		return fmt.Errorf("connect %s: %w", s.cfg.ModbusAddr, err)
	}
	s.handler = h
	s.client = modbus.NewClient(h)
	return nil
}

func (s *modbusSource) resetConn() {
	if s.handler != nil {
		s.handler.Close()
		s.handler = nil
		s.client = nil
	}
}

// read performs one read operation for the register spec.
func (s *modbusSource) read(r modbusReg) ([]byte, error) {
	switch r.fc {
	case 1:
		return s.client.ReadCoils(r.addr, r.qty)
	case 2:
		return s.client.ReadDiscreteInputs(r.addr, r.qty)
	case 3:
		return s.client.ReadHoldingRegisters(r.addr, r.qty)
	case 4:
		return s.client.ReadInputRegisters(r.addr, r.qty)
	default:
		return nil, fmt.Errorf("unsupported function code %d", r.fc)
	}
}

// decodeModbus converts raw response bytes into the canonical value.
func decodeModbus(r modbusReg, raw []byte) any {
	switch r.kind {
	case mbBool:
		return len(raw) > 0 && raw[0]&0x01 != 0
	case mbU16:
		return r.scale * float64(binary.BigEndian.Uint16(raw[:2]))
	case mbI16:
		return r.scale * float64(int16(binary.BigEndian.Uint16(raw[:2])))
	default: // 32-bit kinds span two registers
		if len(raw) < 4 {
			return nil
		}
		if r.swap { // CDAB: swap the two 16-bit words first
			raw = []byte{raw[2], raw[3], raw[0], raw[1]}
		}
		switch r.kind {
		case mbU32:
			return r.scale * float64(binary.BigEndian.Uint32(raw))
		case mbI32:
			return r.scale * float64(int32(binary.BigEndian.Uint32(raw)))
		case mbF32:
			return r.scale * float64(math.Float32frombits(binary.BigEndian.Uint32(raw)))
		}
		return nil
	}
}

// basicTelemetry fills the invariant fields shared by good and offline paths.
func (s *modbusSource) basicTelemetry(i int, now time.Time) schema.Telemetry {
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
		SourceSequence: nil, // modbus has no source sequence.
	}
}

// offlineAll returns one QualityOffline telemetry per tag, used when the
// device is unreachable so the historian records a single gap per outage tick.
func (s *modbusSource) offlineAll(now time.Time, reason error) []schema.Telemetry {
	log.Printf("[gateway/modbus] offline: %v", reason)
	out := make([]schema.Telemetry, 0, len(s.cfg.Tags))
	for i := range s.cfg.Tags {
		t := s.basicTelemetry(i, now)
		t.Quality = schema.QualityOffline
		t.Value = nil
		out = append(out, t)
	}
	return out
}

// Close implements Source; idempotent.
func (s *modbusSource) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetConn()
	s.closed = true
	return nil
}

// parseModbusSpec parses "fc:addr:type[:scale]".
func parseModbusSpec(spec string) (modbusReg, error) {
	parts := strings.Split(strings.TrimSpace(spec), ":")
	if len(parts) < 3 || len(parts) > 4 {
		return modbusReg{}, fmt.Errorf("register spec %q must be fc:addr:type[:scale]", spec)
	}
	var r modbusReg
	switch strings.ToLower(strings.TrimSpace(parts[0])) {
	case "hr":
		r.fc = 3
	case "ir":
		r.fc = 4
	case "c", "coil":
		r.fc = 1
	case "di":
		r.fc = 2
	default:
		return modbusReg{}, fmt.Errorf("unknown register area %q (want hr, ir, c or di)", parts[0])
	}
	addr, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 16)
	if err != nil {
		return modbusReg{}, fmt.Errorf("invalid address %q", parts[1])
	}
	r.addr = uint16(addr)

	typeStr := strings.ToLower(strings.TrimSpace(parts[2]))
	if strings.HasSuffix(typeStr, "sw") {
		r.swap = true
		typeStr = strings.TrimSuffix(typeStr, "sw")
	}
	switch typeStr {
	case "bool":
		r.kind, r.qty = mbBool, 1
	case "u16":
		r.kind, r.qty = mbU16, 1
	case "i16":
		r.kind, r.qty = mbI16, 1
	case "u32":
		r.kind, r.qty = mbU32, 2
	case "i32":
		r.kind, r.qty = mbI32, 2
	case "f32":
		r.kind, r.qty = mbF32, 2
	default:
		return modbusReg{}, fmt.Errorf("unknown type %q (want bool, u16, i16, u32, i32 or f32)", parts[2])
	}
	r.scale = 1
	if len(parts) == 4 {
		if r.kind == mbBool {
			return modbusReg{}, fmt.Errorf("scale is not applicable to bool")
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
		if err != nil || v == 0 {
			return modbusReg{}, fmt.Errorf("invalid scale %q", parts[3])
		}
		r.scale = v
	}
	return r, nil
}
