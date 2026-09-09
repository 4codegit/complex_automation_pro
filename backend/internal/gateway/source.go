package gateway

import (
	"context"
	"time"

	"cap/internal/schema"
)

// Source emits canonical telemetry messages for a gateway. Implementations:
// the simulated Sensor, the opcua.Source, the modbusSource and future mqtt
// drivers. Sources MUST be safe for concurrent use by exactly one polling loop.
//
// Read-only invariant (Level 3, ISA-95): a Source exposes read operations only
// against the OT asset. Implementations MUST NOT provide a write path; control
// remains outside CAP.
type Source interface {
	// Name returns the driver id for logs/metrics, e.g. "simulated", "opcua".
	Name() string
	// Poll reads every configured tag once at t and returns canonical messages.
	// Subscription-based drivers MAY return a delta-only slice since the last
	// call; an empty slice means "no new data" and is not an error.
	Poll(ctx context.Context, now time.Time) ([]schema.Telemetry, error)
	// Close releases the underlying session/connection. Must be idempotent.
	Close() error
}

// Compile-time checks: confirm concrete drivers satisfy Source. The opcua
// driver's check lives in opcua.go (next to the impl) to keep the gopcua
// import out of this file.
var (
	_ Source = (*Sensor)(nil)
)
