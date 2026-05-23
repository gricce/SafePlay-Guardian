package api

import (
	"net/http"
	"strconv"

	"github.com/gricce/SafePlay-Guardian/internal/event"
)

type ReportsHandler struct {
	Store *event.Store
}

func (h *ReportsHandler) ScreenTime(w http.ResponseWriter, r *http.Request) {
	// person_id is optional now — empty means "aggregate the whole home",
	// which is what /reports' "All children" tab uses.
	personID := r.URL.Query().Get("person_id")
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	rows, err := h.Store.ScreenTimePerDay(r.Context(), personID, days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "screen-time")
		return
	}
	if rows == nil {
		rows = []event.ScreenTimeRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": rows})
}

func (h *ReportsHandler) ByDevice(w http.ResponseWriter, r *http.Request) {
	personID := r.URL.Query().Get("person_id")
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	rows, err := h.Store.UsageByDevice(r.Context(), personID, days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "by-device")
		return
	}
	if rows == nil {
		rows = []event.DeviceUsageRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": rows})
}

func (h *ReportsHandler) Blocks(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := h.Store.TopBlocks(r.Context(), days, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "blocks")
		return
	}
	if rows == nil {
		rows = []event.BlockRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"blocks": rows})
}

func (h *ReportsHandler) Events(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	filter := event.RecentFilter{
		DeviceID: q.Get("device_id"),
		PersonID: q.Get("person_id"),
		Limit:    limit,
	}
	if k := q.Get("kind"); k != "" {
		filter.Kinds = []event.Kind{event.Kind(k)}
	}
	events, err := h.Store.Recent(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "events")
		return
	}
	if events == nil {
		events = []event.Event{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}
