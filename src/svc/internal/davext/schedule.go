package davext

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/schedule"
)

var (
	fbStart = regexp.MustCompile(`(?is)<(?:[a-z0-9]+:)?time-range[^>]*\bstart="([^"]+)"`)
	fbEnd   = regexp.MustCompile(`(?is)<(?:[a-z0-9]+:)?time-range[^>]*\bend="([^"]+)"`)
)

func (h *Handler) handleFreeBusy(w http.ResponseWriter, r *http.Request, body []byte) {
	p, ok := principalOrDeny(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	col, ok := parseCollection(r.URL.Path)
	if !ok || col.Kind != "calendar" || !strings.EqualFold(col.Username, p.Username) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	start, end := parseTimeRangeXML(body)
	if start.IsZero() {
		start = time.Now().UTC()
	}
	if end.IsZero() || !end.After(start) {
		end = start.Add(7 * 24 * time.Hour)
	}
	c, err := h.Store.CalendarBySlug(r.Context(), p.ID, col.Slug)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	periods, err := schedule.BusyForCalendar(r.Context(), h.Store, c.ID, start, end)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	u, err := h.Store.UserByID(r.Context(), p.ID)
	if err != nil {
		http.Error(w, "user", http.StatusInternalServerError)
		return
	}
	ics := icsutil.EncodeFreeBusy("fb-"+c.Slug, schedule.LocalMailbox(u.Username), start, end, periods)
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	addScheduleCapability(w.Header())
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(ics))
}

func parseTimeRangeXML(body []byte) (time.Time, time.Time) {
	var start, end time.Time
	if m := fbStart.FindSubmatch(body); len(m) == 2 {
		start, _ = icsutil.ParseICSTime(string(m[1]))
	}
	if m := fbEnd.FindSubmatch(body); len(m) == 2 {
		end, _ = icsutil.ParseICSTime(string(m[1]))
	}
	return start, end
}

func (h *Handler) handleOutbox(w http.ResponseWriter, r *http.Request, body []byte) {
	p, ok := principalOrDeny(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	col, ok := parseCollection(r.URL.Path)
	if !ok || !strings.EqualFold(col.Username, p.Username) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	u, err := h.Store.UserByID(r.Context(), p.ID)
	if err != nil {
		http.Error(w, "user", http.StatusInternalServerError)
		return
	}
	results, err := schedule.HandleOutboxPOST(r.Context(), h.Store, u, string(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<C:schedule-response xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">`)
	for _, res := range results {
		b.WriteString(`<C:response><C:recipient><D:href>`)
		davpath.WriteXML(&b, res.Recipient)
		b.WriteString(`</D:href></C:recipient><C:request-status>`)
		davpath.WriteXML(&b, res.Status)
		b.WriteString(`</C:request-status>`)
		if res.CalendarData != "" {
			b.WriteString(`<C:calendar-data>`)
			davpath.WriteXML(&b, res.CalendarData)
			b.WriteString(`</C:calendar-data>`)
		}
		b.WriteString(`</C:response>`)
	}
	b.WriteString(`</C:schedule-response>`)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	addScheduleCapability(w.Header())
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}
