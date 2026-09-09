package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"cap/internal/store"
)

// getJSONMap fetches a JSON object endpoint.
func getJSONMap(t *testing.T, env *testEnv, path string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := http.Get(env.server.URL + path)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

// insertAnalyticsReading writes one numeric reading directly into the store,
// bypassing ingestion (analytics reads the historian, not the ingest path).
func insertAnalyticsReading(t *testing.T, env *testEnv, tagID string, value float64) {
	t.Helper()
	now := time.Now().UTC()
	err := store.InsertReading(context.Background(), env.db, &store.Reading{
		ID:          store.NewID(),
		MessageID:   store.NewID(),
		GatewayID:   "gw-analytics-test",
		ObservedAt:  now,
		ReceivedAt:  now,
		AssetID:     "plant-a.analytics",
		TagID:       tagID,
		ValueNumber: &value,
		Unit:        "",
		Quality:     "good",
	})
	if err != nil {
		t.Fatalf("insert reading %s: %v", tagID, err)
	}
}

// TestProcessAnalyticsComputesMetallurgy verifies the derived-indicator math:
// percent solids from slurry density, specific reagent consumption, dry
// throughput and profile-corridor statuses.
func TestProcessAnalyticsComputesMetallurgy(t *testing.T) {
	env := newTestEnv(t) // all-in-one: profile seeds present

	insertAnalyticsReading(t, env, "plant-a.crushing.pulp_density", 1.4)
	insertAnalyticsReading(t, env, "plant-a.flotation.reagent_dosage", 55)
	insertAnalyticsReading(t, env, "plant-a.concentrate.tonnage_weight", 250)
	insertAnalyticsReading(t, env, "plant-a.concentrate.final_moisture", 5)
	insertAnalyticsReading(t, env, "plant-a.flotation.ph_level", 9)
	insertAnalyticsReading(t, env, "plant-a.dewatering.cake_moisture", 8)
	insertAnalyticsReading(t, env, "plant-a.crushing.particle_size", 5)

	resp, body := getJSONMap(t, env, "/api/v1/analytics/process")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("analytics status = %d: %v", resp.StatusCode, body)
	}

	kpis, ok := body["kpis"].([]any)
	if !ok || len(kpis) == 0 {
		t.Fatalf("no kpis in response: %v", body)
	}
	byID := map[string]map[string]any{}
	for _, raw := range kpis {
		k, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		byID[k["id"].(string)] = k
	}

	assertKPI := func(id string, wantValue, tolerance float64, wantStatus string) {
		t.Helper()
		k, ok := byID[id]
		if !ok {
			t.Fatalf("kpi %q missing: %v", id, byID)
		}
		value, _ := k["value"].(float64)
		if abs(value-wantValue) > tolerance {
			t.Errorf("kpi %s = %v, want ~%v", id, value, wantValue)
		}
		if k["status"] != wantStatus {
			t.Errorf("kpi %s status = %v, want %s", id, k["status"], wantStatus)
		}
		if k["formula"] == "" || k["detail"] == "" {
			t.Errorf("kpi %s must expose formula and detail for auditability", id)
		}
	}

	// %S = 2.7·(1.4−1)/((2.7−1)·1.4)·100 = 45.35%
	assertKPI("solids_percent", 45.35, 0.1, "ok")
	// q = 55·60/250 = 13.2 mL/t
	assertKPI("reagent_specific", 13.2, 0.01, "ok")
	// Qdry = 250·(1−0.05) = 237.5 t/h
	assertKPI("dry_throughput", 237.5, 0.01, "ok")
	// ph 9 inside seeded corridor 7..11
	assertKPI("ph_corridor", 9, 0.001, "ok")
	// cake moisture 8 within 0..12
	assertKPI("moisture_vs_target", 8, 0.001, "ok")
	// particle size 5 within 0..12
	assertKPI("p80_corridor", 5, 0.001, "ok")

	if prof, ok := body["profile"].(map[string]any); !ok || prof["name"] == "" {
		t.Errorf("profile reference missing: %v", body["profile"])
	}
}

// TestProcessAnalyticsFlagsCorridorViolations: a pH above the profile's max
// must surface as a bad status with the overshoot in the detail line.
func TestProcessAnalyticsFlagsCorridorViolations(t *testing.T) {
	env := newTestEnv(t)
	insertAnalyticsReading(t, env, "plant-a.flotation.ph_level", 12.5) // corridor 7..11

	_, body := getJSONMap(t, env, "/api/v1/analytics/process")
	for _, raw := range body["kpis"].([]any) {
		k := raw.(map[string]any)
		if k["id"] == "ph_corridor" {
			if k["status"] != "bad" {
				t.Errorf("ph_corridor status = %v, want bad", k["status"])
			}
			if detail, _ := k["detail"].(string); detail == "" {
				t.Errorf("violation detail must explain the overshoot")
			}
			return
		}
	}
	t.Fatalf("ph_corridor kpi missing: %v", body["kpis"])
}

// TestProcessAnalyticsNodataIsExplicit: an empty historian yields nodata KPIs,
// never fabricated values.
func TestProcessAnalyticsNodataIsExplicit(t *testing.T) {
	env := newTestEnv(t)
	_, body := getJSONMap(t, env, "/api/v1/analytics/process")
	for _, raw := range body["kpis"].([]any) {
		k := raw.(map[string]any)
		if k["status"] != "nodata" {
			t.Errorf("kpi %v status = %v, want nodata on empty historian", k["id"], k["status"])
		}
		if k["value"] != nil {
			t.Errorf("nodata kpi %v must carry null value, got %v", k["id"], k["value"])
		}
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
