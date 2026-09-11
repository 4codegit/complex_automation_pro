package gateway

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
)

// Runner wires the source, buffer, sender and pulse together.
type Runner struct {
	cfg    *Config
	buf    *Buffer
	source Source
	sender *Sender
	pulse  *Pulse

	version string
	// rejectedLog is the resync register (TZ S3): one line per rejected message.
	rejectedLog *os.File
}

// NewRunner constructs the edge collector from config.
func NewRunner(cfg *Config, version string) (*Runner, error) {
	buf, err := OpenBuffer(cfg.BufferDB)
	if err != nil {
		return nil, err
	}
	source, err := NewSource(cfg)
	if err != nil {
		return nil, err
	}
	sender := NewSender(cfg.ServerURL, cfg.GatewayToken, cfg.MaxBatch, cfg.PushInterval)
	pulse := NewPulse(cfg.ServerURL, cfg.GatewayToken, cfg.ID, version, cfg.PulseInterval,
		func() int64 {
			n, _ := buf.Len(context.Background())
			return n
		},
		func() time.Duration { return sender.Latency },
		sender.Wake,
	)
	r := &Runner{cfg: cfg, buf: buf, source: source, sender: sender, pulse: pulse, version: version}
	sender.Rejected = r.logRejected
	return r, nil
}

// Run starts all subsystems and blocks until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) error {
	defer r.buf.Close()
	defer r.source.Close()
	if r.rejectedLog != nil {
		defer r.rejectedLog.Close()
	}

	log.Printf("[gateway] %s starting: driver=%s server=%s tags=%d buffer=%s",
		r.cfg.ID, r.source.Name(), r.cfg.ServerURL, len(r.cfg.Tags), r.cfg.BufferDB)

	startPoll(ctx, r.cfg, r.buf, r.source)
	go r.sender.Run(ctx, r.buf)
	go r.pulse.Run(ctx)

	// Supervisory control write bridge (ADR-003): opt-in via CONTROL_ENABLED.
	// When enabled, this gateway is no longer strictly read-only — it writes
	// exactly one holding register driven by the server's loop output.
	if r.cfg.ControlEnabled {
		bridge, err := NewControlWriteBridge(r.cfg)
		if err != nil {
			return fmt.Errorf("control bridge: %w", err)
		}
		go bridge.Run(ctx)
	}

	<-ctx.Done()
	log.Printf("[gateway] shutting down")
	return nil
}

// startPoll reads instruments on the configured cadence and enqueues messages.
func startPoll(ctx context.Context, cfg *Config, buf *Buffer, source Source) {
	go func() {
		ticker := time.NewTicker(cfg.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				messages, err := source.Poll(ctx, now)
				if err != nil {
					log.Printf("[gateway] poll (%s): %v", source.Name(), err)
					continue
				}
				for i := range messages {
					payload, err := MarshalPayload(&messages[i])
					if err != nil {
						log.Printf("[gateway] marshal %s: %v", messages[i].MessageID, err)
						continue
					}
					if err := buf.Enqueue(ctx, messages[i].MessageID, payload); err != nil {
						log.Printf("[gateway] enqueue: %v", err)
					}
				}
			}
		}
	}()
}

// logRejected appends a line to the resync register so field engineers can
// reconcile devices that produce messages the server cannot accept.
func (r *Runner) logRejected(messageID, reason string) {
	if r.rejectedLog == nil {
		f, err := os.OpenFile("gateway-rejected.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		r.rejectedLog = f
	}
	line := time.Now().UTC().Format(schemaTimeLayout) + " " + messageID + " " + reason + "\n"
	_, _ = r.rejectedLog.WriteString(line)
}

const schemaTimeLayout = "2006-01-02T15:04:05.000Z"
