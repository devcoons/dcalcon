package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/storage"
	"github.com/google/uuid"
)

type taskDTO struct {
	Href        string               `json:"href"`
	UID         string               `json:"uid"`
	ETag        string               `json:"etag"`
	Summary     string               `json:"summary"`
	Description string               `json:"description"`
	Due         string               `json:"due"`
	Status      string               `json:"status"`
	CalendarID  int64                `json:"calendar_id"`
	Calendar    string               `json:"calendar_name"`
	Attachments []storage.Attachment `json:"attachments"`
}

type taskWrite struct {
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Due         string `json:"due"`
	Status      string `json:"status"`
}

func toTaskDTO(o storage.CalendarObject, cal storage.Calendar, atts []storage.Attachment) taskDTO {
	f := icsutil.TodoFieldsFromICS(o.ICS)
	due := f.Due
	if due == "" {
		due = o.DTEnd
		if due == "" {
			due = o.DTStart
		}
	}
	st := f.Status
	if st == "" {
		st = "NEEDS-ACTION"
	}
	if atts == nil {
		atts = []storage.Attachment{}
	}
	return taskDTO{
		Href: o.Href, UID: o.UID, ETag: o.ETag, Summary: o.Summary,
		Description: f.Description,
		Due:         due, Status: st, CalendarID: cal.ID, Calendar: cal.Name,
		Attachments: atts,
	}
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	cals, err := h.Store.ListCalendars(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]taskDTO, 0)
	for _, c := range cals {
		if c.Kind == "inbox" || c.Kind == "outbox" {
			continue
		}
		list, err := h.Store.ListCalendarObjectsByComponent(r.Context(), c.ID, "VTODO")
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		atts := h.attachmentMap(r.Context(), c.ID)
		for _, o := range list {
			out = append(out, toTaskDTO(o, c, atts[o.Href]))
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) listCalendarTasks(w http.ResponseWriter, r *http.Request) {
	c, ok := h.calendarFor(w, r)
	if !ok {
		return
	}
	list, err := h.Store.ListCalendarObjectsByComponent(r.Context(), c.ID, "VTODO")
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	atts := h.attachmentMap(r.Context(), c.ID)
	out := make([]taskDTO, 0, len(list))
	for _, o := range list {
		out = append(out, toTaskDTO(o, *c, atts[o.Href]))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) getTask(w http.ResponseWriter, r *http.Request) {
	c, ok := h.calendarFor(w, r)
	if !ok {
		return
	}
	href, ok := objectHref(w, r, "task")
	if !ok {
		return
	}
	o, err := h.Store.CalendarObjectByHref(r.Context(), c.ID, href)
	if err != nil || !isTodo(*o) {
		httpx.Error(w, http.StatusNotFound, "task")
		return
	}
	httpx.JSON(w, http.StatusOK, toTaskDTO(*o, *c, h.attachmentsFor(r.Context(), c.ID, href)))
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	c, ok := h.calendarWritable(w, r)
	if !ok {
		return
	}
	var body taskWrite
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Summary = strings.TrimSpace(body.Summary)
	if body.Summary == "" {
		httpx.Error(w, http.StatusBadRequest, "summary is required")
		return
	}
	uid := uuid.NewString()
	due := icsutil.CompactICSTime(body.Due)
	ics := icsutil.NewTodoICS(uid, body.Summary, due, body.Description, body.Status)
	href := uid + ".ics"
	etag := icsutil.ETag(ics)
	if err := h.Store.UpsertCalendarObject(r.Context(), c.ID, href, uid, etag, "VTODO", ics, "", due, body.Summary); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"href": href, "uid": uid, "etag": etag})
}

func (h *Handler) updateTask(w http.ResponseWriter, r *http.Request) {
	c, ok := h.calendarWritable(w, r)
	if !ok {
		return
	}
	href, ok := objectHref(w, r, "task")
	if !ok {
		return
	}
	existing, err := h.Store.CalendarObjectByHref(r.Context(), c.ID, href)
	if err != nil || !isTodo(*existing) {
		httpx.Error(w, http.StatusNotFound, "task")
		return
	}
	var body taskWrite
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	due := icsutil.CompactICSTime(body.Due)
	ics, err := icsutil.UpdateTodoICS(existing.ICS, body.Summary, due, body.Description, body.Status)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "could not update task")
		return
	}
	etag := icsutil.ETag(ics)
	if err := h.Store.UpsertCalendarObject(r.Context(), c.ID, href, existing.UID, etag, "VTODO", ics, "", due, icsutil.SummaryFromICS(ics)); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	o, err := h.Store.CalendarObjectByHref(r.Context(), c.ID, href)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, toTaskDTO(*o, *c, h.attachmentsFor(r.Context(), c.ID, href)))
}

func (h *Handler) deleteTask(w http.ResponseWriter, r *http.Request) {
	c, ok := h.calendarWritable(w, r)
	if !ok {
		return
	}
	href, ok := objectHref(w, r, "task")
	if !ok {
		return
	}
	existing, err := h.Store.CalendarObjectByHref(r.Context(), c.ID, href)
	if err != nil || !isTodo(*existing) {
		httpx.Error(w, http.StatusNotFound, "task")
		return
	}
	if err := h.Store.DeleteCalendarObject(r.Context(), c.ID, href); err != nil {
		httpx.Error(w, http.StatusNotFound, "task")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
