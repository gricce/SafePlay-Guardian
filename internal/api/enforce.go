package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gricce/SafePlay-Guardian/internal/device"
	"github.com/gricce/SafePlay-Guardian/internal/enforce"
)

type EnforceHandler struct {
	Coordinator *enforce.Coordinator
}

// Trigger runs the enforcement coordinator for a device. ?at=RFC3339 lets the
// dashboard preview enforcement for a future moment using the same code path
// the production scheduler will use.
func (h *EnforceHandler) Trigger(w http.ResponseWriter, r *http.Request) {
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
	res, err := h.Coordinator.Enforce(r.Context(), id, at)
	if errors.Is(err, device.ErrNotFound) {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "enforce")
		return
	}
	writeJSON(w, http.StatusOK, res)
}
