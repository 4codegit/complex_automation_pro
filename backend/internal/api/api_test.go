package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"autopro/internal/api"
	"autopro/internal/config"
	"autopro/internal/hub"
	"autopro/internal/simulator"
	"autopro/internal/store"
)

type testEnv struct {
	server *httptest.Server
	db     *sql.DB
}

// registerCrushing mirrors the Python conftest: a fresh DB seeded with the demo
// registry. The crushing asset carries exactly particle_size + pulp_density.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx := context.Background()

	db, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := store.SeedRegistry(ctx, db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := store.SeedRoles(ctx, db); err != nil {
		t.Fatalf("seed roles: %v", err)
	}
	// Tests issue mutations without an X-User header (anonymous subject), so
	// grant platform_admin to "anonymous" once here. Production assigns roles
	// through the access/assignments API under manage_users permission.
	if err := store.AssignRole(ctx, db, "anonymous", "platform_admin", "tests"); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}
	if err := store.EnsureDefaultProfile(ctx, db); err != nil {
		t.Fatalf("seed profiles: %v", err)
	}

	h := hub.New()
	sim := simulator.New(db, h, 10*time.Millisecond, 1*time.Second)
	cfg := &config.Settings{
		Environment:  "test",
		StalenessSec: 60 * time.Second,
		MaxBatchSize: 500,
		DefaultLimit: 100,
		MaxLimit:     1000,
	}
	srv := api.New(db, h, sim, cfg)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	return &testEnv{server: ts, db: db}
}

const (
	assetID        = "plant-a.crushing"
	particleTagID  = "plant-a.crushing.particle_size"
	densityTagID   = "plant-a.crushing.pulp_density"
	messageIDOne   = "018f0e6b-7f1a-7e1d-a0bd-5b31d918c001"
	messageIDTwo   = "018f0e6b-7f1a-7e1d-a0bd-5b31d918c002"
	messageIDThree = "018f0e6b-7f1a-7e1d-a0bd-5b31d918c003"
	observedAtStr  = "2026-07-31T08:15:30.125Z"
)

type payloadOpts struct {
	messageID  string
	tagID      string
	unit       string
	value      any
	assetID    string
	observedAt string
}

func telemetryPayload(o payloadOpts) map[string]any {
	if o.tagID == "" {
		o.tagID = particleTagID
	}
	if o.unit == "" {
		o.unit = "mm"
	}
	if o.value == nil {
		o.value = 5.4
	}
	if o.assetID == "" {
		o.assetID = assetID
	}
	if o.observedAt == "" {
		o.observedAt = observedAtStr
	}
	return map[string]any{
		"schema_version":  "1.0",
		"message_id":      o.messageID,
		"gateway_id":      "gw-crushing-01",
		"source_sequence": 482119,
		"observed_at":     o.observedAt,
		"asset_id":        o.assetID,
		"tag_id":          o.tagID,
		"value":           o.value,
		"unit":            o.unit,
		"quality":         "good",
		"profile_id":      "ore-profile-pilot-01",
	}
}

func postJSON(t *testing.T, env *testEnv, path string, body any) (*http.Response, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(env.server.URL+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("post %s: %v", path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func getJSON(t *testing.T, env *testEnv, path string) (*http.Response, []map[string]any) {
	t.Helper()
	resp, err := http.Get(env.server.URL + path)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	defer resp.Body.Close()
	var out []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func parseTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return parsed
}

func TestIngestAcceptsReadingAndExposesItThroughPullAPI(t *testing.T) {
	env := newTestEnv(t)
	payload := telemetryPayload(payloadOpts{messageID: messageIDOne})

	resp, out := postJSON(t, env, "/api/v1/ingest/telemetry", payload)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("ingest status = %d, want 202", resp.StatusCode)
	}
	results := out["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results len = %d", len(results))
	}
	item := results[0].(map[string]any)
	if item["status"] != "accepted" || item["message_id"] != messageIDOne {
		t.Fatalf("unexpected item: %v", item)
	}

	path := "/api/v1/telemetry?tag_id=" + particleTagID +
		"&asset_id=" + assetID +
		"&from_time=2026-07-31T08:00:00Z&to_time=2026-07-31T09:00:00Z"
	gresp, history := getJSON(t, env, path)
	if gresp.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d", gresp.StatusCode)
	}
	if len(history) != 1 {
		t.Fatalf("history len = %d, want 1", len(history))
	}
	if history[0]["value"] != 5.4 {
		t.Fatalf("value = %v", history[0]["value"])
	}
	if history[0]["quality"] != "good" {
		t.Fatalf("quality = %v", history[0]["quality"])
	}

	latestResp, err := http.Get(env.server.URL + "/api/v1/telemetry/latest?tag_id=" + particleTagID)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	defer latestResp.Body.Close()
	var latest map[string]any
	_ = json.NewDecoder(latestResp.Body).Decode(&latest)
	if latest["message_id"] != messageIDOne {
		t.Fatalf("latest message_id = %v", latest["message_id"])
	}
	if !parseTime(t, latest["observed_at"].(string)).Equal(parseTime(t, observedAtStr)) {
		t.Fatalf("latest observed_at = %v", latest["observed_at"])
	}
}

