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
	return getMap(t, env, path)
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
		"name":       "Окисленная руда (пилот)",
		"ore_domain": "oxide_copper",
		"author":     "metallurgist-01",
		"reason":     "Pilot campaign",
		"params": map[string]any{
			"alpha_nominal": 0.65, "beta_target": 18.0, "epsilon_target_min": 80.0,
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

	if err := store.UpsertAlarmActive(ctx, env.db, "plant.flotation.aft301", "aft301",
		"critical", "Содержание в хвостах выше hi_hi", now); err != nil {
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

	// The actor is the session subject (admin here), not a request field.
	ackResp, acked := postJSON(t, env, "/api/v1/alarms/"+alarmID+"/ack", map[string]any{
		"comment": "замечено, регулируем",
	})
	if ackResp.StatusCode != http.StatusOK {
		t.Fatalf("ack status = %d", ackResp.StatusCode)
	}
	if acked["state"] != store.AlarmStateActiveAck || acked["ack_by"] != "admin" {
		t.Fatalf("acked = %v", acked)
	}

	// Re-acking is rejected.
	again, _ := postJSON(t, env, "/api/v1/alarms/"+alarmID+"/ack", map[string]any{})
	if again.StatusCode != http.StatusConflict {
		t.Fatalf("re-ack status = %d, want 409", again.StatusCode)
	}
}

func TestAlarmJournalFiltersByPeriodAndPriority(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	now := time.Now().UTC()
	_ = store.UpsertAlarmActive(ctx, env.db, "plant.flotation.aft301", "aft301", "critical", "θ выше hi_hi", now)
	_ = store.InsertAlarmEvent(ctx, env.db, &store.AlarmEvent{
		OccurredAt: now, TagID: "plant.flotation.aft301", Metric: "aft301",
		Event: store.AlarmEventRaised, Severity: "critical", Priority: 1, Message: "θ выше hi_hi",
	})
	_ = store.InsertAlarmEvent(ctx, env.db, &store.AlarmEvent{
		OccurredAt: now, TagID: "plant.crushing.tit101", Metric: "tit101",
		Event: store.AlarmEventRaised, Severity: "medium", Priority: 3, Message: "температура вне коридора",
	})

	resp, journal := getJSON(t, env, "/api/v1/alarms/journal?min_priority=2")
	if resp.StatusCode != http.StatusOK || len(journal) == 0 {
		t.Fatalf("journal = %d (status %d)", len(journal), resp.StatusCode)
	}
	for _, ev := range journal {
		if ev["priority"].(float64) > 2 {
			t.Fatalf("journal returned low-priority event: %v", ev)
		}
	}

	_, byTag := getJSON(t, env, "/api/v1/alarms/journal?tag_id=plant.crushing.tit101")
	if len(byTag) != 1 || byTag[0]["tag_id"] != "plant.crushing.tit101" {
		t.Fatalf("tag filter = %v", byTag)
	}
}

func TestAggregateDownsamplesNumericReadings(t *testing.T) {
	env := newTestEnv(t)
	readings := []map[string]any{
		telemetryPayload(payloadOpts{messageID: messageIDOne, value: 98.0, observedAt: "2026-07-31T08:15:30.000Z"}),
		telemetryPayload(payloadOpts{messageID: messageIDTwo, value: 100.0, observedAt: "2026-07-31T08:16:00.000Z"}),
		telemetryPayload(payloadOpts{messageID: messageIDThr, value: 102.0, observedAt: "2026-07-31T08:16:30.000Z"}),
	}
	resp, _ := postJSON(t, env, "/api/v1/ingest/telemetry:batch", readings)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("batch status = %d", resp.StatusCode)
	}

	path := "/api/v1/telemetry/aggregate?tag_id=" + feedTagID +
		"&resolution=5m&agg=avg&from_time=2026-07-31T08:00:00Z&to_time=2026-07-31T09:00:00Z"
	_, out := getMap(t, env, path)

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
	if b["avg"] != float64(100) {
		t.Fatalf("avg = %v", b["avg"])
	}
	if b["min"] != float64(98) || b["max"] != float64(102) || b["sum"] != float64(300) {
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

	_, latest := getObject(t, env, "/api/v1/telemetry/latest?tag_id="+feedTagID)
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

func TestCSVReportExport(t *testing.T) {
	env := newTestEnv(t)
	_, _ = postJSON(t, env, "/api/v1/ingest/telemetry", telemetryPayload(payloadOpts{messageID: messageIDOne}))

	resp, raw := env.do(t, "GET", "/api/v1/reports/readings/csv?tag_id="+feedTagID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("csv status = %d", resp.StatusCode)
	}
	if len(raw) == 0 || json.Valid([]byte(raw)) {
		t.Fatalf("csv payload unexpected: %q", raw[:min(len(raw), 80)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
