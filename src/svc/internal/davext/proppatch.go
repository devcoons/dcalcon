package davext

import (
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/devcoons/dcalcon/internal/davpath"
)

var colorText = regexp.MustCompile(`(?is)<(?:[a-z0-9]+:)?calendar-color(?:\s[^>]*)?>([^<]*)</(?:[a-z0-9]+:)?calendar-color>`)
var displayNameText = regexp.MustCompile(`(?is)<(?:[a-z0-9]+:)?displayname(?:\s[^>]*)?>([^<]*)</(?:[a-z0-9]+:)?displayname>`)
var calendarDescText = regexp.MustCompile(`(?is)<(?:[a-z0-9]+:)?calendar-description(?:\s[^>]*)?>([^<]*)</(?:[a-z0-9]+:)?calendar-description>`)

func (h *Handler) handlePropPatch(w http.ResponseWriter, r *http.Request) bool {
	col, ok := parseCollection(r.URL.Path)
	if !ok || col.Kind != "calendar" {
		return false
	}
	p, ok := principalOrDeny(r)
	if !ok || !strings.EqualFold(col.Username, p.Username) {
		return false
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return false
	}
	var name, color *string
	if m := colorText.FindSubmatch(body); len(m) == 2 {
		s := strings.TrimSpace(string(m[1]))
		color = &s
	}
	if m := displayNameText.FindSubmatch(body); len(m) == 2 {
		s := strings.TrimSpace(string(m[1]))
		name = &s
	}
	if name == nil && color == nil {
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		return false
	}
	if err := h.Store.PatchCalendar(r.Context(), p.ID, col.Slug, name, nil, color); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	href := r.URL.Path
	if !strings.HasSuffix(href, "/") {
		href += "/"
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<D:multistatus xmlns:D="DAV:" xmlns:ICAL="http://apple.com/ns/ical/">`)
	b.WriteString(`<D:response><D:href>`)
	davpath.WriteXML(&b, href)
	b.WriteString(`</D:href><D:propstat><D:prop>`)
	if name != nil {
		b.WriteString(`<D:displayname/>`)
	}
	if color != nil {
		b.WriteString(`<ICAL:calendar-color/>`)
	}
	b.WriteString(`</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response></D:multistatus>`)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write([]byte(b.String()))
	return true
}
