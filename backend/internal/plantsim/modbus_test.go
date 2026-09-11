package plantsim_test

import (
	"testing"
	"time"

	modbus "github.com/goburrow/modbus"

	"cap/internal/plantsim"
)

// newStand starts a full stand (model + Modbus server) on 127.0.0.1:0.
func newStand(t *testing.T, tick time.Duration) (*plantsim.Model, *plantsim.ModbusServer) {
	t.Helper()
	m := plantsim.New(42, time.Now())
	srv, err := plantsim.NewModbusServer("127.0.0.1:0", m, 1)
	if err != nil {
		t.Fatalf("modbus server: %v", err)
	}
	t.Cleanup(srv.Close)
	return m, srv
}

func client(t *testing.T, addr string) modbus.Client {
	t.Helper()
	h := modbus.NewTCPClientHandler(addr)
	h.SlaveId = 1
	h.Timeout = 500 * time.Millisecond
	if err := h.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return modbus.NewClient(h)
}

// TestReadInputAndHoldingRegisters: FC4 returns the model image, FC3 the
// actuator image, both decoded with the documented scales.
func TestReadInputAndHoldingRegisters(t *testing.T) {
	m, srv := newStand(t, 100*time.Millisecond)
	c := client(t, srv.Addr())

	// FC4 block read of the whole sensor area.
	raw, err := c.ReadInputRegisters(0, 28)
	if err != nil {
		t.Fatalf("FC4: %v", err)
	}
	if len(raw) != 56 {
		t.Fatalf("FC4 payload = %d bytes, want 56", len(raw))
	}
	fi101 := uint16(raw[0])<<8 | uint16(raw[1])
	if v := float64(fi101) * 0.1; v < 90 || v > 110 {
		t.Fatalf("fi101 = %.1f, want ~100", v)
	}

	// FC3 block read of the actuator area.
	hraw, err := c.ReadHoldingRegisters(0, 7)
	if err != nil {
		t.Fatalf("FC3: %v", err)
	}
	hc101 := float64(uint16(hraw[0])<<8|uint16(hraw[1])) * 0.1
	if hc101 != 100.0 {
		t.Fatalf("hc101 = %.1f, want 100", hc101)
	}
	_ = m
}

// TestFC6WriteDrivesModel: an FC6 write to the feeder setpoint reaches the
// model — the feed rate follows the new setpoint after the belt lag.
func TestFC6WriteDrivesModel(t *testing.T) {
	m, srv := newStand(t, 10*time.Millisecond)
	c := client(t, srv.Addr())

	// Write hc101 = 150 t/h (raw 1500).
	if _, err := c.WriteSingleRegister(plantsim.RegHC101, 1500); err != nil {
		t.Fatalf("FC6: %v", err)
	}
	hraw, err := c.ReadHoldingRegisters(plantsim.RegHC101, 1)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got := uint16(hraw[0])<<8 | uint16(hraw[1]); got != 1500 {
		t.Fatalf("read back = %d, want 1500", got)
	}

	// Advance the model ~30 s: the belt (tau 8 s) approaches 150 t/h.
	for i := 0; i < 3000; i++ {
		m.Tick(10 * time.Millisecond)
	}
	raw, err := c.ReadInputRegisters(plantsim.RegFI101, 1)
	if err != nil {
		t.Fatalf("FC4: %v", err)
	}
	fi101 := float64(uint16(raw[0])<<8|uint16(raw[1])) * 0.1
	if fi101 < 140 || fi101 > 160 {
		t.Fatalf("fi101 = %.1f, want ~150 after setpoint change", fi101)
	}
}

// TestFC6RangeClamp: out-of-range writes are rejected with exception 3.
func TestFC6RangeClamp(t *testing.T) {
	_, srv := newStand(t, 100*time.Millisecond)
	c := client(t, srv.Addr())

	if _, err := c.WriteSingleRegister(plantsim.RegHI501, 5); err == nil {
		t.Fatal("out-of-range FC6 accepted, want exception")
	}
	if _, err := c.WriteSingleRegister(50, 100); err == nil {
		t.Fatal("out-of-range address accepted, want exception")
	}
}

// TestUnsupportedFunctionException: FC1 (read coils) answers exception 0x01 —
// the stand has no coil area.
func TestUnsupportedFunctionException(t *testing.T) {
	_, srv := newStand(t, 100*time.Millisecond)
	c := client(t, srv.Addr())
	if _, err := c.ReadCoils(0, 8); err == nil {
		t.Fatal("FC1 accepted, want illegal-function exception")
	}
}

// TestReadOutOfRangeAddress: FC4 beyond the register map answers exception 2.
func TestReadOutOfRangeAddress(t *testing.T) {
	_, srv := newStand(t, 100*time.Millisecond)
	c := client(t, srv.Addr())
	if _, err := c.ReadInputRegisters(100, 2); err == nil {
		t.Fatal("out-of-range FC4 accepted, want exception 2")
	}
}

// TestConcurrentSessions: several clients may poll simultaneously.
func TestConcurrentSessions(t *testing.T) {
	_, srv := newStand(t, 20*time.Millisecond)
	done := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() {
			c := client(t, srv.Addr())
			for j := 0; j < 20; j++ {
				if _, err := c.ReadInputRegisters(0, 4); err != nil {
					done <- err
					return
				}
				time.Sleep(2 * time.Millisecond)
			}
			done <- nil
		}()
	}
	for i := 0; i < 4; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent session: %v", err)
		}
	}
}

// TestCommLossScenario: scenario 4 makes the server stop answering; the
// client times out. Recovery returns it to service.
func TestCommLossScenario(t *testing.T) {
	m, srv := newStand(t, 10*time.Millisecond)
	c := client(t, srv.Addr())

	if _, err := c.ReadInputRegisters(0, 2); err != nil {
		t.Fatalf("pre-outage read: %v", err)
	}
	m.Scenario(plantsim.ScenarioCommLoss, 1)
	// The model tick drives the simulated clock used by scenario timing.
	m.Tick(10 * time.Millisecond)
	if _, err := c.ReadInputRegisters(0, 2); err == nil {
		t.Fatal("read succeeded during communication loss, want timeout")
	}
	m.Scenario(plantsim.ScenarioCommLoss, 0)
	if _, err := c.ReadInputRegisters(0, 2); err != nil {
		t.Fatalf("post-recovery read: %v", err)
	}
}
