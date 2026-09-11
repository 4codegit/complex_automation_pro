package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"cap/internal/alarms"
	"cap/internal/api"
	"cap/internal/auth"
	"cap/internal/config"
	"cap/internal/hub"
	"cap/internal/store"
)

type testEnv struct {
	server *httptest.Server
	db     *sql.DB
	hub    *hub.Hub
	alarms *alarms.Engine
	client *http.Client // carries the admin session cookie
}

// newTestEnv builds a fully seeded server: flotation plant registry, alarm
// limits, control loops, roles and an admin account. The returned client is
// authenticated as admin (platform_admin) via a real login.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	env := newTestEnvNoLogin(t)
	env.login(t, "admin", "admin")
	return env
}

// newTestEnvNoLogin returns the server without an authenticated client.
func newTestEnvNoLogin(t *testing.T) *testEnv {
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
		t.Fatalf("seed registry: %v", err)
	}
	if err := store.SeedAlarmLimits(ctx, db); err != nil {
		t.Fatalf("seed alarm limits: %v", err)
	}
	if err := store.SeedControlLoops(ctx, db); err != nil {
		t.Fatalf("seed control loops: %v", err)
	}
	if err := store.SeedRoles(ctx, db); err != nil {
		t.Fatalf("seed roles: %v", err)
	}
	if err := store.EnsureDefaultProfile(ctx, db); err != nil {
		t.Fatalf("seed profiles: %v", err)
	}
	hash, err := auth.HashPassword("admin")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := store.CreateUser(ctx, db, &store.User{Username: "admin", PasswordHash: hash, Active: true}); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	h := hub.New()
	eng := alarms.New(db, h)
	cfg := &config.Settings{
		Environment:  "test",
		StalenessSec: 60 * time.Second,
		MaxBatchSize: 500,
		DefaultLimit: 100,
		MaxLimit:     1000,
		ControlStale: 10 * time.Second,
	}
	srv := api.New(db, h, cfg, eng, nil)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	// A plain client (no cookie jar) so helper calls work pre-login.
	return &testEnv{server: ts, db: db, hub: h, alarms: eng, client: &http.Client{}}
}

// login authenticates the env client as the given user.
func (env *testEnv) login(t *testing.T, username, password string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	env.client = &http.Client{Jar: jar}
	resp, body := env.do(t, http.MethodPost, "/api/v1/access/login",
		map[string]string{"username": username, "password": password})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login %s: %d body=%s", username, resp.StatusCode, body)
	}
}

// sessionToken creates a server-side session for an existing user and returns
// the raw token (used by RBAC tests that switch subjects).
func sessionToken(env *testEnv, subject string) (string, error) {
	token, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	err = store.CreateSession(context.Background(), env.db, auth.HashToken(token), subject, time.Hour, time.Now().UTC())
	return token, err
}

// seedUser creates a login account with the given password.
func seedUser(t *testing.T, env *testEnv, username string) {
	t.Helper()
	seedUserPassword(t, env, username, "pw-"+username)
}

func seedUserPassword(t *testing.T, env *testEnv, username, password string) {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := store.CreateUser(context.Background(), env.db, &store.User{
		Username: username, PasswordHash: hash, Active: true,
	}); err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
}

// seedUserWithRole creates a user, assigns the system role and creates the
// server-side session used by callWithCookie.
func seedUserWithRole(t *testing.T, env *testEnv, username, role string) {
	t.Helper()
	seedUserPassword(t, env, username, "pw-"+username)
	if err := store.AssignRole(context.Background(), env.db, username, role, "tests"); err != nil {
		t.Fatalf("assign %s: %v", username, err)
	}
}

// do performs a JSON round-trip with the authenticated client.
func (env *testEnv) do(t *testing.T, method, path string, body any) (*http.Response, string) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rd = bytes.NewReader(raw)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, env.server.URL+path, rd)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return resp, buf.String()
}

func postJSON(t *testing.T, env *testEnv, path string, body any) (*http.Response, map[string]any) {
	t.Helper()
	resp, raw := env.do(t, http.MethodPost, path, body)
	return resp, decodeMap(t, raw)
}

