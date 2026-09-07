package gateway

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"

	"cap/internal/schema"
)

// opcuaSource is a read-only OPC UA poll driver. It opens one session to the
// configured endpoint and, on every Poll call, issues a single Read request
// covering all configured tags. OPC UA subscriptions are available when
// OPCUA_MODE=subscribe: the source switches the collect path to an
// in-process notification buffer drained by Poll on each tick.
//
// Read-only invariant (see ARCHITECTURE_DECISIONS.md ADR-001): this driver
// performs Read operations only. It exposes no write path to the OT asset.
type opcuaSource struct {
	cfg     *Config
	client  *opcua.Client
	sub     *opcua.Subscription // nil when mode=poll
	notify  chan *opcua.PublishNotificationData
	handles map[uint32]int // clientHandle -> tag index
	nodeIDs []*ua.NodeID   // indexed by tag index for poll path

	// Subscription-mode delta buffer: Notify goroutine appends, Poll drains.
	mu       sync.Mutex
	closed   bool
	connLast time.Time
	deltas   []schema.Telemetry
}

// newOPCUASource validates the config and constructs an OPC UA source. The
// session is established lazily on the first Poll so that a momentarily
// unreachable endpoint does not abort gateway startup: the local buffer
// absorbs the outage, and the first poll emits QualityOffline for each tag.
func newOPCUASource(cfg *Config) (Source, error) {
	if cfg.OPCUAEndpoint == "" {
		return nil, errors.New("OPCUA_ENDPOINT is required when SOURCE_DRIVER=opcua")
	}
	if len(cfg.Tags) == 0 {
		return nil, errors.New("TAGS must contain at least one tag spec for the opcua driver")
	}
	nodeIDs := make([]*ua.NodeID, 0, len(cfg.Tags))
	for _, t := range cfg.Tags {
		if t.NodeID == "" {
			return nil, fmt.Errorf("tag %q is missing a node spec (expected ...:unit=node=<NodeID>)", t.TagID)
		}
		nid, err := ua.ParseNodeID(t.NodeID)
		if err != nil {
			return nil, fmt.Errorf("tag %q: invalid NodeID %q: %w", t.TagID, t.NodeID, err)
		}
		nodeIDs = append(nodeIDs, nid)
	}
	s := &opcuaSource{cfg: cfg, nodeIDs: nodeIDs}
	mode := strings.ToLower(cfg.OPCUAMode)
	switch mode {
	case "", "poll":
		if err := s.connect(context.Background()); err != nil {
			log.Printf("[gateway/opcua] initial connect to %s failed: %v (will retry)", cfg.OPCUAEndpoint, err)
		}
	case "subscribe":
		if err := s.connect(context.Background()); err != nil {
			log.Printf("[gateway/opcua] initial connect to %s failed: %v (will retry)", cfg.OPCUAEndpoint, err)
		} else {
			if err := s.startSubscription(context.Background()); err != nil {
				log.Printf("[gateway/opcua] subscribe start failed: %v (falling back to poll)", err)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported OPCUA_MODE %q (want poll or subscribe)", mode)
	}
	return s, nil
}

// Name implements Source.
func (s *opcuaSource) Name() string { return DriverOPCUA }

// connect (re-)establishes the OPC UA session. Safe to call repeatedly.
func (s *opcuaSource) connect(ctx context.Context) error {
	if s.client != nil {
		_ = s.client.Close(ctx)
		s.client = nil
	}
	opts := []opcua.Option{
		opcua.SecurityPolicy(s.cfg.OPCUAPolicy),
		opcua.SecurityModeString(s.cfg.OPCUASecurity),
	}
	switch strings.ToLower(s.cfg.OPCUAAuth) {
	case "", "anonymous":
		opts = append(opts, opcua.AuthAnonymous())
	case "username":
		if s.cfg.OPCUAUsername == "" {
			return errors.New("OPCUA_AUTH=username requires OPCUA_USERNAME")
		}
		opts = append(opts, opcua.AuthUsername(s.cfg.OPCUAUsername, s.cfg.OPCUAPassword))
	default:
		return fmt.Errorf("unsupported OPCUA_AUTH %q", s.cfg.OPCUAAuth)
	}
	c, err := opcua.NewClient(s.cfg.OPCUAEndpoint, opts...)
	if err != nil {
		return fmt.Errorf("new client: %w", err)
	}
	if err := c.Connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	s.client = c
	return nil
}

// Poll reads every configured tag once and returns canonical messages. In
// subscription mode it drains the accumulated delta buffer; in poll mode it
// issues a single Read for all tags.
func (s *opcuaSource) Poll(ctx context.Context, now time.Time) ([]schema.Telemetry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("source closed")
	}

	if s.cfg.OPCUAMode == "subscribe" {
		return s.drainDeltas(), nil
	}
	return s.pollRead(ctx, now)
}

func (s *opcuaSource) pollRead(ctx context.Context, now time.Time) ([]schema.Telemetry, error) {
	if s.client == nil {
		if err := s.connect(ctx); err != nil {
			return s.offlineAll(now, fmt.Sprintf("connect: %v", err)), nil
		}
	}

	req := &ua.ReadRequest{
		MaxAge:             2000,
		TimestampsToReturn: ua.TimestampsToReturnSource,
		NodesToRead:        s.readValues(),
	}
	resp, err := s.client.Read(ctx, req)
	if err != nil {
		_ = s.client.Close(ctx)
		s.client = nil
		return s.offlineAll(now, fmt.Sprintf("read: %v", err)), nil
	}

	out := make([]schema.Telemetry, 0, len(s.cfg.Tags))
	for i, dv := range resp.Results {
		t := s.basicTelemetry(i, now)
		t.Quality = mapQuality(dv.Status)
		if dv.Value != nil {
			t.Value = coerceValue(dv.Value)
		}
		out = append(out, t)
	}
	return out, nil
}

// drainDeltas collects accumulated subscription notifications. An empty slice
// means "no new data" — the outer runner treats this as a non-event.
func (s *opcuaSource) drainDeltas() []schema.Telemetry {
	if len(s.deltas) == 0 {
		return nil
	}
	out := make([]schema.Telemetry, len(s.deltas))
	copy(out, s.deltas)
	s.deltas = s.deltas[:0]
	return out
}

// startSubscription creates one OPC UA subscription covering all tags,
// attaches monitored items, and launches a background notification drain.
func (s *opcuaSource) startSubscription(ctx context.Context) error {
	if s.client == nil {
		return errors.New("not connected")
	}
	params := &opcua.SubscriptionParameters{
		Interval:                   s.cfg.OPCUASubInterval,
		LifetimeCount:              uint32(s.cfg.OPCUASubInterval.Seconds()*4) + 1,
		MaxKeepAliveCount:          uint32(s.cfg.OPCUASubInterval.Seconds()*8) + 1,
		MaxNotificationsPerPublish: 0, // unlimited
	}
	s.notify = make(chan *opcua.PublishNotificationData, 64)
	sub, err := s.client.Subscribe(ctx, params, s.notify)
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	s.sub = sub
	s.handles = make(map[uint32]int, len(s.cfg.Tags))

	items := make([]*ua.MonitoredItemCreateRequest, 0, len(s.cfg.Tags))
	for i, nid := range s.nodeIDs {
		clientHandle := uint32(i + 1)
		s.handles[clientHandle] = i
		items = append(items, opcua.NewMonitoredItemCreateRequestWithDefaults(
			nid, ua.AttributeIDValue, clientHandle))
	}
	if _, err := sub.Monitor(ctx, ua.TimestampsToReturnSource, items...); err != nil {
		_ = sub.Cancel(ctx)
		s.sub = nil
		return fmt.Errorf("monitor: %w", err)
	}

	go s.notifyLoop(ctx)
	return nil
}

// notifyLoop drains the subscription notify channel and enqueues deltas.
func (s *opcuaSource) notifyLoop(ctx context.Context) {
	defer func() {
		s.mu.Lock()
		s.sub = nil
		s.mu.Unlock()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case n, ok := <-s.notify:
			if !ok {
				return
			}
			s.handleNotification(n)
		}
	}
}

