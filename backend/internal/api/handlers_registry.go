package api

import (
	"encoding/json"
	"net/http"

	"autopro/internal/store"
)

// handlers_registry.go — CRUD endpoints for the admin-managed asset & tag
// registry. Assets and tags are created, updated and deleted through these
// handlers so the plant can evolve its signal list without code changes.

// CreateAsset handles POST /api/v1/assets
func (s *Server) CreateAsset(w http.ResponseWriter, r *http.Request) {
	var a store.Asset
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"code": "invalid_json", "message": "body is not valid JSON",
		})
		return
	}
	if a.Name == "" || a.Area == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"code": "validation", "message": "name and area are required",
		})
		return
	}
	if err := store.CreateAsset(r.Context(), s.db, &a); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{
			"code": "duplicate", "message": err.Error(),
		})
		return
	}
	s.audit(r, "asset.create", "asset", a.ID, "name="+a.Name+" area="+a.Area)
	writeJSON(w, http.StatusCreated, a)
}

// UpdateAsset handles PUT /api/v1/assets/{id}
func (s *Server) UpdateAsset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var a store.Asset
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"code": "invalid_json", "message": "payload is not valid JSON",
		})
		return
	}
	a.ID = id
	if err := store.UpdateAsset(r.Context(), s.db, &a); err != nil {
		if err == store.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"code": "not_found", "message": "asset not found",
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"code": "update_failed", "message": err.Error(),
		})
		return
	}
	s.audit(r, "asset.update", "asset", id, "name="+a.Name+" area="+a.Area)
	writeJSON(w, http.StatusOK, a)
}

// DeleteAsset handles DELETE /api/v1/assets/{id}
func (s *Server) DeleteAsset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := store.DeleteAsset(r.Context(), s.db, id); err != nil {
		if err == store.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"code": "not_found", "message": "asset not found",
			})
			return
		}
		writeJSON(w, http.StatusConflict, map[string]string{
			"code": "dependent_tags", "message": err.Error(),
		})
		return
	}
	s.audit(r, "asset.delete", "asset", id, "")
	writeJSON(w, http.StatusNoContent, nil)
}

// CreateTag handles POST /api/v1/tags
func (s *Server) CreateTag(w http.ResponseWriter, r *http.Request) {
	var t store.Tag
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"code": "invalid_json", "message": "payload is not valid JSON",
		})
		return
	}
	if t.AssetID == "" || t.Unit == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"code": "required", "message": "asset_id and unit are required",
		})
		return
	}
	if t.DataType == "" {
		t.DataType = "number"
	}
	if err := store.CreateTag(r.Context(), s.db, &t); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{
			"code": "duplicate_or_bad_ref", "message": err.Error(),
		})
		return
	}
	s.audit(r, "tag.create", "tag", t.ID, "asset="+t.AssetID+" unit="+t.Unit)
	writeJSON(w, http.StatusCreated, t)
}

// UpdateTag handles PUT /api/v1/tags/{id}
func (s *Server) UpdateTag(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var t store.Tag
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"code": "invalid_json", "message": "payload is not valid JSON",
		})
		return
	}
	t.ID = id
	if err := store.UpdateTag(r.Context(), s.db, &t); err != nil {
		if err == store.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"code": "not_found", "message": "tag not found",
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"code": "update_failed", "message": err.Error(),
		})
		return
	}
	s.audit(r, "tag.update", "tag", id, "asset="+t.AssetID+" unit="+t.Unit)
	writeJSON(w, http.StatusOK, t)
}

// DeleteTag handles DELETE /api/v1/tags/{id}
func (s *Server) DeleteTag(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := store.DeleteTag(r.Context(), s.db, id); err != nil {
		if err == store.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"code": "not_found", "message": "tag not found",
			})
			return
		}
		writeJSON(w, http.StatusConflict, map[string]string{
			"code": "historical_data", "message": err.Error(),
		})
		return
	}
	s.audit(r, "tag.delete", "tag", id, "")
	writeJSON(w, http.StatusNoContent, nil)
}
