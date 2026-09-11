package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"cap/internal/schema"
)

// Pulse sends the heartbeat (TZ S3: "пульс gateway_events каждые N секунд").
// It reports connection state, local buffer depth and delivery latency so the
// platform can render per-gateway health.
type Pulse struct {
	client    *http.Client
	server    string
	token     string // device credential (X-Gateway-Token)
	interval  time.Duration
	gatewayID string
	version   string

	mu        sync.Mutex
	buffer    func() int64 // latest buffer depth callback
	latency   func() time.Duration
	online    bool
	reconnect func() // called once on offline -> online transition
}

// NewPulse builds the heartbeat sender. onReconnect lets the pulse wake the
// sender immediately when connectivity returns (fast backfill).
func NewPulse(server, token, gatewayID, version string, interval time.Duration, bufferLen func() int64, latency func() time.Duration, onReconnect func()) *Pulse {
	return &Pulse{
		client:    &http.Client{Timeout: 5 * time.Second},
		server:    server,
		token:     token,
		interval:  interval,
		gatewayID: gatewayID,
		version:   version,
		buffer:    bufferLen,
		latency:   latency,
		reconnect: onReconnect,
	}
}

// SetOnline updates the reported connection state.
func (p *Pulse) SetOnline(on bool) {
	p.mu.Lock()
	p.online = on
	p.mu.Unlock()
}

// Online reports the last known connection state.
func (p *Pulse) Online() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.online
}

// Run emits pulses until ctx is done. Failures are logged and retried; the
// heartbeat must never crash the gateway.
func (p *Pulse) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	p.beat(ctx) // immediate first beat
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.beat(ctx)
		}
	}
}

func (p *Pulse) beat(ctx context.Context) {
	status := "online"
	event := schema.GatewayEventPulse
	if !p.Online() {
		status = "offline"
		event = schema.GatewayEventOffline
	}
	ev := schema.GatewayEvent{
		GatewayID:  p.gatewayID,
		Event:      event,
		Status:     status,
		BufferSize: p.buffer(),
		LatencyMs:  p.latency().Milliseconds(),
		Version:    p.version,
		Timestamp:  time.Now().UTC(),
	}
	raw, _ := json.Marshal(ev)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.server+"/api/v1/ingest/gateway_events", bytes.NewReader(raw))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if p.token != "" {
		req.Header.Set("X-Gateway-Token", p.token)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		if p.Online() {
			log.Printf("[gateway] pulse failed: server unreachable")
		}
		p.SetOnline(false)
		return
	}
	defer resp.Body.Close()

	wasOffline := !p.Online()
	p.SetOnline(resp.StatusCode == http.StatusAccepted)
	if wasOffline && p.Online() && p.reconnect != nil {
		log.Printf("[gateway] server reachable again — waking sender")
		p.reconnect()
	}
}
