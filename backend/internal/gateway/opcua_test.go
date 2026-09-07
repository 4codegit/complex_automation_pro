package gateway

import (
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"
)

func TestParseTagsNodeSpec(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantTag  string
		wantUnit string
		wantNode string
	}{
		{
			name:     "pipe separator (canonical)",
			raw:      "plant-a.crushing.particle_size:mm|node=ns=2;s=Sim.PV",
			wantTag:  "plant-a.crushing.particle_size",
			wantUnit: "mm",
			wantNode: "ns=2;s=Sim.PV",
		},
		{
			name:     "no node spec keeps simulated default",
			raw:      "plant-a.crushing.particle_size:mm",
			wantTag:  "plant-a.crushing.particle_size",
			wantUnit: "mm",
			wantNode: "",
		},
		{
			name:     "default unit when missing",
			raw:      "plant-a.crushing.particle_size|node=ns=2;s=Sim.PV",
			wantTag:  "plant-a.crushing.particle_size",
			wantUnit: "count",
			wantNode: "ns=2;s=Sim.PV",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			specs, err := parseTags(tc.raw)
			if err != nil {
				t.Fatalf("parseTags: %v", err)
			}
			if len(specs) != 1 {
				t.Fatalf("want 1 spec, got %d", len(specs))
			}
			s := specs[0]
			if s.TagID != tc.wantTag {
				t.Errorf("tag = %q, want %q", s.TagID, tc.wantTag)
			}
			if s.Unit != tc.wantUnit {
				t.Errorf("unit = %q, want %q", s.Unit, tc.wantUnit)
			}
			if s.NodeID != tc.wantNode {
				t.Errorf("node = %q, want %q", s.NodeID, tc.wantNode)
			}
		})
	}
}

func TestCoerceValue(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want any
	}{
		{"nil", nil, nil},
		{"float64", float64(3.14), float64(3.14)},
		{"bool", true, true},
		{"string", "hello", "hello"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var va *ua.Variant
			if c.v != nil {
				var err error
				va, err = ua.NewVariant(c.v)
				if err != nil {
					t.Fatalf("new variant: %v", err)
				}
			}
			got := coerceValue(va)
			if got != c.want {
				t.Errorf("coerceValue(%v) = %v, want %v", c.v, got, c.want)
			}
		})
	}
}

func TestConfigOPCUAModeDefault(t *testing.T) {
	script := `GATEWAY_ID=gw-opcua-01
SERVER_URL=http://127.0.0.1:8000
GATEWAY_BUFFER_DB=./gw.db
SOURCE_DRIVER=opcua
OPCUA_ENDPOINT=opc.tcp://127.0.0.1:4840
TAGS=plant-a.crushing.particle_size:mm|node=ns=2;s=Sim.PV`
	t.Setenv("GATEWAY_ID", "gw-opcua-01")
	t.Setenv("SERVER_URL", "http://127.0.0.1:8000")
	t.Setenv("GATEWAY_BUFFER_DB", "./gw.db")
	t.Setenv("SOURCE_DRIVER", "opcua")
	t.Setenv("OPCUA_ENDPOINT", "opc.tcp://127.0.0.1:4840")
	t.Setenv("TAGS", "plant-a.crushing.particle_size:mm|node=ns=2;s=Sim.PV")
	_ = script
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.OPCUAMode != "poll" {
		t.Errorf("default OPCUAMode = %q, want poll", cfg.OPCUAMode)
	}
	if cfg.OPCUASubInterval != 500*time.Millisecond {
		t.Errorf("default sub interval = %v, want 500ms", cfg.OPCUASubInterval)
	}
}
