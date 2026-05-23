package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gricce/SafePlay-Guardian/internal/device"
)

type DeviceHandler struct {
	Registry device.Registry
	Logger   *slog.Logger
}

type createDeviceRequest struct {
	Label    string  `json:"label"`
	Kind     string  `json:"kind"`
	PersonID *string `json:"person_id,omitempty"`
}

func (h *DeviceHandler) List(w http.ResponseWriter, r *http.Request) {
	f := device.Filter{}
	if s := r.URL.Query().Get("state"); s != "" {
		f.State = device.DeviceState(s)
	}
	devices, err := h.Registry.List(r.Context(), f)
	if err != nil {
		h.serverError(w, "list devices", err)
		return
	}
	if devices == nil {
		devices = []device.Device{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

func (h *DeviceHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Label = strings.TrimSpace(req.Label)
	req.Kind = strings.TrimSpace(req.Kind)
	if req.Label == "" {
		writeError(w, http.StatusBadRequest, "label is required")
		return
	}
	if req.Kind == "" {
		req.Kind = "unknown"
	}
	created, err := h.Registry.Upsert(r.Context(), device.Device{
		Label:    req.Label,
		Kind:     req.Kind,
		PersonID: req.PersonID,
		State:    device.StateActive,
	})
	if err != nil {
		h.serverError(w, "create device", err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *DeviceHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := h.Registry.Get(r.Context(), id)
	if errors.Is(err, device.ErrNotFound) {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		h.serverError(w, "get device", err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

type patchDeviceRequest struct {
	Label    *string `json:"label,omitempty"`
	Kind     *string `json:"kind,omitempty"`
	PersonID *string `json:"person_id,omitempty"`
	State    *string `json:"state,omitempty"`
}

// Patch applies a partial update. Used by the dashboard to claim a discovered
// device (set state=active, assign person, rename). Unspecified fields keep
// their current values.
func (h *DeviceHandler) Patch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := h.Registry.Get(r.Context(), id)
	if errors.Is(err, device.ErrNotFound) {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		h.serverError(w, "get device", err)
		return
	}

	var req patchDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Label != nil {
		if v := strings.TrimSpace(*req.Label); v != "" {
			existing.Label = v
		}
	}
	if req.Kind != nil {
		if v := strings.TrimSpace(*req.Kind); v != "" {
			existing.Kind = v
		}
	}
	if req.PersonID != nil {
		existing.PersonID = req.PersonID
	}
	if req.State != nil {
		switch device.DeviceState(*req.State) {
		case device.StateDiscovered, device.StateActive, device.StatePaused, device.StateRetired:
			existing.State = device.DeviceState(*req.State)
		default:
			writeError(w, http.StatusBadRequest, "invalid state")
			return
		}
	}

	updated, err := h.Registry.Upsert(r.Context(), *existing)
	if err != nil {
		h.serverError(w, "patch device", err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *DeviceHandler) serverError(w http.ResponseWriter, op string, err error) {
	if h.Logger != nil {
		h.Logger.Error("api error", "op", op, "err", err)
	}
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
