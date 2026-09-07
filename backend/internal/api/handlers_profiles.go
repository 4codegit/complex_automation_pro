package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"autopro/internal/store"
)

// ListProfiles returns all ore profiles, newest first.
func (s *Server) ListProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := store.ListProfiles(r.Context(), s.db)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, profiles)
}

type createProfileRequest struct {
	Name      string          `json:"name"`
	OreDomain string          `json:"ore_domain"`
	Params    json.RawMessage `json:"params"`
	Author    string          `json:"author"`
	Reason    string          `json:"reason"`
	Version   int             `json:"version"`
}

// CreateProfile registers a draft ore profile. Params must be a JSON object.
func (s *Server) CreateProfile(w http.ResponseWriter, r *http.Request) {
	var req createProfileRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	if req.Name == "" || req.OreDomain == "" || req.Author == "" {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_profile",
			"name, ore_domain and author are required")
		return
	}
	var probe map[string]any
	if err := json.Unmarshal(req.Params, &probe); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_params", "params must be a JSON object")
		return
	}
	compacted, _ := json.Marshal(probe)

	version := req.Version
	if version < 1 {
		version = 1
	}
	p, err := store.CreateProfile(r.Context(), s.db, &store.Profile{
		ID:        store.NewID(),
		Name:      req.Name,
		OreDomain: req.OreDomain,
		Version:   version,
		Params:    string(compacted),
		Author:    req.Author,
		Reason:    req.Reason,
	})
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "profile.create", "profile", p.ID, "name="+req.Name+" ore="+req.OreDomain+" v="+strconv.Itoa(version))
	writeJSON(w, http.StatusCreated, p)
}

type approveProfileRequest struct {
	ApprovedBy string `json:"approved_by"`
}

// ApproveProfile promotes a draft profile to approved (change-controlled).
func (s *Server) ApproveProfile(w http.ResponseWriter, r *http.Request) {
	var req approveProfileRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	if req.ApprovedBy == "" {
		writeProblem(w, http.StatusUnprocessableEntity, "missing_approver", "approved_by is required")
		return
	}
	id := r.PathValue("id")
	if err := store.ApproveProfile(r.Context(), s.db, id, req.ApprovedBy); err != nil {
		writeProblem(w, http.StatusConflict, "approve_failed", err.Error())
		return
	}
	s.audit(r, "profile.approve", "profile", id, "by="+req.ApprovedBy)
	p, err := store.GetProfile(r.Context(), s.db, id)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ActivateProfile switches the plant to the given profile. Only approved or
// already-active profiles can be activated.
func (s *Server) ActivateProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := store.GetProfile(r.Context(), s.db, id)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "profile not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if p.Status != store.ProfileStatusApproved && p.Status != store.ProfileStatusActive {
		writeProblem(w, http.StatusConflict, "activation_failed",
			"profile must be approved before activation")
		return
	}
	if err := store.SetActiveProfile(r.Context(), s.db, id); err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "profile.activate", "profile", id, "ore="+p.OreDomain+" v="+strconv.Itoa(p.Version))
	active, err := store.GetProfile(r.Context(), s.db, id)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, active)
}

// ActiveProfile returns the currently active ore profile, or JSON null.
func (s *Server) ActiveProfile(w http.ResponseWriter, r *http.Request) {
	p, err := store.GetActiveProfile(r.Context(), s.db)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}
