package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"cap/internal/store"
)

// getObject decodes a JSON response into a single object.
func getObject(t *testing.T, env *testEnv, path string) (*http.Response, map[string]any) {
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

func TestProfileLifecycleDraftApproveActivate(t *testing.T) {
	env := newTestEnv(t)

	resp, profiles := getJSON(t, env, "/api/v1/profiles")
	if resp.StatusCode != http.StatusOK || len(profiles) != 3 {
		t.Fatalf("seeded profiles = %d (status %d)", len(profiles), resp.StatusCode)
	}
	_, active := getObject(t, env, "/api/v1/profiles/active")
	if active["id"] != "ore-baseline-01" {
		t.Fatalf("active profile = %v", active["id"])
	}

	createResp, created := postJSON(t, env, "/api/v1/profiles", map[string]any{
		"name":       "Test oxide ore",
		"ore_domain": "oxide_copper",
		"author":     "metallurgist-01",
		"reason":     "Pilot campaign",
		"params": map[string]any{
			"thresholds": map[string]any{"ph_level": map[string]any{"min": 6.0, "max": 10.0}},
		},
	})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", createResp.StatusCode)
	}
	if created["status"] != store.ProfileStatusDraft {
		t.Fatalf("new profile status = %v", created["status"])
	}
	profileID := created["id"].(string)

	// A draft cannot be activated directly.
	actDraft, _ := postJSON(t, env, "/api/v1/profiles/"+profileID+"/activate", map[string]any{})
	if actDraft.StatusCode != http.StatusConflict {
		t.Fatalf("activate draft status = %d, want 409", actDraft.StatusCode)
	}

	// Approve, then activate.
	apprResp, approved := postJSON(t, env, "/api/v1/profiles/"+profileID+"/approve", map[string]any{
		"approved_by": "chief-metallurgist",
	})
	if apprResp.StatusCode != http.StatusOK || approved["status"] != store.ProfileStatusApproved {
		t.Fatalf("approve = %v (status %d)", approved["status"], apprResp.StatusCode)
	}
	actResp, _ := postJSON(t, env, "/api/v1/profiles/"+profileID+"/activate", map[string]any{})
	if actResp.StatusCode != http.StatusOK {
		t.Fatalf("activate status = %d", actResp.StatusCode)
	}
	_, activeNow := getObject(t, env, "/api/v1/profiles/active")
	if activeNow["id"] != profileID {
		t.Fatalf("active switched to %v, want %s", activeNow["id"], profileID)
	}
}

func TestProfileParamsMustBeJSON(t *testing.T) {
	env := newTestEnv(t)
	resp, _ := postJSON(t, env, "/api/v1/profiles", map[string]any{
		"name": "bad", "ore_domain": "x", "author": "a", "params": "not-json",
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
}

func TestAlarmAcknowledgeLifecycle(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := store.UpsertAlarmActive(ctx, env.db, particleTagID, "particle_size",
		"critical", "particle_size over limit", now); err != nil {
		t.Fatalf("upsert alarm: %v", err)
	}

	resp, active := getJSON(t, env, "/api/v1/alarms/active")
	if resp.StatusCode != http.StatusOK || len(active) != 1 {
		t.Fatalf("active alarms = %d (status %d)", len(active), resp.StatusCode)
	}
	if active[0]["state"] != store.AlarmStateActiveUnack {
		t.Fatalf("state = %v", active[0]["state"])
	}
	alarmID := active[0]["id"].(string)

	ackResp, acked := postJSON(t, env, "/api/v1/alarms/"+alarmID+"/ack", map[string]any{
		"ack_by": "operator-01", "comment": "noticed, monitoring",
	})
	if ackResp.StatusCode != http.StatusOK {
		t.Fatalf("ack status = %d", ackResp.StatusCode)
	}
	if acked["state"] != store.AlarmStateActiveAck || acked["ack_by"] != "operator-01" {
		t.Fatalf("acked = %v", acked)
	}

	// Re-acking is rejected.
	again, _ := postJSON(t, env, "/api/v1/alarms/"+alarmID+"/ack", map[string]any{"ack_by": "x"})
	if again.StatusCode != http.StatusConflict {
		t.Fatalf("re-ack status = %d, want 409", again.StatusCode)
	}
}

func TestAggregateDownsamplesNumericReadings(t *testing.T) {
	env := newTestEnv(t)
	readings := []map[string]any{
		telemetryPayload(payloadOpts{messageID: messageIDOne, value: 5.0, observedAt: "2026-07-31T08:15:30.000Z"}),
		telemetryPayload(payloadOpts{messageID: messageIDTwo, value: 6.0, observedAt: "2026-07-31T08:16:00.000Z"}),
		telemetryPayload(payloadOpts{messageID: messageIDThree, value: 7.0, observedAt: "2026-07-31T08:16:30.000Z"}),
	}
	resp, _ := postJSON(t, env, "/api/v1/ingest/telemetry:batch", readings)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("batch status = %d", resp.StatusCode)
	}

	path := "/api/v1/telemetry/aggregate?tag_id=" + particleTagID +
		"&resolution=5m&agg=avg&from_time=2026-07-31T08:00:00Z&to_time=2026-07-31T09:00:00Z"
	aggResp, err := http.Get(env.server.URL + path)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	defer aggResp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(aggResp.Body).Decode(&out)

	if out["function"] != "avg" {
		t.Fatalf("function = %v", out["function"])
	}
	buckets := out["buckets"].([]any)
	if len(buckets) != 1 {
		t.Fatalf("buckets = %v", buckets)
	}
	b := buckets[0].(map[string]any)
	if b["count"] != float64(3) {
		t.Fatalf("count = %v", b["count"])
	}
	if b["avg"] != float64(6) {
		t.Fatalf("avg = %v", b["avg"])
	}
	if b["min"] != float64(5) || b["max"] != float64(7) || b["sum"] != float64(18) {
		t.Fatalf("min/max/sum = %v", b)
	}
}

func TestLatestFlagsStaleData(t *testing.T) {
	env := newTestEnv(t)
	// observedAt is 2026-07-31T08:15:30Z, far older than the 60s staleness window.
	payload := telemetryPayload(payloadOpts{messageID: messageIDOne})
	resp, _ := postJSON(t, env, "/api/v1/ingest/telemetry", payload)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("ingest status = %d", resp.StatusCode)
	}

	_, latest := getObject(t, env, "/api/v1/telemetry/latest?tag_id="+particleTagID)
	if latest["stale"] != true {
		t.Fatalf("stale = %v, want true", latest["stale"])
	}
	if latest["quality"] != "stale" {
		t.Fatalf("quality = %v, want stale", latest["quality"])
	}
	if latest["age_seconds"] == nil || latest["age_seconds"].(float64) <= 0 {
		t.Fatalf("age_seconds = %v", latest["age_seconds"])
	}
}
