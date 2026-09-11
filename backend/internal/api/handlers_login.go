package api

import (
	"net/http"
	"time"

	"cap/internal/auth"
	"cap/internal/store"
)

// Login authenticates a local user (TZ §12): PBKDF2 password check, opaque
// session token in an HttpOnly cookie. 204 on success, 401 on bad credentials.
func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	if req.Username == "" || req.Password == "" {
		writeProblem(w, http.StatusUnprocessableEntity, "required", "username and password are required")
		return
	}

	user, err := store.GetUserByName(r.Context(), s.db, req.Username)
	if err != nil || !user.Active || !auth.VerifyPassword(req.Password, user.PasswordHash) {
		// Uniform error: no account enumeration.
		writeProblem(w, http.StatusUnauthorized, "bad_credentials", "неверное имя пользователя или пароль")
		return
	}

	token, err := auth.NewToken()
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	now := s.now().UTC()
	ttl := 12 * time.Hour
	if err := store.CreateSession(r.Context(), s.db, auth.HashToken(token), user.Username, ttl, now); err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
	s.audit(r, "access.login", "user", user.Username, "")
	writeJSON(w, http.StatusOK, map[string]any{"username": user.Username, "label": user.Label})
}

// Logout invalidates the current session.
func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = store.DeleteSession(r.Context(), s.db, auth.HashToken(c.Value))
		s.audit(r, "access.logout", "user", subjectOf(r), "")
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	writeJSON(w, http.StatusNoContent, nil)
}
