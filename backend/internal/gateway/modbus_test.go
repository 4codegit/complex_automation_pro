package gateway

import (
	"encoding/binary"
	"io"
	"math"
	"net"
	"testing"
	"time"

	"cap/internal/schema"
)

// fakeModbusServer is a minimal Modbus TCP device for tests: it answers FC1/2
// (coils/discrete inputs) and FC3/4 (registers) from fixed backing maps.
type fakeModbusServer struct {
	ln        net.Listener
	registers map[byte]map[uint16]uint16 // fc 3|4 -> address -> value
	coils     map[byte]map[uint16]bool   // fc 1|2 -> address -> state
}

func newFakeModbusServer(t *testing.T, registers map[byte]map[uint16]uint16, coils map[byte]map[uint16]bool) *fakeModbusServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeModbusServer{ln: ln, registers: registers, coils: coils}
	go s.serve()
	t.Cleanup(func() { ln.Close() })
	return s
}

func (s *fakeModbusServer) addr() string { return s.ln.Addr().String() }

func (s *fakeModbusServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeModbusServer) handle(conn net.Conn) {
	defer conn.Close()
	var header [7]byte
	for {
		if _, err := io.ReadFull(conn, header[:]); err != nil {
			return
		}
		length := binary.BigEndian.Uint16(header[4:6])
		if length < 2 {
			return
		}
		pdu := make([]byte, length-1)
		if _, err := io.ReadFull(conn, pdu); err != nil {
			return
		}
		resp := s.respond(pdu)
		out := make([]byte, 0, len(resp)+7)
		out = binary.BigEndian.AppendUint16(out, binary.BigEndian.Uint16(header[0:2])) // txn id
		out = binary.BigEndian.AppendUint16(out, 0)                                    // protocol id
		out = binary.BigEndian.AppendUint16(out, uint16(len(resp)+1))
		out = append(out, header[6]) // unit id
		out = append(out, resp...)
		if _, err := conn.Write(out); err != nil {
			return
		}
	}
}

func (s *fakeModbusServer) respond(pdu []byte) []byte {
	fc := pdu[0]
	addr := binary.BigEndian.Uint16(pdu[1:3])
	qty := binary.BigEndian.Uint16(pdu[3:5])
	switch fc {
	case 3, 4:
		data := make([]byte, 0, int(qty)*2)
		for i := uint16(0); i < qty; i++ {
			data = binary.BigEndian.AppendUint16(data, s.registers[fc][addr+i])
		}
		return append([]byte{fc, byte(len(data))}, data...)
	case 1, 2:
		n := (int(qty) + 7) / 8
		data := make([]byte, n)
		for i := uint16(0); i < qty; i++ {
			if s.coils[fc][addr+i] {
				data[i/8] |= 1 << (i % 8)
			}
		}
		return append([]byte{fc, byte(n)}, data...)
	default:
		return []byte{fc | 0x80, 0x01} // illegal function
	}
}

func modbusTestConfig(addr string, tags ...TagSpec) *Config {
	return &Config{
		ID:            "gw-modbus-test",
		ModbusAddr:    addr,
		ModbusUnitID:  1,
		ModbusTimeout: 2 * time.Second,
		Driver:        DriverModbus,
		Tags:          tags,
	}
}

