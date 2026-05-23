package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

const CookieName = "spg_session"

type ctxKey struct{}

var userCtxKey = ctxKey{}

// AuthGate wraps the given handler. Routes under /api/ require a valid session
// cookie except for /api/auth/* (login, setup, me) which must remain reachable
// to unauthenticated callers. Non-/api/ traffic (the static SPA, /healthz) is
// not gated — the dashboard JS gates itself on the API responses.
func AuthGate(store *Store, base http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requiresAuth(r) {
			c, err := r.Cookie(CookieName)
			if err != nil {
				writeUnauthorized(w)
				return
			}
			u, err := store.Verify(r.Context(), c.Value)
			if err != nil {
				writeUnauthorized(w)
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), userCtxKey, u))
		}
		base.ServeHTTP(w, r)
	})
}

func requiresAuth(r *http.Request) bool {
	p := r.URL.Path
	if !strings.HasPrefix(p, "/api/") {
		return false
	}
	if strings.HasPrefix(p, "/api/auth/") {
		return false
	}
	// /api/agent/ (Phase 7) authenticates via per-install agent tokens, not
	// session cookies — leave it open here; the agent handler does its own check.
	if strings.HasPrefix(p, "/api/agent/") {
		return false
	}
	return true
}

func UserFromContext(ctx context.Context) *User {
	u, _ := ctx.Value(userCtxKey).(*User)
	return u
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}
