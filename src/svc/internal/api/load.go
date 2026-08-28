package api

import (
	"net/http"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/storage"
)

func (h *Handler) calendarByParam(w http.ResponseWriter, r *http.Request, param string) (*storage.Calendar, bool) {
	p := auth.MustPrincipal(r.Context())
	id, ok := pathID(r, param)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "calendar")
		return nil, false
	}
	c, err := h.Store.CalendarByID(r.Context(), p.ID, id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "calendar")
		return nil, false
	}
	return c, true
}

func (h *Handler) calendarFor(w http.ResponseWriter, r *http.Request) (*storage.Calendar, bool) {
	return h.calendarByParam(w, r, "id")
}

func (h *Handler) calendarWritable(w http.ResponseWriter, r *http.Request) (*storage.Calendar, bool) {
	c, ok := h.calendarFor(w, r)
	if !ok {
		return nil, false
	}
	if writeDenied(w, c) {
		return nil, false
	}
	return c, true
}

func (h *Handler) ownedCalendar(w http.ResponseWriter, r *http.Request) (*storage.Calendar, bool) {
	c, ok := h.calendarFor(w, r)
	if !ok {
		return nil, false
	}
	if !c.IsOwner() {
		httpx.Error(w, http.StatusNotFound, "calendar")
		return nil, false
	}
	return c, true
}

func (h *Handler) personalOwnedCalendar(w http.ResponseWriter, r *http.Request) (*storage.Calendar, bool) {
	c, ok := h.ownedCalendar(w, r)
	if !ok {
		return nil, false
	}
	if c.Kind != "personal" {
		httpx.Error(w, http.StatusBadRequest, "webcal is only for personal calendars")
		return nil, false
	}
	return c, true
}
