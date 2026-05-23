package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gricce/SafePlay-Guardian/internal/device"
	"github.com/gricce/SafePlay-Guardian/internal/enforce"
	"github.com/gricce/SafePlay-Guardian/internal/enforce/agent"
	"github.com/gricce/SafePlay-Guardian/internal/event"
	"github.com/gricce/SafePlay-Guardian/internal/policy"
)

const checkinIntervalSeconds = 60

type AgentHandler struct {
	Registry    device.Registry
	Coordinator *enforce.Coordinator
	Resolver    *policy.Resolver
	Events      *event.Logger
	Agent       *agent.Backend
}

type registerRequest struct {
	DeviceID   string              `json:"device_id"`
	Label      string              `json:"label"`
	Kind       string              `json:"kind"`
	OS         string              `json:"os"`
	OSVersion  string              `json:"os_version"`
	Caps       []device.Capability `json:"caps"`
	MAC        string              `json:"mac"`
	Hostname   string              `json:"hostname"`
	AgentToken string              `json:"agent_token"` // optional pre-provisioned token to claim a manual device
}

type registerResponse struct {
	DeviceID               string `json:"device_id"`
	AgentToken             string `json:"agent_token"`
	CheckinIntervalSeconds int    `json:"checkin_interval_seconds"`
}

// Register is first contact. If the request carries a pre-provisioned token
// that matches a manually-added device, that device is claimed (Reconcile by
// AgentToken). Otherwise a fresh device is created and a token is issued.
// The returned token must be retained by the agent for all future check-ins.
func (h *AgentHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.DeviceID) == "" {
		writeError(w, http.StatusBadRequest, "device_id is required")
		return
	}
	obs := device.Observation{
		Source:     "agent",
		SeenAt:     time.Now().UTC(),
		AgentID:    req.DeviceID,
		AgentToken: strings.TrimSpace(req.AgentToken),
		Label:      req.Label,
		Kind:       req.Kind,
		MAC:        req.MAC,
		Hostname:   req.Hostname,
		Caps:       req.Caps,
	}
	coreID, err := h.Registry.Reconcile(r.Context(), obs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "reconcile")
		return
	}
	dev, err := h.Registry.Get(r.Context(), coreID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup")
		return
	}
	// Issue a token only if the device doesn't already have one. This makes
	// register idempotent — re-running an installer doesn't rotate the token.
	if dev.AgentToken == nil || *dev.AgentToken == "" {
		tok := newAgentToken()
		dev.AgentToken = &tok
		if _, err := h.Registry.Upsert(r.Context(), *dev); err != nil {
			writeError(w, http.StatusInternalServerError, "issue token")
			return
		}
	}
	writeJSON(w, http.StatusOK, registerResponse{
		DeviceID:               coreID,
		AgentToken:             *dev.AgentToken,
		CheckinIntervalSeconds: checkinIntervalSeconds,
	})
}

type agentEventBody struct {
	Kind  string         `json:"kind"`
	At    time.Time      `json:"at"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

type checkinRequest struct {
	DeviceID   string              `json:"device_id"`
	Identities map[string]string   `json:"identities,omitempty"`
	Caps       []device.Capability `json:"caps,omitempty"`
	AgentClock *time.Time          `json:"agent_clock,omitempty"`
	Events     []agentEventBody    `json:"events,omitempty"`
}

type checkinResponse struct {
	DeviceID               string                `json:"device_id"`
	At                     time.Time             `json:"at"`
	Policy                 policy.ResolvedPolicy `json:"policy"`
	CapsAssigned           []device.Capability   `json:"caps_assigned"`
	CheckinIntervalSeconds int                   `json:"checkin_interval_seconds"`
	ClockDriftSeconds      *float64              `json:"clock_drift_seconds,omitempty"`
}

// Checkin is the per-install periodic call. It authenticates via the agent
// token, reconciles the observation, ingests reported events, runs the
// coordinator, and returns the policy slice the agent should enforce.
func (h *AgentHandler) Checkin(w http.ResponseWriter, r *http.Request) {
	token := extractAgentToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing agent token")
		return
	}
	var req checkinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	obs := device.Observation{
		Source:     "agent",
		SeenAt:     time.Now().UTC(),
		AgentToken: token,
		AgentID:    req.DeviceID,
		Caps:       req.Caps,
	}
	if v, ok := req.Identities["mac"]; ok {
		obs.MAC = v
	}
	if v, ok := req.Identities["ip"]; ok {
		obs.IP = v
	}
	if v, ok := req.Identities["hostname"]; ok {
		obs.Hostname = v
	}
	coreID, err := h.Registry.Reconcile(r.Context(), obs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "reconcile")
		return
	}
	dev, err := h.Registry.Get(r.Context(), coreID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup")
		return
	}
	// Token must already belong to this device. Reconcile would have matched
	// on AgentToken first; if it didn't, the agent is presenting a wrong token.
	if dev.AgentToken == nil || *dev.AgentToken != token {
		writeError(w, http.StatusUnauthorized, "invalid agent token")
		return
	}

	// Ingest reported events. Logger applies monitoring-level redaction.
	if h.Events != nil {
		for _, e := range req.Events {
			k := event.Kind(e.Kind)
			if !k.Valid() {
				continue
			}
			_ = h.Events.Append(r.Context(), event.Event{
				Kind:     k,
				At:       e.At,
				DeviceID: coreID,
				PersonID: dev.PersonID,
				Attrs:    e.Attrs,
			})
		}
	}

	now := time.Now().UTC()
	if _, err := h.Coordinator.Enforce(r.Context(), coreID, now); err != nil {
		writeError(w, http.StatusInternalServerError, "enforce")
		return
	}

	resolved, err := h.Resolver.Resolve(r.Context(), coreID, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "resolve")
		return
	}

	capsAssigned := []device.Capability{}
	if h.Agent != nil {
		if pending := h.Agent.PullPending(coreID); pending != nil {
			capsAssigned = pending.Caps
		}
	}

	var drift *float64
	if req.AgentClock != nil {
		d := now.Sub(*req.AgentClock).Seconds()
		drift = &d
	}

	writeJSON(w, http.StatusOK, checkinResponse{
		DeviceID:               coreID,
		At:                     now,
		Policy:                 resolved,
		CapsAssigned:           capsAssigned,
		CheckinIntervalSeconds: checkinIntervalSeconds,
		ClockDriftSeconds:      drift,
	})
}

func extractAgentToken(r *http.Request) string {
	if v := r.Header.Get("Authorization"); strings.HasPrefix(v, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(v, "Bearer "))
	}
	return strings.TrimSpace(r.Header.Get("X-Agent-Token"))
}

func newAgentToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("crypto/rand: %w", err))
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
