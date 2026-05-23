package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gricce/SafePlay-Guardian/internal/auth"
)

type AuthHandler struct {
	Store *auth.Store
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(auth.SessionLifetime),
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: auth.CookieName, Value: "", Path: "/",
		HttpOnly: true, MaxAge: -1, SameSite: http.SameSiteStrictMode,
	})
}

// Setup creates the first parent account. Returns 400 if anyone already
// exists — there is exactly one chance to claim the dashboard.
func (h *AuthHandler) Setup(w http.ResponseWriter, r *http.Request) {
	has, err := h.Store.HasUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "setup")
		return
	}
	if has {
		writeError(w, http.StatusBadRequest, "setup already complete")
		return
	}
	var req credentials
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	user, err := h.Store.Create(r.Context(), req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	token, err := h.Store.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "issue session")
		return
	}
	setSessionCookie(w, token)
	writeJSON(w, http.StatusCreated, user)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	token, err := h.Store.Login(r.Context(), req.Username, req.Password)
	if errors.Is(err, auth.ErrBadCredentials) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "login")
		return
	}
	setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.CookieName); err == nil {
		_ = h.Store.Logout(r.Context(), c.Value)
	}
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Me reports the current session state. Always returns 200 — the response
// body distinguishes "authenticated", "needs setup", and "needs login" so the
// SPA can pick the right view without hitting a 401.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(auth.CookieName)
	if err == nil {
		if u, err := h.Store.Verify(r.Context(), c.Value); err == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"authenticated": true, "user": u,
			})
			return
		}
	}
	has, _ := h.Store.HasUsers(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated":  false,
		"setup_complete": has,
	})
}
