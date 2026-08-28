package davext

import (
	"errors"
	"html"
	"io"
	"net/http"
	"strings"

	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/storage"
)

func (h *Handler) handleMkCalendar(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "MKCALENDAR" && r.Method != "MKCOL" {
		return false
	}
	col, ok := parseCollection(r.URL.Path)
	if !ok || col.Kind != "calendar" {
		return false
	}
	p, ok := principalOrDeny(r)
	if !ok || !strings.EqualFold(col.Username, p.Username) {
		return false
	}
	if !davpath.ValidSlug(col.Slug) {
		http.Error(w, "invalid calendar name", http.StatusBadRequest)
		return true
	}
	if _, err := h.Store.CalendarBySlug(r.Context(), p.ID, col.Slug); err == nil {
		http.Error(w, "collection exists", http.StatusMethodNotAllowed)
		return true
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return true
	}
	if dangerousXML(body) {
		http.Error(w, "invalid xml", http.StatusBadRequest)
		return true
	}
	name := col.Slug
	if m := displayNameText.FindSubmatch(body); len(m) == 2 {
		if s := strings.TrimSpace(html.UnescapeString(string(m[1]))); s != "" {
			name = s
		}
	}
	desc := ""
	if m := calendarDescText.FindSubmatch(body); len(m) == 2 {
		desc = strings.TrimSpace(html.UnescapeString(string(m[1])))
	}
	color := ""
	if m := colorText.FindSubmatch(body); len(m) == 2 {
		color = normalizeColor(string(m[1]))
	}
	if _, err := h.Store.CreateCalendar(r.Context(), p.ID, col.Slug, name, desc, color, "personal", false); err != nil {
		if errors.Is(err, storage.ErrConflict) {
			http.Error(w, "collection exists", http.StatusConflict)
			return true
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	w.Header().Set("DAV", "1, 3, access-control, calendar-access, addressbook, calendar-auto-schedule")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusCreated)
	return true
}

func normalizeColor(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(s, "#") {
		s = "#" + s
	}
	return s
}
