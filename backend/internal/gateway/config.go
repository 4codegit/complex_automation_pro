// Package gateway implements the edge collector: local sensor polling,
// normalization to the canonical contract, store-and-forward buffering in a
// local SQLite file, resilient batch delivery with exponential backoff and a
// heartbeat pulse to the server (TZ_DEVELOPMENT.md S3).
package gateway

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// TagSpec is one local instrument the gateway polls. The OPC UA NodeID is
// used by the opcua driver, the Modbus register spec by the modbus driver,
// the Sparkplug metric path by the sparkplug driver; simulated drivers
// ignore all of them.
type TagSpec struct {
	TagID    string
	AssetID  string
	Unit     string
	Baseline float64
	Noise    float64
	Min      float64
	Max      float64
	NodeID   string
	// Modbus is the register spec "fc:addr:type[:scale]" (see modbus.go).
	Modbus string
	// Sparkplug is the metric path "group/edge[/device]/metric_name".
	Sparkplug string
}

// Config holds the gateway runtime settings.
type Config struct {
	ID            string
	ServerURL     string
	BufferDB      string
	PollInterval  time.Duration
	PushInterval  time.Duration
	PulseInterval time.Duration
	MaxBatch      int
	Tags          []TagSpec

	// Driver selects the Source implementation. "" or "simulated" keeps the
	// demo sensor; "opcua" activates the OPC UA poll driver; "modbus" the
	// Modbus TCP poll driver.
	Driver string
	// Modbus TCP driver settings (ignored by other drivers).
	ModbusAddr    string
	ModbusUnitID  uint8
	ModbusTimeout time.Duration
	// Supervisory control write bridge (ADR-003). CONTROL_ENABLED=false keeps
	// the gateway strictly read-only; enabling it allows exactly one holding
	// register write (FC6) driven by the server's computed loop output.
	ControlEnabled bool
	ControlOutTag  string
	ControlWrite   string
	ControlPoll    time.Duration
	// MQTT/Sparkplug B driver settings (ignored by other drivers).
	MQTTBroker   string
	MQTTClientID string
	MQTTUsername string
	MQTTPassword string
	// OPC UA driver settings (ignored by other drivers).
	OPCUAEndpoint   string
	OPCUASecurity   string
	OPCUAPolicy     string
	OPCUAAuth       string
	OPCUAUsername   string
	OPCUAPassword   string
	OPCUACertDir    string
	OPCUAStaleAfter time.Duration
	// OPCUAMode selects poll or subscribe collection. Default is "poll".
	// Subscription mode creates one OPC UA subscription per source. Tags
	// without an explicit mode inherit this global default.
	OPCUAMode        string
	OPCUASubInterval time.Duration
}

// LoadConfig reads the gateway settings from the environment (envPath is an
// optional .env file). Production deployments override via the environment.
func LoadConfig(envPath string) (*Config, error) {
	loadDotEnv(envPath)

	cfg := &Config{
		ID:               get("GATEWAY_ID", "gw-crushing-01"),
		ServerURL:        strings.TrimRight(get("SERVER_URL", "http://127.0.0.1:8000"), "/"),
		BufferDB:         get("GATEWAY_BUFFER_DB", "./gateway-buffer.db"),
		PollInterval:     getDuration("POLL_INTERVAL", 1*time.Second),
		PushInterval:     getDuration("PUSH_INTERVAL", 1*time.Second),
		PulseInterval:    getDuration("PULSE_INTERVAL", 5*time.Second),
		MaxBatch:         getInt("MAX_BATCH", 100),
		Driver:           get("SOURCE_DRIVER", DriverSimulated),
		ModbusAddr:       get("MODBUS_ADDR", ""),
		ModbusUnitID:     getUint8("MODBUS_UNIT_ID", 1),
		ModbusTimeout:    getDuration("MODBUS_TIMEOUT", 1*time.Second),
		ControlEnabled:   getBool2("CONTROL_ENABLED", false),
		ControlOutTag:    get("CONTROL_OUT_TAG", ""),
		ControlWrite:     get("CONTROL_WRITE", ""),
		ControlPoll:      getDuration("CONTROL_POLL", 1*time.Second),
		MQTTBroker:       get("MQTT_BROKER", ""),
		MQTTClientID:     get("MQTT_CLIENT_ID", ""),
		MQTTUsername:     get("MQTT_USERNAME", ""),
		MQTTPassword:     get("MQTT_PASSWORD", ""),
		OPCUAEndpoint:    get("OPCUA_ENDPOINT", ""),
		OPCUASecurity:    get("OPCUA_SECURITY", "auto"),
		OPCUAPolicy:      get("OPCUA_POLICY", "auto"),
		OPCUAAuth:        get("OPCUA_AUTH", "anonymous"),
		OPCUAUsername:    get("OPCUA_USERNAME", ""),
		OPCUAPassword:    get("OPCUA_PASSWORD", ""),
		OPCUACertDir:     get("OPCUA_CERT_DIR", ""),
		OPCUAStaleAfter:  getDuration("OPCUA_STALE_AFTER", 30*time.Second),
		OPCUAMode:        get("OPCUA_MODE", "poll"),
		OPCUASubInterval: getDuration("OPCUA_SUB_INTERVAL", 500*time.Millisecond),
	}
	tags, err := parseTags(get("TAGS", defaultTags()))
	if err != nil {
		return nil, err
	}
	cfg.Tags = tags
	return cfg, nil
}

