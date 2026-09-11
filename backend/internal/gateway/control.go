package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	modbus "github.com/goburrow/modbus"
)

// ControlWriteBridge is the only component in CAP allowed to write to field
// equipment (TZ §9). It polls the server's control/output feed and translates
// it into Modbus FC6 holding-register writes:
//   - status "auto" (a supervisory loop in auto): the mapped loop output is
//     written on every poll;
//   - status "hold" (a one-shot manual actuator write): written exactly once,
//     when its seq is newer than the last delivered seq;
//   - any poll or connection failure leaves the actuator at its last value —
//     losing the platform never resets field equipment (IEC 61511 instinct).
type ControlWriteBridge struct {
	cfg   *Config
	http  *http.Client
	byTag map[string]modbusReg // mv tag -> register spec
	// writtenSeq remembers the last delivered seq per tag (manual writes).
	writtenSeq map[string]int64
	handler    *modbus.TCPClientHandler
	client     modbus.Client
}

// NewControlWriteBridge indexes the writable holding-register specs from the
// TAGS configuration. Only hr (FC3/FC6) entries can be driven.
func NewControlWriteBridge(cfg *Config) (*ControlWriteBridge, error) {
	byTag := map[string]modbusReg{}
	for _, t := range cfg.Tags {
		if t.Modbus == "" {
			continue
		}
		spec, err := parseModbusSpec(t.Modbus)
		if err != nil {
			return nil, fmt.Errorf("tag %s: %w", t.TagID, err)
		}
		if spec.fc == 3 {
			byTag[t.TagID] = spec
		}
	}
	if len(byTag) == 0 {
		return nil, fmt.Errorf("no writable holding registers (hr) in TAGS")
	}
	return &ControlWriteBridge{
		cfg:        cfg,
		http:       &http.Client{Timeout: 3 * time.Second},
		byTag:      byTag,
		writtenSeq: map[string]int64{},
	}, nil
}

// Run polls the server and writes actuator registers until ctx ends.
func (b *ControlWriteBridge) Run(ctx context.Context) {
	poll := b.cfg.ControlPoll
	if poll <= 0 {
		poll = time.Second
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	log.Printf("[gateway/control] WRITE BRIDGE ACTIVE: %s -> %d writable registers (FC6, poll %s) — the platform can now write to field equipment",
		b.cfg.ModbusAddr, len(b.byTag), poll)
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

type outputItem struct {
	TagID  string  `json:"tag_id"`
	Value  float64 `json:"value"`
	Seq    int64   `json:"seq"`
	Status string  `json:"status"`
}

func (b *ControlWriteBridge) tick(ctx context.Context) {
	url := strings.TrimRight(b.cfg.ServerURL, "/") + "/api/v1/control/output"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return // server unreachable: hold the last actuator values
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return
	}
	var items []outputItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return
	}
	for _, item := range items {
		spec, ok := b.byTag[item.TagID]
		if !ok {
			continue
		}
		switch item.Status {
		case "auto":
			b.write(item.TagID, spec, item.Value)
		case "hold":
			if item.Seq > b.writtenSeq[item.TagID] {
				if b.write(item.TagID, spec, item.Value) {
					b.writtenSeq[item.TagID] = item.Seq
				}
			}
		default:
			// manual loop output: the operator holds the register, the server
			// mirrors the frozen value — nothing to write.
		}
	}
}

// write performs the FC6 write, converting engineering units to raw register
// counts via the tag scale.
func (b *ControlWriteBridge) write(tagID string, spec modbusReg, value float64) bool {
	raw := int(math.Min(65535, math.Max(0, math.Round(value/spec.scale))))
	if b.handler == nil {
		if err := b.connect(); err != nil {
			log.Printf("[gateway/control] connect: %v", err)
			return false
		}
	}
	if _, err := b.client.WriteSingleRegister(spec.addr, uint16(raw)); err != nil {
		log.Printf("[gateway/control] write %s HR%d=%d: %v", tagID, spec.addr, raw, err)
		b.reset()
		return false
	}
	log.Printf("[gateway/control] %s -> HR%d=%d (%.3g)", tagID, spec.addr, raw, value)
	return true
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
