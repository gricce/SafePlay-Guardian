package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gricce/SafePlay-Guardian/internal/device"
	"github.com/gricce/SafePlay-Guardian/internal/policy"
)

type PolicyHandler struct {
	Store    policy.Store
	Resolver *policy.Resolver
}

func (h *PolicyHandler) GetPersonPolicy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.Store.GetPersonPolicy(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read policy")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "no policy set")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *PolicyHandler) PutPersonPolicy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var p policy.Policy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.Store.SetPersonPolicy(r.Context(), id, &p); err != nil {
		writeError(w, http.StatusInternalServerError, "store policy")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *PolicyHandler) GetDeviceOverride(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	o, err := h.Store.GetDeviceOverride(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read override")
		return
	}
	if o == nil {
		writeError(w, http.StatusNotFound, "no override set")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *PolicyHandler) PutDeviceOverride(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var o policy.Override
	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.Store.SetDeviceOverride(r.Context(), id, &o); err != nil {
		writeError(w, http.StatusInternalServerError, "store override")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// Resolved returns the flattened policy for a device at a given time. ?at=RFC3339
// lets the parent (or a dashboard) preview enforcement for a future moment —
// the same path the eventual enforcement coordinator will call.
func (h *PolicyHandler) Resolved(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	at := time.Now().UTC()
	if v := r.URL.Query().Get("at"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "at must be RFC3339")
			return
		}
		at = t
	}
	resolved, err := h.Resolver.Resolve(r.Context(), id, at)
	if errors.Is(err, device.ErrNotFound) {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "resolve")
		return
	}
	writeJSON(w, http.StatusOK, resolved)
}
