package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gricce/SafePlay-Guardian/internal/person"
)

type PeopleHandler struct {
	Store *person.Store
}

type createPersonRequest struct {
	Name string `json:"name"`
}

func (h *PeopleHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list people")
		return
	}
	if list == nil {
		list = []person.Person{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": list})
}

func (h *PeopleHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createPersonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	p, err := h.Store.Create(r.Context(), req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *PeopleHandler) Get(w http.ResponseWriter, r *http.Request) {
	p, err := h.Store.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, person.ErrNotFound) {
		writeError(w, http.StatusNotFound, "person not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get person")
		return
	}
	writeJSON(w, http.StatusOK, p)
}
