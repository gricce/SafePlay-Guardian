package api

import (
	"io/fs"
	"net/http"

	"github.com/gricce/SafePlay-Guardian/internal/auth"
	"github.com/gricce/SafePlay-Guardian/web"
)

// pageRoutes maps clean URLs to the embedded HTML files that back them.
// The actual auth gate is client-side — pages call /api/auth/me on load and
// redirect when unauthenticated — so unauthenticated visitors can still load
// the login / first-run shells.
var pageRoutes = map[string]string{
	"/home":              "home.html",
	"/devices":           "devices.html",
	"/child":             "child.html",
	"/reports":           "reports.html",
	"/policy-time":       "policy-time.html",
	"/policy-schedule":   "policy-schedule.html",
	"/policy-monitoring": "policy-monitoring.html",
	"/first-run":         "first-run.html",
	"/login":             "login.html",
}

// spaHandler serves the dashboard SPA shell. Three concerns layered:
//
//  1. `/` redirects to /home, /login, or /first-run depending on whether the
//     parent has a session, an account, or neither — avoids the flash of an
//     unauthenticated home page before client-side redirect kicks in.
//  2. clean URLs (`/devices`) rewrite to the embedded HTML files
//     (`devices.html`) so users see no `.html` in the address bar.
//  3. everything else (`/css/...`, `/js/...`) is delegated to http.FileServer.
func spaHandler(authStore *auth.Store) http.Handler {
	spa, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(spa))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path

		if p == "/" {
			http.Redirect(w, r, rootRedirect(r, authStore), http.StatusFound)
			return
		}
		if file, ok := pageRoutes[p]; ok {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/" + file
			files.ServeHTTP(w, r2)
			return
		}
		files.ServeHTTP(w, r)
	})
}

func rootRedirect(r *http.Request, store *auth.Store) string {
	if store == nil {
		return "/home"
	}
	if c, err := r.Cookie(auth.CookieName); err == nil {
		if _, err := store.Verify(r.Context(), c.Value); err == nil {
			return "/home"
		}
	}
	has, err := store.HasUsers(r.Context())
	if err == nil && !has {
		return "/first-run"
	}
	return "/login"
}
