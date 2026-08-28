package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/schedule"
	"github.com/devcoons/dcalcon/internal/storage"
	"github.com/go-chi/chi/v5"
)

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return id, err == nil && id > 0
}

func requireID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, ok := pathID(r, name)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func objectHref(w http.ResponseWriter, r *http.Request, kind string) (string, bool) {
	href := chi.URLParam(r, "href")
	if err := davpath.CheckObjectHref(href); err != nil {
		httpx.Error(w, http.StatusNotFound, kind)
		return "", false
	}
	return href, true
}

func (h *Handler) directory(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	list, err := h.Store.Directory(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []storage.DirectoryUser{}
	}
	for i := range list {
		list[i].LocalEmail = schedule.LocalMailbox(list[i].Username)
	}
	httpx.JSON(w, http.StatusOK, list)
}

func (h *Handler) patchCalendar(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, err := h.Store.CalendarByID(r.Context(), p.ID, id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "calendar")
		return
	}
	if !c.IsOwner() {
		httpx.Error(w, http.StatusForbidden, "only the owner can rename this calendar")
		return
	}
	if c.Kind != "personal" {
		httpx.Error(w, http.StatusBadRequest, "system calendars cannot be renamed")
		return
	}
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Color       string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.Store.PatchCalendarMeta(r.Context(), c.ID, body.Name, body.Description, body.Color); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	c, err = h.Store.CalendarByID(r.Context(), p.ID, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

func (h *Handler) deleteCalendar(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, err := h.Store.CalendarByID(r.Context(), p.ID, id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "calendar")
		return
	}
	if !c.IsOwner() {
		httpx.Error(w, http.StatusForbidden, "only the owner can delete this calendar")
		return
	}
	if c.Kind != "personal" {
		httpx.Error(w, http.StatusBadRequest, "system calendars cannot be deleted")
		return
	}
	n, err := h.Store.CountPersonalCalendars(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n <= 1 {
		httpx.Error(w, http.StatusBadRequest, "keep at least one personal calendar")
		return
	}
	if err := h.Store.DeleteCalendar(r.Context(), p.ID, c.ID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) listShares(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, err := h.Store.CalendarByID(r.Context(), p.ID, id)
	if err != nil || !c.IsOwner() {
		httpx.Error(w, http.StatusNotFound, "calendar")
		return
	}
	list, err := h.Store.ListShares(r.Context(), c.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []storage.CalendarShare{}
	}
	httpx.JSON(w, http.StatusOK, list)
}

func (h *Handler) createShare(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, err := h.Store.CalendarByID(r.Context(), p.ID, id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "calendar")
		return
	}
	if !c.IsOwner() {
		httpx.Error(w, http.StatusForbidden, "only the owner can share this calendar")
		return
	}
	if c.Kind == "inbox" || c.Kind == "outbox" || c.Kind == "important_dates" {
		httpx.Error(w, http.StatusBadRequest, "this calendar cannot be shared")
		return
	}
	var body struct {
		Username string `json:"username"`
		Access   string `json:"access"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Access = strings.ToLower(strings.TrimSpace(body.Access))
	if body.Access == "" {
		body.Access = "read"
	}
	if body.Access != "read" && body.Access != "write" {
		httpx.Error(w, http.StatusBadRequest, "access must be read or write")
		return
	}
	name := strings.TrimSpace(body.Username)
	if name == "" || strings.EqualFold(name, p.Username) {
		httpx.Error(w, http.StatusBadRequest, "choose another local user")
		return
	}
	u, err := h.Store.UserByUsername(r.Context(), name)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "user not found")
		return
	}
	if u.Status != "active" {
		httpx.Error(w, http.StatusBadRequest, "user is disabled")
		return
	}
	if err := h.Store.UpsertShare(r.Context(), c.ID, u.ID, body.Access); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "share.create", c.Slug+" -> "+u.Username+" "+body.Access)
	list, err := h.Store.ListShares(r.Context(), c.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, list)
}

func (h *Handler) deleteShare(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	uid, ok := pathID(r, "userId")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid user")
		return
	}
	c, err := h.Store.CalendarByID(r.Context(), p.ID, id)
	if err != nil || !c.IsOwner() {
		httpx.Error(w, http.StatusNotFound, "calendar")
		return
	}
	if err := h.Store.DeleteShare(r.Context(), c.ID, uid); err != nil {
		httpx.Error(w, http.StatusNotFound, "share")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
