package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	modbus "github.com/goburrow/modbus"
)

// ControlWriteBridge is the only component in CAP allowed to write to field
// equipment (ADR-003). It polls the server's control/output endpoint and, when
// the loop reports status "auto", writes the computed output into ONE Modbus
// holding register (FC6). Any poll or connection failure leaves the actuator
// at its last value — the server-side watchdog zeroes the output on stale PV.
type ControlWriteBridge struct {
	cfg     *Config
	handler *modbus.TCPClientHandler
	client  modbus.Client
	spec    modbusReg
	http    *http.Client
}

// NewControlWriteBridge validates the write register spec. Only holding
// registers (fc=3) are writable; anything else is a configuration error.
func NewControlWriteBridge(cfg *Config) (*ControlWriteBridge, error) {
	spec, err := parseModbusSpec(cfg.ControlWrite)
	if err != nil {
		return nil, fmt.Errorf("CONTROL_WRITE: %w", err)
	}
	if spec.fc != 3 {
		return nil, fmt.Errorf("CONTROL_WRITE: only holding registers (hr) are writable, got fc=%d", spec.fc)
	}
	return &ControlWriteBridge{cfg: cfg, spec: spec, http: &http.Client{Timeout: 3 * time.Second}}, nil
}

// Run polls the server and writes the actuator register until ctx ends.
func (b *ControlWriteBridge) Run(ctx context.Context) {
	poll := b.cfg.ControlPoll
	if poll <= 0 {
		poll = time.Second
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	log.Printf("[gateway/control] WRITE BRIDGE ACTIVE: out=%s -> %s %s (FC6, poll %s) — the platform can now write to field equipment",
		b.cfg.ControlOutTag, b.cfg.ModbusAddr, b.specDesc(), poll)
	for {
		select {
		case <-ctx.Done():
			b.reset()
			return
		case <-ticker.C:
			b.tick(ctx)
		}
	}
}

func (b *ControlWriteBridge) tick(ctx context.Context) {
	url := fmt.Sprintf("%s/api/v1/control/output?tag_id=%s", strings.TrimRight(b.cfg.ServerURL, "/"), url.QueryEscape(b.cfg.ControlOutTag))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return // server unreachable: hold the last actuator value
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var payload struct {
		Output float64 `json:"output"`
		Status string  `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil || payload.Status != "auto" {
		return
	}
	raw := int(math.Min(65535, math.Max(0, math.Round(payload.Output/b.spec.scale))))
	if b.handler == nil {
		if err := b.connect(); err != nil {
			log.Printf("[gateway/control] connect: %v", err)
			return
		}
	}
	if _, err := b.client.WriteSingleRegister(b.spec.addr, uint16(raw)); err != nil {
		log.Printf("[gateway/control] write HR%d=%d: %v", b.spec.addr, raw, err)
		b.reset()
		return
	}
	log.Printf("[gateway/control] output %.1f%% -> HR%d=%d", payload.Output, b.spec.addr, raw)
}

func (b *ControlWriteBridge) connect() error {
	h := modbus.NewTCPClientHandler(b.cfg.ModbusAddr)
	h.SlaveId = b.cfg.ModbusUnitID
	h.Timeout = b.cfg.ModbusTimeout
	if err := h.Connect(); err != nil {
		return fmt.Errorf("connect %s: %w", b.cfg.ModbusAddr, err)
	}
	b.handler = h
	b.client = modbus.NewClient(h)
	return nil
}

func (b *ControlWriteBridge) reset() {
	if b.handler != nil {
		b.handler.Close()
		b.handler = nil
		b.client = nil
	}
}

func (b *ControlWriteBridge) specDesc() string {
	return fmt.Sprintf("addr=%d type-kind=%d scale=%g", b.spec.addr, b.spec.kind, b.spec.scale)
}
