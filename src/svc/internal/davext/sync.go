package davext

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/schedule"
	"github.com/devcoons/dcalcon/internal/storage"
)

var syncTokenText = regexp.MustCompile(`(?is)<(?:[a-z0-9]+:)?sync-token(?:\s[^>]*)?>([^<]*)</(?:[a-z0-9]+:)?sync-token>`)

func (h *Handler) handleSync(w http.ResponseWriter, r *http.Request, body []byte) {
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
	token := ""
	if m := syncTokenText.FindSubmatch(body); len(m) == 2 {
		token = strings.TrimSpace(string(m[1]))
	}
	since, valid := storage.ParseSyncToken(token)
	if !valid {
		writeValidSyncToken(w)
		return
	}

	var (
		kind  string
		id    int64
		base  string
		items []hrefETag
	)
	ctx := r.Context()
	switch col.Kind {
	case "calendar":
		c, err := h.Store.CalendarBySlug(ctx, p.ID, col.Slug)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		kind, id, base = "calendar", c.ID, davpath.CalendarPath(p.Username, c.Slug)
		list, err := h.Store.ListCalendarObjectRefs(ctx, c.ID, "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, o := range list {
			items = append(items, hrefETag{href: o.Href, etag: o.ETag})
		}
	case "addressbook":
		if col.Slug == schedule.PeopleBookSlug {
			if err := schedule.RefreshPeopleBook(ctx, h.Store, p.ID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		a, err := h.Store.AddressBookBySlug(ctx, p.ID, col.Slug)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		kind, id, base = "addressbook", a.ID, davpath.AddressBookPath(p.Username, a.Slug)
		list, err := h.Store.ListAddressObjectRefs(ctx, a.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, o := range list {
			items = append(items, hrefETag{href: o.Href, etag: o.ETag})
		}
	default:
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	latest, err := h.Store.LatestChangeID(ctx, kind, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if token != "" && since > latest {
		writeValidSyncToken(w)
		return
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<D:multistatus xmlns:D="DAV:">`)

	if token == "" {
		for _, it := range items {
			writeMember(&b, davpath.ObjectPath(base, it.href), it.etag, false)
		}
	} else {
		changes, err := h.Store.ChangesSince(ctx, kind, id, since)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		etags := map[string]string{}
		for _, it := range items {
			etags[it.href] = it.etag
		}
		for _, ch := range changes {
			if ch.Deleted {
				writeMember(&b, davpath.ObjectPath(base, ch.Href), "", true)
				continue
			}
			writeMember(&b, davpath.ObjectPath(base, ch.Href), etags[ch.Href], false)
		}
	}

	b.WriteString(`<D:sync-token>`)
	davpath.WriteXML(&b, storage.SyncToken(latest))
	b.WriteString(`</D:sync-token></D:multistatus>`)

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	addScheduleCapability(w.Header())
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write([]byte(b.String()))
}

type hrefETag struct {
	href, etag string
}

func writeMember(b *strings.Builder, href, etag string, deleted bool) {
	b.WriteString(`<D:response><D:href>`)
	davpath.WriteXML(b, href)
	b.WriteString(`</D:href>`)
	if deleted {
		b.WriteString(`<D:status>HTTP/1.1 404 Not Found</D:status>`)
	} else {
		quoted := etag
		if quoted != "" && !strings.HasPrefix(quoted, `"`) {
			quoted = `"` + quoted + `"`
		}
		b.WriteString(`<D:propstat><D:prop><D:getetag>`)
		davpath.WriteXML(b, quoted)
		b.WriteString(`</D:getetag></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat>`)
	}
	b.WriteString(`</D:response>`)
}

func writeValidSyncToken(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><D:error xmlns:D="DAV:"><D:valid-sync-token/></D:error>`)
}