func TestParseModbusSpec(t *testing.T) {
	cases := []struct {
		spec  string
		want  modbusReg
		fails bool
	}{
		{spec: "hr:100:f32", want: modbusReg{fc: 3, addr: 100, qty: 2, kind: mbF32, scale: 1}},
		{spec: "ir:30:u16:0.01", want: modbusReg{fc: 4, addr: 30, qty: 1, kind: mbU16, scale: 0.01}},
		{spec: "c:5:bool", want: modbusReg{fc: 1, addr: 5, qty: 1, kind: mbBool, scale: 1}},
		{spec: "di:20:bool", want: modbusReg{fc: 2, addr: 20, qty: 1, kind: mbBool, scale: 1}},
		{spec: "hr:0:i16:-1", want: modbusReg{fc: 3, addr: 0, qty: 1, kind: mbI16, scale: -1}},
		{spec: "hr:200:f32sw", want: modbusReg{fc: 3, addr: 200, qty: 2, kind: mbF32, swap: true, scale: 1}},
		{spec: "hr:1:u32", want: modbusReg{fc: 3, addr: 1, qty: 2, kind: mbU32, scale: 1}},
		{spec: "xx:1:u16", fails: true},
		{spec: "hr:99999:u16", fails: true},
		{spec: "hr:1:f16", fails: true},
		{spec: "hr:1:bool:2", fails: true},
		{spec: "hr:1", fails: true},
	}
	for _, tc := range cases {
		got, err := parseModbusSpec(tc.spec)
		if tc.fails {
			if err == nil {
				t.Errorf("parseModbusSpec(%q) = %+v, want error", tc.spec, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseModbusSpec(%q): %v", tc.spec, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseModbusSpec(%q) = %+v, want %+v", tc.spec, got, tc.want)
		}
	}
}

func TestModbusSourcePollDecodesValues(t *testing.T) {
	// hr:100:f32 = 123.5 (0x42F70000), hr:200:f32sw = 123.5 in CDAB order,
	// ir:30:u16*scale 0.01 = 8.5, di:20:bool = true.
	srv := newFakeModbusServer(t,
		map[byte]map[uint16]uint16{
			3: {100: 0x42F7, 101: 0x0000, 200: 0x0000, 201: 0x42F7},
			4: {30: 850},
		},
		map[byte]map[uint16]bool{2: {20: true}},
	)
	cfg := modbusTestConfig(srv.addr(),
		TagSpec{TagID: "plant-a.grinding.mill_power", AssetID: "plant-a.grinding", Unit: "kW", Modbus: "hr:100:f32"},
		TagSpec{TagID: "plant-a.grinding.mill_power_sw", AssetID: "plant-a.grinding", Unit: "kW", Modbus: "hr:200:f32sw"},
		TagSpec{TagID: "plant-a.flotation.ph_level", AssetID: "plant-a.flotation", Unit: "pH", Modbus: "ir:30:u16:0.01"},
		TagSpec{TagID: "plant-a.receiving.metal_detected", AssetID: "plant-a.receiving", Unit: "state", Modbus: "di:20:bool"},
	)
	src, err := NewSource(cfg)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	defer src.Close()

	msgs, err := src.Poll(t.Context(), time.Now())
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(msgs) != len(cfg.Tags) {
		t.Fatalf("poll returned %d messages, want %d", len(msgs), len(cfg.Tags))
	}
	want := map[string]any{
		"plant-a.grinding.mill_power":      123.5,
		"plant-a.grinding.mill_power_sw":   123.5,
		"plant-a.flotation.ph_level":       8.5,
		"plant-a.receiving.metal_detected": true,
	}
	for _, m := range msgs {
		w, ok := want[m.TagID]
		if !ok {
			t.Errorf("unexpected tag %q", m.TagID)
			continue
		}
		if m.Quality != schema.QualityGood {
			t.Errorf("%s quality = %q, want good", m.TagID, m.Quality)
		}
		if m.Value != w {
			t.Errorf("%s value = %v (%T), want %v", m.TagID, m.Value, m.Value, w)
		}
		if m.GatewayID != cfg.ID || m.AssetID == "" || m.Unit == "" {
			t.Errorf("%s malformed telemetry: %+v", m.TagID, m)
		}
	}
}

func TestModbusSourceUnreachableEmitsOfflineGap(t *testing.T) {
	cfg := modbusTestConfig("127.0.0.1:1", // nothing listens there
		TagSpec{TagID: "plant-a.grinding.mill_power", AssetID: "plant-a.grinding", Unit: "kW", Modbus: "hr:100:f32"},
	)
	src, err := NewSource(cfg)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	defer src.Close()

	msgs, err := src.Poll(t.Context(), time.Now())
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("poll returned %d messages, want 1 offline gap", len(msgs))
	}
	if msgs[0].Quality != schema.QualityOffline || msgs[0].Value != nil {
		t.Errorf("quality = %q value = %v, want offline/nil", msgs[0].Quality, msgs[0].Value)
	}
}

func TestModbusSourceRecoversAfterOutage(t *testing.T) {
	srv := newFakeModbusServer(t,
		map[byte]map[uint16]uint16{3: {100: 0x42F7, 101: 0x0000}},
		nil,
	)
	cfg := modbusTestConfig(srv.addr(),
		TagSpec{TagID: "plant-a.grinding.mill_power", AssetID: "plant-a.grinding", Unit: "kW", Modbus: "hr:100:f32"},
	)
	src, err := NewSource(cfg)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	defer src.Close()

	if _, err := src.Poll(t.Context(), time.Now()); err != nil {
		t.Fatalf("first poll: %v", err)
	}
	// Simulate a device outage: dropping the listener breaks the established
	// connection; the next poll must reset and reconnect transparently.
	srv.ln.Close()
	time.Sleep(50 * time.Millisecond)
	srv.ln, err = net.Listen("tcp", srv.addr())
	if err != nil {
		t.Fatalf("re-listen: %v", err)
	}
	go srv.serve()

	msgs, err := src.Poll(t.Context(), time.Now())
	if err != nil {
		t.Fatalf("poll after recovery: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Quality != schema.QualityGood || msgs[0].Value != 123.5 {
		t.Errorf("after recovery: %+v, want good/123.5", msgs)
	}
	_ = math.Float32bits(123.5) // keep math import if decode constants change
}