func TestIngestIsIdempotentForGatewayMessageID(t *testing.T) {
	env := newTestEnv(t)
	payload := telemetryPayload(payloadOpts{messageID: messageIDOne})

	first, _ := postJSON(t, env, "/api/v1/ingest/telemetry", payload)
	retry, out := postJSON(t, env, "/api/v1/ingest/telemetry", payload)
	if first.StatusCode != http.StatusAccepted || retry.StatusCode != http.StatusAccepted {
		t.Fatalf("unexpected statuses %d/%d", first.StatusCode, retry.StatusCode)
	}
	if out["results"].([]any)[0].(map[string]any)["status"] != "duplicate" {
		t.Fatalf("expected duplicate, got %v", out)
	}

	_, history := getJSON(t, env, "/api/v1/telemetry")
	if len(history) != 1 {
		t.Fatalf("history len = %d, want 1", len(history))
	}
}

func TestIngestRejectsUnknownOrIncompatibleRegistryData(t *testing.T) {
	cases := []struct {
		name        string
		override    func(p map[string]any) map[string]any
		expectedMsg string
	}{
		{
			name: "unknown_tag",
			override: func(p map[string]any) map[string]any {
				p["tag_id"] = "plant-a.crushing.unknown"
				return p
			},
			expectedMsg: "unknown_tag",
		},
		{
			name: "asset_tag_mismatch",
			override: func(p map[string]any) map[string]any {
				p["asset_id"] = "plant-a.flotation"
				return p
			},
			expectedMsg: "asset_tag_mismatch",
		},
		{
			name: "unit_mismatch",
			override: func(p map[string]any) map[string]any {
				p["unit"] = "cm"
				return p
			},
			expectedMsg: "unit_mismatch",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			payload := tc.override(telemetryPayload(payloadOpts{messageID: messageIDOne}))

			resp, out := postJSON(t, env, "/api/v1/ingest/telemetry", payload)
			if resp.StatusCode != http.StatusAccepted {
				t.Fatalf("status = %d, want 202", resp.StatusCode)
			}
			item := out["results"].([]any)[0].(map[string]any)
			if item["status"] != "rejected" || item["code"] != tc.expectedMsg {
				t.Fatalf("item = %v, want rejected/%s", item, tc.expectedMsg)
			}

			_, history := getJSON(t, env, "/api/v1/telemetry")
			if len(history) != 0 {
				t.Fatalf("history len = %d, want 0", len(history))
			}
		})
	}
}

func TestBatchPreservesItemResultsAndStoresOnlyAcceptedReadings(t *testing.T) {
	env := newTestEnv(t)
	accepted := telemetryPayload(payloadOpts{messageID: messageIDOne})
	rejected := telemetryPayload(payloadOpts{messageID: messageIDTwo, unit: "cm"})
	secondAccepted := telemetryPayload(payloadOpts{
		messageID:  messageIDThree,
		tagID:      densityTagID,
		unit:       "g/cm3",
		value:      1.62,
		observedAt: "2026-07-31T08:15:31.125Z",
	})

	resp, out := postJSON(t, env, "/api/v1/ingest/telemetry:batch",
		[]any{accepted, rejected, secondAccepted})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	results := out["results"].([]any)
	statuses := make([]string, 0, len(results))
	for _, r := range results {
		statuses = append(statuses, r.(map[string]any)["status"].(string))
	}
	if len(statuses) != 3 || statuses[0] != "accepted" || statuses[1] != "rejected" || statuses[2] != "accepted" {
		t.Fatalf("statuses = %v", statuses)
	}
	if results[1].(map[string]any)["code"] != "unit_mismatch" {
		t.Fatalf("rejected code = %v", results[1].(map[string]any)["code"])
	}

	_, history := getJSON(t, env, "/api/v1/telemetry?limit=10")
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0]["message_id"] != messageIDThree || history[1]["message_id"] != messageIDOne {
		t.Fatalf("history order = %v", history)
	}
}

func TestRegistryAndRolesAreDiscoverable(t *testing.T) {
	env := newTestEnv(t)

	resp, assets := getJSON(t, env, "/api/v1/assets")
	if resp.StatusCode != http.StatusOK || len(assets) == 0 {
		t.Fatalf("assets status/len = %d/%d", resp.StatusCode, len(assets))
	}
	if assets[0]["id"] != assetID {
		t.Fatalf("assets[0] id = %v", assets[0]["id"])
	}

	_, tags := getJSON(t, env, "/api/v1/tags?asset_id="+assetID)
	ids := make(map[string]bool, len(tags))
	for _, tg := range tags {
		ids[tg["id"].(string)] = true
	}
	if !ids[densityTagID] || !ids[particleTagID] || len(tags) != 2 {
		t.Fatalf("tags for crushing = %v", tags)
	}

	_, roles := getJSON(t, env, "/api/v1/access/roles")
	roleSet := make(map[string]bool)
	for _, r := range roles {
		roleSet[r["id"].(string)] = true
	}
	for _, want := range []string{"director", "metallurgist", "operator", "ot_engineer", "maintenance", "platform_admin"} {
		if !roleSet[want] {
			t.Fatalf("missing role %q", want)
		}
	}
}

func TestInvalidTelemetrySchemaIsRejectedBeforeIngestion(t *testing.T) {
	env := newTestEnv(t)
	payload := telemetryPayload(payloadOpts{messageID: messageIDOne, value: "not-a-number"})

	resp, _ := postJSON(t, env, "/api/v1/ingest/telemetry", payload)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}

	_, history := getJSON(t, env, "/api/v1/telemetry")
	if len(history) != 0 {
		t.Fatalf("history len = %d, want 0", len(history))
	}
}