func (s *opcuaSource) handleNotification(n *opcua.PublishNotificationData) {
	if n.Error != nil {
		log.Printf("[gateway/opcua-sub] notification error: %v", n.Error)
		return
	}
	dcn, ok := n.Value.(*ua.DataChangeNotification)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	for _, mi := range dcn.MonitoredItems {
		idx, ok := s.handles[mi.ClientHandle]
		if !ok {
			continue
		}
		t := s.basicTelemetry(idx, time.Now().UTC())
		t.Quality = mapQuality(mi.Value.Status)
		if mi.Value.Value != nil {
			t.Value = coerceValue(mi.Value.Value)
		}
		s.deltas = append(s.deltas, t)
	}
}
func (s *opcuaSource) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.sub != nil {
		_ = s.sub.Cancel(context.Background())
		s.sub = nil
	}
	if s.client != nil {
		_ = s.client.Close(context.Background())
		s.client = nil
	}
	return nil
}

// readValues builds the NodesToRead slice for a full Read call.
func (s *opcuaSource) readValues() []*ua.ReadValueID {
	ids := make([]*ua.ReadValueID, 0, len(s.nodeIDs))
	for _, nid := range s.nodeIDs {
		ids = append(ids, &ua.ReadValueID{
			NodeID:      nid,
			AttributeID: ua.AttributeIDValue,
		})
	}
	return ids
}

