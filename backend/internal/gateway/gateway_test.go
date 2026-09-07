package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cap/internal/schema"
)

func TestBufferPersistsAndDrains(t *testing.T) {
	ctx := context.Background()
	buf, err := OpenBuffer(filepath.Join(t.TempDir(), "buf.db"))
	if err != nil {
		t.Fatalf("open buffer: %v", err)
	}
	defer buf.Close()

	for i := 0; i < 5; i++ {
		if err := buf.Enqueue(ctx, id(i), `{"message_id":"`+id(i)+`"}`); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}
	// Duplicate is ignored.
	if err := buf.Enqueue(ctx, id(0), `{}`); err != nil {
		t.Fatalf("enqueue dup: %v", err)
	}

	if n, _ := buf.Len(ctx); n != 5 {
		t.Fatalf("len = %d, want 5", n)
	}
	snap, err := buf.Snapshot(ctx, 3)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snap) != 3 || snap[0].MessageID != id(0) {
		t.Fatalf("snapshot = %v", snap)
	}

	if err := buf.Delete(ctx, []string{id(0), id(1)}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n, _ := buf.Len(ctx); n != 3 {
		t.Fatalf("len after delete = %d, want 3", n)
	}
}

func TestSenderBackfillsBufferAfterOutage(t *testing.T) {
	// A server that records batches; the gateway delivers after recovery.
	var mu sync.Mutex
	var received []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var items []schema.Telemetry
		if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		mu.Lock()
		for _, it := range items {
			received = append(received, it.MessageID)
		}
		mu.Unlock()
		results := make([]schema.IngestItemResult, 0, len(items))
		for _, it := range items {
			results = append(results, schema.IngestItemResult{MessageID: it.MessageID, Status: schema.StatusAccepted})
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(schema.IngestResponse{Results: results})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buf, err := OpenBuffer(filepath.Join(t.TempDir(), "buf.db"))
	if err != nil {
		t.Fatalf("open buffer: %v", err)
	}
	defer buf.Close()

	// 1. Simulate readings queued while the server was DOWN (no sender running).
	for i := 0; i < 10; i++ {
		msg := schema.Telemetry{
			SchemaVersion: schema.SchemaVersion,
			MessageID:     id(i),
			GatewayID:     "gw-test",
			AssetID:       "plant-a.crushing",
			TagID:         "plant-a.crushing.particle_size",
			Value:         float64(i),
			Unit:          "mm",
			Quality:       schema.QualityGood,
			ObservedAt:    time.Now().UTC(),
		}
		payload, err := MarshalPayload(&msg)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := buf.Enqueue(ctx, msg.MessageID, payload); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}
	if n, _ := buf.Len(ctx); n != 10 {
		t.Fatalf("buffer before recovery = %d, want 10", n)
	}

	// 2. Server is back: the sender drains the queue.
	sender := NewSender(srv.URL, 100, 10*time.Millisecond)
	go sender.Run(ctx, buf)

	deadline := time.After(5 * time.Second)
	for {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count >= 10 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("delivered %d/10", count)
		case <-time.After(50 * time.Millisecond):
		}
	}

	if n, _ := buf.Len(ctx); n != 0 {
		t.Fatalf("buffer after recovery = %d, want 0", n)
	}
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < 10; i++ {
		if received[i] != id(i) {
			t.Fatalf("order broken at %d: %v", i, received)
		}
	}
}

func TestSenderDropsRejectedAndKeepsOthers(t *testing.T) {
	// Server rejects the odd ids; the sender must remove them and keep going.
	reject := map[string]bool{id(1): true, id(3): true}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var items []schema.Telemetry
		_ = json.NewDecoder(r.Body).Decode(&items)
		results := make([]schema.IngestItemResult, 0, len(items))
		for _, it := range items {
			if reject[it.MessageID] {
				results = append(results, schema.IngestItemResult{
					MessageID: it.MessageID, Status: schema.StatusRejected,
					Code: schema.Str(schema.CodeUnitMismatch), Message: schema.Str("unit mismatch"),
				})
			} else {
				results = append(results, schema.IngestItemResult{MessageID: it.MessageID, Status: schema.StatusAccepted})
			}
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(schema.IngestResponse{Results: results})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buf, err := OpenBuffer(filepath.Join(t.TempDir(), "buf.db"))
	if err != nil {
		t.Fatalf("open buffer: %v", err)
	}
	defer buf.Close()

	for i := 0; i < 5; i++ {
		msg := schema.Telemetry{SchemaVersion: schema.SchemaVersion, MessageID: id(i), GatewayID: "gw", TagID: "x.y.z", Value: 1.0, Unit: "u", Quality: schema.QualityGood, ObservedAt: time.Now().UTC()}
		payload, _ := MarshalPayload(&msg)
		if err := buf.Enqueue(ctx, msg.MessageID, payload); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}

	var rejectedMu sync.Mutex
	rejectedIDs := map[string]string{}
	sender := NewSender(srv.URL, 100, 10*time.Millisecond)
	sender.Rejected = func(mid, reason string) {
		rejectedMu.Lock()
		rejectedIDs[mid] = reason
		rejectedMu.Unlock()
	}
	go sender.Run(ctx, buf)

	deadline := time.After(5 * time.Second)
	for {
		n, _ := buf.Len(ctx)
		if n == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("buffer stuck at %d", n)
		case <-time.After(50 * time.Millisecond):
		}
	}

	rejectedMu.Lock()
	defer rejectedMu.Unlock()
	if len(rejectedIDs) != 2 {
		t.Fatalf("rejected register = %v, want 2 entries", rejectedIDs)
	}
	if rejectedIDs[id(1)] != "unit mismatch" {
		t.Fatalf("rejection reason = %v", rejectedIDs)
	}
}

func id(i int) string {
	return "msg-" + string(rune('a'+i))
}
