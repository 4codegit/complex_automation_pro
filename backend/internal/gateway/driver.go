package gateway

import (
	"fmt"
	"strings"
)

// Driver names supported by NewSource.
const (
	DriverSimulated = "simulated"
	DriverOPCUA     = "opcua"
	DriverModbus    = "modbus"
	DriverSparkplug = "sparkplug"
)

// NewSource builds the configured Source for cfg. The default is the simulated
// sensor so the demo continues to work with no extra env. Unknown drivers
// return an explicit error so misconfiguration is surfaced at startup, not at
// first poll.
func NewSource(cfg *Config) (Source, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	if driver == "" {
		driver = DriverSimulated
	}
	switch driver {
	case DriverSimulated:
		return NewSensor(cfg.ID, cfg.Tags), nil
	case DriverOPCUA:
		return newOPCUASource(cfg)
	case DriverModbus:
		return newModbusSource(cfg)
	case DriverSparkplug:
		return newSparkplugSource(cfg)
	default:
		return nil, fmt.Errorf("unsupported SOURCE_DRIVER %q (want %q, %q, %q or %q)",
			driver, DriverSimulated, DriverOPCUA, DriverModbus, DriverSparkplug)
	}
}