// basicTelemetry fills the invariant fields shared by good and offline paths.
func (s *opcuaSource) basicTelemetry(i int, now time.Time) schema.Telemetry {
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
		SourceSequence: nil, // opcua has no source sequence; per-tag seq left empty.
	}
}

// offlineAll returns one QualityOffline telemetry per tag, used when the session
// is unavailable so the historian records a single gap per outage tick.
func (s *opcuaSource) offlineAll(now time.Time, reason string) []schema.Telemetry {
	log.Printf("[gateway/opcua] offline: %s", reason)
	out := make([]schema.Telemetry, 0, len(s.cfg.Tags))
	for i := range s.cfg.Tags {
		t := s.basicTelemetry(i, now)
		t.Quality = schema.QualityOffline
		t.Value = nil
		out = append(out, t)
	}
	return out
}

// mapQuality converts an OPC UA StatusCode to the canonical CAP quality.
// See ARCHITECTURE_DECISIONS.md ADR-001 for the mapping table.
func mapQuality(st ua.StatusCode) string {
	switch {
	case st == ua.StatusGood:
		return schema.QualityGood
	case st >= ua.StatusUncertain && st < 0x80000000:
		return schema.QualityUncertain
	case st >= 0x80000000:
		return schema.QualityBad
	default:
		return schema.QualityUncertain
	}
}

// coerceValue reduces the OPC UA Variant.Info() union to the canonical value
// kinds understood by the server (number/boolean/string/structured).
func coerceValue(v *ua.Variant) any {
	if v == nil {
		return nil
	}
	switch v.Value().(type) {
	case bool:
		return v.Bool()
	case float32:
		return float64(v.Float())
	case float64:
		return v.Float()
	case int, int8, int16, int32, int64:
		return float64(v.Int())
	case uint, uint8, uint16, uint32, uint64:
		return float64(v.Int())
	case string:
		return v.String()
	default:
		// Anything else (arrays, extension objects, bytestrings) becomes a
		// structured value: serialise its JSON-friendly form.
		return v.Value()
	}
}

// compile-time check kept here (next to the concrete impl)
var _ Source = (*opcuaSource)(nil)
