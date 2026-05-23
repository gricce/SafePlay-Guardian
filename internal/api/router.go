package api

import (
	"log/slog"
	"net/http"

	"github.com/gricce/SafePlay-Guardian/internal/auth"
	"github.com/gricce/SafePlay-Guardian/internal/device"
	"github.com/gricce/SafePlay-Guardian/internal/enforce"
	"github.com/gricce/SafePlay-Guardian/internal/enforce/agent"
	"github.com/gricce/SafePlay-Guardian/internal/event"
	"github.com/gricce/SafePlay-Guardian/internal/person"
	"github.com/gricce/SafePlay-Guardian/internal/policy"
)

type Server struct {
	Registry     device.Registry
	People       *person.Store
	PolicyStore  policy.Store
	Resolver     *policy.Resolver
	Coordinator  *enforce.Coordinator
	Events       *event.Store
	EventLogger  *event.Logger
	Auth         *auth.Store
	AgentBackend *agent.Backend
	Logger       *slog.Logger
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})

	dh := &DeviceHandler{Registry: s.Registry, Logger: s.Logger}
	mux.HandleFunc("GET /api/devices", dh.List)
	mux.HandleFunc("POST /api/devices", dh.Create)
	mux.HandleFunc("GET /api/devices/{id}", dh.Get)
	mux.HandleFunc("PATCH /api/devices/{id}", dh.Patch)

	if s.People != nil {
		ph := &PeopleHandler{Store: s.People}
		mux.HandleFunc("GET /api/people", ph.List)
		mux.HandleFunc("POST /api/people", ph.Create)
		mux.HandleFunc("GET /api/people/{id}", ph.Get)
	}

	if s.PolicyStore != nil && s.Resolver != nil {
		polh := &PolicyHandler{Store: s.PolicyStore, Resolver: s.Resolver}
		mux.HandleFunc("GET /api/people/{id}/policy", polh.GetPersonPolicy)
		mux.HandleFunc("PUT /api/people/{id}/policy", polh.PutPersonPolicy)
		mux.HandleFunc("GET /api/devices/{id}/override", polh.GetDeviceOverride)
		mux.HandleFunc("PUT /api/devices/{id}/override", polh.PutDeviceOverride)
		mux.HandleFunc("GET /api/devices/{id}/resolved", polh.Resolved)
	}

	if s.Coordinator != nil {
		eh := &EnforceHandler{Coordinator: s.Coordinator}
		mux.HandleFunc("POST /api/devices/{id}/enforce", eh.Trigger)
	}

	if s.Events != nil {
		rh := &ReportsHandler{Store: s.Events}
		mux.HandleFunc("GET /api/reports/screen-time", rh.ScreenTime)
		mux.HandleFunc("GET /api/reports/by-device", rh.ByDevice)
		mux.HandleFunc("GET /api/reports/blocks", rh.Blocks)
		mux.HandleFunc("GET /api/events", rh.Events)
	}

	if s.Auth != nil {
		ah := &AuthHandler{Store: s.Auth}
		mux.HandleFunc("GET  /api/auth/me", ah.Me)
		mux.HandleFunc("POST /api/auth/setup", ah.Setup)
		mux.HandleFunc("POST /api/auth/login", ah.Login)
		mux.HandleFunc("POST /api/auth/logout", ah.Logout)
	}

	if s.AgentBackend != nil && s.Coordinator != nil && s.Resolver != nil {
		gh := &AgentHandler{
			Registry:    s.Registry,
			Coordinator: s.Coordinator,
			Resolver:    s.Resolver,
			Events:      s.EventLogger,
			Agent:       s.AgentBackend,
		}
		mux.HandleFunc("POST /api/agent/register", gh.Register)
		mux.HandleFunc("POST /api/agent/checkin", gh.Checkin)
	}

	mux.Handle("GET /", spaHandler(s.Auth))

	if s.Auth != nil {
		return auth.AuthGate(s.Auth, mux)
	}
	return mux
}
