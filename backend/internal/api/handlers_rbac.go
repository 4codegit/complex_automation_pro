package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"cap/internal/store"
)

// handlers_rbac.go — role & assignment management endpoints, exposed under
// /api/v1/access. The platform_admin manages roles and assignments; other
// roles may list and self-lookup.

// ListRoles publishes the permission matrix. This is read-only and open to
// all authed subjects (the matrix is needed for the UI to render roles).
func (s *Server) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := store.ListRoles(r.Context(), s.db)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, roles)
}

// CreateRole handler: POST /api/v1/access/roles (manage_roles). System roles
// cannot be created via API; seed-time only.
type createRoleRequest struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Permissions []string `json:"permissions"`
}

func (s *Server) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req createRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "body is not valid JSON")
		return
	}
	if req.ID == "" || req.Label == "" {
		writeProblem(w, http.StatusUnprocessableEntity, "required", "id and label are required")
		return
	}
	if err := store.CreateRole(r.Context(), s.db, &store.Role{
		ID: req.ID, Label: req.Label, Permissions: req.Permissions,
	}); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeProblem(w, http.StatusConflict, "duplicate", "role already exists")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	role, _ := store.GetRole(r.Context(), s.db, req.ID)
	s.audit(r, "role.create", "role", req.ID, "label="+req.Label)
	writeJSON(w, http.StatusCreated, role)
}

// UpdateRole handler: PUT /api/v1/access/roles/{id} (manage_roles)
func (s *Server) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "body is not valid JSON")
		return
	}
	if err := store.UpdateRolePermissions(r.Context(), s.db, id, req.Permissions); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeProblem(w, http.StatusNotFound, "not_found", "role not found")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	role, _ := store.GetRole(r.Context(), s.db, id)
	s.audit(r, "role.update", "role", id, "permissions="+strings.Join(req.Permissions, ","))
	writeJSON(w, http.StatusOK, role)
}

// DeleteRole handler: DELETE /api/v1/access/roles/{id} (manage_roles). System
// roles return 409.
func (s *Server) DeleteRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := store.DeleteRole(r.Context(), s.db, id); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeProblem(w, http.StatusNotFound, "not_found", "role not found")
		case errors.Is(err, store.ErrConflict):
			writeProblem(w, http.StatusConflict, "system_role", "system roles cannot be deleted")
		default:
			writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		}
		return
	}
	s.audit(r, "role.delete", "role", id, "")
	writeJSON(w, http.StatusNoContent, nil)
}

// AssignRole handler: POST /api/v1/access/assignments (manage_users)
type assignRoleRequest struct {
	Subject string `json:"subject"`
	RoleID  string `json:"role_id"`
}

func (s *Server) AssignRole(w http.ResponseWriter, r *http.Request) {
	var req assignRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "body is not valid JSON")
		return
	}
	if req.Subject == "" || req.RoleID == "" {
		writeProblem(w, http.StatusUnprocessableEntity, "required", "subject and role_id are required")
		return
	}
	actor := actorFromRequest(r)
	if err := store.AssignRole(r.Context(), s.db, req.Subject, req.RoleID, actor.Subject); err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "role.assign", "assignment", req.Subject, "role="+req.RoleID)
	writeJSON(w, http.StatusCreated, map[string]string{
		"subject": req.Subject, "role_id": req.RoleID,
	})
}

// RevokeRole handler: DELETE /api/v1/access/assignments?subject=&role_id= (manage_users)
func (s *Server) RevokeRole(w http.ResponseWriter, r *http.Request) {
	subject := queryStr(r, "subject")
	roleID := queryStr(r, "role_id")
	if subject == "" || roleID == "" {
		writeProblem(w, http.StatusBadRequest, "required", "subject and role_id query params are required")
		return
	}
	if err := store.RevokeRole(r.Context(), s.db, subject, roleID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeProblem(w, http.StatusNotFound, "not_found", "no such assignment")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.audit(r, "role.revoke", "assignment", subject, "role="+roleID)
	writeJSON(w, http.StatusNoContent, nil)
}

// ListAssignments handler: GET /api/v1/access/assignments?subject=
func (s *Server) ListAssignments(w http.ResponseWriter, r *http.Request) {
	subject := queryStr(r, "subject")
	as, err := store.ListAssignments(r.Context(), s.db, subject)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, as)
}

// WhoAmI exposes the authenticated subject with roles and resolved
// permissions so the UI can render available actions. Requires a session.
func (s *Server) WhoAmI(w http.ResponseWriter, r *http.Request) {
	subject := subjectOf(r)
	assignments, err := store.ListAssignments(r.Context(), s.db, subject)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	roles := make([]string, 0, len(assignments))
	permissions := map[string]bool{}
	for _, a := range assignments {
		roles = append(roles, a.RoleID)
		role, err := store.GetRole(r.Context(), s.db, a.RoleID)
		if err != nil {
			continue
		}
		for _, p := range role.Permissions {
			permissions[p] = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subject":     subject,
		"roles":       roles,
		"permissions": permissionList(permissions),
	})
}

func permissionList(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
