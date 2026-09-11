package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"time"

	"cap/internal/schema"
)

// Sender delivers buffered messages to the server as batches. Delivery is
// confirmation-driven: only accepted/duplicate messages are removed from the
// queue; rejected ones are logged as a resync register and dropped (retrying
// would never fix them); transport failures back off exponentially.
type Sender struct {
	client   *http.Client
	server   string
	token    string // device credential (X-Gateway-Token)
	maxBatch int
	interval time.Duration
	backoff  time.Duration
	wake     chan struct{}
	// Rejected is called (if set) with each rejected message id and reason.
	Rejected func(messageID, reason string)
	// Latency is the last successful round-trip; used by the pulse.
	Latency time.Duration
}

// NewSender builds a batch deliverer.
func NewSender(server, token string, maxBatch int, interval time.Duration) *Sender {
	return &Sender{
		client:   &http.Client{Timeout: 10 * time.Second},
		server:   server,
		token:    token,
		maxBatch: maxBatch,
		interval: interval,
		backoff:  1 * time.Second,
		wake:     make(chan struct{}, 1),
	}
}

// post sends an authenticated JSON request to the platform. Machine endpoints
// accept the gateway token instead of a user session.
func (s *Sender) post(ctx context.Context, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.server+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.token != "" {
		req.Header.Set("X-Gateway-Token", s.token)
	}
	return s.client.Do(req)
}

// Wake triggers an immediate flush attempt (used on reconnect).
func (s *Sender) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run flushes the queue until ctx is done. It never fails the process: the
// whole point is resilience against server outages.
func (s *Sender) Run(ctx context.Context, buf *Buffer) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.flushOnce(ctx, buf)
		case <-s.wake:
			s.flushOnce(ctx, buf)
		}
	}
}

// flushOnce sends one snapshot; returns the count of delivered messages.
func (s *Sender) flushOnce(ctx context.Context, buf *Buffer) int {
	snapshot, err := buf.Snapshot(ctx, s.maxBatch)
	if err != nil {
		log.Printf("[gateway] buffer snapshot: %v", err)
		return 0
	}
	if len(snapshot) == 0 {
		s.backoff = time.Second
		return 0
	}

	items := make([]schema.Telemetry, 0, len(snapshot))
	for _, p := range snapshot {
		var t schema.Telemetry
		if err := json.Unmarshal([]byte(p.Payload), &t); err != nil {
			log.Printf("[gateway] corrupt buffered message %s: %v", p.MessageID, err)
			continue
		}
		items = append(items, t)
	}
	if len(items) == 0 {
		return 0
	}

	raw, err := json.Marshal(items)
	if err != nil {
		return 0
	}

	start := time.Now()
	resp, err := s.post(ctx, "/api/v1/ingest/telemetry:batch", raw)
	if err != nil {
		log.Printf("[gateway] server unreachable (%d in buffer), retry in %s", len(items), s.backoff)
		s.backOff()
		return 0
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusAccepted {
		log.Printf("[gateway] server returned %d, retry in %s", resp.StatusCode, s.backoff)
		s.backOff()
		return 0
	}

	s.Latency = time.Since(start)
	s.backoff = time.Second

	var ingest schema.IngestResponse
	if err := json.Unmarshal(body, &ingest); err != nil {
		log.Printf("[gateway] unexpected server response: %v", err)
		return 0
	}

	confirmed := make([]string, 0, len(ingest.Results))
	for _, res := range ingest.Results {
		switch res.Status {
		case schema.StatusAccepted, schema.StatusDuplicate:
			confirmed = append(confirmed, res.MessageID)
		case schema.StatusRejected:
			reason := "unknown"
			if res.Message != nil {
				reason = *res.Message
			}
			log.Printf("[gateway] rejected %s: %s", res.MessageID, reason)
			if s.Rejected != nil {
				s.Rejected(res.MessageID, reason)
			}
			confirmed = append(confirmed, res.MessageID) // drop: retry can't fix it
		}
	}
	if err := buf.Delete(ctx, confirmed); err != nil {
		log.Printf("[gateway] buffer delete: %v", err)
		return 0
	}
	if n, _ := buf.Len(ctx); n > 0 {
		log.Printf("[gateway] delivered %d, %d remain in buffer", len(confirmed), n)
	}
	return len(confirmed)
}

func (s *Sender) backOff() {
	// 1s -> 2s -> 4s -> ... -> 10s, with jitter
	next := s.backoff * 2
	if next > 10*time.Second {
		next = 10 * time.Second
	}
	jitter := time.Duration(rand.Int64N(int64(500 * time.Millisecond)))
	s.backoff = next + jitter
}
