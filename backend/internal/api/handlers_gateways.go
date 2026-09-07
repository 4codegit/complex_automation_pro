package api

import (
	"net/http"

	"autopro/internal/schema"
	"autopro/internal/store"
)

// IngestGatewayEvent accepts the edge gateway heartbeat (pulse/online/offline)
// and refreshes the gateway registry row.
func (s *Server) IngestGatewayEvent(w http.ResponseWriter, r *http.Request) {
	var ev schema.GatewayEvent
	if err := decodeBody(w, r, &ev); err != nil {
		writeProblem(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	if err := ev.Validate(); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_event", err.Error())
		return
	}

	ts := ev.Timestamp.UTC()
	status := ev.Status
	if ev.Status == "" {
		status = "online"
	}
	if err := store.UpsertGateway(r.Context(), s.db, &store.Gateway{
		ID:         ev.GatewayID,
		Name:       ev.GatewayID,
		Area:       "",
		Protocol:   "push",
		Status:     status,
		LastSeenAt: &ts,
		BufferSize: ev.BufferSize,
		Version:    ev.Version,
		CreatedAt:  ts,
	}); err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "accepted", "gateway_id": ev.GatewayID, "received_at": s.now().UTC(),
	})
}

// ListGateways returns the registered edge gateways with last known state.
func (s *Server) ListGateways(w http.ResponseWriter, r *http.Request) {
	gateways, err := store.ListGateways(r.Context(), s.db)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, gateways)
}