func putJSON(t *testing.T, env *testEnv, path string, body any) (*http.Response, map[string]any) {
	t.Helper()
	resp, raw := env.do(t, http.MethodPut, path, body)
	return resp, decodeMap(t, raw)
}

func getJSON(t *testing.T, env *testEnv, path string) (*http.Response, []map[string]any) {
	t.Helper()
	resp, raw := env.do(t, http.MethodGet, path, nil)
	out := []map[string]any{}
	_ = json.Unmarshal([]byte(raw), &out)
	return resp, out
}

func getMap(t *testing.T, env *testEnv, path string) (*http.Response, map[string]any) {
	t.Helper()
	resp, raw := env.do(t, http.MethodGet, path, nil)
	return resp, decodeMap(t, raw)
}

func decodeMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	out := map[string]any{}
	if raw == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	return out
}

const (
	assetID       = "plant.crushing"
	feedTagID     = "plant.crushing.fi101" // t/h
	pHTagID       = "plant.flotation.ai301"
	messageIDOne  = "018f0e6b-7f1a-7e1d-a0bd-5b31d918c001"
	messageIDTwo  = "018f0e6b-7f1a-7e1d-a0bd-5b31d918c002"
	messageIDThr  = "018f0e6b-7f1a-7e1d-a0bd-5b31d918c003"
	observedAtStr = "2026-07-31T08:15:30.125Z"
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
		o.tagID = feedTagID
	}
	if o.unit == "" {
		o.unit = "t/h"
	}
	if o.value == nil {
		o.value = 100.0
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

	path := "/api/v1/telemetry?tag_id=" + feedTagID +
		"&asset_id=" + assetID +
		"&from_time=2026-07-31T08:00:00Z&to_time=2026-07-31T09:00:00Z"
	gresp, history := getJSON(t, env, path)
	if gresp.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d", gresp.StatusCode)
	}
	if len(history) != 1 {
		t.Fatalf("history len = %d, want 1", len(history))
	}
	if history[0]["value"] != 100.0 {
		t.Fatalf("value = %v", history[0]["value"])
	}
	if history[0]["quality"] != "good" {
		t.Fatalf("quality = %v", history[0]["quality"])
	}

	_, latest := getMap(t, env, "/api/v1/telemetry/latest?tag_id="+feedTagID)
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
				p["tag_id"] = "plant.crushing.unknown"
				return p
			},
			expectedMsg: "unknown_tag",
		},
		{
			name: "asset_tag_mismatch",
			override: func(p map[string]any) map[string]any {
				p["asset_id"] = "plant.flotation"
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
		messageID:  messageIDThr,
		tagID:      pHTagID,
		unit:       "pH",
		value:      10.2,
		assetID:    "plant.flotation",
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
	if history[0]["message_id"] != messageIDThr || history[1]["message_id"] != messageIDOne {
		t.Fatalf("history order = %v", history)
	}
}

func TestRegistryCatalogIsSeeded(t *testing.T) {
	env := newTestEnv(t)

	resp, assets := getJSON(t, env, "/api/v1/assets")
	if resp.StatusCode != http.StatusOK || len(assets) != 6 {
		t.Fatalf("assets status/len = %d/%d, want 6 assets", resp.StatusCode, len(assets))
	}
	byID := map[string]bool{}
	for _, a := range assets {
		byID[a["id"].(string)] = true
	}
	for _, want := range []string{"plant.crushing", "plant.grinding", "plant.flotation", "plant.thickening", "plant.filtration", "plant.metallurgy"} {
		if !byID[want] {
			t.Fatalf("missing asset %q", want)
		}
	}

	_, tags := getJSON(t, env, "/api/v1/tags")
	if len(tags) != 43 { // 28 inputs + 7 actuators + 8 calc tags
		t.Fatalf("tags = %d, want 43", len(tags))
	}
	directions := map[string]int{}
	for _, tg := range tags {
		directions[tg["direction"].(string)]++
	}
	if directions["output"] != 7 {
		t.Fatalf("output tags = %d, want 7", directions["output"])
	}

	_, roles := getJSON(t, env, "/api/v1/access/roles")
	roleSet := map[string]bool{}
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
