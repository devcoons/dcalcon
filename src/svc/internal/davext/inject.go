package davext

import (
	"bytes"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/devcoons/dcalcon/internal/storage"
)

var hrefRe = regexp.MustCompile(`(?is)<(?:[a-z0-9]+:)?href(?:\s[^>]*)?>\s*([^<]+)\s*</(?:[a-z0-9]+:)?href>`)
var propCloseRe = regexp.MustCompile(`(?i)</(?:[a-z0-9]+:)?prop>`)
var resourceTypeCloseRe = regexp.MustCompile(`(?i)</(?:[a-z0-9]+:)?resourcetype>`)

func (h *Handler) inject(r *http.Request, body []byte) ([]byte, error) {
	if !bytes.Contains(bytes.ToLower(body), []byte("multistatus")) {
		return body, nil
	}
	p, ok := principalOrDeny(r)
	if !ok {
		return body, nil
	}
	s := string(body)
	matches := hrefRe.FindAllStringSubmatchIndex(s, -1)
	type job struct {
		href string
		at   int
	}
	var jobs []job
	seen := map[string]bool{}
	for _, m := range matches {
		raw := html.UnescapeString(strings.TrimSpace(s[m[2]:m[3]]))
		path := hrefPath(raw)
		if seen[path] {
			continue
		}
		col, ok := parseCollection(path)
		if !ok || !strings.EqualFold(col.Username, p.Username) {
			continue
		}
		seen[path] = true
		jobs = append(jobs, job{href: path, at: m[1]})
	}
	if len(jobs) == 0 {
		return body, nil
	}
	out := s
	for i := len(jobs) - 1; i >= 0; i-- {
		j := jobs[i]
		col, _ := parseCollection(j.href)
		snippet, extraRT := h.collectionSnippet(r, p.ID, col)
		if snippet == "" {
			continue
		}
		patched, ok := insertBeforeClose(out, j.at, propCloseRe, snippet)
		if ok {
			out = patched
		}
		if extraRT != "" {
			patched, ok = insertBeforeClose(out, j.at, resourceTypeCloseRe, extraRT)
			if ok {
				out = patched
			}
		}
	}
	return []byte(out), nil
}

func (h *Handler) collectionSnippet(r *http.Request, userID int64, col collectionRef) (string, string) {
	ctx := r.Context()
	switch col.Kind {
	case "calendar":
		c, err := h.Store.CalendarBySlug(ctx, userID, col.Slug)
		if err != nil {
			return "", ""
		}
		tokenID, _ := h.Store.LatestChangeID(ctx, "calendar", c.ID)
		color := html.EscapeString(c.Color)
		ctag := html.EscapeString(fmt.Sprintf("%d", c.CTag))
		token := html.EscapeString(storage.SyncToken(tokenID))
		snip := `<CS:getctag xmlns:CS="http://calendarserver.org/ns/">` + ctag + `</CS:getctag>` +
			`<ICAL:calendar-color xmlns:ICAL="http://apple.com/ns/ical/">` + color + `</ICAL:calendar-color>` +
			`<D:sync-token xmlns:D="DAV:">` + token + `</D:sync-token>` +
			componentSetXML(c.Kind)
		shares, _ := h.Store.ListShares(ctx, c.ID)
		viewer := ""
		if p, ok := principalOrDeny(r); ok {
			viewer = p.Username
		}
		snip += calendarACLSnippet(c, viewer, shares)
		var rt string
		switch c.Kind {
		case "inbox":
			rt = `<C:schedule-inbox xmlns:C="urn:ietf:params:xml:ns:caldav"/>`
		case "outbox":
			rt = `<C:schedule-outbox xmlns:C="urn:ietf:params:xml:ns:caldav"/>`
		}
		return snip, rt
	case "addressbook":
		a, err := h.Store.AddressBookBySlug(ctx, userID, col.Slug)
		if err != nil {
			return "", ""
		}
		tokenID, _ := h.Store.LatestChangeID(ctx, "addressbook", a.ID)
		ctag := html.EscapeString(fmt.Sprintf("%d", a.CTag))
		token := html.EscapeString(storage.SyncToken(tokenID))
		snip := `<CS:getctag xmlns:CS="http://calendarserver.org/ns/">` + ctag + `</CS:getctag>` +
			`<D:sync-token xmlns:D="DAV:">` + token + `</D:sync-token>`
		return snip, ""
	}
	return "", ""
}

func insertBeforeClose(s string, from int, closeRe *regexp.Regexp, snippet string) (string, bool) {
	if from < 0 || from > len(s) {
		return s, false
	}
	rest := s[from:]
	loc := closeRe.FindStringIndex(rest)
	if loc == nil {
		return s, false
	}
	at := from + loc[0]
	chunk := strings.ToLower(s[from:at])
	if strings.Contains(snippet, "getctag") && strings.Contains(chunk, "getctag") {
		return s, false
	}
	return s[:at] + snippet + s[at:], true
}

func hrefPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		raw = u.Path
	}
	if !strings.HasSuffix(raw, "/") && !strings.Contains(strings.TrimPrefix(raw, "/dav/"), ".") {
		raw += "/"
	}
	return raw
}

func componentSetXML(kind string) string {
	comps := []string{"VEVENT", "VTODO"}
	if kind == "inbox" || kind == "outbox" {
		comps = []string{"VEVENT", "VTODO", "VFREEBUSY"}
	}
	var b strings.Builder
	b.WriteString(`<C:supported-calendar-component-set xmlns:C="urn:ietf:params:xml:ns:caldav">`)
	for _, n := range comps {
		b.WriteString(`<C:comp name="`)
		b.WriteString(n)
		b.WriteString(`"/>`)
	}
	b.WriteString(`</C:supported-calendar-component-set>`)
	return b.String()
}