// defaultTags mirrors the seeded demo registry (one edge gateway per area).
func defaultTags() string {
	return strings.Join([]string{
		"plant-a.crushing.particle_size:mm",
		"plant-a.crushing.pulp_density:g/cm3",
		"plant-a.flotation.ph_level:pH",
		"plant-a.flotation.reagent_dosage:mL/min",
		"plant-a.dewatering.cake_moisture:%",
		"plant-a.dewatering.dryer_temperature:C",
		"plant-a.concentrate.tonnage_weight:t/h",
		"plant-a.concentrate.final_moisture:%",
	}, ",")
}

// demoMetrics provides plausible operating points so the simulated sensor
// yields realistic values without a real instrument.
var demoMetrics = map[string]struct {
	baseline, noise, min, max float64
}{
	"particle_size":     {5.0, 0.5, 0.0, 12.0},
	"pulp_density":      {1.65, 0.05, 1.0, 2.5},
	"ph_level":          {9.5, 0.3, 7.0, 11.0},
	"reagent_dosage":    {55.0, 5.0, 10.0, 120.0},
	"cake_moisture":     {8.0, 0.5, 0.0, 12.0},
	"dryer_temperature": {150.0, 5.0, 60.0, 250.0},
	"tonnage_weight":    {250.0, 10.0, 0.0, 500.0},
	"final_moisture":    {5.0, 0.3, 0.0, 10.0},
}

func parseTags(raw string) ([]TagSpec, error) {
	var specs []TagSpec
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		// Split off the driver address spec. The canonical separator is "|":
		//   asset.tag.metric:unit|node=<NodeID>        (opcua driver)
		//   asset.tag.metric:unit|reg=<fc>:<addr>:<t>  (modbus driver)
		//   asset.tag.metric:unit|sp=group/edge/name   (sparkplug driver)
		// The ":" in the OPC UA NodeID (e.g. "ns=2;s=Sim.PV") makes the older
		// "=" separator ambiguous, so it is trimmed only for back-compat and
		// the trailing "=" is stripped from the unit.
		var nodeID, regSpec, spSpec string
		if idx := strings.Index(item, "|"); idx >= 0 {
			spec := strings.TrimSpace(item[idx+1:])
			item = strings.TrimSpace(item[:idx])
			switch {
			case strings.HasPrefix(spec, "reg="):
				regSpec = strings.TrimPrefix(spec, "reg=")
			case strings.HasPrefix(spec, "sp="):
				spSpec = strings.TrimPrefix(spec, "sp=")
			default:
				nodeID = strings.TrimPrefix(spec, "node=")
			}
		} else if i := strings.Index(item, "node="); i >= 0 {
			nodeID = strings.TrimSpace(item[i+len("node="):])
			item = strings.TrimRight(item[:i], "=")
		}
		parts := strings.Split(item, ":")
		tagID := parts[0]
		unit := "count"
		if len(parts) > 1 && parts[1] != "" {
			unit = parts[1]
		}
		seg := strings.Split(tagID, ".")
		if len(seg) < 3 {
			return nil, fmt.Errorf("tag spec %q must be asset.tag.metric[:unit]", item)
		}
		metric := seg[len(seg)-1]
		dm, ok := demoMetrics[metric]
		if !ok {
			dm = demoMetrics["particle_size"]
		}
		specs = append(specs, TagSpec{
			TagID:     tagID,
			AssetID:   strings.Join(seg[:len(seg)-1], "."),
			Unit:      unit,
			Baseline:  dm.baseline,
			Noise:     dm.noise,
			Min:       dm.min,
			Max:       dm.max,
			NodeID:    nodeID,
			Modbus:    regSpec,
			Sparkplug: spSpec,
		})
	}
	if len(specs) == 0 {
		return nil, fmt.Errorf("TAGS must contain at least one tag spec")
	}
	return specs, nil
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getUint8(key string, def uint8) uint8 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseUint(v, 10, 8); err == nil {
			return uint8(n)
		}
	}
	return def
}

func getBool2(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v == "1" || strings.EqualFold(v, "true")
}

func getDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// loadDotEnv reads a simple KEY=VALUE file without overriding existing env.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	buf := make([]byte, 0, 1<<16)
	chunk := make([]byte, 4096)
	for {
		n, err := f.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
		}
		if err != nil {
			break
		}
	}
	lines := strings.Split(string(buf), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.Trim(strings.TrimSpace(line[eq+1:]), `"'`)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, val)
	}
}
