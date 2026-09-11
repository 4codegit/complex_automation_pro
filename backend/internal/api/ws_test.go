package api_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// wsCookie extracts the session cookie from the authenticated client jar so
// the WebSocket dialer can present it.
func wsCookie(t *testing.T, env *testEnv) string {
	t.Helper()
	u, err := url.Parse(env.server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	for _, c := range env.client.Jar.Cookies(u) {
		if c.Name == "cap_session" {
			return c.Name + "=" + c.Value
		}
	}
	t.Fatal("no session cookie in jar")
	return ""
}

// TestWebSocketStreamsLiveTelemetry guards the Hijack path: the auth and
// logging middleware must let authenticated WebSocket upgrades through, or
// the dashboard goes dark. Telemetry events follow the TZ §13 shape.
func TestWebSocketStreamsLiveTelemetry(t *testing.T) {
	env := newTestEnv(t)
	wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + "/api/v1/ws"

	hdr := http.Header{"Cookie": []string{wsCookie(t, env)}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	resp, _ := postJSON(t, env, "/api/v1/ingest/telemetry", telemetryPayload(payloadOpts{messageID: messageIDOne}))
	if resp.StatusCode != 202 {
		t.Fatalf("ingest status = %d", resp.StatusCode)
	}

	var sawTelemetry bool
	for !sawTelemetry {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read ws: %v", err)
		}
		var ev map[string]any
		if err := json.Unmarshal(data, &ev); err != nil {
			continue
		}
		if ev["type"] == "telemetry" {
			sawTelemetry = true
			if ev["tag_id"] != feedTagID {
				t.Fatalf("unexpected tag: %v", ev)
			}
			if ev["value"] != 100.0 {
				t.Fatalf("unexpected value: %v", ev)
			}
			if ev["quality"] != "good" {
				t.Fatalf("unexpected quality: %v", ev)
			}
		}
	}
}

// TestWebSocketRejectsUnauthenticatedClients: no session cookie, no upgrade.
func TestWebSocketRejectsUnauthenticatedClients(t *testing.T) {
	env := newTestEnvNoLogin(t)
	wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + "/api/v1/ws"
	_, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("unauthenticated ws upgrade succeeded, want rejection")
	}
}
